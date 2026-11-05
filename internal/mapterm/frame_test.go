package mapterm

import (
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func canvasWith(draw func(c *Canvas)) *Canvas {
	c := NewCanvas(6, 3)
	draw(c)
	return c
}

func TestShape(t *testing.T) {
	cases := map[direction]func(c *Canvas){
		dirHorizontal: func(c *Canvas) { c.Line(0, 1, 11, 1, ClassMajorRoad) },
		dirVertical:   func(c *Canvas) { c.Line(0, 0, 0, 11, ClassMajorRoad) },
		dirFalling:    func(c *Canvas) { c.Line(0, 0, 11, 11, ClassMajorRoad) },
		dirRising:     func(c *Canvas) { c.Line(0, 11, 11, 0, ClassMajorRoad) },
	}
	for want, draw := range cases {
		c := canvasWith(draw)
		d, _ := c.Cell(0, 0)
		if want == dirRising {
			d, _ = c.Cell(0, 2)
		}
		if got := shape(d); got != want {
			t.Errorf("want %v, got %v for %08b", want, got, d)
		}
	}
	if shape(0x01) != dirDot {
		t.Error("one dot is a dot")
	}
}

func TestComposeModes(t *testing.T) {
	c := canvasWith(func(c *Canvas) {
		c.Line(0, 1, 11, 1, ClassMotorway)
		c.Line(0, 6, 11, 6, ClassMajorRoad)
		c.Fill([][]FPt{{{0, 8}, {12, 8}, {12, 12}, {0, 12}, {0, 8}}}, ClassWater, nil)
	})
	uni := Compose(c, ModeUnicode)
	if r := uni.At(2, 0).R; r != '═' {
		t.Errorf("motorway %q", r)
	}
	if r := uni.At(2, 1).R; r != '─' {
		t.Errorf("road %q", r)
	}
	if r := uni.At(2, 2).R; r != '≈' {
		t.Errorf("water %q", r)
	}
	asc := Compose(c, ModeASCII)
	if r := asc.At(2, 0).R; r != '=' {
		t.Errorf("ascii motorway %q", r)
	}
	for _, cell := range asc.Cells {
		if cell.R > 0x7e {
			t.Fatalf("ASCII mode drew %q", cell.R)
		}
	}
	br := Compose(c, ModeBraille)
	if r := br.At(2, 0).R; r < 0x2800 || r > 0x28ff {
		t.Errorf("braille %q", r)
	}
	if r := br.At(0, 0).R; r == ' ' {
		t.Error("lit cells must not be blank")
	}
}

func TestParseMode(t *testing.T) {
	if m, ok := ParseMode("ASCII"); !ok || m != ModeASCII {
		t.Error(m)
	}
	if m, ok := ParseMode("sixel"); ok || m != ModeBraille {
		t.Error("unknown falls back to braille")
	}
}

func TestPatternsStayOnTheGround(t *testing.T) {
	// The same area, seen from views a whole cell apart, lights the same dots.
	// Each view fills its whole canvas; on the ground both cover, the lit
	// dots must be exactly the same.
	area := func(v View) (map[[2]int]bool, int, int) {
		c := NewCanvas(40, 20)
		ox, oy := v.Origin()
		c.FillAt([][]FPt{{{-1, -1}, {81, -1}, {81, 81}, {-1, 81}, {-1, -1}}}, ClassWater, sparse, ox, oy)
		out := map[[2]int]bool{}
		for p := range lit(c) {
			out[[2]int{p[0] + ox, p[1] + oy}] = true
		}
		return out, ox, oy
	}
	a := View{Center: odense, Zoom: 15, W: 80, H: 80}
	b := a
	b.Center = Unproject(a.FromDot(48, 40))
	pa, ax, ay := area(a)
	pb, bx, by := area(b)
	checked := 0
	for x := max(ax, bx); x < min(ax, bx)+80; x++ {
		for y := max(ay, by); y < min(ay, by)+80; y++ {
			if pa[[2]int{x, y}] != pb[[2]int{x, y}] {
				t.Fatalf("dot %d,%d differs between views", x, y)
			}
			checked++
		}
	}
	if checked < 1000 || bx == ax {
		t.Fatalf("the views must overlap but differ: %d dots, %d vs %d", checked, ax, bx)
	}
}

func TestASCIIText(t *testing.T) {
	for in, want := range map[string]string{
		"København": "Koebenhavn", "Ærø": "Aeroe", "Århus": "Aarhus", "Straße": "Strasse", "Café": "Cafe", "東京": "??",
	} {
		if got := ASCIIText(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}

func blankFrame(cols, rows int) *Frame {
	f := &Frame{Cols: cols, Rows: rows, Cells: make([]Cell, cols*rows)}
	for i := range f.Cells {
		f.Cells[i].R = ' '
	}
	return f
}

func row(f *Frame, r int) string {
	var b strings.Builder
	for c := range f.Cols {
		b.WriteRune(f.At(c, r).R)
	}
	return b.String()
}

func TestPlaceLabels(t *testing.T) {
	f := blankFrame(30, 4)
	n := PlaceLabels(f, []Label{
		{Text: "Odense", X: 30, Y: 5, Rank: 3},
		{Text: "Vestergade", X: 30, Y: 5, Rank: 14},
		{Text: "Kanten", X: 1, Y: 9, Rank: 4},
		{Text: "Højby", X: 40, Y: 13, Rank: 6},
	}, false)
	if n != 2 {
		t.Errorf("placed %d", n)
	}
	if !strings.Contains(row(f, 1), "Odense") || strings.Contains(row(f, 1), "Vestergade") {
		t.Errorf("the more important label wins: %q", row(f, 1))
	}
	if strings.Contains(row(f, 2), "Kanten") {
		t.Error("a label running off the edge is dropped")
	}
	if !strings.Contains(row(f, 3), "Højby") {
		t.Errorf("%q", row(f, 3))
	}
	if !f.At(13, 1).Label {
		t.Error("label cells are marked")
	}
	g := blankFrame(30, 2)
	PlaceLabels(g, []Label{{Text: "Højby", X: 30, Y: 1, Rank: 6}}, true)
	if !strings.Contains(row(g, 0), "Hoejby") {
		t.Errorf("ascii label %q", row(g, 0))
	}
	dup := blankFrame(60, 2)
	if n := PlaceLabels(dup, []Label{{Text: "A road", X: 20, Y: 1, Rank: 14}, {Text: "A road", X: 60, Y: 1, Rank: 14}}, false); n != 1 {
		t.Errorf("a repeated nearby name is placed once: %d", n)
	}
}

func TestLinesGroupRuns(t *testing.T) {
	f := blankFrame(4, 1)
	for i := range 4 {
		f.Cells[i] = Cell{R: 'x', Class: ClassMajorRoad}
	}
	out := f.Lines(Palettes[0])
	if ansi.Strip(out[0]) != "xxxx" {
		t.Errorf("%q", out[0])
	}
	if n := strings.Count(out[0], "\x1b["); n > 2 {
		t.Errorf("one run needs one style, got %d escapes", n)
	}
}

func TestPaletteByName(t *testing.T) {
	if p, ok := PaletteByName("amber", false); ok || p.Name != "green" {
		t.Error("extras stay locked until allowed")
	}
	if p, ok := PaletteByName("AMBER", true); !ok || p.Name != "amber" {
		t.Error(p.Name)
	}
}

// Contrast targets, from WCAG 2.1: text needs 4.5:1, and graphics such as
// roads 3:1. Background textures may be softer.
func TestPaletteContrast(t *testing.T) {
	important := []Class{ClassMotorway, ClassMajorRoad, ClassMinorRoad, ClassRail, ClassWaterway, ClassWater}
	for _, p := range append(append([]Palette{}, Palettes...), ExtraPalettes...) {
		bg := p.Background
		if bg == nil {
			bg = color.Black
		}
		if r := Contrast(p.Label, bg); r < 4.5 {
			t.Errorf("%s: labels %.2f:1", p.Name, r)
		}
		for s, c := range p.Status {
			if c == nil || Contrast(c, bg) < 3 {
				t.Errorf("%s: status %d marker needs 3:1", p.Name, s)
			}
		}
		for _, c := range important {
			if r := Contrast(p.Classes[c], bg); r < 3 {
				t.Errorf("%s: class %d %.2f:1", p.Name, c, r)
			}
		}
	}
}
