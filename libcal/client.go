// Package libcal provides a small client for USC Libraries' LibCal room
// availability and reservation pages. NewPublic is suitable for public
// availability reads; Open reuses the USC CLI's saved Shibboleth session.
package libcal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/nsigel/usc-cli/internal/auth"
	"github.com/nsigel/usc-cli/internal/config"
	http "github.com/saucesteals/fhttp"
)

const (
	baseURL          = "https://libcal.usc.edu"
	leaveyLocationID = 2895
)

// ErrAuthenticationRequired means LibCal needs the user to sign in through
// USC SSO. Run `usc auth login libcal` and retry.
var ErrAuthenticationRequired = &Error{Code: "libcal_authentication_required", Message: "LibCal authentication required; run usc auth login libcal"}

// Doer is the HTTP surface used by Client. It accepts the same request type as
// usc-cli's authenticated session so callers can supply their own transport.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client reads and books Leavey Library spaces through LibCal.
type Client struct {
	http             Doer
	authenticate     func(context.Context, string, string) (*http.Response, error)
	checkoutPath     string
	reservationsPath string
}

// New creates a LibCal client using the supplied HTTP client.
func New(httpClient Doer) *Client {
	return &Client{http: httpClient}
}

// NewPublic creates a client for unauthenticated availability and room reads.
func NewPublic() *Client {
	return New(&http.Client{Timeout: 30 * time.Second})
}

// Open returns a LibCal client using the USC session saved by usc auth login.
// The SSO handoff is deferred until LibCal generates its booking-specific auth
// redirect. It never prompts; authentication commands own credential collection.
func Open(ctx context.Context) (*Client, error) {
	path, err := config.SessionPath()
	if err != nil {
		return nil, fmt.Errorf("find USC session: %w", err)
	}
	reservationsPath, err := config.LibCalReservationsPath()
	if err != nil {
		return nil, fmt.Errorf("find local LibCal reservations: %w", err)
	}
	session, err := auth.OpenSession(path, auth.SessionOptions{
		AllowMissing:    true,
		ResetDomains:    []string{"libcal.usc.edu", "libauth.com"},
		PreserveCookies: []string{"lc_ebcart"},
	})
	if err != nil {
		return nil, fmt.Errorf("open USC session: %w", err)
	}
	client := New(session)
	client.checkoutPath = filepath.Join(filepath.Dir(path), "libcal-checkout.json")
	client.reservationsPath = reservationsPath
	client.authenticate = func(ctx context.Context, target, referer string) (*http.Response, error) {
		response, err := session.Authenticate(ctx, target, auth.Credentials{}, referer)
		if errors.Is(err, auth.ErrCredentialsRequired) {
			return response, ErrAuthenticationRequired
		}
		return response, err
	}
	return client, nil
}

func (c *Client) request(ctx context.Context, method, path string, form url.Values, accept string) ([]byte, error) {
	return c.requestFrom(ctx, method, path, form, accept, "")
}

func (c *Client) requestFrom(ctx context.Context, method, path string, form url.Values, accept, referer string) ([]byte, error) {
	if c == nil || c.http == nil {
		return nil, errors.New("LibCal client has no HTTP transport")
	}
	requestURL := baseURL + path
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", accept)
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	request.Header.Set("User-Agent", "usc-cli")
	if form != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
		request.Header.Set("X-Requested-With", "XMLHttpRequest")
		if referer == "" {
			referer = baseURL + "/reserve/lvl2"
		}
		request.Header.Set("Referer", referer)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		location, err := url.Parse(response.Header.Get("Location"))
		if err == nil && (strings.Contains(location.Path, "/spaces/auth") || strings.Contains(location.Host, "login.usc.edu")) {
			return nil, ErrAuthenticationRequired
		}
		return nil, responseError(method+" "+path, response.StatusCode, nil)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, responseError(method+" "+path, response.StatusCode, data)
	}
	return data, nil
}
