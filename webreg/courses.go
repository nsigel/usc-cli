package webreg

import (
	"github.com/nsigel/usc-cli/internal/dom"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type Course struct {
	Code          string    `json:"code"`
	Title         string    `json:"title"`
	Description   string    `json:"description,omitempty"`
	Prerequisites []string  `json:"prerequisites,omitempty"`
	Sections      []Section `json:"sections"`
}

type Section struct {
	ID int `json:"id"`

	Clearance   string       `json:"clearance,omitempty"`
	Session     string       `json:"session,omitempty"`
	Type        string       `json:"type,omitempty"`
	Units       string       `json:"units,omitempty"`
	Seats       Seats        `json:"seats"`
	Times       []string     `json:"times,omitempty"`
	Days        []string     `json:"days,omitempty"`
	Instructors []string     `json:"instructors,omitempty"`
	Locations   []string     `json:"locations,omitempty"`
	Notes       []string     `json:"notes,omitempty"`
	InCourseBin *bool        `json:"in_course_bin,omitempty"`
	CourseBin   *BinState    `json:"course_bin,omitempty"`
	GradeOption *GradeOption `json:"grade_option,omitempty"`
}

type Seats struct {
	Registered int    `json:"registered"`
	Capacity   int    `json:"capacity"`
	Closed     bool   `json:"closed"`
	Text       string `json:"text"`
}

type BinState struct {
	Scheduled  bool `json:"scheduled"`
	Registered bool `json:"registered"`
}

type GradeOption struct {
	Current       string        `json:"current,omitempty"`
	Choices       []GradeChoice `json:"choices,omitempty"`
	ChangeRequest bool          `json:"change_request"`

	path  string
	field string
	token string
}

type GradeChoice struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func parseCourses(document *html.Node) []Course {
	courses := []Course{}
	for _, header := range dom.FindAll(document, dom.Class("div", "course-header")) {
		if course, ok := parseCourse(header); ok {
			courses = append(courses, course)
		}
	}
	return courses
}

func parseCourse(header *html.Node) (Course, bool) {
	code := courseCode(header)
	if code == "" {
		return Course{}, false
	}
	course := Course{Code: code, Title: text(dom.FindFirst(header, dom.Class("span", "crsTitl"))), Sections: []Section{}}
	body := header.NextSibling
	for body != nil && body.Type != html.ElementNode {
		body = body.NextSibling
	}
	if description := dom.FindFirst(body, dom.Class("div", "bs-callout-default")); description != nil {
		course.Description = text(description)
	}
	for _, requirement := range dom.FindAll(body, dom.Class("span", "prereqcoreq_val")) {
		course.Prerequisites = append(course.Prerequisites, text(requirement))
	}
	for _, row := range dom.FindAll(body, dom.Class("div", "section")) {
		if section, ok := parseSection(row); ok {
			course.Sections = append(course.Sections, section)
		}
	}
	return course, true
}

func courseCode(header *html.Node) string {
	code := text(dom.FindFirst(header, dom.Class("span", "crsID")))
	return strings.ReplaceAll(strings.TrimSuffix(code, ":"), " ", "")
}

func parseSection(row *html.Node) (Section, bool) {
	id, err := strconv.Atoi(strings.TrimPrefix(dom.Attr(row, "id"), "section_"))
	if err != nil {
		return Section{}, false
	}
	section := Section{ID: id}
	for _, column := range dom.FindAll(row, dom.Class("span", "section_row")) {
		label := dom.FindFirst(column, dom.Class("span", "table-headers-xsmall"))
		if label == nil {
			continue
		}
		name := strings.TrimSuffix(text(label), ":")
		switch name {
		case "Section":
			fields := strings.Fields(text(dom.FindFirst(column, dom.Tag("b"))))
			if len(fields) > 1 {
				section.Clearance = fields[1]
			}
		case "Session":
			section.Session = columnText(column, label)
		case "Type":
			section.Type = columnText(column, label)
		case "Units":
			section.Units = columnText(column, label)
		case "Registered":
			section.Seats = parseSeats(columnText(column, label))
		case "Time":
			section.Times = columnValues(column, label)
		case "Days":
			section.Days = columnValues(column, label)
		case "Instructor":
			section.Instructors = columnValues(column, label)
		case "Location":
			section.Locations = columnValues(column, label)
		case "Action":
			section.CourseBin = parseBinState(column, id)
		case "Grade Option":
			section.GradeOption = parseGradeOption(column, label)
		}
	}
	if form := dom.FindFirst(row, dom.Class("div", "btnAddToMyCourseBin")); form != nil {
		inBin := dom.FindFirst(form, dom.Class("label", "alrdyinmycb")) != nil
		section.InCourseBin = &inBin
	}
	for _, note := range dom.FindAll(row, dom.Class("div", "dvSRtxt")) {

		if dom.Attr(note, "id") == "" {
			if content := text(note); content != "" {
				section.Notes = append(section.Notes, content)
			}
		}
	}
	return section, true
}

func columnText(column, label *html.Node) string {
	var builder strings.Builder
	for child := column.FirstChild; child != nil; child = child.NextSibling {
		if child == label || child.Type == html.ElementNode && (child.Data == "form" || dom.HasClass(child, "grdoptsel") || dom.HasClass(child, "spanUpdate")) {
			continue
		}
		builder.WriteString(" " + dom.Text(child))
	}
	return dom.Collapse(builder.String())
}

func columnValues(column, label *html.Node) []string {
	var values []string
	for child := column.FirstChild; child != nil; child = child.NextSibling {
		if child != label {
			values = append(values, lines(child)...)
		}
	}
	return values
}

func parseSeats(text string) Seats {
	seats := Seats{Text: text, Closed: strings.EqualFold(text, "closed")}
	if registered, capacity, ok := strings.Cut(text, " of "); ok {
		seats.Registered, _ = strconv.Atoi(strings.TrimSpace(registered))
		seats.Capacity, _ = strconv.Atoi(strings.TrimSpace(capacity))
	}
	return seats
}

func parseBinState(column *html.Node, id int) *BinState {
	suffix := "_" + strconv.Itoa(id)
	for _, candidate := range []struct {
		prefix string
		state  BinState
	}{
		{"schedY_regN", BinState{Scheduled: true}},
		{"schedN_regN", BinState{}},
		{"schedY_regY", BinState{Scheduled: true, Registered: true}},
		{"schedN_regY", BinState{Registered: true}},
	} {
		if bar := dom.FindFirst(column, dom.ID(candidate.prefix+suffix)); bar != nil && !hidden(bar) {
			state := candidate.state
			return &state
		}
	}
	return nil
}

func parseGradeOption(column, label *html.Node) *GradeOption {
	option := &GradeOption{Current: columnText(column, label)}
	form := dom.FindFirst(column, dom.Tag("form"))
	if form == nil {
		return option
	}
	option.path = dom.Attr(form, "data-ajax-url")
	option.ChangeRequest = strings.HasSuffix(option.path, "/GradeOptionChangeRequest")
	option.token = formFields(form)[tokenField]
	if selection := dom.FindFirst(form, dom.Tag("select")); selection != nil {
		option.field = dom.Attr(selection, "name")
		for _, choice := range dom.FindAll(selection, dom.Tag("option")) {
			code := dom.Attr(choice, "value")
			if code == "" {
				continue
			}
			name := text(choice)
			option.Choices = append(option.Choices, GradeChoice{Code: code, Name: name})
			if option.Current == "" && dom.HasAttr(choice, "selected") {
				option.Current = name
			}
		}
	}
	return option
}
