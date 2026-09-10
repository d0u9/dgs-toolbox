package organizer

import (
	"fmt"
	"strings"
)

// Obsidian writes filename formats as Moment.js patterns and Go formats dates
// by example, so a pattern has to be translated rather than passed through. An
// unknown token is an error rather than a guess: writing a Capture into the
// wrong file is worse than refusing to write it.
var momentTokens = []struct {
	moment string
	layout string
}{
	{"YYYY", "2006"},
	{"YY", "06"},
	{"MMMM", "January"},
	{"MMM", "Jan"},
	{"MM", "01"},
	{"M", "1"},
	{"DDDD", ""}, // day of the year: no Go layout, handled by the caller
	{"dddd", "Monday"},
	{"ddd", "Mon"},
	{"DD", "02"},
	{"D", "2"},
	{"HH", "15"},
	{"hh", "03"},
	{"mm", "04"},
	{"ss", "05"},
	{"A", "PM"},
	{"a", "pm"},
}

// momentLayout translates a Moment.js pattern into a Go layout. Text inside
// square brackets is literal, as it is in Moment.
func momentLayout(pattern string) (string, error) {
	if strings.TrimSpace(pattern) == "" {
		return "2006-01-02", nil
	}
	var layout strings.Builder
	for index := 0; index < len(pattern); {
		if pattern[index] == '[' {
			end := strings.IndexByte(pattern[index:], ']')
			if end < 0 {
				return "", fmt.Errorf("unclosed [ in date format %q", pattern)
			}
			layout.WriteString(pattern[index+1 : index+end])
			index += end + 1
			continue
		}
		if token, replacement, ok := matchMomentToken(pattern[index:]); ok {
			if replacement == "" {
				return "", fmt.Errorf("date format %q uses %q, which has no equivalent here", pattern, token)
			}
			layout.WriteString(replacement)
			index += len(token)
			continue
		}
		if isMomentLetter(pattern[index]) {
			return "", fmt.Errorf("date format %q uses %q, which is not understood", pattern, pattern[index:index+1])
		}
		layout.WriteByte(pattern[index])
		index++
	}
	return layout.String(), nil
}

func matchMomentToken(text string) (token, replacement string, ok bool) {
	for _, candidate := range momentTokens {
		if strings.HasPrefix(text, candidate.moment) {
			return candidate.moment, candidate.layout, true
		}
	}
	return "", "", false
}

// isMomentLetter reports whether a character would be a format token rather
// than a separator. Letters are tokens in Moment unless bracketed.
func isMomentLetter(char byte) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
}
