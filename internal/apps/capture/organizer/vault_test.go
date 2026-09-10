package organizer

import (
	"os"
	"path/filepath"
	"testing"
)

func writeVaultConfig(t *testing.T, vault, name, body string) {
	t.Helper()
	path := filepath.Join(vault, ".obsidian", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The core plugin's file stays behind when the plugin is switched off, so a
// stale file must not win over the plugin actually creating the notes.
func TestReadVaultSettingsPrefersThePluginInUse(t *testing.T) {
	vault := t.TempDir()
	writeVaultConfig(t, vault, "core-plugins.json", `{"daily-notes": false, "templates": true}`)
	writeVaultConfig(t, vault, "community-plugins.json", `["periodic-notes", "dataview"]`)
	writeVaultConfig(t, vault, "daily-notes.json", `{"folder": "Stale", "template": "old.md"}`)
	writeVaultConfig(t, vault, "plugins/periodic-notes/data.json",
		`{"daily": {"format": "", "folder": "00 Daily Log", "enabled": true}}`)

	settings := ReadVaultSettings(vault)
	if settings.Folder != "00 Daily Log" {
		t.Fatalf("folder = %q, want the folder of the plugin in use", settings.Folder)
	}

	// With the core plugin on and periodic notes gone, the core file wins.
	writeVaultConfig(t, vault, "core-plugins.json", `{"daily-notes": true}`)
	writeVaultConfig(t, vault, "community-plugins.json", `[]`)
	if got := ReadVaultSettings(vault).Folder; got != "Stale" {
		t.Fatalf("folder = %q, want the core plugin's folder", got)
	}
}

func TestReadVaultSettingsFallsBackToTheDefaults(t *testing.T) {
	if got := ReadVaultSettings(t.TempDir()); got.Folder != "Daily" || got.Format != "" {
		t.Fatalf("settings = %+v, want the defaults", got)
	}
	if got := ReadVaultSettings(""); got.Folder != "Daily" {
		t.Fatalf("settings = %+v for no vault", got)
	}
}

// Obsidian writes Moment.js patterns and Go formats by example, so a pattern is
// translated. An unknown token is an error rather than a guess.
func TestMomentLayout(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
		wantErr bool
	}{
		{pattern: "", want: "2006-01-02"},
		{pattern: "YYYY-MM-DD", want: "2006-01-02"},
		{pattern: "YYYY/MM/DD", want: "2006/01/02"},
		{pattern: "YYYY-MM-DD dddd", want: "2006-01-02 Monday"},
		{pattern: "[Daily] YYYY-MM-DD", want: "Daily 2006-01-02"},
		{pattern: "YYYY-MM-DD HH:mm", want: "2006-01-02 15:04"},
		{pattern: "YYYY-DDDD", wantErr: true},
		{pattern: "YYYY-QQ", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.pattern, func(t *testing.T) {
			got, err := momentLayout(tc.pattern)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("layout = %q, want an error naming the token", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("layout = %q, want %q", got, tc.want)
			}
		})
	}
}

// The filename follows the vault's own format, so a Capture lands in the file
// the vault would have created for that day.
func TestDailyNoteNameFollowsTheVaultFormat(t *testing.T) {
	settings := DefaultSettings()
	settings.DailyFormat = "YYYY/YYYY-MM-DD"
	ctx := NewContext(beenHere(), nil).WithSettings(settings)
	name, err := dailyNoteName(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if name != "2026/2026-09-09.md" {
		t.Fatalf("name = %q", name)
	}
}
