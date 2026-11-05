package mapterm

import (
	"math"

	"github.com/knightmare2600/vogter/internal/mapterm/mvt"
)

// Lookup returns a decoded tile from wherever the caller keeps them.
type Lookup func(TileID) (*mvt.Tile, bool)

// Draw paints the view onto c from tiles that lookup already has, and returns
// the tiles it lacked, nearest the centre first. Where a tile is missing, a
// cached tile from up to three zooms lower stands in, enlarged, so the map is
// never blank while the sharper tile loads.
func Draw(c *Canvas, v View, maxZoom int, lookup Lookup, style Style) []TileID {
	z := DataZoom(v.Zoom, maxZoom)
	var missing []TileID
	var drawn []struct {
		id   TileID
		tile *mvt.Tile
	}
	have := map[TileID]bool{}
	for _, id := range v.Tiles(z) {
		if t, ok := lookup(id); ok {
			drawn = append(drawn, struct {
				id   TileID
				tile *mvt.Tile
			}{id, t})
			continue
		}
		missing = append(missing, id)
		for up := 1; up <= 3 && id.Z-up >= 0; up++ {
			p := id.Parent(id.Z - up)
			if t, ok := lookup(p); ok && !have[p] {
				have[p] = true
				drawn = append(drawn, struct {
					id   TileID
					tile *mvt.Tile
				}{p, t})
				break
			}
		}
	}
	// Lower zooms first, so sharper tiles draw over their stand-ins.
	for pass := 0; pass < 2; pass++ {
		for _, d := range drawn {
			if (d.id.Z == z) == (pass == 1) {
				drawTile(c, v, d.id, d.tile, style)
			}
		}
	}
	return missing
}

func drawTile(c *Canvas, v View, id TileID, t *mvt.Tile, style Style) {
	n := float64(int(1) << id.Z)
	ox, oy := v.Origin()
	for _, layer := range style.Order {
		l := t.Layers[layer]
		if l == nil {
			continue
		}
		ext := float64(l.Extent)
		toDot := func(p mvt.Pt) FPt {
			x, y := v.ToDot(World{(float64(id.X) + float64(p.X)/ext) / n, (float64(id.Y) + float64(p.Y)/ext) / n})
			return FPt{x, y}
		}
		for i := range l.Features {
			f := &l.Features[i]
			rule, ok := style.Match(layer, f, v.Zoom)
			if !ok {
				continue
			}
			switch {
			case f.Type == mvt.Polygon && rule.Fill:
				rings := make([][]FPt, len(f.Geometry))
				for r, ring := range f.Geometry {
					rings[r] = make([]FPt, len(ring))
					for j, p := range ring {
						rings[r][j] = toDot(p)
					}
				}
				c.FillAt(rings, rule.Class, rule.Pattern, ox, oy)
			case f.Type == mvt.Polygon || f.Type == mvt.LineString:
				for _, part := range f.Geometry {
					for j := 1; j < len(part); j++ {
						a, b := toDot(part[j-1]), toDot(part[j])
						c.Line(a.X, a.Y, b.X, b.Y, rule.Class)
					}
				}
			}
		}
	}
}

// Cost estimates a decoded tile's memory, for the cache budget, as its point
// count.
func Cost(t *mvt.Tile) int {
	n := 1
	for _, l := range t.Layers {
		for _, f := range l.Features {
			for _, g := range f.Geometry {
				n += len(g)
			}
		}
	}
	return n
}

// round is used by tests and markers.
func round(v float64) int { return int(math.Round(v)) }

// Origin is the canvas's top-left corner in world dots at this zoom, rounded,
// for anchoring patterns to the ground.
func (v View) Origin() (int, int) {
	w := v.FromDot(0, 0)
	s := v.scale()
	return int(math.Floor(w.X * s)), int(math.Floor(w.Y * s))
}
