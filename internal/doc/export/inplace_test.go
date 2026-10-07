package export

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/outline"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

func TestInPlaceExportAndMissingSourcePreflight(t *testing.T) {
	root, target := t.TempDir(), t.TempDir()
	if err := tree.InitWithOptions(root, time.Unix(0, 0), "in-place", 2); err != nil {
		t.Fatal(err)
	}
	st, err := tree.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "Books"), 0755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "Books", "card.pdf")
	if err := os.WriteFile(source, []byte("synthetic PDF"), 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := tree.FileDigest(source)
	if err != nil {
		t.Fatal(err)
	}
	item := tree.Item{ID: "alice", Revisions: []tree.Revision{{Digest: digest, Path: "Books/card.pdf"}}}
	if err := st.WriteItem(item); err != nil {
		t.Fatal(err)
	}
	plan := Plan{Add: []Action{{Path: "card.pdf", Digest: digest, Item: item.ID, Revision: 1, View: "v"}}}
	if _, err := Apply(context.Background(), st, target, []string{"v"}, plan, []tree.Item{item}, nil, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "card.pdf"))
	if err != nil || string(data) != "synthetic PDF" {
		t.Fatal("export", err)
	}
	missing := item
	missing.ID = "bob"
	missing.Revisions = []tree.Revision{{Digest: digest, Path: "missing.pdf"}}
	if err := st.WriteItem(missing); err != nil {
		t.Fatal(err)
	}
	plans, err := PlanJobs(context.Background(), st, []Job{
		{Name: "valid", Path: t.TempDir(), Files: []view.File{{Path: "card.pdf", Item: item.ID, Digest: digest, Revision: 1}}},
		{Name: "missing", Path: t.TempDir(), Files: []view.File{{Path: "missing.pdf", Item: missing.ID, Digest: digest, Revision: 1}}},
	}, []tree.Item{item, missing})
	if err != nil || len(plans) != 2 || len(plans[0].Problems) != 0 || len(plans[1].Problems) == 0 {
		t.Fatalf("source plans: %+v %v", plans, err)
	}
	fresh := t.TempDir()
	plan.Add = append(plan.Add, Action{Path: "missing.pdf", Digest: digest, Item: missing.ID, Revision: 1, View: "v"})
	_, err = Apply(context.Background(), st, fresh, []string{"v"}, plan, []tree.Item{item, missing}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "file is not at its recorded location") {
		t.Fatalf("missing: %v", err)
	}
	entries, err := os.ReadDir(fresh)
	if err != nil || len(entries) != 0 {
		t.Fatal("partial export", err)
	}
}

func TestInPlaceLostRevisionRejectsWholeExportBeforeWrites(t *testing.T) {
	root, target := t.TempDir(), t.TempDir()
	if err := tree.InitWithOptions(root, time.Unix(0, 0), "in-place", 2); err != nil {
		t.Fatal(err)
	}
	st, err := tree.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "paper.pdf")
	if err := os.WriteFile(source, []byte("synthetic original"), 0600); err != nil {
		t.Fatal(err)
	}
	old, err := tree.FileDigest(source)
	if err != nil {
		t.Fatal(err)
	}
	item := tree.Item{ID: "alice", Type: "id_card", Kind: tree.KindDocument, Head: old, Revisions: []tree.Revision{{Digest: old, Path: "paper.pdf"}}}
	if err := st.WriteItem(item); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("synthetic edited"), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := tree.FileDigest(source)
	if err != nil {
		t.Fatal(err)
	}
	item, err = st.RecordFileChange(context.Background(), item.ID, old, "paper.pdf", next, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{Add: []Action{{Path: "current.pdf", Digest: next, Item: item.ID, Revision: 2, View: "v"}, {Path: "old.pdf", Digest: old, Item: item.ID, Revision: 1, View: "v"}}}
	_, err = Apply(context.Background(), st, target, []string{"v"}, plan, []tree.Item{item}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "earlier revision lost") {
		t.Fatal("lost export", err)
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 0 {
		t.Fatal("partial export", err)
	}
}

func TestInPlaceSkeletonExportKeepsMediaBytesAndClearsOldPinPaths(t *testing.T) {
	root, target := t.TempDir(), t.TempDir()
	now := time.Unix(0, 0)
	if err := tree.InitWithOptions(root, now, "in-place", 2); err != nil {
		t.Fatal(err)
	}
	st, err := tree.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "clip.wav")
	data := []byte("synthetic audio bytes")
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := tree.FileDigest(source)
	if err != nil {
		t.Fatal(err)
	}
	item := tree.Item{ID: "alice", Type: "id_card", Kind: tree.KindDocument, Head: digest, Revisions: []tree.Revision{{Digest: digest, Path: "clip.wav"}}}
	if err := st.WriteItem(item); err != nil {
		t.Fatal(err)
	}
	o := outline.Outline{Name: "media", Mount: []outline.Assembly{{Rule: "media"}}, Rules: []view.View{{Name: "media", Folders: []view.Folder{{Name: "Media", Take: view.Values{"id_card"}, File: "{#}-{id}"}, {Name: "Empty"}}}}}
	run := func(o outline.Outline) JobPlan {
		t.Helper()
		jobs, problems := Jobs(FromOutlines([]outline.Outline{o}, nil, []tree.Item{item}), map[string]string{o.Name: target}, "", []string{o.Name})
		if len(problems) != 0 {
			t.Fatal(problems)
		}
		plans, err := PlanJobs(context.Background(), st, jobs, []tree.Item{item}, now)
		if err != nil || len(plans) != 1 || !plans[0].Ready() {
			t.Fatal(plans, err)
		}
		p := plans[0]
		if _, err := Apply(context.Background(), st, target, p.Views, p.Plan, []tree.Item{item}, nil, nil); err != nil {
			t.Fatal(err)
		}
		return p
	}
	run(o)
	before := filepath.Join(target, "Media", "01-alice.wav")
	got, err := os.ReadFile(before)
	if err != nil || string(got) != string(data) {
		t.Fatal("media bytes", err)
	}
	if info, err := os.Stat(filepath.Join(target, "Empty")); err != nil || !info.IsDir() {
		t.Fatal("empty skeleton folder", err)
	}
	o.Pins = map[string]view.Pin{"Media": {Numbers: map[string]int{"alice": 9}}}
	run(o)
	if _, err := os.Stat(before); !os.IsNotExist(err) {
		t.Fatal("old numbered file was left behind")
	}
	got, err = os.ReadFile(filepath.Join(target, "Media", "09-alice.wav"))
	if err != nil || string(got) != string(data) {
		t.Fatal("pinned media bytes", err)
	}
}

func TestStorageAndOutlineVersionsAreIndependent(t *testing.T) {
	for _, storage := range []string{"", "in-place"} {
		for _, version := range []int{1, 2} {
			t.Run(storage+string(rune('0'+version)), func(t *testing.T) {
				root, target := t.TempDir(), t.TempDir()
				now := time.Unix(0, 0)
				if err := tree.InitWithOptions(root, now, storage, version); err != nil {
					t.Fatal(err)
				}
				st, err := tree.Open(root)
				if err != nil {
					t.Fatal(err)
				}
				if st.OutlineVersion() != version {
					t.Fatal("wrong Outline version")
				}
				source := filepath.Join(t.TempDir(), "card.pdf")
				if err := os.WriteFile(source, []byte("synthetic PDF bytes"), 0600); err != nil {
					t.Fatal(err)
				}
				templates, err := st.LoadTemplates()
				if err != nil {
					t.Fatal(err)
				}
				item, err := st.Import(context.Background(), tree.ImportRequest{Source: source, Folder: root, Template: templates[0], Fields: map[string]string{"owner": "alice", "country": "AU"}, Now: now})
				if err != nil {
					t.Fatal(err)
				}
				rule := view.View{Name: "identity", Node: view.Node{Selection: view.Head, File: "Cards/{id}.pdf"}}
				o := outline.Outline{Name: "papers", Rules: []view.View{rule}}
				if version == 2 {
					rule = view.View{Name: "identity", Folders: []view.Folder{{Name: "Cards", Take: view.Values{"id_card"}, File: "{id}"}}}
					o = outline.Outline{Name: "papers", Mount: []outline.Assembly{{Rule: rule.Name}}}
				}
				if err := outline.SaveRule(st, "", rule, now); err != nil {
					t.Fatal(err)
				}
				if err := outline.Save(st, "", o, now); err != nil {
					t.Fatal(err)
				}
				loaded, err := outline.Load(st)
				if err != nil || len(loaded) != 1 {
					t.Fatal(loaded, err)
				}
				jobs, problems := Jobs(FromOutlines(loaded, nil, []tree.Item{item}), map[string]string{"papers": target}, "", []string{"papers"})
				if len(problems) != 0 {
					t.Fatal(problems)
				}
				plans, err := PlanJobs(context.Background(), st, jobs, []tree.Item{item}, now)
				if err != nil || len(plans) != 1 || !plans[0].Ready() {
					t.Fatal(plans, err)
				}
				if _, err := Apply(context.Background(), st, target, plans[0].Views, plans[0].Plan, []tree.Item{item}, nil, nil); err != nil {
					t.Fatal(err)
				}
				bytes, err := os.ReadFile(filepath.Join(target, "Cards", item.ID+".pdf"))
				if err != nil || string(bytes) != "synthetic PDF bytes" {
					t.Fatal("export", err)
				}
			})
		}
	}
}
