package mapterm

import (
	"fmt"
	"image/color"
	"sort"
)

// Animation makes a marker draw attention.
type Animation int

// Marker animations. Phase counts the UI's clock ticks.
const (
	Still Animation = iota
	// Blink shows the marker on alternate beats, for warnings.
	Blink
	// Pulse swaps between a filled and a hollow glyph, for critical states,
	// so it never disappears.
	Pulse
)

// Marker is a point drawn over the map.
type Marker struct {
	ID       string
	Pos      LatLon
	Label    string
	Glyph    rune
	Colour   color.Color
	Category string
	// Rank orders markers sharing a cell; the lowest is shown.
	Rank      int
	Animation Animation
	Highlight bool
}

// Placed is a marker's cell on screen. A cell holding several markers is one
// Placed with all of them, most important first.
type Placed struct {
	Col, Row int
	Markers  []Marker
}

// PlaceMarkers finds the cells markers fall in. Those off screen or with an
// invalid position are left out; markers sharing a cell are grouped.
func (m *Map) PlaceMarkers(markers []Marker) []Placed {
	cells := map[[2]int]*Placed{}
	var order [][2]int
	for _, mk := range markers {
		col, row, ok := m.Locate(mk.Pos)
		if !ok {
			continue
		}
		k := [2]int{col, row}
		p := cells[k]
		if p == nil {
			p = &Placed{Col: col, Row: row}
			cells[k] = p
			order = append(order, k)
		}
		p.Markers = append(p.Markers, mk)
	}
	out := make([]Placed, 0, len(order))
	for _, k := range order {
		p := cells[k]
		sort.SliceStable(p.Markers, func(i, j int) bool { return p.Markers[i].Rank < p.Markers[j].Rank })
		out = append(out, *p)
	}
	return out
}

// Overlay draws markers onto a copy of a frame, leaving the cached frame as
// it was, so a blinking marker never forces the map itself to draw again.
// phase is a counter that advances with the UI clock.
func Overlay(f *Frame, placed []Placed, phase int, ascii bool) (*Frame, map[[2]int]color.Color) {
	out := &Frame{Cols: f.Cols, Rows: f.Rows, Cells: append([]Cell(nil), f.Cells...)}
	colours := map[[2]int]color.Color{}
	for _, p := range placed {
		top := p.Markers[0]
		glyph := top.Glyph
		if glyph == 0 {
			glyph = '●'
		}
		anim := top.Animation
		if top.Highlight && anim == Still {
			anim = Blink
		}
		switch anim {
		case Blink:
			if phase%4 >= 2 {
				continue
			}
		case Pulse:
			if phase%4 >= 2 {
				glyph = '○'
			}
		}
		if n := len(p.Markers); n > 1 {
			glyph = '+'
			if n <= 9 {
				glyph = rune('0' + n)
			}
		}
		if ascii {
			glyph = asciiMarker(glyph)
		}
		c := out.At(p.Col, p.Row)
		*c = Cell{R: glyph, Class: c.Class, Label: true}
		colours[[2]int{p.Col, p.Row}] = top.Colour
	}
	return out, colours
}

func asciiMarker(r rune) rune {
	switch r {
	case '●':
		return '@'
	case '○':
		return 'o'
	case '▲':
		return '^'
	case '◆':
		return '*'
	case '■':
		return '#'
	}
	if r > 0x7e {
		return '*'
	}
	return r
}

// String describes a group of markers for a status line.
func (p Placed) String() string {
	if len(p.Markers) == 1 {
		return p.Markers[0].Label
	}
	return fmt.Sprintf("%s and %d more", p.Markers[0].Label, len(p.Markers)-1)
}
