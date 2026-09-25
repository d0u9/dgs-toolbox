package target

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	root := t.TempDir()
	if got, err := Load(root); err != nil || len(got) != 0 {
		t.Fatalf("no file: %v %v", got, err)
	}
	want := []Target{{Name: "kindle", Folder: "~/Books/Kindle"}, {Name: "icloud", About: "phone", Folder: "/icloud"}}
	if err := Save(root, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil || len(got) != 2 || got[0].Name != "icloud" || got[0].About != "phone" || got[1].Folder != "~/Books/Kindle" {
		t.Fatalf("%+v %v", got, err)
	}
	if err := Save(root, []Target{{Name: "a"}, {Name: "a"}}); err == nil {
		t.Fatal("two of one name saved")
	}
	if err := Save(root, []Target{{Name: "a", Folder: "relative/x"}}); err == nil {
		t.Fatal("a relative folder saved")
	}
	os.WriteFile(filepath.Join(root, File), []byte("x:\n  where: /y\n"), 0o644)
	if _, err := Load(root); err == nil {
		t.Fatal("an unknown key was read")
	}
}

func TestFoldersPreferTheChosenOne(t *testing.T) {
	targets := []Target{{Name: "kindle", Folder: "~/Books/Kindle"}, {Name: "icloud", Folder: "/icloud"}, {Name: "usb"}}
	got := Folders(targets, map[string]string{"icloud": "/Volumes/other/"}, "/home/jane")
	if got["kindle"] != "/home/jane/Books/Kindle" || got["icloud"] != "/Volumes/other" || got["usb"] != "" {
		t.Fatalf("%v", got)
	}
}
