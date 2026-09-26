package export

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/doc/outline"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

// stored puts a PDF with content into root under item and returns its digest.
func stored(t *testing.T, root, item, content string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(content))
	digest := hex.EncodeToString(sum[:])
	path := tree.PDFPath(root, item, digest)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return digest
}

func export(t *testing.T, root, target string, files []view.File, items []tree.Item) Plan {
	t.Helper()
	return exportAs(t, root, target, "v", files, items)
}

// exportAs exports files as the View name.
func exportAs(t *testing.T, root, target, name string, files []view.File, items []tree.Item) Plan {
	t.Helper()
	for i := range files {
		files[i].View = name
	}
	plan, err := Compute(context.Background(), target, []string{name}, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blocked) == 0 {
		if _, err := Apply(context.Background(), root, target, []string{name}, plan, items, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	return plan
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return "<missing>"
	}
	return string(data)
}

func TestExportIsIncremental(t *testing.T) {
	root, target := t.TempDir(), t.TempDir()
	a1 := stored(t, root, "A", "passport one")
	b1 := stored(t, root, "B", "licence")
	items := []tree.Item{{ID: "A", Type: "passport", Fields: map[string]string{"owner": "emma"}}, {ID: "B", Type: "licence"}}

	plan := export(t, root, target, []view.File{
		{Path: "emma/passport.pdf", Item: "A", Digest: a1, Revision: 1},
		{Path: "emma/licence.pdf", Item: "B", Digest: b1, Revision: 1},
	}, items)
	if len(plan.Add) != 2 || read(t, filepath.Join(target, "emma/passport.pdf")) != "passport one" {
		t.Fatalf("first export: %+v", plan)
	}
	m, err := ReadManifest(target)
	if err != nil || len(m.Files) != 2 || m.Files[1].Fields["owner"] != "emma" {
		t.Fatalf("manifest: %v %+v", err, m)
	}

	// A new HEAD replaces the file; the licence, no longer selected, goes,
	// and so does the folder it leaves empty.
	a2 := stored(t, root, "A", "passport two")
	plan = export(t, root, target, []view.File{{Path: "emma/passport.pdf", Item: "A", Digest: a2, Revision: 2}}, items)
	if len(plan.Replace) != 1 || len(plan.Remove) != 1 || len(plan.Add) != 0 {
		t.Fatalf("second export: %+v", plan)
	}
	if read(t, filepath.Join(target, "emma/passport.pdf")) != "passport two" || read(t, filepath.Join(target, "emma/licence.pdf")) != "<missing>" {
		t.Fatal("second export did not replace and remove")
	}

	// Nothing changed: nothing is written.
	plan = export(t, root, target, []view.File{{Path: "emma/passport.pdf", Item: "A", Digest: a2, Revision: 2}}, items)
	if plan.Changes() != 0 || len(plan.Keep) != 1 {
		t.Fatalf("third export: %+v", plan)
	}

	plan = export(t, root, target, nil, items)
	if len(plan.Remove) != 1 {
		t.Fatalf("empty export: %+v", plan)
	}
	if _, err := os.Stat(filepath.Join(target, "emma")); !os.IsNotExist(err) {
		t.Fatal("empty folder left behind")
	}
}

func TestPublishedCallbackOnlyForWrittenPDFs(t *testing.T) {
	root, target := t.TempDir(), t.TempDir()
	digest := stored(t, root, "A", "pdf")
	files := []view.File{{Path: "a.pdf", Item: "A", Digest: digest, Revision: 1, View: "v"}}
	items := []tree.Item{{ID: "A", Type: "card"}}
	count := 0
	run := func() {
		plan, err := Compute(context.Background(), target, []string{"v"}, files)
		if err != nil {
			t.Fatal(err)
		}
		_, err = Apply(context.Background(), root, target, []string{"v"}, plan, items, nil, func(a Action) {
			count++
			if a.Item != "A" || a.Digest != digest {
				t.Fatalf("callback: %+v", a)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	run()
	run()
	if count != 1 {
		t.Fatalf("published callback called %d times", count)
	}
}

func TestExportLeavesOtherFilesAlone(t *testing.T) {
	root, target := t.TempDir(), t.TempDir()
	a := stored(t, root, "A", "passport")
	b := stored(t, root, "B", "bill")
	items := []tree.Item{{ID: "A"}, {ID: "B"}}
	if err := os.WriteFile(filepath.Join(target, "mine.pdf"), []byte("owner's own"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A file the export did not write blocks its path, and nothing is written.
	plan := export(t, root, target, []view.File{{Path: "mine.pdf", Item: "A", Digest: a}, {Path: "b.pdf", Item: "B", Digest: b}}, items)
	if len(plan.Blocked) != 1 || read(t, filepath.Join(target, "b.pdf")) != "<missing>" {
		t.Fatalf("blocked: %+v", plan)
	}
	if _, err := Apply(context.Background(), root, target, []string{"v"}, plan, items, nil, nil); err == nil {
		t.Fatal("Apply wrote a blocked plan")
	}

	// A file changed after export is neither replaced nor removed.
	export(t, root, target, []view.File{{Path: "a.pdf", Item: "A", Digest: a}, {Path: "b.pdf", Item: "B", Digest: b}}, items)
	if err := os.WriteFile(filepath.Join(target, "b.pdf"), []byte("annotated"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan = export(t, root, target, []view.File{{Path: "a.pdf", Item: "A", Digest: a}}, items)
	if len(plan.Left) != 1 || read(t, filepath.Join(target, "b.pdf")) != "annotated" || read(t, filepath.Join(target, "mine.pdf")) != "owner's own" {
		t.Fatalf("left: %+v", plan)
	}
	m, _ := ReadManifest(target)
	if len(m.Files) != 1 {
		t.Fatalf("the manifest still claims a changed file: %+v", m)
	}
	plan = export(t, root, target, []view.File{{Path: "a.pdf", Item: "B", Digest: b}}, items)
	if len(plan.Replace) != 1 {
		t.Fatalf("replace: %+v", plan)
	}
}

func TestExportAdoptsAnIdenticalFile(t *testing.T) {
	root, target := t.TempDir(), t.TempDir()
	a := stored(t, root, "A", "passport")
	if err := os.WriteFile(filepath.Join(target, "a.pdf"), []byte("passport"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := export(t, root, target, []view.File{{Path: "a.pdf", Item: "A", Digest: a}}, []tree.Item{{ID: "A"}})
	if len(plan.Keep) != 1 {
		t.Fatalf("plan: %+v", plan)
	}
	if m, _ := ReadManifest(target); len(m.Files) != 1 {
		t.Fatalf("manifest: %+v", m)
	}
}

func TestCheckTarget(t *testing.T) {
	root := t.TempDir()
	for _, target := range []string{root, filepath.Join(root, "out"), filepath.Dir(root), "relative"} {
		if CheckTarget(root, target) == nil {
			t.Errorf("%s accepted", target)
		}
	}
	if err := CheckTarget(root, t.TempDir()); err != nil {
		t.Error(err)
	}
}

func TestBadManifestRefused(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, ManifestName), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Compute(context.Background(), target, nil, nil); err == nil {
		t.Fatal("a broken manifest was read")
	}
}

func TestViewsShareATarget(t *testing.T) {
	root, target := t.TempDir(), t.TempDir()
	a := stored(t, root, "A", "passport")
	b := stored(t, root, "B", "bill")
	items := []tree.Item{{ID: "A"}, {ID: "B"}}
	exportAs(t, root, target, "ids", []view.File{{Path: "ids/a.pdf", Item: "A", Digest: a}}, items)
	exportAs(t, root, target, "bills", []view.File{{Path: "bills/b.pdf", Item: "B", Digest: b}}, items)
	if read(t, filepath.Join(target, "ids/a.pdf")) != "passport" || read(t, filepath.Join(target, "bills/b.pdf")) != "bill" {
		t.Fatal("one View's export removed the other's")
	}
	m, _ := ReadManifest(target)
	if len(m.Views) != 2 || len(m.Files) != 2 {
		t.Fatalf("manifest %+v", m)
	}

	// A path another View exported is blocked, not taken over.
	plan := exportAs(t, root, target, "bills", []view.File{{Path: "ids/a.pdf", Item: "B", Digest: b}}, items)
	if len(plan.Blocked) != 1 || len(plan.Remove) != 1 {
		t.Fatalf("plan %+v", plan)
	}

	// Emptying one View removes only its own files.
	exportAs(t, root, target, "bills", nil, items)
	if read(t, filepath.Join(target, "ids/a.pdf")) != "passport" || read(t, filepath.Join(target, "bills/b.pdf")) != "<missing>" {
		t.Fatal("wrong files removed")
	}
}

func TestVersionOneManifestIsRead(t *testing.T) {
	target := t.TempDir()
	old := `{"version": 1, "view": "important", "files": [{"path": "a.pdf", "digest": "d", "item": "A", "type": "t", "kind": "record", "revision": 1, "fields": {}}]}`
	if err := os.WriteFile(filepath.Join(target, ManifestName), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := ReadManifest(target)
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != Version || len(m.Views) != 1 || m.Views[0] != "important" || m.Files[0].View != "important" {
		t.Fatalf("manifest %+v", m)
	}
}

func TestJobsAreOutlinesWithAFolder(t *testing.T) {
	rule := []view.View{{Name: "ids"}}
	outlines := []outline.Outline{
		{Name: "google", Folder: "/g", Rules: []view.View{{Name: "ids"}, {Name: "bills"}}},
		{Name: "icloud", Folder: "~/i", Rules: rule},
		{Name: "browse", Rules: rule},
		{Name: "empty", Folder: "/e"},
	}
	jobs, problems := Jobs(FromOutlines(outlines), nil, "/home", nil)
	if len(problems) != 0 || len(jobs) != 2 || jobs[0].Name != "google" || len(jobs[0].Views) != 2 || jobs[1].Path != "/home/i" {
		t.Fatalf("jobs %+v %v", jobs, problems)
	}
	if jobs, _ = Jobs(FromOutlines(outlines), map[string]string{"browse": "/b"}, "/home", []string{"browse"}); len(jobs) != 1 || jobs[0].Path != "/b" {
		t.Fatalf("chosen %+v", jobs)
	}
	_, problems = Jobs(FromOutlines(outlines), nil, "/home", []string{"browse", "empty", "gone"})
	if len(problems) != 3 || !strings.Contains(problems[0], "choose a folder") {
		t.Fatalf("problems %v", problems)
	}
}

func TestPlanJobsChecksEverythingFirst(t *testing.T) {
	root, out := t.TempDir(), t.TempDir()
	a := stored(t, root, "A", "passport")
	items := []tree.Item{{ID: "A", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"owner": "emma"},
		Revisions: []tree.Revision{{Digest: a}}}}
	ids := view.View{Name: "ids", Selection: view.Head, Layout: "{owner}/{type}.{ext}"}
	same := view.View{Name: "same", Selection: view.Head, Layout: "{owner}/id_card.{ext}"}
	google := filepath.Join(out, "google")
	nested := filepath.Join(google, "inner")
	plans, err := PlanJobs(context.Background(), root, []Job{
		{Name: "google", Path: google, Views: []view.View{ids, same}},
		{Name: "inner", Path: nested, Views: []view.View{ids}},
	}, items)
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].Ready() || len(plans[0].Combined.Clashes) != 1 || len(plans[0].Problems) != 1 || len(plans[1].Problems) != 1 {
		t.Fatalf("plans %+v", plans)
	}
	plans, _ = PlanJobs(context.Background(), root, []Job{{Name: "google", Path: google, Views: []view.View{ids}}}, items)
	if !plans[0].Ready() || len(plans[0].Plan.Add) != 1 || plans[0].Plan.Add[0].View != "ids" {
		t.Fatalf("plan %+v", plans[0])
	}
}

func TestAgainstOthers(t *testing.T) {
	plans := []JobPlan{{Name: "kindle", Path: "/out/kindle"}, {Name: "icloud", Path: "/trees/books/export"}}
	AgainstOthers(plans, []Other{{Name: "books", Root: "/trees/books"}})
	if len(plans[0].Problems) != 0 {
		t.Fatalf("a folder apart: %v", plans[0].Problems)
	}
	if len(plans[1].Problems) != 1 || !strings.Contains(plans[1].Problems[0], "overlaps the tree books") {
		t.Fatalf("inside another tree: %v", plans[1].Problems)
	}
}

// Snapshots may share one PDF; a manifest entry found only by digest names
// HEAD when HEAD holds that PDF, never merely the first holder.
func TestByDigestPrefersHead(t *testing.T) {
	it := tree.Item{Revisions: []tree.Revision{
		{ID: "a", Snapshot: true, Digest: "pdf"},
		{ID: "b", Snapshot: true, Digest: "pdf"},
		{ID: "c", Snapshot: true, Digest: "pdf"},
	}, Head: "b"}
	if got := byDigest(it, "pdf").Ref(); got != "b" {
		t.Fatalf("got %s, want HEAD b", got)
	}
	it.Head = "other"
	if got := byDigest(it, "pdf").Ref(); got != "c" {
		t.Fatalf("got %s, want newest c", got)
	}
}
