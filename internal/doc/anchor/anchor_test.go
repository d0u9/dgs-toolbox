package anchor

import (
	"reflect"
	"testing"

	"dgs-toolbox/internal/doc/tree"
)

func item(id, typ string, fields map[string]string) tree.Item {
	return tree.Item{ID: id, Type: typ, Fields: fields, Revisions: []tree.Revision{{Digest: id + "d"}}}
}

func TestHolds(t *testing.T) {
	for _, c := range []struct {
		date, from, to string
		want           bool
	}{
		{"2011-03-15", "2011-01-01", "2012-01-01", true},
		{"2012-01-01", "2011-01-01", "2012-01-01", true},
		{"2012-01-02", "2011-01-01", "2012-01-01", false},
		{"2011-12", "2011-12-15", "", true},  // a month shares a day with the span
		{"2011-11", "2011-12-15", "", false}, // and this one does not
		{"2030-01-01", "2011-01-01", "", true},
		{"soon", "2011-01-01", "", false},
	} {
		if got := Holds(c.date, c.from, c.to); got != c.want {
			t.Errorf("Holds(%q, %q, %q) = %v", c.date, c.from, c.to, got)
		}
	}
}

func TestProposals(t *testing.T) {
	bill := tree.Template{Type: "bill", Fields: []tree.Field{
		{Key: "period", Type: tree.FieldMonth},
		{Key: "tenancy", Type: tree.FieldItem, Match: map[string]string{"type": "=tenancy"},
			Within: &tree.Within{Date: "period", From: "start", To: "end"}},
	}}
	items := []tree.Item{
		item("T1", "tenancy", map[string]string{"start": "2010-01-01", "end": "2011-01-01"}),
		item("T2", "tenancy", map[string]string{"start": "2011-01-01", "end": "2012-01-01"}),
		item("C1", "contract", map[string]string{"start": "2010-01-01", "end": "2011-01-01"}),
		item("B1", "bill", map[string]string{"period": "2010-06"}),
		item("B2", "bill", map[string]string{"period": "2011-01"}), // the month both hold
		item("B3", "bill", map[string]string{"period": "2013-06"}), // none holds
		item("B4", "bill", map[string]string{"period": "2010-06", "tenancy": "T2"}),
	}
	proposals, unsure := Proposals([]tree.Template{bill}, items)
	if want := []Proposal{{Item: "B1", Key: "tenancy", Link: "T1"}}; !reflect.DeepEqual(proposals, want) {
		t.Fatalf("proposals %+v", proposals)
	}
	if want := []Unsure{{Item: "B2", Key: "tenancy", Fitting: []string{"T1", "T2"}}}; !reflect.DeepEqual(unsure, want) {
		t.Fatalf("unsure %+v", unsure)
	}
	if got := Offered(bill.Fields[1], "B1", nil, items); len(got) != 2 {
		t.Fatalf("offered %d, want the two tenancies", len(got))
	}
}
