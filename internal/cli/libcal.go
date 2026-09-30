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
	cmd.AddCommand(libcalCategoriesCommand(), libcalSpacesCommand(), libcalRoomCommand(), libcalAvailabilityCommand(), libcalBookCommand())
	return cmd
}

func libcalBookCommand() *cobra.Command {
	var dateText, afterText, beforeText, categoryText, capacityText, name, email string
	var durationMinutes int
	var includePods, acceptTerms bool
	var customFields []string
	cmd := &cobra.Command{
		Use:   "book",
		Short: "Book the earliest matching Leavey study room",
		Args:  cobra.NoArgs,
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
			fields, err := parseLibCalFields(customFields)
			if err != nil {
				return err
			}
			client, err := libcal.Open(cmd.Context())
			if err != nil {
				return err
			}
			reservation, err := client.BookEarliest(cmd.Context(), options, libcal.ReservationDetails{
				Name: name, Email: email, Fields: fields, AcceptTerms: acceptTerms,
			})
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
	cmd.Flags().StringVar(&name, "name", "", "name to use if LibCal requests it")
	cmd.Flags().StringVar(&email, "email", "", "USC email to use if LibCal requests it")
	cmd.Flags().StringArrayVar(&customFields, "field", nil, "additional LibCal form value as FIELD=VALUE (repeatable)")
	cmd.Flags().BoolVar(&acceptTerms, "accept-terms", false, "confirm acceptance of the displayed LibCal reservation terms")
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
