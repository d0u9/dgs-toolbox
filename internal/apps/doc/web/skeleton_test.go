package web

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/tree"
)

func TestInPlaceMediaAndSkeletonAPIPlansPersistedPins(t *testing.T) {
	root := t.TempDir()
	must(t, tree.InitWithOptions(root, time.Unix(0, 0), "in-place", 2))
	source := t.TempDir()
	write(t, filepath.Join(source, "clip.wav"), "synthetic audio bytes")
	h := Handler(Settings{Root: root, CacheDir: t.TempDir()})
	send := func(path string, body any) map[string]json.RawMessage {
		t.Helper()
		data, err := json.Marshal(body)
		must(t, err)
		rec := do(h, "POST", path, string(data))
		if rec.Code != http.StatusOK {
			t.Fatal(path, rec.Code, rec.Body.String())
		}
		var result map[string]json.RawMessage
		must(t, json.Unmarshal(rec.Body.Bytes(), &result))
		return result
	}
	answer := send("/api/import", map[string]any{"dir": source, "path": "clip.wav", "folder": root, "type": "id_card", "fields": map[string]string{"owner": "alice", "country": "AU"}})
	var item tree.Item
	must(t, json.Unmarshal(answer["item"], &item))
	if item.Revisions[0].Path != "clip.wav" {
		t.Fatal("media path", item)
	}
	for _, endpoint := range []string{"/api/pages", "/api/page", "/api/text", "/api/revision"} {
		rec := do(h, "GET", endpoint+"?item="+item.ID+"&digest="+item.Current()+"&n=1", "")
		if rec.Code == http.StatusOK || !strings.Contains(rec.Body.String(), "not a PDF") {
			t.Fatal("non-PDF parsed", endpoint, rec.Body.String())
		}
	}
	// Media does not become an unread PDF in text search.
	rec := do(h, "GET", "/api/search?q=synthetic", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"unread":0`) {
		t.Fatal("media text queue", rec.Body.String())
	}
	send("/api/defs/rules", map[string]any{"rule": map[string]any{"name": "media", "folders": []any{map[string]any{"name": "Media", "take": []string{"id_card"}, "file": "{#}-{id}"}}}})
	outline := map[string]any{"name": "current", "mount": []any{map[string]any{"rule": "media"}}, "defaults": map[string]string{"revision": "head", "valid": "today"}, "pins": map[string]any{"Media": map[string]any{"items": []string{item.ID}, "numbers": map[string]int{item.ID: 7}}}}
	send("/api/defs/outlines", map[string]any{"outline": outline})
	group := send("/api/outlines/group", outline)
	var files []struct {
		Path   string `json:"path"`
		Number int    `json:"number"`
	}
	must(t, json.Unmarshal(group["files"], &files))
	if len(files) != 1 || files[0].Path != "Media/07-"+item.ID+".wav" || files[0].Number != 7 {
		t.Fatal("group pins/extension", files)
	}
	rec = do(h, "GET", "/api/defs/outlines", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"mount":[{"rule":"media"}]`) || !strings.Contains(rec.Body.String(), `"numbers":{"`+item.ID+`":7}`) {
		t.Fatal("pins did not reload", rec.Body.String())
	}
	why, _ := json.Marshal(map[string]any{"outline": outline, "item": item.ID})
	rec = do(h, "POST", "/api/outlines/explain", string(why))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Media/07-") {
		t.Fatal("Why here", rec.Body.String())
	}
	plan := send("/api/export/plan", map[string]any{"outlines": []string{"current"}, "folders": map[string]string{"current": t.TempDir()}})
	if string(plan["ready"]) != "true" {
		t.Fatal("media export not ready", string(plan["jobs"]))
	}
	// In-place deletion now works, including its sidecar.
	send("/api/items/delete", map[string]string{"item": item.ID})
	if len(state(t, h).Items) != 0 {
		t.Fatal("deleted Item remained in cache")
	}
}

func TestInitAPIOffersIndependentStorageAndOutlineChoices(t *testing.T) {
	for _, storage := range []string{"", "in-place"} {
		for _, version := range []int{1, 2} {
			root := t.TempDir()
			h := Handler(Settings{Root: root, CacheDir: t.TempDir()})
			body, _ := json.Marshal(map[string]any{"layout": storage, "outlines": version})
			rec := do(h, "POST", "/api/init", string(body))
			if rec.Code != http.StatusOK {
				t.Fatal(rec.Code, rec.Body.String())
			}
			st, err := tree.Open(root)
			if err != nil || st.OutlineVersion() != version {
				t.Fatal(st, err)
			}
			s := state(t, h)
			if !s.Tree || s.OutlineVersion != version || s.Capabilities.ImportFolder != (storage == "in-place") {
				t.Fatal(s)
			}
			if rec := do(h, "POST", "/api/init", string(body)); rec.Code == http.StatusOK {
				t.Fatal("existing marker overwritten")
			}
		}
	}
}
