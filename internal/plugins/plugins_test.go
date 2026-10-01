package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func fixture(t *testing.T, files map[string][]byte) (Target, Plugin) {
	t.Helper()
	target := Target{Host: "test", Place: "here", Dir: filepath.Join(t.TempDir(), "plugins")}
	plugin := Plugin{Host: "test", ID: "demo", Files: func() (map[string][]byte, error) { return files, nil }}
	return target, plugin
}

func mustInspect(t *testing.T, target Target, plugin Plugin) Status {
	t.Helper()
	status, err := Inspect(target, plugin)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func TestInstallWritesTheFilesAndARecord(t *testing.T) {
	target, plugin := fixture(t, map[string][]byte{"main.js": []byte("one"), "sub/a.txt": []byte("a")})
	if got := mustInspect(t, target, plugin).State; got != Missing {
		t.Fatalf("before: %s", got)
	}
	status, err := Install(target, plugin, "dgs test", false, now)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != Current || status.Installed.By != "dgs test" || status.Installed.Version != status.Bundled {
		t.Fatalf("after: %+v", status)
	}
	data, err := os.ReadFile(filepath.Join(target.Dir, "demo", "sub", "a.txt"))
	if err != nil || string(data) != "a" {
		t.Fatalf("sub/a.txt: %q %v", data, err)
	}
}

func TestANewerBundleIsOutdatedAndUpdateRemovesWhatItNoLongerCarries(t *testing.T) {
	target, plugin := fixture(t, map[string][]byte{"main.js": []byte("one"), "old.js": []byte("x")})
	if _, err := Install(target, plugin, "", false, now); err != nil {
		t.Fatal(err)
	}
	plugin.Files = func() (map[string][]byte, error) { return map[string][]byte{"main.js": []byte("two")}, nil }
	if got := mustInspect(t, target, plugin).State; got != Outdated {
		t.Fatalf("state %s", got)
	}
	if _, err := Install(target, plugin, "", false, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target.Dir, "demo", "old.js")); !os.IsNotExist(err) {
		t.Fatalf("old.js still there: %v", err)
	}
	if got := mustInspect(t, target, plugin).State; got != Current {
		t.Fatalf("state %s", got)
	}
}

func TestAFileChangedByHandIsNotOverwrittenWithoutForce(t *testing.T) {
	target, plugin := fixture(t, map[string][]byte{"main.js": []byte("one")})
	if _, err := Install(target, plugin, "", false, now); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(target.Dir, "demo", "main.js")
	if err := os.WriteFile(path, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	status := mustInspect(t, target, plugin)
	if status.State != Modified || strings.Join(status.Changed, ",") != "main.js" {
		t.Fatalf("%+v", status)
	}
	if _, err := Install(target, plugin, "", false, now); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("install without force: %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "mine" {
		t.Fatalf("overwritten: %q", data)
	}
	if status, err := Install(target, plugin, "", true, now); err != nil || status.State != Current {
		t.Fatalf("force: %v %+v", err, status)
	}
}

func TestAFolderDgsDidNotWriteIsUnmanaged(t *testing.T) {
	target, plugin := fixture(t, map[string][]byte{"main.js": []byte("one")})
	dir := filepath.Join(target.Dir, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The host's settings alone are not an install.
	if err := os.WriteFile(filepath.Join(dir, "data.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := mustInspect(t, target, plugin).State; got != Missing {
		t.Fatalf("settings only: %s", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.js"), []byte("theirs"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := mustInspect(t, target, plugin).State; got != Unmanaged {
		t.Fatalf("state %s", got)
	}
	if _, err := Uninstall(target, plugin, true); err == nil {
		t.Fatal("uninstalled a folder dgs did not write")
	}
}

func TestUninstallKeepsTheHostsSettings(t *testing.T) {
	target, plugin := fixture(t, map[string][]byte{"main.js": []byte("one")})
	if _, err := Install(target, plugin, "", false, now); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(target.Dir, "demo")
	if err := os.WriteFile(filepath.Join(dir, "data.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := Uninstall(target, plugin, false)
	if err != nil || status.State != Missing {
		t.Fatalf("%v %+v", err, status)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "data.json" {
		t.Fatalf("left %v", entries)
	}
	if err := os.Remove(filepath.Join(dir, "data.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(target, plugin, "", false, now); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(target, plugin, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("empty folder left: %v", err)
	}
}

func TestVersionIsTheContentNotTheOrder(t *testing.T) {
	a := Version(map[string][]byte{"a": []byte("1"), "b": []byte("2")})
	b := Version(map[string][]byte{"b": []byte("2"), "a": []byte("1")})
	c := Version(map[string][]byte{"a": []byte("2"), "b": []byte("1")})
	if a != b || a == c || len(a) != 12 {
		t.Fatalf("%s %s %s", a, b, c)
	}
}

func TestFilesGoIntoTheirFolderBesideTheUsersOwn(t *testing.T) {
	root := t.TempDir()
	target := Target{Host: "test", Kind: KindFiles, Place: "vault", Dir: root, Root: root}
	plugin := Plugin{Host: "test", ID: "public", Kind: KindFiles, Folder: "Scripts/DGS",
		Files: func() (map[string][]byte, error) { return map[string][]byte{"a.js": []byte("dgs")}, nil }}
	dir := filepath.Join(root, "Scripts", "DGS")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mine.js"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := mustInspect(t, target, plugin).State; got != Missing {
		t.Fatalf("a folder with only the user's files: %s", got)
	}
	if _, err := Install(target, plugin, "", false, now); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(target, plugin, false); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "mine.js")); err != nil || string(data) != "mine" {
		t.Fatalf("the user's file after uninstall: %q %v", data, err)
	}
}

func TestInstallRefusesToWriteOverAFileItDidNotWrite(t *testing.T) {
	target, plugin := fixture(t, map[string][]byte{"main.js": []byte("one")})
	if _, err := Install(target, plugin, "", false, now); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target.Dir, "demo", "new.js"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	plugin.Files = func() (map[string][]byte, error) {
		return map[string][]byte{"main.js": []byte("one"), "new.js": []byte("dgs")}, nil
	}
	if _, err := Install(target, plugin, "", false, now); err == nil || !strings.Contains(err.Error(), "new.js") {
		t.Fatalf("want a refusal naming new.js, got %v", err)
	}
	if _, err := Install(target, plugin, "", true, now); err != nil {
		t.Fatal(err)
	}
}
