package tree

import (
	"strings"
	"testing"
)

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

func TestMultipleItemKeepsIDsOnce(t *testing.T) {
	a, b := "01M4TAPN1KG778GCHW2ARHM45M", "01M4TJXMKDD5E8XT3XPEMZB6AM"
	f := Field{Key: "about", Type: FieldItem, Multiple: true}
	got, err := f.clean(a + "," + b + " , " + a)
	if err != nil || got != a+", "+b {
		t.Fatalf("clean = %q, %v", got, err)
	}
	if _, err := f.clean(a + ", trip"); err == nil {
		t.Fatal("took a value that is not an Item ID")
	}
	if ids := ItemLinks(got); len(ids) != 2 || ids[1] != b {
		t.Fatal("ItemLinks", ids)
	}
	if ItemLinks("alice, bob") != nil {
		t.Fatal("ItemLinks read names as IDs")
	}
	one := Template{Type: "ticket", Kind: KindRecord, Fields: []Field{{Key: "about", Type: FieldItem, Multiple: true}}}
	if err := one.Validate(); err != nil && strings.Contains(err.Error(), "multiple") {
		t.Fatal("refused multiple on an item field:", err)
	}
}
