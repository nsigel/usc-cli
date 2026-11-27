package webreg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nsigel/usc-cli/internal/dom"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type CourseBin struct {
	Term               string   `json:"term"`
	RegisteredSections int      `json:"registered_sections"`
	ScheduledSections  int      `json:"scheduled_sections"`
	AsOf               string   `json:"as_of,omitempty"`
	Courses            []Course `json:"courses"`
}

func (c *Client) CourseBin(ctx context.Context, term string) (*CourseBin, error) {
	if err := c.SelectTerm(ctx, term); err != nil {
		return nil, err
	}
	data, err := c.page(ctx, "/CourseBin", "/Departments")
	if err != nil {
		return nil, err
	}
	document := dom.Parse(data)
	bin := &CourseBin{Term: term, Courses: parseCourses(document)}
	if status := dom.FindFirst(document, dom.Class("span", "mycbstat")); status != nil {
		counts := dom.FindAll(status, dom.Class("span", "courseBinStat"))
		if len(counts) == 2 {
			bin.RegisteredSections, _ = strconv.Atoi(text(counts[0]))
			bin.ScheduledSections, _ = strconv.Atoi(text(counts[1]))
		}
		if _, asOf, ok := strings.Cut(text(status), "(As of "); ok {
			bin.AsOf = strings.TrimSuffix(asOf, ")")
		}
	}
	return bin, nil
}

func (b *CourseBin) Section(id int) (*Section, error) {
	for courseIndex := range b.Courses {
		for sectionIndex := range b.Courses[courseIndex].Sections {
			if section := &b.Courses[courseIndex].Sections[sectionIndex]; section.ID == id {
				return section, nil
			}
		}
	}
	return nil, fmt.Errorf("section %d is not in the %s course bin", id, b.Term)
}

func (c *Client) Add(ctx context.Context, term string, sectionID int) error {
	if err := c.SelectTerm(ctx, term); err != nil {
		return err
	}
	token, err := c.token(ctx)
	if err != nil {
		return err
	}
	form := url.Values{"submit": {"Add"}, tokenField: {token}, "X-Requested-With": {"XMLHttpRequest"}}
	_, err = c.postAJAX(ctx, sectionPath(term, sectionID, "add"), form, "/Courses")
	return err
}

func (c *Client) Schedule(ctx context.Context, term string, sectionID int) error {
	return c.sectionAction(ctx, term, sectionID, "schedule")
}

func (c *Client) Unschedule(ctx context.Context, term string, sectionID int) error {
	return c.sectionAction(ctx, term, sectionID, "un-schedule")
}

func (c *Client) Remove(ctx context.Context, term string, sectionID int) error {
	return c.sectionAction(ctx, term, sectionID, "remove")
}

func (c *Client) sectionAction(ctx context.Context, term string, sectionID int, action string) error {
	if err := c.SelectTerm(ctx, term); err != nil {
		return err
	}
	_, err := c.postAJAX(ctx, sectionPath(term, sectionID, action), url.Values{"X-Requested-With": {"XMLHttpRequest"}}, "/CourseBin")
	return err
}

func sectionPath(term string, sectionID int, action string) string {
	return "/api/section/" + url.PathEscape(term) + "/" + strconv.Itoa(sectionID) + "/" + action
}

func (c *Client) SetGradeOption(ctx context.Context, term string, sectionID int, code string) (string, error) {
	bin, err := c.CourseBin(ctx, term)
	if err != nil {
		return "", err
	}
	section, err := bin.Section(sectionID)
	if err != nil {
		return "", err
	}
	option := section.GradeOption
	if option == nil || option.path == "" || option.field == "" {
		return "", fmt.Errorf("section %d has no changeable grade option", sectionID)
	}
	valid := false
	for _, choice := range option.Choices {
		valid = valid || strings.EqualFold(choice.Code, code)
		if strings.EqualFold(choice.Code, code) {
			code = choice.Code
		}
	}
	if !valid {
		return "", fmt.Errorf("grade option %q is not offered for section %d", code, sectionID)
	}
	form := url.Values{option.field: {code}, tokenField: {option.token}, "X-Requested-With": {"XMLHttpRequest"}}
	data, err := c.postAJAX(ctx, option.path, form, "/CourseBin")
	if err != nil {
		return "", err
	}
	return text(dom.Parse(data)), nil
}

func (c *Client) token(ctx context.Context) (string, error) {
	for _, path := range []string{"/CourseBin", "/Checkout"} {
		data, err := c.page(ctx, path, "/Departments")
		if err != nil {
			return "", err
		}
		if token := requestToken(dom.Parse(data)); token != "" {
			return token, nil
		}
	}
	return "", errors.New("WebReg did not provide a request verification token")
}

const calendarDataMarker = `"data":{"Data":`

type CalendarEvent struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	SectionID string    `json:"section_id,omitempty"`
	Status    string    `json:"status"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
}

func (c *Client) Calendar(ctx context.Context, term string) ([]CalendarEvent, error) {
	if err := c.SelectTerm(ctx, term); err != nil {
		return nil, err
	}
	data, err := c.page(ctx, "/Calendar", "/CourseBin")
	if err != nil {
		return nil, err
	}
	index := bytes.Index(data, []byte(calendarDataMarker))
	if index < 0 {
		return nil, errors.New("WebReg calendar page did not include calendar data")
	}
	var wire struct {
		Data []struct {
			ID        string    `json:"Id"`
			Title     string    `json:"Title"`
			SectionID string    `json:"SectionId"`
			Status    string    `json:"Status"`
			Start     time.Time `json:"Start"`
			End       time.Time `json:"End"`
		} `json:"Data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data[index+len(`"data":`):]))
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("read WebReg calendar data: %w", err)
	}
	events := make([]CalendarEvent, 0, len(wire.Data))
	for _, event := range wire.Data {
		events = append(events, CalendarEvent{
			ID: event.ID, Title: event.Title, SectionID: event.SectionID,
			Status: event.Status, Start: event.Start, End: event.End,
		})
	}
	return events, nil
}
