// Package sidecar reads and writes the small JSON file kept beside a source
// GPX. It holds no track data, only what was done to the track — its cleaning
// and where it is cut into segments — so the source stays untouched and every
// change can be undone or re-run with other settings. Two things it holds are
// track data, since they cannot be found again: tracks added from other files
// until they are saved into a new GPX, and routes filled in along the road.
package sidecar

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"dgs-toolbox/internal/geo/clean"
	"dgs-toolbox/internal/geo/compose"
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
	// Fills are stretches filled in along the road; Added are tracks brought
	// from other files, not yet saved into a GPX.
	Fills []compose.Fill  `json:"fills,omitempty"`
	Added []compose.Added `json:"added,omitempty"`
	// Waypoints are standalone points added to a new GPX not yet on disk. A
	// file on disk written by this program takes a waypoint into the GPX
	// itself, so it holds none here.
	Waypoints []compose.Waypoint `json:"waypoints,omitempty"`
	// Names renames the parts of a file this program did not write, keyed as
	// the page keys them: "t0" for the first <trk>, "r0", "w0". A file dgs
	// wrote is renamed in the file itself, so it holds no names here.
	Names map[string]string `json:"names,omitempty"`
	// Plan is the route a GPX was written from, when it was planned by hand,
	// so it opens to be changed again.
	Plan *compose.Plan `json:"plan,omitempty"`
}

// Active reports whether the file records anything.
func (f File) Active() bool {
	return f.Clean.Active() || f.Segments.Active() || len(f.Fills) > 0 || len(f.Added) > 0 || len(f.Waypoints) > 0 || len(f.Names) > 0 || f.Plan != nil
}

// Insert shifts every point index after after by count, for count points
// inserted there.
func (f *File) Insert(after, count int) {
	f.remap(func(i int) (int, bool) {
		if i > after {
			return i + count, true
		}
		return i, true
	})
}

// Delete forgets the points first to last: what points at them only is
// dropped, and every later index moves down.
func (f *File) Delete(first, last int) {
	n := last - first + 1
	f.remap(func(i int) (int, bool) {
		switch {
		case i < first:
			return i, true
		case i > last:
			return i - n, true
		}
		return 0, false
	})
}

// remap moves every point index the file records. move says where an index
// goes, or false when its point is gone: a lasso loses that point, a range or
// stop shrinks to what is left of it, and a cut, a name or a fill whose end is
// gone goes too.
func (f *File) remap(move func(int) (int, bool)) {
	f.Clean = f.Clean.Normalize()
	edits := f.Clean.Edits[:0]
	for _, edit := range f.Clean.Edits {
		if edit.Kind == clean.EditLasso {
			points := []int{}
			for _, i := range edit.Points {
				if moved, ok := move(i); ok {
					points = append(points, moved)
				}
			}
			if len(points) > 0 {
				edit.Points = points
				edits = append(edits, edit)
			}
			continue
		}
		first, last, kept := -1, -1, false
		for i := edit.First; i <= edit.Last; i++ {
			if moved, ok := move(i); ok {
				if !kept {
					first, kept = moved, true
				}
				last = moved
			}
		}
		if kept {
			edit.First, edit.Last = first, last
			edits = append(edits, edit)
		}
	}
	f.Clean.Edits = edits
	if len(edits) == 0 {
		f.Clean.Edits = nil
	}

	var cuts []int
	for _, cut := range f.Segments.Cuts {
		if moved, ok := move(cut); ok {
			cuts = append(cuts, moved)
		}
	}
	f.Segments.Cuts = cuts
	var names []segment.Name
	for _, name := range f.Segments.Names {
		if moved, ok := move(name.Start); ok {
			name.Start = moved
			names = append(names, name)
		}
	}
	f.Segments.Names = names

	var fills []compose.Fill
	for _, fill := range f.Fills {
		first, okFirst := move(fill.First)
		last, okLast := move(fill.Last)
		// A fill keeps its route only while every point of it is still there.
		inserted := true
		for i := fill.First + 1; i <= fill.First+len(fill.Route); i++ {
			if moved, ok := move(i); !ok || moved != first+i-fill.First {
				inserted = false
			}
		}
		if okFirst && okLast && inserted {
			fill.First, fill.Last = first, last
			fills = append(fills, fill)
		}
	}
	f.Fills = fills
}

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
