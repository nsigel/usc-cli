package classes

import (
	"fmt"
	"time"
)

// Weekday is a day of the week. It is a time.Weekday that reads and writes
// its English name ("Monday") in JSON instead of a number.
type Weekday time.Weekday

// String returns the day's English name.
func (day Weekday) String() string {
	return time.Weekday(day).String()
}

// MarshalText writes the day's English name.
func (day Weekday) MarshalText() ([]byte, error) {
	return []byte(day.String()), nil
}

// UnmarshalText reads a day's English name.
func (day *Weekday) UnmarshalText(text []byte) error {
	for candidate := time.Sunday; candidate <= time.Saturday; candidate++ {
		if candidate.String() == string(text) {
			*day = Weekday(candidate)
			return nil
		}
	}
	return fmt.Errorf("unknown weekday %q", text)
}
