package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeBoxConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ExportFilename)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// TestLoadPath_BoxDefaults pins the defaults docs/configuration/box.md
// documents: a file naming no box key still reads as a usable Box setting.
func TestLoadPath_BoxDefaults(t *testing.T) {
	config, err := LoadPath(writeBoxConfig(t, `{}`))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	box := config.BoxSettings()
	if box.Marker != DefaultBoxMarker || box.StateFile != DefaultBoxStateFile {
		t.Errorf("marker %q state file %q", box.Marker, box.StateFile)
	}
	if box.IndexFile != DefaultBoxIndexFile || box.Workers != DefaultBoxWorkers {
		t.Errorf("index file %q workers %d", box.IndexFile, box.Workers)
	}
	if box.Web.Port != DefaultBoxWebPort || box.Thumbnail.Size != DefaultBoxThumbnail {
		t.Errorf("port %d thumbnail %d", box.Web.Port, box.Thumbnail.Size)
	}
	if box.Preview.Size != DefaultBoxPreviewSize || *box.Preview.Keep != DefaultBoxPreviewKeep {
		t.Errorf("preview %d keep %d", box.Preview.Size, *box.Preview.Keep)
	}
	if box.Trash.Dir != DefaultBoxTrashDir || *box.Trash.Keep != DefaultBoxTrashKeep {
		t.Errorf("trash %q keep %d", box.Trash.Dir, *box.Trash.Keep)
	}
	kept, err := LoadPath(writeBoxConfig(t, `{"box":{"preview":{"keep":0},"trash":{"keep":0}}}`))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	box = kept.BoxSettings()
	if *box.Preview.Keep != 0 || *box.Trash.Keep != 0 {
		t.Errorf("an explicit 0 was replaced by a default")
	}
}

// TestLoadPath_BoxExpandsPaths checks a leading ~ is the home directory, so a
// caller never expands one itself.
func TestLoadPath_BoxExpandsPaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	config, err := LoadPath(writeBoxConfig(t, `{"box":{"root":"~/Box","inbox":"~/Scans"}}`))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if want := filepath.Join(home, "Box"); config.Box.Root != want {
		t.Errorf("root %q, want %q", config.Box.Root, want)
	}
	if want := filepath.Join(home, "Scans"); config.Box.Inbox != want {
		t.Errorf("inbox %q, want %q", config.Box.Inbox, want)
	}
}

// TestLoadPath_BoxRefuses covers each refusal box.md states, because a setting
// that cannot mean what it says is worse silently accepted than reported.
func TestLoadPath_BoxRefuses(t *testing.T) {
	for name, body := range map[string]string{
		"marker path":      `{"box":{"marker":"a/.dgs-box"}}`,
		"state file path":  `{"box":{"state_file":"a/.dgs-box-state"}}`,
		"index file path":  `{"box":{"index_file":"a/index.json"}}`,
		"trash path":       `{"box":{"trash":{"dir":"a/trash"}}}`,
		"negative workers": `{"box":{"workers":-1}}`,
		"timezone offset":  `{"box":{"timezone":"+10:00"}}`,
		"unknown zone":     `{"box":{"timezone":"Mars/Olympus"}}`,
		"currency":         `{"box":{"currency":"aud"}}`,
		"negative keep":    `{"box":{"trash":{"keep":-1}}}`,
		"lifetime":         `{"box":{"lifetimes":{"receipt":-1}}}`,
		"unknown key":      `{"box":{"nope":1}}`,
	} {
		if _, err := LoadPath(writeBoxConfig(t, body)); err == nil {
			t.Errorf("%s: loaded without an error", name)
		}
	}
}

// TestBoxLifetime pins that an unnamed type is reported as unnamed rather than
// as a zero lifetime, which would make it permanent.
func TestBoxLifetime(t *testing.T) {
	config, err := LoadPath(writeBoxConfig(t, `{"box":{"lifetimes":{"receipt":3650,"warranty":0}}}`))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if days, ok := config.BoxLifetime("receipt"); !ok || days != 3650 {
		t.Errorf("receipt %d %v", days, ok)
	}
	if days, ok := config.BoxLifetime("warranty"); !ok || days != 0 {
		t.Errorf("warranty %d %v", days, ok)
	}
	if _, ok := config.BoxLifetime("passport"); ok {
		t.Errorf("passport is named")
	}
}
