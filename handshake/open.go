package handshake

import (
	"fmt"

	"github.com/nsigel/usc-cli/internal/auth"
)

// Open restores the saved USC browser session and returns a Handshake client.
// sessionFile is normally the path from DefaultSessionFile (cookies only).
func Open(sessionFile string) (*Client, error) {
	session, err := auth.OpenSession(sessionFile)
	if err != nil {
		return nil, fmt.Errorf("open USC session: %w", err)
	}
	return New(session), nil
}
