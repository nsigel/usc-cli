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
	// without a fresh USC login.
	ErrCredentialsRequired = errors.New("credentials are required")
	// ErrBypassRejected means Duo did not accept the supplied bypass code.
	ErrBypassRejected = errors.New("bypass code was rejected")
)

// Credentials are used only when an existing session cannot reach the target.
type Credentials struct {
	Username   string // USC NetID
	Password   string // USC NetID password
	BypassCode string // current Duo bypass code
}

// Complete reports whether a fresh SSO login can proceed without prompting.
func (c Credentials) Complete() bool {
	return c.Username != "" && c.Password != "" && c.BypassCode != ""
}

type authenticator struct {
	client          *http.Client
	jar             *sessionJar
	duoClientHintUA string
	saml2Request    string
	microsoft       map[string]string
}

// Login restores the cookies in sessionFile and opens target. If USC requests
// a fresh login, Login submits credentials through the USC, Microsoft, and Duo
// flow. A successful call atomically persists the resulting cookie jar.
//
// Target must be an absolute HTTPS URL. A missing session file is treated as
// an empty session. Login returns ErrCredentialsRequired when the session is
// insufficient and credentials is incomplete, and ErrBypassRejected when Duo
// rejects the bypass code.
func Login(ctx context.Context, target, sessionFile string, credentials Credentials) error {
	return login(ctx, target, sessionFile, credentials, false)
}

// LoginFresh ignores the saved session before authenticating and replaces it
// only after the requested application has been reached successfully.
func LoginFresh(ctx context.Context, target, sessionFile string, credentials Credentials) error {
	return login(ctx, target, sessionFile, credentials, true)
}

func login(ctx context.Context, target, sessionFile string, credentials Credentials, fresh bool) error {
	authenticator, err := newAuthenticator()
	if err != nil {
		return err
	}
	if !fresh {
		if err := loadSession(sessionFile, authenticator.jar); err != nil {
			return err
		}
	}
	_, err = authenticator.open(ctx, target, credentials)
	if err != nil {
		return err
	}
	// Half-finished SSO chains contain one-time state that can poison the next
	// attempt, so persist only after the requested application is reached.
	if err := saveSession(sessionFile, authenticator.jar); err != nil {
		return err
	}
	return nil
}

func (a *authenticator) open(ctx context.Context, target string, credentials Credentials) (*page, error) {
	targetURL, err := url.ParseRequestURI(target)
	if err != nil || targetURL.Scheme != "https" || targetURL.Host == "" {
		return nil, errors.New("target must be an absolute HTTPS URL")
	}

	current, err := a.do(ctx, http.MethodGet, targetURL.String(), nil, "", nil, "")
	if err != nil {
		return nil, err
	}
	// Keeping the protocol explicit makes credential submission auditable. The
	// bound turns an identity-provider redirect loop into a useful error.
	for range 40 {
		switch {
		case current.Status >= 300 && current.Status < 400:
			current, err = a.follow(ctx, current)

		case isUSCLogin(current):
			if !credentials.Complete() {
				return nil, ErrCredentialsRequired
			}
			// Credentials are only injected into USC's exact login endpoint, never
			// into a generic form discovered elsewhere in the chain.
			form, formErr := firstForm(current)
			if formErr != nil {
				return nil, fmt.Errorf("USC login form: %w", formErr)
			}
			setField(&form, "j_username", credentials.Username)
			setField(&form, "j_password", credentials.Password)
			current, err = a.submit(ctx, current, form)

		case isUSCSAMLBootstrap(current):
			// USC splits its SAML request across two visits, so the value must
			// survive the intervening credential and Duo pages.
			loginURL := elementValue(current.Body, "loginUrl")
			a.saml2Request = elementValue(current.Body, "saml2Request")
			if a.saml2Request == "" {
				return nil, errors.New("USC SAML redirect page is missing saml2Request")
			}
			current, err = a.do(ctx, http.MethodGet, loginURL, nil, "", current.URL, "")

		case isUSCSAMLContinuation(current):
			form, formErr := firstForm(current)
			if formErr != nil {
				return nil, fmt.Errorf("USC SAML continuation form: %w", formErr)
			}
			setField(&form, "saml2Request", a.saml2Request)
			if secondVisitURL := current.URL.Query().Get("secondVisitUrl"); secondVisitURL != "" {
				form.Action = secondVisitURL
			}
			current, err = a.submit(ctx, current, form)

		case isDuoPrompt(current):
			current, err = a.duo(ctx, current, credentials.BypassCode)

		case current.URL.Host == targetURL.Host && current.Status >= 200 && current.Status < 300:
			// Salesforce serves a successful 200 trampoline before authentication.
			// A real browser follows it, so host and status alone are insufficient.
			if destination, ok := htmlNavigation(current); ok {
				current, err = a.do(ctx, http.MethodGet, destination.String(), nil, "", current.URL, "")
			} else {
				return current, nil
			}

		case needsMicrosoftReload(current):
			// Microsoft's first pass probes ambient Windows SSO. sso_reload asks it
			// to continue into USC federation when that probe has no browser state.
			reloadURL := *current.URL
			query := reloadURL.Query()
			query.Set("sso_reload", "true")
			reloadURL.RawQuery = query.Encode()
			current, err = a.do(ctx, http.MethodGet, reloadURL.String(), nil, "", current.URL, "")

		case isMicrosoftPage(current):
			current, err = a.continueMicrosoft(ctx, current)

		default:
			// SAML handoffs often use an auto-post form or a static HTML redirect
			// instead of an HTTP redirect.
			if form, ok := autoForm(current); ok {
				current, err = a.submit(ctx, current, form)
			} else if destination, ok := htmlNavigation(current); ok {
				current, err = a.do(ctx, http.MethodGet, destination.String(), nil, "", current.URL, "")
			} else {
				return nil, noContinuation(current)
			}
		}
		if err != nil {
			return nil, err
		}
	}
	return nil, errors.New("authentication exceeded 40 protocol steps")
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
	if current.Status == http.StatusTemporaryRedirect || current.Status == http.StatusPermanentRedirect {
		return a.do(ctx, current.RedirectMethod, destination.String(), current.RedirectBody, current.RedirectContentType, current.RedirectReferer, "")
	}
	return a.do(ctx, http.MethodGet, destination.String(), nil, "", current.URL, "")
}

func isUSCLogin(page *page) bool {
	return page.URL.Hostname() == "login.usc.edu" && page.URL.Path == "/login/login"
}

func isUSCSAMLBootstrap(page *page) bool {
	return page.URL.Hostname() == "login.usc.edu" &&
		page.URL.Path == "/sso/SSORedirect/metaAlias/USCRealm/idp" &&
		elementValue(page.Body, "loginUrl") != ""
}

func isUSCSAMLContinuation(page *page) bool {
	return page.URL.Hostname() == "login.usc.edu" && page.URL.Path == "/sso/saml2/continue/metaAlias/USCRealm/idp"
}

func isDuoPrompt(page *page) bool {
	return strings.HasPrefix(page.URL.Host, "api-") &&
		strings.Contains(page.URL.Host, ".duosecurity.com") &&
		strings.HasPrefix(page.URL.Path, "/prompt/")
}

func isMicrosoftPage(page *page) bool {
	return page.URL.Host == "login.microsoftonline.com"
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
