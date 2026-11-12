// Package webreg is a client for USC Web Registration (webreg.usc.edu).
//
// WebReg is a server-rendered application: catalog, course bin, and checkout
// data are parsed from its pages, while course-bin changes use its small
// section API. The server keeps an active term per session, so every
// term-scoped method selects its term before reading or changing state.
package webreg

import (
	"context"
	"errors"
	"fmt"
	"github.com/nsigel/usc-cli/internal/dom"
	"io"
	"net/url"
	"strings"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/site"
	http "github.com/saucesteals/fhttp"
)

const (
	baseURL = "https://webreg.usc.edu"
	host    = "webreg.usc.edu"

	documentAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	maxRedirects   = 5
	maxBody        = 16 << 20
)

// errSessionInvalid reports that WebReg redirected to sign-in.
var errSessionInvalid = &auth.SiteError{Site: site.WebReg, Err: auth.ErrSessionInvalid}

// Doer is the HTTP surface the client needs. auth.Session satisfies it.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client reads and changes WebReg state with an authenticated session. A
// client is sequential: WebReg's active term is shared session state.
type Client struct {
	http Doer
}

// New creates a WebReg client from an authenticated HTTP client.
func New(httpClient Doer) *Client {
	return &Client{http: httpClient}
}

// Open signs in through USC SSO and returns an authenticated client. See
// auth.Options for supplying credentials and keeping the session between
// runs. It never prompts; authentication failures are *auth.SiteError values.
func Open(ctx context.Context, options auth.Options) (*Client, error) {
	session, err := auth.Open(ctx, site.WebReg, options)
	if err != nil {
		return nil, err
	}
	return New(session), nil
}

// Error describes a WebReg request the server rejected. Message is a short
// server explanation when one was safe to include.
type Error struct {
	Operation string
	Status    int
	Message   string
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("WebReg %s: HTTP %d: %s", e.Operation, e.Status, e.Message)
	}
	return fmt.Sprintf("WebReg %s: HTTP %d", e.Operation, e.Status)
}

// SelectTerm makes term WebReg's active term for this session.
func (c *Client) SelectTerm(ctx context.Context, term string) error {
	if err := validTerm(term); err != nil {
		return err
	}
	_, err := c.page(ctx, "/Terms?term="+url.QueryEscape(term)+"&handler=TermSelect", "/Terms")
	return err
}

func validTerm(term string) error {
	if len(term) != 5 || strings.Trim(term, "0123456789") != "" {
		return fmt.Errorf("term must be a five-digit WebReg term code such as 20271, got %q", term)
	}
	return nil
}

// page GETs a WebReg page, following same-site redirects such as term
// selection. Any redirect toward sign-in reports errSessionInvalid.
func (c *Client) page(ctx context.Context, path, referer string) ([]byte, error) {
	target := baseURL + path
	for range maxRedirects + 1 {
		request, err := c.newRequest(ctx, http.MethodGet, target, nil, referer)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Accept", documentAccept)
		response, data, err := c.do(request)
		if err != nil {
			return nil, err
		}
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			next, err := response.Request.URL.Parse(response.Header.Get("Location"))
			if err != nil || response.Header.Get("Location") == "" {
				return nil, &Error{Operation: "GET " + path, Status: response.StatusCode, Message: "redirect without a location"}
			}
			if signInURL(next) {
				return nil, errSessionInvalid
			}
			referer, target = response.Request.URL.Path, next.String()
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, &Error{Operation: "GET " + path, Status: response.StatusCode, Message: shortText(data)}
		}
		return data, nil
	}
	return nil, fmt.Errorf("WebReg GET %s: too many redirects", path)
}

// post sends a same-origin form POST the way WebReg's pages do. ajax adds the
// XMLHttpRequest markers its section API requires.
func (c *Client) post(ctx context.Context, path string, form url.Values, referer string, ajax bool) ([]byte, error) {
	request, err := c.newRequest(ctx, http.MethodPost, baseURL+path, strings.NewReader(form.Encode()), referer)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Origin", baseURL)
	if ajax {
		request.Header.Set("Accept", "*/*")
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
		request.Header.Set("X-Requested-With", "XMLHttpRequest")
		request.Header.Set("X-Csrf", "1")
	} else {
		request.Header.Set("Accept", documentAccept)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	response, data, err := c.do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		if next, err := response.Request.URL.Parse(response.Header.Get("Location")); err == nil && signInURL(next) {
			return nil, errSessionInvalid
		}
	}
	if response.StatusCode == http.StatusUnauthorized {
		return nil, errSessionInvalid
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &Error{Operation: "POST " + path, Status: response.StatusCode, Message: shortText(data)}
	}
	return data, nil
}

func (c *Client) newRequest(ctx context.Context, method, target string, body io.Reader, referer string) (*http.Request, error) {
	if c == nil || c.http == nil {
		return nil, errors.New("WebReg client has no HTTP transport")
	}
	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if referer != "" {
		request.Header.Set("Referer", baseURL+referer)
	}
	return request, nil
}

func (c *Client) do(request *http.Request) (*http.Response, []byte, error) {
	response, err := c.http.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBody))
	if err != nil {
		return nil, nil, err
	}
	if response.Request == nil {
		response.Request = request
	}
	// Transports that follow redirects themselves land on the sign-in page
	// with a 200, so the final URL matters as much as the status.
	if signInURL(response.Request.URL) {
		return nil, nil, errSessionInvalid
	}
	return response, data, nil
}

func signInURL(location *url.URL) bool {
	if location == nil {
		return false
	}
	if location.Host != "" && location.Host != host {
		return true
	}
	path := strings.ToLower(location.Path)
	return strings.HasPrefix(path, "/auth/login") || path == "/signin-oidc"
}

// shortText returns a brief plain-text server message, or nothing when the
// body is a full page or may echo session state.
func shortText(data []byte) string {
	message := text(dom.Parse(data))
	if len(data) > 2048 || len(message) > 300 {
		return ""
	}
	lower := strings.ToLower(message)
	if strings.Contains(lower, "token") || strings.Contains(lower, "cookie") {
		return ""
	}
	return message
}
