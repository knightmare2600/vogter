package mapterm

import (
	"math"
	"slices"
)

// Line draws a straight line between two dot positions, clipped to the
// canvas, so a long road far off-screen costs almost nothing.
func (c *Canvas) Line(x0, y0, x1, y1 float64, class Class) {
	var ok bool
	if x0, y0, x1, y1, ok = clip(x0, y0, x1, y1, -1, -1, float64(c.W()), float64(c.H())); !ok {
		return
	}
	ax, ay := int(math.Round(x0)), int(math.Round(y0))
	bx, by := int(math.Round(x1)), int(math.Round(y1))
	dx, dy := abs(bx-ax), -abs(by-ay)
	sx, sy := 1, 1
	if ax > bx {
		sx = -1
	}
	if ay > by {
		sy = -1
	}
	e := dx + dy
	for {
		c.Set(ax, ay, class)
		if ax == bx && ay == by {
			return
		}
		e2 := 2 * e
		if e2 >= dy {
			e += dy
			ax += sx
		}
		if e2 <= dx {
			e += dx
			ay += sy
		}
	}
}

// clip trims a segment to a rectangle with the Liang–Barsky method.
func clip(x0, y0, x1, y1, minX, minY, maxX, maxY float64) (float64, float64, float64, float64, bool) {
	t0, t1 := 0.0, 1.0
	dx, dy := x1-x0, y1-y0
	for _, e := range [4][2]float64{{-dx, x0 - minX}, {dx, maxX - x0}, {-dy, y0 - minY}, {dy, maxY - y0}} {
		p, q := e[0], e[1]
		if p == 0 {
			if q < 0 {
				return 0, 0, 0, 0, false
			}
			continue
		}
		r := q / p
		if p < 0 {
			t0 = math.Max(t0, r)
		} else {
			t1 = math.Min(t1, r)
		}
		if t0 > t1 {
			return 0, 0, 0, 0, false
		}
	}
	return x0 + t0*dx, y0 + t0*dy, x0 + t1*dx, y0 + t1*dy, true
}

// FPt is a position in dots.
type FPt struct{ X, Y float64 }

// Fill fills the area inside rings by the even-odd rule, which treats holes,
// such as a courtyard, correctly whatever the rings' direction. pattern picks
// which dots of the area light, so that large fills such as water can be
// drawn lightly.
func (c *Canvas) Fill(rings [][]FPt, class Class, pattern func(x, y int) bool) {
	c.FillAt(rings, class, pattern, 0, 0)
}

// FillAt is Fill with the pattern anchored at an offset, the canvas origin's
// position in the world, so that patterns stay put on the ground when the
// map pans.
func (c *Canvas) FillAt(rings [][]FPt, class Class, pattern func(x, y int) bool, ox, oy int) {
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, r := range rings {
		for _, p := range r {
			minY, maxY = math.Min(minY, p.Y), math.Max(maxY, p.Y)
		}
	}
	y0 := max(int(math.Ceil(minY-0.5)), 0)
	y1 := min(int(math.Floor(maxY-0.5)), c.H()-1)
	var xs []float64
	for y := y0; y <= y1; y++ {
		sy := float64(y) + 0.5
		xs = xs[:0]
		for _, r := range rings {
			for i := 1; i < len(r); i++ {
				a, b := r[i-1], r[i]
				if (a.Y <= sy) != (b.Y <= sy) {
					xs = append(xs, a.X+(sy-a.Y)*(b.X-a.X)/(b.Y-a.Y))
				}
			}
		}
		slices.Sort(xs)
		for i := 0; i+1 < len(xs); i += 2 {
			from := max(int(math.Ceil(xs[i]-0.5)), 0)
			to := min(int(math.Floor(xs[i+1]-0.5)), c.W()-1)
			for x := from; x <= to; x++ {
				if pattern == nil || pattern(x+ox, y+oy) {
					c.Set(x, y, class)
				}
			}
		}
	}
}
