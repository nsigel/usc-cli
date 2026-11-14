package auth

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/nsigel/usc-cli/internal/dom"
	"github.com/nsigel/usc-cli/site"
)

const bypassURL = "https://account.usc.edu/2fa/duo-bypass-code"

// BypassCode contains a secret. Callers must not log it.
type BypassCode struct {
	Code             string `json:"code"`
	Validity         string `json:"validity,omitempty"`
	CredentialsSaved bool   `json:"credentials_saved"`
}

var bypassCodePattern = regexp.MustCompile(`^[0-9]{9}$`)

// CreateBypassCode requests a Duo bypass code from USC Account Services.
// Issuance may invalidate earlier codes. It never prompts or retries issuance.
// If CredentialsFile is set, the resolved credentials are saved with the new
// code before session persistence. Otherwise the caller owns the returned code.
// Validity is USC's description, not an independently verified expiration time.
// A non-nil result may accompany a persistence error after issuance succeeded.
func CreateBypassCode(ctx context.Context, options Options) (*BypassCode, error) {
	session, err := OpenSession(options, CookiePolicy{})
	if err != nil {
		return nil, err
	}
	// Persist the issued credential before cookies: a session write failure
	// must not discard a replacement for an invalidated bypass code.
	page, err := session.authenticator.open(ctx, bypassURL, session.credentials, nil)
	if err != nil {
		forgetRejected(options.CredentialsFile, err, session.credentials)
		return nil, ForSite(site.Account, err)
	}
	if page.URL.Scheme != "https" || page.URL.Host != "account.usc.edu" || page.URL.Path != "/2fa/duo-bypass-code" {
		return nil, ForSite(site.Account, ErrSessionInvalid)
	}
	document := dom.Parse(page.Body)
	code := dom.Attr(dom.FindFirst(document, dom.ID("bypassCode")), "value")
	if !bypassCodePattern.MatchString(code) {
		return nil, ForSite(site.Account, fmt.Errorf("%w: bypass-code page did not contain a valid code; interactive identity verification may be required", ErrSessionInvalid))
	}
	result := &BypassCode{Code: code}
	if options.CredentialsFile != "" {
		credentials := session.credentials
		credentials.BypassCode = code
		if err := SaveCredentials(options.CredentialsFile, credentials); err != nil {
			return result, fmt.Errorf("bypass code issued but credentials could not be saved: %w", err)
		}
		result.CredentialsSaved = true
	}
	for _, paragraph := range dom.FindAll(document, dom.Tag("p")) {
		text := dom.Collapse(dom.Text(paragraph))
		if strings.HasPrefix(text, "Your bypass code will expire ") {
			result.Validity, _, _ = strings.Cut(text, ".")
			break
		}
	}
	if err := session.save(); err != nil {
		return result, fmt.Errorf("bypass code issued but session could not be saved: %w", err)
	}
	return result, nil
}
