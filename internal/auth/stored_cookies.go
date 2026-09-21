package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	http "github.com/saucesteals/fhttp"
)

// StoredCookie is a session cookie plus the URL that set it.
// Callers must never log Cookie.Value.
type StoredCookie struct {
	SetBy  string
	Cookie http.Cookie
}

// LoadStoredCookies reads cookies from the saved session file without creating
// an HTTP client. Missing files return ErrSessionNotFound.
func LoadStoredCookies(sessionFile string) ([]StoredCookie, error) {
	data, err := os.ReadFile(sessionFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	var session savedSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("read session: %w", err)
	}
	if session.Version != sessionVersion {
		return nil, fmt.Errorf("unsupported session version %d", session.Version)
	}
	out := make([]StoredCookie, 0, len(session.Cookies))
	for _, entry := range session.Cookies {
		out = append(out, StoredCookie{
			SetBy:  entry.SetBy,
			Cookie: entry.Cookie,
		})
	}
	return out, nil
}
