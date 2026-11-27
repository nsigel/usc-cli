// Package weblogin serves a one-time sign-in page on the loopback interface.
package weblogin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/site"
)

//go:embed page
var pageFiles embed.FS

const lifetime = 10 * time.Minute

type Options struct {
	// Auth supplies CredentialsFile, SessionFile, and DuoDeviceName; other fields are ignored.
	Auth auth.Options
	// Probe is the site opened to confirm a sign-in that needs no new Duo device.
	Probe site.Name
}

type Started struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Server struct {
	listener net.Listener
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
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	return &Server{
		listener: listener,
		token:    hex.EncodeToString(secret),
		hosts:    []string{"127.0.0.1:" + port, "localhost:" + port},
		expires:  time.Now().Add(lifetime).UTC().Truncate(time.Second),
		flow:     newFlow(options),
	}, nil
}

func (s *Server) Started() Started {
	return Started{URL: "http://" + s.hosts[0] + "/#" + s.token, ExpiresAt: s.expires}
}

// Serve runs until sign-in finishes or the page expires.
func (s *Server) Serve(ctx context.Context) error {
	ctx, cancel := context.WithDeadline(ctx, s.expires)
	defer cancel()
	s.flow.ctx = ctx
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
	s.flow.workers.Wait()
	switch current := s.flow.snapshot(); current.Step {
	case stepDone:
		return nil
	case stepFailed:
		return errors.New(current.Error)
	}
	return errors.New("the sign-in page expired before sign-in finished")
}

func (s *Server) handler() http.Handler {
	assets, err := fs.Sub(pageFiles, "page")
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(assets))
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, _ *http.Request) {
		s.writeState(w)
	})
	mux.HandleFunc("GET /api/wait", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.flow.snapshot())
	})
	mux.HandleFunc("POST /api/credentials", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if readJSON(w, r, &input) {
			s.flow.submitCredentials(input.Username, input.Password)
			s.writeState(w)
		}
	})
	mux.HandleFunc("POST /api/method", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Type     string `json:"type"`
			DeviceID string `json:"device_id"`
			Code     string `json:"code"`
		}
		if readJSON(w, r, &input) {
			s.flow.chooseMethod(input.Type, input.DeviceID, input.Code)
			s.writeState(w)
		}
	})
	mux.HandleFunc("POST /api/code", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Code string `json:"code"`
		}
		if readJSON(w, r, &input) {
			s.flow.submitCode(input.Code)
			s.writeState(w)
		}
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// An exact loopback Host defeats DNS rebinding.
		if !s.local(r.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		header := w.Header()
		header.Set("Cache-Control", "no-store")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
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

func (s *Server) writeState(w http.ResponseWriter) {
	current := s.flow.snapshot()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(current)
	if current.Step == stepDone || current.Step == stepFailed {
		s.flow.release()
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(into); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return false
	}
	return true
}
