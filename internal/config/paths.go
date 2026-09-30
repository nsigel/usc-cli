// Package config resolves the shared USC CLI configuration paths.
package config

import (
	"os"
	"path/filepath"
)

// Directory returns the directory containing USC credentials and sessions.
// USC_CONFIG_DIR overrides the platform's standard user configuration path.
func Directory() (string, error) {
	root := os.Getenv("USC_CONFIG_DIR")
	if root == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(base, "usc")
	}
	return filepath.Abs(root)
}

// SessionPath returns the path to the shared USC browser-cookie session.
func SessionPath() (string, error) {
	root, err := Directory()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "session.json"), nil
}

// CredentialsPath returns the path to saved USC and Marshall credentials.
func CredentialsPath() (string, error) {
	root, err := Directory()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "credentials.json"), nil
}
