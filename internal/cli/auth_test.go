package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/nsigel/usc-cli/internal/auth"
	"github.com/nsigel/usc-cli/internal/config"
	"github.com/spf13/cobra"
)

func TestResolveCredentialsDoesNotInjectEnvironment(t *testing.T) {
	store := config.New(t.TempDir())
	if _, err := store.EnsureProfile("default", "saved-user"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCredentials("default", config.Credentials{
		Username: "saved-user", Password: "saved-password", BypassCode: "saved-code",
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("USC_USERNAME", "environment-user")
	t.Setenv("USC_PASSWORD", "environment-password")
	t.Setenv("USC_DUO_BYPASS", "environment-code")

	app := &App{Store: store}
	credentials, err := app.resolveCredentials("default", auth.Credentials{})
	if err != nil {
		t.Fatal(err)
	}
	want := (auth.Credentials{Username: "saved-user", Password: "saved-password", BypassCode: "saved-code"})
	if credentials != want {
		t.Fatalf("resolveCredentials() = %#v, want %#v", credentials, want)
	}
}

func TestLoginStillAcceptsEnvironmentCredentials(t *testing.T) {
	t.Setenv("USC_PROFILE", "")
	t.Setenv("USC_USERNAME", "environment-user")
	t.Setenv("USC_PASSWORD", "environment-password")
	t.Setenv("USC_DUO_BYPASS", "environment-code")

	var received auth.Credentials
	app := &App{
		Store: config.New(t.TempDir()),
		Out:   &bytes.Buffer{},
		Login: func(_ context.Context, _, _ string, credentials auth.Credentials) (auth.Result, error) {
			received = credentials
			return auth.Result{}, nil
		},
	}
	if err := app.runLogin(&cobra.Command{}, loginOptions{nonInteractive: true}); err != nil {
		t.Fatal(err)
	}
	want := (auth.Credentials{Username: "environment-user", Password: "environment-password", BypassCode: "environment-code"})
	if received != want {
		t.Fatalf("login credentials = %#v, want %#v", received, want)
	}
}

func TestBypassCommandDoesNotReadEnvironmentCode(t *testing.T) {
	store := config.New(t.TempDir())
	if _, err := store.EnsureProfile("default", "saved-user"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCredentials("default", config.Credentials{
		Username: "saved-user", Password: "saved-password", BypassCode: "old-code",
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("USC_PROFILE", "")
	t.Setenv("USC_DUO_BYPASS", "environment-code")

	prompted := false
	app := &App{Store: store, Out: &bytes.Buffer{}}
	app.prompt = func(_ string, _ bool) (string, error) {
		prompted = true
		return "prompted-code", nil
	}
	if err := app.bypassCommand().Execute(); err != nil {
		t.Fatal(err)
	}
	if !prompted {
		t.Fatal("bypass command used USC_DUO_BYPASS instead of prompting")
	}
	credentials, err := store.LoadCredentials("default")
	if err != nil {
		t.Fatal(err)
	}
	if credentials.BypassCode != "prompted-code" {
		t.Fatalf("saved bypass code = %q, want prompted-code", credentials.BypassCode)
	}
}
