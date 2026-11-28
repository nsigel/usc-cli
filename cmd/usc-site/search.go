package main

import (
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strings"
	"unicode"
)

// maxResults is the most sections one search returns.
const maxResults = 5

// section is one heading of a docs page and the Markdown under it.
type section struct {
	page    string
	heading string
	url     string
	text    string
	words   map[string]int
	titles  map[string]bool
}

// stopWords are left out of queries so natural-language questions rank by
// their meaningful terms.
var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "can": true, "do": true, "does": true,
	"for": true, "how": true, "i": true, "in": true, "is": true, "it": true, "my": true,
	"of": true, "on": true, "or": true, "the": true, "to": true, "what": true, "with": true,
}

// search serves GET /.well-known/search?q=: the docs sections that best
// match the query, as Markdown.
func search(files fs.FS, origin string) http.Handler {
	sections := indexDocs(files, origin)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if query == "" {
			http.Error(w, "missing query: pass ?q=", http.StatusBadRequest)
			return
		}
		var terms []string
		for _, term := range words(query) {
			if !stopWords[term] && !slices.Contains(terms, term) {
				terms = append(terms, term)
			}
		}

		type match struct {
			section        *section
			matched, score int
		}
		var matches []match
		for i := range sections {
			s := &sections[i]
			var m match
			for _, term := range terms {
				count := s.words[term]
				if s.titles[term] {
					count += 5
				}
				if count > 0 {
					m.matched++
					m.score += count
				}
			}
			if m.matched > 0 {
				m.section = s
				matches = append(matches, m)
			}
		}
		slices.SortStableFunc(matches, func(a, b match) int {
			if a.matched != b.matched {
				return b.matched - a.matched
			}
			return b.score - a.score
		})

		var body strings.Builder
		fmt.Fprintf(&body, "# Search results for: %q\n", query)
		shown := min(len(matches), maxResults)
		switch {
		case len(matches) == 0:
			fmt.Fprintf(&body, "\nNo matching sections found. Try different keywords, or see the [documentation index](%s/llms.txt).\n", origin)
		case len(matches) > shown:
			fmt.Fprintf(&body, "\nShowing %d of %d matching sections.\n", shown, len(matches))
		}
		for _, m := range matches[:shown] {
			s := m.section
			title := s.page
			if s.heading != "" {
				title += " — " + s.heading
			}
			fmt.Fprintf(&body, "\n## %s\n\nSource: %s\n\n%s\n", title, s.url, s.text)
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = w.Write([]byte(body.String()))
	})
}

// indexDocs splits each docs page's Markdown version into sections at its
// second-level headings.
func indexDocs(files fs.FS, origin string) []section {
	var sections []section
	pages, _ := fs.Glob(files, "docs/*/index.md")
	pages = append([]string{"docs/index.md"}, pages...)
	for _, name := range pages {
		data, err := fs.ReadFile(files, name)
		if err != nil {
			continue
		}
		front, body, _ := strings.Cut(strings.TrimPrefix(string(data), "---\n"), "\n---\n")
		page := frontField(front, "title")
		pageURL := origin + "/" + path.Dir(name) + "/"
		body, _ = strings.CutPrefix(strings.TrimSpace(body), "# "+page+"\n")

		slugs := map[string]int{}
		current := section{page: page, url: pageURL}
		var text []string
		flush := func() {
			current.text = strings.TrimSpace(strings.Join(text, "\n"))
			if current.text != "" {
				current.words = map[string]int{}
				for _, word := range words(current.text) {
					current.words[word]++
				}
				current.titles = map[string]bool{}
				for _, word := range words(page + " " + current.heading) {
					current.titles[word] = true
				}
				sections = append(sections, current)
			}
			text = nil
		}
		fenced := false
		for line := range strings.SplitSeq(body, "\n") {
			if strings.HasPrefix(line, "```") {
				fenced = !fenced
			}
			if heading, ok := strings.CutPrefix(line, "## "); ok && !fenced {
				flush()
				slug := slugify(heading)
				if n := slugs[slug]; n > 0 {
					slugs[slug]++
					slug = fmt.Sprintf("%s-%d", slug, n)
				} else {
					slugs[slug] = 1
				}
				current = section{page: page, heading: heading, url: pageURL + "#" + slug}
				continue
			}
			text = append(text, line)
		}
		flush()
	}
	return sections
}

// frontField returns a quoted field from a page's YAML front matter.
func frontField(front, key string) string {
	for line := range strings.SplitSeq(front, "\n") {
		if value, ok := strings.CutPrefix(line, key+": "); ok {
			return strings.Trim(value, `"`)
		}
	}
	return ""
}

// words returns the lowercase letter and digit runs in text.
func words(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// slugify returns the anchor the site gives a heading.
func slugify(heading string) string {
	var slug strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
			slug.WriteRune(r)
		case r == ' ':
			slug.WriteRune('-')
		}
	}
	return slug.String()
}
