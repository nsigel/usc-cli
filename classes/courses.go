package classes

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Course is a course offered in one term and all of its sections.
type Course struct {
	TermCode string `json:"term_code"`
	// Code is the code the course is scheduled under. With TermCode it
	// identifies the course, and it is the code its sections belong to.
	Code CourseCode `json:"code"`
	// PublishedCode is the code the course was listed under where it was
	// read. It differs from Code only for a cross-listed course: TAC-368 is
	// also published as CSCI-368 in the CSCI program.
	PublishedCode CourseCode `json:"published_code"`
	IsCrossListed bool       `json:"is_cross_listed"`
	// Suffix is the catalog letters USC prints after the code, such as "L"
	// for a course with a lab or "g" for general education. It describes
	// the course and is not part of its code.
	Suffix string `json:"suffix,omitempty"`

	Title       string `json:"title"`
	Description string `json:"description"`
	// Units lists the unit values the course can be taken for: one value
	// for most courses, several for variable-unit courses.
	Units []float64 `json:"units"`

	Prerequisites []CourseRequirement `json:"prerequisites"`
	Corequisites  []CourseRequirement `json:"corequisites"`
	// Restrictions limit who may register, by class standing or degree
	// level (course), major, or school.
	CourseRestrictions []Restriction `json:"course_restrictions"`
	MajorRestrictions  []Restriction `json:"major_restrictions"`
	SchoolRestrictions []Restriction `json:"school_restrictions"`

	Notes                  string `json:"notes,omitempty"`
	TermNotes              string `json:"term_notes,omitempty"`
	DuplicateCredit        string `json:"duplicate_credit,omitempty"`
	RecommendedPreparation string `json:"recommended_preparation,omitempty"`
	GeneralEducationCode   string `json:"general_education_code,omitempty"`

	Sections []Section `json:"sections"`
}

// CourseRequirement is one prerequisite or corequisite: RequiredCount of its
// Options must be completed.
type CourseRequirement struct {
	RequiredCount int                       `json:"required_count"`
	Options       []CourseRequirementOption `json:"options"`
}

// CourseRequirementOption is one way to meet a requirement: taking every
// course in Courses. Most options are a single course.
type CourseRequirementOption struct {
	Courses []CourseCode `json:"courses"`
}

// Restriction opens a course only to a group of students, or closes it to
// them.
type Restriction struct {
	// GroupCode is USC's code for the group: a class standing such as "U1",
	// a major such as "CSCI", or a school.
	GroupCode string `json:"group_code"`
	// GroupName is the group in words: "Freshman", "Computer Science".
	GroupName string `json:"group_name"`
	// IsExcluded is true when the group may not register, and false when
	// only the listed groups may.
	IsExcluded bool `json:"is_excluded"`
}

// Section is one scheduled lecture, discussion, lab, quiz, or other part of a
// course that a student registers for.
type Section struct {
	// ID is USC's five-digit section number, the number students register
	// with: "30009".
	ID string `json:"id"`
	// Type is the kind of section as USC names it: "Lecture", "Discussion",
	// "Lab", "Quiz", "Lecture-Lab", ...
	Type string `json:"type"`
	// LinkCode groups the sections of a course that must be taken together.
	LinkCode string `json:"link_code,omitempty"`
	// SessionCode is USC's session within the term: "001" is the regular
	// session; others have their own start, end, and drop dates.
	SessionCode string `json:"session_code"`
	// Title is set when the section has its own title, as special-topics
	// courses do.
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Notes       string `json:"notes,omitempty"`
	// Units lists the unit values the section can be taken for; 0 for a
	// section such as a discussion that carries no units of its own.
	Units []float64 `json:"units"`

	// RequiresDepartmentClearance is true for "D" sections, which a
	// department must clear a student for before registration.
	RequiresDepartmentClearance bool `json:"requires_department_clearance"`
	IsCancelled                 bool `json:"is_cancelled"`
	SeatCapacity                int  `json:"seat_capacity"`
	RegisteredSeatCount         int  `json:"registered_seat_count"`
	// WaitlistedSeatCount is nil for a section with no waitlist.
	WaitlistedSeatCount *int `json:"waitlisted_seat_count"`

	// Meetings is empty when the section's time is to be announced.
	Meetings    []Meeting    `json:"meetings"`
	Instructors []Instructor `json:"instructors"`
	SyllabusURL string       `json:"syllabus_url,omitempty"`
}

// Meeting is one recurring weekly meeting time of a section.
type Meeting struct {
	// Weekdays are in week order starting Monday.
	Weekdays []Weekday `json:"weekdays"`
	// StartTime and EndTime are 24-hour campus time: "14:00".
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	// Location is the building and room, such as "THH201". The public
	// Schedule of Classes does not publish it; WebReg does.
	Location string `json:"location,omitempty"`
}

// Instructor is a person teaching a section.
type Instructor struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// ProgramCourses returns every course a program offers in a term, with all
// sections. USC only answers when the program is named together with a
// school it is listed under (see Schools).
func (c *Client) ProgramCourses(ctx context.Context, termCode, schoolPrefix, programPrefix string) ([]Course, error) {
	if err := validateTermCode(termCode); err != nil {
		return nil, err
	}
	if schoolPrefix == "" || programPrefix == "" {
		return nil, fmt.Errorf("a school prefix and a program prefix are required")
	}
	var wire struct {
		Courses []courseResponse `json:"courses"`
	}
	query := url.Values{
		"termCode": {termCode},
		"school":   {strings.ToUpper(schoolPrefix)},
		"program":  {strings.ToUpper(programPrefix)},
	}
	if err := c.get(ctx, "Courses/CoursesByTermSchoolProgram", query, &wire); err != nil {
		return nil, err
	}
	return normalizeCourses(wire.Courses)
}

// Course returns one course and all of its sections. A cross-listed course
// can be asked for by any of its codes; the result's Code is the one it is
// scheduled under.
func (c *Client) Course(ctx context.Context, termCode string, code CourseCode) (Course, error) {
	if err := validateTermCode(termCode); err != nil {
		return Course{}, err
	}
	var wire courseResponse
	query := url.Values{
		"termCode":   {termCode},
		"courseCode": {code.String()},
	}
	if err := c.get(ctx, "Courses/Course", query, &wire); err != nil {
		return Course{}, err
	}
	return normalizeCourse(wire)
}

type courseResponse struct {
	TermCode            int                `json:"termCode"`
	ScheduledCourseCode courseCodeResponse `json:"scheduledCourseCode"`
	PublishedCourseCode courseCodeResponse `json:"publishedCourseCode"`
	// Suffix is the catalog letters ("L", "gp"). The suffix inside a course
	// code is something else: its sequence letter.
	Suffix                  string                `json:"suffix"`
	IsCrossListed           bool                  `json:"isCrossListed"`
	Name                    string                `json:"name"`
	Description             string                `json:"description"`
	CourseUnits             []float64             `json:"courseUnits"`
	PrerequisiteCourseCodes []requirementResponse `json:"prerequisiteCourseCodes"`
	CorequisiteCourseCodes  []requirementResponse `json:"corequisiteCourseCodes"`
	CourseRestrictions      []restrictionResponse `json:"courseRestrictions"`
	MajorRestrictions       []restrictionResponse `json:"majorRestrictions"`
	SchoolRestrictions      []restrictionResponse `json:"schoolRestrictions"`
	CourseNotes             string                `json:"courseNotes"`
	TermNotes               string                `json:"termNotes"`
	DuplicateCredit         string                `json:"duplicateCredit"`
	RecommendedPrep         string                `json:"recommendedPrep"`
	GECode                  string                `json:"geCode"`
	Sections                []sectionResponse     `json:"sections"`
}

type courseCodeResponse struct {
	Prefix string `json:"prefix"`
	Number string `json:"number"`
	Suffix string `json:"suffix"`
}

func (wire courseCodeResponse) courseCode() CourseCode {
	return CourseCode{Prefix: wire.Prefix, Number: wire.Number, Sequence: wire.Suffix}
}

type requirementResponse struct {
	RequiredCourses int                         `json:"requiredCourses"`
	CourseOptions   []requirementOptionResponse `json:"courseOptions"`
}

// requirementOptionResponse is a course and, in Ands, the courses that must
// be taken along with it for the option to count.
type requirementOptionResponse struct {
	courseCodeResponse
	Ands []requirementOptionResponse `json:"ands"`
}

type restrictionResponse struct {
	GroupCode        string `json:"groupCode"`
	GroupDescription string `json:"groupDescription"`
	// RestrictionCode is "I" (inclusive: only this group) or "E"
	// (exclusive: not this group).
	RestrictionCode string `json:"restrictionCode"`
}

type sectionResponse struct {
	SISSectionID string `json:"sisSectionId"`
	RNRMode      string `json:"rnrMode"`
	LinkCode     string `json:"linkCode"`
	Session      struct {
		RNRSessionCode string `json:"rnrSessionCode"`
	} `json:"session"`
	Name            string               `json:"name"`
	Description     string               `json:"description"`
	Notes           string               `json:"notes"`
	Units           []string             `json:"units"`
	HasDClearance   bool                 `json:"hasDClearance"`
	IsCancelled     bool                 `json:"isCancelled"`
	TotalSeats      int                  `json:"totalSeats"`
	RegisteredSeats int                  `json:"registeredSeats"`
	WaitlistedSeats *int                 `json:"waitlistedSeats"`
	Schedule        []meetingResponse    `json:"schedule"`
	Instructors     []instructorResponse `json:"instructors"`
	Syllabus        string               `json:"syllabus"`
}

type meetingResponse struct {
	Days []string `json:"days"`
	// StartTime and EndTime are "HH:MM", or "TBA".
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

type instructorResponse struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

func normalizeCourses(wire []courseResponse) ([]Course, error) {
	courses := make([]Course, len(wire))
	for index, wireCourse := range wire {
		course, err := normalizeCourse(wireCourse)
		if err != nil {
			return nil, err
		}
		courses[index] = course
	}
	return courses, nil
}

func normalizeCourse(wire courseResponse) (Course, error) {
	course := Course{
		TermCode:               strconv.Itoa(wire.TermCode),
		Code:                   wire.ScheduledCourseCode.courseCode(),
		PublishedCode:          wire.PublishedCourseCode.courseCode(),
		IsCrossListed:          wire.IsCrossListed,
		Suffix:                 wire.Suffix,
		Title:                  wire.Name,
		Description:            wire.Description,
		Units:                  append([]float64{}, wire.CourseUnits...),
		Prerequisites:          normalizeRequirements(wire.PrerequisiteCourseCodes),
		Corequisites:           normalizeRequirements(wire.CorequisiteCourseCodes),
		CourseRestrictions:     normalizeRestrictions(wire.CourseRestrictions),
		MajorRestrictions:      normalizeRestrictions(wire.MajorRestrictions),
		SchoolRestrictions:     normalizeRestrictions(wire.SchoolRestrictions),
		Notes:                  wire.CourseNotes,
		TermNotes:              wire.TermNotes,
		DuplicateCredit:        wire.DuplicateCredit,
		RecommendedPreparation: wire.RecommendedPrep,
		GeneralEducationCode:   wire.GECode,
		Sections:               make([]Section, len(wire.Sections)),
	}
	for index, wireSection := range wire.Sections {
		section, err := normalizeSection(wireSection)
		if err != nil {
			return Course{}, fmt.Errorf("course %s: %w", course.Code, err)
		}
		course.Sections[index] = section
	}
	return course, nil
}

func normalizeRequirements(wire []requirementResponse) []CourseRequirement {
	requirements := make([]CourseRequirement, len(wire))
	for requirementIndex, wireRequirement := range wire {
		requirement := CourseRequirement{
			RequiredCount: wireRequirement.RequiredCourses,
			Options:       make([]CourseRequirementOption, len(wireRequirement.CourseOptions)),
		}
		for optionIndex, wireOption := range wireRequirement.CourseOptions {
			requirement.Options[optionIndex] = CourseRequirementOption{Courses: requirementOptionCourses(wireOption)}
		}
		requirements[requirementIndex] = requirement
	}
	return requirements
}

func requirementOptionCourses(wire requirementOptionResponse) []CourseCode {
	courses := []CourseCode{wire.courseCode()}
	for _, alsoRequired := range wire.Ands {
		courses = append(courses, requirementOptionCourses(alsoRequired)...)
	}
	return courses
}

func normalizeRestrictions(wire []restrictionResponse) []Restriction {
	restrictions := make([]Restriction, len(wire))
	for index, wireRestriction := range wire {
		restrictions[index] = Restriction{
			GroupCode:  wireRestriction.GroupCode,
			GroupName:  wireRestriction.GroupDescription,
			IsExcluded: wireRestriction.RestrictionCode == "E",
		}
	}
	return restrictions
}

func normalizeSection(wire sectionResponse) (Section, error) {
	sectionID, err := ParseSectionID(wire.SISSectionID)
	if err != nil {
		return Section{}, err
	}
	units, err := parseUnits(wire.Units)
	if err != nil {
		return Section{}, fmt.Errorf("section %s: %w", sectionID, err)
	}
	meetings, err := normalizeMeetings(wire.Schedule)
	if err != nil {
		return Section{}, fmt.Errorf("section %s: %w", sectionID, err)
	}
	section := Section{
		ID:                          sectionID,
		Type:                        wire.RNRMode,
		LinkCode:                    wire.LinkCode,
		SessionCode:                 wire.Session.RNRSessionCode,
		Title:                       wire.Name,
		Description:                 wire.Description,
		Notes:                       wire.Notes,
		Units:                       units,
		RequiresDepartmentClearance: wire.HasDClearance,
		IsCancelled:                 wire.IsCancelled,
		SeatCapacity:                wire.TotalSeats,
		RegisteredSeatCount:         wire.RegisteredSeats,
		WaitlistedSeatCount:         wire.WaitlistedSeats,
		Meetings:                    meetings,
		Instructors:                 make([]Instructor, len(wire.Instructors)),
		SyllabusURL:                 wire.Syllabus,
	}
	for index, wireInstructor := range wire.Instructors {
		section.Instructors[index] = Instructor(wireInstructor)
	}
	return section, nil
}

// ParseSectionID reads a section number as USC prints it, alone ("30009") or
// followed by its clearance letter ("30009 R"), and returns the five digits.
func ParseSectionID(text string) (string, error) {
	fields := strings.Fields(text)
	if len(fields) == 0 || len(fields[0]) != 5 || strings.Trim(fields[0], "0123456789") != "" {
		return "", fmt.Errorf("section ID must be five digits such as 30009, got %q", text)
	}
	return fields[0], nil
}

func parseUnits(wire []string) ([]float64, error) {
	units := make([]float64, len(wire))
	for index, text := range wire {
		value, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, fmt.Errorf("unreadable units %q", text)
		}
		units[index] = value
	}
	return units, nil
}

var weekdaysByAbbreviation = map[string]Weekday{
	"Mon": Weekday(1), "Tue": Weekday(2), "Wed": Weekday(3), "Thu": Weekday(4),
	"Fri": Weekday(5), "Sat": Weekday(6), "Sun": Weekday(0),
}

// normalizeMeetings drops the placeholder USC lists for a section whose time
// is to be announced, which has no days and "TBA" for both times.
func normalizeMeetings(wire []meetingResponse) ([]Meeting, error) {
	meetings := []Meeting{}
	for _, wireMeeting := range wire {
		if len(wireMeeting.Days) == 0 && wireMeeting.StartTime == "TBA" {
			continue
		}
		meeting := Meeting{
			Weekdays:  make([]Weekday, len(wireMeeting.Days)),
			StartTime: clockTime(wireMeeting.StartTime),
			EndTime:   clockTime(wireMeeting.EndTime),
		}
		for index, abbreviation := range wireMeeting.Days {
			weekday, known := weekdaysByAbbreviation[abbreviation]
			if !known {
				return nil, fmt.Errorf("unknown meeting day %q", abbreviation)
			}
			meeting.Weekdays[index] = weekday
		}
		meetings = append(meetings, meeting)
	}
	return meetings, nil
}

func clockTime(wire string) string {
	if wire == "TBA" {
		return ""
	}
	return wire
}
