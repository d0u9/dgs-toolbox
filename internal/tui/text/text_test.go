package text

import (
	"strings"
	"testing"
)

func TestPlural(t *testing.T) {
	cases := []struct {
		count int
		noun  string
		want  string
	}{
		{1, "target", "1 target"},
		{0, "target", "0 targets"},
		{2, "target", "2 targets"},
		{1, "party", "1 party"},
		{2, "party", "2 parties"},
	}
	for _, c := range cases {
		if got := Plural(c.count, c.noun); got != c.want {
			t.Fatalf("Plural(%d, %q) = %q, want %q", c.count, c.noun, got, c.want)
		}
	}
}

func TestWrapped_BreaksInsideAWordWithNoSpaces(t *testing.T) {
	got := Wrapped("abcdefghij", 4)
	want := []string{"abcd", "efgh", "ij"}
	if len(got) != len(want) {
		t.Fatalf("Wrapped = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Wrapped = %v, want %v", got, want)
		}
	}
}

func TestFit_PadsShortContentToHeight(t *testing.T) {
	got := Fit("one\ntwo", 4, 5)
	lines := 1
	for _, c := range got {
		if c == '\n' {
			lines++
		}
	}
	if lines != 4 {
		t.Fatalf("Fit produced %d lines, want 4", lines)
	}
}

func TestFit_TruncatesLongContentToHeight(t *testing.T) {
	got := Fit("one\ntwo\nthree\nfour", 2, 5)
	want := "one\ntwo"
	if got != want {
		t.Fatalf("Fit = %q, want %q", got, want)
	}
}

func TestHanging_TabularRowContinuesUnderItsLastColumn(t *testing.T) {
	got := Hanging("  a  —  no client file for this", 20)
	want := []string{"  a  —  no client", "        file for", "        this"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("Hanging = %q, want %q", got, want)
	}
}

func TestHanging_ProseContinuesUnderItsIndent(t *testing.T) {
	got := Hanging("  one two three four", 10)
	for _, row := range got[1:] {
		if !strings.HasPrefix(row, "  ") {
			t.Fatalf("Hanging = %q, want every row under the indent", got)
		}
	}
	if got := Hanging("short", 10); len(got) != 1 || got[0] != "short" {
		t.Fatalf("Hanging(short) = %q", got)
	}
}
