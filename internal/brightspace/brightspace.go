// Package brightspace implements Brightspace's Valence API contract.
package brightspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"

	http "github.com/saucesteals/fhttp"
)

const (
	baseURL   = "https://brightspace.usc.edu"
	lpVersion = "1.31"
	leVersion = "1.67"
)

var courseName = regexp.MustCompile(`^(\S+)\s+([A-Z]+-\d+[A-Z]*):\s+(.+)$`)
var htmlTag = regexp.MustCompile(`<[^>]+>`)

// Doer is the HTTP surface needed by the Brightspace API. It keeps command
// tests offline and lets authentication retain ownership of cookie handling.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client calls Brightspace with an authenticated browser session.
type Client struct {
	http Doer
}

// New creates a Brightspace API client from an authenticated HTTP client.
func New(httpClient Doer) *Client {
	return &Client{http: httpClient}
}

// Error describes a failed Brightspace API request.
type Error struct {
	Label  string
	Status int
	Body   string
}

func (e *Error) Error() string {
	switch e.Status {
	case 401:
		return "session expired or invalid — run `usc auth login brightspace`"
	case 403:
		return fmt.Sprintf("%s: access denied (403)", e.Label)
	default:
		return fmt.Sprintf("%s: HTTP %d — %s", e.Label, e.Status, e.Body)
	}
}

// User is Brightspace's current-user response. Brightspace fields vary by
// tenant, so this response is deliberately passed through unchanged.
type User map[string]any

// WhoAmI returns the authenticated Brightspace user.
func (c *Client) WhoAmI(ctx context.Context) (User, error) {
	var user User
	err := c.get(ctx, "/d2l/api/lp/"+lpVersion+"/users/whoami", nil, "whoami", &user)
	return user, err
}

// Course is a normalized Brightspace enrollment.
type Course struct {
	ID      int     `json:"id"`
	Code    *string `json:"code"`
	Title   string  `json:"title"`
	Section *string `json:"section"`
	Type    string  `json:"type"`
	Role    string  `json:"role"`
	RawName string  `json:"raw_name"`
}

// Courses returns course offerings, or all enrollment types when includeAll is true.
func (c *Client) Courses(ctx context.Context, includeAll bool) ([]Course, error) {
	path := "/d2l/api/lp/" + lpVersion + "/enrollments/myenrollments/"
	query := url.Values{"pageSize": {"100"}}
	var courses []Course
	for {
		var page enrollmentPage
		if err := c.get(ctx, path, query, "enrollments", &page); err != nil {
			return nil, err
		}
		for _, enrollment := range page.Items {
			course := normalizeCourse(enrollment)
			if includeAll || course.Type == "Course Offering" {
				courses = append(courses, course)
			}
		}
		if !page.PagingInfo.HasMoreItems || page.PagingInfo.Bookmark == "" {
			return courses, nil
		}
		query = url.Values{"bookmark": {page.PagingInfo.Bookmark}}
	}
}

type enrollmentPage struct {
	Items      []enrollment `json:"Items"`
	PagingInfo struct {
		HasMoreItems bool   `json:"HasMoreItems"`
		Bookmark     string `json:"Bookmark"`
	} `json:"PagingInfo"`
}

type enrollment struct {
	OrgUnit struct {
		ID   int    `json:"Id"`
		Name string `json:"Name"`
		Type struct {
			Name string `json:"Name"`
		} `json:"Type"`
	} `json:"OrgUnit"`
	Role struct {
		Name string `json:"Name"`
	} `json:"Role"`
}

func normalizeCourse(enrollment enrollment) Course {
	course := Course{
		ID:      enrollment.OrgUnit.ID,
		Title:   enrollment.OrgUnit.Name,
		Type:    enrollment.OrgUnit.Type.Name,
		Role:    enrollment.Role.Name,
		RawName: enrollment.OrgUnit.Name,
	}
	if matches := courseName.FindStringSubmatch(course.RawName); matches != nil {
		course.Section = stringPointer(matches[1])
		course.Code = stringPointer(matches[2])
		course.Title = matches[3]
	}
	return course
}

// TableOfContents is the normalized course-content tree.
type TableOfContents struct {
	CourseID int      `json:"course_id"`
	Modules  []Module `json:"modules"`
}

// Module is a Brightspace content module and its nested child modules.
type Module struct {
	ID          int      `json:"id"`
	Title       string   `json:"title"`
	Description *string  `json:"description"`
	Topics      []Topic  `json:"topics"`
	Modules     []Module `json:"modules"`
}

// Topic is a Brightspace content topic.
type Topic struct {
	ID      int     `json:"id"`
	Title   string  `json:"title"`
	Type    int     `json:"type"`
	URL     string  `json:"url"`
	DueDate *string `json:"due_date"`
}

// Content returns the full nested content table of contents for courseID.
func (c *Client) Content(ctx context.Context, courseID int) (TableOfContents, error) {
	var response struct {
		Modules []contentModule `json:"Modules"`
	}
	path := fmt.Sprintf("/d2l/api/le/%s/%d/content/toc", leVersion, courseID)
	if err := c.get(ctx, path, nil, fmt.Sprintf("toc(%d)", courseID), &response); err != nil {
		return TableOfContents{}, err
	}
	modules := make([]Module, len(response.Modules))
	for index, module := range response.Modules {
		modules[index] = normalizeModule(module)
	}
	return TableOfContents{CourseID: courseID, Modules: modules}, nil
}

type contentModule struct {
	ID          int             `json:"ModuleId"`
	Title       string          `json:"Title"`
	Description *richText       `json:"Description"`
	Topics      []contentTopic  `json:"Topics"`
	Modules     []contentModule `json:"Modules"`
}

type contentTopic struct {
	ID      int     `json:"TopicId"`
	Title   string  `json:"Title"`
	Type    int     `json:"Type"`
	URL     string  `json:"Url"`
	DueDate *string `json:"DueDate"`
}

type richText struct {
	Text string `json:"Text"`
	HTML string `json:"Html"`
}

func normalizeModule(module contentModule) Module {
	result := Module{ID: module.ID, Title: module.Title}
	if module.Description != nil {
		result.Description = stringPointer(module.Description.Text)
	}
	result.Topics = make([]Topic, len(module.Topics))
	for index, topic := range module.Topics {
		result.Topics[index] = Topic{ID: topic.ID, Title: topic.Title, Type: topic.Type, URL: topic.URL, DueDate: topic.DueDate}
	}
	result.Modules = make([]Module, len(module.Modules))
	for index, child := range module.Modules {
		result.Modules[index] = normalizeModule(child)
	}
	return result
}

// Grades is the normalized gradebook for a course.
type Grades struct {
	CourseID int     `json:"course_id"`
	Grades   []Grade `json:"grades"`
}

// Grade joins a grade item with the current user's value for that item.
type Grade struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	GradeType        string   `json:"grade_type"`
	MaxPoints        *float64 `json:"max_points"`
	Weight           *float64 `json:"weight"`
	IsBonus          bool     `json:"is_bonus"`
	ExcludeFromFinal bool     `json:"exclude_from_final"`
	Score            *float64 `json:"score"`
	ScoreMax         *float64 `json:"score_max"`
	DisplayedGrade   *string  `json:"displayed_grade"`
	Feedback         *string  `json:"feedback"`
	LastModified     *string  `json:"last_modified"`
}

// Grades returns grade items joined with the current user's grade values.
func (c *Client) Grades(ctx context.Context, courseID int) (Grades, error) {
	var items []gradeItem
	itemsPath := fmt.Sprintf("/d2l/api/le/%s/%d/grades/", leVersion, courseID)
	if err := c.get(ctx, itemsPath, nil, fmt.Sprintf("grades/items(%d)", courseID), &items); err != nil {
		return Grades{}, err
	}
	var values []gradeValue
	valuesPath := fmt.Sprintf("/d2l/api/le/%s/%d/grades/values/myGradeValues/", leVersion, courseID)
	if err := c.get(ctx, valuesPath, nil, fmt.Sprintf("grades/values(%d)", courseID), &values); err != nil {
		return Grades{}, err
	}
	byID := make(map[string]gradeValue, len(values))
	for _, value := range values {
		byID[value.ID.String()] = value
	}
	grades := make([]Grade, 0, len(items))
	for _, item := range items {
		value := byID[item.ID.String()]
		var feedback *string
		if strings.TrimSpace(value.Comments.Text) != "" {
			feedback = stringPointer(strings.TrimSpace(value.Comments.Text))
		}
		grades = append(grades, Grade{
			ID: item.ID.String(), Name: item.Name, GradeType: item.GradeType,
			MaxPoints: item.MaxPoints, Weight: item.Weight, IsBonus: item.IsBonus,
			ExcludeFromFinal: item.ExcludeFromFinal, Score: value.PointsNumerator,
			ScoreMax: value.PointsDenominator, DisplayedGrade: value.DisplayedGrade,
			Feedback: feedback, LastModified: value.LastModified,
		})
	}
	return Grades{CourseID: courseID, Grades: grades}, nil
}

type identifier json.RawMessage

func (id *identifier) UnmarshalJSON(data []byte) error {
	*id = append((*id)[:0], data...)
	return nil
}

func (id identifier) String() string {
	var value string
	if json.Unmarshal(id, &value) == nil {
		return value
	}
	return string(id)
}

type gradeItem struct {
	ID               identifier `json:"Id"`
	Name             string     `json:"Name"`
	GradeType        string     `json:"GradeType"`
	MaxPoints        *float64   `json:"MaxPoints"`
	Weight           *float64   `json:"Weight"`
	IsBonus          bool       `json:"IsBonus"`
	ExcludeFromFinal bool       `json:"ExcludeFromFinalGradeCalculation"`
}

type gradeValue struct {
	ID                identifier `json:"GradeObjectIdentifier"`
	PointsNumerator   *float64   `json:"PointsNumerator"`
	PointsDenominator *float64   `json:"PointsDenominator"`
	DisplayedGrade    *string    `json:"DisplayedGrade"`
	Comments          richText   `json:"Comments"`
	LastModified      *string    `json:"LastModified"`
}

// Announcement is a normalized Brightspace news item.
type Announcement struct {
	ID               int          `json:"id"`
	Title            string       `json:"title"`
	Body             string       `json:"body"`
	StartDate        *string      `json:"start_date"`
	EndDate          *string      `json:"end_date"`
	CreatedDate      *string      `json:"created_date"`
	LastModifiedDate *string      `json:"last_modified_date"`
	IsPinned         bool         `json:"is_pinned"`
	IsHidden         bool         `json:"is_hidden"`
	Attachments      []Attachment `json:"attachments"`
}

// Attachment is an announcement attachment's metadata.
type Attachment struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Size *int   `json:"size"`
}

// Announcements returns news items for courseID, optionally since an ISO-8601 date.
func (c *Client) Announcements(ctx context.Context, courseID int, since string) ([]Announcement, error) {
	query := url.Values{}
	if since != "" {
		query.Set("since", since)
	}
	var items []newsItem
	path := fmt.Sprintf("/d2l/api/le/%s/%d/news/", leVersion, courseID)
	if err := c.get(ctx, path, query, fmt.Sprintf("news(%d)", courseID), &items); err != nil {
		return nil, err
	}
	announcements := make([]Announcement, len(items))
	for index, item := range items {
		attachments := make([]Attachment, len(item.Attachments))
		for attachmentIndex, attachment := range item.Attachments {
			attachments[attachmentIndex] = Attachment{ID: attachment.ID, Name: attachment.Name, Size: attachment.Size}
		}
		announcements[index] = Announcement{
			ID: item.ID, Title: item.Title, Body: plain(item.Body), StartDate: item.StartDate,
			EndDate: item.EndDate, CreatedDate: item.CreatedDate, LastModifiedDate: item.LastModifiedDate,
			IsPinned: item.IsPinned, IsHidden: item.IsHidden, Attachments: attachments,
		}
	}
	return announcements, nil
}

type newsItem struct {
	ID               int       `json:"Id"`
	Title            string    `json:"Title"`
	Body             *richText `json:"Body"`
	StartDate        *string   `json:"StartDate"`
	EndDate          *string   `json:"EndDate"`
	CreatedDate      *string   `json:"CreatedDate"`
	LastModifiedDate *string   `json:"LastModifiedDate"`
	IsPinned         bool      `json:"IsPinned"`
	IsHidden         bool      `json:"IsHidden"`
	Attachments      []struct {
		ID   int    `json:"FileId"`
		Name string `json:"FileName"`
		Size *int   `json:"FileSize"`
	} `json:"Attachments"`
}

func plain(value *richText) string {
	if value == nil {
		return ""
	}
	if text := strings.TrimSpace(value.Text); text != "" {
		return text
	}
	return strings.TrimSpace(html.UnescapeString(htmlTag.ReplaceAllString(value.HTML, "")))
}

// Assignment is a Brightspace dropbox folder.
type Assignment struct {
	ID           int             `json:"id"`
	Name         string          `json:"name"`
	DueDate      *string         `json:"due_date"`
	Availability json.RawMessage `json:"availability"`
	GradingType  json.RawMessage `json:"grading_type"`
	Score        json.RawMessage `json:"score"`
}

// Assignments returns Brightspace dropbox folders for courseID.
func (c *Client) Assignments(ctx context.Context, courseID int) ([]Assignment, error) {
	var folders []assignment
	path := fmt.Sprintf("/d2l/api/le/%s/%d/dropbox/folders/", leVersion, courseID)
	if err := c.get(ctx, path, nil, fmt.Sprintf("dropbox(%d)", courseID), &folders); err != nil {
		return nil, err
	}
	result := make([]Assignment, len(folders))
	for index, folder := range folders {
		result[index] = Assignment{ID: folder.ID, Name: folder.Name, DueDate: folder.DueDate, Availability: folder.Availability, GradingType: folder.GradingType, Score: folder.Score}
	}
	return result, nil
}

type assignment struct {
	ID           int             `json:"Id"`
	Name         string          `json:"Name"`
	DueDate      *string         `json:"DueDate"`
	Availability json.RawMessage `json:"Availability"`
	GradingType  json.RawMessage `json:"GradingType"`
	Score        json.RawMessage `json:"Score"`
}

func (c *Client) get(ctx context.Context, path string, query url.Values, label string, destination any) error {
	if c.http == nil {
		return errors.New("brightspace client has no HTTP session")
	}
	requestURL := baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &Error{Label: label, Status: response.StatusCode, Body: truncate(string(body), 200)}
	}
	if err := json.Unmarshal(body, destination); err != nil {
		return fmt.Errorf("%s: decode response: %w", label, err)
	}
	return nil
}

func stringPointer(value string) *string { return &value }

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
