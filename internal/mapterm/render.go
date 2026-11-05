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
	z := DataZoom(v.DetailZoom(), maxZoom)
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
	if v.DetailZoom() < style.LandBelow {
		drawLand(c, v, id, t, ox, oy)
	}
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
			rule, ok := style.Match(layer, f, v.DetailZoom())
			if !ok || (rule.Class == ClassBuilding && v.Zoom < MinBuildingZoom) {
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
						// A polygon is cut where its tile ends; that cut is not
						// a shore or a wall, so it is not drawn.
						if f.Type == mvt.Polygon && onTileEdge(part[j-1], part[j], l.Extent) {
							continue
						}
						a, b := toDot(part[j-1]), toDot(part[j])
						c.Line(a.X, a.Y, b.X, b.Y, rule.Class)
					}
				}
			}
		}
	}
}

// onTileEdge reports a segment lying along or beyond one side of its tile,
// where a polygon was clipped.
func onTileEdge(a, b mvt.Pt, extent int) bool {
	e := int32(extent)
	return (a.X <= 0 && b.X <= 0) || (a.X >= e && b.X >= e) || (a.Y <= 0 && b.Y <= 0) || (a.Y >= e && b.Y >= e)
}

// drawLand textures the land of a tile. The schema has no land shapes, as
// land is wherever water is not, so the tile's square and its water are
// filled together by the even-odd rule, which leaves the land. A tile with no
// layers at all says nothing about land or sea and is left blank.
func drawLand(c *Canvas, v View, id TileID, t *mvt.Tile, ox, oy int) {
	if len(t.Layers) == 0 {
		return
	}
	n := float64(int(1) << id.Z)
	corner := func(x, y float64) FPt {
		px, py := v.ToDot(World{(float64(id.X) + x) / n, (float64(id.Y) + y) / n})
		return FPt{px, py}
	}
	rings := [][]FPt{{corner(0, 0), corner(1, 0), corner(1, 1), corner(0, 1), corner(0, 0)}}
	if l := t.Layers["water"]; l != nil {
		ext := float64(l.Extent)
		for _, f := range l.Features {
			if f.Type != mvt.Polygon {
				continue
			}
			for _, ring := range f.Geometry {
				r := make([]FPt, len(ring))
				for j, p := range ring {
					// Clamp to the tile, so water in the buffer around it
					// cannot cut land out of the neighbouring tile.
					x := min(max(float64(p.X)/ext, 0), 1)
					y := min(max(float64(p.Y)/ext, 0), 1)
					r[j] = corner(x, y)
				}
				rings = append(rings, r)
			}
		}
	}
	c.FillAt(rings, ClassLand, landTexture, ox, oy)
}

// MinBuildingZoom is the least zoom at which buildings are drawn, whatever
// the detail: below it each building is a dot or two, and together they fill
// a small panel solid.
const MinBuildingZoom = 15

// MinStreetNameZoom is the same limit for minor street names, which would
// otherwise cover a small panel.
const MinStreetNameZoom = 14.5

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
