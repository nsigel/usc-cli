package cli

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/webreg"
	"github.com/spf13/cobra"
)

func webregCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "webreg", Short: "Plan and submit USC Web Registration changes"}
	cmd.AddCommand(
		webregTermsCommand(), webregProgramsCommand(), webregCoursesCommand(),
		webregBinCommand(), webregCalendarCommand(),
		webregSectionCommand("add", "Add a section to myCourseBin", (*webreg.Client).Add),
		webregSectionCommand("schedule", "Schedule a course-bin section for registration at checkout", (*webreg.Client).Schedule),
		webregSectionCommand("unschedule", "Unschedule a section; a registered section is dropped at checkout", (*webreg.Client).Unschedule),
		webregSectionCommand("remove", "Remove an unscheduled, unregistered section from myCourseBin", (*webreg.Client).Remove),
		webregGradeOptionCommand(), webregCheckoutCommand(), webregSubmitCommand(),
	)
	return cmd
}

func addTermFlag(cmd *cobra.Command, term *string) {
	cmd.Flags().StringVar(term, "term", "", "five-digit term code from usc webreg terms, e.g. 20271")
	_ = cmd.MarkFlagRequired("term")
}

func openWebReg(ctx context.Context) (*webreg.Client, error) {
	client, err := webreg.Open(ctx)
	if errors.Is(err, auth.ErrCredentialsRequired) {
		return nil, withAction(errors.New("authentication required"), "usc auth login webreg")
	}
	return client, err
}

func sectionID(value string) (int, error) {
	id, err := strconv.Atoi(value)
	if err != nil || id < 1 {
		return 0, errors.New("SECTION_ID must be a positive section number, e.g. 14790")
	}
	return id, nil
}

func webregTermsCommand() *cobra.Command {
	return &cobra.Command{
		Use: "terms", Short: "List registration terms", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openWebReg(cmd.Context())
			if err != nil {
				return err
			}
			terms, err := client.Terms(cmd.Context())
			if err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{"terms": terms})
		},
	}
}

func webregProgramsCommand() *cobra.Command {
	var term string
	cmd := &cobra.Command{
		Use: "programs", Short: "List a term's schools and program codes", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openWebReg(cmd.Context())
			if err != nil {
				return err
			}
			schools, err := client.Programs(cmd.Context(), term)
			if err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{"term": term, "schools": schools})
		},
	}
	addTermFlag(cmd, &term)
	return cmd
}

func webregCoursesCommand() *cobra.Command {
	var term string
	var query webreg.CourseQuery
	cmd := &cobra.Command{
		Use: "courses", Short: "List a program's courses or one course with sections", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openWebReg(cmd.Context())
			if err != nil {
				return err
			}
			courses, err := client.Courses(cmd.Context(), term, query)
			if err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{"term": term, "courses": courses, "count": len(courses)})
		},
	}
	addTermFlag(cmd, &term)
	cmd.Flags().StringVar(&query.Program, "program", "", "program code, e.g. BUAD")
	cmd.Flags().StringVar(&query.Course, "course", "", "course code, e.g. BUAD-302")
	cmd.MarkFlagsOneRequired("program", "course")
	cmd.MarkFlagsMutuallyExclusive("program", "course")
	return cmd
}

func webregBinCommand() *cobra.Command {
	var term string
	cmd := &cobra.Command{
		Use: "bin", Aliases: []string{"coursebin"}, Short: "Show myCourseBin with schedule and registration state", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openWebReg(cmd.Context())
			if err != nil {
				return err
			}
			bin, err := client.CourseBin(cmd.Context(), term)
			if err != nil {
				return err
			}
			return writeJSON(cmd, bin)
		},
	}
	addTermFlag(cmd, &term)
	return cmd
}

func webregCalendarCommand() *cobra.Command {
	var term string
	cmd := &cobra.Command{
		Use: "calendar", Short: "Show weekly meetings with scheduled, registered, and conflict status", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openWebReg(cmd.Context())
			if err != nil {
				return err
			}
			events, err := client.Calendar(cmd.Context(), term)
			if err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{"term": term, "events": events})
		},
	}
	addTermFlag(cmd, &term)
	return cmd
}

func webregSectionCommand(use, short string, action func(*webreg.Client, context.Context, string, int) error) *cobra.Command {
	var term string
	cmd := &cobra.Command{
		Use: use + " SECTION_ID", Short: short, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := sectionID(args[0])
			if err != nil {
				return err
			}
			client, err := openWebReg(cmd.Context())
			if err != nil {
				return err
			}
			if err := action(client, cmd.Context(), term, id); err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{"term": term, "section": id, "action": use, "ok": true})
		},
	}
	addTermFlag(cmd, &term)
	return cmd
}

func webregGradeOptionCommand() *cobra.Command {
	var term string
	cmd := &cobra.Command{
		Use: "grade-option SECTION_ID CODE", Short: "Change a course-bin section's grade option (codes from usc webreg bin)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := sectionID(args[0])
			if err != nil {
				return err
			}
			client, err := openWebReg(cmd.Context())
			if err != nil {
				return err
			}
			response, err := client.SetGradeOption(cmd.Context(), term, id, args[1])
			if err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{"term": term, "section": id, "grade_option": strings.ToUpper(args[1]), "response": response})
		},
	}
	addTermFlag(cmd, &term)
	return cmd
}

func webregCheckoutCommand() *cobra.Command {
	var term string
	cmd := &cobra.Command{
		Use: "checkout", Short: "Preview pending registrations and drops without submitting", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openWebReg(cmd.Context())
			if err != nil {
				return err
			}
			checkout, err := client.Checkout(cmd.Context(), term)
			if err != nil {
				return err
			}
			return writeJSON(cmd, checkout)
		},
	}
	addTermFlag(cmd, &term)
	return cmd
}

func webregSubmitCommand() *cobra.Command {
	var term string
	var confirm bool
	cmd := &cobra.Command{
		Use: "submit", Short: "Submit checkout: register and drop the sections checkout lists", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !confirm {
				return errors.New("submit changes your enrollment; review usc webreg checkout, then pass --confirm")
			}
			client, err := openWebReg(cmd.Context())
			if err != nil {
				return err
			}
			result, err := client.Submit(cmd.Context(), term)
			if err != nil {
				return err
			}
			return writeJSON(cmd, result)
		},
	}
	addTermFlag(cmd, &term)
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm the enrollment change shown by usc webreg checkout")
	return cmd
}
