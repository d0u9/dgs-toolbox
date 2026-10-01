package plugins

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/config"
)

func vaultFlags(t *testing.T, vault string) map[string]string {
	t.Helper()
	raw, err := json.Marshal([]string{vault})
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{"vault": string(raw)}
}

// A dgs built without make plugins cannot install the Obsidian plugin itself,
// so these tests leave its error aside and look at everything else.
func TestBootstrapSetsUpAFreshVaultAndSaysWhatIsLeft(t *testing.T) {
	vault := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vault, ".obsidian"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, ".obsidian", "app.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	global := config.Default()
	var out bytes.Buffer
	_ = bootstrap(&out, vaultFlags(t, vault), global)

	for _, path := range []string{
		config.DefaultPluginsObsidianPublic + "/quickAddShim.js",
		config.DefaultPluginsObsidianQuickAdd + "/quickNote.js",
		config.DefaultPluginsObsidianTemplates + "/Daily Log Template.md",
		config.DefaultPluginsObsidianTimelines,
	} {
		if _, err := os.Stat(filepath.Join(vault, filepath.FromSlash(path))); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(vault, filepath.FromSlash(config.DefaultPluginsObsidianQuickAdd), "quickNote.js"))
	if err != nil || !strings.Contains(string(data), `"`+config.DefaultPluginsObsidianPublic+`"`) || strings.Contains(string(data), "{{dgs:") {
		t.Errorf("quickNote.js does not name the Public folder: %v", err)
	}
	for _, todo := range []string{"install Templater", "install QuickAdd", "Script files folder location"} {
		if !strings.Contains(out.String(), todo) {
			t.Errorf("no todo %q in:\n%s", todo, out.String())
		}
	}
}

func TestBootstrapKeepsWhatIsThereAndTakesExportedFiles(t *testing.T) {
	vault := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vault, ".obsidian"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, ".obsidian", "app.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	from := t.TempDir()
	if err := os.WriteFile(filepath.Join(from, "timelines.json"), []byte("exported"), 0o644); err != nil {
		t.Fatal(err)
	}
	global := config.Default()
	flags := vaultFlags(t, vault)
	flags["from"] = from
	var out bytes.Buffer
	_ = bootstrap(&out, flags, global)
	timelines := filepath.Join(vault, filepath.FromSlash(config.DefaultPluginsObsidianTimelines))
	if data, _ := os.ReadFile(timelines); string(data) != "exported" {
		t.Fatalf("timelines.json %q", data)
	}

	if err := os.WriteFile(timelines, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = bootstrap(&out, vaultFlags(t, vault), global)
	if data, _ := os.ReadFile(timelines); string(data) != "mine" {
		t.Fatalf("bootstrap replaced timelines.json: %q", data)
	}

	exported := t.TempDir()
	if err := export(&out, exported, vaultFlags(t, vault), global); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(exported, "timelines.json")); string(data) != "mine" {
		t.Fatalf("exported timelines.json %q", data)
	}
	if err := export(&out, exported, vaultFlags(t, vault), global); err == nil {
		t.Fatal("export wrote over an earlier export without --force")
	}
}
