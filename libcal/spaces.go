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
	"strings"
	"time"
)

// Category identifies a Leavey LibCal reservation category. Rooms is the
// default; Pods is a separate one-person category.
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
	selected, err := selectCategories(selector, true)
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

// AvailableSlot is one continuous available start time for a space. Checksum
// is an opaque LibCal value used only when Reserve is called.
type AvailableSlot struct {
	Space           Space     `json:"space"`
	Start           time.Time `json:"start"`
	End             time.Time `json:"end"`
	DurationMinutes int       `json:"duration_minutes"`
	Checksum        string    `json:"-"`
}

// AvailabilityOptions filters Leavey's availability grid.
type AvailabilityOptions struct {
	Date        time.Time
	After       time.Duration
	Before      time.Duration
	Duration    time.Duration
	Category    Category
	IncludePods bool
	MinCapacity int
	MaxCapacity int
}

// Availability returns continuous start times that satisfy the requested
// duration. LibCal reports each room in 30-minute grid intervals.
func (c *Client) Availability(ctx context.Context, options AvailabilityOptions) ([]AvailableSlot, error) {
	if options.Duration == 0 {
		options.Duration = time.Hour
	}
	if options.Duration < 30*time.Minute || options.Duration > 2*time.Hour || options.Duration%(30*time.Minute) != 0 {
		return nil, errors.New("duration must be a multiple of 30 minutes between 30 minutes and 2 hours")
	}
	if options.After < 0 || options.After >= 24*time.Hour || options.Before < 0 || options.Before >= 24*time.Hour {
		return nil, errors.New("time filters must be between 00:00 and 23:59")
	}
	if options.Before > 0 && options.After > 0 && options.Before <= options.After {
		return nil, errors.New("end time must be later than start time")
	}
	selected, err := selectCategories(options.Category, false)
	if err != nil {
		return nil, err
	}
	if options.IncludePods && !containsCategory(selected, CategoryPods) {
		selected = append(selected, categoryBySlug(CategoryPods))
	}
	location, err := pacific()
	if err != nil {
		return nil, err
	}
	date := options.Date
	if date.IsZero() {
		date = time.Now().In(location)
	}
	date = time.Date(date.In(location).Year(), date.In(location).Month(), date.In(location).Day(), 0, 0, 0, 0, location)
	day := date.Format("2006-01-02")
	// Include the following day so evening reservations can cross midnight.
	endDay := date.AddDate(0, 0, 2).Format("2006-01-02")

	var result []AvailableSlot
	for _, category := range selected {
		spaces, err := c.spacesForCategory(ctx, category)
		if err != nil {
			return nil, err
		}
		byID := make(map[int]Space, len(spaces))
		for _, space := range spaces {
			if options.MinCapacity > 0 && space.Capacity < options.MinCapacity {
				continue
			}
			if options.MaxCapacity > 0 && space.Capacity > options.MaxCapacity {
				continue
			}
			byID[space.ID] = space
		}
		if len(byID) == 0 {
			continue
		}
		grid, err := c.grid(ctx, category.ID, day, endDay)
		if err != nil {
			return nil, fmt.Errorf("check %s availability: %w", category.Name, err)
		}
		roomSlots := make(map[int][]gridSlot)
		for _, slot := range grid.Slots {
			if _, ok := byID[slot.ItemID]; ok {
				roomSlots[slot.ItemID] = append(roomSlots[slot.ItemID], slot)
			}
		}
		for id, slots := range roomSlots {
			candidates, err := continuousSlots(byID[id], slots, date, options)
			if err != nil {
				return nil, err
			}
			result = append(result, candidates...)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].Start.Equal(result[j].Start) {
			return result[i].Start.Before(result[j].Start)
		}
		if result[i].Space.Kind != result[j].Space.Kind {
			return result[i].Space.Kind == "room"
		}
		return result[i].Space.Name < result[j].Space.Name
	})
	return result, nil
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

func continuousSlots(space Space, raw []gridSlot, date time.Time, options AvailabilityOptions) ([]AvailableSlot, error) {
	location := date.Location()
	type interval struct {
		start, end time.Time
		checksum   string
		available  bool
	}
	intervals := make([]interval, 0, len(raw))
	for _, slot := range raw {
		start, err := parseLibCalTime(slot.Start, location)
		if err != nil {
			return nil, err
		}
		end, err := parseLibCalTime(slot.End, location)
		if err != nil {
			return nil, err
		}
		if start.Before(date) {
			continue
		}
		intervals = append(intervals, interval{start: start, end: end, checksum: slot.Checksum, available: slot.ClassName == ""})
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].start.Before(intervals[j].start) })
	byStart := make(map[int64]interval, len(intervals))
	for _, slot := range intervals {
		byStart[slot.start.Unix()] = slot
	}
	var available []AvailableSlot
	for _, first := range intervals {
		if !first.start.Before(date.AddDate(0, 0, 1)) || !first.available || first.start.Minute()%30 != 0 {
			continue
		}
		minuteOfDay := time.Duration(first.start.Hour())*time.Hour + time.Duration(first.start.Minute())*time.Minute
		if options.After > 0 && minuteOfDay < options.After {
			continue
		}
		if options.Before > 0 && minuteOfDay >= options.Before {
			continue
		}
		end := first.start.Add(options.Duration)
		cursor := first.start
		ok := true
		for cursor.Before(end) {
			part, found := byStart[cursor.Unix()]
			if !found || !part.available || !part.end.After(cursor) || part.end.After(end) {
				ok = false
				break
			}
			cursor = part.end
		}
		if ok && cursor.Equal(end) {
			available = append(available, AvailableSlot{
				Space:           space,
				Start:           first.start,
				End:             end,
				DurationMinutes: int(options.Duration / time.Minute),
				Checksum:        first.checksum,
			})
		}
	}
	return available, nil
}

func parseLibCalTime(value string, location *time.Location) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse LibCal time %q", value)
}

func selectCategories(selector Category, includeAll bool) ([]categoryPage, error) {
	switch selector {
	case "", CategoryRooms:
		return append([]categoryPage(nil), leaveyCategories[:3]...), nil
	case CategoryAll:
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
		if includeAll {
			return nil, fmt.Errorf("unknown Leavey category %q (use rooms, pods, all, lvl1, lvl2, or lvl3)", selector)
		}
		return nil, fmt.Errorf("unknown Leavey category %q (use rooms, pods, lvl1, lvl2, or lvl3)", selector)
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

func containsCategory(categories []categoryPage, target Category) bool {
	for _, category := range categories {
		if category.Slug == target {
			return true
		}
	}
	return false
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

// TimeOfDay parses a local time such as "18:30" into a duration since
// midnight, suitable for AvailabilityOptions.After or Before.
func TimeOfDay(value string) (time.Duration, error) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return 0, errors.New("time must use HH:MM in 24-hour format")
	}
	return time.Duration(parsed.Hour())*time.Hour + time.Duration(parsed.Minute())*time.Minute, nil
}
