// Package auth implements USC's shared SSO flow without exposing
// secret-bearing values.
package auth

import (
	"bytes"
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
	saml2Request    string
	microsoft       map[string]string
}

func (a *authenticator) open(ctx context.Context, target string, credentials Credentials, initialReferer *url.URL) (*page, error) {
	targetURL, err := url.ParseRequestURI(target)
	if err != nil || targetURL.Scheme != "https" || targetURL.Host == "" {
		return nil, errors.New("target must be an absolute HTTPS URL")
	}

	current, err := a.do(ctx, http.MethodGet, targetURL.String(), nil, "", initialReferer, "")
	if err != nil {
		return nil, err
	}
	for range maxProtocolSteps {
		switch {
		case current.Status >= 300 && current.Status < 400:
			current, err = a.follow(ctx, current)

		case isUSCLogin(current):
			if !credentials.Complete() {
				return nil, ErrCredentialsRequired
			}
			form, ok := firstForm(current)
			if !ok {
				return nil, errors.New("USC login page has no form")
			}
			setField(&form, "j_username", credentials.Username)
			setField(&form, "j_password", credentials.Password)
			current, err = a.submit(ctx, current, form)

		case isUSCLoginRejected(current):
			return nil, ErrPasswordRejected

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
			form, ok := firstForm(current)
			if !ok {
				return nil, errors.New("USC SAML continuation page has no form")
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
			if destination, ok := staticRedirect(current); ok {
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
			if form, ok := firstForm(current); ok {
				current, err = a.submit(ctx, current, form)
			} else if destination, ok := staticRedirect(current); ok {
				current, err = a.do(ctx, http.MethodGet, destination.String(), nil, "", current.URL, "")
			} else {
				return nil, noContinuation(current)
			}
		}
		if err != nil {
			return nil, err
		}
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
	if current.Status == http.StatusTemporaryRedirect || current.Status == http.StatusPermanentRedirect {
		return a.do(ctx, current.RedirectMethod, destination.String(), current.RedirectBody, current.RedirectContentType, current.RedirectReferer, "")
	}
	return a.do(ctx, http.MethodGet, destination.String(), nil, "", current.URL, "")
}

func isUSCLogin(page *page) bool {
	return page.URL.Hostname() == "login.usc.edu" && page.URL.Path == "/login/login"
}

func isUSCLoginRejected(page *page) bool {
	return page.URL.Hostname() == "login.usc.edu" && page.URL.Path == "/login/authuserpassword" &&
		bytes.Contains(page.Body, []byte(`id="loginform"`))
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
