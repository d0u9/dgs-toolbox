package conf

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"dgs-toolbox/internal/conf/secretstore"
	"dgs-toolbox/internal/conf/target"
)

// compareMigrationTargets uses the export renderer on each side. A failed
// render is reported as unknown, never mistaken for an unchanged target.
func compareMigrationTargets(out io.Writer, before, after loaded, root, secrets, oldID, newID string) {
	oldTargets := map[string]target.Target{}
	newTargets := map[string]target.Target{}
	for _, t := range target.List(before.inv, before.derived) {
		oldTargets[migrationTargetKey(t, oldID, newID)] = t
	}
	for _, t := range target.List(after.inv, after.derived) {
		newTargets[migrationTargetKey(t, newID, newID)] = t
	}
	keys := make([]string, 0, len(oldTargets)+len(newTargets))
	seen := map[string]bool{}
	for key := range oldTargets {
		keys = append(keys, key)
		seen[key] = true
	}
	for key := range newTargets {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	oldRenderer := renderer{l: before, rootPath: root, secretsDir: secrets}
	newRenderer := renderer{l: after, rootPath: root, secretsDir: secrets}
	secretProblem := missingMigrationSecret(before, after, secrets)
	lines := []string{}
	work := []string{}
	for _, key := range keys {
		old, hadOld := oldTargets[key]
		newTarget, hasNew := newTargets[key]
		owner := newTarget.Node
		if owner == "" {
			owner = newTarget.User
		}
		if !hasNew {
			owner = old.Node
			if owner == "" {
				owner = old.User
			}
		}
		label := newTarget.Service + "/" + newTarget.Instance
		if !hasNew {
			label = old.Service + "/" + old.Instance
		}
		if secretProblem != "" {
			lines = append(lines, fmt.Sprintf("  %s: %s: render not compared (%s)", owner, label, secretProblem))
			work = append(work, fmt.Sprintf("  %s: review %s (render not compared)", owner, label))
			continue
		}
		var oldFiles, newFiles []exportFile
		var oldErr, newErr error
		if hadOld {
			oldFiles, oldErr = oldRenderer.renderAll([]string{old.Instance})
		}
		if hasNew {
			newFiles, newErr = newRenderer.renderAll([]string{newTarget.Instance})
		}
		if oldErr != nil || newErr != nil {
			lines = append(lines, fmt.Sprintf("  %s: %s: render not compared (before: %s; after: %s)", owner, label, renderState(oldErr, hadOld), renderState(newErr, hasNew)))
			work = append(work, fmt.Sprintf("  %s: review %s (render not compared)", owner, label))
			continue
		}
		if !hadOld || !hasNew {
			files, action := newFiles, "add"
			if !hasNew {
				files, action = oldFiles, "remove"
			}
			for _, file := range files {
				lines = append(lines, fmt.Sprintf("  %s: %s: %s %s", owner, label, action, file.Path))
			}
			work = append(work, migrationWorkItem(owner, label, newTarget.Export, hasNew))
			continue
		}
		changes := compareExportFiles(oldFiles, newFiles)
		for _, change := range changes {
			lines = append(lines, fmt.Sprintf("  %s: %s: %s", owner, label, change))
		}
		if len(changes) != 0 {
			work = append(work, migrationWorkItem(owner, label, newTarget.Export, true))
		}
	}
	fmt.Fprintln(out, "Service and export impact:")
	if len(lines) == 0 {
		fmt.Fprintln(out, "  none (all targets rendered identically)")
	} else {
		for _, line := range lines {
			fmt.Fprintln(out, line)
		}
	}
	fmt.Fprintln(out, "Deployment and handoff list:")
	if len(work) == 0 {
		fmt.Fprintln(out, "  none")
	} else {
		for _, item := range work {
			fmt.Fprintln(out, item)
		}
	}
}

func migrationWorkItem(owner, label, export string, present bool) string {
	if !present {
		return fmt.Sprintf("  %s: retire old export for %s after cutover", owner, label)
	}
	if export != "" {
		return fmt.Sprintf("  %s: regenerate and hand off %s", owner, label)
	}
	return fmt.Sprintf("  %s: export, install and restart/reload %s", owner, label)
}

func missingMigrationSecret(before, after loaded, secrets string) string {
	paths := append(secretstore.ImpliedPaths(before.inv, before.manifests, before.derived), secretstore.ImpliedPaths(after.inv, after.manifests, after.derived)...)
	if len(paths) == 0 {
		return ""
	}
	if secrets == "" {
		return "conf.secrets is not configured"
	}
	return ""
}

func migrationTargetKey(t target.Target, oldID, newID string) string {
	owner := t.Node
	if owner == "" {
		owner = t.User
	}
	if owner == oldID {
		owner = newID
	}
	instance := t.Instance
	if oldID != newID && strings.HasPrefix(instance, oldID+"-") {
		instance = newID + strings.TrimPrefix(instance, oldID)
	}
	return owner + "/" + t.Service + "/" + t.Export + "/" + instance
}

func renderState(err error, exists bool) string {
	if !exists {
		return "absent"
	}
	if err != nil {
		return err.Error()
	}
	return "rendered"
}

func compareExportFiles(oldFiles, newFiles []exportFile) []string {
	oldByName := map[string]exportFile{}
	newByName := map[string]exportFile{}
	for _, file := range oldFiles {
		oldByName[exportOutputName(file.Path)] = file
	}
	for _, file := range newFiles {
		newByName[exportOutputName(file.Path)] = file
	}
	keys := make([]string, 0, len(oldByName)+len(newByName))
	seen := map[string]bool{}
	for key := range oldByName {
		keys = append(keys, key)
		seen[key] = true
	}
	for key := range newByName {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var changes []string
	for _, key := range keys {
		old, hadOld := oldByName[key]
		newFile, hasNew := newByName[key]
		switch {
		case !hadOld:
			changes = append(changes, "add "+newFile.Path)
		case !hasNew:
			changes = append(changes, "remove "+old.Path)
		default:
			if old.Path != newFile.Path {
				state := "content unchanged"
				if !bytes.Equal(old.Bytes, newFile.Bytes) || old.fileMode() != newFile.fileMode() {
					state = "content or mode also changed"
				}
				changes = append(changes, "export path "+old.Path+" -> "+newFile.Path+" ("+state+")")
			}
			if old.Path == newFile.Path && (!bytes.Equal(old.Bytes, newFile.Bytes) || old.fileMode() != newFile.fileMode()) {
				changes = append(changes, "update "+newFile.Path+" (content or mode changed)")
			}
		}
	}
	return changes
}

func exportOutputName(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) <= 3 {
		return path
	}
	return strings.Join(parts[3:], "/")
}

func compareMigrationSecrets(out io.Writer, before, after loaded) {
	oldPaths := secretstore.ImpliedPaths(before.inv, before.manifests, before.derived)
	newPaths := secretstore.ImpliedPaths(after.inv, after.manifests, after.derived)
	oldSet, newSet := map[string]bool{}, map[string]bool{}
	for _, path := range oldPaths {
		oldSet[path.String()] = true
	}
	for _, path := range newPaths {
		newSet[path.String()] = true
	}
	var changes []string
	for path := range oldSet {
		if !newSet[path] {
			changes = append(changes, "  removed: "+path)
		}
	}
	for path := range newSet {
		if !oldSet[path] {
			changes = append(changes, "  added: "+path)
		}
	}
	sort.Strings(changes)
	fmt.Fprintln(out, "Implied secret paths:")
	if len(changes) == 0 {
		fmt.Fprintln(out, "  unchanged")
	} else {
		for _, change := range changes {
			fmt.Fprintln(out, change)
		}
	}
}
