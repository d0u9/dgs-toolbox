package filebrowse_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/filebrowse"
)

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(entries []filebrowse.Entry) []string {
	out := []string{}
	for _, entry := range entries {
		out = append(out, entry.Name)
	}
	return out
}

func equal(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestListFiltersByExtension(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "one.gpx"))
	write(t, filepath.Join(dir, "TWO.GPX"))
	write(t, filepath.Join(dir, "notes.txt"))
	write(t, filepath.Join(dir, ".hidden.gpx"))
	if err := os.Mkdir(filepath.Join(dir, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}

	listing, err := filebrowse.List(dir, filebrowse.Options{Extensions: []string{"gpx"}})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, names(listing.Files), []string{"one.gpx", "TWO.GPX"})
	// A folder is listed whatever the filter, because the dialog browses
	// through folders to reach the files it wants.
	equal(t, names(listing.Dirs), []string{"folder"})
	if listing.Parent != filepath.Dir(listing.Path) {
		t.Fatalf("parent %q", listing.Parent)
	}
}

func TestListWithoutFilterListsEveryFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "one.gpx"))
	write(t, filepath.Join(dir, "notes.txt"))

	listing, err := filebrowse.List(dir, filebrowse.Options{})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, names(listing.Files), []string{"notes.txt", "one.gpx"})
}

func TestListHidden(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".secret.gpx"))
	write(t, filepath.Join(dir, "plain.gpx"))

	visible, err := filebrowse.List(dir, filebrowse.Options{Extensions: []string{".gpx"}})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, names(visible.Files), []string{"plain.gpx"})

	all, err := filebrowse.List(dir, filebrowse.Options{Extensions: []string{".gpx"}, Hidden: true})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, names(all.Files), []string{".secret.gpx", "plain.gpx"})
}

func TestListRootHasNoParent(t *testing.T) {
	listing, err := filebrowse.List(string(filepath.Separator), filebrowse.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if listing.Parent != "" {
		t.Fatalf("parent %q, want empty", listing.Parent)
	}
}

func TestListMissingFolderFails(t *testing.T) {
	if _, err := filebrowse.List(filepath.Join(t.TempDir(), "gone"), filebrowse.Options{}); err == nil {
		t.Fatal("listing a folder that is not there has to fail")
	}
}

// A folder a symlink points at is a folder, so a linked folder can be
// stepped into.
func TestListFollowsLinkedFolder(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	listing, err := filebrowse.List(dir, filebrowse.Options{})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, names(listing.Dirs), []string{"link", "real"})
}

func TestPlacesStartWithTheRootAndHome(t *testing.T) {
	root := t.TempDir()
	places := filebrowse.Places(root)
	if len(places) == 0 {
		t.Fatal("no places")
	}
	if places[0].Path != root {
		t.Fatalf("first place %+v, want the root %q", places[0], root)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	found := false
	seen := map[string]bool{}
	for _, place := range places {
		if seen[place.Path] {
			t.Fatalf("%q listed twice", place.Path)
		}
		seen[place.Path] = true
		if place.Path == home && place.Name == "Home" {
			found = true
		}
	}
	if !found {
		t.Fatalf("home %q is not among %+v", home, places)
	}
}

// A place that is not there is not offered, so the dialog never shows a
// folder that cannot be opened.
func TestPlacesLeaveOutWhatIsNotThere(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	for _, place := range filebrowse.Places(gone) {
		if place.Path == gone {
			t.Fatalf("%q is offered although it is not there", gone)
		}
	}
}

func TestCheckNameRefusesAPath(t *testing.T) {
	for _, name := range []string{"", "   ", ".", "..", "a/b", `a\b`} {
		if err := filebrowse.CheckName(name); err == nil {
			t.Fatalf("%q is accepted as a name", name)
		}
	}
	if err := filebrowse.CheckName("walk.gpx"); err != nil {
		t.Fatal(err)
	}
}

func TestCreateFolder(t *testing.T) {
	dir := t.TempDir()
	path, err := filebrowse.CreateFolder(dir, " Day one ")
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("stat %q: %v", path, err)
	}
	if filepath.Base(path) != "Day one" {
		t.Fatalf("path %q", path)
	}
	// A name already there is refused, whatever it holds: nothing on disk is
	// replaced or merged.
	if _, err := filebrowse.CreateFolder(dir, "Day one"); !errors.Is(err, filebrowse.ErrExists) {
		t.Fatalf("second create = %v", err)
	}
	write(t, filepath.Join(dir, "walk.gpx"))
	if _, err := filebrowse.CreateFolder(dir, "walk.gpx"); !errors.Is(err, filebrowse.ErrExists) {
		t.Fatalf("create over a file = %v", err)
	}
	if _, err := filebrowse.CreateFolder(dir, "a/b"); !errors.Is(err, filebrowse.ErrPathName) {
		t.Fatalf("create with a path = %v", err)
	}
}

func TestRename(t *testing.T) {
	dir := t.TempDir()
	from := filepath.Join(dir, "walk.gpx")
	write(t, from)
	to, err := filebrowse.Rename(from, "west lake.gpx")
	if err != nil {
		t.Fatal(err)
	}
	if to != filepath.Join(dir, "west lake.gpx") {
		t.Fatalf("renamed to %q", to)
	}
	if _, err := os.Stat(from); err == nil {
		t.Fatal("the old name is still there")
	}
	// The name stays inside the folder, and a name already taken is refused,
	// so renaming never replaces a file.
	write(t, filepath.Join(dir, "other.gpx"))
	if _, err := filebrowse.Rename(to, "other.gpx"); !errors.Is(err, filebrowse.ErrExists) {
		t.Fatalf("rename onto a file = %v", err)
	}
	if _, err := filebrowse.Rename(to, "../escaped.gpx"); !errors.Is(err, filebrowse.ErrPathName) {
		t.Fatalf("rename out of the folder = %v", err)
	}
	if _, err := filebrowse.Rename(filepath.Join(dir, "gone.gpx"), "x.gpx"); err == nil {
		t.Fatal("renaming what is not there has to fail")
	}
}

func TestTaken(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "walk.gpx"))
	// Only the name as it was written and a name that is not there: whether
	// "Walk.gpx" is taken is the filesystem's answer, not this package's —
	// APFS and NTFS say yes, ext4 says no, and both are right.
	for name, want := range map[string]bool{"walk.gpx": true, "free.gpx": false} {
		got, err := filebrowse.Taken(dir, name)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("Taken(%q) = %v, want %v", name, got, want)
		}
	}
}
