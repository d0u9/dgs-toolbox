package web

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/doc/tree"
)

func TestImageImportPreviewAndRetentionOverride(t *testing.T) {
	for _, keep := range []bool{true, false} {
		t.Run(map[bool]string{true: "default archive", false: "PDF only"}[keep], func(t *testing.T) {
			root, scans := setup(t, true)
			var img bytes.Buffer
			must(t, png.Encode(&img, image.NewNRGBA(image.Rect(0, 0, 12, 18))))
			must(t, os.WriteFile(filepath.Join(scans, "scan.PNG"), img.Bytes(), 0600))
			h := Handler(Settings{Root: root, CacheDir: t.TempDir()})
			preview := do(h, "GET", "/api/source/file?dir="+url.QueryEscape(scans)+"&path=scan.PNG", "")
			if preview.Code != http.StatusOK || !bytes.HasPrefix(preview.Body.Bytes(), []byte("%PDF-")) {
				t.Fatalf("preview: %d %s", preview.Code, preview.Body.String())
			}
			option := ""
			if !keep {
				option = `,"keep_original":false`
			}
			rec := do(h, "POST", "/api/import", `{"dir":`+q(scans)+`,"path":"scan.PNG","type":"id_card","fields":{"owner":"alice","country":"AU"}`+option+`}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
			}
			var item tree.Item
			must(t, json.Unmarshal(rec.Body.Bytes(), &item))
			if (len(item.Revisions[0].Originals) > 0) != keep {
				t.Fatal("retention override ignored")
			}
			for _, f := range source(t, h, scans) {
				if f.Path == "scan.PNG" && f.Item != item.ID {
					t.Fatal("image not recognised after import")
				}
			}
		})
	}
}

func TestImageRetentionConfigurationDefault(t *testing.T) {
	disabled := false
	root, _ := setup(t, true)
	h := Handler(Settings{Root: root, KeepOriginal: &disabled})
	if state(t, h).KeepOriginal {
		t.Fatal("configuration default ignored")
	}
}
