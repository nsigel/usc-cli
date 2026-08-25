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

const DefaultTarget = "https://webreg.usc.edu/Terms"

var (
	ErrCredentialsRequired = errors.New("credentials are required")
	ErrBypassRejected      = errors.New("bypass code was rejected")
)

// Credentials are used only when an existing session cannot reach the target.
type Credentials struct {
	Username   string
	Password   string
	BypassCode string
}

// Complete reports whether a fresh SSO login can proceed without prompting.
func (c Credentials) Complete() bool {
	return c.Username != "" && c.Password != "" && c.BypassCode != ""
}

type authenticator struct {
	client          *http.Client
	jar             *sessionJar
	saml2Request    string
	microsoft       map[string]string
	usedCredentials bool
}

// Result describes the authenticated page reached by Login.
type Result struct {
	URL             string
	Status          int
	Reauthenticated bool
}

// Login restores a saved session, authenticates when necessary, and persists
// the resulting cookies.
func Login(ctx context.Context, target, sessionFile string, credentials Credentials) (Result, error) {
	authenticator, err := newAuthenticator()
	if err != nil {
		return Result{}, err
	}
	if err := loadSession(sessionFile, authenticator.jar); err != nil {
		return Result{}, err
	}
	page, err := authenticator.open(ctx, target, credentials)
	if err != nil {
		return Result{}, err
	}
	if err := saveSession(sessionFile, authenticator.jar); err != nil {
		return Result{}, err
	}
	return Result{
		URL:             page.URL.String(),
		Status:          page.Status,
		Reauthenticated: authenticator.usedCredentials,
	}, nil
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
	for range 40 {
		switch {
		case current.Status >= 300 && current.Status < 400:
			current, err = a.follow(ctx, current)

		case isUSCLogin(current):
			if !credentials.Complete() {
				return nil, ErrCredentialsRequired
			}
			a.usedCredentials = true
			form, formErr := firstForm(current)
			if formErr != nil {
				return nil, fmt.Errorf("USC login form: %w", formErr)
			}
			setField(&form, "j_username", credentials.Username)
			setField(&form, "j_password", credentials.Password)
			current, err = a.submit(ctx, current, form)

		case isUSCSAMLBootstrap(current):
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
			return current, nil

		case needsMicrosoftReload(current):
			reloadURL := *current.URL
			query := reloadURL.Query()
			query.Set("sso_reload", "true")
			reloadURL.RawQuery = query.Encode()
			current, err = a.do(ctx, http.MethodGet, reloadURL.String(), nil, "", current.URL, "")

		case isMicrosoftPage(current):
			current, err = a.continueMicrosoft(ctx, current)

		default:
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
		strings.HasSuffix(page.URL.Path, "/oauth2/v2.0/authorize") &&
		page.URL.Query().Get("sso_reload") != "true"
}

func noContinuation(page *page) error {
	if isMicrosoftPage(page) {
		return fmt.Errorf("microsoft page has no form or supported redirect (title: %q)", pageTitle(page.Body))
	}
	return fmt.Errorf("no safe continuation for %s%s (HTTP %d)", page.URL.Host, page.URL.Path, page.Status)
}
