package auth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"slices"

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

// OpenSession restores cookies from SessionStore or SessionFile, if set. Nothing
// is written until Authenticate succeeds.
func OpenSession(options Options, policy CookiePolicy) (*Session, error) {
	if options.SessionStore != nil && options.SessionFile != "" {
		return nil, errors.New("set either SessionStore or SessionFile, not both")
	}
	if options.SessionFile != "" {
		options.SessionStore = FileSessionStore{Path: options.SessionFile}
	}
	credentials, err := options.ResolveCredentials()
	if err != nil {
		return nil, err
	}
	authenticator, err := newAuthenticator()
	if err != nil {
		return nil, err
	}
	if options.SessionStore != nil {
		if err := loadStoredSession(options.SessionStore, authenticator.jar, policy.ResetDomains); err != nil {
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
	store := s.options.SessionStore
	if store == nil {
		return nil
	}
	cookies := s.authenticator.jar.snapshot()
	if len(s.policy.PreserveCookies) > 0 {
		previous, err := store.Load()
		if err != nil && !errors.Is(err, ErrSessionNotFound) {
			return err
		}
		preserved := func(entry StoredCookie) bool {
			return cookieMatchesDomains(entry, s.policy.ResetDomains) && slices.Contains(s.policy.PreserveCookies, entry.Cookie.Name)
		}
		cookies = slices.DeleteFunc(cookies, preserved)
		for _, entry := range previous {
			if preserved(entry) {
				cookies = append(cookies, entry)
			}
		}
	}
	return store.Save(cookies)
}
