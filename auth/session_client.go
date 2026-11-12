package auth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"

	http "github.com/saucesteals/fhttp"
)

// Session sends requests with USC browser cookies. Do never reauthenticates;
// Authenticate is the explicit SSO handoff.
type Session struct {
	authenticator *authenticator
	options       Options
	credentials   Credentials
	policy        CookiePolicy
}

// CookiePolicy makes a session start over at some sites each time it is
// opened. LibCal needs this: a stale checkout cookie breaks a new booking.
type CookiePolicy struct {
	ResetDomains    []string // Skip these domains' saved cookies; replace them on save.
	PreserveCookies []string // Cookie names within ResetDomains to leave untouched on disk.
}

// OpenSession restores the cookies in options.SessionFile, if any. Nothing
// is written until Authenticate succeeds.
func OpenSession(options Options, policy CookiePolicy) (*Session, error) {
	credentials, err := options.ResolveCredentials()
	if err != nil {
		return nil, err
	}
	authenticator, err := newAuthenticator()
	if err != nil {
		return nil, err
	}
	if options.SessionFile != "" {
		if err := loadSession(options.SessionFile, authenticator.jar, policy.ResetDomains); err != nil {
			return nil, err
		}
	}
	return &Session{authenticator: authenticator, options: options, credentials: credentials, policy: policy}, nil
}

// Do implements the small HTTP contract used by site clients.
func (s *Session) Do(request *http.Request) (*http.Response, error) {
	return s.authenticator.client.Do(request)
}

// Authenticate opens target, completing USC SSO with the session's
// credentials if its cookies are not enough, then saves the cookies. The
// caller owns the final response body, including when saving returns an error.
func (s *Session) Authenticate(ctx context.Context, target, referer string) (*http.Response, error) {
	var refererURL *url.URL
	if referer != "" {
		parsed, err := url.ParseRequestURI(referer)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return nil, errors.New("referer must be an absolute HTTPS URL")
		}
		refererURL = parsed
	}
	page, err := s.authenticator.open(ctx, target, s.credentials, refererURL)
	if err != nil {
		forgetRejected(s.options.CredentialsFile, err, s.credentials)
		return nil, err
	}
	response := &http.Response{
		StatusCode: page.Status,
		Header:     page.Header,
		Body:       io.NopCloser(bytes.NewReader(page.Body)),
		Request:    &http.Request{URL: page.URL},
	}
	return response, s.save()
}

func (s *Session) save() error {
	switch {
	case s.options.SessionFile == "":
		return nil
	case len(s.policy.ResetDomains) > 0:
		return saveSessionReplacingDomains(s.options.SessionFile, s.authenticator.jar, s.policy.ResetDomains, s.policy.PreserveCookies)
	}
	return saveSession(s.options.SessionFile, s.authenticator.jar)
}
