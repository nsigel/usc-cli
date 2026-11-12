package auth

import (
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/nsigel/usc-cli/internal/dom"
	"golang.org/x/net/html"
)

type field struct {
	Name  string
	Value string
}

type form struct {
	Method string
	Action string
	Fields []field
}

func firstForm(page *page) (form, error) {
	form, ok := autoForm(page)
	if !ok {
		return form, errors.New("no form found")
	}
	return form, nil
}

// autoForm reads the page's first form. SSO handoff pages make the first form
// their continuation; restricting the choice avoids submitting unrelated
// account or recovery forms.
func autoForm(page *page) (form, bool) {
	node := dom.FindFirst(dom.Parse(page.Body), dom.Tag("form"))
	if node == nil {
		return form{}, false
	}
	result := form{Method: "GET", Action: dom.Attr(node, "action")}
	if method := dom.Attr(node, "method"); method != "" {
		result.Method = method
	}
	for _, input := range dom.FindAll(node, func(node *html.Node) bool { return node.Data == "input" || node.Data == "button" }) {
		if name := dom.Attr(input, "name"); name != "" {
			result.Fields = append(result.Fields, field{Name: name, Value: dom.Attr(input, "value")})
		}
	}
	return result, len(result.Fields) > 0
}

var (
	metaRefreshPattern      = regexp.MustCompile(`(?is)(?:^|;)\s*url\s*=\s*["']?\s*([^"'<>\s;]+)`)
	scriptNavigationPattern = regexp.MustCompile(`(?is)(?:(?:window\.)?location(?:\.href)?\s*=|location\.replace\()\s*["']([^"']+)["']`)
	scriptURLPattern        = regexp.MustCompile(`(?is)(?:var|let|const)\s+url\s*=\s*["']([^"']+)["']`)
)

// htmlNavigation recognizes only static browser redirects used by the known
// providers. It is intentionally not a general JavaScript evaluator.
func htmlNavigation(page *page) (*url.URL, bool) {
	document := dom.Parse(page.Body)
	destination := metaRefresh(document)
	if destination == "" {
		destination = scriptNavigation(document)
	}
	if destination == "" {
		return nil, false
	}
	target, err := page.URL.Parse(destination)
	return target, err == nil
}

func metaRefresh(document *html.Node) string {
	for _, meta := range dom.FindAll(document, dom.Tag("meta")) {
		if !strings.EqualFold(dom.Attr(meta, "http-equiv"), "refresh") {
			continue
		}
		if match := metaRefreshPattern.FindStringSubmatch(dom.Attr(meta, "content")); match != nil {
			return match[1]
		}
	}
	return ""
}

func scriptNavigation(document *html.Node) string {
	for _, node := range dom.FindAll(document, dom.Tag("script")) {
		script := dom.RawText(node)
		match := scriptNavigationPattern.FindStringSubmatch(script)
		if match == nil && strings.Contains(script, "location") {
			// Salesforce stores its destination in a local `url` variable before
			// calling location.replace rather than using a string literal.
			match = scriptURLPattern.FindStringSubmatch(script)
		}
		if match != nil {
			return match[1]
		}
	}
	return ""
}

func pageTitle(body []byte) string {
	return strings.TrimSpace(dom.RawText(dom.FindFirst(dom.Parse(body), dom.Tag("title"))))
}

func elementValue(body []byte, id string) string {
	return dom.Attr(dom.FindFirst(dom.Parse(body), dom.ID(id)), "value")
}

func setField(form *form, name, value string) {
	for index := range form.Fields {
		if form.Fields[index].Name == name {
			form.Fields[index].Value = value
			return
		}
	}
	form.Fields = append(form.Fields, field{Name: name, Value: value})
}

func encodeFields(fields []field) string {
	encoded := make([]string, 0, len(fields))
	for _, field := range fields {
		encoded = append(encoded, url.QueryEscape(field.Name)+"="+url.QueryEscape(field.Value))
	}
	return strings.Join(encoded, "&")
}
