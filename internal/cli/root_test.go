package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/nsigel/usc-cli/internal/auth"
	"github.com/nsigel/usc-cli/internal/config"
)

func TestLoginCreatesDefaultProfileWithoutRememberingSecrets(t *testing.T) {
	t.Setenv("USC_USERNAME", "tommy")
	t.Setenv("USC_PASSWORD", "secret")
	t.Setenv("USC_DUO_BYPASS", "123456789")
	store := config.New(t.TempDir())
	var received auth.Credentials
	var sessionPath string
	output := &bytes.Buffer{}
	app := &App{
		Store: store, Out: output, Err: &bytes.Buffer{},
		Login: func(_ context.Context, _ string, session string, credentials auth.Credentials) (auth.Result, error) {
			received, sessionPath = credentials, session
			return auth.Result{URL: auth.DefaultTarget, Status: 200}, nil
		},
	}
	command := app.Command()
	command.SetArgs([]string{"login", "--non-interactive"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if received.Username != "tommy" || received.Password != "secret" || received.BypassCode != "123456789" {
		t.Fatalf("login received %#v", received)
	}
	if sessionPath != store.SessionPath(config.DefaultProfile) {
		t.Fatalf("session path = %q", sessionPath)
	}
	profile, err := store.LoadProfile(config.DefaultProfile)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Username != "tommy" {
		t.Fatalf("profile username = %q", profile.Username)
	}
	if _, err := os.Stat(store.CredentialsPath(config.DefaultProfile)); !os.IsNotExist(err) {
		t.Fatalf("credentials file unexpectedly exists: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["authenticated"] != true || result["profile"] != config.DefaultProfile {
		t.Fatalf("output = %#v", result)
	}
}

func TestNamedLoginCanRememberCredentials(t *testing.T) {
	t.Setenv("USC_USERNAME", "student")
	t.Setenv("USC_PASSWORD", "secret")
	t.Setenv("USC_DUO_BYPASS", "987654321")
	store := config.New(t.TempDir())
	app := &App{
		Store: store, Out: &bytes.Buffer{}, Err: &bytes.Buffer{},
		Login: func(context.Context, string, string, auth.Credentials) (auth.Result, error) {
			return auth.Result{URL: auth.DefaultTarget, Status: 200}, nil
		},
	}
	command := app.Command()
	command.SetArgs([]string{"--profile", "grad-school", "login", "--remember", "--non-interactive"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	credentials, err := store.LoadCredentials("grad-school")
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Username != "student" || credentials.Password != "secret" || credentials.BypassCode != "987654321" {
		t.Fatalf("saved credentials = %#v", credentials)
	}
}

func TestProfileLifecycle(t *testing.T) {
	store := config.New(t.TempDir())
	for _, args := range [][]string{
		{"profile", "add", "personal", "--username", "tommy"},
		{"profile", "use", "personal"},
		{"profile", "show"},
	} {
		app := &App{Store: store, Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
		command := app.Command()
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("usc %v: %v", args, err)
		}
	}
	current, err := store.ResolveProfile("")
	if err != nil || current != "personal" {
		t.Fatalf("current profile = %q, %v", current, err)
	}
}
