package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProfilePrecedence(t *testing.T) {
	store := New(t.TempDir())
	if profile, err := store.ResolveProfile(""); err != nil || profile != DefaultProfile {
		t.Fatalf("ResolveProfile without config = %q, %v", profile, err)
	}
	if _, err := store.EnsureProfile("saved", "user"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCurrent("saved"); err != nil {
		t.Fatal(err)
	}
	if profile, err := store.ResolveProfile(""); err != nil || profile != "saved" {
		t.Fatalf("ResolveProfile with config = %q, %v", profile, err)
	}
	t.Setenv("USC_PROFILE", "environment")
	if profile, err := store.ResolveProfile(""); err != nil || profile != "environment" {
		t.Fatalf("ResolveProfile with environment = %q, %v", profile, err)
	}
	if profile, err := store.ResolveProfile("flag"); err != nil || profile != "flag" {
		t.Fatalf("ResolveProfile with flag = %q, %v", profile, err)
	}
}

func TestProfileFilesArePrivate(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.EnsureProfile("default", "tommy"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCredentials("default", Credentials{
		Username: "tommy", Password: "secret", BypassCode: "123456789",
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{store.ProfilePath("default"), store.CredentialsPath("default")} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("%s mode = %o, want 600", filepath.Base(name), got)
		}
	}
}

func TestValidateProfileNameRejectsPaths(t *testing.T) {
	for _, name := range []string{"", ".", "../other", "two words", "/absolute"} {
		if err := ValidateProfileName(name); err == nil {
			t.Errorf("ValidateProfileName(%q) succeeded", name)
		}
	}
}

func TestRemovingCurrentProfileResetsSelection(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.EnsureProfile("school", "tommy"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCurrent("school"); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveProfile("school"); err != nil {
		t.Fatal(err)
	}
	if profile, err := store.ResolveProfile(""); err != nil || profile != DefaultProfile {
		t.Fatalf("ResolveProfile after removal = %q, %v", profile, err)
	}
}
