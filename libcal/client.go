// Package libcal reads Leavey Library room availability from USC Libraries'
// LibCal and books rooms. NewPublic is enough for availability; booking needs
// a client from Open.
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

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/site"
	http "github.com/saucesteals/fhttp"
)

const (
	baseURL          = "https://libcal.usc.edu"
	leaveyLocationID = 2895
	ajaxAccept       = "application/json, text/javascript, */*;q=0.01"
)

// ErrAuthenticationRequired means LibCal's checkout needs a USC sign-in and
// no complete credentials were available.
var ErrAuthenticationRequired = &Error{
	Code: "libcal_authentication_required", Message: "LibCal needs a USC sign-in",
	Cause: &auth.SiteError{Site: site.LibCal, Err: auth.ErrCredentialsRequired},
}

// Doer is the HTTP surface the client needs. auth.Session satisfies it.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client reads and books Leavey Library spaces through LibCal.
type Client struct {
	http         Doer
	authenticate func(context.Context, string, string) (*http.Response, error)
	checkoutPath string
}

// New creates a LibCal client using the supplied HTTP client.
func New(httpClient Doer) *Client {
	return &Client{http: httpClient}
}

// NewPublic creates a client for unauthenticated availability and room reads.
func NewPublic() *Client {
	return New(&http.Client{Timeout: 30 * time.Second})
}

// Open returns a booking client. LibCal only issues its SSO redirect once a
// booking is staged, so sign-in happens during Book, with the credentials in
// options. With a SessionFile, checkout recovery state is kept beside it and
// bookings from other processes using that file are serialized.
func Open(options auth.Options) (*Client, error) {
	session, err := auth.OpenSession(options, auth.CookiePolicy{
		ResetDomains:    []string{"libcal.usc.edu", "libauth.com"},
		PreserveCookies: []string{"lc_ebcart"},
	})
	if err != nil {
		return nil, fmt.Errorf("open USC session: %w", err)
	}
	client := New(session)
	if options.SessionFile != "" {
		client.checkoutPath = filepath.Join(filepath.Dir(options.SessionFile), "libcal-checkout.json")
	}
	client.authenticate = func(ctx context.Context, target, referer string) (*http.Response, error) {
		response, err := session.Authenticate(ctx, target, referer)
		if errors.Is(err, auth.ErrCredentialsRequired) {
			return response, ErrAuthenticationRequired
		}
		return response, auth.ForSite(site.LibCal, err)
	}
	return client, nil
}

// request sends one LibCal request and returns a successful response body.
// form, when set, is posted as LibCal's own pages post it, with referer
// defaulting to a reservation page.
func (c *Client) request(ctx context.Context, method, path string, form url.Values, accept, referer string) ([]byte, error) {
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
