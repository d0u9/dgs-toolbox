// Package spikes finds sudden jumps in a track: single fixes far off the path
// that the recorder reached and left again at once (A→B→A). It reads no files
// and draws nothing; the cleaning pipeline removes what it finds.
package spikes

import (
	"math"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/track"
)

// DefaultMaxSpeed is the speed no recorded fix is reached and left at: 100 m/s
// (360 km/h), above any car or train. A track recorded in flight exceeds it
// and needs it raised.
const DefaultMaxSpeed = 100.0

// DefaultMaxAcceleration is the change of speed per second no vehicle makes:
// 15 m/s², beyond hard braking.
const DefaultMaxAcceleration = 15.0

// DefaultTurnAngle is how sharply the path must reverse at a fix, in degrees,
// for it to be an out-and-back spike: 160°, nearly straight back. A hairpin
// bend recorded every few seconds can turn 150° at one fix; it rarely turns
// 160°.
const DefaultTurnAngle = 160.0

// DefaultMinJump is how far, in metres, an out-and-back fix must stand from
// both neighbours: 30 m, beyond the jitter of a fix standing still.
const DefaultMinJump = 30.0

// MaxBurst is the longest run of consecutive bad fixes removed as one spike.
const MaxBurst = 5

// Params are the thresholds of spike detection.
type Params struct {
	MaxSpeed        float64 `json:"maxSpeed"`        // metres per second
	MaxAcceleration float64 `json:"maxAcceleration"` // metres per second squared
	TurnAngle       float64 `json:"turnAngle"`       // degrees
	MinJump         float64 `json:"minJump"`         // metres
}

// Defaults are the documented default parameters.
func Defaults() Params {
	return Params{MaxSpeed: DefaultMaxSpeed, MaxAcceleration: DefaultMaxAcceleration, TurnAngle: DefaultTurnAngle, MinJump: DefaultMinJump}
}

// Detect returns, in order, the indices of the samples that are spikes. Only
// samples with keep[i] true take part; keep may be nil for all. Neighbours are
// the nearest taking part in the same segment, and a flagged sample stops being
// a neighbour.
//
// A sample is a spike when, between its neighbours:
//   - it was both reached and left faster than MaxSpeed; or
//   - the speed rose into it and fell out of it by more than MaxAcceleration;
//   - or the path turns back at it by at least TurnAngle while it stands at
//     least MinJump from both neighbours and further from each than they are
//     from each other.
//
// A run of up to MaxBurst samples reached faster than MaxSpeed and left faster
// than MaxSpeed, while going straight past it is not, is a spike as a whole.
// The speed rules need times; the turn rule does not.
func Detect(line track.Line, keep []bool, p Params) []int {
	var flagged []int
	prev := -1
	for i := 0; i < len(line); i++ {
		if keep != nil && !keep[i] {
			continue
		}
		if prev < 0 || line[prev].Segment != line[i].Segment {
			prev = i
			continue
		}
		next := nextKept(line, keep, i)
		if next >= 0 && isSpike(line[prev], line[i], line[next], p) {
			flagged = append(flagged, i)
			continue // prev stays: the spike is no neighbour
		}
		if run := burst(line, keep, prev, i, p); len(run) > 0 {
			flagged = append(flagged, run...)
			i = run[len(run)-1]
			continue
		}
		prev = i
	}
	return flagged
}

// burst is the run starting at first that was entered from prev and left
// faster than MaxSpeed, when passing straight from prev to after it is not.
func burst(line track.Line, keep []bool, prev, first int, p Params) []int {
	if speed(line[prev], line[first]) <= p.MaxSpeed {
		return nil
	}
	run := []int{first}
	for last := first; len(run) <= MaxBurst; {
		next := nextKept(line, keep, last)
		if next < 0 {
			return nil
		}
		if speed(line[last], line[next]) > p.MaxSpeed && speed(line[prev], line[next]) <= p.MaxSpeed {
			return run
		}
		run = append(run, next)
		last = next
	}
	return nil
}

// speed between two samples in metres per second; 0 without elapsed time.
func speed(a, b track.Sample) float64 {
	if !timed(a, b) {
		return 0
	}
	dt := b.Time.Sub(a.Time).Seconds()
	if dt <= 0 {
		return 0
	}
	return geo.Distance(a.LatLon, b.LatLon) / dt
}

func nextKept(line track.Line, keep []bool, i int) int {
	for j := i + 1; j < len(line); j++ {
		if line[j].Segment != line[i].Segment {
			return -1
		}
		if keep == nil || keep[j] {
			return j
		}
	}
	return -1
}

func isSpike(a, b, c track.Sample, p Params) bool {
	in, out := geo.Distance(a.LatLon, b.LatLon), geo.Distance(b.LatLon, c.LatLon)
	if timed(a, b, c) {
		dtIn, dtOut := b.Time.Sub(a.Time).Seconds(), c.Time.Sub(b.Time).Seconds()
		if dtIn > 0 && dtOut > 0 {
			vIn, vOut := in/dtIn, out/dtOut
			if vIn > p.MaxSpeed && vOut > p.MaxSpeed {
				return true
			}
			// Speed through the spike against speed straight past it.
			vPast := geo.Distance(a.LatLon, c.LatLon) / (dtIn + dtOut)
			if (vIn-vPast)/dtIn > p.MaxAcceleration && (vOut-vPast)/dtOut > p.MaxAcceleration {
				return true
			}
		}
	}
	past := geo.Distance(a.LatLon, c.LatLon)
	return in >= p.MinJump && out >= p.MinJump && past < math.Min(in, out) && turn(a, b, c) >= p.TurnAngle
}

func timed(samples ...track.Sample) bool {
	for _, s := range samples {
		if s.Time.IsZero() {
			return false
		}
	}
	return true
}

// turn is how far the heading changes at b, in degrees: 0 straight on, 180
// straight back.
func turn(a, b, c track.Sample) float64 {
	plane := geo.NewPlane(b.LatLon)
	ax, ay := plane.XY(a.LatLon)
	cx, cy := plane.XY(c.LatLon)
	inX, inY := -ax, -ay // a → b
	outX, outY := cx, cy // b → c
	norm := math.Hypot(inX, inY) * math.Hypot(outX, outY)
	if norm == 0 {
		return 0
	}
	cos := math.Max(-1, math.Min(1, (inX*outX+inY*outY)/norm))
	return math.Acos(cos) * 180 / math.Pi
}
