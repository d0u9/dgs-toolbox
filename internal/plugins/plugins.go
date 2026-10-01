// Package plugins installs the plugins dgs carries into the applications they
// extend, and says what is installed where. A plugin is a set of files that
// lands in one folder of its host application; which folders those are is the
// host's to say (see the obsidian package), and everything after that —
// comparing, writing, removing — is the same for every host and lives here.
//
// What is installed is told apart from what dgs carries by content, not by a
// version string: an install records the SHA-256 of every file it wrote, so a
// file changed by hand afterwards is seen as such and is not overwritten
// without being asked.
package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RecordFile is written beside a plugin's files by every install. It is what
// makes a folder one dgs manages.
const RecordFile = ".dgs-install.json"

// Plugin is one plugin dgs carries.
type Plugin struct {
	// Host is the application it extends: "obsidian".
	Host string
	// ID is its folder name in the host, and the id the host knows it by.
	ID string
	// Description says what it does, for a listing.
	Description string
	// Files builds what is installed, by path relative to the plugin's folder.
	// It fails when this dgs was built without part of the plugin.
	Files func() (map[string][]byte, error)
}

// Name is how a plugin is named on the command line: "obsidian/dgs-toolbox".
func (p Plugin) Name() string { return p.Host + "/" + p.ID }

// Target is one place a host keeps its plugins: an Obsidian vault's
// configuration folder, for example.
type Target struct {
	// Host is the application the place belongs to.
	Host string
	// Place names it for a reader.
	Place string
	// Dir is the folder each plugin gets a folder of its own in.
	Dir string
	// Enabled says whether the host has the plugin with this id switched on.
	// Nil when the host keeps no such record.
	Enabled func(id string) (bool, error)
}

// State is how what is installed compares with what dgs carries.
type State string

const (
	// Missing is nothing installed.
	Missing State = "missing"
	// Current is the same files dgs carries.
	Current State = "ok"
	// Outdated is what an earlier dgs installed, untouched since.
	Outdated State = "outdated"
	// Modified is an install whose files were changed or removed since.
	Modified State = "modified"
	// Unmanaged is a folder dgs did not install: there is no record.
	Unmanaged State = "unmanaged"
)

// Record is what an install writes into RecordFile.
type Record struct {
	Plugin string `json:"plugin"`
	// Version is the digest of every file installed; see Version.
	Version string `json:"version"`
	// By is the dgs that installed it, as `dgs --version` says.
	By          string            `json:"by"`
	InstalledAt time.Time         `json:"installedAt"`
	Files       map[string]string `json:"files"`
}

// Status is one plugin in one place.
type Status struct {
	Plugin Plugin
	Target Target
	State  State
	// Installed is the record found, when there is one.
	Installed *Record
	// Bundled is the version this dgs carries; empty when it cannot build the
	// plugin, and BundleErr says why.
	Bundled   string
	BundleErr error
	// Changed lists the files that differ from the record, for Modified.
	Changed []string
	// Enabled is whether the host has it switched on; nil when unknown.
	Enabled *bool
}

// Dir is the plugin's own folder in the target.
func Dir(target Target, plugin Plugin) string { return filepath.Join(target.Dir, plugin.ID) }

// Version is the digest of a set of files: the first 12 hex digits of the
// SHA-256 of every path and its content's SHA-256, in path order. Two installs
// of the same files have the same version, whichever dgs wrote them.
func Version(files map[string][]byte) string {
	digests := make(map[string]string, len(files))
	for path, data := range files {
		digests[path] = digest(data)
	}
	return versionOf(digests)
}

func versionOf(digests map[string]string) string {
	paths := make([]string, 0, len(digests))
	for path := range digests {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	sum := sha256.New()
	for _, path := range paths {
		fmt.Fprintf(sum, "%s\x00%s\n", path, digests[path])
	}
	return hex.EncodeToString(sum.Sum(nil))[:12]
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Inspect compares what is in the target with what dgs carries. It changes
// nothing.
func Inspect(target Target, plugin Plugin) (Status, error) {
	status := Status{Plugin: plugin, Target: target}
	if files, err := plugin.Files(); err != nil {
		status.BundleErr = err
	} else {
		status.Bundled = Version(files)
	}
	if target.Enabled != nil {
		enabled, err := target.Enabled(plugin.ID)
		if err != nil {
			return status, err
		}
		status.Enabled = &enabled
	}

	dir := Dir(target, plugin)
	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		status.State = Missing
		return status, nil
	case err != nil:
		return status, err
	case !info.IsDir():
		return status, fmt.Errorf("%s is a file, where the plugin's folder belongs", dir)
	}
	record, err := readRecord(dir)
	if errors.Is(err, fs.ErrNotExist) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return status, err
		}
		status.State = Missing
		for _, entry := range entries {
			if entry.Name() != "data.json" {
				status.State = Unmanaged
				break
			}
		}
		return status, nil
	}
	if err != nil {
		return status, err
	}
	status.Installed = record
	if status.Changed, err = changed(dir, record); err != nil {
		return status, err
	}
	switch {
	case len(status.Changed) > 0:
		status.State = Modified
	case status.Bundled != "" && record.Version != status.Bundled:
		status.State = Outdated
	default:
		status.State = Current
	}
	return status, nil
}

func readRecord(dir string) (*Record, error) {
	data, err := os.ReadFile(filepath.Join(dir, RecordFile))
	if err != nil {
		return nil, err
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, RecordFile), err)
	}
	return &record, nil
}

// changed lists the recorded files whose content is no longer what was written.
func changed(dir string, record *Record) ([]string, error) {
	var paths []string
	for path, want := range record.Files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if errors.Is(err, fs.ErrNotExist) {
			paths = append(paths, path)
			continue
		}
		if err != nil {
			return nil, err
		}
		if digest(data) != want {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// Install writes what dgs carries into the target. It refuses to overwrite a
// Modified or Unmanaged folder unless force is set, since that would lose
// what someone wrote there. Each file is written beside its final name and
// renamed over it; the record is written last, so an install cut short reads
// as Modified rather than as Current. Files an earlier install wrote that this
// one does not carry are removed. Anything else in the folder — the host's
// settings for the plugin, data.json in Obsidian — is left alone.
func Install(target Target, plugin Plugin, by string, force bool, now time.Time) (Status, error) {
	status, err := Inspect(target, plugin)
	if err != nil {
		return status, err
	}
	if status.BundleErr != nil {
		return status, status.BundleErr
	}
	if !force && (status.State == Modified || status.State == Unmanaged) {
		return status, fmt.Errorf("%s in %s is %s; --force replaces it", plugin.Name(), target.Place, describe(status))
	}
	if status.State == Current {
		return status, nil
	}
	files, err := plugin.Files()
	if err != nil {
		return status, err
	}
	dir := Dir(target, plugin)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return status, err
	}
	record := Record{Plugin: plugin.Name(), Version: Version(files), By: by, InstalledAt: now.UTC(), Files: map[string]string{}}
	for _, path := range sortedKeys(files) {
		if err := writeFile(filepath.Join(dir, filepath.FromSlash(path)), files[path]); err != nil {
			return status, err
		}
		record.Files[path] = digest(files[path])
	}
	if status.Installed != nil {
		for path := range status.Installed.Files {
			if _, kept := files[path]; !kept {
				if err := os.Remove(filepath.Join(dir, filepath.FromSlash(path))); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return status, err
				}
			}
		}
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return status, err
	}
	if err := writeFile(filepath.Join(dir, RecordFile), append(data, '\n')); err != nil {
		return status, err
	}
	return Inspect(target, plugin)
}

// Uninstall removes the files an install wrote, and its record. The folder
// goes too when nothing else is left in it; the host's settings for the
// plugin stay, so installing it again finds them. A Modified folder is left
// alone unless force is set; an Unmanaged one always is, since dgs does not
// know which of its files are the plugin's.
func Uninstall(target Target, plugin Plugin, force bool) (Status, error) {
	status, err := Inspect(target, plugin)
	if err != nil {
		return status, err
	}
	switch status.State {
	case Missing:
		return status, nil
	case Unmanaged:
		return status, fmt.Errorf("%s in %s was not installed by dgs; remove %s yourself", plugin.Name(), target.Place, Dir(target, plugin))
	case Modified:
		if !force {
			return status, fmt.Errorf("%s in %s is %s; --force removes it anyway", plugin.Name(), target.Place, describe(status))
		}
	}
	dir := Dir(target, plugin)
	for path := range status.Installed.Files {
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(path))); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return status, err
		}
	}
	if err := os.Remove(filepath.Join(dir, RecordFile)); err != nil {
		return status, err
	}
	removeEmpty(dir)
	return Inspect(target, plugin)
}

// removeEmpty removes dir and the folders under it that are left empty.
func removeEmpty(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			removeEmpty(filepath.Join(dir, entry.Name()))
		}
	}
	_ = os.Remove(dir) // fails, as it should, when something is left
}

func describe(status Status) string {
	if status.State == Modified {
		return "modified (" + strings.Join(status.Changed, ", ") + ")"
	}
	return string(status.State)
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	part := path + ".dgs-part"
	if err := os.WriteFile(part, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(part, path); err != nil {
		_ = os.Remove(part)
		return err
	}
	return nil
}

func sortedKeys(files map[string][]byte) []string {
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
