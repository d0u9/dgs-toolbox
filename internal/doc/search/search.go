// Package search finds Items by the text read off their PDFs: every word of
// a query, and "more like this" by the same term similarity that suggests a
// type (classify). It works on plain text; where the text comes from is the
// caller's.
package search

import (
	"sort"
	"strings"
	"unicode"

	"dgs-toolbox/internal/doc/classify"
)

// Doc is one Item's text.
type Doc struct {
	ID   string
	Text string
}

// Hit is a Doc that matched, with a line of its text around the first word.
type Hit struct {
	ID      string  `json:"id"`
	Snippet string  `json:"snippet,omitempty"`
	Score   float64 `json:"score,omitempty"`
}

// DefaultSnippet is how many characters a snippet keeps around the match.
const DefaultSnippet = 60

// Words is a query split on space. Case is ignored.
func Words(query string) []string {
	return strings.Fields(strings.ToLower(query))
}

// Find is every Doc containing every word of query, ignoring case, in the
// order given. A word matches inside a longer one, so a query in Chinese,
// which has no spaces, matches as it is typed. An empty query finds nothing.
func Find(docs []Doc, query string, snippet int) []Hit {
	words := Words(query)
	hits := []Hit{}
	if len(words) == 0 {
		return hits
	}
	for _, d := range docs {
		lower := strings.ToLower(d.Text)
		first := -1
		for _, w := range words {
			at := strings.Index(lower, w)
			if at < 0 {
				first = -2
				break
			}
			if first == -1 || at < first {
				first = at
			}
		}
		if first >= 0 {
			hits = append(hits, Hit{ID: d.ID, Snippet: around(d.Text, lower, first, snippet)})
		}
	}
	return hits
}

// around is the text near byte offset at, on one line, cut at rune
// boundaries. lower is text lowercased; it can differ in length from text,
// so the cut is made on lower when it does.
func around(text, lower string, at, width int) string {
	src := text
	if len(lower) != len(text) {
		src = lower
	}
	start, end := at-width/2, at+width/2
	if start < 0 {
		start = 0
	}
	if end > len(src) {
		end = len(src)
	}
	for start > 0 && !startOfRune(src[start]) {
		start--
	}
	for end < len(src) && !startOfRune(src[end]) {
		end++
	}
	out := strings.Join(strings.FieldsFunc(src[start:end], func(r rune) bool { return r == '\n' || unicode.IsSpace(r) }), " ")
	if start > 0 {
		out = "…" + out
	}
	if end < len(src) {
		out += "…"
	}
	return out
}

func startOfRune(b byte) bool { return b&0xC0 != 0x80 }

// DefaultSimilar is the least similarity More reports.
const DefaultSimilar = 0.2

// More is the Docs most like the one with id, most alike first, at least
// min alike, at most limit of them. The Doc itself is not among them.
func More(docs []Doc, id string, min float64, limit int) []Hit {
	var terms map[string]float64
	for _, d := range docs {
		if d.ID == id {
			terms = classify.Terms(d.Text)
		}
	}
	hits := []Hit{}
	if len(terms) == 0 {
		return hits
	}
	for _, d := range docs {
		if d.ID == id {
			continue
		}
		if score := classify.Cosine(terms, classify.Terms(d.Text)); score >= min {
			hits = append(hits, Hit{ID: d.ID, Score: score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}
