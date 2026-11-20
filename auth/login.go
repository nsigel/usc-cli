// Package auth implements USC's shared SSO flow without exposing
// secret-bearing values.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	http "github.com/saucesteals/fhttp"
)

var (
	// ErrCredentialsRequired means the saved session cannot reach the target
	// and no complete credentials were supplied for a fresh USC login.
	ErrCredentialsRequired = errors.New("USC sign-in is required")
	// ErrPasswordRejected means USC did not accept the username and password.
	ErrPasswordRejected = errors.New("USC rejected the username or password")
	// ErrBypassRejected means Duo did not accept the supplied bypass code.
	ErrBypassRejected = errors.New("Duo rejected the bypass code")
	// ErrSessionInvalid means a site stopped accepting its session after the
	// client was opened. Open a new client.
	ErrSessionInvalid = errors.New("session expired")
	// ErrSessionNotFound means there is no saved session file.
	ErrSessionNotFound = errors.New("no saved session")
)

// Credentials are used only when an existing session cannot reach the target.
type Credentials struct {
	Username   string // USC NetID
	Password   string // USC NetID password
	BypassCode string // current Duo bypass code
}

// Complete reports whether a fresh SSO login can proceed.
func (c Credentials) Complete() bool {
	return c.Username != "" && c.Password != "" && c.BypassCode != ""
}

// maxProtocolSteps turns an identity-provider redirect loop into an error.
const maxProtocolSteps = 40

type authenticator struct {
	client          *http.Client
	jar             *sessionJar
	duoClientHintUA string
	microsoft       map[string]string
	passwordSent    bool // the password went out during this open
	duoStarted      bool // Duo was chosen during this open
}

// open navigates to target, taking one sign-in step per page until it gets
// there. Every step is explicit so that credentials are only ever sent to
// the exact pages that ask for them.
func (a *authenticator) open(ctx context.Context, target string, credentials Credentials, initialReferer *url.URL) (*page, error) {
	targetURL, err := url.ParseRequestURI(target)
	if err != nil {
		return nil, errors.New("target must be an absolute HTTPS URL")
	}
	a.passwordSent, a.duoStarted = false, false
	current, err := a.get(ctx, targetURL.String(), initialReferer)
	for range maxProtocolSteps {
		if err != nil {
			return nil, err
		}
		switch {
		case current.Status >= 300 && current.Status < 400:
			current, err = a.follow(ctx, current)
		case isDuoPrompt(current):
			current, err = a.duo(ctx, current, credentials.BypassCode)
		case current.URL.Host == targetURL.Host && current.Status >= 200 && current.Status < 300:
			// Salesforce serves a successful 200 trampoline before authentication.
			// A real browser follows it, so host and status alone are insufficient.
			destination, ok := staticRedirect(current)
			if !ok {
				return current, nil
			}
			current, err = a.get(ctx, destination.String(), current.URL)
		case isMicrosoftPage(current):
			current, err = a.microsoftStep(ctx, current, credentials)
		default:
			current, err = a.continueHandoff(ctx, current)
		}
	}
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("authentication exceeded %d protocol steps", maxProtocolSteps)
}

func (a *authenticator) follow(ctx context.Context, current *page) (*page, error) {
	location := current.Header.Get("Location")
	if location == "" {
		return nil, fmt.Errorf("redirect from %s%s has no Location", current.URL.Host, current.URL.Path)
	}
	destination, err := current.URL.Parse(location)
	if err != nil {
		return nil, err
	}
	// 307 and 308 repeat the original request at the new address. A form
	// posted to Microsoft can hold the password, so it stays with Microsoft.
	if current.Status == http.StatusTemporaryRedirect || current.Status == http.StatusPermanentRedirect {
		if len(current.request.body) > 0 && isMicrosoftHost(current.URL) && !isMicrosoftHost(destination) {
			return nil, fmt.Errorf("refusing to repeat a form from %s to %s", current.URL.Host, destination.Host)
		}
		replay := current.request
		replay.url = destination.String()
		replay.duoTraceGroup = ""
		return a.do(ctx, replay)
	}
	return a.get(ctx, destination.String(), current.URL)
}

// reloadMicrosoft moves Microsoft past its first pass, which probes ambient
// Windows SSO. sso_reload continues to the sign-in page without it.
func (a *authenticator) reloadMicrosoft(ctx context.Context, current *page) (*page, error) {
	reload := *current.URL
	query := reload.Query()
	query.Set("sso_reload", "true")
	reload.RawQuery = query.Encode()
	return a.get(ctx, reload.String(), current.URL)
}

// continueHandoff advances a page no provider claims: SAML handoffs use an
// auto-posting form or a static redirect instead of an HTTP redirect.
func (a *authenticator) continueHandoff(ctx context.Context, current *page) (*page, error) {
	if form, ok := firstForm(current); ok {
		return a.submit(ctx, current, form)
	}
	if destination, ok := staticRedirect(current); ok {
		return a.get(ctx, destination.String(), current.URL)
	}
	return nil, noContinuation(current)
}

func isDuoPrompt(page *page) bool {
	return strings.HasPrefix(page.URL.Host, "api-") &&
		strings.Contains(page.URL.Host, ".duosecurity.com") &&
		strings.HasPrefix(page.URL.Path, "/prompt/")
}

func isMicrosoftPage(page *page) bool {
	return isMicrosoftHost(page.URL)
}

func isMicrosoftHost(target *url.URL) bool {
	return target.Hostname() == "login.microsoftonline.com"
}

func needsMicrosoftReload(page *page) bool {
	return isMicrosoftPage(page) &&
		(strings.HasSuffix(page.URL.Path, "/oauth2/v2.0/authorize") || strings.HasSuffix(page.URL.Path, "/saml2")) &&
		page.URL.Query().Get("sso_reload") != "true"
}

func noContinuation(page *page) error {
	if isMicrosoftPage(page) {
		return fmt.Errorf("microsoft page has no form or supported redirect (title: %q)", pageTitle(page.Body))
	}
	return fmt.Errorf("no safe continuation for %s%s (HTTP %d)", page.URL.Host, page.URL.Path, page.Status)
}
