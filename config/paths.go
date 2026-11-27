package config

import (
	"os"
	"path/filepath"
)

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

func CredentialsPath() (string, error) {
	return path("credentials.json")
}

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
