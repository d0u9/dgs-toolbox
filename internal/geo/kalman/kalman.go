// Package kalman smooths position jitter out of a track with a constant-
// velocity Kalman filter followed by a Rauch–Tung–Striebel (RTS) smoother. It
// moves fixes rather than removing them. It reads no files and draws nothing.
package kalman

import (
	"math"
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/track"
)

// DefaultPositionSigma is the standard deviation of a fix's error in metres at
// HDOP 1: 5 m, typical of a phone or watch in the open.
const DefaultPositionSigma = 5.0

// DefaultAcceleration is the standard deviation of the acceleration the model
// allows, in metres per second squared: 1, enough to follow a car pulling away
// while keeping a walker's line straight.
const DefaultAcceleration = 1.0

// DefaultMaxGap is the longest pause the filter carries speed across: 60 s.
// After a longer one the recorder may have moved anywhere, so smoothing
// restarts.
const DefaultMaxGap = 60 * time.Second

// Params are the noise settings of the model.
type Params struct {
	PositionSigma float64 `json:"positionSigma"` // metres at HDOP 1
	Acceleration  float64 `json:"acceleration"`  // metres per second squared
	MaxGap        float64 `json:"maxGap"`        // seconds
}

// Defaults are the documented default parameters.
func Defaults() Params {
	return Params{PositionSigma: DefaultPositionSigma, Acceleration: DefaultAcceleration, MaxGap: DefaultMaxGap.Seconds()}
}

// Smooth returns the smoothed position of every sample. Samples with keep[i]
// false (keep may be nil for all), without a time, or alone in their run are
// returned where they were. A run is the samples taking part in one segment
// with no pause longer than MaxGap; each is smoothed on its own.
//
// East and north are filtered separately, each as position and velocity, in
// a flat frame around the run's first fix. A fix with an HDOP counts as
// PositionSigma × HDOP metres uncertain.
func Smooth(line track.Line, keep []bool, p Params) []geo.LatLon {
	out := make([]geo.LatLon, len(line))
	for i, sample := range line {
		out[i] = sample.LatLon
	}
	var run []int
	flush := func() {
		if len(run) > 1 {
			smoothRun(line, run, p, out)
		}
		run = run[:0]
	}
	for i, sample := range line {
		if (keep != nil && !keep[i]) || sample.Time.IsZero() {
			continue
		}
		if len(run) > 0 {
			last := line[run[len(run)-1]]
			gap := sample.Time.Sub(last.Time).Seconds()
			if last.Segment != sample.Segment || gap > p.MaxGap || gap <= 0 {
				flush()
			}
		}
		run = append(run, i)
	}
	flush()
	return out
}

// state is position and velocity along one axis with its covariance.
type state struct {
	x, v          float64
	pxx, pxv, pvv float64
}

func smoothRun(line track.Line, run []int, p Params, out []geo.LatLon) {
	plane := geo.NewPlane(line[run[0]].LatLon)
	n := len(run)
	xs, ys, rs, dts := make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n)
	for k, i := range run {
		xs[k], ys[k] = plane.XY(line[i].LatLon)
		sigma := p.PositionSigma
		if line[i].HasHDOP && line[i].HDOP > 0 {
			sigma *= line[i].HDOP
		}
		rs[k] = sigma * sigma
		if k > 0 {
			dts[k] = line[i].Time.Sub(line[run[k-1]].Time).Seconds()
		}
	}
	q := p.Acceleration * p.Acceleration
	east := smoothAxis(xs, rs, dts, q)
	north := smoothAxis(ys, rs, dts, q)
	for k, i := range run {
		out[i] = plane.LatLon(east[k], north[k])
	}
}

// smoothAxis runs the forward filter and the RTS smoother over one axis and
// returns the smoothed positions.
func smoothAxis(z, r, dt []float64, q float64) []float64 {
	n := len(z)
	predicted, filtered := make([]state, n), make([]state, n)
	// Start at the first fix, not moving, with its own uncertainty and a wide
	// uncertainty in speed.
	filtered[0] = state{x: z[0], pxx: r[0], pvv: 100}
	predicted[0] = filtered[0]
	for k := 1; k < n; k++ {
		predicted[k] = predict(filtered[k-1], dt[k], q)
		filtered[k] = update(predicted[k], z[k], r[k])
	}
	smoothed := make([]float64, n)
	next := filtered[n-1]
	smoothed[n-1] = next.x
	for k := n - 2; k >= 0; k-- {
		f, pr := filtered[k], predicted[k+1]
		// C = P_f Fᵀ P_p⁻¹ with F = [[1, dt], [0, 1]].
		d := dt[k+1]
		a := f.pxx + d*f.pxv // (P_f Fᵀ) row 0
		b := f.pxv
		c := f.pxv + d*f.pvv // (P_f Fᵀ) row 1
		e := f.pvv
		det := pr.pxx*pr.pvv - pr.pxv*pr.pxv
		if det <= 0 || math.IsNaN(det) {
			smoothed[k] = f.x
			next = f
			continue
		}
		inv00, inv01, inv11 := pr.pvv/det, -pr.pxv/det, pr.pxx/det
		c00, c01 := a*inv00+b*inv01, a*inv01+b*inv11
		c10, c11 := c*inv00+e*inv01, c*inv01+e*inv11
		dx, dv := next.x-pr.x, next.v-pr.v
		s := state{
			x: f.x + c00*dx + c01*dv,
			v: f.v + c10*dx + c11*dv,
		}
		// Each step back reads only the smoothed mean after it, so the smoothed
		// covariance is not kept.
		smoothed[k] = s.x
		next = s
	}
	return smoothed
}

func predict(s state, dt, q float64) state {
	return state{
		x:   s.x + dt*s.v,
		v:   s.v,
		pxx: s.pxx + 2*dt*s.pxv + dt*dt*s.pvv + q*dt*dt*dt/3,
		pxv: s.pxv + dt*s.pvv + q*dt*dt/2,
		pvv: s.pvv + q*dt,
	}
}

func update(s state, z, r float64) state {
	innovation := s.pxx + r
	kx, kv := s.pxx/innovation, s.pxv/innovation
	residual := z - s.x
	return state{
		x:   s.x + kx*residual,
		v:   s.v + kv*residual,
		pxx: (1 - kx) * s.pxx,
		pxv: (1 - kx) * s.pxv,
		pvv: s.pvv - kv*s.pxv,
	}
}
