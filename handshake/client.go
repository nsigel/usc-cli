// Package handshake reads events and career fairs from USC Handshake's
// student site.
package handshake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/site"
	http "github.com/saucesteals/fhttp"
)

const (
	baseURL       = "https://usc.joinhandshake.com"
	searchGraphQL = "/stu/graphql"
	detailGraphQL = "/hs/graphql"
)

var errSessionInvalid = &auth.SiteError{Site: site.Handshake, Err: auth.ErrSessionInvalid}

// Doer is the HTTP surface the client needs. auth.Session satisfies it.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client calls Handshake with an authenticated session.
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
	session, err := auth.Open(ctx, site.Handshake, options)
	if err != nil {
		return nil, err
	}
	return New(session), nil
}

// Error is a Handshake request that failed with an HTTP status.
type Error struct {
	Label  string
	Status int
	Body   string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: HTTP %d: %s", e.Label, e.Status, e.Body)
}

// Handshake answers an expired session with a redirect to sign-in or a 401.
func (c *Client) send(request *http.Request, label string) ([]byte, error) {
	if c.http == nil {
		return nil, errors.New("handshake client has no HTTP session")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode < 400 || response.StatusCode == http.StatusUnauthorized {
		return nil, errSessionInvalid
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if strings.HasPrefix(message, "<") {
			message = http.StatusText(response.StatusCode)
		}
		return nil, &Error{Label: label, Status: response.StatusCode, Body: message[:min(len(message), 200)]}
	}
	return body, nil
}

func (c *Client) graphQL(ctx context.Context, path, operation, query string, variables map[string]any, destination any) error {
	payload, err := json.Marshal(map[string]any{"operationName": operation, "query": query, "variables": variables})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", baseURL)
	request.Header.Set("Referer", baseURL+"/stu/events")
	body, err := c.send(request, operation)
	if err != nil {
		return err
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		// An expired session can also answer with the sign-in page itself.
		if bytes.HasPrefix(bytes.TrimSpace(body), []byte("<")) {
			return errSessionInvalid
		}
		return fmt.Errorf("%s: decode response: %w", operation, err)
	}
	if len(envelope.Errors) > 0 {
		messages := make([]string, len(envelope.Errors))
		for index, graphErr := range envelope.Errors {
			messages[index] = graphErr.Message
		}
		return fmt.Errorf("%s: %s", operation, strings.Join(messages, "; "))
	}
	if err := json.Unmarshal(envelope.Data, destination); err != nil {
		return fmt.Errorf("%s: decode data: %w", operation, err)
	}
	return nil
}

func (c *Client) getHTML(ctx context.Context, path, label string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	return c.send(request, label)
}
