package mapterm

import (
	"testing"

	"github.com/knightmare2600/vogter/internal/mapterm/mvt"
)

func TestStreetLabelSitsOnTheVisiblePart(t *testing.T) {
	// A street crossing the whole tile, viewed zoomed in on its far end:
	// its middle is off-screen, but its name must still show.
	tile := &mvt.Tile{Layers: map[string]*mvt.Layer{"transportation_name": {Name: "transportation_name", Extent: 4096,
		Features: []mvt.Feature{{Type: mvt.LineString, Props: map[string]any{"name": "Vestergade", "class": "minor"},
			Geometry: [][]mvt.Pt{{pt(0, 2048), pt(1024, 2048), pt(2048, 2048), pt(3072, 2048), pt(3900, 2048)}}}}}}}
	id := TileID{14, 8664, 5150}
	n := float64(int(1) << 14)
	v := View{Center: Unproject(World{(float64(id.X) + 3900.0/4096) / n, (float64(id.Y) + 0.5) / n}), Zoom: 17, W: 120, H: 40}
	labels := CollectLabels(v, 14, func(t2 TileID) (*mvt.Tile, bool) { return tile, t2 == id })
	if len(labels) != 1 || labels[0].X < 0 || labels[0].X > 120 {
		t.Fatalf("%+v", labels)
	}
	if hidden := CollectLabels(View{Center: v.Center, Zoom: 14.5, W: 120, H: 40}, 14, func(t2 TileID) (*mvt.Tile, bool) { return tile, t2 == id }); len(hidden) != 0 {
		t.Error("minor street names wait for street zooms")
	}
}
