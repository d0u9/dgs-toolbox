package gpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/segment"
	"dgs-toolbox/internal/geo/stops"
)

// segmentJSON is one segment of a track as the page lists it.
type segmentJSON struct {
	First    int     `json:"first"` // source point indices
	Last     int     `json:"last"`
	Name     string  `json:"name"` // as given; empty for the page to name
	Points   int     `json:"points"`
	Distance float64 `json:"distance"` // metres
	Start    *int64  `json:"start"`    // Unix milliseconds of the first timed point
	End      *int64  `json:"end"`
}

// segments divides what cleaning kept at the saved cuts. Cuts are kept as
// source indices; a cut on a point cleaning removed moves to the next kept one.
func segments(a analysis) ([]segmentJSON, []int, []int) {
	n := len(a.line)
	kept := make([]int, 0, len(a.segmentParams().Cuts))
	for _, cut := range a.segmentParams().Cuts {
		kept = append(kept, a.keptAt(cut))
	}
	ranges := segment.Split(n, kept)
	list := make([]segmentJSON, len(ranges))
	cuts := []int{}
	for i, r := range ranges {
		first, last := a.index[r.First], a.index[r.Last]
		s := segmentJSON{
			First:    first,
			Last:     last,
			Name:     segment.NameOf(a.segmentParams().Names, first),
			Points:   r.Last - r.First + 1,
			Distance: a.distances[r.Last] - a.distances[r.First],
		}
		for k := r.First; k <= r.Last; k++ {
			if t := a.line[k].Time; !t.IsZero() {
				millis := t.UnixMilli()
				if s.Start == nil {
					s.Start = &millis
				}
				end := millis
				s.End = &end
			}
		}
		if i > 0 {
			cuts = append(cuts, first)
		}
		list[i] = s
	}
	candidates := []int{}
	for _, cut := range segment.Candidates(a.stops) {
		candidates = append(candidates, a.index[cut])
	}
	return list, cuts, candidates
}

func (a analysis) segmentParams() segment.Params { return a.cleaning.Segments }

// keptAt is the index in the kept line of the first kept point at or after a
// source index, or the last kept point.
func (a analysis) keptAt(source int) int {
	k := sort.SearchInts(a.index, source)
	if k >= len(a.index) {
		k = len(a.index) - 1
	}
	return k
}

// saveSegments records a track's cuts and segment names in its sidecar.
func (a api) saveSegments(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path  string         `json:"path"`
		Cuts  []int          `json:"cuts"`
		Names []segment.Name `json:"names"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	if err := checkGPX(body.Path); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	file, _, err := loadSidecar(body.Path)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	names := []segment.Name{}
	for _, name := range body.Names {
		if name.Name = strings.TrimSpace(name.Name); name.Name != "" {
			names = append(names, name)
		}
	}
	file.Segments = segment.Params{Cuts: segment.Clean(body.Cuts, 1<<31-1), Names: names}
	if len(file.Segments.Cuts) == 0 {
		file.Segments.Cuts = nil
	}
	if len(file.Segments.Names) == 0 {
		file.Segments.Names = nil
	}
	if err := saveSidecar(body.Path, file); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// writeSegments writes chosen segments of a track, as they are after
// cleaning, one <trk> each: added to another GPX file — recorded in its
// sidecar until that file is saved as a new one — or into a new file. No
// existing GPX is written.
func (a api) writeSegments(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path     string `json:"path"`
		Segments []struct {
			First int    `json:"first"`
			Last  int    `json:"last"`
			Name  string `json:"name"`
		} `json:"segments"`
		Target struct {
			Mode string `json:"mode"` // "add" or "create"
			Path string `json:"path"`
		} `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	if len(body.Segments) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("choose at least one segment"))
		return
	}
	target, err := filepath.Abs(body.Target.Path)
	if isDraft(body.Target.Path) && body.Target.Mode == "add" {
		target = body.Target.Path
	}
	if err != nil || !strings.EqualFold(filepath.Ext(target), ".gpx") {
		writeError(w, http.StatusBadRequest, errors.New("the target must be a .gpx file"))
		return
	}
	source := body.Path
	if !isDraft(source) {
		source, _ = filepath.Abs(body.Path)
	}
	if sameFile(source, target) {
		writeError(w, http.StatusBadRequest, errors.New("the source GPX is never written; choose another file"))
		return
	}
	result, err := analyse(body.Path, stops.Defaults()) // stops do not change the segments written
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	list, _, _ := segments(result)
	var tracks []gpxfile.Track
	for _, wanted := range body.Segments {
		found := false
		for _, s := range list {
			if s.First == wanted.First && s.Last == wanted.Last {
				found = true
			}
		}
		if !found {
			writeError(w, http.StatusConflict, fmt.Errorf("segment %d–%d is not one of the track's segments; reload the page", wanted.First, wanted.Last))
			return
		}
		tracks = append(tracks, result.trackOf(wanted.First, wanted.Last, strings.TrimSpace(wanted.Name)))
	}
	switch body.Target.Mode {
	case "add":
		err = addTracks(target, source, tracks)
	case "create":
		if _, statErr := os.Stat(filepath.Dir(target)); statErr != nil {
			writeError(w, statusFor(statErr), fmt.Errorf("folder %s: %w", filepath.Dir(target), statErr))
			return
		}
		err = gpxfile.Create(target, strings.TrimSuffix(filepath.Base(target), filepath.Ext(target)), tracks)
	default:
		writeError(w, http.StatusBadRequest, errors.New(`target mode must be "add" or "create"`))
		return
	}
	switch {
	case errors.Is(err, gpxfile.ErrExists):
		writeError(w, http.StatusConflict, err)
	case err != nil:
		writeError(w, statusFor(err), err)
	default:
		writeJSON(w, map[string]any{"ok": true, "path": target, "tracks": len(tracks)})
	}
}

// trackOf is the kept points of source points first to last as a GPX track,
// at their cleaned positions, a <trkseg> per recorded segment. Filled points
// keep their <src>.
func (a analysis) trackOf(first, last int, name string) gpxfile.Track {
	trk := gpxfile.Track{Name: name}
	current := -1
	for k := a.keptAt(first); k < len(a.line) && a.index[k] <= last; k++ {
		sample := a.line[k]
		if sample.Segment != current {
			trk.Segments = append(trk.Segments, gpxfile.Segment{})
			current = sample.Segment
		}
		seg := &trk.Segments[len(trk.Segments)-1]
		seg.Points = append(seg.Points, gpxfile.Point{
			LatLon:        sample.LatLon,
			Elevation:     sample.Elevation,
			HasElevation:  sample.HasElevation,
			Time:          sample.Time,
			HDOP:          sample.HDOP,
			HasHDOP:       sample.HasHDOP,
			Satellites:    sample.Satellites,
			HasSatellites: sample.HasSatellites,
			Source:        sample.Source,
		})
	}
	return trk
}

func sameFile(a, b string) bool {
	if a == b {
		return true
	}
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ia, ib)
}
