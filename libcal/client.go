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

var ErrAuthenticationRequired = &Error{
	Code: "libcal_authentication_required", Message: "LibCal needs a USC sign-in",
	Cause: &auth.SiteError{Site: site.LibCal, Err: auth.ErrCredentialsRequired},
}

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type Client struct {
	http         Doer
	authenticate func(context.Context, string, string) (*http.Response, error)
	checkoutPath string
}

func New(httpClient Doer) *Client {
	return &Client{http: httpClient}
}

func NewPublic() *Client {
	return New(&http.Client{Timeout: 30 * time.Second})
}

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
			referer = defaultReferer
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
		if err == nil && strings.Contains(location.Path, "/spaces/auth") {
			return nil, ErrAuthenticationRequired
		}
		return nil, responseError(method+" "+path, response.StatusCode, nil)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, responseError(method+" "+path, response.StatusCode, data)
	}
	return data, nil
}
