package directory

import (
	"context"
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
	baseURL    = "https://uscdirectory.usc.edu"
	studentDir = "/web/directory/student/"
	maxBody    = 4 << 20
)

var errSessionInvalid = &auth.SiteError{Site: site.Directory, Err: auth.ErrSessionInvalid}

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
	session, err := auth.Open(ctx, site.Directory, options)
	if err != nil {
		return nil, err
	}
	return New(session), nil
}

type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("directory: HTTP %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("directory: HTTP %d", e.Status)
}

func (c *Client) proxy(ctx context.Context, query url.Values) ([]byte, error) {
	if c == nil || c.http == nil {
		return nil, errors.New("directory client has no HTTP session")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+studentDir+"proxy.php?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/plain, */*; q=0.01")
	request.Header.Set("X-Requested-With", "XMLHttpRequest")
	request.Header.Set("Referer", baseURL+studentDir)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode < 400 || response.StatusCode == http.StatusUnauthorized {
		return nil, errSessionInvalid
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBody))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &Error{Status: response.StatusCode, Message: shortMessage(data)}
	}
	return data, nil
}

func shortMessage(data []byte) string {
	message := strings.TrimSpace(string(data))
	if len(message) > 300 || strings.ContainsAny(message, "<\n") {
		return ""
	}
	return message
}
