// Package classify ranks types for a document by how much its text looks
// like the text of documents already filed under each. It learns nothing and
// stores nothing: every call compares the texts it is given.
package classify

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// DefaultThreshold is the score a type's best match must reach to be
// proposed. Cosine similarity of word counts is 1 for the same text; two
// scans of one kind of form usually score well above this, and unrelated
// documents well below.
const DefaultThreshold = 0.35

// Sample is the text of one filed document and its type.
type Sample struct {
	Type string
	Text string
}

// Score is one type's similarity to the text, taken from its best sample.
type Score struct {
	Type  string  `json:"type"`
	Score float64 `json:"score"`
}

// Rank scores every type among samples against text, best first; ties go by
// type name. A type whose samples share no word with the text scores 0 and is
// left out.
func Rank(samples []Sample, text string) []Score {
	target := Terms(text)
	best := map[string]float64{}
	for _, s := range samples {
		if score := Cosine(target, Terms(s.Text)); score > best[s.Type] {
			best[s.Type] = score
		}
	}
	out := make([]Score, 0, len(best))
	for t, score := range best {
		out = append(out, Score{Type: t, Score: score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Type < out[j].Type
	})
	return out
}

// Best is the top of ranked when it reaches threshold.
func Best(ranked []Score, threshold float64) (string, bool) {
	if len(ranked) == 0 || ranked[0].Score < threshold {
		return "", false
	}
	return ranked[0].Type, true
}

// Terms counts the words of text. A run of letters or digits is a word,
// lowercased, unless it is digits only: numbers are what differ between two
// copies of one form, so they say nothing about its type. Han, kana and
// hangul are not written with spaces, so a run of them counts as its
// overlapping pairs of characters, and a lone character as itself.
func Terms(text string) map[string]float64 {
	out := map[string]float64{}
	var word []rune
	var cjk []rune
	flushWord := func() {
		if len(word) > 0 && !allDigits(word) {
			out[strings.ToLower(string(word))]++
		}
		word = word[:0]
	}
	flushCJK := func() {
		switch {
		case len(cjk) == 1:
			out[string(cjk)]++
		case len(cjk) > 1:
			for i := 0; i+1 < len(cjk); i++ {
				out[string(cjk[i:i+2])]++
			}
		}
		cjk = cjk[:0]
	}
	for _, r := range text {
		switch {
		case isCJK(r):
			flushWord()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			word = append(word, r)
		default:
			flushWord()
			flushCJK()
		}
	}
	flushWord()
	flushCJK()
	return out
}

// Cosine is the cosine of the angle between two term counts: 1 for the same
// proportions, 0 for nothing in common.
func Cosine(a, b map[string]float64) float64 {
	if len(a) > len(b) {
		a, b = b, a
	}
	var dot, na, nb float64
	for t, x := range a {
		dot += x * b[t]
		na += x * x
	}
	for _, y := range b {
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r)
}

func allDigits(word []rune) bool {
	for _, r := range word {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
