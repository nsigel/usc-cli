package auth

import (
	"bytes"
	"errors"
	"net/url"
	"regexp"
	"strings"

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

func autoForm(page *page) (form, bool) {
	document := parseHTML(page.Body)
	if document == nil {
		return form{}, false
	}

	// SSO handoff pages make the first form their continuation. Restricting the
	// choice avoids submitting unrelated account or recovery forms.
	var formNode *html.Node
	walkHTML(document, func(node *html.Node) bool {
		if node.Type == html.ElementNode && node.Data == "form" {
			formNode = node
			return true
		}
		return false
	})
	if formNode == nil {
		return form{}, false
	}

	result := form{Method: "GET", Action: htmlAttribute(formNode, "action")}
	if method := htmlAttribute(formNode, "method"); method != "" {
		result.Method = method
	}
	walkHTML(formNode, func(node *html.Node) bool {
		if node.Type != html.ElementNode || (node.Data != "input" && node.Data != "button") {
			return false
		}
		if name := htmlAttribute(node, "name"); name != "" {
			result.Fields = append(result.Fields, field{Name: name, Value: htmlAttribute(node, "value")})
		}
		return false
	})
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
	document := parseHTML(page.Body)
	if document == nil {
		return nil, false
	}
	var rawDestination string
	walkHTML(document, func(node *html.Node) bool {
		if node.Type != html.ElementNode || node.Data != "meta" || !strings.EqualFold(htmlAttribute(node, "http-equiv"), "refresh") {
			return false
		}
		match := metaRefreshPattern.FindStringSubmatch(htmlAttribute(node, "content"))
		if len(match) == 2 {
			rawDestination = match[1]
			return true
		}
		return false
	})
	if rawDestination == "" {
		walkHTML(document, func(node *html.Node) bool {
			if node.Type != html.ElementNode || node.Data != "script" {
				return false
			}
			script := htmlText(node)
			match := scriptNavigationPattern.FindStringSubmatch(script)
			if len(match) != 2 && strings.Contains(script, "location") {
				// Salesforce stores its destination in a local `url` variable before
				// calling location.replace rather than using a string literal.
				match = scriptURLPattern.FindStringSubmatch(script)
			}
			if len(match) == 2 {
				rawDestination = match[1]
				return true
			}
			return false
		})
	}
	if rawDestination == "" {
		return nil, false
	}
	destination, err := page.URL.Parse(rawDestination)
	return destination, err == nil
}

func pageTitle(body []byte) string {
	document := parseHTML(body)
	if document == nil {
		return ""
	}
	var title string
	walkHTML(document, func(node *html.Node) bool {
		if node.Type == html.ElementNode && node.Data == "title" {
			title = strings.TrimSpace(htmlText(node))
			return true
		}
		return false
	})
	return title
}

func elementValue(body []byte, id string) string {
	document := parseHTML(body)
	if document == nil {
		return ""
	}
	var value string
	walkHTML(document, func(node *html.Node) bool {
		if node.Type == html.ElementNode && htmlAttribute(node, "id") == id {
			value = htmlAttribute(node, "value")
			return true
		}
		return false
	})
	return value
}

func parseHTML(body []byte) *html.Node {
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	return document
}

func walkHTML(node *html.Node, visit func(*html.Node) bool) bool {
	if visit(node) {
		return true
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if walkHTML(child, visit) {
			return true
		}
	}
	return false
}

func htmlAttribute(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val
		}
	}
	return ""
}

func htmlText(node *html.Node) string {
	var text strings.Builder
	walkHTML(node, func(child *html.Node) bool {
		if child.Type == html.TextNode {
			text.WriteString(child.Data)
		}
		return false
	})
	return text.String()
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
