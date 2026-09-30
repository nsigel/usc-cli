package auth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"os"

	http "github.com/saucesteals/fhttp"
)

// ErrSessionNotFound means no saved browser session is available.
var ErrSessionNotFound = errors.New("no saved session")

// Session sends requests with USC browser cookies. Do never reauthenticates;
// Authenticate is an explicit handoff used by auth commands and site clients.
type Session struct {
	authenticator *authenticator
	sessionFile   string
	options       SessionOptions
}

// SessionOptions controls how saved cookies are restored and persisted.
// The zero value requires an existing session and restores all cookies.
type SessionOptions struct {
	Fresh           bool     // Start empty and replace the saved session after authentication.
	AllowMissing    bool     // Treat a missing file as an empty session.
	ResetDomains    []string // Omit these domains' cookies and replace them after authentication.
	PreserveCookies []string // Retain these cookie names within ResetDomains on disk only.
}

// OpenSession opens a cookie jar for a site client. It writes nothing until
// Authenticate succeeds.
func OpenSession(sessionFile string, options SessionOptions) (*Session, error) {
	if !options.Fresh && !options.AllowMissing {
		if _, err := os.Stat(sessionFile); errors.Is(err, os.ErrNotExist) {
			return nil, ErrSessionNotFound
		} else if err != nil {
			return nil, err
		}
	}
	authenticator, err := newAuthenticator()
	if err != nil {
		return nil, err
	}
	if !options.Fresh {
		if err := loadSession(sessionFile, authenticator.jar, options.ResetDomains); err != nil {
			return nil, err
		}
	}
	return &Session{authenticator: authenticator, sessionFile: sessionFile, options: options}, nil
}

// Do implements the small HTTP contract used by site clients.
func (s *Session) Do(request *http.Request) (*http.Response, error) {
	return s.authenticator.client.Do(request)
}

// Authenticate follows a site's SSO redirect using this session's existing
// cookie jar. This is useful for applications such as LibCal that only create
// a valid SSO redirect after a booking has been staged. The caller owns the
// final response body, including when persisting the session returns an error.
func (s *Session) Authenticate(ctx context.Context, target string, credentials Credentials, referer string) (*http.Response, error) {
	if s == nil || s.authenticator == nil {
		return nil, errors.New("USC session is unavailable")
	}
	var refererURL *url.URL
	if referer != "" {
		parsed, err := url.ParseRequestURI(referer)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return nil, errors.New("referer must be an absolute HTTPS URL")
		}
		refererURL = parsed
	}
	page, err := s.authenticator.open(ctx, target, credentials, refererURL)
	if err != nil {
		return nil, err
	}
	response := &http.Response{
		StatusCode: page.Status,
		Header:     page.Header,
		Body:       io.NopCloser(bytes.NewReader(page.Body)),
		Request:    &http.Request{URL: page.URL},
	}
	if !s.options.Fresh && len(s.options.ResetDomains) > 0 {
		err = saveSessionReplacingDomains(s.sessionFile, s.authenticator.jar, s.options.ResetDomains, s.options.PreserveCookies)
	} else {
		err = saveSession(s.sessionFile, s.authenticator.jar)
	}
	return response, err
}
