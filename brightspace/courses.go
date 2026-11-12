package brightspace

import (
	"context"
	"net/url"
	"regexp"
)

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

// courseName splits names such as "20263_14721 BUAD-304: Organizational Behavior".
var courseName = regexp.MustCompile(`^(\S+)\s+([A-Z]+-\d+[A-Z]*):\s+(.+)$`)

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
