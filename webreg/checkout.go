package webreg

import (
	"context"
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
	Term    string          `json:"term"`
	Ready   bool            `json:"ready"`
	Message string          `json:"message,omitempty"`
	Groups  []CheckoutGroup `json:"groups"`
}

// CheckoutGroup is one checkout heading, e.g. Action "REGISTER", and its courses.
type CheckoutGroup struct {
	Action  string   `json:"action"`
	Heading string   `json:"heading"`
	Courses []Course `json:"courses"`
}

// CheckoutResult is WebReg's registration response. WebReg does not process
// partial registrations: on failure nothing was changed.
type CheckoutResult struct {
	Term      string   `json:"term"`
	Succeeded bool     `json:"succeeded"`
	Status    string   `json:"status"`
	Messages  []string `json:"messages,omitempty"`
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

	courses := parseCourses(wrapper)
	byCode := make(map[string]Course, len(courses))
	for _, course := range courses {
		byCode[course.Code] = course
	}
	for _, node := range dom.FindAll(wrapper, func(node *html.Node) bool {
		return node.Data == "h5" && dom.HasClass(node, "RegAdDrpTitl") || node.Data == "div" && dom.HasClass(node, "course-header")
	}) {
		if node.Data == "h5" {
			heading := text(node)
			checkout.Groups = append(checkout.Groups, CheckoutGroup{Action: checkoutAction(heading), Heading: heading, Courses: []Course{}})
			continue
		}
		if len(checkout.Groups) == 0 {
			checkout.Groups = append(checkout.Groups, CheckoutGroup{Courses: []Course{}})
		}
		group := &checkout.Groups[len(checkout.Groups)-1]
		group.Courses = append(group.Courses, byCode[courseCode(node)])
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
	checkout.Ready = values.Get(tokenField) != "" && len(checkout.Groups) > 0
	return checkout, values, nil
}

// checkoutAction extracts the uppercase verb from headings such as
// "You are about to REGISTER for the following sections:".
func checkoutAction(heading string) string {
	for _, word := range strings.Fields(heading) {
		word = strings.Trim(word, ":,.")
		if len(word) > 1 && word == strings.ToUpper(word) && strings.ToLower(word) != word {
			return word
		}
	}
	return ""
}

// Submit registers and drops the sections Checkout lists. This is the same
// irreversible action as WebReg's Submit button. Callers should show the
// preview from Checkout and obtain the student's confirmation first.
func (c *Client) Submit(ctx context.Context, term string) (*CheckoutResult, error) {
	checkout, form, err := c.checkout(ctx, term)
	if err != nil {
		return nil, err
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
	result := &CheckoutResult{Term: term}
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
	lower := strings.ToLower(result.Status)
	result.Succeeded = result.Status != "" && !strings.Contains(lower, "fail") && !strings.Contains(lower, "error") && !strings.Contains(lower, "not ")
	return result, nil
}
