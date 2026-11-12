// Package classes reads the public USC Schedule of Classes.
package classes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	http "github.com/saucesteals/fhttp"
)

const courseURL = "https://classes.usc.edu/api/Courses/Course"

// Client reads the public USC Schedule of Classes API.
type Client struct {
	http *http.Client
}

// New returns a public Schedule of Classes client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

// Course is a course and all of its scheduled sections for one term.
type Course struct {
	SchoolRestrictions      []Restriction       `json:"school_restrictions"`
	MajorRestrictions       []Restriction       `json:"major_restrictions"`
	CourseRestrictions      []Restriction       `json:"course_restrictions"`
	CorequisiteCourseCodes  []CourseRequirement `json:"corequisite_course_codes"`
	PrerequisiteCourseCodes []CourseRequirement `json:"prerequisite_course_codes"`
	GECode                  string              `json:"ge_code"`
	RecommendedPrep         string              `json:"recommended_prep"`
	DuplicateCredit         string              `json:"duplicate_credit"`
	TermNotes               string              `json:"term_notes"`
	CourseNotes             string              `json:"course_notes"`
	MaxUnits                *string             `json:"max_units"`
	IsCrossListed           bool                `json:"is_cross_listed"`
	Sequence                string              `json:"sequence"`
	Suffix                  string              `json:"suffix"`
	PublishedCourseCode     CourseCode          `json:"published_course_code"`
	ScheduledCourseCode     CourseCode          `json:"scheduled_course_code"`
	Code                    string              `json:"code"`
	Title                   string              `json:"title"`
	Description             string              `json:"description"`
	TermCode                string              `json:"term_code"`
	Units                   []float64           `json:"units"`
	RemainingSeats          int                 `json:"remaining_seats"`
	SeatCount               int                 `json:"seat_count"`
	Sections                []Section           `json:"sections"`
}

// Section is one scheduled lecture, lab, quiz, or other class section.
type Section struct {
	SessionCode     string       `json:"session_code"`
	Clearance       string       `json:"clearance"`
	RNRModeCode     string       `json:"rnr_mode_code"`
	Description     string       `json:"description"`
	Notes           string       `json:"notes"`
	Name            string       `json:"name"`
	LinkCode        string       `json:"link_code"`
	ID              int          `json:"id"`
	Type            string       `json:"type"`
	Cancelled       bool         `json:"cancelled"`
	DClearance      bool         `json:"d_clearance"`
	TotalSeats      int          `json:"total_seats"`
	RegisteredSeats int          `json:"registered_seats"`
	WaitlistedSeats *int         `json:"waitlisted_seats"`
	OpenSeats       int          `json:"open_seats"`
	Full            bool         `json:"full"`
	Units           []string     `json:"units"`
	Schedule        []Meeting    `json:"schedule"`
	Instructors     []Instructor `json:"instructors"`
	Syllabus        string       `json:"syllabus"`
}

// Meeting is one recurring meeting time for a section.
type Meeting struct {
	SectionLocationSequenceNumber int      `json:"section_location_sequence_number"`
	DayCode                       string   `json:"day_code"`
	Days                          []string `json:"days"`
	StartTime                     string   `json:"start_time"`
	EndTime                       string   `json:"end_time"`
}

// Instructor is a section instructor.
type Instructor struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type courseResponse struct {
	SchoolRestrictions      []restrictionResponse       `json:"schoolRestrictions"`
	MajorRestrictions       []restrictionResponse       `json:"majorRestrictions"`
	CourseRestrictions      []restrictionResponse       `json:"courseRestrictions"`
	CorequisiteCourseCodes  []courseRequirementResponse `json:"corequisiteCourseCodes"`
	PrerequisiteCourseCodes []courseRequirementResponse `json:"prerequisiteCourseCodes"`
	GECode                  string                      `json:"geCode"`
	RecommendedPrep         string                      `json:"recommendedPrep"`
	DuplicateCredit         string                      `json:"duplicateCredit"`
	TermNotes               string                      `json:"termNotes"`
	CourseNotes             string                      `json:"courseNotes"`
	MaxUnits                *string                     `json:"maxUnits"`
	IsCrossListed           bool                        `json:"isCrossListed"`
	Sequence                string                      `json:"sequence"`
	Suffix                  string                      `json:"suffix"`
	PublishedCourseCode     courseCodeResponse          `json:"publishedCourseCode"`
	ScheduledCourseCode     courseCodeResponse          `json:"scheduledCourseCode"`
	FullCourseName          string                      `json:"fullCourseName"`
	Name                    string                      `json:"name"`
	Description             string                      `json:"description"`
	TermCode                int                         `json:"termCode"`
	CourseUnits             []float64                   `json:"courseUnits"`
	RemainingSectionSeats   int                         `json:"remainingSectionSeats"`
	SectionSeatCount        int                         `json:"sectionSeatCount"`
	Sections                []sectionResponse           `json:"sections"`
}

type sectionResponse struct {
	Session struct {
		RNRSessionCode string `json:"rnrSessionCode"`
	} `json:"session"`
	RNRModeCode     string               `json:"rnrModeCode"`
	Description     string               `json:"description"`
	Notes           string               `json:"notes"`
	Name            string               `json:"name"`
	LinkCode        string               `json:"linkCode"`
	SISSectionID    string               `json:"sisSectionId"`
	RNRMode         string               `json:"rnrMode"`
	IsCancelled     bool                 `json:"isCancelled"`
	HasDClearance   bool                 `json:"hasDClearance"`
	TotalSeats      int                  `json:"totalSeats"`
	RegisteredSeats int                  `json:"registeredSeats"`
	WaitlistedSeats *int                 `json:"waitlistedSeats"`
	IsFull          bool                 `json:"isFull"`
	Units           []string             `json:"units"`
	Schedule        []meetingResponse    `json:"schedule"`
	Instructors     []instructorResponse `json:"instructors"`
	Syllabus        string               `json:"syllabus"`
}

type meetingResponse struct {
	SectionLocationSequenceNumber int      `json:"sectionLocationSqNumber"`
	DayCode                       string   `json:"dayCode"`
	Days                          []string `json:"days"`
	StartTime                     string   `json:"startTime"`
	EndTime                       string   `json:"endTime"`
}

type instructorResponse struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

// Course returns courseCode and all of its sections in termCode.
func (c *Client) Course(ctx context.Context, termCode, courseCode string) (Course, error) {
	termCode = strings.TrimSpace(termCode)
	if len(termCode) != 5 || strings.Trim(termCode, "0123456789") != "" {
		return Course{}, errors.New("term code must be five digits, such as 20271")
	}
	courseCode = strings.ToUpper(strings.TrimSpace(courseCode))
	if courseCode == "" {
		return Course{}, errors.New("course code must not be empty")
	}

	requestURL := courseURL + "?" + url.Values{
		"termCode":   {termCode},
		"courseCode": {courseCode},
	}.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return Course{}, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return Course{}, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return Course{}, fmt.Errorf("course %s was not found in term %s", courseCode, termCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 200))
		return Course{}, fmt.Errorf("classes course: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var wire courseResponse
	if err := json.NewDecoder(response.Body).Decode(&wire); err != nil {
		return Course{}, fmt.Errorf("classes course: decode response: %w", err)
	}
	return normalizeCourse(wire), nil
}

func normalizeCourse(wire courseResponse) Course {
	course := Course{
		SchoolRestrictions:      normalizeRestrictions(wire.SchoolRestrictions),
		MajorRestrictions:       normalizeRestrictions(wire.MajorRestrictions),
		CourseRestrictions:      normalizeRestrictions(wire.CourseRestrictions),
		CorequisiteCourseCodes:  normalizeRequirements(wire.CorequisiteCourseCodes),
		PrerequisiteCourseCodes: normalizeRequirements(wire.PrerequisiteCourseCodes),
		GECode:                  wire.GECode,
		RecommendedPrep:         wire.RecommendedPrep,
		DuplicateCredit:         wire.DuplicateCredit,
		TermNotes:               wire.TermNotes,
		CourseNotes:             wire.CourseNotes,
		MaxUnits:                wire.MaxUnits,
		IsCrossListed:           wire.IsCrossListed,
		Sequence:                wire.Sequence,
		Suffix:                  wire.Suffix,
		PublishedCourseCode:     CourseCode(wire.PublishedCourseCode),
		ScheduledCourseCode:     CourseCode(wire.ScheduledCourseCode),
		Code:                    wire.ScheduledCourseCode.CourseHyphen,
		Title:                   wire.Name,
		Description:             wire.Description,
		TermCode:                strconv.Itoa(wire.TermCode),
		Units:                   append([]float64(nil), wire.CourseUnits...),
		RemainingSeats:          wire.RemainingSectionSeats,
		SeatCount:               wire.SectionSeatCount,
		Sections:                make([]Section, len(wire.Sections)),
	}
	for index, wireSection := range wire.Sections {
		section := Section{
			SessionCode:     wireSection.Session.RNRSessionCode,
			Clearance:       "R",
			RNRModeCode:     wireSection.RNRModeCode,
			Description:     wireSection.Description,
			Notes:           wireSection.Notes,
			Name:            wireSection.Name,
			LinkCode:        wireSection.LinkCode,
			ID:              sectionID(wireSection.SISSectionID),
			Type:            wireSection.RNRMode,
			Cancelled:       wireSection.IsCancelled,
			DClearance:      wireSection.HasDClearance,
			TotalSeats:      wireSection.TotalSeats,
			RegisteredSeats: wireSection.RegisteredSeats,
			WaitlistedSeats: wireSection.WaitlistedSeats,
			OpenSeats:       max(wireSection.TotalSeats-wireSection.RegisteredSeats, 0),
			Full:            wireSection.IsFull,
			Units:           append([]string(nil), wireSection.Units...),
			Schedule:        make([]Meeting, len(wireSection.Schedule)),
			Instructors:     make([]Instructor, len(wireSection.Instructors)),
			Syllabus:        wireSection.Syllabus,
		}
		if section.DClearance {
			section.Clearance = "D"
		}
		for meetingIndex, wireMeeting := range wireSection.Schedule {
			section.Schedule[meetingIndex] = Meeting{
				SectionLocationSequenceNumber: wireMeeting.SectionLocationSequenceNumber,
				DayCode:                       wireMeeting.DayCode,
				Days:                          append([]string(nil), wireMeeting.Days...),
				StartTime:                     wireMeeting.StartTime,
				EndTime:                       wireMeeting.EndTime,
			}
		}
		for instructorIndex, wireInstructor := range wireSection.Instructors {
			section.Instructors[instructorIndex] = Instructor(wireInstructor)
		}
		course.Sections[index] = section
	}
	return course
}

func firstField(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

type CourseCode struct {
	Prefix       string `json:"prefix"`
	Number       string `json:"number"`
	Suffix       string `json:"suffix"`
	CourseHyphen string `json:"course_hyphen"`
}
type courseCodeResponse struct {
	Prefix       string
	Number       string
	Suffix       string
	CourseHyphen string
}

func sectionID(value string) int { id, _ := strconv.Atoi(firstField(value)); return id }

type Restriction struct {
	GroupCode        string `json:"group_code"`
	GroupDescription string `json:"group_description"`
	RestrictionCode  string `json:"restriction_code"`
	Restriction      string `json:"restriction"`
}
type restrictionResponse struct {
	GroupCode        string
	GroupDescription string
	RestrictionCode  string
	Restriction      string
}
type CourseRequirement struct {
	Order           int            `json:"order"`
	RequiredCourses int            `json:"required_courses"`
	CourseOptions   []CourseOption `json:"course_options"`
}
type CourseOption struct {
	CourseCode
	Ands []CourseOption `json:"ands"`
}
type courseRequirementResponse struct {
	Order           int
	RequiredCourses int
	CourseOptions   []courseOptionResponse
}
type courseOptionResponse struct {
	courseCodeResponse
	Ands []courseOptionResponse
}

func normalizeRestrictions(w []restrictionResponse) []Restriction {
	r := make([]Restriction, len(w))
	for i, v := range w {
		r[i] = Restriction(v)
	}
	return r
}
func normalizeOptions(w []courseOptionResponse) []CourseOption {
	r := make([]CourseOption, len(w))
	for i, v := range w {
		r[i] = CourseOption{CourseCode(v.courseCodeResponse), normalizeOptions(v.Ands)}
	}
	return r
}
func normalizeRequirements(w []courseRequirementResponse) []CourseRequirement {
	r := make([]CourseRequirement, len(w))
	for i, v := range w {
		r[i] = CourseRequirement{v.Order, v.RequiredCourses, normalizeOptions(v.CourseOptions)}
	}
	return r
}
