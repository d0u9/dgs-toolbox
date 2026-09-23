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
	// Waypoints are standalone points added to a GPX this program wrote, or
	// to a new one not yet on disk, and not yet written into it.
	Waypoints []compose.Waypoint `json:"waypoints,omitempty"`
	// Routes are routes copied from other files, not yet written into the GPX.
	Routes []compose.Route `json:"routes,omitempty"`
	// Deleted are the parts of the GPX itself that are taken out, keyed as the
	// page keys them — "t0", "r0", "w0" — counting the file on disk. Only a
	// GPX this program wrote is deleted from: the parts go out of the file
	// when the edit is saved. A recording is never written, so it holds none.
	Deleted []string `json:"deleted,omitempty"`
	// Moved are waypoints put somewhere else, keyed as Deleted is, each
	// [lon, lat] in WGS-84. Only a GPX this program wrote holds any.
	Moved map[string][2]float64 `json:"moved,omitempty"`
	// Names renames the parts of a file this program did not write, keyed as
	// the page keys them: "t0" for the first <trk>, "r0", "w0". A file dgs
	// wrote is renamed in the file itself, so it holds no names here.
	Names map[string]string `json:"names,omitempty"`
	// Order puts the parts of one kind in another order than the file has
	// them, keyed "r" for routes and "w" for waypoints, each holding every
	// key of that kind — "w2", "w0", "w1" — in the order wanted. Tracks keep
	// the file's order: the edits on their points are kept by point index,
	// which the file's own order gives. The order is taken into the GPX when
	// the file is saved, so only a GPX this program wrote holds one.
	Order map[string][]string `json:"order,omitempty"`
	// Plan is the route a GPX was written from, when it was planned by hand,
	// so it opens to be changed again.
	Plan *compose.Plan `json:"plan,omitempty"`
}

// Active reports whether the file records anything.
func (f File) Active() bool {
	return f.Clean.Active() || f.Segments.Active() || len(f.Fills) > 0 || len(f.Added) > 0 || len(f.Waypoints) > 0 ||
		len(f.Routes) > 0 || len(f.Deleted) > 0 || len(f.Moved) > 0 || len(f.Names) > 0 || len(f.Order) > 0 || f.Plan != nil
}

// Insert shifts every point index after after by count, for count points
// inserted there.
func (f *File) Insert(after, count int) {
	f.remap(func(i int) (int, bool) {
		if i > after {
			return i + count, true
		}
		return i, true
	}, false)
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
	}, false)
}

// Reorder puts runs of points in another order, for tracks put in another
// order in their file. runs are the runs' first and last indices as they are
// now, listed in the order they go in; together they are to cover the points
// from the first run's start, which they are laid from. An index in no run
// keeps its place. A range that spanned two runs splits where they part, and
// a fill whose route no longer lies between its ends goes.
func (f *File) Reorder(runs [][2]int) {
	if len(runs) == 0 {
		return
	}
	start := runs[0][0]
	for _, run := range runs {
		start = min(start, run[0])
	}
	type moved struct{ first, last, to int }
	var at []moved
	next := start
	for _, run := range runs {
		at = append(at, moved{run[0], run[1], next})
		next += run[1] - run[0] + 1
	}
	f.remap(func(i int) (int, bool) {
		for _, run := range at {
			if i >= run.first && i <= run.last {
				return run.to + i - run.first, true
			}
		}
		return i, true
	}, true)
}

// remap moves every point index the file records. move says where an index
// goes, or false when its point is gone: a lasso loses that point, a range or
// stop shrinks to what is left of it, and a cut, a name or a fill whose end is
// gone goes too. split parts a range whose points no longer follow one
// another; without it a range spans from its first point to its last.
func (f *File) remap(move func(int) (int, bool), split bool) {
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
		// What is left of the range. Inserted points inside it join it; with
		// split, points it spanned that were put apart part it into runs.
		first, last, kept := -1, -1, false
		for i := edit.First; i <= edit.Last; i++ {
			moved, ok := move(i)
			if !ok {
				continue
			}
			if split && kept && moved != last+1 {
				run := edit
				run.First, run.Last = first, last
				edits = append(edits, run)
				kept = false
			}
			if !kept {
				first, kept = moved, true
			}
			last = moved
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
