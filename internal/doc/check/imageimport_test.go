package check

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/tree"
)

func TestArchivedImageVerifiedAndCorruptionReported(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	if err := tree.Init(root, now); err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 4, 6)))
	source := filepath.Join(t.TempDir(), "scan.png")
	os.WriteFile(source, data.Bytes(), 0600)
	templates, err := testStore(t, root).LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	item, err := testStore(t, root).Import(context.Background(), tree.ImportRequest{Source: source, Template: templates[0], Fields: map[string]string{"owner": "alice", "country": "AU"}, KeepOriginal: true, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	report, err := Tree(context.Background(), testStore(t, root), nil)
	if err != nil || len(report.Problems) != 0 || report.Checked != 2 {
		t.Fatalf("verify: %+v %v", report, err)
	}
	path := testStore(t, root).OriginalPath(item, item.Revisions[0].Digest, item.Revisions[0].Originals[0])
	os.WriteFile(path, []byte("damaged"), 0600)
	report, err = Tree(context.Background(), testStore(t, root), nil)
	if err != nil || kinds(report)[Changed] != 1 {
		t.Fatalf("corruption: %+v %v", report, err)
	}
	os.Remove(path)
	report, err = Tree(context.Background(), testStore(t, root), nil)
	if err != nil || kinds(report)[Missing] != 1 {
		t.Fatalf("missing: %+v %v", report, err)
	}
}
