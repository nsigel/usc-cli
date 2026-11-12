package brightspace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

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
		result[index] = Assignment(folder)
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

// Submission is one of the current user's submissions to a dropbox folder.
type Submission struct {
	ID          int              `json:"id"`
	SubmittedBy string           `json:"submitted_by"`
	Date        *string          `json:"date"`
	Comment     string           `json:"comment"`
	Files       []SubmissionFile `json:"files"`
}

// SubmissionFile is a file attached to a dropbox submission.
type SubmissionFile struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// Submissions returns the current user's submissions to a dropbox folder,
// oldest first. For students Brightspace scopes this list to their own entity;
// the mysubmissions route is not authorized for browser sessions.
func (c *Client) Submissions(ctx context.Context, courseID, folderID int) ([]Submission, error) {
	var entities []entityDropbox
	path := fmt.Sprintf("/d2l/api/le/%s/%d/dropbox/folders/%d/submissions/", leVersion, courseID, folderID)
	if err := c.get(ctx, path, nil, fmt.Sprintf("submissions(%d)", folderID), &entities); err != nil {
		return nil, err
	}
	result := make([]Submission, 0)
	for _, entity := range entities {
		for _, submission := range entity.Submissions {
			files := make([]SubmissionFile, 0, len(submission.Files))
			for _, file := range submission.Files {
				if !file.IsDeleted {
					files = append(files, SubmissionFile{ID: file.ID, Name: file.Name, Size: file.Size})
				}
			}
			result = append(result, Submission{ID: submission.ID, SubmittedBy: submission.SubmittedBy.DisplayName, Date: submission.Date, Comment: plain(&submission.Comment), Files: files})
		}
	}
	return result, nil
}

// DownloadSubmissionFile returns the body of a file from one of the current
// user's dropbox submissions.
func (c *Client) DownloadSubmissionFile(ctx context.Context, courseID, folderID, submissionID, fileID int) (io.ReadCloser, error) {
	return c.Download(ctx, fmt.Sprintf("/d2l/api/le/%s/%d/dropbox/folders/%d/submissions/%d/files/%d", leVersion, courseID, folderID, submissionID, fileID))
}

type entityDropbox struct {
	Submissions []struct {
		ID          int `json:"Id"`
		SubmittedBy struct {
			DisplayName string `json:"DisplayName"`
		} `json:"SubmittedBy"`
		Date    *string  `json:"SubmissionDate"`
		Comment richText `json:"Comment"`
		Files   []struct {
			ID        int    `json:"FileId"`
			Name      string `json:"FileName"`
			Size      int64  `json:"Size"`
			IsDeleted bool   `json:"IsDeleted"`
		} `json:"Files"`
	} `json:"Submissions"`
}
