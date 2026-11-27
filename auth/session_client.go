package auth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"

	http "github.com/saucesteals/fhttp"
)

type Session struct {
	authenticator *authenticator
	options       Options
	credentials   Credentials
	policy        CookiePolicy
}

type CookiePolicy struct {
	ResetDomains    []string
	PreserveCookies []string
}

func OpenSession(options Options, policy CookiePolicy) (*Session, error) {
	credentials, err := options.ResolveCredentials()
	if err != nil {
		return nil, err
	}
	authenticator, err := newAuthenticator()
	if err != nil {
		return nil, err
	}
	authenticator.requestDuoPasscode = options.RequestDuoPasscode
	authenticator.duoPasscode = options.DuoPasscode
	authenticator.duoPushDevice = options.DuoPushDevice
	authenticator.duoSMSDevice = options.DuoSMSDevice
	authenticator.duoPushStarted = options.DuoPushStarted
	if options.SessionFile != "" {
		if err := loadSession(options.SessionFile, authenticator.jar, policy.ResetDomains); err != nil {
			return nil, err
		}
	}
	return &Session{authenticator: authenticator, options: options, credentials: credentials, policy: policy}, nil
}

func (s *Session) Do(request *http.Request) (*http.Response, error) {
	return s.authenticator.client.Do(request)
}

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
