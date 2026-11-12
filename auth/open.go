package auth

import (
	"context"
	"errors"

	"github.com/nsigel/usc-cli/site"
)

// Options is how a program signs in. The zero value has no credentials and
// keeps its cookies in memory, so it can only reach public pages.
//
// Supply credentials either directly or as a credentials file written by usc
// auth login (config.CredentialsPath is the usc command's). Set SessionFile
// to keep cookies between runs; a program owns its session file and should
// not share one with the usc command or with another running program.
type Options struct {
	// Credentials are submitted only when the session's cookies are not enough.
	// Fields set here override those read from CredentialsFile.
	Credentials Credentials
	// CredentialsFile supplies any credentials not set above. A secret from
	// it that USC rejects is removed from the file so it is never submitted
	// twice.
	CredentialsFile string
	// SessionFile stores the session's cookies. Empty keeps them in memory.
	SessionFile string
}

// ResolveCredentials returns the credentials a login would submit: the
// explicit ones, completed from CredentialsFile.
func (o Options) ResolveCredentials() (Credentials, error) {
	credentials := o.Credentials
	if o.CredentialsFile == "" {
		return credentials, nil
	}
	saved, err := LoadCredentials(o.CredentialsFile)
	if err != nil {
		return Credentials{}, err
	}
	if credentials.Username == "" {
		credentials.Username = saved.Username
	}
	if credentials.Password == "" {
		credentials.Password = saved.Password
	}
	if credentials.BypassCode == "" {
		credentials.BypassCode = saved.BypassCode
	}
	return credentials, nil
}

// SiteError is an authentication failure at one site. It unwraps to
// ErrCredentialsRequired, ErrPasswordRejected, ErrBypassRejected, or
// ErrSessionInvalid.
type SiteError struct {
	Site site.Name
	Err  error
}

func (e *SiteError) Error() string { return string(e.Site) + ": " + e.Err.Error() }
func (e *SiteError) Unwrap() error { return e.Err }

// ForSite attributes an authentication failure to a site. Other errors, and
// errors already attributed, pass through unchanged.
func ForSite(name site.Name, err error) error {
	var attributed *SiteError
	if errors.As(err, &attributed) {
		return err
	}
	for _, failure := range []error{ErrCredentialsRequired, ErrPasswordRejected, ErrBypassRejected, ErrSessionInvalid} {
		if errors.Is(err, failure) {
			return &SiteError{Site: name, Err: err}
		}
	}
	return err
}

// Open signs in to a site through USC SSO and returns its session. It never
// prompts. Authentication failures are returned as *SiteError.
func Open(ctx context.Context, name site.Name, options Options) (*Session, error) {
	target, err := site.LoginURL(name)
	if err != nil {
		return nil, err
	}
	session, err := OpenSession(options, CookiePolicy{})
	if err != nil {
		return nil, err
	}
	response, err := session.Authenticate(ctx, target, "")
	if response != nil {
		response.Body.Close()
	}
	if err != nil {
		return nil, ForSite(name, err)
	}
	return session, nil
}
