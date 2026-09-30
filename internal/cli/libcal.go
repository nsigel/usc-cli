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
	cmd.AddCommand(libcalCategoriesCommand(), libcalSpacesCommand(), libcalRoomCommand(), libcalAvailabilityCommand(), libcalBookCommand(), libcalReservationsCommand(), libcalReleaseCommand())
	return cmd
}

func libcalBookCommand() *cobra.Command {
	var dateText, afterText, beforeText, categoryText, capacityText, name, email, startText string
	var durationMinutes, spaceID int
	var includePods, acceptTerms bool
	var customFields []string
	cmd := &cobra.Command{
		Use:   "book",
		Short: "Book a matching Leavey study room",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if (spaceID > 0) != (startText != "") {
				return errors.New("--space and --start must be used together")
			}
			date, err := parseLibCalDate(dateText)
			if err != nil {
				return err
			}
			options := libcal.AvailabilityOptions{
				Date:        date,
				Duration:    time.Duration(durationMinutes) * time.Minute,
				Category:    libcal.Category(strings.ToLower(categoryText)),
				IncludePods: includePods,
			}
			if afterText != "" {
				options.After, err = libcal.TimeOfDay(afterText)
				if err != nil {
					return err
				}
			}
			if beforeText != "" {
				options.Before, err = libcal.TimeOfDay(beforeText)
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
			fields, err := parseLibCalFields(customFields)
			if err != nil {
				return err
			}
			details := libcal.ReservationDetails{
				Name: name, Email: email, Fields: fields, AcceptTerms: acceptTerms,
			}
			var selected *libcal.AvailableSlot
			if spaceID > 0 || startText != "" {
				start, err := libcal.TimeOfDay(startText)
				if err != nil {
					return fmt.Errorf("invalid --start: %w", err)
				}
				slots, err := libcal.NewPublic().Availability(cmd.Context(), options)
				if err != nil {
					return err
				}
				slot, ok := selectLibCalSlot(slots, spaceID, start)
				if !ok {
					return &libcal.Error{Code: "libcal_selected_slot_unavailable", Message: "the selected LibCal space and start time are no longer available; refresh availability"}
				}
				selected = &slot
			}
			client, err := libcal.Open(cmd.Context())
			if err != nil {
				return err
			}
			var reservation libcal.Reservation
			if selected != nil {
				reservation, err = client.Book(cmd.Context(), *selected, details)
			} else {
				reservation, err = client.BookEarliest(cmd.Context(), options, details)
			}
			if err != nil {
				return err
			}
			return writeJSON(cmd, reservation)
		},
	}
	cmd.Flags().StringVar(&dateText, "date", "today", "date in Los Angeles time: today, tomorrow, or YYYY-MM-DD")
	cmd.Flags().StringVar(&afterText, "after", "", "earliest start time, HH:MM in 24-hour format")
	cmd.Flags().StringVar(&beforeText, "before", "", "latest start time, HH:MM in 24-hour format")
	cmd.Flags().IntVar(&durationMinutes, "duration", 60, "reservation length in minutes (30-120, in 30-minute increments)")
	cmd.Flags().StringVar(&categoryText, "category", "rooms", "rooms, pods, all, lvl1, lvl2, or lvl3")
	cmd.Flags().BoolVar(&includePods, "include-pods", false, "include one-person study pods with group rooms")
	cmd.Flags().StringVar(&capacityText, "capacity", "", "capacity band: 1-4, 5-8, or 9-12 people")
	cmd.Flags().IntVar(&spaceID, "space", 0, "exact space ID selected from availability (requires --start)")
	cmd.Flags().StringVar(&startText, "start", "", "exact start time selected from availability, HH:MM (requires --space)")
	cmd.Flags().StringVar(&name, "name", "", "name to use if LibCal requests it")
	cmd.Flags().StringVar(&email, "email", "", "USC email to use if LibCal requests it")
	cmd.Flags().StringArrayVar(&customFields, "field", nil, "additional LibCal form value as FIELD=VALUE (repeatable)")
	cmd.Flags().BoolVar(&acceptTerms, "accept-terms", false, "confirm acceptance of the displayed LibCal reservation terms")
	return cmd
}

func selectLibCalSlot(slots []libcal.AvailableSlot, spaceID int, start time.Duration) (libcal.AvailableSlot, bool) {
	for _, slot := range slots {
		minute := time.Duration(slot.Start.Hour())*time.Hour + time.Duration(slot.Start.Minute())*time.Minute
		if slot.Space.ID == spaceID && minute == start {
			return slot, true
		}
	}
	return libcal.AvailableSlot{}, false
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

func libcalAvailabilityCommand() *cobra.Command {
	var dateText, afterText, beforeText, categoryText, capacityText string
	var durationMinutes int
	var includePods bool
	cmd := &cobra.Command{
		Use: "availability", Short: "Find available Leavey study rooms", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			date, err := parseLibCalDate(dateText)
			if err != nil {
				return err
			}
			options := libcal.AvailabilityOptions{
				Date:        date,
				Duration:    time.Duration(durationMinutes) * time.Minute,
				Category:    libcal.Category(strings.ToLower(categoryText)),
				IncludePods: includePods,
			}
			if afterText != "" {
				options.After, err = libcal.TimeOfDay(afterText)
				if err != nil {
					return err
				}
			}
			if beforeText != "" {
				options.Before, err = libcal.TimeOfDay(beforeText)
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
			slots, err := libcal.NewPublic().Availability(cmd.Context(), options)
			if err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{
				"location":         "Leavey Library",
				"date":             date.Format("2006-01-02"),
				"timezone":         "America/Los_Angeles",
				"duration_minutes": durationMinutes,
				"availability":     slots,
				"count":            len(slots),
			})
		},
	}
	cmd.Flags().StringVar(&dateText, "date", "today", "date in Los Angeles time: today, tomorrow, or YYYY-MM-DD")
	cmd.Flags().StringVar(&afterText, "after", "", "earliest start time, HH:MM in 24-hour format")
	cmd.Flags().StringVar(&beforeText, "before", "", "latest start time, HH:MM in 24-hour format")
	cmd.Flags().IntVar(&durationMinutes, "duration", 60, "reservation length in minutes (30-120, in 30-minute increments)")
	cmd.Flags().StringVar(&categoryText, "category", "rooms", "rooms, pods, all, lvl1, lvl2, or lvl3")
	cmd.Flags().BoolVar(&includePods, "include-pods", false, "include one-person study pods with group rooms")
	cmd.Flags().StringVar(&capacityText, "capacity", "", "capacity band: 1-4, 5-8, or 9-12 people")
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
		return 0, 0, errors.New("capacity must be one of 1-4, 5-8, or 9-12")
	}
	minimum, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, errors.New("capacity must be one of 1-4, 5-8, or 9-12")
	}
	maximum, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, errors.New("capacity must be one of 1-4, 5-8, or 9-12")
	}
	if !(minimum == 1 && maximum == 4 || minimum == 5 && maximum == 8 || minimum == 9 && maximum == 12) {
		return 0, 0, errors.New("capacity must be one of 1-4, 5-8, or 9-12")
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
