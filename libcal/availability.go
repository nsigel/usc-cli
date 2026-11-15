package libcal

import (
	"context"
	"time"
)

type AvailabilityOptions struct {
	Start       time.Time
	End         time.Time
	Category    Category
	MinCapacity int
	MaxCapacity int
}

// Availability contains spaces whose returned intervals cover the exact requested
// span. It neither holds a space nor guarantees a later booking will succeed.
type Availability struct {
	Start     time.Time         `json:"start"`
	End       time.Time         `json:"end"`
	Timezone  string            `json:"timezone"`
	Matches   []Space           `json:"matches"`
	WindowEnd map[Category]bool `json:"window_end"`
}

func (c *Client) Availability(ctx context.Context, options AvailabilityOptions) (Availability, error) {
	if err := validateInterval(options.Start, options.End); err != nil {
		return Availability{}, err
	}
	zone, err := pacific()
	if err != nil {
		return Availability{}, err
	}
	start, end := options.Start.In(zone), options.End.In(zone)
	schedule, err := c.Schedule(ctx, ScheduleOptions{
		Date: start, EndDate: localDay(end.Add(-time.Nanosecond), zone).AddDate(0, 0, 1),
		Category: options.Category, MinCapacity: options.MinCapacity, MaxCapacity: options.MaxCapacity,
	})
	if err != nil {
		return Availability{}, err
	}
	return schedule.Availability(start, end)
}

// Availability also filters an already-fetched schedule without another request.
func (s Schedule) Availability(start, end time.Time) (Availability, error) {
	if err := validateInterval(start, end); err != nil {
		return Availability{}, err
	}
	result := Availability{Start: start, End: end, Timezone: s.Timezone, Matches: []Space{}, WindowEnd: map[Category]bool{}}
	for _, category := range s.Categories {
		result.WindowEnd[category.Category.Slug] = category.WindowEnd
		for _, space := range category.Spaces {
			cursor := start
			valid := true
			for _, interval := range space.Intervals {
				if !interval.End.After(start) || !interval.Start.Before(end) {
					continue
				}
				if !interval.Available || !interval.Start.Equal(cursor) || !interval.End.After(cursor) || interval.End.After(end) {
					valid = false
					break
				}
				cursor = interval.End
			}
			if valid && cursor.Equal(end) {
				result.Matches = append(result.Matches, space.Space)
			}
		}
	}
	return result, nil
}
