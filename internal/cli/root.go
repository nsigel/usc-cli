package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nsigel/usc-cli/internal/auth"
	"github.com/nsigel/usc-cli/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type LoginFunc func(context.Context, string, string, auth.Credentials) (auth.Result, error)

type App struct {
	Store  *config.Store
	In     io.Reader
	Out    io.Writer
	Err    io.Writer
	Login  LoginFunc
	prompt func(string, bool) (string, error)
	reader *bufio.Reader

	profile string
	format  string
}

func New() (*cobra.Command, error) {
	root, err := config.DefaultRoot()
	if err != nil {
		return nil, err
	}
	app := &App{
		Store: config.New(root), In: os.Stdin, Out: os.Stdout, Err: os.Stderr,
		Login: auth.Login,
	}
	return app.Command(), nil
}

func (a *App) Command() *cobra.Command {
	if a.In == nil {
		a.In = os.Stdin
	}
	if a.Out == nil {
		a.Out = os.Stdout
	}
	if a.Err == nil {
		a.Err = os.Stderr
	}
	if a.Login == nil {
		a.Login = auth.Login
	}
	a.reader = bufio.NewReader(a.In)

	cmd := &cobra.Command{
		Use:           "usc",
		Short:         "USC services from your terminal",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.SetIn(a.In)
	cmd.SetOut(a.Out)
	cmd.SetErr(a.Err)
	cmd.PersistentFlags().StringVar(&a.profile, "profile", "", "profile to use (default: USC_PROFILE, active profile, or default)")
	cmd.PersistentFlags().StringVar(&a.format, "format", "json", "output format: json or human")
	cmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if a.format != "json" && a.format != "human" {
			return fmt.Errorf("unsupported format %q (use json or human)", a.format)
		}
		return nil
	}
	cmd.AddCommand(a.loginCommand(), a.statusCommand(), a.logoutCommand(), a.bypassCommand(), a.profileCommand())
	return cmd
}

func (a *App) selectedProfile() (string, error) {
	return a.Store.ResolveProfile(a.profile)
}

func (a *App) writeJSON(value any) error {
	encoder := json.NewEncoder(a.Out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func (a *App) ask(label string, secret bool) (string, error) {
	if a.prompt != nil {
		return a.prompt(label, secret)
	}
	if _, err := fmt.Fprint(a.Err, label); err != nil {
		return "", err
	}
	if secret {
		if input, ok := a.In.(*os.File); ok && term.IsTerminal(int(input.Fd())) {
			value, err := term.ReadPassword(int(input.Fd()))
			fmt.Fprintln(a.Err)
			return strings.TrimSpace(string(value)), err
		}
	}
	value, err := a.reader.ReadString('\n')
	if err != nil && len(value) == 0 {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func fileExists(name string) bool {
	info, err := os.Stat(name)
	return err == nil && !info.IsDir()
}
