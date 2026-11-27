package brightspace

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
)

type Announcement struct {
	CourseID         int          `json:"course_id"`
	CourseCode       *string      `json:"course_code"`
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

type Attachment struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Size *int   `json:"size"`
}

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
			attachments[attachmentIndex] = Attachment(attachment)
		}
		announcements[index] = Announcement{
			CourseID: courseID, ID: item.ID, Title: item.Title, Body: plain(item.Body), StartDate: item.StartDate,
			EndDate: item.EndDate, CreatedDate: item.CreatedDate, LastModifiedDate: item.LastModifiedDate,
			IsPinned: item.IsPinned, IsHidden: item.IsHidden, Attachments: attachments,
		}
	}
	return announcements, nil
}

type AnnouncementFeed struct {
	Announcements []Announcement `json:"announcements"`
	Errors        []CourseError  `json:"errors,omitempty"`
}

type CourseError struct {
	CourseID int    `json:"course_id"`
	Error    string `json:"error"`
}

func (c *Client) AllAnnouncements(ctx context.Context, since string) (AnnouncementFeed, error) {
	courses, err := c.Courses(ctx)
	if err != nil {
		return AnnouncementFeed{}, err
	}
	feed := AnnouncementFeed{Announcements: []Announcement{}}
	for _, course := range courses {
		announcements, err := c.Announcements(ctx, course.ID, since)
		if ctx.Err() != nil {
			return AnnouncementFeed{}, ctx.Err()
		}
		if err != nil {
			feed.Errors = append(feed.Errors, CourseError{CourseID: course.ID, Error: err.Error()})
			continue
		}
		for _, announcement := range announcements {
			announcement.CourseCode = course.Code
			feed.Announcements = append(feed.Announcements, announcement)
		}
	}
	return feed, nil
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

var htmlTag = regexp.MustCompile(`<[^>]+>`)

func plain(value *richText) string {
	if value == nil {
		return ""
	}
	if text := strings.TrimSpace(value.Text); text != "" {
		return text
	}
	return strings.TrimSpace(html.UnescapeString(htmlTag.ReplaceAllString(value.HTML, "")))
}
