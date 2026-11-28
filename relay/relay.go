// Package relay forwards sealed messages between the sign-in page on usc.sh
// and a usc process that the page cannot reach directly. It holds no keys and
// keeps nothing once a message is delivered.
//
// Channels live in memory, so a site runs one relay.
package relay

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	lifetime  = 15 * time.Minute
	pollWait  = 25 * time.Second
	replyWait = 10 * time.Second
	// ownerGrace is how long a channel lasts after its owner stops asking for
	// messages, which it does when its process dies.
	ownerGrace  = 10 * time.Second
	maxRequest  = 4 << 10
	maxReply    = 16 << 10
	maxChannels = 10000
	maxPerHost  = 10
	maxQueued   = 8
)

var channelName = regexp.MustCompile(`^[0-9a-f]{32}$`)

type message struct {
	id    string
	body  []byte
	reply chan []byte
}

type channel struct {
	owner  string
	host   string
	expiry *time.Timer
	queue  chan *message
	closed chan struct{}

	mu      sync.Mutex
	waiting map[string]*message
	polling int
	polled  time.Time
}

func (c *channel) abandoned() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.polling == 0 && time.Since(c.polled) > ownerGrace
}

func (c *channel) poll(change int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.polling += change
	c.polled = time.Now()
}

type relay struct {
	mu       sync.Mutex
	channels map[string]*channel
	perHost  map[string]int
}

// Handler serves the relay under /relay/.
func Handler() http.Handler {
	r := &relay{channels: map[string]*channel{}, perHost: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /relay/{channel}", r.register)
	mux.HandleFunc("DELETE /relay/{channel}", r.owned(r.remove))
	mux.HandleFunc("GET /relay/{channel}/next", r.owned(next))
	mux.HandleFunc("POST /relay/{channel}/reply/{id}", r.owned(reply))
	mux.HandleFunc("POST /relay/{channel}", r.ask)
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, request)
	})
}

func (r *relay) find(name string) *channel {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.channels[name]
}

func (r *relay) drop(name string, found *channel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.channels[name] == found {
		delete(r.channels, name)
		if r.perHost[found.host]--; r.perHost[found.host] <= 0 {
			delete(r.perHost, found.host)
		}
		found.expiry.Stop()
		close(found.closed)
	}
}

func bearer(request *http.Request) string {
	token, _ := strings.CutPrefix(request.Header.Get("Authorization"), "Bearer ")
	return token
}

// caller names who is asking, to cap how many channels one machine holds. The
// header is Cloudflare's; a site not behind it sees every caller as its proxy.
func caller(request *http.Request) string {
	if address := request.Header.Get("CF-Connecting-IP"); address != "" {
		return address
	}
	host, _, _ := net.SplitHostPort(request.RemoteAddr)
	return host
}

// register opens a channel for the process that will answer on it. Whoever
// registers first owns the name until the channel closes.
func (r *relay) register(w http.ResponseWriter, request *http.Request) {
	name, owner := request.PathValue("channel"), bearer(request)
	if !channelName.MatchString(name) || owner == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	created := &channel{
		owner:   owner,
		host:    caller(request),
		queue:   make(chan *message, maxQueued),
		closed:  make(chan struct{}),
		waiting: map[string]*message{},
		polled:  time.Now(),
	}
	r.mu.Lock()
	_, taken := r.channels[name]
	full := len(r.channels) >= maxChannels || r.perHost[created.host] >= maxPerHost
	if !taken && !full {
		r.channels[name] = created
		r.perHost[created.host]++
		created.expiry = time.AfterFunc(lifetime, func() { r.drop(name, created) })
	}
	r.mu.Unlock()
	switch {
	case taken:
		http.Error(w, "conflict", http.StatusConflict)
	case full:
		http.Error(w, "too many sign-in pages are open", http.StatusTooManyRequests)
	default:
		w.WriteHeader(http.StatusCreated)
	}
}

func (r *relay) owned(handle func(http.ResponseWriter, *http.Request, *channel)) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		found := r.find(request.PathValue("channel"))
		switch {
		case found == nil:
			http.Error(w, "not found", http.StatusNotFound)
		case subtle.ConstantTimeCompare([]byte(bearer(request)), []byte(found.owner)) != 1:
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		default:
			handle(w, request, found)
		}
	}
}

func (r *relay) remove(w http.ResponseWriter, request *http.Request, found *channel) {
	r.drop(request.PathValue("channel"), found)
	w.WriteHeader(http.StatusNoContent)
}

func next(w http.ResponseWriter, request *http.Request, found *channel) {
	found.poll(1)
	defer found.poll(-1)
	select {
	case pending := <-found.queue:
		w.Header().Set("Relay-Request", pending.id)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(pending.body)
	case <-time.After(pollWait):
		w.WriteHeader(http.StatusNoContent)
	case <-found.closed:
		http.Error(w, "not found", http.StatusNotFound)
	case <-request.Context().Done():
	}
}

func reply(w http.ResponseWriter, request *http.Request, found *channel) {
	body, err := io.ReadAll(http.MaxBytesReader(w, request.Body, maxReply))
	if err != nil {
		http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		return
	}
	found.mu.Lock()
	pending := found.waiting[request.PathValue("id")]
	found.mu.Unlock()
	if pending == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	select {
	case pending.reply <- body:
	default:
	}
	w.WriteHeader(http.StatusNoContent)
}

// ask queues a message from the page and holds the request open for its reply.
func (r *relay) ask(w http.ResponseWriter, request *http.Request) {
	found := r.find(request.PathValue("channel"))
	if found != nil && found.abandoned() {
		r.drop(request.PathValue("channel"), found)
		found = nil
	}
	if found == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, request.Body, maxRequest))
	if err != nil {
		http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		return
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	pending := &message{id: hex.EncodeToString(id), body: body, reply: make(chan []byte, 1)}
	found.mu.Lock()
	found.waiting[pending.id] = pending
	found.mu.Unlock()
	defer func() {
		found.mu.Lock()
		delete(found.waiting, pending.id)
		found.mu.Unlock()
	}()
	select {
	case found.queue <- pending:
	default:
		http.Error(w, "busy", http.StatusServiceUnavailable)
		return
	}
	select {
	case answer := <-pending.reply:
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(answer)
	case <-time.After(replyWait):
		http.Error(w, "timeout", http.StatusGatewayTimeout)
	case <-found.closed:
		http.Error(w, "not found", http.StatusNotFound)
	case <-request.Context().Done():
	}
}
