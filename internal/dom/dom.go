// Package dom holds the small HTML helpers the service clients share for
// reading server-rendered pages.
package dom

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// Parse returns the document for data. The HTML parser recovers from any
// markup, so a read failure yields an empty document instead of an error.
func Parse(data []byte) *html.Node {
	document, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return &html.Node{Type: html.DocumentNode}
	}
	return document
}

// FindAll returns the element descendants of node that match, in document order.
func FindAll(node *html.Node, match func(*html.Node) bool) []*html.Node {
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

// FindFirst returns the first element descendant of node that matches, or nil.
func FindFirst(node *html.Node, match func(*html.Node) bool) *html.Node {
	if node == nil {
		return nil
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && match(child) {
			return child
		}
		if found := FindFirst(child, match); found != nil {
			return found
		}
	}
	return nil
}

// Tag matches elements by name.
func Tag(name string) func(*html.Node) bool {
	return func(node *html.Node) bool { return node.Data == name }
}

// Class matches elements with a class; an empty tag matches any element.
func Class(tag, class string) func(*html.Node) bool {
	return func(node *html.Node) bool {
		return (tag == "" || node.Data == tag) && HasClass(node, class)
	}
}

// ID matches the element with an id.
func ID(id string) func(*html.Node) bool {
	return func(node *html.Node) bool { return Attr(node, "id") == id }
}

// Attr returns an attribute's value, or "" when node is nil or lacks it.
func Attr(node *html.Node, name string) string {
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

// HasAttr reports whether node carries an attribute, even an empty one.
func HasAttr(node *html.Node, name string) bool {
	if node == nil {
		return false
	}
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return true
		}
	}
	return false
}

// HasClass reports whether node's class list contains class.
func HasClass(node *html.Node, class string) bool {
	for _, candidate := range strings.Fields(Attr(node, "class")) {
		if candidate == class {
			return true
		}
	}
	return false
}

// Text returns the text a browser would show for node: <br> becomes a line
// break and script and style contents are skipped.
func Text(node *html.Node) string {
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

// RawText concatenates every text node under node, including script source.
func RawText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
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

// Collapse joins text's words with single spaces.
func Collapse(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
