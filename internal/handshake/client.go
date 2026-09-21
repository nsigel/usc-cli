// Package handshake implements USC Handshake's student event contract.
package handshake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"

	http "github.com/saucesteals/fhttp"
)

const baseURL = "https://usc.joinhandshake.com"

var (
	// ErrSessionInvalid means Handshake did not accept the saved application session.
	ErrSessionInvalid = errors.New("Handshake session expired or invalid")
	htmlBreak         = regexp.MustCompile(`(?i)</?(?:br|p|div|li|h[1-6])\b[^>]*>`)
	htmlTag           = regexp.MustCompile(`<[^>]+>`)
	space             = regexp.MustCompile(`[\t\r ]+`)
	blankLines        = regexp.MustCompile(`\n{3,}`)
)

// Doer is the authenticated HTTP surface needed by the Handshake client.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client calls Handshake with an authenticated browser session.
type Client struct {
	http Doer
}

// New creates a Handshake client.
func New(httpClient Doer) *Client {
	return &Client{http: httpClient}
}

// Error describes a failed Handshake request.
type Error struct {
	Label  string
	Status int
	Body   string
}

func (e *Error) Error() string {
	if e.Status == http.StatusUnauthorized {
		return ErrSessionInvalid.Error()
	}
	return fmt.Sprintf("%s: HTTP %d — %s", e.Label, e.Status, e.Body)
}

func (e *Error) Unwrap() error {
	if e.Status == http.StatusUnauthorized {
		return ErrSessionInvalid
	}
	return nil
}

type graphQLRequest struct {
	OperationName string         `json:"operationName"`
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables"`
}

type graphQLError struct {
	Message string `json:"message"`
}

type graphQLResponse[T any] struct {
	Data   T              `json:"data"`
	Errors []graphQLError `json:"errors"`
}

func (c *Client) graphQL(ctx context.Context, operation, query string, variables map[string]any, destination any) error {
	return c.graphQLAt(ctx, "/stu/graphql", operation, query, variables, destination)
}

func (c *Client) graphQLAt(ctx context.Context, path, operation, query string, variables map[string]any, destination any) error {
	if c.http == nil {
		return errors.New("handshake client has no HTTP session")
	}
	payload, err := json.Marshal(graphQLRequest{OperationName: operation, Query: query, Variables: variables})
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
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 || response.StatusCode == http.StatusUnauthorized {
		return ErrSessionInvalid
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &Error{Label: operation, Status: response.StatusCode, Body: truncate(strings.TrimSpace(string(body)), 200)}
	}
	envelope := graphQLResponse[json.RawMessage]{}
	if err := json.Unmarshal(body, &envelope); err != nil {
		if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/html") {
			return ErrSessionInvalid
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
	if c.http == nil {
		return nil, errors.New("handshake client has no HTTP session")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode < 400 || response.StatusCode == http.StatusUnauthorized {
		return nil, ErrSessionInvalid
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &Error{Label: label, Status: response.StatusCode, Body: truncate(strings.TrimSpace(string(body)), 200)}
	}
	return body, nil
}

func plainText(value string) string {
	value = htmlBreak.ReplaceAllString(value, "\n")
	value = htmlTag.ReplaceAllString(value, "")
	value = html.UnescapeString(value)
	lines := strings.Split(value, "\n")
	for index := range lines {
		lines[index] = strings.TrimSpace(space.ReplaceAllString(lines[index], " "))
	}
	return strings.TrimSpace(blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

func slug(value string) string {
	var result strings.Builder
	separator := false
	for _, char := range strings.ToLower(strings.TrimSpace(value)) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			if separator && result.Len() > 0 {
				result.WriteByte('-')
			}
			separator = false
			result.WriteRune(char)
		} else {
			separator = true
		}
	}
	return result.String()
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
