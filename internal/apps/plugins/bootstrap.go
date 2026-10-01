package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/plugins/bundled"
)

// configFolder is the Obsidian configuration folder bootstrap sets up and
// checks: the desktop's. Another, such as one chosen for a phone, is a
// trimmed copy its owner keeps by hand.
const configFolder = ".obsidian"

// bootstrap installs everything, writes the seeds a vault lacks, and checks
// the settings of the plugins the scripts and templates run in, which dgs
// does not write: Obsidian rewrites them from memory while it runs.
func bootstrap(out io.Writer, flags map[string]string, global config.Config) error {
	vaults, err := vaultsOf(flags, global)
	if err != nil {
		return err
	}
	paths := pathsOf(global)
	seeds, err := bundled.Seeds(paths)
	if err != nil {
		return err
	}
	from := flags["from"]
	if from != "" {
		if info, err := os.Stat(from); err != nil || !info.IsDir() {
			return fmt.Errorf("--from %s is not a folder", from)
		}
	}

	installErr := run(out, nil, flags, global, "install")
	var failures []error
	if installErr != nil {
		failures = append(failures, installErr)
	}
	for _, vault := range vaults {
		fmt.Fprintf(out, "\n%s\n", vault)
		for _, seed := range seeds {
			line, err := plant(vault, seed, from)
			if err != nil {
				failures = append(failures, err)
				continue
			}
			fmt.Fprintf(out, "  %-8s %s\n", line, seed.Path)
		}
		todos, err := check(vault, paths)
		if err != nil {
			failures = append(failures, err)
		}
		if len(todos) == 0 {
			fmt.Fprintln(out, "  nothing left to do by hand")
		}
		for _, todo := range todos {
			fmt.Fprintf(out, "  todo     %s\n", todo)
		}
	}
	return errors.Join(failures...)
}

// plant writes a seed where the vault has nothing, from the --from folder
// when it has the file, and says what it did.
func plant(vault string, seed bundled.Seed, from string) (string, error) {
	target := filepath.Join(vault, filepath.FromSlash(seed.Path))
	if _, err := os.Stat(target); err == nil {
		return "kept", nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	data, said := seed.Data, "written"
	if from != "" {
		exported, err := os.ReadFile(filepath.Join(from, seed.Name))
		switch {
		case err == nil:
			data, said = exported, "copied"
		case !errors.Is(err, fs.ErrNotExist):
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	return said, os.WriteFile(target, data, 0o644)
}

// export copies a vault's seeds, as they are, into dir under their names.
func export(out io.Writer, dir string, flags map[string]string, global config.Config) error {
	vaults, err := vaultsOf(flags, global)
	if err != nil {
		return err
	}
	if len(vaults) != 1 {
		return fmt.Errorf("export reads one vault and %d are named; choose one with --vault", len(vaults))
	}
	seeds, err := bundled.Seeds(pathsOf(global))
	if err != nil {
		return err
	}
	force := flags["force"] == "true"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, seed := range seeds {
		data, err := os.ReadFile(filepath.Join(vaults[0], filepath.FromSlash(seed.Path)))
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(out, "missing  %s (%s)\n", seed.Name, seed.Path)
			continue
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dir, seed.Name)
		if _, err := os.Stat(target); err == nil && !force {
			return fmt.Errorf("%s is already there; --force replaces it", target)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "copied   %s (%s)\n", seed.Name, seed.Path)
	}
	return nil
}

// check lists what the vault's own settings lack for the files dgs installs to
// work: the plugins they run in, switched on, and pointed at dgs's folders.
func check(vault string, paths bundled.Paths) ([]string, error) {
	configDir := filepath.Join(vault, configFolder)
	var todos []string
	enabled, err := readJSON[[]string](filepath.Join(configDir, "community-plugins.json"))
	if err != nil {
		return nil, err
	}
	for _, plugin := range []struct{ id, name string }{
		{"dgs-toolbox", "DGS Toolbox"}, {"templater-obsidian", "Templater"}, {"quickadd", "QuickAdd"},
	} {
		if _, err := os.Stat(filepath.Join(configDir, "plugins", plugin.id, "manifest.json")); err != nil {
			todos = append(todos, fmt.Sprintf("install %s from Obsidian's community plugins", plugin.name))
		} else if !slices.Contains(enabled, plugin.id) {
			todos = append(todos, fmt.Sprintf("switch %s on in Obsidian's community plugins", plugin.name))
		}
	}

	type folderTemplate struct {
		Folder   string `json:"folder"`
		Template string `json:"template"`
	}
	templater, err := readJSON[struct {
		UserScripts     string           `json:"user_scripts_folder"`
		Templates       string           `json:"templates_folder"`
		FolderTemplates []folderTemplate `json:"folder_templates"`
	}](filepath.Join(configDir, "plugins", "templater-obsidian", "data.json"))
	if err != nil {
		return nil, err
	}
	if templater.UserScripts != paths.Public {
		todos = append(todos, fmt.Sprintf("Templater: set Script files folder location to %s", paths.Public))
	}
	if templater.Templates == "" || !within(paths.Templates, templater.Templates) {
		todos = append(todos, fmt.Sprintf("Templater: set Template folder location to %s, or a folder holding it", paths.Templates))
	}
	daily := paths.Templates + "/Daily Log Template.md"
	dailyNotes, err := readJSON[struct {
		Template string `json:"template"`
	}](filepath.Join(configDir, "daily-notes.json"))
	if err != nil {
		return nil, err
	}
	used := strings.TrimSuffix(dailyNotes.Template, ".md") == strings.TrimSuffix(daily, ".md")
	for _, rule := range templater.FolderTemplates {
		used = used || rule.Template == daily
	}
	if !used {
		todos = append(todos, fmt.Sprintf("Templater: add a folder template giving the daily log folder %s", daily))
	}

	scripts, err := quickAddScripts(filepath.Join(configDir, "plugins", "quickadd", "data.json"))
	if err != nil {
		return nil, err
	}
	var names []string
	for _, plugin := range bundled.All(paths) {
		if plugin.ID != "quickadd" {
			continue
		}
		shipped, err := plugin.Files()
		if err != nil {
			return nil, err
		}
		for name := range shipped {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if want := paths.QuickAdd + "/" + name; !slices.Contains(scripts, want) {
			todos = append(todos, fmt.Sprintf("QuickAdd: add a macro choice running %s", want))
		}
	}
	for _, script := range scripts {
		if _, err := os.Stat(filepath.Join(vault, filepath.FromSlash(script))); errors.Is(err, fs.ErrNotExist) {
			todos = append(todos, fmt.Sprintf("QuickAdd: a choice runs %s, which is not there", script))
		}
	}
	return todos, nil
}

// within says whether folder is base or inside it.
func within(folder, base string) bool {
	base = path.Clean(base)
	return folder == base || strings.HasPrefix(folder, base+"/")
}

// quickAddScripts is the path of every user script a QuickAdd choice runs.
func quickAddScripts(file string) ([]string, error) {
	type choice struct {
		Choices []json.RawMessage `json:"choices"`
		Macro   *struct {
			Commands []struct {
				Type string `json:"type"`
				Path string `json:"path"`
			} `json:"commands"`
		} `json:"macro"`
	}
	data, err := readJSON[choice](file)
	if err != nil {
		return nil, err
	}
	var scripts []string
	var walk func([]json.RawMessage) error
	walk = func(raws []json.RawMessage) error {
		for _, raw := range raws {
			var c choice
			if err := json.Unmarshal(raw, &c); err != nil {
				return fmt.Errorf("%s: %w", file, err)
			}
			if c.Macro != nil {
				for _, command := range c.Macro.Commands {
					if command.Type == "UserScript" {
						scripts = append(scripts, command.Path)
					}
				}
			}
			if err := walk(c.Choices); err != nil {
				return err
			}
		}
		return nil
	}
	return scripts, walk(data.Choices)
}

// readJSON reads a settings file; one that is not there is its zero value,
// as it is to the plugin that writes it.
func readJSON[T any](file string) (T, error) {
	var value T
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return value, nil
	}
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, fmt.Errorf("%s: %w", file, err)
	}
	return value, nil
}
