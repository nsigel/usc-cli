package auth

import (
	"context"
	"errors"
	"net/url"
	"os"

	http "github.com/saucesteals/fhttp"
)

// ErrSessionNotFound means no saved browser session is available.
var ErrSessionNotFound = errors.New("no saved session")

// Session sends requests with USC browser cookies. Do never reauthenticates;
// Authenticate is an explicit handoff used by auth commands and site clients.
type Session struct {
	client         *http.Client
	authenticator  *authenticator
	sessionFile    string
	preserve       map[string]bool
	replaceDomains map[string]bool
}

// OpenSession restores the saved browser session for use by a site client.
func OpenSession(sessionFile string) (*Session, error) {
	if _, err := os.Stat(sessionFile); errors.Is(err, os.ErrNotExist) {
		return nil, ErrSessionNotFound
	} else if err != nil {
		return nil, err
	}
	return openSession(sessionFile, nil, nil, nil)
}

// OpenSessionWithDomainPolicy restores the USC session without sending cookies
// from the listed domains. After Authenticate succeeds, those domains are
// replaced with the new flow's cookies, while preserved cookie names retain
// their old values.
func OpenSessionWithDomainPolicy(sessionFile string, ignoredDomains, preserveCookies []string) (*Session, error) {
	ignored := make(map[string]bool, len(ignoredDomains))
	for _, domain := range ignoredDomains {
		ignored[domain] = true
	}
	preserve := make(map[string]bool, len(preserveCookies))
	for _, name := range preserveCookies {
		preserve[name] = true
	}
	return openSession(sessionFile, ignored, preserve, ignored)
}

// OpenFreshSession creates an empty cookie jar that is only persisted if
// Authenticate succeeds.
func OpenFreshSession(sessionFile string) (*Session, error) {
	authenticator, err := newAuthenticator()
	if err != nil {
		return nil, err
	}
	return &Session{client: authenticator.client, authenticator: authenticator, sessionFile: sessionFile}, nil
}

func openSession(sessionFile string, ignoredDomains, preserve, replaceDomains map[string]bool) (*Session, error) {
	if _, err := os.Stat(sessionFile); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	authenticator, err := newAuthenticator()
	if err != nil {
		return nil, err
	}
	if ignoredDomains != nil {
		if err := loadSessionWithPolicy(sessionFile, authenticator.jar, ignoredDomains); err != nil {
			return nil, err
		}
	} else {
		if err := loadSession(sessionFile, authenticator.jar); err != nil {
			return nil, err
		}
	}
	return &Session{client: authenticator.client, authenticator: authenticator, sessionFile: sessionFile, preserve: preserve, replaceDomains: replaceDomains}, nil
}

// Do implements the small HTTP contract used by site clients.
func (s *Session) Do(request *http.Request) (*http.Response, error) {
	return s.client.Do(request)
}

// Authenticate follows a site's SSO redirect using this session's existing
// cookie jar. This is useful for applications such as LibCal that only create
// a valid SSO redirect after a booking has been staged.
func (s *Session) Authenticate(ctx context.Context, target string, credentials Credentials, referer string) error {
	if s == nil || s.authenticator == nil {
		return errors.New("USC session is unavailable")
	}
	var refererURL *url.URL
	if referer != "" {
		parsed, err := url.ParseRequestURI(referer)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return errors.New("referer must be an absolute HTTPS URL")
		}
		refererURL = parsed
	}
	if _, err := s.authenticator.open(ctx, target, credentials, refererURL); err != nil {
		return err
	}
	if len(s.replaceDomains) > 0 {
		return saveSessionReplacingDomains(s.sessionFile, s.authenticator.jar, s.replaceDomains, s.preserve)
	}
	return saveSession(s.sessionFile, s.authenticator.jar)
}
