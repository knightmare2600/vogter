package mapterm

import "math"

// MaxLat is the latitude where Web Mercator's square world ends.
const MaxLat = 85.05112878

// LatLon is a geographic position in degrees.
type LatLon struct{ Lat, Lon float64 }

// Valid reports whether the position is a real place, which the Mercator
// projection can show.
func (p LatLon) Valid() bool {
	return !math.IsNaN(p.Lat) && !math.IsNaN(p.Lon) && !math.IsInf(p.Lat, 0) && !math.IsInf(p.Lon, 0) &&
		p.Lat >= -90 && p.Lat <= 90 && p.Lon >= -180 && p.Lon <= 180
}

// World is a position on the Web Mercator square, 0 to 1 each way, with y
// growing southwards as in tile numbering.
type World struct{ X, Y float64 }

// Project maps a position onto the world square, clamping latitude to MaxLat.
func Project(p LatLon) World {
	lat := math.Max(-MaxLat, math.Min(MaxLat, p.Lat)) * math.Pi / 180
	return World{
		X: (p.Lon + 180) / 360,
		Y: (1 - math.Log(math.Tan(lat)+1/math.Cos(lat))/math.Pi) / 2,
	}
}

// Unproject is the inverse of Project. X wraps around the world.
func Unproject(w World) LatLon {
	x := w.X - math.Floor(w.X)
	n := math.Pi * (1 - 2*w.Y)
	return LatLon{Lat: math.Atan(math.Sinh(n)) * 180 / math.Pi, Lon: x*360 - 180}
}

// MetresPerDot is the ground distance one dot covers at a latitude and zoom,
// for a scale bar.
func MetresPerDot(lat, zoom float64) float64 {
	const earthCircumference = 40075016.686
	return earthCircumference * math.Cos(lat*math.Pi/180) / (TileDots * math.Pow(2, zoom))
}
