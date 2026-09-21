package handshake

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultSessionFile is the cookies-only USC session path used by the CLI.
// Override with USC_CONFIG_DIR (directory) or USC_SESSION (exact file path).
func DefaultSessionFile() (string, error) {
	if override := os.Getenv("USC_SESSION"); override != "" {
		return override, nil
	}
	root := os.Getenv("USC_CONFIG_DIR")
	if root == "" {
		config, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("find config directory: %w", err)
		}
		root = filepath.Join(config, "usc")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve config directory: %w", err)
	}
	return filepath.Join(absolute, "session.json"), nil
}
