package libcal

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/nsigel/usc-cli/internal/dom"
	http "github.com/saucesteals/fhttp"
	"golang.org/x/net/html"
)

type bookingForm struct {
	Action  string
	Method  string
	Referer string
	Fields  []bookingFormField
}

type bookingFormField struct {
	Name     string
	Type     string
	Label    string
	Value    string
	Required bool
	Checked  bool
	Options  []bookingOption
}

type bookingOption struct {
	Value    string
	Label    string
	Disabled bool
	Selected bool
}

func parseBookingForm(raw string) (bookingForm, error) {
	document := dom.Parse([]byte(raw))
	node := dom.FindFirst(document, dom.ID("s-lc-eq-bform"))
	if node == nil || node.Data != "form" {
		node = dom.FindFirst(document, dom.Tag("form"))
	}
	if node == nil {
		return bookingForm{}, errors.New("LibCal returned a page without a reservation form")
	}
	form := bookingForm{Action: dom.Attr(node, "action"), Method: strings.ToUpper(dom.Attr(node, "method"))}
	if form.Action == "" {
		return bookingForm{}, errors.New("LibCal reservation form has no submission URL")
	}
	if form.Method == "" {
		form.Method = http.MethodPost
	}
	labels := make(map[string]string)
	for _, label := range dom.FindAll(node, dom.Tag("label")) {
		if id := dom.Attr(label, "for"); id != "" {
			labels[id] = spacedText(label)
		}
	}
	controls := dom.FindAll(node, func(node *html.Node) bool {
		return node.Data == "input" || node.Data == "textarea" || node.Data == "select"
	})
	for _, control := range controls {
		if field, ok := parseField(control, labels[dom.Attr(control, "id")]); ok {
			form.Fields = append(form.Fields, field)
		}
	}
	return form, nil
}

func parseField(node *html.Node, label string) (bookingFormField, bool) {
	field := bookingFormField{
		Name: dom.Attr(node, "name"), Type: node.Data, Label: label,
		// LibCal marks required controls with classes instead of the attribute.
		Required: dom.HasAttr(node, "required") || dom.HasClass(node, "notempty") || dom.HasClass(node, "requirechecked"),
	}
	if field.Name == "" || dom.HasAttr(node, "disabled") {
		return bookingFormField{}, false
	}
	switch node.Data {
	case "input":
		field.Type = cmp.Or(strings.ToLower(dom.Attr(node, "type")), "text")
		if slices.Contains([]string{"password", "submit", "button", "reset"}, field.Type) {
			return bookingFormField{}, false
		}
		field.Value = dom.Attr(node, "value")
		field.Checked = dom.HasAttr(node, "checked")
	case "textarea":
		field.Value = spacedText(node)
	case "select":
		for _, option := range dom.FindAll(node, dom.Tag("option")) {
			value := dom.Attr(option, "value")
			if !dom.HasAttr(option, "value") {
				value = spacedText(option)
			}
			field.Options = append(field.Options, bookingOption{
				Value: value, Label: spacedText(option), Selected: dom.HasAttr(option, "selected"),
				Disabled: dom.HasAttr(option, "disabled") || dom.HasAttr(option.Parent, "disabled"),
			})
		}
	}
	return field, true
}

func (form bookingForm) answers(details ReservationDetails) (url.Values, error) {
	for name := range details.Fields {
		if !slices.ContainsFunc(form.Fields, func(field bookingFormField) bool { return field.Name == name && field.Type != "hidden" }) {
			return nil, &Error{Code: "libcal_invalid_details", Message: "unknown checkout field " + name}
		}
	}
	values := make(url.Values)
	radioGroups := make(map[string]bool)
	for _, field := range form.Fields {
		if field.Type == "hidden" {
			values.Set(field.Name, field.Value)
			continue
		}
		if field.Type == "radio" {
			if radioGroups[field.Name] {
				continue
			}
			radioGroups[field.Name] = true
			field.Required = slices.ContainsFunc(form.radios(field.Name), func(choice bookingFormField) bool { return choice.Required })
		}
		value, err := form.answer(field, details)
		if err != nil {
			return nil, err
		}
		if value == "" && field.Required {
			return nil, form.fieldError(field, "a value is required")
		}
		if value != "" {
			values.Add(field.Name, value)
		}
	}
	return values, nil
}

func (form bookingForm) answer(field bookingFormField, details ReservationDetails) (string, error) {
	given, provided := details.Fields[field.Name]
	value := field.Value
	if provided {
		value = given
	}
	switch field.Type {
	case "checkbox":
		if details.AcceptTerms && isTermsField(field) {
			return cmp.Or(value, "1"), nil
		}
		if provided || field.Checked {
			return value, nil
		}
		return "", nil
	case "radio":
		// As in a browser, the last checked button of a group wins.
		selected := ""
		for _, choice := range form.radios(field.Name) {
			if provided && choice.Value == given || !provided && choice.Checked {
				selected = choice.Value
			}
		}
		if selected == "" && given != "" {
			return "", form.fieldError(field, "invalid choice")
		}
		return selected, nil
	case "select":
		isOffered := func(option bookingOption) bool { return option.Value == given && !option.Disabled }
		if !provided {
			isOffered = func(option bookingOption) bool { return option.Selected && !option.Disabled }
		}
		if index := slices.IndexFunc(field.Options, isOffered); index >= 0 {
			return field.Options[index].Value, nil
		}
		if given != "" {
			return "", form.fieldError(field, "invalid choice")
		}
		return "", nil
	}
	return value, nil
}

func (form bookingForm) radios(name string) []bookingFormField {
	var choices []bookingFormField
	for _, field := range form.Fields {
		if field.Type == "radio" && field.Name == name {
			choices = append(choices, field)
		}
	}
	return choices
}

func isTermsField(field bookingFormField) bool {
	name := strings.ToLower(field.Name + " " + field.Label)
	return strings.Contains(name, "terms") || strings.Contains(name, "condition") || strings.Contains(name, "agree")
}

func (form bookingForm) fieldError(field bookingFormField, reason string) error {
	var choices []string
	for _, option := range field.Options {
		if !option.Disabled && option.Value != "" {
			choices = append(choices, fmt.Sprintf("%q (%s)", option.Value, option.Label))
		}
	}
	if field.Type == "radio" {
		for _, choice := range form.radios(field.Name) {
			choices = append(choices, fmt.Sprintf("%q (%s)", choice.Value, choice.Label))
		}
	}
	message := fmt.Sprintf("LibCal field %s: %s", field.Name, reason)
	if len(choices) > 0 {
		message += "; choices: " + strings.Join(choices, ", ")
	}
	return &Error{Code: "libcal_invalid_details", Message: message}
}

func spacedText(node *html.Node) string {
	var parts []string
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			parts = append(parts, current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.TrimSpace(strings.Join(parts, " "))
}
