package brightspace

import (
	"context"
	"net/url"
	"regexp"
)

type User struct {
	ID         string `json:"id"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	UniqueName string `json:"unique_name"`
}

func (c *Client) WhoAmI(ctx context.Context) (User, error) {
	var wire struct {
		Identifier string `json:"Identifier"`
		FirstName  string `json:"FirstName"`
		LastName   string `json:"LastName"`
		UniqueName string `json:"UniqueName"`
	}
	err := c.get(ctx, "/d2l/api/lp/"+lpVersion+"/users/whoami", nil, "whoami", &wire)
	return User{ID: wire.Identifier, FirstName: wire.FirstName, LastName: wire.LastName, UniqueName: wire.UniqueName}, err
}

type Course struct {
	ID      int     `json:"id"`
	Code    *string `json:"code"`
	Title   string  `json:"title"`
	Section *string `json:"section"`
	Type    string  `json:"type"`
	Role    string  `json:"role"`
	RawName string  `json:"raw_name"`
}

const courseOffering = "Course Offering"

func (c *Client) Courses(ctx context.Context) ([]Course, error) {
	enrollments, err := c.Enrollments(ctx)
	if err != nil {
		return nil, err
	}
	courses := []Course{}
	for _, enrollment := range enrollments {
		if enrollment.Type == courseOffering {
			courses = append(courses, enrollment)
		}
	}
	return courses, nil
}

func (c *Client) Enrollments(ctx context.Context) ([]Course, error) {
	path := "/d2l/api/lp/" + lpVersion + "/enrollments/myenrollments/"
	query := url.Values{"pageSize": {"100"}}
	enrollments := []Course{}
	for {
		var page enrollmentPage
		if err := c.get(ctx, path, query, "enrollments", &page); err != nil {
			return nil, err
		}
		for _, enrollment := range page.Items {
			enrollments = append(enrollments, normalizeCourse(enrollment))
		}
		if !page.PagingInfo.HasMoreItems || page.PagingInfo.Bookmark == "" {
			return enrollments, nil
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
	Access struct {
		ClasslistRoleName string `json:"ClasslistRoleName"`
	} `json:"Access"`
}

var courseName = regexp.MustCompile(`^(\S+)\s+([A-Z]+-\d+[A-Z]*):\s+(.+)$`)

func normalizeCourse(enrollment enrollment) Course {
	course := Course{
		ID:      enrollment.OrgUnit.ID,
		Title:   enrollment.OrgUnit.Name,
		Type:    enrollment.OrgUnit.Type.Name,
		Role:    enrollment.Access.ClasslistRoleName,
		RawName: enrollment.OrgUnit.Name,
	}
	if matches := courseName.FindStringSubmatch(course.RawName); matches != nil {
		course.Section = stringPointer(matches[1])
		course.Code = stringPointer(matches[2])
		course.Title = matches[3]
	}
	return course
}
