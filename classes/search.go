package classes

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

// Search returns the courses in a term that match text the way the Schedule
// of Classes search box does: course codes, titles, descriptions, and
// instructor names.
func (c *Client) Search(ctx context.Context, termCode, text string) ([]Course, error) {
	if err := validateTermCode(termCode); err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("search text must not be empty")
	}
	var wire struct {
		Courses []courseResponse `json:"courses"`
	}
	query := url.Values{
		"termCode":   {termCode},
		"searchTerm": {text},
	}
	if err := c.get(ctx, "Search/Basic", query, &wire); err != nil {
		return nil, err
	}
	return normalizeCourses(wire.Courses)
}
