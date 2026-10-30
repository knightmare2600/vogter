package ui

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/theme"
)

type yAxis int

const (
	yPercent yAxis = iota
	yBits
	yPlain
)

func (y yAxis) label(v float64) string {
	switch y {
	case yPercent:
		if v < 10 && v != math.Trunc(v) {
			return fmt.Sprintf("%.1f%%", v)
		}
		return fmt.Sprintf("%.0f%%", v)
	case yBits:
		return fmtBits(v)
	}
	return fmtNumber(math.Round(v*100) / 100)
}

type chartSeries struct {
	name   string
	points []model.Point
	colour color.Color
}

// niceCeil rounds up to 1, 2, 2.5 or 5 × 10ⁿ so axis labels stay readable.
func niceCeil(v float64) float64 {
	if v <= 0 {
		return 1
	}
	mag := math.Pow(10, math.Floor(math.Log10(v)))
	n := v / mag
	for _, s := range []float64{1, 2, 2.5, 5} {
		if n <= s {
			return s * mag
		}
	}
	return 10 * mag
}

// chartTop is the Y axis maximum. floor is the minimum range, so flat
// near-zero series are not stretched into noise. Percent charts need no
// headroom above 100%.
func chartTop(series []chartSeries, floor float64, y yAxis) float64 {
	peak := 0.0
	for _, s := range series {
		for _, p := range s.points {
			peak = max(peak, p.V)
		}
	}
	top := niceCeil(max(peak*1.15, floor))
	if y == yPercent && peak <= 100 {
		top = min(top, 100)
	}
	return top
}

// braille dot bit for a position within a 2×4 cell.
var dotBits = [4][2]rune{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}

type canvas struct {
	w, h   int // cells
	dots   []rune
	colour []color.Color
}

func newCanvas(w, h int) *canvas {
	return &canvas{w: w, h: h, dots: make([]rune, w*h), colour: make([]color.Color, w*h)}
}

func (c *canvas) set(x, y int, col color.Color) {
	if x < 0 || y < 0 || x >= c.w*2 || y >= c.h*4 {
		return
	}
	i := y/4*c.w + x/2
	c.dots[i] |= dotBits[y%4][x%2]
	c.colour[i] = col
}

// line draws from (x0,y0) to (x1,y1) with Bresenham's algorithm.
func (c *canvas) line(x0, y0, x1, y1 int, col color.Color) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	e := dx + dy
	for {
		c.set(x0, y0, col)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * e
		if e2 >= dy {
			e += dy
			x0 += sx
		}
		if e2 <= dx {
			e += dx
			y0 += sy
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (c *canvas) plot(s chartSeries, x0, x1, top float64) {
	px := func(t float64) int { return int(math.Round((t - x0) / (x1 - x0) * float64(c.w*2-1))) }
	py := func(v float64) int { return int(math.Round((1 - v/top) * float64(c.h*4-1))) }
	prevX, prevY, have := 0, 0, false
	for _, p := range s.points {
		if p.T < x0 || p.T > x1 {
			continue
		}
		x, y := px(p.T), py(p.V)
		if have {
			c.line(prevX, prevY, x, y, s.colour)
		} else {
			c.set(x, y, s.colour)
		}
		prevX, prevY, have = x, y, true
	}
}

// timeChart draws a braille line chart inside a panel of exactly w×h cells.
// A legend appears top right when there are several series and it fits.
func timeChart(t *theme.Theme, w, h int, o panelOpts, series []chartSeries, x0, x1, floor float64, y yAxis) []string {
	has := false
	for _, s := range series {
		has = has || len(s.points) > 0
	}
	if !has {
		return panel(t, w, h, o, []string{"", center(fg(t.Muted).Render("no data yet"), w-2)})
	}
	top := chartTop(series, floor, y)
	labels := [3]string{y.label(top), y.label(top / 2), y.label(0)}
	lw := max(lipgloss.Width(labels[0]), lipgloss.Width(labels[1]), lipgloss.Width(labels[2]))
	axis := fg(t.Border)
	muted := fg(t.Muted)

	innerW, innerH := w-2, h-2
	plotW, plotH := innerW-lw-1, innerH-2
	if plotW < 4 || plotH < 2 {
		return panel(t, w, h, o, nil)
	}
	c := newCanvas(plotW, plotH)
	for _, s := range series {
		c.plot(s, x0, x1, top)
	}
	// A boxed legend, as long as it covers under 60% of the plot each way.
	legendW := 0
	var legend []string
	if len(series) > 1 {
		for _, s := range series {
			legendW = max(legendW, len([]rune(s.name)))
		}
		if (legendW+2)*100 < plotW*60 && (len(series)+2)*100 < plotH*60 {
			legend = append(legend, axis.Render("┌"+strings.Repeat("─", legendW)+"┐"))
			for _, s := range series {
				legend = append(legend, axis.Render("│")+fg(s.colour).Render(pad(s.name, legendW))+axis.Render("│"))
			}
			legend = append(legend, axis.Render("└"+strings.Repeat("─", legendW)+"┘"))
		}
	}

	lines := make([]string, 0, innerH)
	for row := 0; row < plotH; row++ {
		var b strings.Builder
		switch row {
		case 0:
			b.WriteString(muted.Render(pad(labels[0], lw)))
		case plotH / 2:
			b.WriteString(muted.Render(pad(labels[1], lw)))
		case plotH - 1:
			b.WriteString(muted.Render(pad(labels[2], lw)))
		default:
			b.WriteString(strings.Repeat(" ", lw))
		}
		b.WriteString(axis.Render("│"))
		end := plotW
		if row < len(legend) {
			end = plotW - legendW - 2
		}
		for col := 0; col < end; col++ {
			i := row*plotW + col
			if c.dots[i] == 0 {
				b.WriteByte(' ')
				continue
			}
			b.WriteString(fg(c.colour[i]).Render(string(0x2800 + c.dots[i])))
		}
		if row < len(legend) {
			b.WriteString(legend[row])
		}
		lines = append(lines, b.String())
	}
	lines = append(lines, strings.Repeat(" ", lw)+axis.Render("└"+strings.Repeat("─", plotW)))
	left, mid, right := fmtHHMM(x0), fmtHHMM((x0+x1)/2), "now"
	row := []rune(strings.Repeat(" ", innerW))
	copy(row, []rune(left))
	copy(row[max(lw+1+plotW/2-len(mid)/2, 0):], []rune(mid))
	copy(row[max(innerW-len(right), 0):], []rune(right))
	lines = append(lines, muted.Render(string(row)))
	return panel(t, w, h, o, lines)
}
