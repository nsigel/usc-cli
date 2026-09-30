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
var ErrAuthenticationRequired = errors.New("LibCal authentication required")

// Doer is the HTTP surface used by Client. It accepts the same request type as
// usc-cli's authenticated session so callers can supply their own transport.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client reads and books Leavey Library spaces through LibCal.
type Client struct {
	http         Doer
	authenticate func(context.Context, string, string) error
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
	session, err := auth.OpenSessionWithDomainPolicy(path, []string{"libcal.usc.edu", "libauth.com"}, []string{"lc_ebcart"})
	if err != nil {
		return nil, fmt.Errorf("open USC session: %w", err)
	}
	client := New(session)
	client.authenticate = func(ctx context.Context, target, referer string) error {
		err := session.Authenticate(ctx, target, auth.Credentials{}, referer)
		if errors.Is(err, auth.ErrCredentialsRequired) {
			return ErrAuthenticationRequired
		}
		return err
	}
	return client, nil
}

// ResponseError describes a rejected LibCal HTTP request without exposing the
// response body, which may contain user or session state.
type ResponseError struct {
	Operation string
	Status    int
	BodyBytes int
	Reason    string
}

func (e *ResponseError) Error() string {
	message := fmt.Sprintf("LibCal %s: HTTP %d", e.Operation, e.Status)
	if e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden {
		message = fmt.Sprintf("LibCal %s: %v (HTTP %d)", e.Operation, ErrAuthenticationRequired, e.Status)
	}
	if e.BodyBytes > 0 {
		message += fmt.Sprintf(" (%d-byte response)", e.BodyBytes)
	}
	if e.Reason != "" {
		message += ": " + e.Reason
	}
	return message
}

func (e *ResponseError) Unwrap() error {
	if e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden {
		return ErrAuthenticationRequired
	}
	return nil
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
		location, _ := url.Parse(response.Header.Get("Location"))
		if strings.Contains(location.Path, "/spaces/auth") || strings.Contains(location.Host, "login.usc.edu") {
			return nil, ErrAuthenticationRequired
		}
		return nil, &ResponseError{Operation: method + " " + path, Status: response.StatusCode}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &ResponseError{
			Operation: method + " " + path,
			Status:    response.StatusCode,
			BodyBytes: len(data),
			Reason:    responseReason(data),
		}
	}
	return data, nil
}

func responseReason(data []byte) string {
	body := strings.ToLower(string(data))
	switch {
	case strings.Contains(body, "invalid id"):
		return "LibCal rejected the session identifier"
	case strings.Contains(body, "patron"):
		return "LibCal rejected reservation identity data"
	case strings.Contains(body, "checksum"):
		return "LibCal rejected the availability slot"
	case strings.Contains(body, "authentication") || strings.Contains(body, "sign in"):
		return "LibCal rejected the authentication state"
	case strings.Contains(body, "reservation") || strings.Contains(body, "booking") || strings.Contains(body, "room"):
		return "LibCal rejected the reservation details"
	case strings.Contains(body, "session") || strings.Contains(body, "state") || strings.Contains(body, "token"):
		return "LibCal rejected the session state"
	case strings.Contains(body, "request") || strings.Contains(body, "data") || strings.Contains(body, "field"):
		return "LibCal rejected the request data"
	default:
		return "LibCal rejected the request"
	}
}
