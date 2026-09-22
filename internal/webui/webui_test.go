package webui_test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"dgs-toolbox/internal/webui"
)

// The files a page links. A page that links one of these by name breaks if
// the file is renamed, so the names are pinned here.
var shared = []string{"fonts.css", "tokens.css", "base.css", "controls.css", "filedialog.css"}

func TestAssetsHoldTheSharedFiles(t *testing.T) {
	for _, name := range shared {
		b, err := fs.ReadFile(webui.Assets(), name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if len(b) == 0 {
			t.Fatalf("%s is empty", name)
		}
	}
}

// The faces fonts.css names ship with the binary, so a page draws the same
// on a machine with no network.
func TestFontsAreEmbedded(t *testing.T) {
	for _, name := range []string{"fonts/manrope-latin.woff2", "fonts/manrope-latin-ext.woff2", "fonts/LICENSE-manrope.txt"} {
		b, err := fs.ReadFile(webui.Assets(), name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if len(b) == 0 {
			t.Fatalf("%s is empty", name)
		}
	}
}

// The shared scripts, pinned for the same reason: a page imports them by
// name.
var sharedScripts = []string{"filedialog.js"}

func TestAssetsHoldTheSharedScripts(t *testing.T) {
	for _, name := range sharedScripts {
		b, err := fs.ReadFile(webui.Assets(), name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if len(b) == 0 {
			t.Fatalf("%s is empty", name)
		}
	}
}

func TestHandlerServesScripts(t *testing.T) {
	mux := http.NewServeMux()
	webui.Mount(mux)

	for _, name := range sharedScripts {
		req := httptest.NewRequest(http.MethodGet, webui.Prefix+name, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s%s: status %d", webui.Prefix, name, rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
			t.Fatalf("GET %s%s: content type %q", webui.Prefix, name, got)
		}
	}
}

func TestHandlerServesCSS(t *testing.T) {
	mux := http.NewServeMux()
	webui.Mount(mux)

	for _, name := range shared {
		req := httptest.NewRequest(http.MethodGet, webui.Prefix+name, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s%s: status %d", webui.Prefix, name, rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
			t.Fatalf("GET %s%s: content type %q", webui.Prefix, name, got)
		}
	}
}

// Every token a served page uses has to be declared in tokens.css, because a
// page cannot see any other declaration.
func TestPagesUseDeclaredTokens(t *testing.T) {
	tokens, err := fs.ReadFile(webui.Assets(), "tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s*(--[a-z0-9-]+):`).FindAllStringSubmatch(string(tokens), -1) {
		declared[m[1]] = true
	}

	root := filepath.Join("..", "..")
	used := regexp.MustCompile(`var\((--[a-z0-9-]+)\)`)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".css" || strings.Contains(path, "vendor") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(b)
		local := map[string]bool{}
		for _, m := range regexp.MustCompile(`(?m)^\s*(--[a-z0-9-]+):`).FindAllStringSubmatch(text, -1) {
			local[m[1]] = true
		}
		for _, m := range used.FindAllStringSubmatch(text, -1) {
			if !declared[m[1]] && !local[m[1]] {
				t.Errorf("%s uses %s, which tokens.css does not declare", path, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
