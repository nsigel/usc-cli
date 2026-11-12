package webreg

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

func parseHTML(data []byte) *html.Node {
	document, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return &html.Node{Type: html.DocumentNode}
	}
	return document
}

// findAll returns matching descendants of node in document order.
func findAll(node *html.Node, match func(*html.Node) bool) []*html.Node {
	var found []*html.Node
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && match(child) {
				found = append(found, child)
			}
			walk(child)
		}
	}
	if node != nil {
		walk(node)
	}
	return found
}

func findFirst(node *html.Node, match func(*html.Node) bool) *html.Node {
	if found := findAll(node, match); len(found) > 0 {
		return found[0]
	}
	return nil
}

func attr(node *html.Node, name string) string {
	if node == nil {
		return ""
	}
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val
		}
	}
	return ""
}

func hasClass(node *html.Node, class string) bool {
	for _, candidate := range strings.Fields(attr(node, "class")) {
		if candidate == class {
			return true
		}
	}
	return false
}

func byClass(tag, class string) func(*html.Node) bool {
	return func(node *html.Node) bool {
		return (tag == "" || node.Data == tag) && hasClass(node, class)
	}
}

func byID(id string) func(*html.Node) bool {
	return func(node *html.Node) bool { return attr(node, "id") == id }
}

// hidden reports an inline display:none, which WebReg uses to switch between
// pre-rendered course-bin states.
func hidden(node *html.Node) bool {
	style := strings.ReplaceAll(strings.ToLower(attr(node, "style")), " ", "")
	return strings.Contains(style, "display:none")
}

// textOf returns node's text with <br> as a line break. Scripts are skipped.
func textOf(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		switch {
		case current.Type == html.TextNode:
			builder.WriteString(current.Data)
		case current.Type == html.ElementNode && current.Data == "br":
			builder.WriteString("\n")
		case current.Type == html.ElementNode && (current.Data == "script" || current.Data == "style"):
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	if node != nil {
		walk(node)
	}
	return builder.String()
}

func collapse(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// lines splits text at <br> boundaries and drops empty entries.
func lines(text string) []string {
	var result []string
	for _, line := range strings.Split(text, "\n") {
		if line = collapse(line); line != "" {
			result = append(result, line)
		}
	}
	return result
}

// formFields returns a form's named input values.
func formFields(form *html.Node) map[string]string {
	fields := make(map[string]string)
	for _, input := range findAll(form, func(node *html.Node) bool { return node.Data == "input" }) {
		if name := attr(input, "name"); name != "" {
			fields[name] = attr(input, "value")
		}
	}
	return fields
}

func requestToken(document *html.Node) string {
	input := findFirst(document, func(node *html.Node) bool {
		return node.Data == "input" && attr(node, "name") == "__RequestVerificationToken" && attr(node, "value") != ""
	})
	return attr(input, "value")
}
