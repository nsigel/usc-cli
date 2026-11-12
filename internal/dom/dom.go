package dom

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

func Parse(data []byte) *html.Node {
	// html.Parse fails only when its reader does, which a byte slice cannot.
	document, _ := html.Parse(bytes.NewReader(data))
	return document
}

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

func Tag(name string) func(*html.Node) bool {
	return func(node *html.Node) bool { return node.Data == name }
}

func Class(tag, class string) func(*html.Node) bool {
	return func(node *html.Node) bool {
		return (tag == "" || node.Data == tag) && HasClass(node, class)
	}
}

func ID(id string) func(*html.Node) bool {
	return func(node *html.Node) bool { return Attr(node, "id") == id }
}

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

func HasClass(node *html.Node, class string) bool {
	for _, candidate := range strings.Fields(Attr(node, "class")) {
		if candidate == class {
			return true
		}
	}
	return false
}

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

func Collapse(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
