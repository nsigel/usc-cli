package libcal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// Category identifies a Leavey LibCal reservation category.
type Category string

const (
	CategoryRooms  Category = "rooms"
	CategoryPods   Category = "pods"
	CategoryAll    Category = "all"
	CategoryFirst  Category = "lvl1"
	CategorySecond Category = "lvl2"
	CategoryThird  Category = "lvl3"
)

// CategoryInfo describes one live Leavey reservation category.
type CategoryInfo struct {
	ID   int      `json:"id"`
	Name string   `json:"name"`
	Slug Category `json:"slug"`
	Kind string   `json:"kind"`
}

type categoryPage struct {
	CategoryInfo
	Path string
}

var leaveyCategories = []categoryPage{
	{CategoryInfo: CategoryInfo{ID: 8441, Name: "Leavey Group Study Rooms (1st Floor)", Slug: CategoryFirst, Kind: "room"}, Path: "/reserve/lvl1"},
	{CategoryInfo: CategoryInfo{ID: 4889, Name: "Leavey Group Study Rooms (2nd Floor)", Slug: CategorySecond, Kind: "room"}, Path: "/reserve/lvl2"},
	{CategoryInfo: CategoryInfo{ID: 4898, Name: "Leavey Group Study Rooms (3rd Floor)", Slug: CategoryThird, Kind: "room"}, Path: "/reserve/lvl3"},
	{CategoryInfo: CategoryInfo{ID: 49218, Name: "Leavey Study Pods", Slug: CategoryPods, Kind: "pod"}, Path: "/reserve/lvl-pods"},
}

// Categories returns USC's current Leavey LibCal categories in site order.
func Categories() []CategoryInfo {
	result := make([]CategoryInfo, len(leaveyCategories))
	for i, category := range leaveyCategories {
		result[i] = category.CategoryInfo
	}
	return result
}

// Space is a reservable room or pod in Leavey Library.
type Space struct {
	ID           int      `json:"id"`
	Name         string   `json:"name"`
	Capacity     int      `json:"capacity"`
	Category     string   `json:"category"`
	CategoryID   int      `json:"category_id"`
	CategorySlug Category `json:"category_slug"`
	Kind         string   `json:"kind"`
	URL          string   `json:"url"`
}

var (
	resourceBlockPattern = regexp.MustCompile(`(?s)resources\.push\(\{(.*?)\}\);`)
	stringProperty       = regexp.MustCompile(`(?m)^\s*%s:\s*"((?:\\.|[^"\\])*)"`)
	integerProperty      = regexp.MustCompile(`(?m)^\s*%s:\s*(-?\d+)`)
)

// Spaces lists reservable Leavey spaces. A blank selector or "all" includes
// group study rooms and pods; "rooms" excludes pods by default.
func (c *Client) Spaces(ctx context.Context, selector Category) ([]Space, error) {
	selected, err := selectCategories(selector)
	if err != nil {
		return nil, err
	}
	var spaces []Space
	for _, category := range selected {
		items, err := c.spacesForCategory(ctx, category)
		if err != nil {
			return nil, err
		}
		spaces = append(spaces, items...)
	}
	sort.Slice(spaces, func(i, j int) bool {
		if spaces[i].CategoryID != spaces[j].CategoryID {
			return spaces[i].CategoryID < spaces[j].CategoryID
		}
		return spaces[i].Name < spaces[j].Name
	})
	return spaces, nil
}

func (c *Client) spacesForCategory(ctx context.Context, category categoryPage) ([]Space, error) {
	data, err := c.request(ctx, "GET", category.Path, nil, "text/html,application/xhtml+xml")
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", category.Name, err)
	}
	blocks := resourceBlockPattern.FindAllSubmatch(data, -1)
	spaces := make([]Space, 0, len(blocks))
	for _, match := range blocks {
		block := string(match[1])
		id, err := intProperty(block, "eid")
		if err != nil {
			return nil, fmt.Errorf("parse %s room id: %w", category.Name, err)
		}
		capacity, err := intProperty(block, "capacity")
		if err != nil {
			return nil, fmt.Errorf("parse %s room capacity: %w", category.Name, err)
		}
		name, err := stringPropertyValue(block, "title")
		if err != nil {
			return nil, fmt.Errorf("parse %s room name: %w", category.Name, err)
		}
		path, err := stringPropertyValue(block, "url")
		if err != nil {
			return nil, fmt.Errorf("parse %s room link: %w", category.Name, err)
		}
		spaces = append(spaces, Space{
			ID:           id,
			Name:         name,
			Capacity:     capacity,
			Category:     category.Name,
			CategoryID:   category.ID,
			CategorySlug: category.Slug,
			Kind:         category.Kind,
			URL:          baseURL + path,
		})
	}
	if len(spaces) == 0 {
		return nil, fmt.Errorf("parse %s: no reservable rooms found", category.Name)
	}
	return spaces, nil
}

func intProperty(block, name string) (int, error) {
	pattern := regexp.MustCompile(fmt.Sprintf(integerProperty.String(), regexp.QuoteMeta(name)))
	match := pattern.FindStringSubmatch(block)
	if match == nil {
		return 0, errors.New("property is missing")
	}
	return strconv.Atoi(match[1])
}

func stringPropertyValue(block, name string) (string, error) {
	pattern := regexp.MustCompile(fmt.Sprintf(stringProperty.String(), regexp.QuoteMeta(name)))
	match := pattern.FindStringSubmatch(block)
	if match == nil {
		return "", errors.New("property is missing")
	}
	value, err := strconv.Unquote("\"" + match[1] + "\"")
	if err != nil {
		return "", err
	}
	return value, nil
}

// Selection identifies the exact space, start, and end to reserve.
type Selection struct {
	SpaceID int       `json:"space_id"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
}

type availableSlot struct {
	Space    Space
	Start    time.Time
	End      time.Time
	Checksum string
}

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
	location, err := pacific()
	if err != nil {
		return Schedule{}, err
	}
	date := options.Date
	if date.IsZero() {
		date = time.Now()
	}
	date = localDay(date, location)
	end := options.EndDate
	if end.IsZero() {
		end = date.AddDate(0, 0, 1)
	} else {
		end = localDay(end, location)
	}
	if !end.After(date) {
		return Schedule{}, errors.New("end date must be after start date")
	}
	result := Schedule{
		StartDate: date.Format("2006-01-02"), EndDate: end.Format("2006-01-02"),
		Timezone: location.String(), Categories: []CategorySchedule{},
	}
	for _, category := range selected {
		spaces, err := c.spacesForCategory(ctx, category)
		if err != nil {
			return Schedule{}, err
		}
		grid, err := c.grid(ctx, category.ID, result.StartDate, result.EndDate)
		if err != nil {
			return Schedule{}, fmt.Errorf("check %s schedule: %w", category.Name, err)
		}
		entry := CategorySchedule{Category: category.CategoryInfo, WindowEnd: grid.WindowEnd, Spaces: []SpaceSchedule{}}
		byID := make(map[int]int)
		for _, space := range spaces {
			if options.MinCapacity > 0 && space.Capacity < options.MinCapacity ||
				options.MaxCapacity > 0 && space.Capacity > options.MaxCapacity {
				continue
			}
			byID[space.ID] = len(entry.Spaces)
			entry.Spaces = append(entry.Spaces, SpaceSchedule{Space: space, Intervals: []Interval{}})
		}
		for _, raw := range grid.Slots {
			index, ok := byID[raw.ItemID]
			if !ok {
				continue
			}
			start, err := parseLibCalTime(raw.Start, location)
			if err != nil {
				return Schedule{}, err
			}
			finish, err := parseLibCalTime(raw.End, location)
			if err != nil {
				return Schedule{}, err
			}
			entry.Spaces[index].Intervals = append(entry.Spaces[index].Intervals, Interval{
				Start: start, End: finish, Available: raw.ClassName == "",
				Status: raw.ClassName, checksum: raw.Checksum,
			})
		}
		for i := range entry.Spaces {
			sort.SliceStable(entry.Spaces[i].Intervals, func(a, b int) bool {
				return entry.Spaces[i].Intervals[a].Start.Before(entry.Spaces[i].Intervals[b].Start)
			})
		}
		result.Categories = append(result.Categories, entry)
	}
	return result, nil
}

func localDay(value time.Time, location *time.Location) time.Time {
	value = value.In(location)
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, location)
}

// Validate rejects invalid inputs and durations over two hours without requests.
func (s Selection) Validate() error {
	if s.SpaceID < 1 || s.Start.IsZero() || s.End.IsZero() {
		return errors.New("an explicit space, start, and end are required")
	}
	if !s.End.After(s.Start) {
		return errors.New("end must be after start")
	}
	if s.End.Sub(s.Start) > 2*time.Hour {
		return &Error{Code: "libcal_duration_limit", Message: "a LibCal reservation cannot exceed 2 hours; choose a shorter interval from the schedule"}
	}
	if s.Start.Second() != 0 || s.Start.Nanosecond() != 0 ||
		(s.End.Second() != 0 || s.End.Nanosecond() != 0) {
		return errors.New("start and end must use whole minutes")
	}
	return nil
}

// Resolve only the selected space and start; LibCal's hold response validates
// the requested end. Never substitute a different room, start, or duration.
func (c *Client) selectedSlot(ctx context.Context, selection Selection) (availableSlot, error) {
	if err := selection.Validate(); err != nil {
		return availableSlot{}, err
	}
	spaces, err := c.Spaces(ctx, CategoryAll)
	if err != nil {
		return availableSlot{}, err
	}
	for _, space := range spaces {
		if space.ID != selection.SpaceID {
			continue
		}
		schedule, err := c.Schedule(ctx, ScheduleOptions{Date: selection.Start, Category: space.CategorySlug})
		if err != nil {
			return availableSlot{}, err
		}
		for _, category := range schedule.Categories {
			for _, room := range category.Spaces {
				if room.Space.ID != selection.SpaceID {
					continue
				}
				for _, interval := range room.Intervals {
					if interval.Start.Equal(selection.Start) && interval.Available && interval.checksum != "" {
						end := selection.End.In(interval.Start.Location())
						return availableSlot{Space: room.Space, Start: interval.Start, End: end, Checksum: interval.checksum}, nil
					}
				}
			}
		}
		return availableSlot{}, &Error{Code: "libcal_selected_slot_unavailable", Message: "the selected space and start are unavailable; refresh the schedule"}
	}
	return availableSlot{}, fmt.Errorf("Leavey space %d was not found", selection.SpaceID)
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
	data, err := c.request(ctx, "POST", "/spaces/availability/grid", values, "application/json, text/javascript, */*;q=0.01")
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

func selectCategories(selector Category) ([]categoryPage, error) {
	switch selector {
	case CategoryRooms:
		return append([]categoryPage(nil), leaveyCategories[:3]...), nil
	case "", CategoryAll:
		return append([]categoryPage(nil), leaveyCategories...), nil
	case CategoryPods:
		return []categoryPage{categoryBySlug(CategoryPods)}, nil
	case CategoryFirst:
		return []categoryPage{categoryBySlug(CategoryFirst)}, nil
	case CategorySecond:
		return []categoryPage{categoryBySlug(CategorySecond)}, nil
	case CategoryThird:
		return []categoryPage{categoryBySlug(CategoryThird)}, nil
	default:
		return nil, fmt.Errorf("unknown Leavey category %q (use rooms, pods, all, lvl1, lvl2, or lvl3)", selector)
	}
}

func categoryBySlug(slug Category) categoryPage {
	for _, category := range leaveyCategories {
		if category.Slug == slug {
			return category
		}
	}
	return categoryPage{}
}

func categoryReturnURL(category categoryPage) string {
	for i, candidate := range leaveyCategories {
		if candidate.ID == category.ID {
			return fmt.Sprintf("%s?c=%d", candidate.Path, i+1)
		}
	}
	return category.Path
}

func pacific() (*time.Location, error) {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return nil, fmt.Errorf("load USC time zone: %w", err)
	}
	return location, nil
}

// DateInLosAngeles parses a date as it appears on USC's local calendar.
func DateInLosAngeles(value string) (time.Time, error) {
	location, err := pacific()
	if err != nil {
		return time.Time{}, err
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, location)
	if err != nil {
		return time.Time{}, errors.New("date must use YYYY-MM-DD")
	}
	return parsed, nil
}
