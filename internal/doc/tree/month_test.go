package tree

import "testing"

func TestMonthFields(t *testing.T) {
	tpl, err := ParseTemplate([]byte("type: bank_card\nkind: document\nfields:\n  - key: expires\n    type: month\n    per_revision: true\ndefaults:\n  expires: 2030-02\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"2030-02", "2024-12"} {
		got, err := RevisionFields(tpl, map[string]string{"expires": value})
		if err != nil || got["expires"] != value {
			t.Fatalf("%s: %v, %v", value, got, err)
		}
	}
	for _, value := range []string{"2030-00", "2030-13", "2030-2", "02/30", "2030-02-01"} {
		if _, err := RevisionFields(tpl, map[string]string{"expires": value}); err == nil {
			t.Errorf("accepted %q", value)
		}
		tpl.Defaults["expires"] = value
		if err := tpl.Validate(); err == nil {
			t.Errorf("accepted default %q", value)
		}
	}
}
