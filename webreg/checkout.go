package webreg

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"strings"

	"github.com/nsigel/usc-cli/classes"
	"github.com/nsigel/usc-cli/internal/dom"
	"golang.org/x/net/html"
)

// CheckoutAction is what checkout will do with a section.
type CheckoutAction string

const (
	// CheckoutRegister registers a scheduled section the student is not yet
	// registered in.
	CheckoutRegister CheckoutAction = "register"
	// CheckoutDrop drops a registered section the student unscheduled.
	CheckoutDrop CheckoutAction = "drop"
)

var checkoutActionsByHeading = map[string]CheckoutAction{
	"You are about to REGISTER for the following sections:": CheckoutRegister,
	"You are about to DROP the following sections:":         CheckoutDrop,
}

// Checkout is a preview of the registration changes WebReg has pending for a
// term. Pass it to Submit to make exactly these changes. A preview survives a
// JSON round trip, so it can be shown, approved, and submitted later.
type Checkout struct {
	Term string `json:"term"`
	// Message is WebReg's notice above the changes, such as why nothing can
	// be submitted.
	Message string `json:"message,omitempty"`
	// Changes is empty when there is nothing to submit.
	Changes []CheckoutChange `json:"changes"`

	submitForm url.Values
}

// CheckoutChange is one section checkout will register or drop.
type CheckoutChange struct {
	Action      CheckoutAction     `json:"action"`
	CourseCode  classes.CourseCode `json:"course_code"`
	CourseTitle string             `json:"course_title"`
	Section     Section            `json:"section"`
}

// CheckoutResult is the page WebReg answers a submission with.
//
// WebReg's wording is not a dependable success signal, and it does not process
// partial registrations. To know what happened, read CourseBin afterwards and
// check each section's CourseBin.Registered.
type CheckoutResult struct {
	Term     string   `json:"term"`
	Status   string   `json:"status"`
	Messages []string `json:"messages,omitempty"`
}

// ErrCheckoutChanged means WebReg's pending changes are no longer the ones in
// the preview given to Submit. Nothing was submitted.
var ErrCheckoutChanged = errors.New("WebReg's pending changes differ from the checkout preview")

// Checkout previews the changes Submit would make. It changes nothing.
func (c *Client) Checkout(ctx context.Context, term string) (*Checkout, error) {
	if err := c.SelectTerm(ctx, term); err != nil {
		return nil, err
	}
	data, err := c.page(ctx, "/Checkout", "/CourseBin")
	if err != nil {
		return nil, err
	}
	wrapper := dom.FindFirst(dom.Parse(data), dom.Class("div", "content-wrapper-regconfirm"))
	if wrapper == nil {
		return nil, errors.New("WebReg checkout page has an unexpected layout")
	}

	checkout := &Checkout{Term: term, Changes: []CheckoutChange{}}
	if message := dom.FindFirst(wrapper, dom.Tag("h6")); message != nil {
		checkout.Message = text(message)
	}
	if checkout.Changes, err = parseCheckoutChanges(wrapper); err != nil {
		return nil, err
	}

	form := dom.FindFirst(wrapper, func(node *html.Node) bool {
		return node.Data == "form" && dom.Attr(node, "id") == "MainForm"
	})
	if form == nil {
		if len(checkout.Changes) > 0 {
			return nil, errors.New("WebReg checkout lists changes but no form to submit them")
		}
		return checkout, nil
	}
	checkout.submitForm = url.Values{}
	for name, value := range formFields(form) {
		if name != "btnSubmit" {
			checkout.submitForm.Set(name, value)
		}
	}
	if activeTerm := checkout.submitForm.Get("activeTerm"); activeTerm != "" && activeTerm != term {
		return nil, fmt.Errorf("WebReg checkout is for term %s, not %s", activeTerm, term)
	}
	return checkout, nil
}

func parseCheckoutChanges(wrapper *html.Node) ([]CheckoutChange, error) {
	changes := []CheckoutChange{}
	var action CheckoutAction
	headingsAndCourses := dom.FindAll(wrapper, func(node *html.Node) bool {
		return node.Data == "h5" && dom.HasClass(node, "RegAdDrpTitl") ||
			node.Data == "div" && dom.HasClass(node, "course-header")
	})
	for _, node := range headingsAndCourses {
		if node.Data == "h5" {
			heading := text(node)
			known := false
			if action, known = checkoutActionsByHeading[heading]; !known {
				return nil, fmt.Errorf("WebReg checkout has an unknown action heading %q", heading)
			}
			continue
		}
		if action == "" {
			return nil, errors.New("WebReg checkout lists a course before any action heading")
		}
		course, err := parseCourse(node)
		if err != nil {
			return nil, err
		}
		for _, section := range course.Sections {
			changes = append(changes, CheckoutChange{
				Action:      action,
				CourseCode:  course.Code,
				CourseTitle: course.Title,
				Section:     section,
			})
		}
	}
	if err := requireEverySectionPreviewed(wrapper, changes); err != nil {
		return nil, err
	}
	return changes, nil
}

func requireEverySectionPreviewed(wrapper *html.Node, changes []CheckoutChange) error {
	sectionRows := len(dom.FindAll(wrapper, dom.Class("div", "section")))
	if sectionRows != len(changes) {
		return fmt.Errorf("WebReg checkout shows %d sections but %d could be read", sectionRows, len(changes))
	}
	return nil
}

// Submit makes exactly the changes in preview: it registers and drops real
// classes and cannot be undone. It reads checkout again first and returns
// ErrCheckoutChanged, submitting nothing, if the sections to register and
// drop have changed since the preview.
//
// WebReg has no conditional submit, so a change made by another signed-in
// browser between that read and the post is not caught; use a session nothing
// else is using.
func (c *Client) Submit(ctx context.Context, preview *Checkout) (*CheckoutResult, error) {
	if preview == nil || len(preview.Changes) == 0 {
		return nil, errors.New("the checkout preview has no changes to submit")
	}
	current, err := c.Checkout(ctx, preview.Term)
	if err != nil {
		return nil, err
	}
	if !current.sameChangesAs(preview) {
		return nil, ErrCheckoutChanged
	}
	if current.submitForm.Get(tokenField) == "" {
		return nil, errors.New("WebReg checkout did not provide a request verification token")
	}
	data, err := c.postForm(ctx, "/CheckoutResponse", current.submitForm, "/Checkout")
	if err != nil {
		return nil, err
	}
	return parseCheckoutResult(preview.Term, data)
}

func (checkout *Checkout) sameChangesAs(other *Checkout) bool {
	return checkout.Term == other.Term &&
		maps.Equal(checkout.actionsBySectionID(), other.actionsBySectionID())
}

func (checkout *Checkout) actionsBySectionID() map[string]CheckoutAction {
	actions := make(map[string]CheckoutAction, len(checkout.Changes))
	for _, change := range checkout.Changes {
		actions[change.Section.ID] = change.Action
	}
	return actions
}

func parseCheckoutResult(term string, data []byte) (*CheckoutResult, error) {
	wrapper := dom.FindFirst(dom.Parse(data), dom.Class("div", "content-wrapper-regconfirm"))
	if wrapper == nil {
		return nil, errors.New("WebReg returned an unexpected checkout response; read the course bin before retrying")
	}
	result := &CheckoutResult{Term: term}
	if status := dom.FindFirst(wrapper, dom.Tag("b")); status != nil {
		result.Status = text(status)
	}
	heading := dom.FindFirst(wrapper, dom.Tag("h3"))
	var collectMessages func(*html.Node)
	collectMessages = func(node *html.Node) {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			isStatus := child.Type == html.ElementNode && child.Data == "b" && text(child) == result.Status
			if child == heading || isStatus {
				continue
			}
			isTextBlock := child.Type == html.ElementNode &&
				strings.Contains(" div span p li ", " "+child.Data+" ") &&
				dom.FindFirst(child, dom.Tag("div")) == nil
			if isTextBlock {
				result.Messages = append(result.Messages, lines(child)...)
				continue
			}
			collectMessages(child)
		}
	}
	collectMessages(wrapper)
	return result, nil
}
