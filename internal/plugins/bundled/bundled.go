// Package bundled holds the plugins dgs carries, compiled into the binary so
// that dgs alone can install them.
//
// A plugin's sources live in <host>/<id>/. What the build generates beside
// them — the timeline engine compiled to WebAssembly and Go's JavaScript glue
// for it — goes into <host>/<id>/generated/, which `make plugins` writes and
// git ignores. A dgs built without running it still builds, and says so when
// asked to install.
//
// The scripts and templates name the vault folders they are installed into —
// a QuickAdd script loads the Public scripts from their folder — and those
// folders are the reader's to choose. So a text file says {{dgs:public}},
// {{dgs:quickadd}}, {{dgs:templates}} or {{dgs:timelines}} where it means one,
// and install writes the configured path in its place.
package bundled

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"dgs-toolbox/internal/plugins"
	"dgs-toolbox/internal/plugins/obsidian"
)

//go:embed obsidian
var sources embed.FS

// Paths are the vault paths the Obsidian files are installed into, relative
// to the vault: plugins/config.json's obsidian.folders and obsidian.timelines.
type Paths struct {
	Public, QuickAdd, Templates, Timelines string
}

func (p Paths) replacer() *strings.Replacer {
	return strings.NewReplacer(
		"{{dgs:public}}", p.Public,
		"{{dgs:quickadd}}", p.QuickAdd,
		"{{dgs:templates}}", p.Templates,
		"{{dgs:timelines}}", p.Timelines,
	)
}

// All is every plugin dgs carries, in name order.
func All(paths Paths) []plugins.Plugin {
	return []plugins.Plugin{
		obsidianToolbox(paths),
		folder(paths, "public", paths.Public, "the scripts Templater, QuickAdd and dgs-toolbox share: location, weather, the daily log"),
		folder(paths, "quickadd", paths.QuickAdd, "the QuickAdd scripts: quick note, idea, copy coordinates"),
		folder(paths, "templates", paths.Templates, "the Templater templates: the daily log, and inserting and updating location, weather and dates"),
	}
}

// obsidianToolbox is the Obsidian plugin. Obsidian loads one script, so its
// main.js is assembled from three: Go's wasm_exec.js, which defines the Go
// runtime the engine needs, then engine.js, then the plugin itself.
func obsidianToolbox(paths Paths) plugins.Plugin {
	const dir = "obsidian/dgs-toolbox"
	return plugins.Plugin{
		Host:        obsidian.Host,
		ID:          "dgs-toolbox",
		Description: "timelines, written by the same code as dgs capture",
		Files: func() (map[string][]byte, error) {
			read := func(name string) ([]byte, error) {
				data, err := fs.ReadFile(sources, dir+"/"+name)
				if err != nil {
					return nil, fmt.Errorf("this dgs was built without %s: build it with make install, which runs make plugins first", name)
				}
				return data, nil
			}
			var parts [][]byte
			for _, name := range []string{"generated/wasm_exec.js", "engine.js", "main.js"} {
				data, err := read(name)
				if err != nil {
					return nil, err
				}
				parts = append(parts, data)
			}
			manifest, err := read("manifest.json")
			if err != nil {
				return nil, err
			}
			wasm, err := read("generated/timeline.wasm")
			if err != nil {
				return nil, err
			}
			styles, err := read("styles.css")
			if err != nil {
				return nil, err
			}
			return map[string][]byte{
				"manifest.json": manifest,
				"styles.css":    styles,
				"main.js":       []byte(paths.replacer().Replace(string(bytes.Join(parts, []byte("\n"))))),
				"timeline.wasm": wasm,
			}, nil
		},
	}
}

// folder is the files under obsidian/<id>/, installed into a folder of the
// vault.
func folder(paths Paths, id, into, description string) plugins.Plugin {
	return plugins.Plugin{
		Host:        obsidian.Host,
		ID:          id,
		Kind:        plugins.KindFiles,
		Folder:      into,
		Description: description,
		Files: func() (map[string][]byte, error) {
			root := "obsidian/" + id
			files := map[string][]byte{}
			err := fs.WalkDir(sources, root, func(name string, entry fs.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				data, err := fs.ReadFile(sources, name)
				if err != nil {
					return err
				}
				if text(name) {
					data = []byte(paths.replacer().Replace(string(data)))
				}
				files[strings.TrimPrefix(name, root+"/")] = data
				return nil
			})
			return files, err
		},
	}
}

func text(name string) bool {
	switch path.Ext(name) {
	case ".js", ".md", ".json", ".css":
		return true
	}
	return false
}

// Seed is a starting file bootstrap writes once, when there is none, for the
// reader to make their own: a vault path and its content.
type Seed struct {
	// Name is how export and --from name it.
	Name string
	// Path is where it goes in the vault.
	Path string
	// Description says what it is, for a listing.
	Description string
	Data        []byte
}

// Seeds are the starting files of an Obsidian vault.
func Seeds(paths Paths) ([]Seed, error) {
	timelines, err := fs.ReadFile(sources, "obsidian/seeds/timelines.json")
	if err != nil {
		return nil, err
	}
	settings := fmt.Sprintf("{\n  \"timelinesFile\": %q\n}\n", paths.Timelines)
	return []Seed{
		{Name: "timelines.json", Path: paths.Timelines, Description: "the timelines the plugin and dgs capture write to", Data: timelines},
		{Name: "dgs-toolbox.json", Path: ".obsidian/plugins/dgs-toolbox/data.json", Description: "the dgs-toolbox plugin's settings", Data: []byte(settings)},
	}, nil
}
