package conf

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/cred/publish"
)

// exportFile is one target's rendered bytes, at the path they belong under
// within an export — see docs/apps/conf/export.md#what-is-written.
type exportFile struct {
	Path  string
	Bytes []byte
}

// CheckedInstances is every instance currently checked, sorted.
func (m Model) CheckedInstances() []string {
	var out []string
	for _, n := range m.nodes {
		for _, inst := range n.instances {
			if inst.broken == "" && m.checked[inst.name] {
				out = append(out, inst.name)
			}
		}
	}
	sort.Strings(out)
	return out
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
		kind, output := t.Service, ""
		if t.Export != "" {
			export, ok := m.l.exports[confgen.ExportKey(t.Service, t.Export)]
			if !ok {
				return nil, fmt.Errorf("%s: export %q is not defined", instance, t.Export)
			}
			kind, output = t.Service+"-"+t.Export, export.Output
		} else {
			manifest, ok := m.l.manifests[t.Service]
			if !ok {
				return nil, fmt.Errorf("%s: service %q is not defined", instance, t.Service)
			}
			output = manifest.Output
		}
		out, err := m.renderTarget(instance)
		if err != nil {
			return nil, err
		}
		path := filepath.Join(t.Node, kind, instance, output)
		files = append(files, exportFile{Path: path, Bytes: out})
	}
	return files, nil
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
		if err := writeExport(filepath.Join(destDir, f.Path), f.Bytes, overwrite); err != nil {
			return err
		}
	}
	return nil
}

// writeExport publishes one export file. Overwriting is publish.Replace: the
// same temporary file in the same directory, renamed over the old one, so a
// reader sees either the old file or the new one and never a partial file.
func writeExport(path string, data []byte, overwrite bool) error {
	if !overwrite {
		return publish.Create(path, data, 0o600, 0o700)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return publish.Replace(path, data, 0o600)
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
		w, err := zw.Create(filepath.ToSlash(f.Path))
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

	return writeExport(zipPath, buf.Bytes(), overwrite)
}
