package mapterm

import "math"

// View is what the map shows: a centre, a zoom and a size in dots.
type View struct {
	Center LatLon
	Zoom   float64
	// W and H are the canvas size in dots.
	W, H int
}

// Zoom limits. Past the source's maximum, tiles are enlarged.
const (
	MinZoom = 0
	MaxZoom = 19
)

// scale is the number of dots across the whole world at this view's zoom.
func (v View) scale() float64 { return TileDots * math.Pow(2, v.Zoom) }

// ToDot maps a world position to canvas dots. The x result is the nearest
// copy of the position, as the world wraps east to west.
func (v View) ToDot(w World) (float64, float64) {
	s := v.scale()
	c := Project(v.Center)
	dx := w.X - c.X
	dx -= math.Round(dx)
	return dx*s + float64(v.W)/2, (w.Y-c.Y)*s + float64(v.H)/2
}

// FromDot maps canvas dots to a world position.
func (v View) FromDot(x, y float64) World {
	s := v.scale()
	c := Project(v.Center)
	return World{X: c.X + (x-float64(v.W)/2)/s, Y: c.Y + (y-float64(v.H)/2)/s}
}

// Tiles lists the tiles at zoom z that cover the view, nearest the centre
// first so that the middle of the map fills in first.
func (v View) Tiles(z int) []TileID {
	if v.W <= 0 || v.H <= 0 {
		return nil
	}
	a := TileAt(v.FromDot(0, 0), z)
	b := TileAt(v.FromDot(float64(v.W), float64(v.H)), z)
	n := 1 << z
	// Count columns across the wrap at the antimeridian.
	cols := (b.X - a.X + n) % n
	if v.W > 0 && float64(v.W) >= v.scale() {
		cols = n - 1
	}
	c := TileAt(Project(v.Center), z)
	seen := map[TileID]bool{}
	var out []TileID
	for dy := a.Y; dy <= b.Y; dy++ {
		for i := 0; i <= cols; i++ {
			t := wrap(TileID{z, a.X + i, dy})
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	dist := func(t TileID) int {
		dx := abs(t.X - c.X)
		dx = min(dx, n-dx)
		return dx*dx + (t.Y-c.Y)*(t.Y-c.Y)
	}
	sortBy(out, dist)
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func sortBy(ts []TileID, key func(TileID) int) {
	for i := 1; i < len(ts); i++ {
		for j := i; j > 0 && key(ts[j]) < key(ts[j-1]); j-- {
			ts[j], ts[j-1] = ts[j-1], ts[j]
		}
	}
}
