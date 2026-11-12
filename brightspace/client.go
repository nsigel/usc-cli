// Package brightspace reads USC Brightspace through its Valence API.
package brightspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/site"
	http "github.com/saucesteals/fhttp"
)

const (
	baseURL   = "https://brightspace.usc.edu"
	lpVersion = "1.31"
	leVersion = "1.67"
)

// errSessionInvalid reports that Brightspace stopped accepting the session.
var errSessionInvalid = &auth.SiteError{Site: site.Brightspace, Err: auth.ErrSessionInvalid}

// Doer is the HTTP surface the client needs. auth.Session satisfies it.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client calls Brightspace with an authenticated session.
type Client struct {
	http Doer
}

// New creates a client from an authenticated HTTP client.
func New(httpClient Doer) *Client {
	return &Client{http: httpClient}
}

// Open signs in through USC SSO and returns an authenticated client. See
// auth.Options for supplying credentials and keeping the session between
// runs. It never prompts; authentication failures are *auth.SiteError values.
func Open(ctx context.Context, options auth.Options) (*Client, error) {
	session, err := auth.Open(ctx, site.Brightspace, options)
	if err != nil {
		return nil, err
	}
	return New(session), nil
}

// Error is a Brightspace request that failed with an HTTP status.
type Error struct {
	Label  string
	Status int
	Body   string
}

func (e *Error) Error() string {
	if e.Status == http.StatusForbidden {
		return fmt.Sprintf("%s: access denied (403)", e.Label)
	}
	return fmt.Sprintf("%s: HTTP %d: %s", e.Label, e.Status, e.Body)
}

// get decodes a JSON API response into destination. label names the request
// in errors.
func (c *Client) get(ctx context.Context, path string, query url.Values, label string, destination any) error {
	target := baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	response, err := c.do(ctx, target, "application/json", label)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		return fmt.Errorf("%s: decode response: %w", label, err)
	}
	return nil
}

// do sends a GET and returns a successful response; the caller closes it.
func (c *Client) do(ctx context.Context, target, accept, label string) (*http.Response, error) {
	if c.http == nil {
		return nil, errors.New("brightspace client has no HTTP session")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", accept)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return response, nil
	}
	defer response.Body.Close()
	// USC answers an unauthenticated whoami with 403. That endpoint checks
	// identity, not course permissions; other 403s are real access denials.
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden && label == "whoami" {
		return nil, errSessionInvalid
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 200))
	return nil, &Error{Label: label, Status: response.StatusCode, Body: strings.TrimSpace(string(body))}
}

// Download returns the authenticated body of a Brightspace content URL. Only
// same-origin URLs are accepted so USC cookies never reach another host.
func (c *Client) Download(ctx context.Context, contentURL string) (io.ReadCloser, error) {
	base, _ := url.Parse(baseURL)
	target, err := base.Parse(contentURL)
	if err != nil {
		return nil, fmt.Errorf("invalid content URL: %w", err)
	}
	if target.Scheme != base.Scheme || target.Host != base.Host {
		return nil, errors.New("content URL is not hosted by Brightspace")
	}
	response, err := c.do(ctx, target.String(), "application/pdf,application/octet-stream;q=0.9,*/*;q=0.1", "download")
	if err != nil {
		return nil, err
	}
	// An expired session serves the sign-in page instead of the file.
	if strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/html") {
		response.Body.Close()
		return nil, errSessionInvalid
	}
	return response.Body, nil
}

// richText is Brightspace's paired plain-text and HTML rendering of a field.
type richText struct {
	Text string `json:"Text"`
	HTML string `json:"Html"`
}

func stringPointer(value string) *string { return &value }
