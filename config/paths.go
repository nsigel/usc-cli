// Package config locates the usc command's configuration files.
package config

import (
	"os"
	"path/filepath"
)

// Directory returns the usc command's configuration directory: USC_CONFIG_DIR
// when set, otherwise "usc" under the platform's user configuration directory.
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

// CredentialsPath returns the file where usc auth login saves credentials.
func CredentialsPath() (string, error) {
	return path("credentials.json")
}

// SessionPath returns the usc command's own cookie session file.
func SessionPath() (string, error) {
	return path("session.json")
}

func path(name string) (string, error) {
	directory, err := Directory()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, name), nil
}
