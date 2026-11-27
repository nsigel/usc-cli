package webreg

import (
	"context"
	"errors"
	"github.com/nsigel/usc-cli/internal/dom"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type Term struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

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

type School struct {
	Name     string    `json:"name"`
	Programs []Program `json:"programs"`
}

type Program struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func (c *Client) Programs(ctx context.Context, term string) ([]School, error) {
	if err := c.SelectTerm(ctx, term); err != nil {
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

type CourseQuery struct {
	Program string
	Course  string
}

const maxCoursePages = 40

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
		document := dom.Parse(data)
		courses = append(courses, parseCourses(document)...)
		next := dom.FindFirst(document, func(node *html.Node) bool {
			return node.Data == "a" && dom.Attr(node, "rel") == "next" && dom.Attr(node, "href") != ""
		})
		if next == nil {
			return courses, nil
		}
	}
	return courses, nil
}

func normalizeCourseCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if fields := strings.Fields(code); len(fields) == 2 {
		return fields[0] + "-" + fields[1]
	}
	return code
}
