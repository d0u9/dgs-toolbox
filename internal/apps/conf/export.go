package conf

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/cred/publish"
)

// exportFile is one target's rendered bytes, at the path they belong under
// within an export — see docs/apps/conf/export.md#what-is-written.
type exportFile struct {
	Path  string
	Bytes []byte
	// Executable is set for a file whose name says it is run rather than
	// read — a deployment's install script. It is derived from the output
	// name, so a template author writes no mode.
	Executable bool
}

// fileMode is the mode a rendered file is published under. A rendered
// configuration carries credentials, so 0600 stays the rule; a script
// carries none and is written to be run, so it gets the owner's execute bit
// and nothing more.
func (f exportFile) fileMode() os.FileMode {
	if f.Executable {
		return 0o700
	}
	return 0o600
}

// renderAll renders every instance, in order, stopping at the first failure:
// an export renders every target first and publishes only once all of them
// have rendered, so a half-written result is not among the outcomes.
func (m renderer) renderAll(instances []string) ([]exportFile, error) {
	files := make([]exportFile, 0, len(instances))
	for _, instance := range instances {
		t, err := m.findTarget(instance)
		if err != nil {
			return nil, err
		}
		// A deployment is filed under the service it runs; a file written
		// for a person, under the service and the way it was written, since
		// an export's name is unique only within its service and two of them
		// would otherwise share a directory. Both sit under the node or the
		// person the bundle is for.
		kind := t.Service
		if t.Export != "" {
			kind = t.Service + "-" + t.Export
		}
		rendered, err := m.renderTarget(instance)
		if err != nil {
			return nil, err
		}
		for _, a := range rendered {
			bytes := a.Bytes
			if strings.EqualFold(filepath.Ext(a.Output), ".json") {
				bytes = indentJSON(bytes)
			}
			files = append(files, exportFile{
				Path:       filepath.Join(t.Node, kind, instance, a.Output),
				Bytes:      bytes,
				Executable: a.Executable,
			})
		}

		// A containerised instance of a service that declares a deploy/
		// writes a second file beside the first: what starts the program,
		// next to how the program behaves. Both are rendered from one
		// `ports` field, which is what makes the port mapping in it derived
		// rather than maintained by hand.
		deploy, err := m.deployFor(instance)
		if err != nil {
			return nil, err
		}
		for _, d := range deploy {
			files = append(files, exportFile{
				Path:       filepath.Join(t.Node, kind, instance, d.Output),
				Bytes:      d.Bytes,
				Executable: d.Executable,
			})
		}
	}
	return files, nil
}

// indentJSON lays out a rendered .json file two spaces per level, keeping
// the keys in the order the template wrote them. A template renders JSON as
// it is easiest to write, not to read, and an export is read by a person
// before it is pasted anywhere. Output that does not parse is left as it is:
// the file is still what the template produced, and the server reading it
// reports the error.
func indentJSON(data []byte) []byte {
	var buf bytes.Buffer
	if err := json.Indent(&buf, bytes.TrimSpace(data), "", "  "); err != nil {
		return data
	}
	buf.WriteByte('\n')
	return buf.Bytes()
}

// ExportFolder renders every checked instance and publishes each as its own
// file under destDir, each through publish.Create — checked, synced and
// linked to its final name, so destDir never holds a half-written result. It
// refuses if any target of destDir already exists, unless overwrite says the
// caller asked for those files to be replaced.
func (m renderer) ExportFolder(instances []string, destDir string, overwrite bool) error {
	files, err := m.renderAll(instances)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := writeExport(filepath.Join(destDir, f.Path), f.Bytes, f.fileMode(), overwrite); err != nil {
			return err
		}
	}
	return nil
}

// writeExport publishes one export file. Overwriting is publish.Replace: the
// same temporary file in the same directory, renamed over the old one, so a
// reader sees either the old file or the new one and never a partial file.
func writeExport(path string, data []byte, mode os.FileMode, overwrite bool) error {
	if !overwrite {
		return publish.Create(path, data, mode, 0o700)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return publish.Replace(path, data, mode)
}

// existingOf is every path of a rendered export that is already on disk,
// relative to destDir and in the order the export writes them. It is what a
// confirmation needs before it can ask about overwriting.
func existingOf(files []exportFile, destDir string) []string {
	var existing []string
	for _, f := range files {
		if _, err := os.Lstat(filepath.Join(destDir, f.Path)); err == nil {
			existing = append(existing, f.Path)
		}
	}
	return existing
}

// ExportZip renders every checked instance into one .zip archive, built
// entirely in memory before anything is written, and publishes it to
// zipPath through publish.Create, or over an existing archive when overwrite
// says the caller asked for that.
func (m renderer) ExportZip(instances []string, zipPath string, overwrite bool) error {
	files, err := m.renderAll(instances)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		// The header rather than Create, so the execute bit survives
		// the archive: a script extracted without it is a script the
		// person has to chmod before the deployment it belongs to runs.
		header := &zip.FileHeader{Name: filepath.ToSlash(f.Path), Method: zip.Deflate}
		header.SetMode(f.fileMode())
		w, err := zw.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("zip: %s: %w", f.Path, err)
		}
		if _, err := w.Write(f.Bytes); err != nil {
			return fmt.Errorf("zip: %s: %w", f.Path, err)
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("zip: %w", err)
	}

	return writeExport(zipPath, buf.Bytes(), 0o600, overwrite)
}
