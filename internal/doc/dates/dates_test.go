package dates

import (
	"reflect"
	"testing"
)

func values(f []Found) []string {
	var out []string
	for _, x := range f {
		out = append(out, x.Value)
	}
	return out
}

func TestFind(t *testing.T) {
	text := "签发 2016.03.05 有效期 2016年3月5日-2036年03月05日 born 03/04/1990, paid 25/12/2025 bad 2026-02-30 ref 12345-06-07"
	got := values(Find(text, DMY))
	want := []string{"2016-03-05", "2016-03-05", "2036-03-05", "1990-04-03", "2025-12-25"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DMY got %v", got)
	}
	if got := values(Find("03/04/1990 12/25/2025", MDY)); !reflect.DeepEqual(got, []string{"1990-03-04", "2025-12-25"}) {
		t.Fatalf("MDY got %v", got)
	}
}

func TestFindOffsets(t *testing.T) {
	text := "on 2026-09-25."
	f := Find(text, DMY)
	if len(f) != 1 || text[f[0].Start:f[0].End] != "2026-09-25" {
		t.Fatalf("%+v", f)
	}
}

func TestParse(t *testing.T) {
	if v, ok := Parse(" 2030.01.02 ", DMY); !ok || v != "2030-01-02" {
		t.Fatal(v, ok)
	}
	for _, bad := range []string{"长期", "2030.01.02 x", "", "13/13/2020"} {
		if _, ok := Parse(bad, DMY); ok {
			t.Errorf("%q parsed", bad)
		}
	}
	if _, err := ParseOrder("ymd"); err == nil {
		t.Fatal("ymd accepted")
	}
}
