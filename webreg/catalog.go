package webreg

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Term is a registration term WebReg offers.
type Term struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// Terms lists the terms open in WebReg. Active marks the session's current
// term, if one has been selected.
func (c *Client) Terms(ctx context.Context) ([]Term, error) {
	data, err := c.page(ctx, "/Terms", "")
	if err != nil {
		return nil, err
	}
	document := parseHTML(data)
	active := strings.TrimSuffix(collapse(textOf(findFirst(document, byID("activeTermTab")))), " Classes")
	terms := []Term{}
	list := findFirst(document, byClass("ul", "terms-list"))
	for _, link := range findAll(list, func(node *html.Node) bool { return node.Data == "a" }) {
		target, err := url.Parse(attr(link, "href"))
		if err != nil || target.Query().Get("term") == "" {
			continue
		}
		name := collapse(textOf(link))
		terms = append(terms, Term{Code: target.Query().Get("term"), Name: name, Active: active != "" && name == active})
	}
	if len(terms) == 0 {
		return nil, errors.New("WebReg terms page listed no terms")
	}
	return terms, nil
}

// School groups the programs (subject codes) WebReg lists under a school.
type School struct {
	Name     string    `json:"name"`
	Programs []Program `json:"programs"`
}

// Program is a subject such as BUAD that can be passed to Courses.
type Program struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Programs lists a term's schools and their programs.
func (c *Client) Programs(ctx context.Context, term string) ([]School, error) {
	if err := c.SelectTerm(ctx, term); err != nil {
		return nil, err
	}
	data, err := c.page(ctx, "/Departments", "/Terms")
	if err != nil {
		return nil, err
	}
	document := parseHTML(data)
	schools := []School{}
	for _, header := range findAll(document, byClass("div", "department-header")) {
		school := School{Name: collapse(textOf(findFirst(header, byClass("span", "department-name")))), Programs: []Program{}}
		body := findFirst(document, byID(strings.TrimPrefix(attr(header, "href"), "#")))
		for _, link := range findAll(body, byClass("a", "deptSub")) {
			target, err := url.Parse(attr(link, "href"))
			if err != nil {
				continue
			}
			code := target.Query().Get("Program")
			name := collapse(textOf(link))
			name = strings.TrimSpace(strings.TrimSuffix(name, "("+code+")"))
			school.Programs = append(school.Programs, Program{Code: code, Name: name})
		}
		schools = append(schools, school)
	}
	if len(schools) == 0 {
		return nil, errors.New("WebReg listed no programs for term " + term)
	}
	return schools, nil
}

// CourseQuery selects WebReg catalog courses. Set exactly one field.
type CourseQuery struct {
	Program string // subject code, e.g. BUAD
	Course  string // course code, e.g. BUAD-302
}

const maxCoursePages = 40

// Courses lists a term's courses for a program or a single course code,
// following WebReg's pagination. Each section reports whether it is already
// in the course bin.
func (c *Client) Courses(ctx context.Context, term string, query CourseQuery) ([]Course, error) {
	values := url.Values{}
	switch {
	case query.Program != "" && query.Course != "":
		return nil, errors.New("set either a program or a course, not both")
	case query.Program != "":
		values.Set("Program", strings.ToUpper(strings.TrimSpace(query.Program)))
	case query.Course != "":
		values.Set("Course", normalizeCourseCode(query.Course))
	default:
		return nil, errors.New("a program or course is required")
	}
	if err := c.SelectTerm(ctx, term); err != nil {
		return nil, err
	}
	courses := []Course{}
	for page := 1; page <= maxCoursePages; page++ {
		if page > 1 {
			values.Set("pageNumber", strconv.Itoa(page))
		}
		data, err := c.page(ctx, "/Courses?"+values.Encode(), "/Departments")
		if err != nil {
			return nil, err
		}
		document := parseHTML(data)
		courses = append(courses, parseCourses(document)...)
		next := findFirst(document, func(node *html.Node) bool {
			return node.Data == "a" && attr(node, "rel") == "next" && attr(node, "href") != ""
		})
		if next == nil {
			return courses, nil
		}
	}
	return courses, nil
}

// normalizeCourseCode accepts "buad 302" or "BUAD-302".
func normalizeCourseCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if fields := strings.Fields(code); len(fields) == 2 {
		return fields[0] + "-" + fields[1]
	}
	return code
}
