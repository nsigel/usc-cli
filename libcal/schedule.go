package libcal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	http "github.com/saucesteals/fhttp"
)

// ScheduleOptions selects a date range and optional inventory filters.
// EndDate is exclusive and defaults to the day after Date. No duration,
// ranking, or room-type preference is applied.
type ScheduleOptions struct {
	Date        time.Time
	EndDate     time.Time
	Category    Category
	MinCapacity int
	MaxCapacity int
}

// Schedule contains the server's intervals, including unavailable intervals.
// Missing intervals are not evidence of availability. WindowEnd is preserved
// per category. Callers choose continuous intervals of up to two hours.
type Schedule struct {
	StartDate  string             `json:"start_date"`
	EndDate    string             `json:"end_date"`
	Timezone   string             `json:"timezone"`
	Categories []CategorySchedule `json:"categories"`
}

type CategorySchedule struct {
	Category  CategoryInfo    `json:"category"`
	WindowEnd bool            `json:"window_end"`
	Spaces    []SpaceSchedule `json:"spaces"`
}

type SpaceSchedule struct {
	Space     Space      `json:"space"`
	Intervals []Interval `json:"intervals"`
}

type Interval struct {
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Available bool      `json:"available"`
	Status    string    `json:"status"`
	checksum  string
}

// Schedule returns the live grid without synthesizing durations or choosing
// a slot. Callers select a continuous span of up to two hours for Book.
func (c *Client) Schedule(ctx context.Context, options ScheduleOptions) (Schedule, error) {
	if options.MinCapacity < 0 || options.MaxCapacity < 0 ||
		(options.MaxCapacity > 0 && options.MinCapacity > options.MaxCapacity) {
		return Schedule{}, errors.New("capacity bounds must be nonnegative and minimum must not exceed maximum")
	}
	selected, err := selectCategories(options.Category)
	if err != nil {
		return Schedule{}, err
	}
	start, end, err := scheduleWindow(options)
	if err != nil {
		return Schedule{}, err
	}
	result := Schedule{
		StartDate: start.Format(time.DateOnly), EndDate: end.Format(time.DateOnly),
		Timezone: start.Location().String(), Categories: make([]CategorySchedule, len(selected)),
	}
	for index, category := range selected {
		result.Categories[index], err = c.categorySchedule(ctx, category, options, start, end)
		if err != nil {
			return Schedule{}, fmt.Errorf("check %s schedule: %w", category.Name, err)
		}
	}
	return result, nil
}

// scheduleWindow resolves the options to whole Los Angeles days: Date
// defaults to today and EndDate to the following day.
func scheduleWindow(options ScheduleOptions) (start, end time.Time, err error) {
	location, err := pacific()
	if err != nil {
		return start, end, err
	}
	start = options.Date
	if start.IsZero() {
		start = time.Now()
	}
	start = localDay(start, location)
	end = start.AddDate(0, 0, 1)
	if !options.EndDate.IsZero() {
		end = localDay(options.EndDate, location)
	}
	if !end.After(start) {
		return start, end, errors.New("end date must be after start date")
	}
	return start, end, nil
}

func (c *Client) categorySchedule(ctx context.Context, category categoryPage, options ScheduleOptions, start, end time.Time) (CategorySchedule, error) {
	spaces, err := c.spacesIn(ctx, category)
	if err != nil {
		return CategorySchedule{}, err
	}
	grid, err := c.grid(ctx, category.ID, start.Format(time.DateOnly), end.Format(time.DateOnly))
	if err != nil {
		return CategorySchedule{}, err
	}
	schedule := CategorySchedule{Category: category.CategoryInfo, WindowEnd: grid.WindowEnd, Spaces: []SpaceSchedule{}}
	position := make(map[int]int)
	for _, space := range spaces {
		if options.MinCapacity > 0 && space.Capacity < options.MinCapacity ||
			options.MaxCapacity > 0 && space.Capacity > options.MaxCapacity {
			continue
		}
		position[space.ID] = len(schedule.Spaces)
		schedule.Spaces = append(schedule.Spaces, SpaceSchedule{Space: space, Intervals: []Interval{}})
	}
	for _, slot := range grid.Slots {
		index, ok := position[slot.ItemID]
		if !ok {
			continue
		}
		slotStart, err := parseLibCalTime(slot.Start, start.Location())
		if err != nil {
			return CategorySchedule{}, err
		}
		slotEnd, err := parseLibCalTime(slot.End, start.Location())
		if err != nil {
			return CategorySchedule{}, err
		}
		schedule.Spaces[index].Intervals = append(schedule.Spaces[index].Intervals, Interval{
			Start: slotStart, End: slotEnd, Available: slot.ClassName == "",
			Status: slot.ClassName, checksum: slot.Checksum,
		})
	}
	for _, space := range schedule.Spaces {
		sort.SliceStable(space.Intervals, func(a, b int) bool { return space.Intervals[a].Start.Before(space.Intervals[b].Start) })
	}
	return schedule, nil
}

type gridResponse struct {
	Slots     []gridSlot `json:"slots"`
	WindowEnd bool       `json:"windowEnd"`
}

type gridSlot struct {
	Start     string `json:"start"`
	End       string `json:"end"`
	ItemID    int    `json:"itemId"`
	Checksum  string `json:"checksum"`
	ClassName string `json:"className"`
}

func (c *Client) grid(ctx context.Context, categoryID int, start, end string) (gridResponse, error) {
	values := url.Values{
		"lid":       {strconv.Itoa(leaveyLocationID)},
		"gid":       {strconv.Itoa(categoryID)},
		"eid":       {"-1"},
		"seat":      {"0"},
		"seatId":    {"0"},
		"zone":      {"0"},
		"start":     {start},
		"end":       {end},
		"pageIndex": {"0"},
		"pageSize":  {"100"},
	}
	data, err := c.request(ctx, http.MethodPost, "/spaces/availability/grid", values, ajaxAccept, "")
	if err != nil {
		return gridResponse{}, err
	}
	var result gridResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return gridResponse{}, fmt.Errorf("decode availability response: %w", err)
	}
	return result, nil
}

func parseLibCalTime(value string, location *time.Location) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse LibCal time %q", value)
}

func localDay(value time.Time, location *time.Location) time.Time {
	value = value.In(location)
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, location)
}

func pacific() (*time.Location, error) {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return nil, fmt.Errorf("load USC time zone: %w", err)
	}
	return location, nil
}

// ParseDate reads a day on USC's calendar: "today", "tomorrow", or YYYY-MM-DD.
func ParseDate(value string) (time.Time, error) {
	location, err := pacific()
	if err != nil {
		return time.Time{}, err
	}
	today := localDay(time.Now(), location)
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "today":
		return today, nil
	case "tomorrow":
		return today.AddDate(0, 0, 1), nil
	}
	parsed, err := time.ParseInLocation(time.DateOnly, value, location)
	if err != nil {
		return time.Time{}, errors.New("date must be today, tomorrow, or YYYY-MM-DD")
	}
	return parsed, nil
}
