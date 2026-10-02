// Package classes implements the public USC Schedule of Classes contract.
package classes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
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
	Code           string    `json:"code"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	TermCode       int       `json:"term_code"`
	Units          []float64 `json:"units"`
	RemainingSeats int       `json:"remaining_seats"`
	SeatCount      int       `json:"seat_count"`
	Sections       []Section `json:"sections"`
}

// Section is one scheduled lecture, lab, quiz, or other class section.
type Section struct {
	ID              string       `json:"id"`
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
	DayCode   string   `json:"day_code"`
	Days      []string `json:"days"`
	StartTime string   `json:"start_time"`
	EndTime   string   `json:"end_time"`
}

// Instructor is a section instructor.
type Instructor struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type courseResponse struct {
	FullCourseName        string            `json:"fullCourseName"`
	Name                  string            `json:"name"`
	Description           string            `json:"description"`
	TermCode              int               `json:"termCode"`
	CourseUnits           []float64         `json:"courseUnits"`
	RemainingSectionSeats int               `json:"remainingSectionSeats"`
	SectionSeatCount      int               `json:"sectionSeatCount"`
	Sections              []sectionResponse `json:"sections"`
}

type sectionResponse struct {
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
	DayCode   string   `json:"dayCode"`
	Days      []string `json:"days"`
	StartTime string   `json:"startTime"`
	EndTime   string   `json:"endTime"`
}

type instructorResponse struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

// Course returns courseCode and all of its sections in termCode.
func (c *Client) Course(ctx context.Context, termCode, courseCode string) (Course, error) {
	termCode = strings.TrimSpace(termCode)
	if len(termCode) != 5 || strings.Trim(termCode, "0123456789") != "" {
		return Course{}, errors.New("TERM_CODE must be a five-digit term code")
	}
	courseCode = strings.ToUpper(strings.TrimSpace(courseCode))
	if courseCode == "" {
		return Course{}, errors.New("COURSE_CODE must not be empty")
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
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 201))
		if readErr != nil {
			return Course{}, readErr
		}
		return Course{}, fmt.Errorf("classes course: HTTP %d — %s", response.StatusCode, truncate(strings.TrimSpace(string(body)), 200))
	}

	var wire courseResponse
	if err := json.NewDecoder(response.Body).Decode(&wire); err != nil {
		return Course{}, fmt.Errorf("classes course: decode response: %w", err)
	}
	return normalizeCourse(wire), nil
}

func normalizeCourse(wire courseResponse) Course {
	course := Course{
		Code:           wire.FullCourseName,
		Title:          wire.Name,
		Description:    wire.Description,
		TermCode:       wire.TermCode,
		Units:          append([]float64(nil), wire.CourseUnits...),
		RemainingSeats: wire.RemainingSectionSeats,
		SeatCount:      wire.SectionSeatCount,
		Sections:       make([]Section, len(wire.Sections)),
	}
	for index, wireSection := range wire.Sections {
		openSeats := wireSection.TotalSeats - wireSection.RegisteredSeats
		if openSeats < 0 {
			openSeats = 0
		}
		section := Section{
			ID:              firstField(wireSection.SISSectionID),
			Type:            wireSection.RNRMode,
			Cancelled:       wireSection.IsCancelled,
			DClearance:      wireSection.HasDClearance,
			TotalSeats:      wireSection.TotalSeats,
			RegisteredSeats: wireSection.RegisteredSeats,
			WaitlistedSeats: wireSection.WaitlistedSeats,
			OpenSeats:       openSeats,
			Full:            wireSection.IsFull,
			Units:           append([]string(nil), wireSection.Units...),
			Schedule:        make([]Meeting, len(wireSection.Schedule)),
			Instructors:     make([]Instructor, len(wireSection.Instructors)),
			Syllabus:        wireSection.Syllabus,
		}
		for meetingIndex, wireMeeting := range wireSection.Schedule {
			section.Schedule[meetingIndex] = Meeting{
				DayCode:   wireMeeting.DayCode,
				Days:      append([]string(nil), wireMeeting.Days...),
				StartTime: wireMeeting.StartTime,
				EndTime:   wireMeeting.EndTime,
			}
		}
		for instructorIndex, wireInstructor := range wireSection.Instructors {
			section.Instructors[instructorIndex] = Instructor{
				FirstName: wireInstructor.FirstName,
				LastName:  wireInstructor.LastName,
			}
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

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
