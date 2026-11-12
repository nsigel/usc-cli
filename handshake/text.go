package handshake

import (
	"html"
	"regexp"
	"strings"
)

var (
	htmlBreak  = regexp.MustCompile(`(?i)</?(?:br|p|div|li|h[1-6])\b[^>]*>`)
	htmlTag    = regexp.MustCompile(`<[^>]+>`)
	space      = regexp.MustCompile(`[\t\r ]+`)
	blankLines = regexp.MustCompile(`\n{3,}`)
)

// plainText renders a rich-text description as paragraphs of plain text.
func plainText(value string) string {
	value = htmlBreak.ReplaceAllString(value, "\n")
	value = htmlTag.ReplaceAllString(value, "")
	value = html.UnescapeString(value)
	lines := strings.Split(value, "\n")
	for index := range lines {
		lines[index] = strings.TrimSpace(space.ReplaceAllString(lines[index], " "))
	}
	return strings.TrimSpace(blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

// slug lowercases value and joins its alphanumeric runs with hyphens, so
// "In Person" and "in-person" compare equal.
func slug(value string) string {
	var result strings.Builder
	separator := false
	for _, char := range strings.ToLower(strings.TrimSpace(value)) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			if separator && result.Len() > 0 {
				result.WriteByte('-')
			}
			separator = false
			result.WriteRune(char)
		} else {
			separator = true
		}
	}
	return result.String()
}
