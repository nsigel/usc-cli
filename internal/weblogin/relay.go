package weblogin

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// Messages are padded to a multiple of this, so their length does not
	// give away the length of a password.
	paddedBlock = 512
	maxMessage  = 1 << 16
	maxClients  = 256
)

var (
	requestLabel = []byte("request")
	relayClient  = &http.Client{Timeout: 40 * time.Second}
)

// relay reaches the sign-in page on usc.sh, which cannot connect to this
// machine. Both ends seal every message with a key that travels only in the
// link's fragment, so the site forwarding the messages cannot read them.
type relay struct {
	base    string
	channel string
	owner   string
	aead    cipher.AEAD

	// Each request carries its sender's name and a rising number, so a
	// request the relay sends twice is answered once.
	client string
	sent   uint64
	seen   map[string]uint64
}

func newRelay(base string, key []byte) (*relay, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	owner := make([]byte, 24)
	if _, err := rand.Read(owner); err != nil {
		return nil, err
	}
	name := sha256.Sum256(append([]byte("usc-cli relay channel"), key...))
	return &relay{
		base:    strings.TrimRight(base, "/"),
		channel: hex.EncodeToString(name[:16]),
		owner:   hex.EncodeToString(owner[:16]),
		aead:    aead,
		client:  hex.EncodeToString(owner[16:]),
		seen:    map[string]uint64{},
	}, nil
}

// seal binds a request to requestLabel and an answer to its request's nonce,
// so the relay cannot pass off one message as another.
func (r *relay) seal(plain, bound []byte) []byte {
	plain = append(plain, bytes.Repeat([]byte{' '}, (paddedBlock-len(plain)%paddedBlock)%paddedBlock)...)
	nonce := make([]byte, r.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}
	return r.aead.Seal(nonce, nonce, plain, bound)
}

func (r *relay) open(sealed, bound []byte) ([]byte, error) {
	if len(sealed) < r.aead.NonceSize() {
		return nil, errors.New("message too short")
	}
	return r.aead.Open(nil, sealed[:r.aead.NonceSize()], sealed[r.aead.NonceSize():], bound)
}

// ask sends one request as the page would. A status other than 200 comes
// back without a state.
func (r *relay) ask(ctx context.Context, path string) (state, int, error) {
	r.sent++
	plain, err := json.Marshal(map[string]any{"client": r.client, "seq": r.sent, "path": path})
	if err != nil {
		return state{}, 0, err
	}
	sealed := r.seal(plain, requestLabel)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.base+"/relay/"+r.channel, bytes.NewReader(sealed))
	if err != nil {
		return state{}, 0, err
	}
	request.Header.Set("User-Agent", "usc-cli")
	response, err := relayClient.Do(request)
	if err != nil {
		return state{}, 0, err
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(response.Body, maxMessage))
	if err != nil || response.StatusCode != http.StatusOK {
		return state{}, response.StatusCode, err
	}
	var current state
	opened, err := r.open(answer, sealed[:r.aead.NonceSize()])
	if err == nil {
		err = json.Unmarshal(opened, &current)
	}
	return current, response.StatusCode, err
}

// send makes one call that only the process serving the sign-in may make.
func (r *relay) send(ctx context.Context, method, path string, body []byte) (http.Header, []byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, method, r.base+"/relay/"+r.channel+path, bytes.NewReader(body))
	if err != nil {
		return nil, nil, 0, err
	}
	request.Header.Set("Authorization", "Bearer "+r.owner)
	request.Header.Set("User-Agent", "usc-cli")
	response, err := relayClient.Do(request)
	if err != nil {
		return nil, nil, 0, err
	}
	defer response.Body.Close()
	received, err := io.ReadAll(io.LimitReader(response.Body, maxMessage))
	return response.Header, received, response.StatusCode, err
}

func (r *relay) register(ctx context.Context) error {
	_, _, status, err := r.send(ctx, http.MethodPut, "", nil)
	var failed *url.Error
	if errors.As(err, &failed) {
		return fmt.Errorf("could not reach the sign-in relay at %s: %w", r.base, failed.Err)
	}
	if err == nil && status != http.StatusCreated {
		err = errors.New("the sign-in relay at " + r.base + " refused a new page: " + http.StatusText(status))
	}
	return err
}

// serve answers the page's requests one at a time until done closes or ctx ends.
func (r *relay) serve(ctx context.Context, done <-chan struct{}, api func(path string, body []byte) (state, bool)) {
	defer func() {
		closing, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _, _, _ = r.send(closing, http.MethodDelete, "", nil)
	}()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		default:
		}
		header, sealed, status, err := r.send(ctx, http.MethodGet, "/next", nil)
		switch {
		case err == nil && status == http.StatusOK:
			_, _, _, _ = r.send(ctx, http.MethodPost, "/reply/"+header.Get("Relay-Request"), r.answer(sealed, api))
			continue
		case err == nil && status == http.StatusNoContent:
			continue
		case err == nil && status == http.StatusNotFound:
			// The relay restarted and forgot this page.
			_ = r.register(ctx)
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

// answer returns nothing for a request it cannot read, which the page cannot
// open and so treats as a failure.
func (r *relay) answer(sealed []byte, api func(path string, body []byte) (state, bool)) []byte {
	plain, err := r.open(sealed, requestLabel)
	if err != nil {
		return nil
	}
	var request struct {
		Client string          `json:"client"`
		Seq    uint64          `json:"seq"`
		Path   string          `json:"path"`
		Body   json.RawMessage `json:"body"`
	}
	if json.Unmarshal(plain, &request) != nil {
		return nil
	}
	last, known := r.seen[request.Client]
	if request.Seq <= last || (!known && len(r.seen) >= maxClients) {
		return nil
	}
	r.seen[request.Client] = request.Seq
	current, ok := api(request.Path, request.Body)
	if !ok {
		return nil
	}
	encoded, err := json.Marshal(current)
	if err != nil {
		return nil
	}
	return r.seal(encoded, sealed[:r.aead.NonceSize()])
}
