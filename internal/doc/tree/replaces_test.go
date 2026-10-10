package tree

import (
	"os"
	"strings"
	"testing"
)

// A Template's replaces names what a replacing Item shares with the old one;
// left out, a licence needs only its owner and a visa keeps owner and country.
func TestReplacesByTemplate(t *testing.T) {
	s := inPlaceStore(t)
	licence := "type: licence\nkind: document\nfields:\n  - key: owner\n    required: true\n  - key: country\n    type: country\n"
	write := func(name, text string) {
		if err := os.WriteFile(s.TemplatePath(strings.TrimSuffix(name, ".yaml")), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("licence.yaml", licence)
	write("visa.yaml", strings.Replace(licence, "licence", "visa", 1))
	for _, it := range []Item{
		{ID: "LICENCE", Type: "licence", Kind: KindDocument, Fields: map[string]string{"owner": "alice", "country": "CN"}},
		{ID: "VISA", Type: "visa", Kind: KindDocument, Fields: map[string]string{"owner": "alice", "country": "AU"}},
	} {
		if err := s.WriteItem(it); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ValidateSupersession("LICENCE", "licence", map[string]string{"owner": "alice", "country": "AU"}); err != nil {
		t.Fatalf("licence across countries: %v", err)
	}
	if err := s.ValidateSupersession("LICENCE", "licence", map[string]string{"owner": "bob", "country": "CN"}); err == nil {
		t.Fatal("licence of another owner accepted")
	}
	if err := s.ValidateSupersession("VISA", "visa", map[string]string{"owner": "alice", "country": "NZ"}); err == nil {
		t.Fatal("visa across countries accepted")
	}
	if err := s.ValidateSupersession("VISA", "visa", map[string]string{"owner": "alice", "country": "澳大利亚"}); err != nil {
		t.Fatalf("visa same country: %v", err)
	}
	write("licence.yaml", licence+"replaces: [owner, country]\n")
	if err := s.ValidateSupersession("LICENCE", "licence", map[string]string{"owner": "alice", "country": "AU"}); err == nil {
		t.Fatal("replaces country ignored")
	}
}

// A child adds to its parent's replaces; a field it does not have is refused.
func TestReplacesInheritAndValidate(t *testing.T) {
	parent := Template{Type: "card", Abstract: true, Fields: []Field{{Key: "owner", Required: true}, {Key: "country", Type: FieldCountry, Required: true}}, Replaces: []string{"owner"}}
	child := Template{Type: "bank_card", Extends: "card", Kind: KindDocument, Replaces: []string{"country"}}
	got, err := Resolve([]Template{parent, child})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range got {
		if r.Type == "bank_card" && strings.Join(r.Replaces, ",") != "owner,country" {
			t.Fatalf("replaces = %v", r.Replaces)
		}
	}
	bad := Template{Type: "x", Kind: KindRecord, Fields: []Field{{Key: "owner"}}, Replaces: []string{"number"}}
	if err := bad.Validate(); err == nil {
		t.Fatal("replaces of a missing field accepted")
	}
}
