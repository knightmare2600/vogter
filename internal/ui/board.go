package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

const (
	// boardMinWidth is the narrowest canvas a page is drawn on. A smaller
	// terminal scrolls sideways across it.
	boardMinWidth = 120
	// boardMinUnit is the fewest terminal rows per grid row. A page that
	// does not fit at this height scrolls.
	boardMinUnit = 3
	// boardPanStep is how far one sideways key moves.
	boardPanStep = 8
	// topCol and topFirstCol size a top hosts table.
	topCol      = 8
	topFirstCol = 16
)

// placed is a widget's cell rectangle on the board.
type placed struct {
	index, x, y, w, h int
}

// boardGeom is a page laid out on its canvas, and the window onto it.
type boardGeom struct {
	cells      []placed
	canvasW    int
	canvasH    int
	viewW      int
	viewH      int
	vbar, hbar bool
}

func (g boardGeom) maxX() int { return max(g.canvasW-g.viewW, 0) }
func (g boardGeom) maxY() int { return max(g.canvasH-g.viewH, 0) }

func (m Model) openBoard() *model.Board {
	for i := range m.boards {
		if m.boards[i].ID == m.boardID {
			return &m.boards[i]
		}
	}
	return nil
}

func (m Model) boardPageNow() *model.BoardPage {
	if b := m.openBoard(); b != nil && m.boardPage < len(b.Pages) {
		return &b.Pages[m.boardPage]
	}
	return nil
}

// boardLayout maps a page onto a screen of w×h cells. The grid fills the
// screen when it fits. When it does not, it keeps Zabbix's layout on a larger
// canvas, and the screen becomes a scrolling window onto that.
func boardLayout(p *model.BoardPage, w, h int) boardGeom {
	rows := p.GridRows()
	g := boardGeom{viewW: w, viewH: h, hbar: w < boardMinWidth}
	if g.hbar {
		g.viewH = max(h-1, 1)
	}
	g.canvasH = max(g.viewH, rows*boardMinUnit)
	if g.vbar = g.canvasH > g.viewH; g.vbar {
		g.viewW = max(w-1, 1)
	}
	g.canvasW = max(g.viewW, boardMinWidth)
	col := func(n int) int { return n * g.canvasW / model.GridColumns }
	row := func(n int) int { return n * g.canvasH / rows }
	for i, wg := range p.Widgets {
		x, y := col(wg.X), row(wg.Y)
		g.cells = append(g.cells, placed{i, x, y, col(wg.X+wg.W) - x, row(wg.Y+wg.H) - y})
	}
	return g
}

func (m Model) boardsView(w, h int) []string {
	p := m.boardPageNow()
	if m.boardID == "" || p == nil {
		return m.boardList(w, h)
	}
	g := boardLayout(p, w, h)
	blank := strings.TrimRight(strings.Repeat(strings.Repeat(" ", g.canvasW)+"\n", g.canvasH), "\n")
	layers := []*lipgloss.Layer{lipgloss.NewLayer(blank)}
	for _, c := range g.cells {
		var d model.WidgetData
		if m.boardData != nil && m.boardData.BoardID == m.boardID && m.boardData.Page == m.boardPage && c.index < len(m.boardData.Widgets) {
			d = m.boardData.Widgets[c.index]
		}
		body := m.widget(p.Widgets[c.index], d, c.w, c.h)
		if len(body) > 0 {
			layers = append(layers, lipgloss.NewLayer(strings.Join(body, "\n")).X(c.x).Y(c.y))
		}
	}
	lines := strings.Split(lipgloss.NewCompositor(layers...).Render(), "\n")
	top, left := min(m.boardScroll, g.maxY()), min(m.boardX, g.maxX())
	lines = lines[top:min(top+g.viewH, len(lines))]
	var side []string
	if g.vbar {
		side = scrollbar(&m.theme, g.viewH, g.canvasH, top, "│")
	}
	for i, l := range lines {
		lines[i] = pad(ansi.Cut(l, left, left+g.viewW), g.viewW)
		if g.vbar {
			lines[i] += side[i]
		}
	}
	if g.hbar {
		lines = append(lines, strings.Join(scrollbar(&m.theme, g.viewW, g.canvasW, left, "─"), ""))
	}
	return lines
}

func (m Model) boardList(w, h int) []string {
	t := &m.theme
	o := panelOpts{title: title(t, "Zabbix dashboards")}
	if len(m.boards) == 0 {
		msg := "loading dashboards"
		if m.boardsLoaded {
			msg = "this account can see no dashboards"
		}
		return panel(t, w, h, o, []string{"", center(fg(t.Muted).Render(msg), w-2)})
	}
	tb := table{
		headers: []string{"Name", "Pages", "Widgets"},
		widths:  flexWidths(w-2, []int{0, 5, 7}, 10, 1),
		marker:  true,
		gap:     true,
		n:       len(m.boards),
		sel:     m.boardSel,
		scroll:  m.scroll.boards,
		row: func(i int, base lipgloss.Style) []string {
			b := m.boards[i]
			return []string{
				base.Foreground(t.FG).Bold(true).Render(b.Name),
				base.Foreground(t.Muted).Render(fmt.Sprint(len(b.Pages))),
				base.Foreground(t.Muted).Render(fmt.Sprint(b.Widgets())),
			}
		},
	}
	return panel(t, w, h, o, tb.render(t, w-2, h-2))
}

// widget draws one widget in exactly w×h cells.
func (m Model) widget(wg model.Widget, d model.WidgetData, w, h int) []string {
	t := &m.theme
	o := panelOpts{title: title(t, truncate(wg.Title(), max(w-6, 1)))}
	inner := max(w-2, 1)
	lines := func(l []string) []string { return panel(t, w, h, o, l) }
	if d.Err != "" {
		return lines([]string{fg(t.Err).Render(truncate(d.Err, inner))})
	}
	switch wg.Type {
	case "svggraph":
		return m.graphWidget(wg, d, w, h, o)
	case "problems":
		rows, hidden := m.problemLines(wg, inner, h-2)
		if hidden > 0 {
			o.bottom = fg(t.Muted).Render(fmt.Sprintf("+%d more", hidden))
		}
		return lines(rows)
	case "problemsbysv":
		return lines(m.severityLines(wg, inner))
	case "clock":
		return lines(m.clockLines(wg, inner, h-2))
	case "item":
		return lines(m.itemLines(wg, d, inner))
	case "hostavail":
		if d.Avail == nil {
			return lines(nil)
		}
		return lines(wrapChips([]string{
			fg(t.OK).Render("● ") + fg(t.FG).Render(fmt.Sprintf("%d available", d.Avail.Up)),
			fg(t.Err).Render("● ") + fg(t.FG).Render(fmt.Sprintf("%d not available", d.Avail.Down)),
			fg(t.Muted).Render(fmt.Sprintf("○ %d unknown", d.Avail.Unknown)),
		}, inner))
	case "systeminfo":
		var out []string
		for _, kv := range d.Info {
			out = append(out, fg(t.Muted).Render(pad(kv[0], 16))+fg(t.FG).Render(kv[1]))
		}
		return lines(out)
	case "tophosts":
		return lines(m.topLines(d, inner, h-2))
	}
	return lines([]string{
		center(fg(t.Muted).Render(wg.Type+" widget"), inner),
		center(fg(t.Border).Render("not drawn in a terminal · o opens Zabbix"), inner),
	})
}

func (m Model) graphWidget(wg model.Widget, d model.WidgetData, w, h int, o panelOpts) []string {
	t := &m.theme
	series := make([]chartSeries, len(d.Series))
	for i, s := range d.Series {
		series[i] = chartSeries{truncate(s.Name, 24), s.Points, t.Series[i%len(t.Series)]}
	}
	y := yPlain
	if len(d.Series) > 0 {
		switch d.Series[0].Units {
		case "%":
			y = yPercent
		case "bps":
			y = yBits
		case "B":
			y = yBytes
		}
	}
	if d.Dropped > 0 {
		o.bottom = fg(t.Muted).Render(fmt.Sprintf("+%d more series", d.Dropped))
	}
	floor := 1.0
	if v, err := strconv.ParseFloat(wg.Get("lefty_max"), 64); err == nil && v > 0 {
		floor = v
	}
	now := float64(m.now().Unix())
	return timeChart(t, w, h, o, series, now-float64(max(d.Range, 60)), now, floor, y)
}

// problemLines returns the rows that fit and how many did not.
func (m Model) problemLines(wg model.Widget, w, h int) ([]string, int) {
	t := &m.theme
	if m.snapshot == nil {
		return nil, 0
	}
	list := model.WidgetProblems(wg, m.snapshot.Problems)
	if len(list) == 0 {
		return []string{fg(t.OK).Render("✓ ") + fg(t.Muted).Render("no active problems")}, 0
	}
	now := m.now()
	var out []string
	for _, p := range list[:min(len(list), max(h, 0))] {
		name := p.Name
		if p.Host != "" {
			name = p.Host + " · " + name
		}
		age := fmt.Sprintf(" %7s ", fmtAge(p.Clock, now))
		out = append(out, severityBadge(t, p.Severity)+fg(t.Muted).Render(age)+fg(t.FG).Render(truncate(name, max(w-16, 1))))
	}
	return out, len(list) - len(out)
}

func (m Model) severityLines(wg model.Widget, w int) []string {
	t := &m.theme
	if m.snapshot == nil {
		return nil
	}
	var counts [6]int
	for _, p := range model.WidgetProblems(wg, m.snapshot.Problems) {
		counts[p.Severity]++
	}
	var chips []string
	for s := model.Disaster; s >= model.NotClassified; s-- {
		chips = append(chips, fg(t.Sev[s]).Render("■ ")+fg(t.FG).Render(fmt.Sprintf("%s %d", s.Label(), counts[s])))
	}
	return wrapChips(chips, w)
}

// wrapChips joins chips into as many lines as w needs.
func wrapChips(chips []string, w int) []string {
	var out []string
	line := ""
	for _, c := range chips {
		if line != "" && lipgloss.Width(line)+3+lipgloss.Width(c) > w {
			out = append(out, line)
			line = ""
		}
		if line != "" {
			line += "   "
		}
		line += c
	}
	return append(out, line)
}

func (m Model) clockLines(wg model.Widget, w, h int) []string {
	t := &m.theme
	layout := "15:04"
	if wg.Get("time_sec") == "1" {
		layout = "15:04:05"
	}
	text := m.now().Format(layout)
	big := bigText(t, text, "", t.Accent)
	if h < 3 || lipgloss.Width(big[0]) > w {
		return []string{center(fg(t.FG).Bold(true).Render(text), w)}
	}
	var out []string
	for range (h - 3) / 2 {
		out = append(out, "")
	}
	for _, l := range big {
		out = append(out, center(l, w))
	}
	return out
}

func (m Model) itemLines(wg model.Widget, d model.WidgetData, w int) []string {
	t := &m.theme
	if len(d.Items) == 0 {
		return []string{fg(t.Muted).Render("no data")}
	}
	it := d.Items[0]
	host := ""
	for _, h := range m.snapshotHosts() {
		if h.ID == it.HostID {
			host = h.Name
		}
	}
	desc := wg.Get("description")
	if desc == "" {
		desc = it.Name
	}
	desc = strings.NewReplacer("{HOST.NAME}", host, "{HOST.HOST}", host, "{ITEM.NAME}", it.Name, "\r", "", "\n", " · ").Replace(desc)
	return []string{
		center(fg(t.Accent).Bold(true).Render(fmtItemValue(it, m.now())), w),
		center(fg(t.Muted).Render(truncate(desc, w)), w),
	}
}

func (m Model) snapshotHosts() []model.HostRow {
	if m.snapshot == nil {
		return nil
	}
	return m.snapshot.Hosts
}

func (m Model) topLines(d model.WidgetData, w, h int) []string {
	t := &m.theme
	if d.Top == nil {
		return nil
	}
	if len(d.Top.Rows) == 0 {
		return []string{fg(t.Muted).Render("no data")}
	}
	// Columns that do not fit are left out, last first; o shows them all.
	columns := d.Top.Columns
	for len(columns) > 1 && topFirstCol+(topCol+2)*(len(columns)-1) > w {
		columns = columns[:len(columns)-1]
	}
	cols := make([]int, len(columns))
	for i := range cols {
		cols[i] = topCol
	}
	cols[0] = 0
	widths := flexWidths(w, cols, topFirstCol, 0)
	row := func(cells []string, style func(int) lipgloss.Style) string {
		var b strings.Builder
		for i, c := range cells {
			b.WriteString(style(i).Render(pad(truncate(c, widths[i]), widths[i])) + "  ")
		}
		return b.String()
	}
	out := []string{row(cells(columns), func(int) lipgloss.Style { return fg(t.Muted) })}
	for _, r := range d.Top.Rows[:min(len(d.Top.Rows), max(h-1, 0))] {
		vals := make([]string, len(columns))
		colours := make([]lipgloss.Style, len(vals))
		for i, c := range columns {
			colours[i] = fg(t.FG)
			switch {
			case c.Host:
				vals[i] = r.Host
			case r.Cells[i] == nil:
				vals[i] = "-"
			default:
				vals[i] = fmtItemValue(*r.Cells[i], m.now())
				colours[i] = fg(itemColour(m, r.Cells[i]))
			}
		}
		out = append(out, row(vals, func(i int) lipgloss.Style { return colours[i] }))
	}
	return out
}

func cells(columns []model.TopColumn) []string {
	out := make([]string, len(columns))
	for i, c := range columns {
		out[i] = c.Name
	}
	return out
}

// itemColour grades percentages like the rest of the app.
func itemColour(m Model, it *zabbix.Item) color.Color {
	if v, ok := it.Value(); ok && it.Units == "%" {
		return level(&m.theme, v)
	}
	return m.theme.FG
}
