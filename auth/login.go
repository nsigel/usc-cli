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
	ErrDuoSMSRejected      = errors.New("Duo rejected the SMS code")
	ErrCredentialsRequired = errors.New("USC sign-in is required")

	ErrPasswordRejected = errors.New("USC rejected the username or password")

	ErrBypassRejected = errors.New("Duo rejected the bypass code")

	ErrDuoPasscodeRejected = errors.New("Duo rejected the mobile passcode")

	ErrDuoPushRejected = errors.New("Duo push was not approved")
	ErrDuoPushTimedOut = fmt.Errorf("%w: approval timed out", ErrDuoPushRejected)

	ErrSessionInvalid = errors.New("session expired")
)

type Credentials struct {
	DuoDeviceID   string
	Username      string
	Password      string
	BypassCode    string
	DuoTOTPSecret string
}

func (c Credentials) Complete() bool {
	return c.Username != "" && c.Password != "" && (c.BypassCode != "" || c.DuoTOTPSecret != "")
}

const maxProtocolSteps = 40

type authenticator struct {
	duoSMSDevice       string
	requestDuoPasscode func() (string, error)
	duoPasscode        string
	duoPushDevice      string
	duoPushStarted     func(string)
	duoFactors         []DuoMethod
	client             *http.Client
	jar                *sessionJar
	duoClientHintUA    string
	microsoft          map[string]string
	passwordSent       bool
	duoStarted         bool
}

func (a *authenticator) open(ctx context.Context, target string, credentials Credentials, initialReferer *url.URL) (*page, error) {
	targetURL, err := url.ParseRequestURI(target)
	if err != nil || targetURL.Scheme != "https" || targetURL.Host == "" {
		return nil, errors.New("target must be an absolute HTTPS URL")
	}
	a.passwordSent, a.duoStarted = false, false
	current, err := a.get(ctx, targetURL.String(), initialReferer)
	for range maxProtocolSteps {
		if err != nil {
			return nil, err
		}
		switch {
		case targetURL.Host == "account.usc.edu" && targetURL.Path == "/2fa/enroll" &&
			current.URL.Scheme == "https" && current.URL.Host == "uw1.devicemanagement.duosecurity.com" &&
			current.URL.Path == "/frame/device-management/portal" && current.Status == 200:
			return current, nil
		case current.Status >= 300 && current.Status < 400:
			current, err = a.follow(ctx, current)
		case isDuoPrompt(current):
			current, err = a.duo(ctx, current, credentials)
		case current.URL.Host == targetURL.Host && current.Status >= 200 && current.Status < 300:

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

func (a *authenticator) reloadMicrosoft(ctx context.Context, current *page) (*page, error) {
	reload := *current.URL
	query := reload.Query()
	query.Set("sso_reload", "true")
	reload.RawQuery = query.Encode()
	return a.get(ctx, reload.String(), current.URL)
}

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
	host := strings.ToLower(page.URL.Hostname())
	return strings.HasPrefix(host, "api-") &&
		strings.HasSuffix(host, ".duosecurity.com") &&
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
