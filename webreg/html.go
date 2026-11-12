package webreg

import (
	"strings"

	"github.com/nsigel/usc-cli/internal/dom"
	"golang.org/x/net/html"
)

func text(node *html.Node) string {
	return dom.Collapse(dom.Text(node))
}

func lines(node *html.Node) []string {
	var result []string
	for _, line := range strings.Split(dom.Text(node), "\n") {
		if line = dom.Collapse(line); line != "" {
			result = append(result, line)
		}
	}
	return result
}

// hidden reports an inline display:none, which WebReg uses to switch between
// pre-rendered course-bin states.
func hidden(node *html.Node) bool {
	style := strings.ReplaceAll(strings.ToLower(dom.Attr(node, "style")), " ", "")
	return strings.Contains(style, "display:none")
}

func formFields(form *html.Node) map[string]string {
	fields := make(map[string]string)
	for _, input := range dom.FindAll(form, dom.Tag("input")) {
		if name := dom.Attr(input, "name"); name != "" {
			fields[name] = dom.Attr(input, "value")
		}
	}
	return fields
}

const tokenField = "__RequestVerificationToken"

func requestToken(document *html.Node) string {
	input := dom.FindFirst(document, func(node *html.Node) bool {
		return node.Data == "input" && dom.Attr(node, "name") == tokenField && dom.Attr(node, "value") != ""
	})
	return dom.Attr(input, "value")
}
