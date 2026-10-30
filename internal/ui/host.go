package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

// timeRange is a host page chart range.
type timeRange struct {
	secs  int64
	label string
}

var ranges = []timeRange{{15 * 60, "15m"}, {3600, "1h"}, {6 * 3600, "6h"}, {24 * 3600, "24h"}, {7 * 24 * 3600, "7d"}}

const (
	hostHeadH = 5
	sideW     = 36
)

// detailRow is the watched host in the latest snapshot.
func (m Model) detailRow() *model.HostRow {
	if m.snapshot == nil {
		return nil
	}
	for i := range m.snapshot.Hosts {
		if m.snapshot.Hosts[i].ID == m.detailHost {
			return &m.snapshot.Hosts[i]
		}
	}
	return nil
}

func (m Model) filteredItems() []*zabbix.Item {
	if m.detail == nil {
		return nil
	}
	return model.FilterItems(m.detail.Items, m.itemFilter)
}

func (m Model) hostView(w, h int) []string {
	t := &m.theme
	out := m.hostHead(w)
	bodyH := max(h-hostHeadH, 0)
	if m.detail == nil {
		return append(out, "", center(fg(t.Muted).Render("loading history…"), w))
	}
	if m.hostTab == tabItems {
		return append(out, m.itemsTable(w, bodyH)...)
	}
	return append(out, m.hostGraphs(w, bodyH)...)
}

func (m Model) hostHead(w int) []string {
	t := &m.theme
	o := panelOpts{title: title(t, "Host")}
	r := m.detailRow()
	if r == nil {
		return panel(t, w, hostHeadH, o, nil)
	}
	state, colour := hostState(t, r)
	if state == "maintenance" {
		state = "in maintenance"
	}
	muted := fg(t.Muted)
	sep := fg(t.Border).Render("  ·  ")
	base := lipgloss.NewStyle()
	line1 := statusDot(t, base, r) + " " + title(t, r.Name) + "  " + pill(state, t.BG, colour) + sep +
		muted.Render(r.Address) + sep + muted.Render(strings.Join(r.Groups, ", "))
	counts := problemCounts(t, base, r)
	inner := w - 2
	line1 = pad(line1, max(inner-lipgloss.Width(counts), 0)) + counts

	var facts strings.Builder
	if m.detail != nil {
		for _, f := range m.detail.Facts {
			facts.WriteString(muted.Render(f.Label+" ") + fg(t.FG).Render(truncate(f.Value, 48)) + "    ")
		}
	}
	tab := func(label string, on bool) string {
		if on {
			return lipgloss.NewStyle().Foreground(t.BG).Background(t.Accent).Bold(true).Render(" " + label + " ")
		}
		return muted.Render(" " + label + " ")
	}
	tabs := tab("Graphs", m.hostTab == tabGraphs) + " " + tab("Latest data", m.hostTab == tabItems) + "      " + muted.Render("range ")
	for i, r := range ranges {
		st := fg(t.Border)
		if i == m.rangeIdx {
			st = fg(t.Accent).Bold(true).Underline(true)
		}
		tabs += st.Render(r.label) + " "
	}
	return panel(t, w, hostHeadH, o, []string{line1, facts.String(), tabs})
}

func lastValue(points []model.Point) model.Metric {
	if len(points) == 0 {
		return model.Metric{}
	}
	return model.Some(points[len(points)-1].V)
}

func (m Model) hostGraphs(w, h int) []string {
	t := &m.theme
	d := m.detail
	chartsW := max(w-sideW, 40)
	rows := split(h, 50, 50)
	cols := split(chartsW, 50, 50)
	now := float64(m.now().Unix())
	x0 := now - float64(d.Range)
	r := ranges[m.rangeIdx].label

	chart := func(col, row int, name string, series []chartSeries, floor float64, y yAxis) []string {
		return timeChart(t, cols[col], rows[row], panelOpts{title: title(t, name+"  · "+r)}, series, x0, now, floor, y)
	}
	cpu := chart(0, 0, "CPU  "+fmtPct(lastValue(d.CPU)), []chartSeries{{"cpu", d.CPU, t.Series[0]}}, 5, yPercent)
	mem := chart(1, 0, "Memory  "+fmtPct(lastValue(d.Mem)), []chartSeries{{"mem", d.Mem, t.Series[2]}}, 5, yPercent)
	netName := fmt.Sprintf("Network  ↓ %s  ↑ %s", fmtMetric(lastValue(d.NetIn), fmtBits), fmtMetric(lastValue(d.NetOut), fmtBits))
	net := chart(0, 1, netName, []chartSeries{{"in", d.NetIn, t.Series[1]}, {"out", d.NetOut, t.Series[3]}}, 1000, yBits)
	load := chart(1, 1, "Load 1m  "+fmtMetric(lastValue(d.Load), fmtLoad), []chartSeries{{"load", d.Load, t.Series[4]}}, 1, yPlain)
	charts := append(joinRow(cpu, mem), joinRow(net, load)...)
	if w-chartsW < 10 {
		return charts
	}
	return joinRow(charts, m.hostSide(w-chartsW, h))
}

func (m Model) hostSide(w, h int) []string {
	t := &m.theme
	d := m.detail
	inner := w - 4
	disksH := min(max(len(d.Disks)*2+2, 4), h/2)
	var lines []string
	for _, disk := range d.Disks {
		used := model.Some(disk.Used)
		lines = append(lines,
			fg(t.FG).Render(pad(truncate(disk.Mount, max(inner-7, 1)), max(inner-6, 0)))+fg(level(t, disk.Used)).Render(fmt.Sprintf("%6s", fmtPct(used))),
			meter(t, lipgloss.NewStyle(), used, inner))
	}
	if len(lines) == 0 {
		lines = []string{fg(t.Muted).Render("no filesystems")}
	}
	out := panel(t, w, disksH, panelOpts{title: title(t, "Filesystems")}, lines)

	now := m.now()
	var mine []*model.Problem
	if m.snapshot != nil {
		for i := range m.snapshot.Problems {
			if p := &m.snapshot.Problems[i]; p.HostID == d.HostID {
				mine = append(mine, p)
			}
		}
	}
	lines = nil
	for _, p := range mine {
		lines = append(lines, severityBadge(t, p.Severity)+fg(t.Muted).Render(" "+fmtAge(p.Clock, now)+" ago"),
			fg(t.FG).Render(truncate(p.Name, inner)))
	}
	if len(lines) == 0 {
		lines = []string{fg(t.OK).Render("✓ no active problems")}
	}
	return append(out, panel(t, w, h-disksH, panelOpts{title: title(t, fmt.Sprintf("Problems %d", len(mine)))}, lines)...)
}

func (m Model) itemsTable(w, h int) []string {
	t := &m.theme
	items := m.filteredItems()
	now := m.now()
	name := fmt.Sprintf("Latest data  %d of %d", len(items), len(m.detail.Items))
	if m.itemFilter != "" {
		name += "  · /" + m.itemFilter
	}
	inner := w - 2
	tb := table{
		headers: []string{"Name", "Key", "Last value", "Updated", ""},
		widths:  []int{inner*34/100 - 2, inner * 30 / 100, inner * 20 / 100, 10, 8},
		marker:  true,
		gap:     true,
		n:       len(items),
		sel:     m.itemSel,
		scroll:  m.scroll.items,
		row: func(i int, base lipgloss.Style) []string {
			it := items[i]
			var clock int64
			fmt.Sscan(it.LastClock, &clock)
			age, at := "never", ""
			if clock > 0 {
				age, at = fmtAge(clock, now)+" ago", time.Unix(clock, 0).Format("15:04:05")
			}
			c := t.FG
			if it.Numeric() {
				c = t.Accent
			}
			return []string{
				base.Foreground(t.FG).Render(it.Name),
				base.Foreground(t.Border).Render(it.Key),
				base.Foreground(c).Bold(true).Render(fmtItemValue(*it, now)),
				base.Foreground(t.Muted).Render(age),
				base.Foreground(t.Border).Render(at),
			}
		},
	}
	return panel(t, w, h, panelOpts{title: title(t, name)}, tb.render(t, w-2, h-2))
}
