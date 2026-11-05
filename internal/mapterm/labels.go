package mapterm

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/knightmare2600/vogter/internal/mapterm/mvt"
)

// Label is text to place on the map.
type Label struct {
	Text string
	X, Y float64 // dots
	// Rank orders labels; lower wins a contested space.
	Rank int
}

// placeRank orders place classes, largest first.
var placeRank = map[string]int{
	"continent": 0, "country": 1, "state": 2, "city": 3, "town": 4,
	"village": 6, "suburb": 7, "hamlet": 8, "quarter": 8, "neighbourhood": 9, "isolated_dwelling": 10,
}

// placeMinZoom hides small places until they are worth reading.
var placeMinZoom = map[string]float64{
	"city": 0, "town": 8, "village": 11, "suburb": 12, "hamlet": 13, "quarter": 14, "neighbourhood": 14, "isolated_dwelling": 15,
}

// CollectLabels gathers names from the tiles a view uses: places, then water
// and road names at street zooms.
func CollectLabels(v View, maxZoom int, lookup Lookup) []Label {
	z := DataZoom(v.Zoom, maxZoom)
	var out []Label
	for _, id := range v.Tiles(z) {
		t, ok := lookup(id)
		if !ok {
			continue
		}
		n := float64(int(1) << id.Z)
		at := func(l *mvt.Layer, p mvt.Pt) (float64, float64) {
			ext := float64(l.Extent)
			return v.ToDot(World{(float64(id.X) + float64(p.X)/ext) / n, (float64(id.Y) + float64(p.Y)/ext) / n})
		}
		if l := t.Layers["place"]; l != nil {
			for _, f := range l.Features {
				class := str(&f, "class")
				rank, ok := placeRank[class]
				if !ok || v.Zoom < placeMinZoom[class] || len(f.Geometry) == 0 || len(f.Geometry[0]) == 0 {
					continue
				}
				if name := name(&f); name != "" {
					x, y := at(l, f.Geometry[0][0])
					out = append(out, Label{name, x, y, rank})
				}
			}
		}
		// A line's label goes on its visible point nearest the centre, as
		// its middle is often off-screen when zoomed in.
		anchor := func(l *mvt.Layer, part []mvt.Pt) (float64, float64, bool) {
			best, bx, by := math.Inf(1), 0.0, 0.0
			for _, p := range part {
				x, y := at(l, p)
				if x < 0 || y < 0 || x >= float64(v.W) || y >= float64(v.H) {
					continue
				}
				if d := (x-float64(v.W)/2)*(x-float64(v.W)/2) + (y-float64(v.H)/2)*(y-float64(v.H)/2); d < best {
					best, bx, by = d, x, y
				}
			}
			return bx, by, !math.IsInf(best, 1)
		}
		if v.Zoom >= 13 {
			if l := t.Layers["water_name"]; l != nil {
				for _, f := range l.Features {
					if name := name(&f); name != "" && len(f.Geometry) > 0 {
						if x, y, ok := anchor(l, f.Geometry[0]); ok {
							out = append(out, Label{name, x, y, 11})
						}
					}
				}
			}
		}
		if v.Zoom >= 14 {
			if l := t.Layers["transportation_name"]; l != nil {
				for _, f := range l.Features {
					name := name(&f)
					if name == "" {
						continue
					}
					rank := 14
					if c := str(&f, "class"); c == "primary" || c == "secondary" || c == "trunk" || c == "motorway" {
						rank = 12
					} else if v.Zoom < 15.5 {
						continue
					}
					for _, part := range f.Geometry {
						if x, y, ok := anchor(l, part); ok {
							out = append(out, Label{name, x, y, rank})
						}
					}
				}
			}
		}
	}
	return out
}

func name(f *mvt.Feature) string {
	for _, k := range []string{"name", "name:latin", "name_en"} {
		if v, ok := f.Props[k]; ok {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" {
				return s
			}
		}
	}
	return ""
}

// transliterate spells Danish and other common letters in ASCII the way
// their own languages do, so København reads as Koebenhavn, not K?benhavn.
var transliterate = strings.NewReplacer(
	"æ", "ae", "Æ", "Ae", "ø", "oe", "Ø", "Oe", "å", "aa", "Å", "Aa",
	"ä", "ae", "Ä", "Ae", "ö", "oe", "Ö", "Oe", "ü", "ue", "Ü", "Ue", "ß", "ss",
	"é", "e", "è", "e", "ê", "e", "á", "a", "à", "a", "í", "i", "ó", "o", "ú", "u", "ç", "c", "ñ", "n",
)

// ASCIIText makes text safe for ASCII mode.
func ASCIIText(s string) string {
	s = transliterate.Replace(s)
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e {
			return '?'
		}
		return r
	}, s)
}

// PlaceLabels writes labels into a frame, most important first, skipping any
// that would overlap one already placed, run off the edge, or repeat a name
// placed nearby. It returns how many it placed.
func PlaceLabels(f *Frame, labels []Label, ascii bool) int {
	sort.SliceStable(labels, func(i, j int) bool { return labels[i].Rank < labels[j].Rank })
	taken := make([]bool, len(f.Cells))
	type spot struct{ col, row int }
	placed := map[string][]spot{}
	n := 0
	for _, l := range labels {
		text := l.Text
		if ascii {
			text = ASCIIText(text)
		}
		w := utf8.RuneCountInString(text)
		if w == 0 || w > f.Cols {
			continue
		}
		row := int(math.Floor(l.Y / CellH))
		col := int(math.Floor(l.X/CellW)) - w/2
		if row < 0 || row >= f.Rows || col < 0 || col+w > f.Cols {
			continue
		}
		near := false
		for _, s := range placed[text] {
			if abs(s.row-row) < 6 && abs(s.col-col) < 30 {
				near = true
			}
		}
		// Keep one free cell either side, so labels never run together.
		free := !near
		for c := max(col-1, 0); free && c < min(col+w+1, f.Cols); c++ {
			free = !taken[row*f.Cols+c]
		}
		if !free {
			continue
		}
		for i, r := range []rune(text) {
			cell := f.At(col+i, row)
			*cell = Cell{R: r, Class: cell.Class, Label: true}
		}
		for c := max(col-1, 0); c < min(col+w+1, f.Cols); c++ {
			taken[row*f.Cols+c] = true
		}
		placed[text] = append(placed[text], spot{col, row})
		n++
	}
	return n
}
