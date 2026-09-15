package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTrashDarwin(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 9, 15, 14, 0, 0, 0, time.UTC)
	for i := 0; i < 2; i++ {
		path := filepath.Join(home, "laptop.agekey")
		if err := os.WriteFile(path, []byte("key"), 0o600); err != nil {
			t.Fatal(err)
		}
		moved, err := trash("darwin", home, "", path, now)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); err == nil {
			t.Error("original still there")
		}
		want := filepath.Join(home, ".Trash", "laptop.agekey")
		if i == 1 {
			want = filepath.Join(home, ".Trash", "laptop 20260915-140000-1.agekey")
		}
		if moved != want {
			t.Errorf("moved to %s, want %s", moved, want)
		}
	}
}

func TestTrashFreedesktop(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "a key.agekey")
	if err := os.WriteFile(path, []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	moved, err := trash("linux", home, "", path, time.Date(2026, 9, 15, 14, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if moved != filepath.Join(home, ".local/share/Trash/files/a key.agekey") {
		t.Errorf("moved to %s", moved)
	}
	info, err := os.ReadFile(filepath.Join(home, ".local/share/Trash/info/a key.agekey.trashinfo"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(info), "Path="+strings.ReplaceAll(path, " ", "%20")) || !strings.Contains(string(info), "DeletionDate=2026-09-15T14:00:00") {
		t.Errorf("info:\n%s", info)
	}
}
