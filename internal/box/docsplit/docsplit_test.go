package docsplit_test

import (
	"reflect"
	"strings"
	"testing"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/docsplit"
	"dgs-toolbox/internal/box/money"
	"dgs-toolbox/internal/box/pagerange"
	"dgs-toolbox/internal/box/sidecar"
)

func split() sidecar.File {
	return sidecar.File{
		Type:  "unsorted",
		Tags:  []string{"tax", "2024"},
		Pages: 10,
		Documents: []sidecar.Document{
			{Pages: []pagerange.Range{{From: 1, To: 3}}, Type: "statement", Description: "bank", Tags: []string{"bank", "tax"}},
			{Pages: []pagerange.Range{{From: 2, To: 5}}, Total: money.Amount{Minor: 1250, Currency: "AUD"}},
			{Pages: []pagerange.Range{{From: 6, To: 6}}, EventDate: box.Date{Year: 2024, Month: 7, Day: 1}, EventZone: "Asia/Tokyo"},
		},
		IgnoredPages: []pagerange.Range{{From: 7, To: 8}},
	}
}

func TestResolveInheritsAndOnlyAdds(t *testing.T) {
	file := split()
	if err := docsplit.Check(file); err != nil {
		t.Fatal(err)
	}
	got := docsplit.Resolve(file)
	if len(got) != 3 {
		t.Fatalf("got %d documents", len(got))
	}
	if got[0].Type != "statement" || got[0].Description != "bank" {
		t.Errorf("first: %+v", got[0])
	}
	if want := []string{"tax", "2024", "bank"}; !reflect.DeepEqual(got[0].Tags, want) {
		t.Errorf("tags: got %v, want %v", got[0].Tags, want)
	}
	if got[1].Type != "unsorted" || got[1].Total.Minor != 1250 {
		t.Errorf("second: %+v", got[1])
	}
	if want := []string{"tax", "2024"}; !reflect.DeepEqual(got[1].Tags, want) {
		t.Errorf("inherited tags: got %v", got[1].Tags)
	}
	if got[2].EventDate.Day != 1 || got[2].EventZone != "Asia/Tokyo" {
		t.Errorf("third: %+v", got[2])
	}
}

func TestCheckRefusesSettingWhatTheFileHas(t *testing.T) {
	for name, change := range map[string]func(*sidecar.File){
		"type":        func(f *sidecar.File) { f.Type = "receipt" },
		"description": func(f *sidecar.File) { f.Description = "batch" },
		"date":        func(f *sidecar.File) { f.EventDate = box.Date{Year: 2024, Month: 1, Day: 1} },
		"total":       func(f *sidecar.File) { f.Total = money.Amount{Minor: 1, Currency: "AUD"} },
		"zone":        func(f *sidecar.File) { f.EventZone = "Asia/Tokyo" },
	} {
		file := split()
		change(&file)
		if err := docsplit.Check(file); err == nil {
			t.Errorf("%s: accepted a document overriding the file", name)
		}
	}
}

func TestCheckBounds(t *testing.T) {
	file := split()
	file.Documents[2].Pages = []pagerange.Range{{From: 6, To: 11}}
	if err := docsplit.Check(file); err == nil || !strings.Contains(err.Error(), "past the last page") {
		t.Errorf("got %v", err)
	}
	file = split()
	file.IgnoredPages = []pagerange.Range{{From: 11, To: 11}}
	if err := docsplit.Check(file); err == nil {
		t.Error("ignored page past the end accepted")
	}
}

func TestCoverFindsForgottenPages(t *testing.T) {
	got := docsplit.Cover(split()).Unassigned
	if want := []int{9, 10}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestUnsplitIsOneDocument(t *testing.T) {
	file := sidecar.File{Type: "receipt", Pages: 4}
	got := docsplit.Resolve(file)
	if len(got) != 1 || got[0].Type != "receipt" || pagerange.Format(got[0].Pages) != "1-4" {
		t.Fatalf("got %+v", got)
	}
	if len(docsplit.Cover(file).Unassigned) != 0 {
		t.Error("unsplit file has unassigned pages")
	}
}

func TestCheckRefusesAnIgnoredPageInADocument(t *testing.T) {
	file := split()
	file.IgnoredPages = []pagerange.Range{{From: 3, To: 3}}
	if err := docsplit.Check(file); err == nil {
		t.Fatal("page 3 is ignored and in document 1")
	}
}
