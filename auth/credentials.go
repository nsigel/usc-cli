package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type savedCredentials struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	BypassCode string `json:"bypass_code"`
}

// LoadCredentials reads a credentials file written by SaveCredentials. A
// missing file yields empty credentials.
func LoadCredentials(file string) (Credentials, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, nil
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("read saved credentials: %w", err)
	}
	var saved savedCredentials
	if err := json.Unmarshal(data, &saved); err != nil {
		return Credentials{}, fmt.Errorf("read saved credentials: %w", err)
	}
	return Credentials(saved), nil
}

// SaveCredentials writes a credentials file readable only by its owner.
func SaveCredentials(file string, credentials Credentials) error {
	data, err := json.MarshalIndent(savedCredentials(credentials), "", "  ")
	if err != nil {
		return err
	}
	if err := writePrivateFile(file, append(data, '\n')); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	return nil
}

func forgetRejected(file string, err error, submitted Credentials) {
	password, bypass := errors.Is(err, ErrPasswordRejected), errors.Is(err, ErrBypassRejected)
	if file == "" || !password && !bypass {
		return
	}
	saved, loadErr := LoadCredentials(file)
	if loadErr != nil {
		return
	}
	switch {
	case password && saved.Password != "" && saved.Password == submitted.Password:
		saved.Password = ""
	case bypass && saved.BypassCode != "" && saved.BypassCode == submitted.BypassCode:
		saved.BypassCode = ""
	default:
		return
	}
	_ = SaveCredentials(file, saved)
}

func writePrivateFile(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".usc-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}
