package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nsigel/usc-cli/libcal"
	"github.com/spf13/cobra"
)

func libcalCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "libcal", Short: "Check and reserve Leavey Library spaces"}
	cmd.AddCommand(libcalCategoriesCommand(), libcalSpacesCommand(), libcalRoomCommand(), libcalScheduleCommand(), libcalBookCommand(), libcalReservationsCommand(), libcalReleaseCommand())
	return cmd
}

// Selection flags deliberately use complete timestamps, so exact selections
// survive cross-midnight bookings and daylight-saving transitions.
type libcalSelectionFlags struct {
	spaceID int
	start   string
	end     string
}

func (f *libcalSelectionFlags) add(cmd *cobra.Command) {
	cmd.Flags().IntVar(&f.spaceID, "space", 0, "exact LibCal space ID")
	cmd.Flags().StringVar(&f.start, "start", "", "exact start timestamp from schedule, RFC3339 with UTC offset")
	cmd.Flags().StringVar(&f.end, "end", "", "chosen end timestamp within available schedule intervals, RFC3339 with UTC offset (maximum 2 hours)")
}

func (f libcalSelectionFlags) selection() (libcal.Selection, error) {
	if f.spaceID < 1 || f.start == "" || f.end == "" {
		return libcal.Selection{}, errors.New("--space, --start, and --end are required")
	}
	start, err := time.Parse(time.RFC3339, f.start)
	if err != nil {
		return libcal.Selection{}, errors.New("--start must be an RFC3339 timestamp with a UTC offset")
	}
	selection := libcal.Selection{SpaceID: f.spaceID, Start: start}
	if f.end != "" {
		selection.End, err = time.Parse(time.RFC3339, f.end)
		if err != nil {
			return libcal.Selection{}, errors.New("--end must be an RFC3339 timestamp with a UTC offset")
		}
		if !selection.End.After(selection.Start) {
			return libcal.Selection{}, errors.New("--end must be after --start")
		}
	}
	if err := selection.Validate(); err != nil {
		return libcal.Selection{}, err
	}
	return selection, nil
}

func libcalBookCommand() *cobra.Command {
	var flags libcalSelectionFlags
	var name, email string
	var acceptTerms bool
	var customFields []string
	cmd := &cobra.Command{
		Use: "book", Short: "Book an explicitly selected space, start, and end", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			selection, err := flags.selection()
			if err != nil {
				return err
			}
			if !acceptTerms {
				return errors.New("booking requires --accept-terms to accept the LibCal reservation terms")
			}
			fields, err := parseLibCalFields(customFields)
			if err != nil {
				return err
			}
			client, err := libcal.Open(cmd.Context())
			if err != nil {
				return err
			}
			reservation, err := client.Book(cmd.Context(), selection, libcal.ReservationDetails{
				Name: name, Email: email, Fields: fields, AcceptTerms: acceptTerms,
			})
			if err != nil {
				return err
			}
			return writeJSON(cmd, reservation)
		},
	}
	flags.add(cmd)
	cmd.Flags().StringVar(&name, "name", "", "name to use if LibCal requests it")
	cmd.Flags().StringVar(&email, "email", "", "email to use if LibCal requests it")
	cmd.Flags().StringArrayVar(&customFields, "field", nil, "explicit checkout answer as FIELD=VALUE (repeatable)")
	cmd.Flags().BoolVar(&acceptTerms, "accept-terms", false, "accept the LibCal reservation terms")
	return cmd
}

func libcalReservationsCommand() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "reservations",
		Short: "List reservations confirmed by this CLI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reservations, err := libcal.Reservations(all)
			if err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{
				"source":       "local_cli_history",
				"complete":     false,
				"reservations": reservations,
				"count":        len(reservations),
			})
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include past reservations")
	return cmd
}

func parseLibCalFields(items []string) (map[string]string, error) {
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

func libcalCategoriesCommand() *cobra.Command {
	return &cobra.Command{
		Use: "categories", Short: "List Leavey room categories", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeJSON(cmd, map[string]any{"location": "Leavey Library", "categories": libcal.Categories()})
		},
	}
}

func libcalSpacesCommand() *cobra.Command {
	return &cobra.Command{
		Use: "spaces [CATEGORY]", Short: "List Leavey rooms and pods", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selector := libcal.CategoryAll
			if len(args) == 1 {
				selector = libcal.Category(strings.ToLower(args[0]))
			}
			spaces, err := libcal.NewPublic().Spaces(cmd.Context(), selector)
			if err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{"location": "Leavey Library", "spaces": spaces, "count": len(spaces)})
		},
	}
}

func libcalRoomCommand() *cobra.Command {
	return &cobra.Command{
		Use: "room SPACE_ID", Short: "Show a Leavey room's details", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := positiveID(args[0], "SPACE_ID")
			if err != nil {
				return err
			}
			spaces, err := libcal.NewPublic().Spaces(cmd.Context(), libcal.CategoryAll)
			if err != nil {
				return err
			}
			for _, space := range spaces {
				if space.ID == id {
					return writeJSON(cmd, space)
				}
			}
			return fmt.Errorf("Leavey space %d was not found", id)
		},
	}
}

func libcalScheduleCommand() *cobra.Command {
	var dateText, endDateText, categoryText, capacityText string
	var minCapacity, maxCapacity int
	cmd := &cobra.Command{
		Use: "schedule", Aliases: []string{"availability"},
		Short: "Read the server's schedule intervals without choosing a duration", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			date, err := parseLibCalDate(dateText)
			if err != nil {
				return err
			}
			options := libcal.ScheduleOptions{
				Date: date, Category: libcal.Category(strings.ToLower(categoryText)),
				MinCapacity: minCapacity, MaxCapacity: maxCapacity,
			}
			if endDateText != "" {
				options.EndDate, err = parseLibCalDate(endDateText)
				if err != nil {
					return err
				}
			}
			if capacityText != "" {
				options.MinCapacity, options.MaxCapacity, err = parseCapacityRange(capacityText)
				if err != nil {
					return err
				}
			}
			schedule, err := libcal.NewPublic().Schedule(cmd.Context(), options)
			if err != nil {
				return err
			}
			return writeJSON(cmd, schedule)
		},
	}
	cmd.Flags().StringVar(&dateText, "date", "today", "start date in Los Angeles time: today, tomorrow, or YYYY-MM-DD")
	cmd.Flags().StringVar(&endDateText, "end-date", "", "exclusive end date; defaults to the following day")
	cmd.Flags().StringVar(&categoryText, "category", "all", "rooms, pods, all, lvl1, lvl2, or lvl3")
	cmd.Flags().StringVar(&capacityText, "capacity", "", "inclusive capacity range, e.g. 6-12")
	cmd.Flags().IntVar(&minCapacity, "min-capacity", 0, "minimum space capacity (0 means no minimum)")
	cmd.Flags().IntVar(&maxCapacity, "max-capacity", 0, "maximum space capacity (0 means no maximum)")
	cmd.MarkFlagsMutuallyExclusive("capacity", "min-capacity")
	cmd.MarkFlagsMutuallyExclusive("capacity", "max-capacity")
	return cmd
}

func parseLibCalDate(value string) (time.Time, error) {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return time.Time{}, fmt.Errorf("load USC time zone: %w", err)
	}
	today := time.Now().In(location)
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "today":
		return today, nil
	case "tomorrow":
		return today.AddDate(0, 0, 1), nil
	default:
		return libcal.DateInLosAngeles(value)
	}
}

func parseCapacityRange(value string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(value), "-")
	if len(parts) != 2 {
		return 0, 0, errors.New("capacity must be a positive inclusive range, e.g. 6-12")
	}
	minimum, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, errors.New("capacity must be a positive inclusive range, e.g. 6-12")
	}
	maximum, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, errors.New("capacity must be a positive inclusive range, e.g. 6-12")
	}
	if minimum < 1 || maximum < minimum {
		return 0, 0, errors.New("capacity must be a positive inclusive range, e.g. 6-12")
	}
	return minimum, maximum, nil
}

// libcalReleaseCommand only releases temporary checkout state, never a reservation.
func libcalReleaseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "release",
		Short: "Release an unfinished CLI checkout (check confirmation email first if submission was interrupted)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := libcal.Open(cmd.Context())
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
