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

func TestPedestrianStreetsAreStreets(t *testing.T) {
	ped := &mvt.Feature{Props: map[string]any{"class": "path", "subclass": "pedestrian"}}
	foot := &mvt.Feature{Props: map[string]any{"class": "path", "subclass": "footway"}}
	service := &mvt.Feature{Props: map[string]any{"class": "service"}}
	if _, ok := DefaultStyle.Match("transportation", ped, 14); !ok {
		t.Error("a pedestrian street shows with other streets, from 14")
	}
	if _, ok := DefaultStyle.Match("transportation", foot, 15.9); ok {
		t.Error("footways wait for 16")
	}
	if _, ok := DefaultStyle.Match("transportation", service, 15); !ok {
		t.Error("service roads from 15")
	}
}

func TestOnlyGreenSpaceIsTextured(t *testing.T) {
	feature := func(class, subclass string) *mvt.Feature {
		return &mvt.Feature{Props: map[string]any{"class": class, "subclass": subclass}}
	}
	for _, c := range [][2]string{{"grass", "park"}, {"grass", "recreation_ground"}, {"wood", "forest"}} {
		if _, ok := DefaultStyle.Match("landcover", feature(c[0], c[1]), 12); !ok {
			t.Errorf("%v is green space", c)
		}
	}
	if _, ok := DefaultStyle.Match("park", feature("protected_area", ""), 12); ok {
		t.Error("a protected area, such as the reserve over Odense Fjord, is not textured")
	}
	for _, class := range []string{"Natura2000", "Reserva Natural Marinha", "wildlife_refuge"} {
		if _, ok := DefaultStyle.Match("park", feature(class, ""), 5); ok {
			t.Errorf("%s, often at sea, is not textured", class)
		}
	}
	for _, c := range [][2]string{{"grass", "meadow"}, {"grass", "grassland"}, {"farmland", "farmland"}, {"wetland", "wetland"}} {
		if _, ok := DefaultStyle.Match("landcover", feature(c[0], c[1]), 12); ok {
			t.Errorf("%v is countryside, left plain", c)
		}
	}
}

func TestBorders(t *testing.T) {
	border := func(level, maritime int64) *mvt.Feature {
		return &mvt.Feature{Props: map[string]any{"admin_level": level, "maritime": maritime}}
	}
	if _, ok := DefaultStyle.Match("boundary", border(2, 1), 5); ok {
		t.Error("a border at sea is not drawn")
	}
	if _, ok := DefaultStyle.Match("boundary", border(2, 0), 1); !ok {
		t.Error("a country border on land is")
	}
	if _, ok := DefaultStyle.Match("boundary", border(4, 0), 3); ok {
		t.Error("state borders wait for zoom 5")
	}
	if _, ok := DefaultStyle.Match("boundary", border(4, 0), 6); !ok {
		t.Error("state borders from zoom 5")
	}
}
