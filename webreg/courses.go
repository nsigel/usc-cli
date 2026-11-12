package webreg

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Course is a WebReg course and the sections shown for it on one page.
type Course struct {
	Code          string    `json:"code"`
	Title         string    `json:"title"`
	Description   string    `json:"description,omitempty"`
	Prerequisites []string  `json:"prerequisites,omitempty"`
	Sections      []Section `json:"sections"`
}

// Section is one WebReg section row. Fields that only one page shows are
// omitted elsewhere: InCourseBin comes from catalog pages, CourseBin and
// GradeOption from the course bin.
type Section struct {
	ID int `json:"id"`
	// Clearance is WebReg's letter beside the section ID: "R" sections are
	// open to registration and "D" sections need department clearance.
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

// Seats is the section's registration count as WebReg displays it.
type Seats struct {
	Registered int    `json:"registered"`
	Capacity   int    `json:"capacity"`
	Closed     bool   `json:"closed"`
	Text       string `json:"text"`
}

// BinState is a course-bin section's schedule and registration state.
// Checkout registers scheduled, unregistered sections and drops registered,
// unscheduled ones.
type BinState struct {
	Scheduled  bool `json:"scheduled"`
	Registered bool `json:"registered"`
}

// GradeOption is a course-bin section's grading basis and the choices WebReg
// offers. ChangeRequest is true for registered sections, whose change is
// submitted as a request instead of applied directly.
type GradeOption struct {
	Current       string        `json:"current,omitempty"`
	Choices       []GradeChoice `json:"choices,omitempty"`
	ChangeRequest bool          `json:"change_request"`

	path  string
	field string
	token string
}

// GradeChoice is a grade-option code such as L (letter) and its label.
type GradeChoice struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// parseCourses reads every course header and its sections from a page.
func parseCourses(document *html.Node) []Course {
	courses := []Course{}
	for _, header := range findAll(document, byClass("div", "course-header")) {
		code := courseCode(header)
		if code == "" {
			continue
		}
		course := Course{Code: code, Title: collapse(textOf(findFirst(header, byClass("span", "crsTitl")))), Sections: []Section{}}
		// The header's href names its body. It omits suffixes such as the x in
		// BUAD-200x, so the displayed code cannot be used to find it.
		body := findFirst(document, byID(strings.TrimPrefix(attr(header, "href"), "#")))
		if body != nil {
			if description := findFirst(body, byClass("div", "bs-callout-default")); description != nil {
				course.Description = collapse(textOf(description))
			}
			for _, requirement := range findAll(body, byClass("span", "prereqcoreq_val")) {
				course.Prerequisites = append(course.Prerequisites, collapse(textOf(requirement)))
			}
			for _, row := range findAll(body, byClass("div", "section")) {
				if section, ok := parseSection(row); ok {
					course.Sections = append(course.Sections, section)
				}
			}
		}
		courses = append(courses, course)
	}
	return courses
}

// courseCode reads a header's code, joining suffixes such as "BUAD-200 x".
func courseCode(header *html.Node) string {
	code := collapse(textOf(findFirst(header, byClass("span", "crsID"))))
	return strings.ReplaceAll(strings.TrimSuffix(code, ":"), " ", "")
}

func parseSection(row *html.Node) (Section, bool) {
	id, err := strconv.Atoi(strings.TrimPrefix(attr(row, "id"), "section_"))
	if err != nil {
		return Section{}, false
	}
	section := Section{ID: id}
	for _, column := range findAll(row, byClass("span", "section_row")) {
		label := findFirst(column, byClass("span", "table-headers-xsmall"))
		if label == nil {
			continue
		}
		name := strings.TrimSuffix(collapse(textOf(label)), ":")
		switch name {
		case "Section":
			fields := strings.Fields(collapse(textOf(findFirst(column, func(node *html.Node) bool { return node.Data == "b" }))))
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
	if form := findFirst(row, byClass("div", "btnAddToMyCourseBin")); form != nil {
		inBin := findFirst(form, byClass("label", "alrdyinmycb")) != nil
		section.InCourseBin = &inBin
	}
	for _, note := range findAll(row, byClass("div", "dvSRtxt")) {
		// Status notes have IDs and are toggled by the action bar; the
		// remaining callouts are section notes such as fees or locations.
		if attr(note, "id") == "" {
			if text := collapse(textOf(note)); text != "" {
				section.Notes = append(section.Notes, text)
			}
		}
	}
	return section, true
}

// columnText returns a column's text without its mobile label or forms.
func columnText(column, label *html.Node) string {
	var builder strings.Builder
	for child := column.FirstChild; child != nil; child = child.NextSibling {
		if child == label || child.Type == html.ElementNode && (child.Data == "form" || hasClass(child, "grdoptsel") || hasClass(child, "spanUpdate")) {
			continue
		}
		builder.WriteString(" " + textOf(child))
	}
	return collapse(builder.String())
}

// columnValues returns a multi-valued column such as instructors, one value
// per line in WebReg's markup.
func columnValues(column, label *html.Node) []string {
	var values []string
	for child := column.FirstChild; child != nil; child = child.NextSibling {
		if child != label {
			values = append(values, lines(textOf(child))...)
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

// parseBinState reads which pre-rendered action bar WebReg left visible.
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
		if bar := findFirst(column, byID(candidate.prefix+suffix)); bar != nil && !hidden(bar) {
			state := candidate.state
			return &state
		}
	}
	return nil
}

func parseGradeOption(column, label *html.Node) *GradeOption {
	option := &GradeOption{Current: columnText(column, label)}
	form := findFirst(column, func(node *html.Node) bool { return node.Data == "form" })
	if form == nil {
		return option
	}
	option.path = attr(form, "data-ajax-url")
	option.ChangeRequest = strings.HasSuffix(option.path, "/GradeOptionChangeRequest")
	option.token = formFields(form)["__RequestVerificationToken"]
	if selection := findFirst(form, func(node *html.Node) bool { return node.Data == "select" }); selection != nil {
		option.field = attr(selection, "name")
		for _, choice := range findAll(selection, func(node *html.Node) bool { return node.Data == "option" }) {
			code := attr(choice, "value")
			if code == "" {
				continue
			}
			name := collapse(textOf(choice))
			option.Choices = append(option.Choices, GradeChoice{Code: code, Name: name})
			if option.Current == "" && attr(choice, "selected") != "" {
				option.Current = name
			}
		}
	}
	return option
}
