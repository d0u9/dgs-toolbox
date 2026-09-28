package tree

import "testing"

func TestMultipleSelectKeepsOptionsOrder(t *testing.T) {
	f := Field{Key: "service", Type: FieldSelect, Multiple: true, Options: []string{"电", "水", "气"}}
	got, err := f.clean("气,电 , 电")
	if err != nil || got != "电, 气" {
		t.Fatalf("clean = %q, %v", got, err)
	}
	if _, err := f.clean("电, 油"); err == nil {
		t.Fatal("took a value that is not an option")
	}
	one := Template{Type: "bill", Kind: KindRecord, Fields: []Field{{Key: "service", Type: FieldText, Multiple: true}}}
	if err := one.Validate(); err == nil {
		t.Fatal("took multiple on a text field")
	}
}
