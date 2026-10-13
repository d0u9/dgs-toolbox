package view

import (
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/tree"
)

func skeletonItem(id, typ, owner, country, name string) tree.Item {
	return tree.Item{ID: id, Type: typ, Kind: tree.KindDocument, Fields: map[string]string{"owner": owner, "country": country, "name": name}, Head: id, Revisions: []tree.Revision{{ID: id, Digest: strings.Repeat("a", 64), Path: id + ".pdf", Added: "2020-01-01"}}}
}
func skeletonTypes() Types {
	return TypesOf([]tree.Template{{Type: "passport", Lineage: []string{"passport"}}, {Type: "id_card", Lineage: []string{"id_card"}}, {Type: "tenancy", Anchor: true, Lineage: []string{"tenancy"}}, {Type: "bill", Lineage: []string{"bill"}}})
}

func TestSkeletonEachFirstMatchNumbersAndPins(t *testing.T) {
	items := []tree.Item{skeletonItem("passport-a", "passport", "alice", "中国", "护照"), skeletonItem("id-a", "id_card", "alice", "中国", "身份证"), skeletonItem("passport-b", "passport", "bob", "澳大利亚", "护照")}
	folders := []Folder{{Name: "{#}-{owner}", Each: "owner", Order: Values{"alice", "bob"}, Folders: []Folder{{Name: "01-CHN", If: "country == 中国", Folders: []Folder{{Name: "00-重要证照", Take: Values{"passport", "id_card"}, File: "{#}-{name}", Order: Values{"身份证", "护照"}}}}, {Name: "02-AUS", If: "country == 澳大利亚", Folders: []Folder{{Name: "IDs", Take: Values{"passport"}}}}}}, {Name: "fallback", Take: Values{"passport", "id_card"}}}
	assigned := map[string]bool{}
	result, err := BuildFolders(folders, items, skeletonTypes(), Defaults{}, nil, "", assigned, map[string]map[int]string{}, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, f := range result.Files {
		paths[f.Item] = f.Path
	}
	if paths["id-a"] != "01-alice/01-CHN/00-重要证照/01-身份证.pdf" || paths["passport-a"] != "01-alice/01-CHN/00-重要证照/02-护照.pdf" || paths["passport-b"] != "02-bob/02-AUS/IDs/01-护照.pdf" {
		t.Fatal(paths)
	}
	pins := map[string]Pin{"fallback": {Items: []string{"passport-a"}, Numbers: map[string]int{"passport-a": 9}}}
	result, err = BuildFolders(folders, items, skeletonTypes(), Defaults{}, pins, "", map[string]bool{}, map[string]map[int]string{}, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	paths = map[string]string{}
	for _, f := range result.Files {
		paths[f.Item] = f.Path
	}
	if paths["passport-a"] != "fallback/09-护照.pdf" {
		t.Fatal("pin", paths)
	}
}

func TestSkeletonAnchorAndValidityPerRevision(t *testing.T) {
	tenant := skeletonItem("home", "tenancy", "alice", "澳大利亚", "Lease")
	tenant.Fields["address"] = "Example Street"
	tenant.Fields["date"] = "2020-01-01/2030-12-31"
	bill := skeletonItem("rent", "bill", "alice", "澳大利亚", "Rent")
	bill.Fields["about"] = "home"
	result, err := BuildFolders([]Folder{{Name: "{address}/{date.start}-{date.end}", Each: "tenancy"}}, []tree.Item{tenant, bill}, skeletonTypes(), Defaults{}, nil, "11-Rental", map[string]bool{}, map[string]map[int]string{}, time.Unix(0, 0))
	if err != nil || len(result.Files) != 2 || !strings.HasPrefix(result.Files[0].Path, "11-Rental/Example Street/2020-01-01-2030-12-31/") {
		t.Fatal(result, err)
	}
	card := skeletonItem("card", "passport", "alice", "中国", "护照")
	card.Revisions = []tree.Revision{{ID: "old", Paper: "old", Digest: strings.Repeat("a", 64), Path: "old.pdf", Snapshot: true, Fields: map[string]string{"name": "护照", "date": "2016-01-01/2020-12-31"}}, {ID: "corrected", Paper: "old", Digest: strings.Repeat("a", 64), Path: "old.pdf", Snapshot: true, Fields: map[string]string{"name": "护照", "date": "2016-01-01/2021-12-31"}}, {ID: "new", Paper: "new", Digest: strings.Repeat("b", 64), Path: "new.pdf", Snapshot: true, Fields: map[string]string{"name": "护照", "date": "2026-01-01/2036-12-31"}}}
	card.Head = "new"
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	folder := []Folder{{Name: "IDs", Take: Values{"passport"}}}
	current, err := BuildFolders(folder, []tree.Item{card}, skeletonTypes(), Defaults{Revision: All, Valid: "today"}, nil, "", map[string]bool{}, map[string]map[int]string{}, now)
	if err != nil || len(current.Files) != 1 || current.Files[0].Revision != 3 {
		t.Fatal(current, err)
	}
	expired, err := BuildFolders(folder, []tree.Item{card}, skeletonTypes(), Defaults{Revision: All, Valid: "expired"}, nil, "", map[string]bool{}, map[string]map[int]string{}, now)
	if err != nil || len(expired.Files) != 1 || expired.Files[0].Revision != 2 {
		t.Fatal(expired, err)
	}
	all, err := BuildFolders(folder, []tree.Item{card}, skeletonTypes(), Defaults{Revision: All}, nil, "", map[string]bool{}, map[string]map[int]string{}, now)
	if err != nil || len(all.Files) != 2 || all.Files[0].Path != "IDs/01-护照-2016.pdf" || all.Files[1].Path != "IDs/01-护照-2026.pdf" {
		t.Fatal(all, err)
	}
	// Explicit extensions still need distinct revision names for same-year scans.
	card.Revisions[2].Fields["date"] = "2016-06-01/2036-12-31"
	folder[0].File = "{#}-{name}.{ext}"
	all, err = BuildFolders(folder, []tree.Item{card}, skeletonTypes(), Defaults{Revision: All}, nil, "", map[string]bool{}, map[string]map[int]string{}, now)
	if err != nil || len(all.Files) != 2 || all.Files[0].Path == all.Files[1].Path || len(all.Clashes) != 0 || !strings.HasSuffix(all.Files[0].Path, ".pdf") {
		t.Fatal("same-year revision names", all, err)
	}
	card.SupersededBy = "replacement"
	current, err = BuildFolders(folder, []tree.Item{card}, skeletonTypes(), Defaults{Revision: Head, Valid: "today"}, nil, "", map[string]bool{}, map[string]map[int]string{}, now)
	if err != nil || len(current.Files) != 0 {
		t.Fatal("superseded current", current, err)
	}
}

func TestSkeletonDefaultsOverrideAndRejectsUnsafeNames(t *testing.T) {
	item := skeletonItem("a", "passport", "alice", "中国", "Passport")
	item.Fields["date.end"] = "2020-01-01"
	folder := []Folder{{Name: "IDs", Take: Values{"passport"}, Defaults: Defaults{Valid: "expired"}}}
	result, err := BuildFolders(folder, []tree.Item{item}, skeletonTypes(), Defaults{Valid: "today"}, nil, "", map[string]bool{}, map[string]map[int]string{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(result.Files) != 1 {
		t.Fatal("override", result, err)
	}
	for _, name := range []string{"../outside", "/absolute", "bad\\name", "{#label}-folder"} {
		_, err := BuildFolders([]Folder{{Name: name, Take: Values{"passport"}}}, []tree.Item{item}, skeletonTypes(), Defaults{}, nil, "", map[string]bool{}, map[string]map[int]string{}, time.Now())
		if err == nil {
			t.Fatal("unsafe name accepted", name)
		}
	}
}

func TestSkeletonNumberingSharesFolderAndReservesPinnedNames(t *testing.T) {
	items := []tree.Item{skeletonItem("a", "id_card", "alice", "中国", "Card"), skeletonItem("b", "passport", "alice", "中国", "Passport")}
	folders := []Folder{{Name: "IDs", Take: Values{"id_card"}}, {Name: "IDs", Take: Values{"passport"}}}
	result, err := BuildFolders(folders, items, skeletonTypes(), Defaults{}, map[string]Pin{"IDs": {Numbers: map[string]int{"Future card": 1}}}, "", map[string]bool{}, map[string]map[int]string{}, time.Unix(0, 0))
	if err != nil || len(result.Files) != 2 || result.Files[0].Path != "IDs/02-Card.pdf" || result.Files[1].Path != "IDs/03-Passport.pdf" {
		t.Fatal(result, err)
	}
}

func TestSkeletonEachDropsEmptyFolders(t *testing.T) {
	a := skeletonItem("a", "passport", "alice", "x", "A")
	a.Fields["topic"], a.Fields["focus"] = "PL", "Rust"
	b := skeletonItem("b", "passport", "alice", "x", "B")
	b.Fields["topic"], b.Fields["focus"] = "OS", "Kernel"
	folders := []Folder{
		{Name: "{topic}", Each: "topic", Folders: []Folder{{Name: "{focus}", Each: "focus", If: "topic == PL", Take: Values{"passport"}}}},
		{Name: "{topic}", Each: "topic", Take: Values{"passport"}, Folders: []Folder{{Name: "literal"}}},
	}
	r, err := BuildFolders(folders, []tree.Item{a, b}, skeletonTypes(), Defaults{}, nil, "", map[string]bool{}, map[string]map[int]string{}, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.Folders, " "); got != "PL PL/Rust OS OS/literal" {
		t.Fatal(got)
	}
	// A dropped group leaves no gap in {#}.
	r, err = BuildFolders([]Folder{{Name: "{#}-{topic}", Each: "topic", If: "topic != OS", Take: Values{"passport"}}}, []tree.Item{b, a}, skeletonTypes(), Defaults{}, nil, "", map[string]bool{}, map[string]map[int]string{}, time.Unix(0, 0))
	if err != nil || strings.Join(r.Folders, " ") != "01-PL" {
		t.Fatal(r.Folders, err)
	}
}

func TestSkeletonEachStart(t *testing.T) {
	a := skeletonItem("a", "passport", "alice", "x", "A")
	a.Fields["topic"] = "PL"
	r, err := BuildFolders([]Folder{{Name: "{#}-{topic}", Each: "topic", Start: 3, Take: Values{"passport"}}}, []tree.Item{a}, skeletonTypes(), Defaults{}, nil, "", map[string]bool{}, map[string]map[int]string{}, time.Unix(0, 0))
	if err != nil || strings.Join(r.Folders, " ") != "03-PL" {
		t.Fatal(r.Folders, err)
	}
}

func TestSkeletonChildrenFirstAndTakeNarrows(t *testing.T) {
	a := skeletonItem("a", "passport", "alice", "x", "A")
	a.Fields["topic"], a.Fields["focus"] = "PL", "Rust"
	b := skeletonItem("b", "passport", "alice", "x", "B")
	b.Fields["topic"] = "PL"
	c := skeletonItem("c", "bill", "alice", "x", "C")
	c.Fields["topic"], c.Fields["focus"] = "PL", "Rust"
	// The parent keeps what its child leaves; the bill never reaches the child.
	folders := []Folder{{Name: "{topic}", Each: "topic", Take: Values{"passport"}, Folders: []Folder{{Name: "{focus}", Each: "focus"}}}}
	r, err := BuildFolders(folders, []tree.Item{a, b, c}, skeletonTypes(), Defaults{}, nil, "", map[string]bool{}, map[string]map[int]string{}, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, f := range r.Files {
		paths[f.Item] = f.Path
	}
	if paths["a"] != "PL/Rust/01-A.pdf" || paths["b"] != "PL/01-B.pdf" || paths["c"] != "" {
		t.Fatal(paths)
	}
}
