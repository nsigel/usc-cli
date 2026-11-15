package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nsigel/usc-cli/libcal"
	"github.com/spf13/cobra"
)

func libcalCommand() *cobra.Command {
	return group("libcal", "Check and reserve Leavey Library spaces",
		libcalCategoriesCommand(), libcalSpacesCommand(), libcalSpaceCommand(),
		libcalScheduleCommand(), libcalBookCommand(), libcalReleaseCommand(),
	)
}

func libcalCategoriesCommand() *cobra.Command {
	return &cobra.Command{
		Use: "categories", Short: "List Leavey room categories", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeJSON(cmd, libcal.Categories())
		},
	}
}

func libcalSpacesCommand() *cobra.Command {
	return &cobra.Command{
		Use: "spaces [CATEGORY]", Short: "List Leavey rooms and pods: rooms, pods, all, lvl1, lvl2, or lvl3", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			category := libcal.CategoryAll
			if len(args) == 1 {
				category = libcal.Category(strings.ToLower(args[0]))
			}
			spaces, err := libcal.NewPublic().Spaces(cmd.Context(), category)
			if err != nil {
				return err
			}
			return writeJSON(cmd, spaces)
		},
	}
}

func libcalSpaceCommand() *cobra.Command {
	return &cobra.Command{
		Use: "space SPACE_ID", Short: "Show one Leavey room or pod", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := positiveID(args[0], "SPACE_ID")
			if err != nil {
				return err
			}
			space, err := libcal.NewPublic().Space(cmd.Context(), id)
			if err != nil {
				return err
			}
			return writeJSON(cmd, space)
		},
	}
}

func libcalScheduleCommand() *cobra.Command {
	var date, endDate, category string
	var options libcal.ScheduleOptions
	cmd := &cobra.Command{
		Use: "schedule", Short: "Show each space's available and unavailable intervals", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var err error
			if options.Date, err = libcal.ParseDate(date); err != nil {
				return err
			}
			if endDate != "" {
				if options.EndDate, err = libcal.ParseDate(endDate); err != nil {
					return err
				}
			}
			options.Category = libcal.Category(strings.ToLower(category))
			schedule, err := libcal.NewPublic().Schedule(cmd.Context(), options)
			if err != nil {
				return err
			}
			return writeJSON(cmd, schedule)
		},
	}
	cmd.Flags().StringVar(&date, "date", "today", "start date in Los Angeles time: today, tomorrow, or YYYY-MM-DD")
	cmd.Flags().StringVar(&endDate, "end-date", "", "exclusive end date; defaults to the day after --date")
	cmd.Flags().StringVar(&category, "category", "all", "rooms, pods, all, lvl1, lvl2, or lvl3")
	cmd.Flags().IntVar(&options.MinCapacity, "min-capacity", 0, "minimum space capacity")
	cmd.Flags().IntVar(&options.MaxCapacity, "max-capacity", 0, "maximum space capacity")
	return cmd
}

func libcalBookCommand() *cobra.Command {
	var spaceID int
	var start, end string
	var fields []string
	var details libcal.ReservationDetails
	cmd := &cobra.Command{
		Use: "book", Short: "Book an explicitly selected space, start, and end", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			selection := libcal.Selection{SpaceID: spaceID}
			var err error
			if selection.Start, err = parseTimestamp("--start", start); err != nil {
				return err
			}
			if selection.End, err = parseTimestamp("--end", end); err != nil {
				return err
			}
			if err := selection.Validate(); err != nil {
				return err
			}
			if !details.AcceptTerms {
				return errors.New("booking requires --accept-terms to accept the LibCal reservation terms")
			}
			if details.Fields, err = parseFields(fields); err != nil {
				return err
			}
			client, err := openLibCal()
			if err != nil {
				return err
			}
			reservation, err := client.Book(cmd.Context(), selection, details)
			if err != nil {
				return err
			}
			return writeJSON(cmd, reservation)
		},
	}
	cmd.Flags().IntVar(&spaceID, "space", 0, "space ID from the schedule")
	cmd.Flags().StringVar(&start, "start", "", "start of an available interval, RFC3339 with UTC offset")
	cmd.Flags().StringVar(&end, "end", "", "end of a later contiguous interval, at most 2 hours after --start")
	cmd.Flags().StringArrayVar(&fields, "field", nil, "checkout form answer as FIELD=VALUE (repeatable)")
	cmd.Flags().BoolVar(&details.AcceptTerms, "accept-terms", false, "accept the LibCal reservation terms")
	for _, name := range []string{"space", "start", "end"} {
		_ = cmd.MarkFlagRequired(name)
	}
	return cmd
}

func parseTimestamp(flag, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be an RFC3339 timestamp with a UTC offset, such as 2026-10-02T23:00:00-07:00", flag)
	}
	return parsed, nil
}

func parseFields(items []string) (map[string]string, error) {
	fields := make(map[string]string, len(items))
	for _, item := range items {
		name, value, ok := strings.Cut(item, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid --field %q (use FIELD=VALUE)", item)
		}
		fields[name] = value
	}
	return fields, nil
}

func libcalReleaseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "release",
		Short: "Release an unfinished checkout; after an interrupted booking, check the confirmation email first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := openLibCal()
			if err != nil {
				return err
			}
			if err := client.Release(cmd.Context()); err != nil {
				return err
			}
			return writeJSON(cmd, map[string]bool{"released": true})
		},
	}
}
