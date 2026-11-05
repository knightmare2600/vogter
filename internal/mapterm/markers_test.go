package mapterm

import (
	"image/color"
	"testing"
)

func TestMarkersFollowTheMap(t *testing.T) {
	m := New(odense, 14)
	m.SetSize(80, 24)
	mk := []Marker{
		{ID: "1", Pos: odense, Label: "core", Rank: 2},
		{ID: "2", Pos: odense, Label: "edge", Rank: 1},
		{ID: "3", Pos: LatLon{-33.9, 151.2}, Label: "sydney"},
		{ID: "4", Pos: LatLon{999, 0}, Label: "nowhere"},
	}
	placed := m.PlaceMarkers(mk)
	if len(placed) != 1 || placed[0].Col != 40 || placed[0].Row != 12 || len(placed[0].Markers) != 2 {
		t.Fatalf("%+v", placed)
	}
	if placed[0].Markers[0].Label != "edge" || placed[0].String() != "edge and 1 more" {
		t.Errorf("the most important comes first: %v", placed[0])
	}
	m.Pan(0.25, 0)
	if p := m.PlaceMarkers(mk[:1]); len(p) != 1 || p[0].Col != 20 {
		t.Errorf("panning a quarter screen east moves markers 20 cells west: %+v", p)
	}
	m.ZoomBy(1)
	if p := m.PlaceMarkers(mk[:1]); len(p) != 1 || p[0].Col != 0 {
		t.Errorf("zooming doubles the distance from the centre: %+v", p)
	}
}

func TestOverlay(t *testing.T) {
	f := blankFrame(10, 3)
	red := color.RGBA{255, 0, 0, 255}
	placed := []Placed{
		{Col: 1, Row: 1, Markers: []Marker{{Glyph: '●', Colour: red}}},
		{Col: 3, Row: 1, Markers: []Marker{{Animation: Blink}}},
		{Col: 5, Row: 1, Markers: []Marker{{Animation: Pulse}}},
		{Col: 7, Row: 1, Markers: []Marker{{}, {}, {}}},
	}
	on, colours := Overlay(f, placed, 0, false)
	if on.At(1, 1).R != '●' || colours[[2]int{1, 1}] != red {
		t.Error("static marker")
	}
	if on.At(3, 1).R != '●' || on.At(5, 1).R != '●' || on.At(7, 1).R != '3' {
		t.Errorf("%q %q %q", on.At(3, 1).R, on.At(5, 1).R, on.At(7, 1).R)
	}
	off, _ := Overlay(f, placed, 2, false)
	if off.At(3, 1).R != ' ' {
		t.Error("a blinking marker is hidden on the off beat")
	}
	if off.At(5, 1).R != '○' {
		t.Error("a pulsing marker never disappears")
	}
	if f.At(1, 1).R != ' ' {
		t.Error("the cached frame must not change")
	}
	asc, _ := Overlay(f, placed, 2, true)
	if asc.At(5, 1).R != 'o' || asc.At(1, 1).R != '@' {
		t.Errorf("%q %q", asc.At(5, 1).R, asc.At(1, 1).R)
	}
	if out := on.LinesWith(Palettes[0], colours); len(out) != 3 {
		t.Error(out)
	}
}
