package directory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// MaxResults is the most students one search returns. The directory stops
// there; narrow the search to see the rest.
const MaxResults = 100

// StudentQuery filters a search: either Query, a basic search over name,
// major, and email, or any of the other fields. Each matches as a substring.
// The directory ignores Query alongside fields, so Students rejects both.
type StudentQuery struct {
	Query     string
	FirstName string
	LastName  string
	Major     string
	School    string
	Email     string
	Phone     string
}

// Student is one directory entry. Employment fields are set only for
// students who hold a USC job.
type Student struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	FullName      string   `json:"full_name,omitempty"`
	Email         string   `json:"email,omitempty"`
	Phone         string   `json:"phone,omitempty"`
	Major         string   `json:"major,omitempty"`
	Minor         string   `json:"minor,omitempty"`
	DegreeProgram string   `json:"degree_program,omitempty"`
	School        string   `json:"school,omitempty"`
	Title         string   `json:"title,omitempty"`
	Department    string   `json:"department,omitempty"`
	Division      string   `json:"division,omitempty"`
	MailCode      string   `json:"mail_code,omitempty"`
	Address       []string `json:"address,omitempty"`
}

// StudentResults is a search's matches. Truncated means the directory
// stopped at MaxResults and more students may match.
type StudentResults struct {
	Students  []Student `json:"students"`
	Truncated bool      `json:"truncated"`
}

// Students searches the student directory.
func (c *Client) Students(ctx context.Context, query StudentQuery) (StudentResults, error) {
	values := url.Values{}
	if basic := strings.TrimSpace(query.Query); basic != "" {
		values.Set("basic", basic)
	}
	for name, value := range map[string]string{
		"first": query.FirstName, "last": query.LastName,
		"major": query.Major, "school": query.School, "email": query.Email, "tel": query.Phone,
	} {
		if value = strings.TrimSpace(value); value != "" {
			values.Set(name, value)
		}
	}
	switch {
	case len(values) == 0:
		return StudentResults{}, errors.New("a student search needs a query or at least one field")
	case values.Has("basic") && len(values) > 1:
		return StudentResults{}, errors.New("a student search takes a query or fields, not both")
	}
	data, err := c.proxy(ctx, values)
	if err != nil {
		return StudentResults{}, err
	}
	entries, err := decodeEntries(data)
	if err != nil {
		return StudentResults{}, err
	}
	students := make([]Student, len(entries))
	for i, entry := range entries {
		students[i] = entry.student()
	}
	return StudentResults{Students: students, Truncated: len(students) >= MaxResults}, nil
}

// Student returns one student by the id a search returned.
func (c *Client) Student(ctx context.Context, id string) (Student, error) {
	id = strings.TrimSpace(id)
	if id == "" || strings.Trim(strings.ToLower(id), "abcdefghijklmnopqrstuvwxyz0123456789") != "" {
		return Student{}, fmt.Errorf("student id must be letters and digits, such as an id a search returned; got %q", id)
	}
	data, err := c.proxy(ctx, url.Values{"uscpvid": {id}})
	if err != nil {
		return Student{}, err
	}
	entries, err := decodeEntries(data)
	if err != nil {
		return Student{}, err
	}
	if len(entries) == 0 {
		return Student{}, fmt.Errorf("no student with id %q", id)
	}
	return entries[0].student(), nil
}

// entry is a directory LDAP record: every attribute is a list of values.
type entry map[string][]string

// decodeEntries reads the proxy's answer, which is null for no match, one
// object for a single match, and an array otherwise.
func decodeEntries(data []byte) ([]entry, error) {
	data = bytes.TrimSpace(data)
	switch {
	case len(data) == 0:
		// The directory sends nothing, not null, to requests it refuses.
		return nil, &Error{Status: 200, Message: "empty response"}
	case string(data) == "null":
		return nil, nil
	case data[0] == '{':
		var one entry
		if err := json.Unmarshal(data, &one); err != nil {
			return nil, fmt.Errorf("directory: decode student: %w", err)
		}
		return []entry{one}, nil
	case data[0] == '[':
		var many []entry
		if err := json.Unmarshal(data, &many); err != nil {
			return nil, fmt.Errorf("directory: decode students: %w", err)
		}
		return many, nil
	}
	return nil, &Error{Status: 200, Message: shortMessage(data)}
}

// value joins a multi-valued attribute the way the directory page does.
func (e entry) value(name string) string {
	return strings.Join(e[name], ", ")
}

func (e entry) student() Student {
	student := Student{
		ID:            e.value("uscpvid"),
		Name:          e.value("displayname"),
		FullName:      e.value("cn"),
		Email:         e.value("mail"),
		Phone:         e.value("telephonenumber"),
		Major:         e.value("uscmajor"),
		Minor:         e.value("uscminordescription"),
		DegreeProgram: e.value("uscstudentdegreeprogram"),
		School:        e.value("uscmajorowningschool"),
		Title:         e.value("title"),
		Department:    e.value("departmentnumber"),
		Division:      e.value("usccostcenternamelevel5"),
		MailCode:      e.value("uscofficeaddressmailcode"),
	}
	// LDAP postal addresses separate lines with "$".
	for _, address := range e["postaladdress"] {
		for _, line := range strings.Split(address, "$") {
			if line = strings.TrimSpace(line); line != "" {
				student.Address = append(student.Address, line)
			}
		}
	}
	return student
}
