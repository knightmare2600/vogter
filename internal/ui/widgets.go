package ui

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/theme"
)

// Every panel and table in Vogter is built from these helpers. Lip Gloss
// has no border titles, so panels draw their own frame.

func fg(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

// pad clips or fills a styled string to exactly w cells.
func pad(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) > w {
		s = ansi.Truncate(s, w, "")
	}
	return s + strings.Repeat(" ", w-lipgloss.Width(s))
}

func center(s string, w int) string {
	// Same rounding as ratatui, so centred text lines up with zabterm's.
	left := max(w/2-lipgloss.Width(s)/2, 0)
	return pad(strings.Repeat(" ", left)+s, w)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:max(n-1, 0)]) + "…"
}

type panelOpts struct {
	title, bottom string
	border        color.Color
}

// panel frames lines in a rounded border of exactly w×h cells. Titles are
// already styled. The result is a slice of h lines.
func panel(t *theme.Theme, w, h int, o panelOpts, lines []string) []string {
	if w < 2 || h < 2 {
		return nil
	}
	if o.border == nil {
		o.border = t.Border
	}
	b := fg(o.border)
	rule := func(left, title, right string) string {
		if title == "" {
			return b.Render(left + strings.Repeat("─", w-2) + right)
		}
		title = pad(" "+title+" ", min(lipgloss.Width(title)+2, w-2))
		return b.Render(left) + title + b.Render(strings.Repeat("─", max(w-2-lipgloss.Width(title), 0))+right)
	}
	out := make([]string, 0, h)
	out = append(out, rule("╭", o.title, "╮"))
	for i := 0; i < h-2; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out = append(out, b.Render("│")+pad(line, w-2)+b.Render("│"))
	}
	return append(out, rule("╰", o.bottom, "╯"))
}

func title(t *theme.Theme, s string) string { return fg(t.FG).Bold(true).Render(s) }

// joinRow places blocks of equal height side by side.
func joinRow(blocks ...[]string) []string {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]string, len(blocks[0]))
	for _, b := range blocks {
		for i := range out {
			if i < len(b) {
				out[i] += b[i]
			}
		}
	}
	return out
}

// split divides w by percentages, rounding each boundary as ratatui does.
// The last part takes whatever is left.
func split(w int, pcts ...int) []int {
	out := make([]int, len(pcts))
	cum, prev := 0, 0
	for i, p := range pcts {
		cum += p
		edge := (w*cum + 50) / 100
		if i == len(pcts)-1 {
			edge = w
		}
		out[i] = edge - prev
		prev = edge
	}
	return out
}

// level picks ok, warn or err for a percentage.
func level(t *theme.Theme, pct float64) color.Color {
	switch {
	case pct >= 90:
		return t.Err
	case pct >= 70:
		return t.Warn
	}
	return t.OK
}

// meter is a ━━━━──── bar for a percentage, on the base style of its row.
func meter(t *theme.Theme, base lipgloss.Style, m model.Metric, w int) string {
	if !m.Set {
		return base.Foreground(t.Border).Render(strings.Repeat("─", w))
	}
	filled := int(math.Round(min(max(m.Value, 0), 100) / 100 * float64(w)))
	if m.Value > 0 {
		filled = max(filled, 1)
	}
	return base.Foreground(level(t, m.Value)).Render(strings.Repeat("━", filled)) +
		base.Foreground(t.Border).Render(strings.Repeat("─", w-filled))
}

func severityBadge(t *theme.Theme, s model.Severity) string {
	return lipgloss.NewStyle().Background(t.Sev[s]).Foreground(t.BG).Bold(true).Render(fmt.Sprintf(" %-4s ", s.Short()))
}

func pill(s string, fgc, bg color.Color) string {
	return lipgloss.NewStyle().Foreground(fgc).Background(bg).Render(" " + s + " ")
}

func statusDot(t *theme.Theme, base lipgloss.Style, h *model.HostRow) string {
	switch {
	case !h.Enabled:
		return base.Foreground(t.Muted).Render("✕")
	case h.Maintenance:
		return base.Foreground(t.Info).Render("◆")
	case h.Availability == model.Up:
		return base.Foreground(t.OK).Render("●")
	case h.Availability == model.Down:
		return base.Foreground(t.Err).Render("●")
	}
	return base.Foreground(t.Muted).Render("○")
}

// hostName is struck through when disabled and red when unreachable.
func hostName(t *theme.Theme, base lipgloss.Style, h *model.HostRow, w int) string {
	s := base.Foreground(t.FG).Bold(true)
	switch {
	case !h.Enabled:
		s = base.Foreground(t.Muted).Strikethrough(true)
	case h.Availability == model.Down:
		s = base.Foreground(t.Err).Bold(true)
	}
	return s.Render(truncate(h.Name, w))
}

// valueFg mutes metrics of hosts whose values are stale.
func valueFg(t *theme.Theme, h *model.HostRow) color.Color {
	if h.Availability == model.Down || !h.Enabled {
		return t.Muted
	}
	return t.FG
}

// problemCounts shows one coloured counter per severity, or ✓.
func problemCounts(t *theme.Theme, base lipgloss.Style, h *model.HostRow) string {
	if h.ProblemCount() == 0 {
		return base.Foreground(t.OK).Render("✓")
	}
	var b strings.Builder
	for s := model.Disaster; s >= model.NotClassified; s-- {
		if n := h.Problems[s]; n > 0 {
			b.WriteString(lipgloss.NewStyle().Background(t.Sev[s]).Foreground(t.BG).Bold(true).Render(fmt.Sprintf(" %d ", n)))
			b.WriteString(base.Render(" "))
		}
	}
	return b.String()
}

var ticks = []rune("▁▂▃▄▅▆▇█")

// sparkline shows the last w samples, scaled to the window's own range (at
// least 8 points wide) so the shape shows at any utilisation.
func sparkline(t *theme.Theme, base lipgloss.Style, points []model.Point, w int) string {
	if len(points) == 0 {
		return base.Foreground(t.Border).Render(strings.Repeat("·", w))
	}
	tail := points[max(len(points)-w, 0):]
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, p := range tail {
		lo, hi = min(lo, p.V), max(hi, p.V)
	}
	span := max(hi-lo, 8)
	floor := max(lo-(span-(hi-lo))/2, 0)
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", w-len(tail)))
	for _, p := range tail {
		i := math.Round((p.V - floor) / span * 7)
		b.WriteRune(ticks[int(min(max(i, 0), 7))])
	}
	c := t.Accent
	if last := tail[len(tail)-1].V; last >= 70 {
		c = level(t, last)
	}
	return base.Foreground(c).Render(b.String())
}

var digits = [10][3]string{
	{"█▀█", "█ █", "▀▀▀"},
	{"▀█ ", " █ ", "▀▀▀"},
	{"▀▀█", "█▀▀", "▀▀▀"},
	{"▀▀█", " ▀█", "▀▀▀"},
	{"█ █", "▀▀█", "  ▀"},
	{"█▀▀", "▀▀█", "▀▀▀"},
	{"█▀▀", "█▀█", "▀▀▀"},
	{"▀▀█", "  █", "  ▀"},
	{"█▀█", "█▀█", "▀▀▀"},
	{"█▀█", "▀▀█", "▀▀▀"},
}

// bigText renders digits, '.' and '-' in the 3-row font. suffix is drawn
// small on the baseline, and balanced so the digits stay centred.
func bigText(t *theme.Theme, text, suffix string, c color.Color) []string {
	var rows [3]strings.Builder
	for i, ch := range text {
		g := [3]string{" ", " ", " "}
		switch {
		case ch >= '0' && ch <= '9':
			g = digits[ch-'0']
		case ch == '.':
			g = [3]string{" ", " ", "▀"}
		case ch == '-':
			g = [3]string{"   ", "▀▀▀", "   "}
		case ch == ':':
			g = [3]string{" ", "▀", "▀"}
		}
		for r := range rows {
			if i > 0 {
				rows[r].WriteByte(' ')
			}
			rows[r].WriteString(g[r])
		}
	}
	margin := strings.Repeat(" ", len([]rune(suffix))+1)
	out := make([]string, 3)
	for r := range rows {
		tail := margin
		if r == 2 {
			tail = " " + suffix
		}
		out[r] = margin + fg(c).Bold(true).Render(rows[r].String()) + fg(t.Muted).Render(tail)
	}
	return out
}

// scroller keeps a table's first visible row between frames, the way
// ratatui's TableState does: the view only moves when the selection would
// leave it.
type scroller struct{ offset int }

func (s *scroller) window(sel, h, n int) (int, int) {
	if h <= 0 {
		return 0, 0
	}
	s.offset = min(s.offset, max(n-h, 0))
	if sel < s.offset {
		s.offset = sel
	}
	if sel >= s.offset+h {
		s.offset = sel - h + 1
	}
	return s.offset, min(s.offset+h, n)
}

type table struct {
	headers []string
	widths  []int
	// marker reserves a column for the selection bar.
	marker bool
	// gap adds an empty row under the header.
	gap    bool
	n, sel int
	scroll *scroller
	// row returns the cells of row i, styled on base so a selected row
	// keeps one background behind every cell.
	row func(i int, base lipgloss.Style) []string
}

// render returns the table's lines, at most h of them and w cells wide.
func (tb table) render(t *theme.Theme, w, h int) []string {
	const spacing = 2
	// A list that does not fit gives up its last column for a scrollbar.
	rows := h - 1
	if tb.gap {
		rows--
	}
	bar := rows > 0 && tb.n > rows
	if bar {
		w--
	}
	line := func(cells []string, base lipgloss.Style, mark string) string {
		var b strings.Builder
		if tb.marker {
			b.WriteString(mark)
		}
		sep := base.Render(strings.Repeat(" ", spacing))
		for i, c := range cells {
			if i > 0 {
				b.WriteString(sep)
			}
			fill := max(tb.widths[i]-lipgloss.Width(c), 0)
			if lipgloss.Width(c) > tb.widths[i] {
				c = ansi.Truncate(c, tb.widths[i], "")
				fill = tb.widths[i] - lipgloss.Width(c)
			}
			b.WriteString(c + base.Render(strings.Repeat(" ", fill)))
		}
		used := lipgloss.Width(b.String())
		if used < w {
			b.WriteString(base.Render(strings.Repeat(" ", w-used)))
		}
		return ansi.Truncate(b.String(), w, "")
	}

	head := make([]string, len(tb.headers))
	for i, s := range tb.headers {
		head[i] = fg(t.Muted).Render(s)
	}
	out := []string{line(head, lipgloss.NewStyle(), " ")}
	if tb.gap {
		out = append(out, "")
	}
	first, last := tb.scroll.window(tb.sel, h-len(out), tb.n)
	var side []string
	if bar {
		side = scrollbar(t, rows, tb.n, first, "│")
	}
	for i := first; i < last; i++ {
		base, mark := lipgloss.NewStyle(), " "
		if i == tb.sel {
			base = base.Background(t.Surface)
			mark = fg(t.Accent).Render("▌")
		}
		l := line(tb.row(i, base), base, mark)
		if bar {
			l += side[i-first]
		}
		out = append(out, l)
	}
	return out
}

// scrollThumb places a scrollbar's thumb on a track of the given length.
func scrollThumb(track, total, offset int) (start, size int) {
	size = min(max(track*track/max(total, 1), 1), track)
	return offset * (track - size) / max(total-track, 1), size
}

// scrollbar returns one styled cell per track position.
func scrollbar(t *theme.Theme, track, total, offset int, rail string) []string {
	start, size := scrollThumb(track, total, offset)
	out := make([]string, track)
	for i := range out {
		if i >= start && i < start+size {
			out[i] = fg(t.Accent).Render("█")
		} else {
			out[i] = fg(t.Border).Render(rail)
		}
	}
	return out
}

// flexWidths gives the columns marked 0 the space the fixed ones leave,
// sharing it evenly, with at least minW each. spacing is the gap between
// columns and extra any width reserved besides them.
func flexWidths(total int, cols []int, minW, extra int) []int {
	const spacing = 2
	used, flex := extra+spacing*(len(cols)-1), 0
	for _, c := range cols {
		used += c
		if c == 0 {
			flex++
		}
	}
	out := append([]int(nil), cols...)
	if flex == 0 {
		return out
	}
	share := max((total-used)/flex, minW)
	spare := max(total-used-share*flex, 0)
	for i, c := range out {
		if c == 0 {
			out[i] = share + spare
			spare = 0
		}
	}
	return out
}
