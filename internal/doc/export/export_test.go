package export

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

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
	plan, err := Compute(context.Background(), target, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blocked) == 0 {
		if _, err := Apply(context.Background(), root, target, "v", plan, items, nil); err != nil {
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
	items := []tree.Item{{ID: "A", Type: "passport", Fields: map[string]string{"owner": "jane"}}, {ID: "B", Type: "licence"}}

	plan := export(t, root, target, []view.File{
		{Path: "jane/passport.pdf", Item: "A", Digest: a1, Revision: 1},
		{Path: "jane/licence.pdf", Item: "B", Digest: b1, Revision: 1},
	}, items)
	if len(plan.Add) != 2 || read(t, filepath.Join(target, "jane/passport.pdf")) != "passport one" {
		t.Fatalf("first export: %+v", plan)
	}
	m, err := ReadManifest(target)
	if err != nil || len(m.Files) != 2 || m.Files[1].Fields["owner"] != "jane" {
		t.Fatalf("manifest: %v %+v", err, m)
	}

	// A new HEAD replaces the file; the licence, no longer selected, goes,
	// and so does the folder it leaves empty.
	a2 := stored(t, root, "A", "passport two")
	plan = export(t, root, target, []view.File{{Path: "jane/passport.pdf", Item: "A", Digest: a2, Revision: 2}}, items)
	if len(plan.Replace) != 1 || len(plan.Remove) != 1 || len(plan.Add) != 0 {
		t.Fatalf("second export: %+v", plan)
	}
	if read(t, filepath.Join(target, "jane/passport.pdf")) != "passport two" || read(t, filepath.Join(target, "jane/licence.pdf")) != "<missing>" {
		t.Fatal("second export did not replace and remove")
	}

	// Nothing changed: nothing is written.
	plan = export(t, root, target, []view.File{{Path: "jane/passport.pdf", Item: "A", Digest: a2, Revision: 2}}, items)
	if plan.Changes() != 0 || len(plan.Keep) != 1 {
		t.Fatalf("third export: %+v", plan)
	}

	plan = export(t, root, target, nil, items)
	if len(plan.Remove) != 1 {
		t.Fatalf("empty export: %+v", plan)
	}
	if _, err := os.Stat(filepath.Join(target, "jane")); !os.IsNotExist(err) {
		t.Fatal("empty folder left behind")
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
	if _, err := Apply(context.Background(), root, target, "v", plan, items, nil); err == nil {
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
	if _, err := Compute(context.Background(), target, nil); err == nil {
		t.Fatal("a broken manifest was read")
	}
}
