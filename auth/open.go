package auth

import (
	"context"
	"errors"

	"github.com/nsigel/usc-cli/config"
	"github.com/nsigel/usc-cli/site"
)

// Options configures a service client's USC authentication. The zero value
// reuses the CLI's saved session without prompting or loading saved passwords.
type Options struct {
	// SessionFile overrides the shared session path resolved by config.SessionPath.
	SessionFile string
	// Credentials are submitted only if the existing SSO session is insufficient.
	// They are not persisted. An incomplete value yields ErrCredentialsRequired
	// when a fresh login is needed.
	Credentials Credentials
}

// SessionPath resolves the explicit session file or the shared CLI default.
func (o Options) SessionPath() (string, error) {
	if o.SessionFile != "" {
		return o.SessionFile, nil
	}
	return config.SessionPath()
}

// Open establishes a site's session through USC SSO and saves its cookies.
// It never prompts. LibCal uses shared SSO here; libcal.Open performs its
// booking-specific handoff later. Public sites do not need authentication.
func Open(ctx context.Context, name site.Name, options Options) (*Session, error) {
	selected, err := site.Find(name)
	if err != nil {
		return nil, err
	}
	if selected.LoginURL == "" {
		return nil, errors.New("site does not require USC authentication")
	}
	path, err := options.SessionPath()
	if err != nil {
		return nil, err
	}
	session, err := OpenSession(path, SessionOptions{AllowMissing: true})
	if err != nil {
		return nil, err
	}
	response, err := session.Authenticate(ctx, selected.LoginURL, options.Credentials, "")
	if response != nil {
		response.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	return session, nil
}
