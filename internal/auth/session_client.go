package auth

import (
	"errors"
	"os"

	http "github.com/saucesteals/fhttp"
)

// ErrSessionNotFound means no saved browser session is available.
var ErrSessionNotFound = errors.New("no saved session")

// Session sends requests with the saved USC browser cookies. It intentionally
// does not reauthenticate: only auth commands may collect credentials.
type Session struct {
	client *http.Client
}

// OpenSession restores the saved browser session for use by a site client.
func OpenSession(sessionFile string) (*Session, error) {
	if _, err := os.Stat(sessionFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	authenticator, err := newAuthenticator()
	if err != nil {
		return nil, err
	}
	if err := loadSession(sessionFile, authenticator.jar); err != nil {
		return nil, err
	}
	return &Session{client: authenticator.client}, nil
}

// Do implements the small HTTP contract used by site clients.
func (s *Session) Do(request *http.Request) (*http.Response, error) {
	return s.client.Do(request)
}
