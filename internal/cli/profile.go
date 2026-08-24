package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func (a *App) profileCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Manage isolated USC accounts and sessions"}
	cmd.AddCommand(
		a.profileListCommand(),
		a.profileAddCommand(),
		a.profileUseCommand(),
		a.profileShowCommand(),
		a.profileRemoveCommand(),
	)
	return cmd
}

func (a *App) profileListCommand() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List configured profiles", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			profiles, err := a.Store.ListProfiles()
			if err != nil {
				return err
			}
			current, err := a.Store.ResolveProfile("")
			if err != nil {
				return err
			}
			if a.format == "human" {
				for _, profile := range profiles {
					marker := " "
					if profile.Name == current {
						marker = "*"
					}
					if _, err := fmt.Fprintf(a.Out, "%s %-20s %s\n", marker, profile.Name, profile.Username); err != nil {
						return err
					}
				}
				return nil
			}
			items := make([]map[string]any, 0, len(profiles))
			for _, profile := range profiles {
				items = append(items, map[string]any{
					"name": profile.Name, "username": profile.Username, "current": profile.Name == current,
				})
			}
			return a.writeJSON(map[string]any{"profiles": items})
		},
	}
}

func (a *App) profileAddCommand() *cobra.Command {
	var username string
	var use bool
	cmd := &cobra.Command{
		Use: "add NAME", Short: "Create a profile without logging in", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			profile, err := a.Store.EnsureProfile(args[0], username)
			if err != nil {
				return err
			}
			if use {
				if err := a.Store.SetCurrent(profile.Name); err != nil {
					return err
				}
			}
			if a.format == "human" {
				_, err = fmt.Fprintf(a.Out, "Created profile %s.\n", profile.Name)
				return err
			}
			return a.writeJSON(map[string]any{"created": true, "name": profile.Name, "username": profile.Username, "current": use})
		},
	}
	cmd.Flags().StringVarP(&username, "username", "u", "", "USC NetID")
	cmd.Flags().BoolVar(&use, "use", false, "make this the active profile")
	return cmd
}

func (a *App) profileUseCommand() *cobra.Command {
	return &cobra.Command{
		Use: "use NAME", Short: "Select the default profile", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := a.Store.SetCurrent(args[0]); err != nil {
				return err
			}
			if a.format == "human" {
				_, err := fmt.Fprintf(a.Out, "Using profile %s.\n", args[0])
				return err
			}
			return a.writeJSON(map[string]any{"current_profile": args[0]})
		},
	}
}

func (a *App) profileShowCommand() *cobra.Command {
	return &cobra.Command{
		Use: "show [NAME]", Short: "Show non-secret profile details", Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			var err error
			if len(args) == 1 {
				name = args[0]
			} else {
				name, err = a.selectedProfile()
				if err != nil {
					return err
				}
			}
			profile, err := a.Store.LoadProfile(name)
			if err != nil {
				return err
			}
			current, err := a.Store.ResolveProfile("")
			if err != nil {
				return err
			}
			result := map[string]any{
				"name": profile.Name, "username": profile.Username, "current": profile.Name == current,
				"has_session":                fileExists(a.Store.SessionPath(name)),
				"has_remembered_credentials": fileExists(a.Store.CredentialsPath(name)),
				"profile_path":               a.Store.ProfilePath(name), "session_path": a.Store.SessionPath(name),
			}
			if a.format == "human" {
				_, err = fmt.Fprintf(a.Out, "Profile: %s\nUsername: %s\nSession: %t\nRemembered credentials: %t\n", profile.Name, profile.Username, result["has_session"], result["has_remembered_credentials"])
				return err
			}
			return a.writeJSON(result)
		},
	}
}

func (a *App) profileRemoveCommand() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use: "remove NAME", Aliases: []string{"delete"}, Short: "Remove a profile and its saved session", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if !force {
				return errors.New("profile removal deletes its session and remembered credentials; pass --force")
			}
			if _, err := a.Store.LoadProfile(args[0]); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("profile %q does not exist", args[0])
				}
				return err
			}
			if err := a.Store.RemoveProfile(args[0]); err != nil {
				return err
			}
			if a.format == "human" {
				_, err := fmt.Fprintf(a.Out, "Removed profile %s.\n", args[0])
				return err
			}
			return a.writeJSON(map[string]any{"removed": true, "name": args[0]})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "confirm removal of the profile directory")
	return cmd
}
