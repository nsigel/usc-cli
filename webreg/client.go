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
	http "github.com/saucesteals/fhttp"
)

const (
	baseURL = "https://webreg.usc.edu"
	host    = "webreg.usc.edu"

	documentAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	maxRedirects   = 5
	maxBody        = 16 << 20
)

var errSessionInvalid = &auth.SiteError{Site: auth.SiteWebReg, Err: auth.ErrSessionInvalid}

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type Client struct {
	http Doer
}

func New(httpClient Doer) *Client {
	return &Client{http: httpClient}
}

func Open(ctx context.Context, options auth.Options) (*Client, error) {
	session, err := auth.Open(ctx, auth.SiteWebReg, options)
	if err != nil {
		return nil, err
	}
	return New(session), nil
}

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
			return nil, &Error{Operation: "GET " + path, Status: response.StatusCode, Message: safeServerMessage(data)}
		}
		return data, nil
	}
	return nil, fmt.Errorf("WebReg GET %s: too many redirects", path)
}

func (c *Client) postForm(ctx context.Context, path string, form url.Values, referer string) ([]byte, error) {
	return c.post(ctx, path, form, referer, map[string]string{
		"Accept":       documentAccept,
		"Content-Type": "application/x-www-form-urlencoded",
	})
}

func (c *Client) postAJAX(ctx context.Context, path string, form url.Values, referer string) ([]byte, error) {
	return c.post(ctx, path, form, referer, map[string]string{
		"Accept":           "*/*",
		"Content-Type":     "application/x-www-form-urlencoded; charset=UTF-8",
		"X-Requested-With": "XMLHttpRequest",
		"X-Csrf":           "1",
	})
}

func (c *Client) post(ctx context.Context, path string, form url.Values, referer string, headers map[string]string) ([]byte, error) {
	request, err := c.newRequest(ctx, http.MethodPost, baseURL+path, strings.NewReader(form.Encode()), referer)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Origin", baseURL)
	for name, value := range headers {
		request.Header.Set(name, value)
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
		return nil, &Error{Operation: "POST " + path, Status: response.StatusCode, Message: safeServerMessage(data)}
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
	if landedOnSignIn(response) {
		return nil, nil, errSessionInvalid
	}
	return response, data, nil
}

func landedOnSignIn(response *http.Response) bool {
	return signInURL(response.Request.URL)
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

func safeServerMessage(data []byte) string {
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
