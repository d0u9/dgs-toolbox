// Package hampel finds fixes whose speed stands out from their neighbours' —
// the drift of a recorder losing its signal — with a Hampel filter: a rolling
// median and median absolute deviation (MAD) of speed. It reads no files and
// draws nothing; the cleaning pipeline removes what it finds.
package hampel

import (
	"math"
	"sort"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/track"
)

// DefaultHalfWindow is how many neighbours on each side the median is taken
// over: 7, about 15 fixes, enough to outvote a short burst of drift.
const DefaultHalfWindow = 7

// DefaultThreshold is how many scaled MADs above the median a speed must be to
// be drift: 3, the usual Hampel choice.
const DefaultThreshold = 3.0

// DefaultMinDeviation is the smallest spread of speed, in metres per second,
// the threshold is measured in: 1 m/s. Steady movement has a MAD near zero, and
// without a floor any wobble would count as drift.
const DefaultMinDeviation = 1.0

// maxPasses bounds how often speeds are measured again after removals.
const maxPasses = 10

// madScale turns a MAD into a standard deviation for normally distributed data.
const madScale = 1.4826

// Params are the settings of the filter.
type Params struct {
	HalfWindow   int     `json:"halfWindow"`
	Threshold    float64 `json:"threshold"`
	MinDeviation float64 `json:"minDeviation"` // metres per second
	// Quality lowers the threshold for fixes the recorder rated poorly: HDOP
	// above 2 or fewer than 6 satellites, down to a quarter of it.
	Quality bool `json:"quality"`
}

// Defaults are the documented default parameters.
func Defaults() Params {
	return Params{HalfWindow: DefaultHalfWindow, Threshold: DefaultThreshold, MinDeviation: DefaultMinDeviation, Quality: true}
}

// Detect returns, in order, the indices of the samples whose speed is drift.
// Only timed samples with keep[i] true take part; keep may be nil for all.
//
// A sample's speed is the distance from the previous sample taking part in its
// segment over the time between. A sample is drift when that speed exceeds the
// median of its window by Threshold scaled deviations. Only too fast counts:
// slowing down is how every stop begins.
//
// A false fix raises the speed of the sample after it too, on the way back.
// So each pass removes only the samples that stand out most in their own
// neighbourhood, and speeds are measured again, until nothing stands out.
func Detect(line track.Line, keep []bool, p Params) []int {
	active := make([]bool, len(line))
	for i, sample := range line {
		active[i] = (keep == nil || keep[i]) && !sample.Time.IsZero()
	}
	var flagged []int
	for pass := 0; pass < maxPasses; pass++ {
		removed := detectPass(line, active, p)
		if len(removed) == 0 {
			break
		}
		for _, i := range removed {
			active[i] = false
		}
		flagged = append(flagged, removed...)
	}
	sort.Ints(flagged)
	return flagged
}

func detectPass(line track.Line, active []bool, p Params) []int {
	// Runs of active samples within one segment, with their speeds.
	var removed []int
	for start := 0; start < len(line); {
		var run []int
		segment := -1
		for i := start; i < len(line); i++ {
			if !active[i] {
				continue
			}
			if segment >= 0 && line[i].Segment != segment {
				break
			}
			segment = line[i].Segment
			run = append(run, i)
		}
		if len(run) == 0 {
			break
		}
		removed = append(removed, runOutliers(line, run, p)...)
		start = run[len(run)-1] + 1
	}
	return removed
}

func runOutliers(line track.Line, run []int, p Params) []int {
	n := len(run)
	speeds := make([]float64, n)
	speeds[0] = math.NaN()
	for k := 1; k < n; k++ {
		a, b := line[run[k-1]], line[run[k]]
		dt := b.Time.Sub(a.Time).Seconds()
		speeds[k] = math.NaN()
		if dt > 0 {
			speeds[k] = geo.Distance(a.LatLon, b.LatLon) / dt
		}
	}
	// excess is how far above its window a speed is, in thresholds; 0 when not.
	excess := make([]float64, n)
	window := make([]float64, 0, 2*p.HalfWindow+1)
	for k := range speeds {
		if math.IsNaN(speeds[k]) {
			continue
		}
		window = window[:0]
		for j := max(0, k-p.HalfWindow); j <= min(n-1, k+p.HalfWindow); j++ {
			if !math.IsNaN(speeds[j]) {
				window = append(window, speeds[j])
			}
		}
		if len(window) < 3 {
			continue
		}
		median, mad := medianMAD(window)
		deviation := math.Max(p.MinDeviation, madScale*mad)
		limit := p.Threshold * quality(line[run[k]], p) * deviation
		if over := speeds[k] - median; over > limit {
			excess[k] = over / limit
		}
	}
	var outliers []int
	for k := range excess {
		if excess[k] == 0 {
			continue
		}
		// Only the worst in its neighbourhood goes this pass.
		worst := true
		for j := max(0, k-1); j <= min(n-1, k+1); j++ {
			if j != k && (excess[j] > excess[k] || (excess[j] == excess[k] && j < k)) {
				worst = false
			}
		}
		if worst {
			outliers = append(outliers, run[k])
		}
	}
	return outliers
}

// quality scales the threshold down, to a quarter, for poorly rated fixes.
func quality(s track.Sample, p Params) float64 {
	if !p.Quality {
		return 1
	}
	factor := 1.0
	if s.HasHDOP && s.HDOP > 0 {
		factor *= math.Min(1, 2/s.HDOP)
	}
	if s.HasSatellites {
		factor *= math.Min(1, float64(s.Satellites)/6)
	}
	return math.Max(0.25, factor)
}

func medianMAD(values []float64) (median, mad float64) {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	median = middle(sorted)
	for i, v := range sorted {
		sorted[i] = math.Abs(v - median)
	}
	sort.Float64s(sorted)
	return median, middle(sorted)
}

func middle(sorted []float64) float64 {
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}
