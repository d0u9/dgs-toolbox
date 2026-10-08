package web

import (
	"dgs-toolbox/internal/doc/itemcache"
	"dgs-toolbox/internal/doc/tree"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImportUpdatesOnlyNamedItems(t *testing.T) {
	root := t.TempDir()
	must(t, tree.Init(root, time.Unix(0, 0)))
	st, err := tree.Open(root)
	must(t, err)
	old := tree.Item{ID: "OLD", Type: "bill", Kind: tree.KindRecord, Fields: map[string]string{"title": "Before"}}
	untouched := tree.Item{ID: "OTHER", Type: "bill", Kind: tree.KindRecord}
	must(t, st.WriteItem(old))
	must(t, st.WriteItem(untouched))
	cache := itemcache.New(st, "")
	_, err = cache.Items()
	must(t, err)
	// Unrelated files must not be reread during a named import update.
	must(t, os.WriteFile(st.Sidecar("OTHER"), []byte("invalid: ["), 0600))
	added := tree.Item{ID: "NEW", Type: "bill", Kind: tree.KindRecord}
	h := (server{items: cache}).followWrites(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		must(t, st.WriteItem(added))
		old.Fields["title"] = "After"
		must(t, st.WriteItem(old))
		writeJSON(w, 200, map[string]any{"item": added})
	}))
	rec := do(h, "POST", "/api/import", `{"replaces":"OLD"}`)
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	items, err := cache.Items()
	must(t, err)
	if len(items) != 3 {
		t.Fatalf("items=%d", len(items))
	}
	for _, item := range items {
		if item.ID == "OLD" && item.Fields["title"] != "After" {
			t.Fatal("replacement stale")
		}
	}
}

func BenchmarkImportCacheUpdate(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			root := b.TempDir()
			if err := tree.Init(root, time.Unix(0, 0)); err != nil {
				b.Fatal(err)
			}
			st, err := tree.Open(root)
			if err != nil {
				b.Fatal(err)
			}
			for i := 0; i < n; i++ {
				if err := st.WriteItem(tree.Item{ID: fmt.Sprintf("ITEM-%04d", i), Type: "bill", Kind: tree.KindRecord}); err != nil {
					b.Fatal(err)
				}
			}
			cache := itemcache.New(st, filepath.Join(b.TempDir(), "cache"))
			if _, err := cache.Items(); err != nil {
				b.Fatal(err)
			}
			for _, full := range []bool{true, false} {
				name := "incremental"
				if full {
					name = "full"
				}
				b.Run(name, func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						var err error
						if full {
							err = cache.Wrote()
						} else {
							err = cache.Wrote("ITEM-0000")
						}
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

func TestSourceSnapshotIncludesDigestBeforeFirstImport(t *testing.T) {
	root, scans := setup(t, true)
	h := Handler(Settings{Root: root, CacheDir: t.TempDir()})
	files := source(t, h, scans)
	for _, file := range files {
		want, err := tree.FileDigest(filepath.Join(scans, filepath.FromSlash(file.Path)))
		must(t, err)
		if file.Digest != want || file.Item != "" {
			t.Fatalf("initial source digest missing or incorrectly marked")
		}
	}
}

func BenchmarkSourceListingReady(b *testing.B) {
	root, scans := b.TempDir(), b.TempDir()
	if err := tree.Init(root, time.Unix(0, 0)); err != nil {
		b.Fatal(err)
	}
	payload := make([]byte, 1<<20)
	for i := 0; i < 128; i++ {
		if err := os.WriteFile(filepath.Join(scans, fmt.Sprintf("scan-%03d.pdf", i)), payload, 0600); err != nil {
			b.Fatal(err)
		}
	}
	h := Handler(Settings{Root: root, CacheDir: b.TempDir()})
	for _, metadata := range []bool{false, true} {
		name, query := "full", ""
		if metadata {
			name, query = "metadata", "&metadata=1"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				rec := do(h, "GET", "/api/source?dir="+url.QueryEscape(scans)+query, "")
				if rec.Code != 200 {
					b.Fatal(rec.Code)
				}
			}
		})
	}
}

func BenchmarkInPlaceSourceReady(b *testing.B) {
	root := b.TempDir()
	if err := tree.InitWithLayout(root, time.Unix(0, 0), "in-place"); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("scan-%04d.pdf", i)), []byte("synthetic"), 0600); err != nil {
			b.Fatal(err)
		}
	}
	h := Handler(Settings{Root: root, CacheDir: b.TempDir()})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := do(h, "GET", "/api/source?dir="+url.QueryEscape(root), "")
		if rec.Code != 200 {
			b.Fatal(rec.Code)
		}
	}
}
