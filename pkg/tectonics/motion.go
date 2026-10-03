package tectonics

import (
	"math"
	"math/rand/v2"
)

// Motion is one plate's rigid rotation about an Euler pole.
type Motion struct {
	Pole       [3]float64 // unit rotation axis (right-handed)
	DegPerStep float64
	SpeedCmYr  float64
	// Pending is rotation (degrees) owed but not yet applied. Step resamples
	// nearest-pixel, so a sub-pixel rotation would be quantised away every
	// step; instead DegPerStep accumulates here and Step applies only whole
	// rim-pixel angles (90/S degrees), carrying the remainder. Slow plates
	// therefore move in discrete one-pixel hops at the correct long-run rate.
	// The proper fix is a per-plate cumulative rotation sampled from a
	// reference frame (Lagrangian advection); that is future work.
	Pending float64
}

func dot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func cross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func scale(a [3]float64, s float64) [3]float64 { return [3]float64{a[0] * s, a[1] * s, a[2] * s} }

func add(a, b [3]float64) [3]float64 { return [3]float64{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }

// rotate v about the unit axis by deg degrees (Rodrigues' formula).
func rotate(v, axis [3]float64, deg float64) [3]float64 {
	r := deg * math.Pi / 180
	c, s := math.Cos(r), math.Sin(r)
	k := axis
	kv := cross(k, v)
	kd := dot(k, v)
	return [3]float64{
		v[0]*c + kv[0]*s + k[0]*kd*(1-c),
		v[1]*c + kv[1]*s + k[1]*kd*(1-c),
		v[2]*c + kv[2]*s + k[2]*kd*(1-c),
	}
}

// tangent projects v onto the plane perpendicular to the unit vector c.
func tangent(v, c [3]float64) [3]float64 { return add(v, scale(c, -dot(v, c))) }

// DegPerStep converts a plate speed to degrees of rotation per step.
func DegPerStep(speedCmYr, radiusKm, myrPerStep float64) float64 {
	km := speedCmYr * 1e-5 * myrPerStep * 1e6 // cm/yr → km over the step
	return km / (2 * math.Pi * radiusKm) * 360
}

// velocityAt is the plate's surface velocity (radians of arc per step, as a
// tangent vector) at unit direction d: ω × d.
func velocityAt(m Motion, d [3]float64) [3]float64 {
	return cross(scale(m.Pole, m.DegPerStep*math.Pi/180), d)
}

// pushDirection is the unit tangent at c that points away from the plate's
// thinnest boundary stretch. Boundary pixels are weighted by (1-th)^4 so the
// thin stretch dominates. Returns the zero vector when the plate has no
// boundary (a single-plate world).
func pushDirection(th *Grid[float64], labels *Grid[int32], id int32, c [3]float64) [3]float64 {
	nb := Neighbors4(th.S)
	dirs := Dirs(th.S)
	var acc [3]float64
	found := false
	for i, l := range labels.Cells {
		if l != id {
			continue
		}
		for _, j := range nb[i] {
			if labels.Cells[j] != id {
				w := math.Pow(1-th.Cells[i], 4)
				acc = add(acc, scale(dirs[i], w))
				found = true
				break
			}
		}
	}
	if !found {
		return [3]float64{}
	}
	thin := unit(acc)
	t := tangent(add(c, scale(thin, -1)), c) // from the thin side through c, onward
	if dot(t, t) < 1e-12 {
		return [3]float64{}
	}
	return unit(t)
}

func poleFor(c, t [3]float64) [3]float64 { return unit(cross(c, t)) }

// InitMotion draws each plate's speed and push direction.
func InitMotion(th *Grid[float64], labels *Grid[int32], plates []Plate, p Params, master int64) []Motion {
	rng := newRNG(master, "plates.motion")
	ms := make([]Motion, len(plates))
	for i, pl := range plates {
		speed := p.SpeedMinCmYr + rng.Float64()*(p.SpeedMaxCmYr-p.SpeedMinCmYr)
		t := pushDirection(th, labels, pl.ID, pl.Centroid)
		jitter := (rng.Float64()*2 - 1) * p.PushJitterDeg
		if dot(t, t) == 0 { // no boundary: pick any tangent
			t = unit(tangent([3]float64{0.3, 0.5, 0.8}, pl.Centroid))
		}
		t = rotate(t, pl.Centroid, jitter)
		ms[i] = Motion{Pole: poleFor(pl.Centroid, t), SpeedCmYr: speed,
			DegPerStep: DegPerStep(speed, p.RadiusKm, p.MyrPerStep())}
	}
	return ms
}

// Reaim blends each plate's current push direction with a fresh one computed
// from its present thinnest boundary, so locked plates slowly turn away.
func Reaim(th *Grid[float64], labels *Grid[int32], plates []Plate, ms []Motion, p Params, rng *rand.Rand) {
	for i, pl := range plates {
		if pl.Retired {
			continue
		}
		fresh := pushDirection(th, labels, pl.ID, pl.Centroid)
		if dot(fresh, fresh) == 0 {
			continue
		}
		old := unit(tangent(velocityAt(ms[i], pl.Centroid), pl.Centroid))
		sum := add(scale(old, 1-p.ReaimFresh), scale(fresh, p.ReaimFresh))
		blend := fresh // old and fresh cancel: unit() would not return zero, so test the raw sum
		if dot(sum, sum) >= 1e-12 {
			blend = unit(sum)
		}
		blend = rotate(blend, pl.Centroid, (rng.Float64()*2-1)*p.PushJitterDeg*0.5)
		ms[i].Pole = poleFor(pl.Centroid, unit(tangent(blend, pl.Centroid)))
	}
}
