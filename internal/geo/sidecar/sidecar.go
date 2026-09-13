// Package sidecar reads and writes the small JSON file kept beside a source
// GPX. It holds no track data, only what was done to the track — its cleaning
// and where it is cut into segments — so the source stays untouched and every
// change can be undone or re-run with other settings.
package sidecar

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"dgs-toolbox/internal/geo/clean"
	"dgs-toolbox/internal/geo/segment"
)

// Suffix is appended to the source's file name: walk.gpx has walk.gpx.dgs.json.
const Suffix = ".dgs.json"

// Version is the format this package writes.
const Version = 1

// File is a sidecar's content.
type File struct {
	Version  int            `json:"version"`
	Clean    clean.Params   `json:"clean"`
	Segments segment.Params `json:"segments"`
}

// Active reports whether the file records anything.
func (f File) Active() bool { return f.Clean.Active() || f.Segments.Active() }

// PathFor is the sidecar of a source file.
func PathFor(source string) string { return source + Suffix }

// Load reads the sidecar of source. Without one it returns the defaults and
// false. Settings missing from the file keep their defaults, so a sidecar
// written before a setting existed still loads.
func Load(source string) (File, bool, error) {
	file := File{Version: Version, Clean: clean.Defaults()}
	data, err := os.ReadFile(PathFor(source))
	if errors.Is(err, os.ErrNotExist) {
		return file, false, nil
	}
	if err != nil {
		return file, false, err
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return File{Version: Version, Clean: clean.Defaults()}, false, fmt.Errorf("%s: %w", PathFor(source), err)
	}
	if file.Version > Version {
		return File{Version: Version, Clean: clean.Defaults()}, false, fmt.Errorf("%s: version %d is newer than this build reads", PathFor(source), file.Version)
	}
	file.Clean = file.Clean.Normalize()
	return file, true, nil
}

// Save writes the sidecar of source, replacing it in one step so a crash never
// leaves half a file. A sidecar that records nothing is removed instead: undoing
// every change leaves no trace beside the source.
func Save(source string, file File) error {
	path := PathFor(source)
	if !file.Active() {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	file.Version = Version
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
