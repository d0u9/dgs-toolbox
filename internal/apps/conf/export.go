package conf

import (
	"archive/zip"
	"bytes"
	"fmt"
	"path/filepath"
	"sort"

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
func (m Model) renderAll(instances []string) ([]exportFile, error) {
	files := make([]exportFile, 0, len(instances))
	for _, instance := range instances {
		t, err := m.findTarget(instance)
		if err != nil {
			return nil, err
		}
		role, ok := m.manifests[t.Service].Roles[t.Role]
		if !ok {
			return nil, fmt.Errorf("%s: service %q has no role %q", instance, t.Service, t.Role)
		}
		out, err := m.renderTarget(instance)
		if err != nil {
			return nil, err
		}
		path := filepath.Join(t.Node, t.Service, t.Role, instance, role.Output)
		files = append(files, exportFile{Path: path, Bytes: out})
	}
	return files, nil
}

// ExportFolder renders every checked instance and publishes each as its own
// file under destDir, each through publish.Create — checked, synced and
// linked to its final name, so destDir never holds a half-written result. It
// refuses if any target of destDir already exists.
func (m Model) ExportFolder(instances []string, destDir string) error {
	files, err := m.renderAll(instances)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := publish.Create(filepath.Join(destDir, f.Path), f.Bytes, 0o600, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// ExportZip renders every checked instance into one .zip archive, built
// entirely in memory before anything is written, and publishes it to
// zipPath through publish.Create.
func (m Model) ExportZip(instances []string, zipPath string) error {
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

	return publish.Create(zipPath, buf.Bytes(), 0o600, 0o700)
}
