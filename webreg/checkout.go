package webreg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/nsigel/usc-cli/internal/dom"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// Checkout is the pending registration WebReg will submit for a term:
// scheduled, unregistered sections to add and unscheduled, registered
// sections to drop, grouped as WebReg presents them.
type Checkout struct {
	FormFingerprint string          `json:"form_fingerprint"`
	Term            string          `json:"term"`
	Ready           bool            `json:"ready"`
	Message         string          `json:"message,omitempty"`
	Groups          []CheckoutGroup `json:"groups"`
}

// CheckoutGroup is one checkout heading, e.g. Action "REGISTER", and its courses.
type CheckoutGroup struct {
	Action  CheckoutAction `json:"action"`
	Heading string         `json:"heading"`
	Courses []Course       `json:"courses"`
}

// CheckoutResult preserves the response without inferring enrollment from prose.
// WebReg wording is not a reliable success signal. Confirm registration by
// reading CourseBin and checking each section's Registered state.
type CheckoutResult struct {
	Term     string          `json:"term"`
	Outcome  CheckoutOutcome `json:"outcome"`
	Status   string          `json:"status"`
	Messages []string        `json:"messages,omitempty"`
}

// Checkout previews the changes Submit would send. It changes nothing.
func (c *Client) Checkout(ctx context.Context, term string) (*Checkout, error) {
	checkout, _, err := c.checkout(ctx, term)
	return checkout, err
}

func (c *Client) checkout(ctx context.Context, term string) (*Checkout, url.Values, error) {
	if err := c.SelectTerm(ctx, term); err != nil {
		return nil, nil, err
	}
	data, err := c.page(ctx, "/Checkout", "/CourseBin")
	if err != nil {
		return nil, nil, err
	}
	document := dom.Parse(data)
	wrapper := dom.FindFirst(document, dom.Class("div", "content-wrapper-regconfirm"))
	if wrapper == nil {
		return nil, nil, errors.New("WebReg checkout page has an unexpected layout")
	}
	checkout := &Checkout{Term: term, Groups: []CheckoutGroup{}}
	if message := dom.FindFirst(wrapper, dom.Tag("h6")); message != nil {
		checkout.Message = text(message)
	}
	for _, node := range dom.FindAll(wrapper, func(node *html.Node) bool {
		return node.Data == "h5" && dom.HasClass(node, "RegAdDrpTitl") || node.Data == "div" && dom.HasClass(node, "course-header")
	}) {
		if node.Data == "h5" {
			heading := text(node)
			if checkoutAction(heading) == "" {
				return nil, nil, errors.New("WebReg checkout contains an unknown action")
			}
			checkout.Groups = append(checkout.Groups, CheckoutGroup{Action: checkoutAction(heading), Heading: heading, Courses: []Course{}})
			continue
		}
		course, ok := parseCourse(node)
		if !ok {
			return nil, nil, errors.New("WebReg checkout contains an unreadable course")
		}
		if len(checkout.Groups) == 0 {
			return nil, nil, errors.New("WebReg checkout course has no action")
		}
		group := &checkout.Groups[len(checkout.Groups)-1]
		group.Courses = append(group.Courses, course)
	}

	parsedSections := 0
	seen := map[int]bool{}
	for _, group := range checkout.Groups {
		for _, course := range group.Courses {
			if len(course.Sections) == 0 {
				return nil, nil, errors.New("WebReg checkout course has no readable sections")
			}
			for _, section := range course.Sections {
				if section.ID <= 0 || seen[section.ID] {
					return nil, nil, errors.New("WebReg checkout contains an invalid or duplicate section")
				}
				seen[section.ID] = true
				parsedSections++
			}
		}
	}
	if parsedSections != len(dom.FindAll(wrapper, dom.Class("div", "section"))) {
		return nil, nil, errors.New("WebReg checkout contains unreadable sections")
	}

	form := dom.FindFirst(wrapper, func(node *html.Node) bool { return node.Data == "form" && dom.Attr(node, "id") == "MainForm" })
	if form == nil {
		return checkout, nil, nil
	}
	values := url.Values{}
	for name, value := range formFields(form) {
		if name != "btnSubmit" {
			values.Set(name, value)
		}
	}
	comparison := url.Values{}
	for key, value := range values {
		if key != tokenField {
			comparison[key] = value
		}
	}
	digest := sha256.Sum256([]byte(comparison.Encode()))
	checkout.FormFingerprint = hex.EncodeToString(digest[:])
	checkout.Ready = values.Get(tokenField) != "" && parsedSections > 0
	return checkout, values, nil
}

type CheckoutAction string

const (
	Register CheckoutAction = "register"
	Drop     CheckoutAction = "drop"
)

type CheckoutOutcome string

const CheckoutUnknown CheckoutOutcome = "unknown"

func checkoutAction(heading string) CheckoutAction {
	switch strings.TrimSpace(heading) {
	case "You are about to REGISTER for the following sections:":
		return Register
	case "You are about to DROP the following sections:":
		return Drop
	default:
		return ""
	}
}

// ErrCheckoutChanged reports that the pending actions or submitted form no longer
// match the approved checkout preview.
var ErrCheckoutChanged = errors.New("WebReg pending changes differ from the approved checkout preview")

// sameCheckoutChanges deliberately excludes presentation and live enrollment
// counts. Neither changes the action the student approved.
func sameCheckoutChanges(current, expected Checkout) bool {
	if current.Term != expected.Term || current.FormFingerprint != expected.FormFingerprint {
		return false
	}

	type sectionChange struct {
		Action    CheckoutAction
		SectionID int
	}

	collectChanges := func(checkout Checkout) (map[sectionChange]bool, bool) {
		changes := make(map[sectionChange]bool)
		for _, group := range checkout.Groups {
			if group.Action != Register && group.Action != Drop {
				return nil, false
			}

			for _, course := range group.Courses {
				for _, section := range course.Sections {
					change := sectionChange{Action: group.Action, SectionID: section.ID}
					if section.ID <= 0 || changes[change] {
						return nil, false
					}

					changes[change] = true
				}
			}
		}

		return changes, true
	}

	currentChanges, currentValid := collectChanges(current)
	expectedChanges, expectedValid := collectChanges(expected)
	if !currentValid || !expectedValid || len(currentChanges) != len(expectedChanges) {
		return false
	}

	for change := range expectedChanges {
		if !currentChanges[change] {
			return false
		}
	}

	return true
}

// Submit registers and drops the sections Checkout lists. This is the same
// irreversible action as WebReg's Submit button. Callers should show the
// preview from Checkout and obtain the student's confirmation first.
// The server has no conditional-submit primitive: another client can still
// change the bin between the final read and POST. Use exclusive session ownership.
func (c *Client) Submit(ctx context.Context, expected Checkout) (*CheckoutResult, error) {
	term := expected.Term
	if !expected.Ready {
		return nil, errors.New("checkout preview is not ready")
	}
	for _, group := range expected.Groups {
		if group.Action != Register && group.Action != Drop {
			return nil, errors.New("unknown checkout action")
		}
	}

	checkout, form, err := c.checkout(ctx, term)
	if err != nil {
		return nil, err
	}
	if !sameCheckoutChanges(*checkout, expected) {
		return nil, ErrCheckoutChanged
	}
	if !checkout.Ready {
		message := "WebReg has no pending registration changes for term " + term
		if checkout.Message != "" {
			message += ": " + checkout.Message
		}
		return nil, errors.New(message)
	}
	if form.Get("activeTerm") != "" && form.Get("activeTerm") != term {
		return nil, errors.New("WebReg checkout is for term " + form.Get("activeTerm") + ", not " + term)
	}
	data, err := c.postForm(ctx, "/CheckoutResponse", form, "/Checkout")
	if err != nil {
		return nil, err
	}
	return parseCheckoutResult(term, data)
}

func parseCheckoutResult(term string, data []byte) (*CheckoutResult, error) {
	document := dom.Parse(data)
	wrapper := dom.FindFirst(document, dom.Class("div", "content-wrapper-regconfirm"))
	if wrapper == nil {
		return nil, errors.New("WebReg returned an unexpected checkout response; check myCourseBin before retrying")
	}
	result := &CheckoutResult{Term: term, Outcome: CheckoutUnknown}
	if status := dom.FindFirst(wrapper, dom.Tag("b")); status != nil {
		result.Status = text(status)
	}
	heading := dom.FindFirst(wrapper, dom.Tag("h3"))
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child == heading || child.Type == html.ElementNode && child.Data == "b" && text(child) == result.Status {
				continue
			}
			if child.Type == html.ElementNode && (child.Data == "div" || child.Data == "span" || child.Data == "p" || child.Data == "li") && dom.FindFirst(child, dom.Tag("div")) == nil {
				result.Messages = append(result.Messages, lines(child)...)
				continue
			}
			walk(child)
		}
	}
	walk(wrapper)
	return result, nil
}
