package search

import (
	"strings"
	"testing"
)

var docs = []Doc{
	{ID: "a", Text: "Australian Passport\nSurname SMITH\nGiven names JANE"},
	{ID: "b", Text: "Electricity bill\nAccount for Jane Smith\nAmount due 120.00"},
	{ID: "c", Text: "中华人民共和国居民身份证\n姓名 张三"},
	{ID: "d", Text: "Australian Passport\nSurname BROWN\nGiven names TOM"},
}

func ids(hits []Hit) string {
	var out []string
	for _, h := range hits {
		out = append(out, h.ID)
	}
	return strings.Join(out, ",")
}

func TestFind(t *testing.T) {
	for query, want := range map[string]string{
		"jane":          "a,b",
		"JANE smith":    "a,b",
		"passport jane": "a",
		"身份证":           "c",
		"张":             "c",
		"nothing":       "",
		"  ":            "",
	} {
		if got := ids(Find(docs, query, DefaultSnippet)); got != want {
			t.Errorf("Find(%q) = %s, want %s", query, got, want)
		}
	}
	hit := Find(docs, "amount", 20)[0]
	if !strings.Contains(hit.Snippet, "Amount due") || strings.Contains(hit.Snippet, "\n") {
		t.Errorf("snippet %q", hit.Snippet)
	}
	if s := Find(docs, "姓名", 8)[0].Snippet; !strings.Contains(s, "姓名") {
		t.Errorf("CJK snippet %q", s)
	}
}

func TestMore(t *testing.T) {
	if got := ids(More(docs, "a", DefaultSimilar, 10)); !strings.HasPrefix(got, "d") || strings.Contains(got, "a") {
		t.Errorf("More(a) = %s", got)
	}
	if got := More(docs, "a", DefaultSimilar, 1); len(got) != 1 {
		t.Errorf("limit: %v", got)
	}
	if got := More(docs, "missing", 0, 10); len(got) != 0 {
		t.Errorf("unknown: %v", got)
	}
}
