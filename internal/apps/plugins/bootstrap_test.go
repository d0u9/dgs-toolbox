package plugins

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/entrytemplate"
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
	data, err := os.ReadFile(filepath.Join(vault, filepath.FromSlash(config.DefaultPluginsObsidianQuickAdd), "captureIdea.js"))
	if err != nil || !strings.Contains(string(data), `"`+config.DefaultPluginsObsidianPublic+`"`) || strings.Contains(string(data), "{{dgs:") {
		t.Errorf("captureIdea.js does not name the Public folder: %v", err)
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

// freshVault is a vault with only its desktop configuration folder.
func freshVault(t *testing.T) string {
	t.Helper()
	vault := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vault, ".obsidian"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, ".obsidian", "app.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return vault
}

// A fresh vault starts with the compiled-in entry templates, the ones dgs
// capture and the plugin would otherwise fall back to, so there is a file to
// change rather than one to go and find out about.
func TestBootstrapPlantsTheEntryTemplates(t *testing.T) {
	vault := freshVault(t)
	var out bytes.Buffer
	_ = bootstrap(&out, vaultFlags(t, vault), config.Default())
	for _, name := range []string{"daily-entry.md", "location-entry.md", "quick-note.md", "daily-note.md"} {
		data, err := os.ReadFile(filepath.Join(vault, filepath.FromSlash(config.DefaultPluginsObsidianEntries), name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		builtin, _ := entrytemplate.Builtin(name)
		if string(data) != builtin {
			t.Errorf("%s is not the compiled-in template", name)
		}
		if !strings.Contains(out.String(), "written  "+config.DefaultPluginsObsidianEntries+"/"+name) {
			t.Errorf("bootstrap did not say it wrote %s:\n%s", name, out.String())
		}
	}
	for _, absent := range []string{"timeline-entry.md"} {
		if _, err := os.Stat(filepath.Join(vault, filepath.FromSlash(config.DefaultPluginsObsidianEntries), absent)); err == nil {
			t.Errorf("bootstrap invented %s", absent)
		}
	}
	if strings.Contains(out.String(), "todo     fix ") {
		t.Errorf("a planted template does not render:\n%s", out.String())
	}
}

// The entry templates are the reader's once planted: kept on the next run,
// taken from an export, and carried by the next export.
func TestBootstrapKeepsExportsAndTakesEntryTemplates(t *testing.T) {
	vault := freshVault(t)
	global := config.Default()
	from := t.TempDir()
	if err := os.WriteFile(filepath.Join(from, "daily-entry.md"), []byte("- exported {{.Content}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	flags := vaultFlags(t, vault)
	flags["from"] = from
	var out bytes.Buffer
	_ = bootstrap(&out, flags, global)
	folder := filepath.Join(vault, filepath.FromSlash(config.DefaultPluginsObsidianEntries))
	if data, _ := os.ReadFile(filepath.Join(folder, "daily-entry.md")); string(data) != "- exported {{.Content}}\n" {
		t.Fatalf("daily-entry.md = %q, want the exported one", data)
	}
	builtin, _ := entrytemplate.Builtin("location-entry.md")
	if data, _ := os.ReadFile(filepath.Join(folder, "location-entry.md")); string(data) != builtin {
		t.Fatal("location-entry.md, missing from the export, is not the compiled-in one")
	}

	if err := os.WriteFile(filepath.Join(folder, "location-entry.md"), []byte("- mine {{.Content}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	_ = bootstrap(&out, vaultFlags(t, vault), global)
	if data, _ := os.ReadFile(filepath.Join(folder, "location-entry.md")); string(data) != "- mine {{.Content}}\n" {
		t.Fatalf("bootstrap replaced location-entry.md: %q", data)
	}
	if !strings.Contains(out.String(), "kept     "+config.DefaultPluginsObsidianEntries+"/location-entry.md") {
		t.Errorf("bootstrap did not say it kept location-entry.md:\n%s", out.String())
	}

	exported := t.TempDir()
	if err := export(&out, exported, vaultFlags(t, vault), global); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(exported, "location-entry.md")); string(data) != "- mine {{.Content}}\n" {
		t.Fatalf("exported location-entry.md = %q", data)
	}
}

// A template neither writer could use is something to fix by hand, said where
// bootstrap says what is left; one that renders, or a file that is not a
// template, is not.
func TestBootstrapChecksTheEntryTemplates(t *testing.T) {
	vault := freshVault(t)
	folder := filepath.Join(vault, filepath.FromSlash(config.DefaultPluginsObsidianEntries))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"daily-entry.md":     "- {{.Clock}} {{.Nope}}\n",
		"location-entry.md":  "- {{.Clock\n",
		"timeline-entry.md":  "- {{.Clock}} {{with .ContentLines}}{{index . 0}}{{end}} {{.MapLinks \"Apple\"}} {{.CopyLink}}\n",
		"daily-note.md":      "---\ndate: {{.Date}}\ncountry: \"{{mapped \"country\" .Country}}\"\n---\n",
		"notes.txt":          "{{ not a template",
		"broken-note-ish.md": "{{end}}",
	} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	todos, err := check(vault, pathsOf(config.Default()))
	if err != nil {
		t.Fatal(err)
	}
	fixes := map[string]bool{}
	for _, todo := range todos {
		if rest, ok := strings.CutPrefix(todo, "fix "+config.DefaultPluginsObsidianEntries+"/"); ok {
			fixes[strings.SplitN(rest, ":", 2)[0]] = true
		}
	}
	want := map[string]bool{"daily-entry.md": true, "location-entry.md": true, "broken-note-ish.md": true}
	if !reflect.DeepEqual(fixes, want) {
		t.Errorf("templates to fix = %v, want %v\n%s", fixes, want, strings.Join(todos, "\n"))
	}
}

// No entry template folder is no todo: the compiled-in templates stand in.
func TestBootstrapWithoutAnEntryTemplateFolder(t *testing.T) {
	todos, err := check(freshVault(t), pathsOf(config.Default()))
	if err != nil {
		t.Fatal(err)
	}
	for _, todo := range todos {
		if strings.HasPrefix(todo, "fix ") {
			t.Errorf("todo %q for a folder that is not there", todo)
		}
	}
}
