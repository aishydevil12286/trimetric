package main

import "math"

// point is a WGS84 coordinate.
type point struct {
	Lat, Lng float64
}

// polyline is an ordered run of points describing a route's path.
type polyline []point

// lengths returns the cumulative planar length at each vertex, along with the
// total. Distances are only used to interpolate along the line and to fill in
// shape_dist_traveled, so treating degrees as flat is accurate enough.
func (p polyline) lengths() ([]float64, float64) {
	cum := make([]float64, len(p))
	var total float64
	for i := 1; i < len(p); i++ {
		total += math.Hypot(p[i].Lat-p[i-1].Lat, p[i].Lng-p[i-1].Lng)
		cum[i] = total
	}
	return cum, total
}

// at returns the position a given fraction of the way along the line, plus the
// bearing in degrees at that position.
func (p polyline) at(frac float64) (point, float64) {
	if len(p) == 0 {
		return point{}, 0
	}
	if len(p) == 1 {
		return p[0], 0
	}

	cum, total := p.lengths()
	if total == 0 {
		return p[0], 0
	}

	target := math.Mod(frac, 1)
	if target < 0 {
		target += 1
	}
	target *= total

	// Walk to the segment containing the target distance.
	i := 1
	for i < len(p)-1 && cum[i] < target {
		i++
	}

	segLen := cum[i] - cum[i-1]
	var t float64
	if segLen > 0 {
		t = (target - cum[i-1]) / segLen
	}

	a, b := p[i-1], p[i]
	pos := point{
		Lat: a.Lat + (b.Lat-a.Lat)*t,
		Lng: a.Lng + (b.Lng-a.Lng)*t,
	}

	// Longitude is compressed by latitude, so scale it before taking the
	// heading or every bearing skews east.
	dx := (b.Lng - a.Lng) * math.Cos(pos.Lat*math.Pi/180)
	dy := b.Lat - a.Lat
	bearing := math.Mod(math.Atan2(dx, dy)*180/math.Pi+360, 360)

	return pos, bearing
}

// wobble is a smooth pseudo-random offset built from a few sine waves. It
// bends the generated corridors so they read as roads rather than as rays out
// of a hub.
func wobble(t, seed float64) float64 {
	return math.Sin(t*2.7+seed)*0.35 +
		math.Sin(t*6.1+seed*1.7)*0.18 +
		math.Sin(t*11.3+seed*2.9)*0.07
}
