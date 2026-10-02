package brightspace

import (
	"context"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/site"
)

// Open creates an authenticated client using the CLI's saved USC session.
// It establishes the application session through SSO without prompting.
func Open(ctx context.Context) (*Client, error) {
	return OpenWithOptions(ctx, auth.Options{})
}

// OpenWithOptions creates an authenticated client with an optional session path
// and explicit credentials. Missing or expired SSO requires credentials and
// returns auth.ErrCredentialsRequired when they have not been supplied.
// Reopen the client if a later API call returns ErrSessionInvalid.
func OpenWithOptions(ctx context.Context, options auth.Options) (*Client, error) {
	session, err := auth.Open(ctx, site.Brightspace, options)
	if err != nil {
		return nil, err
	}
	return New(session), nil
}
