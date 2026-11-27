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

var errSessionInvalid = &auth.SiteError{Site: site.Brightspace, Err: auth.ErrSessionInvalid}

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
	session, err := auth.Open(ctx, site.Brightspace, options)
	if err != nil {
		return nil, err
	}
	return New(session), nil
}

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

	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden && label == "whoami" {
		return nil, errSessionInvalid
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 200))
	return nil, &Error{Label: label, Status: response.StatusCode, Body: strings.TrimSpace(string(body))}
}

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

	if strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/html") {
		response.Body.Close()
		return nil, errSessionInvalid
	}
	return response.Body, nil
}

type richText struct {
	Text string `json:"Text"`
	HTML string `json:"Html"`
}

func stringPointer(value string) *string { return &value }
