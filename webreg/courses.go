package webreg

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nsigel/usc-cli/classes"
	"github.com/nsigel/usc-cli/internal/dom"
	"golang.org/x/net/html"
)

// Course is a WebReg course and the sections shown for it on one page.
type Course struct {
	Code          classes.CourseCode `json:"code"`
	Title         string             `json:"title"`
	Description   string             `json:"description,omitempty"`
	Prerequisites []string           `json:"prerequisites,omitempty"`
	Sections      []Section          `json:"sections"`
}

// Section is one WebReg section row. It names what it shares with
// classes.Section the same way, so the two can be compared field by field.
// Fields that only one page shows are omitted elsewhere: InCourseBin comes
// from catalog pages, CourseBin and GradeOption from the course bin.
type Section struct {
	// ID is USC's five-digit section number: "30009".
	ID          string `json:"id"`
	Type        string `json:"type,omitempty"`
	SessionCode string `json:"session_code,omitempty"`
	// Units is the section's units as WebReg prints them: "4.0".
	Units string `json:"units,omitempty"`

	// RequiresDepartmentClearance is true for "D" sections; "R" sections
	// are open to registration.
	RequiresDepartmentClearance bool `json:"requires_department_clearance"`
	// IsClosed is true when WebReg shows "Closed" in place of seat counts;
	// SeatCapacity and RegisteredSeatCount are then zero.
	IsClosed            bool `json:"is_closed"`
	SeatCapacity        int  `json:"seat_capacity"`
	RegisteredSeatCount int  `json:"registered_seat_count"`

	// Meetings carry each meeting's Location, which the public Schedule of
	// Classes does not publish.
	Meetings    []classes.Meeting    `json:"meetings"`
	Instructors []classes.Instructor `json:"instructors"`
	Notes       []string             `json:"notes,omitempty"`

	InCourseBin *bool        `json:"in_course_bin,omitempty"`
	CourseBin   *BinState    `json:"course_bin,omitempty"`
	GradeOption *GradeOption `json:"grade_option,omitempty"`
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

func parseCourses(document *html.Node) ([]Course, error) {
	courses := []Course{}
	for _, header := range dom.FindAll(document, dom.Class("div", "course-header")) {
		course, err := parseCourse(header)
		if err != nil {
			return nil, err
		}
		courses = append(courses, course)
	}
	return courses, nil
}

// parseCourse reads one course header and the body that follows it. The body
// is the header's next element rather than a lookup by ID: checkout lists a
// course once per action, with the same ID each time.
func parseCourse(header *html.Node) (Course, error) {
	// WebReg prints the code with its catalog suffix and a colon: "BUAD-200 x:".
	printedCode := strings.TrimSuffix(text(dom.FindFirst(header, dom.Class("span", "crsID"))), ":")
	code, err := classes.ParseCourseCode(printedCode)
	if err != nil {
		return Course{}, fmt.Errorf("WebReg course header: %w", err)
	}
	course := Course{
		Code:     code,
		Title:    text(dom.FindFirst(header, dom.Class("span", "crsTitl"))),
		Sections: []Section{},
	}
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
		section, err := parseSection(row)
		if err != nil {
			return Course{}, fmt.Errorf("WebReg course %s: %w", code, err)
		}
		course.Sections = append(course.Sections, section)
	}
	return course, nil
}

func parseSection(row *html.Node) (Section, error) {
	sectionID, err := classes.ParseSectionID(strings.TrimPrefix(dom.Attr(row, "id"), "section_"))
	if err != nil {
		return Section{}, err
	}
	section := Section{ID: sectionID, Instructors: []classes.Instructor{}}
	var times, days, locations []string
	for _, column := range dom.FindAll(row, dom.Class("span", "section_row")) {
		label := dom.FindFirst(column, dom.Class("span", "table-headers-xsmall"))
		if label == nil {
			continue
		}
		switch strings.TrimSuffix(text(label), ":") {
		case "Section":
			// The section number is followed by its clearance letter: "30009 D".
			printed := strings.Fields(text(dom.FindFirst(column, dom.Tag("b"))))
			section.RequiresDepartmentClearance = len(printed) > 1 && printed[1] == "D"
		case "Session":
			section.SessionCode = columnText(column, label)
		case "Type":
			section.Type = columnText(column, label)
		case "Units":
			section.Units = columnText(column, label)
		case "Registered":
			if err := section.parseSeats(columnText(column, label)); err != nil {
				return Section{}, fmt.Errorf("section %s: %w", sectionID, err)
			}
		case "Time":
			times = columnValues(column, label)
		case "Days":
			days = columnValues(column, label)
		case "Location":
			locations = columnValues(column, label)
		case "Instructor":
			for _, printedName := range columnValues(column, label) {
				section.Instructors = append(section.Instructors, parseInstructor(printedName))
			}
		case "Action":
			section.CourseBin = parseBinState(column, sectionID)
		case "Grade Option":
			section.GradeOption = parseGradeOption(column, label)
		}
	}
	section.Meetings, err = parseMeetings(times, days, locations)
	if err != nil {
		return Section{}, fmt.Errorf("section %s: %w", sectionID, err)
	}
	if form := dom.FindFirst(row, dom.Class("div", "btnAddToMyCourseBin")); form != nil {
		inBin := dom.FindFirst(form, dom.Class("label", "alrdyinmycb")) != nil
		section.InCourseBin = &inBin
	}
	for _, note := range dom.FindAll(row, dom.Class("div", "dvSRtxt")) {
		// Status notes have IDs and are toggled by the action bar; the
		// remaining callouts are section notes such as fees or locations.
		if dom.Attr(note, "id") == "" {
			if content := text(note); content != "" {
				section.Notes = append(section.Notes, content)
			}
		}
	}
	return section, nil
}

func parseInstructor(printedName string) classes.Instructor {
	lastName, firstName, _ := strings.Cut(printedName, ",")
	return classes.Instructor{
		FirstName: strings.TrimSpace(firstName),
		LastName:  strings.TrimSpace(lastName),
	}
}

// weekdaysByAbbreviation is how WebReg's Days column writes each day. It
// runs them together, as in "MW" and "TTh".
var weekdaysByAbbreviation = map[string]classes.Weekday{
	"M":  classes.Weekday(time.Monday),
	"T":  classes.Weekday(time.Tuesday),
	"W":  classes.Weekday(time.Wednesday),
	"Th": classes.Weekday(time.Thursday),
	"H":  classes.Weekday(time.Thursday),
	"F":  classes.Weekday(time.Friday),
	"Sa": classes.Weekday(time.Saturday),
	"S":  classes.Weekday(time.Saturday),
	"Su": classes.Weekday(time.Sunday),
	"U":  classes.Weekday(time.Sunday),
}

func parseWeekdays(printedDays string) ([]classes.Weekday, error) {
	weekdays := []classes.Weekday{}
	remaining := printedDays
	for remaining != "" {
		weekday, abbreviationLength, known := longestWeekdayAbbreviation(remaining)
		if !known {
			return nil, fmt.Errorf("unreadable meeting days %q", printedDays)
		}
		weekdays = append(weekdays, weekday)
		remaining = remaining[abbreviationLength:]
	}
	return weekdays, nil
}

func longestWeekdayAbbreviation(printedDays string) (weekday classes.Weekday, length int, known bool) {
	for length = min(2, len(printedDays)); length > 0; length-- {
		if weekday, known = weekdaysByAbbreviation[printedDays[:length]]; known {
			return weekday, length, true
		}
	}
	return 0, 0, false
}

// parseMeetings joins WebReg's Time, Days, and Location columns, which list
// one line per meeting in the same order. A section whose time is to be
// announced prints "TBA" and has no meetings.
func parseMeetings(times, days, locations []string) ([]classes.Meeting, error) {
	meetings := []classes.Meeting{}
	for index, printedTime := range times {
		if strings.EqualFold(printedTime, "TBA") {
			continue
		}
		start, end, found := strings.Cut(printedTime, "-")
		if !found {
			return nil, fmt.Errorf("unreadable meeting time %q", printedTime)
		}
		meeting := classes.Meeting{Weekdays: []classes.Weekday{}}
		var err error
		if meeting.StartTime, err = clockTime(start); err != nil {
			return nil, err
		}
		if meeting.EndTime, err = clockTime(end); err != nil {
			return nil, err
		}
		if index < len(days) {
			if meeting.Weekdays, err = parseWeekdays(days[index]); err != nil {
				return nil, err
			}
		}
		if index < len(locations) {
			meeting.Location = locations[index]
		}
		meetings = append(meetings, meeting)
	}
	return meetings, nil
}

func clockTime(printed string) (string, error) {
	parsed, err := time.Parse("03:04pm", strings.TrimSpace(printed))
	if err != nil {
		return "", fmt.Errorf("unreadable meeting time %q", printed)
	}
	return parsed.Format("15:04"), nil
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

func (section *Section) parseSeats(printed string) error {
	if strings.EqualFold(printed, "closed") {
		section.IsClosed = true
		return nil
	}
	registered, capacity, found := strings.Cut(printed, " of ")
	if !found {
		return fmt.Errorf("unreadable seat count %q", printed)
	}
	var err error
	if section.RegisteredSeatCount, err = strconv.Atoi(strings.TrimSpace(registered)); err != nil {
		return fmt.Errorf("unreadable seat count %q", printed)
	}
	if section.SeatCapacity, err = strconv.Atoi(strings.TrimSpace(capacity)); err != nil {
		return fmt.Errorf("unreadable seat count %q", printed)
	}
	return nil
}

// parseBinState finds which of the four pre-rendered action bars WebReg is
// showing for the section.
func parseBinState(column *html.Node, sectionID string) *BinState {
	for _, candidate := range []struct {
		barIDPrefix string
		state       BinState
	}{
		{"schedY_regN", BinState{Scheduled: true}},
		{"schedN_regN", BinState{}},
		{"schedY_regY", BinState{Scheduled: true, Registered: true}},
		{"schedN_regY", BinState{Registered: true}},
	} {
		bar := dom.FindFirst(column, dom.ID(candidate.barIDPrefix+"_"+sectionID))
		if bar != nil && !hidden(bar) {
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
