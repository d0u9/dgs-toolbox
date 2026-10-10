package web

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/tree"
)

func TestInPlaceBrowsePreviewAndMissingFile(t *testing.T) {
	root := t.TempDir()
	must(t, tree.InitWithLayout(root, time.Unix(0, 0), "in-place"))
	st, err := tree.Open(root)
	must(t, err)
	pdf := scan(t)
	write(t, filepath.Join(root, "Books", "card.pdf"), string(pdf))
	digest, err := tree.FileDigest(filepath.Join(root, "Books", "card.pdf"))
	must(t, err)
	item := tree.Item{ID: "alice", Type: "id_card", Kind: tree.KindDocument, Head: digest, Fields: map[string]string{"owner": "alice"}, Revisions: []tree.Revision{{Digest: digest, Path: "Books/card.pdf"}}}
	must(t, st.WriteItem(item))
	h := Handler(Settings{Root: root, CacheDir: t.TempDir()})
	s := state(t, h)
	if !s.Capabilities.FileChanges || len(s.Items) != 1 || s.Items[0].Revisions[0].Path != "Books/card.pdf" {
		t.Fatalf("Browse: %+v", s)
	}
	query := "?item=alice&digest=" + digest
	for _, endpoint := range []string{"/api/revision", "/api/pages", "/api/page"} {
		rec := do(h, "GET", endpoint+query+"&n=1", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", endpoint, rec.Code, rec.Body.String())
		}
	}
	must(t, os.Remove(filepath.Join(root, "Books", "card.pdf")))
	for _, endpoint := range []string{"/api/revision", "/api/pages", "/api/page"} {
		rec := do(h, "GET", endpoint+query+"&n=1", "")
		if rec.Code == http.StatusOK || !strings.Contains(rec.Body.String(), "file is not at its recorded location") {
			t.Fatalf("missing %s: %d %s", endpoint, rec.Code, rec.Body.String())
		}
	}
}

func TestInPlaceImportAPIRecordsCopiesAndOffersReuse(t *testing.T) {
	root := t.TempDir()
	must(t, tree.InitWithLayout(root, time.Unix(0, 0), "in-place"))
	pdf := scan(t)
	write(t, filepath.Join(root, "Books", "card.pdf"), string(pdf))
	h := Handler(Settings{Root: root, CacheDir: t.TempDir()})
	send := func(body map[string]any) (int, map[string]json.RawMessage) {
		t.Helper()
		data, err := json.Marshal(body)
		must(t, err)
		rec := do(h, "POST", "/api/import", string(data))
		var answer map[string]json.RawMessage
		must(t, json.Unmarshal(rec.Body.Bytes(), &answer))
		return rec.Code, answer
	}
	fields := map[string]string{"owner": "alice", "country": "AU"}
	request := map[string]any{"dir": filepath.Join(root, "Books"), "path": "card.pdf", "type": "id_card", "fields": fields}
	status, answer := send(request)
	if status != http.StatusOK {
		t.Fatalf("record: %d %s", status, answer["error"])
	}
	var first tree.Item
	must(t, json.Unmarshal(answer["item"], &first))
	if first.Revisions[0].Path != "Books/card.pdf" || len(state(t, h).Items) != 1 {
		t.Fatal("sidecar or cache did not follow import")
	}
	listed := source(t, h, root)
	if len(listed) != 1 || listed[0].Item != first.ID {
		t.Fatalf("recorded file missing from in-place import listing: %+v", listed)
	}
	outside := t.TempDir()
	write(t, filepath.Join(outside, "another.pdf"), string(pdf))
	fields["owner"] = "bob"
	request["dir"] = outside
	request["path"] = "another.pdf"
	status, answer = send(request)
	if status != http.StatusConflict {
		t.Fatalf("offer status: %d", status)
	}
	var offer tree.ReuseFile
	must(t, json.Unmarshal(answer["reuse"], &offer))
	if offer.Item != first.ID || offer.Path != "Books/card.pdf" || len(state(t, h).Items) != 1 {
		t.Fatal("invalid reuse offer or premature sidecar")
	}
	request["reuse"] = offer.Item
	status, answer = send(request)
	if status != http.StatusOK {
		t.Fatalf("reuse: %d %s", status, answer["error"])
	}
	if len(state(t, h).Items) != 2 {
		t.Fatal("reuse cache stale")
	}
	// Stale confirmation cannot name different source content.
	fields["owner"] = "carol"
	write(t, filepath.Join(outside, "another.pdf"), "different synthetic PDF")
	status, _ = send(request)
	if status == http.StatusOK || len(state(t, h).Items) != 2 {
		t.Fatal("accepted stale reuse")
	}
	delete(request, "reuse")
	request["folder"] = filepath.Join(root, "Books")
	status, answer = send(request)
	if status != http.StatusOK {
		t.Fatalf("copy: %d %s", status, answer["error"])
	}
	got, err := os.ReadFile(filepath.Join(root, "Books", "another.pdf"))
	must(t, err)
	if string(got) != "different synthetic PDF" {
		t.Fatal("copy content")
	}
	// Existing tree content cannot be overwritten by a second external file.
	fields["owner"] = "dave"
	write(t, filepath.Join(outside, "another.pdf"), "third synthetic PDF")
	status, _ = send(request)
	if status != http.StatusConflict || len(state(t, h).Items) != 3 {
		t.Fatal("name collision accepted")
	}
	got, err = os.ReadFile(filepath.Join(root, "Books", "another.pdf"))
	must(t, err)
	if string(got) != "different synthetic PDF" {
		t.Fatal("collision overwrote file")
	}
	if rec := do(h, "GET", "/api/source?dir="+url.QueryEscape(outside), ""); rec.Code != http.StatusOK {
		t.Fatal("external source listing")
	}
}

func TestInPlacePhotoImportAPIAndDeferredOperations(t *testing.T) {
	root, scans := t.TempDir(), t.TempDir()
	must(t, tree.InitWithLayout(root, time.Unix(0, 0), "in-place"))
	// Reuse the existing synthetic scan helper's page image.
	image := image.NewNRGBA(image.Rect(0, 0, 8, 12))
	image.Set(1, 1, color.NRGBA{R: 60, A: 255})
	var data bytes.Buffer
	must(t, png.Encode(&data, image))
	write(t, filepath.Join(scans, "page.png"), data.String())
	h := Handler(Settings{Root: root, CacheDir: t.TempDir()})
	body, _ := json.Marshal(map[string]any{"dir": scans, "path": "page.png", "folder": root, "pdf_name": "registration.pdf", "type": "id_card", "fields": map[string]string{"owner": "alice", "country": "AU"}, "keep_original": true})
	rec := do(h, "POST", "/api/import", string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("photo: %d %s", rec.Code, rec.Body.String())
	}
	s := state(t, h)
	if len(s.Items) != 1 || len(s.Items[0].Revisions[0].Originals) != 1 {
		t.Fatal("photo metadata")
	}
	r := s.Items[0].Revisions[0]
	if r.Path != "registration.pdf" || r.Originals[0].Path != "registration/page.png" {
		t.Fatal("photo paths")
	}
	query := "?item=" + s.Items[0].ID + "&digest=" + r.Ref() + "&n=1"
	if got := do(h, "GET", "/api/page"+query, ""); got.Code != http.StatusOK {
		t.Fatalf("photo preview: %d %s", got.Code, got.Body.String())
	}
	// An Item without a PDF writes only its sidecar, so it is not refused.
	if rec := do(h, "POST", "/api/import", `{"no_pdf":true,"type":"id_card"}`); strings.Contains(rec.Body.String(), "not supported") {
		t.Fatal("creation without a PDF refused:", rec.Body.String())
	}
}
