package weblogin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	sitePage "github.com/nsigel/usc-cli/internal/weblogin/page"
)

var (
	errNotPage = errors.New("not a sign-in page URL")
	// ErrPageClosed reports that the sign-in page stopped serving before
	// reporting completion (for example, it expired or its process exited).
	ErrPageClosed = errors.New("the sign-in page closed or expired before sign-in finished")
	localClient   = &http.Client{Timeout: 10 * time.Second}
)

// Wait blocks until the sign-in page at pageURL finishes or stops serving.
// It returns nil on completion, ErrPageClosed if the page stops answering
// before reporting a final state, or an error describing why sign-in failed.
func Wait(ctx context.Context, pageURL string) error {
	page, err := url.Parse(pageURL)
	if err != nil || page.Fragment == "" {
		return errNotPage
	}
	var read func() (current state, serving bool, err error)
	switch {
	case page.Path == sitePage.Path:
		key, err := base64.RawURLEncoding.DecodeString(page.Fragment)
		if err != nil || len(key) != 32 {
			return errNotPage
		}
		relay, err := newRelay(page.Scheme+"://"+page.Host, key)
		if err != nil {
			return err
		}
		failures := 0
		read = func() (state, bool, error) {
			current, status, err := relay.ask(ctx, "/api/wait")
			if status == http.StatusOK {
				failures = 0
				if err != nil {
					return state{}, false, errNotPage
				}
				return current, true, nil
			}
			if ctx.Err() != nil {
				return state{}, false, ctx.Err()
			}
			failures++
			switch {
			// A relay that restarts forgets the page briefly while the owner re-registers.
			case status == http.StatusNotFound && failures >= 5,
				// Two consecutive relay timeouts (~20s) mean the serving process stopped answering.
				status == http.StatusGatewayTimeout && failures >= 2:
				return state{}, false, nil
			case failures >= 15:
				return state{}, false, errors.New("could not reach the sign-in relay at " + page.Host)
			}
			return state{}, true, nil
		}
	case page.Scheme == "http" && page.Hostname() == "127.0.0.1":
		read = func() (state, bool, error) {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+page.Host+"/api/wait", nil)
			if err != nil {
				return state{}, false, err
			}
			request.Header.Set("Authorization", "Bearer "+page.Fragment)
			response, err := localClient.Do(request)
			if err != nil {
				if ctx.Err() != nil {
					return state{}, false, ctx.Err()
				}
				return state{}, false, nil
			}
			defer response.Body.Close()
			var current state
			if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&current) != nil {
				return state{}, false, errNotPage
			}
			return current, true, nil
		}
	default:
		return errNotPage
	}
	for {
		current, serving, err := read()
		switch {
		case err != nil:
			return err
		case !serving:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return ErrPageClosed
		case current.Step == stepDone:
			return nil
		case current.Step == stepFailed:
			return errors.New(current.Error)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
