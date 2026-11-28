// Package weblogin serves a one-time sign-in page, on the loopback interface
// or through usc.sh.
package weblogin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/site"
	"github.com/nsigel/usc-cli/weblogin/page"
)

const lifetime = 10 * time.Minute

type Options struct {
	// Auth supplies CredentialsFile, SessionFile, and DuoDeviceName; other fields are ignored.
	Auth auth.Options
	// Probe is the site opened to confirm a sign-in that needs no new Duo device.
	Probe site.Name
	// Relay is the site that serves the page and forwards its sealed requests
	// here, for a browser that cannot reach this machine. Empty serves the
	// page on the loopback interface.
	Relay string
}

type Started struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Server struct {
	listener net.Listener
	relay    *relay
	url      string
	token    string
	hosts    []string
	expires  time.Time
	flow     *flow
}

func Listen(options Options) (*Server, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	server := &Server{
		expires: time.Now().Add(lifetime).UTC().Truncate(time.Second),
		flow:    newFlow(options),
	}
	if options.Relay != "" {
		relay, err := newRelay(options.Relay, secret)
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := relay.register(ctx); err != nil {
			return nil, err
		}
		server.relay = relay
		server.url = relay.base + page.Path + "#" + base64.RawURLEncoding.EncodeToString(secret)
		return server, nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	server.listener = listener
	server.token = hex.EncodeToString(secret)
	server.hosts = []string{"127.0.0.1:" + port, "localhost:" + port}
	server.url = "http://" + server.hosts[0] + "/#" + server.token
	return server, nil
}

func (s *Server) Started() Started {
	return Started{URL: s.url, ExpiresAt: s.expires}
}

// Serve runs until sign-in finishes or the page expires.
func (s *Server) Serve(ctx context.Context) error {
	ctx, cancel := context.WithDeadline(ctx, s.expires)
	defer cancel()
	s.flow.ctx = ctx
	if s.relay != nil {
		s.relay.serve(ctx, s.flow.delivered, s.api)
	} else if err := s.serveLoopback(ctx); err != nil {
		return err
	}
	s.flow.workers.Wait()
	switch current := s.flow.snapshot(); current.Step {
	case stepDone:
		return nil
	case stepFailed:
		return errors.New(current.Error)
	}
	return errors.New("the sign-in page expired before sign-in finished")
}

func (s *Server) serveLoopback(ctx context.Context) error {
	server := &http.Server{Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second}
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(s.listener) }()
	select {
	case err := <-stopped:
		return err
	case <-ctx.Done():
	case <-s.flow.delivered:
	}
	shutdown, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	_ = server.Shutdown(shutdown)
	return nil
}

// api runs one request from the page, whichever way it arrived. It reports
// false for a request it cannot read.
func (s *Server) api(path string, body []byte) (state, bool) {
	switch path {
	case "/api/wait":
		return s.flow.snapshot(), true
	case "/api/state":
	case "/api/credentials":
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if json.Unmarshal(body, &input) != nil {
			return state{}, false
		}
		s.flow.submitCredentials(input.Username, input.Password)
	case "/api/method":
		var input struct {
			Type     string `json:"type"`
			DeviceID string `json:"device_id"`
			Code     string `json:"code"`
		}
		if json.Unmarshal(body, &input) != nil {
			return state{}, false
		}
		s.flow.chooseMethod(input.Type, input.DeviceID, input.Code)
	case "/api/code":
		var input struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(body, &input) != nil {
			return state{}, false
		}
		s.flow.submitCode(input.Code)
	default:
		return state{}, false
	}
	current := s.flow.snapshot()
	if current.Step == stepDone || current.Step == stepFailed {
		s.flow.release()
	}
	return current, true
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /", page.Handler())
	// healthz needs no token so agents can check the page is still serving.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Status    string    `json:"status"`
			ExpiresAt time.Time `json:"expires_at"`
		}{"ok", s.expires})
	})
	api := func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		if r.Method == http.MethodPost {
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
				return
			}
			var err error
			if body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 4096)); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
		}
		current, ok := s.api(r.URL.Path, body)
		if !ok {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(current)
	}
	mux.HandleFunc("GET /api/", api)
	mux.HandleFunc("POST /api/", api)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// An exact loopback Host defeats DNS rebinding.
		if !s.local(r.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		page.Secure(w.Header())
		if strings.HasPrefix(r.URL.Path, "/api/") && !s.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) local(host string) bool {
	for _, allowed := range s.hosts {
		if host == allowed {
			return true
		}
	}
	return false
}

func (s *Server) authorized(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" && !s.local(strings.TrimPrefix(origin, "http://")) {
		return false
	}
	token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return found && subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) == 1
}
