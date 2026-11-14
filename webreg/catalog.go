package webreg

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/nsigel/usc-cli/classes"
	"github.com/nsigel/usc-cli/internal/dom"

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
	document := dom.Parse(data)
	active := strings.TrimSuffix(text(dom.FindFirst(document, dom.ID("activeTermTab"))), " Classes")
	terms := []Term{}
	list := dom.FindFirst(document, dom.Class("ul", "terms-list"))
	for _, link := range dom.FindAll(list, dom.Tag("a")) {
		target, err := url.Parse(dom.Attr(link, "href"))
		if err != nil || target.Query().Get("term") == "" {
			continue
		}
		name := text(link)
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
	if err := c.useTerm(ctx, term); err != nil {
		return nil, err
	}
	data, err := c.page(ctx, "/Departments", "/Terms")
	if err != nil {
		return nil, err
	}
	document := dom.Parse(data)
	schools := []School{}
	for _, header := range dom.FindAll(document, dom.Class("div", "department-header")) {
		school := School{Name: text(dom.FindFirst(header, dom.Class("span", "department-name"))), Programs: []Program{}}
		body := dom.FindFirst(document, dom.ID(strings.TrimPrefix(dom.Attr(header, "href"), "#")))
		for _, link := range dom.FindAll(body, dom.Class("a", "deptSub")) {
			target, err := url.Parse(dom.Attr(link, "href"))
			if err != nil {
				continue
			}
			code := target.Query().Get("Program")
			name := text(link)
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

const maxCoursePages = 40

// ProgramCourses lists the courses a program offers in a term, following
// WebReg's pagination. Each section reports whether it is already in the
// course bin.
func (c *Client) ProgramCourses(ctx context.Context, term, programPrefix string) ([]Course, error) {
	programPrefix = strings.ToUpper(strings.TrimSpace(programPrefix))
	if programPrefix == "" {
		return nil, errors.New("a program prefix is required")
	}
	return c.courses(ctx, term, url.Values{"Program": {programPrefix}})
}

// Course returns one course in a term with its sections, each reporting
// whether it is already in the course bin.
func (c *Client) Course(ctx context.Context, term string, code classes.CourseCode) (Course, error) {
	courses, err := c.courses(ctx, term, url.Values{"Course": {code.String()}})
	if err != nil {
		return Course{}, err
	}
	for _, course := range courses {
		if course.Code.String() == code.String() {
			return course, nil
		}
	}
	return Course{}, fmt.Errorf("WebReg lists no course %s in term %s", code, term)
}

func (c *Client) courses(ctx context.Context, term string, query url.Values) ([]Course, error) {
	if err := c.useTerm(ctx, term); err != nil {
		return nil, err
	}
	courses := []Course{}
	for page := 1; page <= maxCoursePages; page++ {
		if page > 1 {
			query.Set("pageNumber", strconv.Itoa(page))
		}
		data, err := c.page(ctx, "/Courses?"+query.Encode(), "/Departments")
		if err != nil {
			return nil, err
		}
		document := dom.Parse(data)
		pageCourses, err := parseCourses(document)
		if err != nil {
			return nil, err
		}
		courses = append(courses, pageCourses...)
		next := dom.FindFirst(document, func(node *html.Node) bool {
			return node.Data == "a" && dom.Attr(node, "rel") == "next" && dom.Attr(node, "href") != ""
		})
		if next == nil {
			return courses, nil
		}
	}
	return nil, fmt.Errorf("WebReg listed more than %d pages of courses", maxCoursePages)
}
