package mapterm

import (
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// Palette colours a frame. Background, when set, fills the map area, which
// the solarized palettes need for their designed contrast.
type Palette struct {
	Name       string
	Background color.Color
	Label      color.Color
	Classes    map[Class]color.Color
	// Status colours markers by health. Shapes differ as well, so colour is
	// never the only cue.
	Status [StatusCount]color.Color
}

// Status is a marker's health, best last.
type Status int

// Marker statuses.
const (
	StatusCritical Status = iota
	StatusWarning
	StatusUnknown
	StatusOK
	StatusCount
)

// StatusGlyphs are the marker shapes by status.
var StatusGlyphs = [StatusCount]rune{'■', '▲', '○', '●'}

var phosphorStatus = [StatusCount]color.Color{hex("#ff4b4b"), hex("#ffd23f"), hex("#9aa0a6"), hex("#3ddc5a")}

func hex(s string) color.Color { return lipgloss.Color(s) }

// phosphor is a one-colour palette in the style of an old monitor: brighter
// for more important features.
func phosphor(name, dim, mid, bright, label string) Palette {
	return Palette{
		Name:       name,
		Status:     phosphorStatus,
		Background: hex("#050805"),
		Label:      hex(label),
		Classes: map[Class]color.Color{
			ClassLand: hex(dim), ClassPark: hex(dim), ClassBuilding: hex(dim),
			ClassWater: hex(mid), ClassBoundary: hex(mid), ClassMinorRoad: hex(mid), ClassWaterway: hex(mid), ClassRail: hex(mid),
			ClassMajorRoad: hex(bright), ClassMotorway: hex(bright),
		},
	}
}

// Solarized colours, from Ethan Schoonover's published values.
const (
	solBase03 = "#002b36"
	solBase02 = "#073642"
	solBase01 = "#586e75"
	solBase00 = "#657b83"
	solBase0  = "#839496"
	solBase1  = "#93a1a1"
	solBase2  = "#eee8d5"
	solBase3  = "#fdf6e3"
	solYellow = "#b58900"
	solOrange = "#cb4b16"
	solRed    = "#dc322f"
	solViolet = "#6c71c4"
	solBlue   = "#268bd2"
	solCyan   = "#2aa198"
	solGreen  = "#859900"
)

// Palettes, the default first. Every palette gives labels at least 4.5:1
// contrast and roads and water at least 3:1, the WCAG targets for text and
// graphics, which a test holds them to; soft textures such as parks may be
// lower so they stay in the background. Green phosphor is the default look; the other
// phosphors are kept for later.
var Palettes = []Palette{
	phosphor("green", "#1f6f2a", "#2fbf45", "#7dff8c", "#c8ffd0"),
	{
		Name: "solarized-dark", Background: hex(solBase03), Label: hex(solBase1),
		Status: [StatusCount]color.Color{hex(solRed), hex(solYellow), hex(solBase1), hex(solGreen)},
		Classes: map[Class]color.Color{
			ClassLand: hex(solBase01), ClassPark: hex(solGreen), ClassWater: hex(solBlue), ClassBuilding: hex(solBase01),
			ClassBoundary: hex(solViolet), ClassMinorRoad: hex(solBase0), ClassWaterway: hex(solCyan), ClassRail: hex(solBase1),
			ClassMajorRoad: hex(solYellow), ClassMotorway: hex(solOrange),
		},
	},
	{
		Name: "solarized-light", Background: hex(solBase3), Label: hex(solBase02),
		// Solarized's own green and yellow fall below 3:1 on base3, so a
		// darker green and the orange stand in.
		Status: [StatusCount]color.Color{hex(solRed), hex(solOrange), hex(solBase01), hex("#5c6f00")},
		Classes: map[Class]color.Color{
			ClassLand: hex(solBase1), ClassPark: hex(solGreen), ClassWater: hex(solBlue), ClassBuilding: hex(solBase1),
			ClassBoundary: hex(solViolet), ClassMinorRoad: hex(solBase00), ClassWaterway: hex(solBlue), ClassRail: hex(solBase01),
			ClassMajorRoad: hex(solOrange), ClassMotorway: hex(solRed),
		},
	},
}

// ExtraPalettes unlock once the map is known to work: the other phosphors.
var ExtraPalettes = []Palette{
	phosphor("amber", "#7a4a0c", "#c77b14", "#ffb347", "#ffe0b0"),
	phosphor("blue", "#1a3f7a", "#2f6fd1", "#7fb2ff", "#d2e4ff"),
	phosphor("cyan", "#12666a", "#1fb3b8", "#6ff3f7", "#cdfcfd"),
}

// PaletteByName finds a palette, among the extras too when allowed.
func PaletteByName(name string, extras bool) (Palette, bool) {
	all := Palettes
	if extras {
		all = append(append([]Palette{}, Palettes...), ExtraPalettes...)
	}
	for _, p := range all {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return Palettes[0], false
}

// Lines renders a frame as styled text, one string per row. Runs of cells in
// the same colour share one style, to keep the output small.
func (f *Frame) Lines(p Palette) []string { return f.LinesWith(p, nil) }

// LinesWith is Lines with some cells' colours set by position, for markers.
func (f *Frame) LinesWith(p Palette, overrides map[[2]int]color.Color) []string {
	out := make([]string, f.Rows)
	var b, run strings.Builder
	for row := range f.Rows {
		b.Reset()
		var cur color.Color
		flush := func() {
			if run.Len() == 0 {
				return
			}
			s := lipgloss.NewStyle()
			if cur != nil {
				s = s.Foreground(cur)
			}
			if p.Background != nil {
				s = s.Background(p.Background)
			}
			b.WriteString(s.Render(run.String()))
			run.Reset()
		}
		for col := range f.Cols {
			c := f.At(col, row)
			fg := p.Classes[c.Class]
			if c.Label {
				fg = p.Label
			}
			if o, ok := overrides[[2]int{col, row}]; ok && o != nil {
				fg = o
			}
			if fg != cur {
				flush()
				cur = fg
			}
			run.WriteRune(c.R)
		}
		flush()
		out[row] = b.String()
	}
	return out
}

// Contrast is the WCAG contrast ratio between two colours, 1 to 21.
func Contrast(a, b color.Color) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		s := float64(v) / 0xffff
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}
