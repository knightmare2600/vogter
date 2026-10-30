// Package theme holds the built-in palettes and reduces each one to the
// colour roles the UI paints with. Values and role formulas are those of
// zabterm (src/theme.rs). Omarchy integration arrives in Milestone 5.
package theme

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"
)

// Builtin lists the built-in themes in cycling order.
var Builtin = []string{"tokyo-night", "catppuccin", "gruvbox", "nord", "everforest", "rose-pine", "kanagawa", "matte-black"}

// RGB is a 24-bit colour.
type RGB struct{ R, G, B uint8 }

// RGBA implements color.Color.
func (c RGB) RGBA() (r, g, b, a uint32) {
	return uint32(c.R) * 0x101, uint32(c.G) * 0x101, uint32(c.B) * 0x101, 0xffff
}

// Hex parses "#rrggbb", "rrggbb" or "0xrrggbb", tolerating quotes.
func Hex(s string) (RGB, error) {
	s = strings.Trim(strings.TrimSpace(s), `"'`)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "#"), "0x")
	if len(s) < 6 {
		return RGB{}, fmt.Errorf("bad colour %q", s)
	}
	n, err := strconv.ParseUint(s[:6], 16, 32)
	if err != nil {
		return RGB{}, fmt.Errorf("bad colour %q", s)
	}
	return RGB{uint8(n >> 16), uint8(n >> 8), uint8(n)}, nil
}

// Mix blends c towards o by t (0..1), rounding each channel half away from
// zero as Rust's f32::round does.
func (c RGB) Mix(o RGB, t float64) RGB {
	l := func(a, b uint8) uint8 {
		return uint8(math.Round(float64(a) + (float64(b)-float64(a))*t))
	}
	return RGB{l(c.R, o.R), l(c.G, o.G), l(c.B, o.B)}
}

// Palette is the raw set of colours a theme is built from.
type Palette struct {
	BG, FG, Red, Green, Yellow, Blue, Magenta, Cyan, BrightBlack, Accent RGB
}

func mustPalette(hexes ...string) Palette {
	c := make([]RGB, len(hexes))
	for i, h := range hexes {
		v, err := Hex(h)
		if err != nil {
			panic(err)
		}
		c[i] = v
	}
	return Palette{c[0], c[1], c[2], c[3], c[4], c[5], c[6], c[7], c[8], c[9]}
}

var builtins = map[string]Palette{
	"tokyo-night": mustPalette("#1a1b26", "#c0caf5", "#f7768e", "#9ece6a", "#e0af68", "#7aa2f7", "#bb9af7", "#7dcfff", "#565f89", "#7aa2f7"),
	"catppuccin":  mustPalette("#1e1e2e", "#cdd6f4", "#f38ba8", "#a6e3a1", "#f9e2af", "#89b4fa", "#f5c2e7", "#94e2d5", "#6c7086", "#cba6f7"),
	"gruvbox":     mustPalette("#282828", "#ebdbb2", "#fb4934", "#b8bb26", "#fabd2f", "#83a598", "#d3869b", "#8ec07c", "#928374", "#fe8019"),
	"nord":        mustPalette("#2e3440", "#d8dee9", "#bf616a", "#a3be8c", "#ebcb8b", "#81a1c1", "#b48ead", "#88c0d0", "#616e88", "#88c0d0"),
	"everforest":  mustPalette("#2d353b", "#d3c6aa", "#e67e80", "#a7c080", "#dbbc7f", "#7fbbb3", "#d699b6", "#83c092", "#7a8478", "#a7c080"),
	"rose-pine":   mustPalette("#191724", "#e0def4", "#eb6f92", "#9ccfd8", "#f6c177", "#31748f", "#c4a7e7", "#ebbcba", "#6e6a86", "#c4a7e7"),
	"kanagawa":    mustPalette("#1f1f28", "#dcd7ba", "#e46876", "#98bb6c", "#e6c384", "#7e9cd8", "#957fb8", "#7fb4ca", "#727169", "#7e9cd8"),
	"matte-black": mustPalette("#121212", "#bebebe", "#d35f5f", "#8fa876", "#ffc107", "#e68e0d", "#b0a0c8", "#8a8a8d", "#5c5c5f", "#f59e0b"),
}

// Theme is a palette reduced to UI roles.
type Theme struct {
	Name                  string
	BG, FG, Muted, Border color.Color
	Surface, Accent       color.Color
	OK, Warn, Err, Info   color.Color
	Sev                   [6]color.Color
	Series                [6]color.Color
}

// Load returns the named built-in theme. "auto", "omarchy" and unknown
// names fall back to tokyo-night until Omarchy support lands.
func Load(name string) Theme {
	p, ok := builtins[name]
	if !ok {
		name = "tokyo-night"
		p = builtins[name]
	}
	return FromPalette(name, p)
}

// FromPalette derives the UI roles.
func FromPalette(name string, p Palette) Theme {
	return Theme{
		Name:    name,
		BG:      p.BG,
		FG:      p.FG,
		Muted:   p.FG.Mix(p.BG, 0.45),
		Border:  p.BrightBlack.Mix(p.BG, 0.25),
		Surface: p.BG.Mix(p.Accent, 0.16),
		Accent:  p.Accent,
		OK:      p.Green,
		Warn:    p.Yellow,
		Err:     p.Red,
		Info:    p.Blue,
		Sev: [6]color.Color{
			p.BrightBlack.Mix(p.FG, 0.35),
			p.Blue,
			p.Yellow,
			p.Yellow.Mix(p.Red, 0.4),
			p.Yellow.Mix(p.Red, 0.75),
			p.Red,
		},
		Series: [6]color.Color{p.Accent, p.Green, p.Magenta, p.Cyan, p.Yellow, p.Red},
	}
}

// Next returns the theme after current in cycling order.
func Next(current string) string {
	for i, n := range Builtin {
		if n == current {
			return Builtin[(i+1)%len(Builtin)]
		}
	}
	return Builtin[0]
}
