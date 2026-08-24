package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

const (
	DefaultProfile = "default"
	fileVersion    = 1
)

var profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type Config struct {
	Version        int    `json:"version"`
	CurrentProfile string `json:"current_profile"`
}

type Profile struct {
	Version   int       `json:"version"`
	Name      string    `json:"name"`
	Username  string    `json:"username,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Credentials struct {
	Version    int    `json:"version"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	BypassCode string `json:"bypass_code"`
}

type Store struct {
	root string
}

func DefaultRoot() (string, error) {
	if configured := os.Getenv("USC_CONFIG_DIR"); configured != "" {
		return filepath.Abs(configured)
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(root, "usc-cli"), nil
}

func New(root string) *Store {
	return &Store{root: root}
}

func (s *Store) Root() string {
	return s.root
}

func ValidateProfileName(name string) error {
	if !profileNamePattern.MatchString(name) {
		return errors.New("profile name must be 1-64 letters, numbers, dots, dashes, or underscores and start with a letter or number")
	}
	return nil
}

func (s *Store) ResolveProfile(explicit string) (string, error) {
	if explicit != "" {
		if err := ValidateProfileName(explicit); err != nil {
			return "", err
		}
		return explicit, nil
	}
	if environment := os.Getenv("USC_PROFILE"); environment != "" {
		if err := ValidateProfileName(environment); err != nil {
			return "", fmt.Errorf("USC_PROFILE: %w", err)
		}
		return environment, nil
	}
	cfg, err := s.LoadConfig()
	if errors.Is(err, os.ErrNotExist) {
		return DefaultProfile, nil
	}
	if err != nil {
		return "", err
	}
	if cfg.CurrentProfile == "" {
		return DefaultProfile, nil
	}
	return cfg.CurrentProfile, ValidateProfileName(cfg.CurrentProfile)
}

func (s *Store) LoadConfig() (Config, error) {
	var cfg Config
	if err := readJSON(filepath.Join(s.root, "config.json"), &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Version != fileVersion {
		return Config{}, fmt.Errorf("unsupported config version %d", cfg.Version)
	}
	return cfg, nil
}

func (s *Store) SetCurrent(name string) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	if _, err := s.LoadProfile(name); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("profile %q does not exist", name)
		}
		return err
	}
	return writeJSON(filepath.Join(s.root, "config.json"), Config{
		Version: fileVersion, CurrentProfile: name,
	})
}

func (s *Store) EnsureProfile(name, username string) (Profile, error) {
	if err := ValidateProfileName(name); err != nil {
		return Profile{}, err
	}
	now := time.Now().UTC()
	profile, err := s.LoadProfile(name)
	if errors.Is(err, os.ErrNotExist) {
		profile = Profile{Version: fileVersion, Name: name, CreatedAt: now}
	} else if err != nil {
		return Profile{}, err
	}
	if username != "" {
		profile.Username = username
	}
	profile.UpdatedAt = now
	if err := writeJSON(s.ProfilePath(name), profile); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func (s *Store) LoadProfile(name string) (Profile, error) {
	if err := ValidateProfileName(name); err != nil {
		return Profile{}, err
	}
	var profile Profile
	if err := readJSON(s.ProfilePath(name), &profile); err != nil {
		return Profile{}, err
	}
	if profile.Version != fileVersion {
		return Profile{}, fmt.Errorf("unsupported profile version %d", profile.Version)
	}
	if profile.Name != name {
		return Profile{}, fmt.Errorf("profile file contains name %q, expected %q", profile.Name, name)
	}
	return profile, nil
}

func (s *Store) ListProfiles() ([]Profile, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "profiles"))
	if errors.Is(err, os.ErrNotExist) {
		return []Profile{}, nil
	}
	if err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || ValidateProfileName(entry.Name()) != nil {
			continue
		}
		profile, err := s.LoadProfile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("load profile %q: %w", entry.Name(), err)
		}
		profiles = append(profiles, profile)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles, nil
}

func (s *Store) SaveCredentials(name string, creds Credentials) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	creds.Version = fileVersion
	return writeJSON(s.CredentialsPath(name), creds)
}

func (s *Store) LoadCredentials(name string) (Credentials, error) {
	if err := ValidateProfileName(name); err != nil {
		return Credentials{}, err
	}
	var creds Credentials
	if err := readJSON(s.CredentialsPath(name), &creds); err != nil {
		return Credentials{}, err
	}
	if creds.Version != fileVersion {
		return Credentials{}, fmt.Errorf("unsupported credentials version %d", creds.Version)
	}
	return creds, nil
}

func (s *Store) ForgetCredentials(name string) error {
	return removeFile(s.CredentialsPath(name))
}

func (s *Store) ClearSession(name string) error {
	return removeFile(s.SessionPath(name))
}

func (s *Store) RemoveProfile(name string) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	if err := os.RemoveAll(s.ProfileDir(name)); err != nil {
		return err
	}
	cfg, err := s.LoadConfig()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if cfg.CurrentProfile == name {
		return removeFile(filepath.Join(s.root, "config.json"))
	}
	return nil
}

func (s *Store) ProfileDir(name string) string {
	return filepath.Join(s.root, "profiles", name)
}

func (s *Store) ProfilePath(name string) string {
	return filepath.Join(s.ProfileDir(name), "profile.json")
}

func (s *Store) CredentialsPath(name string) string {
	return filepath.Join(s.ProfileDir(name), "credentials.json")
}

func (s *Store) SessionPath(name string) string {
	return filepath.Join(s.ProfileDir(name), "session.json")
}

func readJSON(name string, target any) error {
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	return nil
}

func writeJSON(name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".usc-cli-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
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
	return os.Rename(temporaryName, name)
}

func removeFile(name string) error {
	err := os.Remove(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
