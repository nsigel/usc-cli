package classes

import (
	"context"
	"encoding/json"
	"fmt"
	http "github.com/saucesteals/fhttp"
	"net/url"
	"strconv"
)

type Term struct {
	TermCode string `json:"term_code"`
	Season   string `json:"season"`
	Year     int    `json:"year"`
}
type Program struct {
	Prefix    string `json:"prefix"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}
type School struct {
	Prefix    string    `json:"prefix"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	Programs  []Program `json:"programs"`
}
type SearchResult struct {
	CourseCount int       `json:"course_count"`
	Courses     []Course  `json:"courses"`
	Programs    []Program `json:"programs"`
}
type programResponse struct {
	Prefix    string
	Name      string
	SortOrder int
}

func normalizePrograms(w []programResponse) []Program {
	p := make([]Program, len(w))
	for i, v := range w {
		p[i] = Program(v)
	}
	return p
}
func (c *Client) get(ctx context.Context, path string, values url.Values, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://classes.usc.edu/api/"+path+"?"+values.Encode(), nil)
	if err != nil {
		return err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("classes: HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(result)
}
func (c *Client) Terms(ctx context.Context) ([]Term, error) {
	var wire []struct {
		TermCode int
		Season   string
		Year     int
	}
	if err := c.get(ctx, "Terms/Active", nil, &wire); err != nil {
		return nil, err
	}
	terms := make([]Term, len(wire))
	for i, w := range wire {
		terms[i] = Term{strconv.Itoa(w.TermCode), w.Season, w.Year}
	}
	return terms, nil
}
func (c *Client) Schools(ctx context.Context, termCode string) ([]School, error) {
	var wire []struct {
		Prefix    string
		Name      string
		SortOrder int
		Programs  []programResponse
	}
	if err := c.get(ctx, "Schools/TermCode", url.Values{"termCode": {termCode}}, &wire); err != nil {
		return nil, err
	}
	schools := make([]School, len(wire))
	for i, w := range wire {
		schools[i] = School{w.Prefix, w.Name, w.SortOrder, normalizePrograms(w.Programs)}
	}
	return schools, nil
}

// Courses requires the school's prefix: USC returns no courses when school is omitted.
func (c *Client) Courses(ctx context.Context, termCode, schoolPrefix, programPrefix string) ([]Course, error) {
	if termCode == "" || schoolPrefix == "" || programPrefix == "" {
		return nil, fmt.Errorf("term code, school prefix, and program prefix are required")
	}
	var wire struct{ Courses []courseResponse }
	if err := c.get(ctx, "Courses/CoursesByTermSchoolProgram", url.Values{"termCode": {termCode}, "school": {schoolPrefix}, "program": {programPrefix}}, &wire); err != nil {
		return nil, err
	}
	return normalizeCourses(wire.Courses), nil
}
func normalizeCourses(w []courseResponse) []Course {
	courses := make([]Course, len(w))
	for i, v := range w {
		courses[i] = normalizeCourse(v)
	}
	return courses
}
func (c *Client) Search(ctx context.Context, termCode, searchTerm string) (SearchResult, error) {
	var wire struct {
		CourseCount int
		Courses     []courseResponse
		Programs    []programResponse
	}
	err := c.get(ctx, "Search/Basic", url.Values{"termCode": {termCode}, "searchTerm": {searchTerm}}, &wire)
	return SearchResult{wire.CourseCount, normalizeCourses(wire.Courses), normalizePrograms(wire.Programs)}, err
}
