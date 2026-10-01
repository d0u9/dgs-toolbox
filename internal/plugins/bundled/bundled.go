// Package bundled holds the plugins dgs carries, compiled into the binary so
// that dgs alone can install them.
//
// A plugin's sources live in <host>/<id>/. What the build generates beside
// them — the timeline engine compiled to WebAssembly and Go's JavaScript glue
// for it — goes into <host>/<id>/generated/, which `make plugins` writes and
// git ignores. A dgs built without running it still builds, and says so when
// asked to install.
package bundled

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"

	"dgs-toolbox/internal/plugins"
	"dgs-toolbox/internal/plugins/obsidian"
)

//go:embed obsidian
var sources embed.FS

// All is every plugin dgs carries, in name order.
func All() []plugins.Plugin {
	return []plugins.Plugin{obsidianToolbox()}
}

// obsidianToolbox is the Obsidian plugin. Obsidian loads one script, so its
// main.js is assembled from three: Go's wasm_exec.js, which defines the Go
// runtime the engine needs, then engine.js, then the plugin itself.
func obsidianToolbox() plugins.Plugin {
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
				"main.js":       bytes.Join(parts, []byte("\n")),
				"timeline.wasm": wasm,
			}, nil
		},
	}
}
