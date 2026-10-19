package photo

import (
	"bytes"
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/verifiedcopy"
)

func TestCLIImportProfileFillsUnsetFlagsAndNeverMoves(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "card")
	library, other := filepath.Join(root, "library"), filepath.Join(root, "other")
	for _, dir := range []string{library, other} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeTree(t, source, "DCIM/IMG_0001.JPG", "DCIM/IMG_0001.DNG")
	global := config.Default()
	global.Photo.Import.Profiles = map[string]config.PhotoImportProfile{
		"card": {Source: source, Dest: []string{library}, Ext: []string{"JPG"}},
	}
	flags := map[string]string{"profile": "card", "dest": "[]", "ext": "[]", "workers": "1", "duplicates": "skip", "incremental": "true", "move": "false", tui.GivenFlags: "[]"}
	var out bytes.Buffer
	if err := importDirectoriesContext(context.Background(), &out, nil, flags, global); err != nil {
		t.Fatal(err, out.String())
	}
	if _, err := os.Stat(filepath.Join(library, "DCIM", "IMG_0001.JPG")); err != nil {
		t.Fatal("profile destination not used", err)
	}
	if _, err := os.Stat(filepath.Join(library, "DCIM", "IMG_0001.DNG")); err == nil {
		t.Fatal("profile ext not applied")
	}
	if _, err := os.Stat(filepath.Join(source, "DCIM", "IMG_0001.JPG")); err != nil {
		t.Fatal("a profile moved the source")
	}

	// A given --dest replaces the profile's list.
	flags["dest"] = `["` + other + `"]`
	flags[tui.GivenFlags] = `["profile","dest"]`
	if err := importDirectoriesContext(context.Background(), &out, nil, flags, global); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(other, "DCIM", "IMG_0001.JPG")); err != nil {
		t.Fatal("given --dest not used", err)
	}

	if err := importDirectoriesContext(context.Background(), &out, []string{source}, flags, global); err == nil || !strings.Contains(err.Error(), "not given with --profile") {
		t.Fatalf("source with profile: %v", err)
	}
	flags["profile"] = "nope"
	if err := importDirectoriesContext(context.Background(), &out, nil, flags, global); err == nil || !strings.Contains(err.Error(), "configured: card") {
		t.Fatalf("unknown profile: %v", err)
	}
}

func TestWithImportProfileFillsOnlyFlagsNotGiven(t *testing.T) {
	incremental := false
	global := config.Default()
	global.Photo.Import.Profiles = map[string]config.PhotoImportProfile{"card": {
		Source: "/card", Dest: []string{"/a", "/b"}, Ext: []string{"JPG"}, Duplicates: "keep-both", Workers: 4, Incremental: &incremental, Organize: true,
	}}
	defaults := map[string]string{"profile": "card", "dest": "[]", "ext": "[]", "workers": "1", "duplicates": "skip", "incremental": "true", "organize": "false", "move": "false"}
	flags := maps.Clone(defaults)
	flags[tui.GivenFlags] = `["profile"]`
	args, merged, err := withImportProfile(nil, flags, global)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"dest": `["/a","/b"]`, "ext": `["JPG"]`, "workers": "4", "duplicates": "keep-both", "incremental": "false", "organize": "true", "move": "false"}
	for key, value := range want {
		if merged[key] != value {
			t.Errorf("%s = %q, want %q", key, merged[key], value)
		}
	}
	if !slices.Equal(args, []string{"/card"}) || flags["workers"] != "1" {
		t.Fatal("source not taken from the profile, or the caller's flags changed")
	}
	flags["workers"], flags["organize"] = "2", "false"
	flags[tui.GivenFlags] = `["profile","workers","organize"]`
	_, merged, _ = withImportProfile(nil, flags, global)
	if merged["workers"] != "2" || merged["organize"] != "false" {
		t.Fatalf("given flags overridden: workers %q organize %q", merged["workers"], merged["organize"])
	}
	// Without --profile nothing changes.
	defaults["profile"] = ""
	args, merged, _ = withImportProfile([]string{"/x"}, defaults, global)
	if !slices.Equal(args, []string{"/x"}) || merged["workers"] != "1" {
		t.Fatal("a run without --profile changed")
	}
}

func TestTailFitKeepsThePathEnd(t *testing.T) {
	for _, c := range []struct {
		text  string
		width int
		want  string
	}{{"/a/b/card", 20, "/a/b/card"}, {"/volumes/sd/dcim", 8, "…sd/dcim"}, {"相机/卡", 4, "…/卡"}, {"相机/卡", 3, "…卡"}} {
		if got := tailFit(c.text, c.width); got != c.want {
			t.Errorf("tailFit(%q, %d) = %q, want %q", c.text, c.width, got, c.want)
		}
	}
}

func TestAllowCheckRenameLocksEveryDestinationUntilReleased(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	var out bytes.Buffer
	release, err := allowCheckRename(&out, []string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Warning:") {
		t.Fatal("no warning printed")
	}
	for _, root := range []string{a, b} {
		if _, err := os.Stat(filepath.Join(root, verifiedcopy.LockName)); err != nil {
			t.Fatal("destination not locked", err)
		}
	}
	// A second run on a shared destination is refused and leaves no lock of its own.
	c := t.TempDir()
	if _, err := allowCheckRename(&out, []string{c, b}); err == nil || !strings.Contains(err.Error(), "held by another import") {
		t.Fatalf("second run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c, verifiedcopy.LockName)); !os.IsNotExist(err) {
		t.Fatal("a refused run left its lock behind")
	}
	release()
	for _, root := range []string{a, b} {
		if _, err := os.Stat(filepath.Join(root, verifiedcopy.LockName)); !os.IsNotExist(err) {
			t.Fatal("lock left after release")
		}
	}
}

func TestProfileCarriesAllowCheckRename(t *testing.T) {
	global := config.Default()
	global.Photo.Import.Profiles = map[string]config.PhotoImportProfile{"nas": {Source: "/card", Dest: []string{"/nas"}, AllowCheckRename: true}}
	_, merged, err := withImportProfile(nil, map[string]string{"profile": "nas", "allow-check-rename": "false", tui.GivenFlags: `["profile"]`}, global)
	if err != nil || merged["allow-check-rename"] != "true" {
		t.Fatalf("allow-check-rename = %q, %v", merged["allow-check-rename"], err)
	}
}

func TestProfileExtensionsMissingFromTheCardAreSkipped(t *testing.T) {
	root := t.TempDir()
	source, library := filepath.Join(root, "card"), filepath.Join(root, "library")
	writeTree(t, source, "DCIM/IMG_0001.CR2")
	if err := os.MkdirAll(library, 0o755); err != nil {
		t.Fatal(err)
	}
	global := config.Default()
	global.Photo.Import.Profiles = map[string]config.PhotoImportProfile{"raw": {Source: source, Dest: []string{library}, Ext: []string{"JPG", "CR2"}}}
	flags := map[string]string{"profile": "raw", "dest": "[]", "ext": "[]", "workers": "1", "duplicates": "skip", "incremental": "true", tui.GivenFlags: `["profile"]`}
	var out bytes.Buffer
	if err := importDirectoriesContext(context.Background(), &out, nil, flags, global); err != nil {
		t.Fatal(err, out.String())
	}
	if !strings.Contains(out.String(), "No JPG files in") {
		t.Fatalf("no notice: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(library, "DCIM", "IMG_0001.CR2")); err != nil {
		t.Fatal("CR2 not imported", err)
	}
	// A typed --ext still refuses a misspelling.
	flags["ext"], flags[tui.GivenFlags] = `["JPG"]`, `["profile","ext"]`
	if err := importDirectoriesContext(context.Background(), &out, nil, flags, global); err == nil || !strings.Contains(err.Error(), "has extension JPG") {
		t.Fatalf("typed ext: %v", err)
	}
}

func TestFailedImportSaysSoInItsLastLine(t *testing.T) {
	root := t.TempDir()
	source, library := filepath.Join(root, "card"), filepath.Join(root, "library")
	writeTree(t, source, "DCIM/IMG_0001.JPG")
	// The destination's folder for DCIM is a file: nothing can be published.
	writeTree(t, library, "DCIM")
	flags := map[string]string{"dest": `["` + library + `"]`, "ext": "[]", "workers": "1", "duplicates": "skip", "incremental": "true", tui.GivenFlags: "[]"}
	var out bytes.Buffer
	err := importDirectoriesContext(context.Background(), &out, []string{source}, flags, config.Default())
	if err == nil || !strings.HasPrefix(err.Error(), "IMPORT FAILED: none of 1 files") {
		t.Fatalf("err = %v\n%s", err, out.String())
	}
}
