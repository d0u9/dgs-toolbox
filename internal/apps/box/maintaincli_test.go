package box

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/digest"
	"dgs-toolbox/internal/box/maintain"
	"dgs-toolbox/internal/box/publish"
	"dgs-toolbox/internal/box/sidecar"
	"dgs-toolbox/internal/config"
)

var intake = box.Date{Year: 2026, Month: 9, Day: 22}

// boxWith returns a Box holding one published scan per body, and the
// configuration the actions read.
func boxWith(t *testing.T, bodies ...string) (string, config.Config) {
	t.Helper()
	root := t.TempDir()
	global := config.Default()
	global.Box.Root = root
	global.Box.CacheDir = t.TempDir()
	if err := maintain.Init(root, global.BoxMarker(), time.Now()); err != nil {
		t.Fatalf("init: %v", err)
	}
	for i, body := range bodies {
		inbox := t.TempDir()
		path := filepath.Join(inbox, "Scan_00"+string(rune('1'+i))+".pdf")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := publish.Publish(context.Background(), publish.Request{
			Root: root, Source: path, IntakeDate: intake, Digest: digest.Whole([]byte(body)),
			Sidecar: sidecar.File{Kind: sidecar.KindPDF, Type: "unsorted"},
		}); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}
	return root, global
}

func TestIndexActionReportsWhatItIndexed(t *testing.T) {
	_, global := boxWith(t, "a boarding pass", "a power bill")

	var out bytes.Buffer
	if err := indexAction(nil, &out, nil, nil, global); err != nil {
		t.Fatalf("indexAction: %v", err)
	}
	if !strings.Contains(out.String(), "2 scans indexed") {
		t.Errorf("output does not say what was indexed:\n%s", out.String())
	}
}

func TestVerifyActionPassesAndNamesTheCost(t *testing.T) {
	_, global := boxWith(t, "a boarding pass")

	var out bytes.Buffer
	if err := verifyAction(nil, &out, nil, map[string]string{"quiet": "true"}, global); err != nil {
		t.Fatalf("verifyAction on a sound Box: %v", err)
	}
	if !strings.Contains(out.String(), "1 files read") {
		t.Errorf("output does not say what it read:\n%s", out.String())
	}
}

// TestVerifyActionFailsOnAMismatch is the whole reason verify is worth running
// from a schedule: a Box that no longer holds what it says it holds must not
// exit zero.
func TestVerifyActionFailsOnAMismatch(t *testing.T) {
	root, global := boxWith(t, "a boarding pass")
	rot(t, root)

	var out bytes.Buffer
	err := verifyAction(nil, &out, nil, map[string]string{"quiet": "true"}, global)
	if err == nil {
		t.Fatal("verifyAction on a changed file: want an error so the exit status is not zero")
	}
	if !strings.Contains(out.String(), "recorded") || !strings.Contains(out.String(), "bytes give") {
		t.Errorf("the report does not say what changed to what:\n%s", out.String())
	}
	if strings.Contains(err.Error(), "0 of") {
		t.Errorf("error does not count the mismatches: %v", err)
	}
}

// TestVerifyActionReportsProgress covers the reason the TUI was not worth
// keeping: without this, a run reads tens of gigabytes in silence.
func TestVerifyActionReportsProgress(t *testing.T) {
	_, global := boxWith(t, "a boarding pass", "a power bill")

	var out bytes.Buffer
	if err := verifyAction(nil, &out, nil, nil, global); err != nil {
		t.Fatalf("verifyAction: %v", err)
	}
	if strings.Count(out.String(), "reading ") != 2 {
		t.Errorf("want one progress line per file:\n%s", out.String())
	}
}

func TestDedupeActionGroupsAndDiscardsNothing(t *testing.T) {
	root, global := boxWith(t, "a boarding pass", "a boarding pass")

	before := scanCount(t, root)
	var out bytes.Buffer
	if err := dedupeAction(nil, &out, nil, nil, global); err != nil {
		t.Fatalf("dedupeAction: %v", err)
	}
	if !strings.Contains(out.String(), "1 documents are in the Box more than once") {
		t.Errorf("output does not report the duplicate:\n%s", out.String())
	}
	if after := scanCount(t, root); after != before {
		t.Errorf("dedupe changed the Box: %d files before, %d after", before, after)
	}
}

// TestActionsRefuseWithoutARoot covers the case that would otherwise read
// whatever directory the shell happened to be in.
func TestActionsRefuseWithoutARoot(t *testing.T) {
	for name, action := range map[string]func(_ io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error{
		"index":  indexAction,
		"verify": verifyAction,
		"dedupe": dedupeAction,
	} {
		err := action(nil, &bytes.Buffer{}, nil, nil, config.Default())
		if err == nil || !strings.Contains(err.Error(), "box.root") {
			t.Errorf("%s with no directory and no box.root: got %v, want an error naming the setting", name, err)
		}
	}
}

// rot changes the bytes of the one scan in the Box without touching its
// sidecar, which is what verify exists to find.
func rot(t *testing.T, root string) {
	t.Helper()
	changed := false
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".pdf" || changed {
			return err
		}
		if err := os.WriteFile(path, []byte("rot"), 0o644); err != nil {
			t.Fatal(err)
		}
		changed = true
		return nil
	})
	if !changed {
		t.Fatal("no scan to change")
	}
}

func scanCount(t *testing.T, root string) int {
	t.Helper()
	count := 0
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			count++
		}
		return err
	})
	return count
}
