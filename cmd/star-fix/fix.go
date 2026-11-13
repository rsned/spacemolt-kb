package main

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Point is a galaxy-map position (+y is down, as on the in-game map).
type Point struct{ X, Y float64 }

// Star is a known system's star with the game's temperature range for its class.
type Star struct {
	System string
	Class  string
	At     Point
	Lo, Hi float64 // kelvin; Hi is +Inf for an open-ended range
}

// Sighting is one star_sightings entry from get_location with a telescope fitted.
type Sighting struct {
	Bearing     float64 `json:"bearing"`
	Brightness  int     `json:"brightness"`
	Temperature float64 `json:"temperature"`
}

// Match is a sighting tied to a known star.
type Match struct {
	Star     Star
	Err      float64 // degrees between the sighting and the true bearing
	Distance float64
}

// bearing is the telescope/Pathfinder compass bearing from a to b: 0° is east
// (+x) and angles grow toward +y, which is down on the map.
func bearing(a, b Point) float64 {
	return math.Mod(math.Atan2(b.Y-a.Y, b.X-a.X)*180/math.Pi+360, 360)
}

// angleDiff is the smallest absolute difference between two bearings.
func angleDiff(a, b float64) float64 {
	return math.Abs(math.Mod(a-b+540, 360) - 180)
}

var tempNum = regexp.MustCompile(`[\d,]+`)

// parseTempRange reads the temp_range text from poi_metadata_stars
// ("7,500–10,000 K", ">30,000 K"). Text with no numbers gives 0,0.
func parseTempRange(s string) (lo, hi float64) {
	var nums []float64
	for _, m := range tempNum.FindAllString(s, -1) {
		if v, err := strconv.ParseFloat(strings.ReplaceAll(m, ",", ""), 64); err == nil {
			nums = append(nums, v)
		}
	}
	switch {
	case len(nums) == 0:
		return 0, 0
	case strings.HasPrefix(strings.TrimSpace(s), ">"):
		return nums[0], math.Inf(1)
	default:
		return nums[0], nums[len(nums)-1]
	}
}

// matchSighting finds the known star the sighting most likely is: within tol
// degrees, its class's temperature range holding the reading, and nearest to
// the observer (a telescope lists the brightest stars, so nearer is likelier).
func matchSighting(obs Point, obsID string, s Sighting, stars []Star, tol float64) (Match, bool) {
	var best Match
	found := false
	for _, m := range candidates(obs, obsID, s, stars, tol) {
		if !found || m.Distance < best.Distance {
			best, found = m, true
		}
	}
	return best, found
}

// candidates lists every known star that could be the sighting: within tol
// degrees and with the reading inside its class's temperature range.
func candidates(obs Point, obsID string, s Sighting, stars []Star, tol float64) []Match {
	var out []Match
	for _, st := range stars {
		if st.System == obsID || s.Temperature < st.Lo || s.Temperature > st.Hi {
			continue
		}
		d := math.Hypot(st.At.X-obs.X, st.At.Y-obs.Y)
		if d == 0 {
			continue
		}
		if e := angleDiff(bearing(obs, st.At), s.Bearing); e <= tol {
			out = append(out, Match{Star: st, Err: e, Distance: d})
		}
	}
	return out
}

// intersect crosses two bearing lines and reports where they meet, if they
// meet ahead of both observers.
func intersect(a Point, ba float64, b Point, bb float64) (Point, bool) {
	ax, ay := math.Cos(ba*math.Pi/180), math.Sin(ba*math.Pi/180)
	bx, by := math.Cos(bb*math.Pi/180), math.Sin(bb*math.Pi/180)
	den := ax*by - ay*bx
	if math.Abs(den) < 1e-9 {
		return Point{}, false
	}
	dx, dy := b.X-a.X, b.Y-a.Y
	t := (dx*by - dy*bx) / den
	u := (dx*ay - dy*ax) / den
	if t <= 0 || u <= 0 {
		return Point{}, false
	}
	return Point{a.X + t*ax, a.Y + t*ay}, true
}
