package ui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/knightmare2600/vogter/internal/model"
)

// wideHosts is the width from which the dashboard table shows every column.
const wideHosts = 130

func (m Model) dashboard(w, h int) []string {
	hostRows := min(max(len(m.snapshot.Hosts)+3, 5), max(h/3, 5))
	midH := max(h-7-4-hostRows, 8)
	st := model.Summarise(m.snapshot)
	out := m.tiles(w, st)
	out = append(out, m.severityPanel(w, st)...)
	cols := split(w, 62, 38)
	out = append(out, joinRow(m.fleetChart(cols[0], midH), m.feed(cols[1], midH))...)
	return append(out, m.dashHosts(w, hostRows)...)
}

func (m Model) tile(w int, name string, big []string, sub string) []string {
	lines := make([]string, 0, 5)
	for _, l := range big {
		lines = append(lines, center(l, w-2))
	}
	lines = append(lines, "", center(sub, w-2))
	return panel(&m.theme, w, 7, panelOpts{title: title(&m.theme, name)}, lines)
}

// tilePct shows one decimal below 10, so small values do not read as 0.
func tilePct(v model.Metric) string {
	switch {
	case !v.Set:
		return "-"
	case v.Value < 10:
		return fmt.Sprintf("%.1f", v.Value)
	}
	return fmt.Sprintf("%.0f", v.Value)
}

func (m Model) tiles(w int, st model.Stats) []string {
	t := &m.theme
	muted := fg(t.Muted)
	cols := split(w, 20, 20, 20, 20, 20)

	downDot := t.Border
	if st.Down > 0 {
		downDot = t.Err
	}
	hosts := m.tile(cols[0], "Hosts", bigText(t, fmt.Sprint(st.Total), "", t.FG),
		fg(t.OK).Render("● ")+muted.Render(fmt.Sprintf("%d up   ", st.Up))+fg(downDot).Render("● ")+muted.Render(fmt.Sprintf("%d down", st.Down)))

	availColour, availText := t.Muted, "-"
	if st.Availability.Set {
		availText = fmt.Sprintf("%.0f", st.Availability.Value)
		switch a := st.Availability.Value; {
		case a >= 99.9:
			availColour = t.OK
		case a >= 90:
			availColour = t.Warn
		default:
			availColour = t.Err
		}
	}
	availSub := muted.Render("agents reachable")
	if st.Down > 0 {
		availSub = fg(t.Err).Render(fmt.Sprintf("%d unreachable", st.Down))
	} else if st.Maintenance > 0 {
		availSub = muted.Render(fmt.Sprintf("◆ %d in maintenance", st.Maintenance))
	}
	avail := m.tile(cols[1], "Availability", bigText(t, availText, "%", availColour), availSub)

	probColour := t.OK
	probSub := fg(t.OK).Render("all clear")
	if st.Worst >= 0 {
		probColour = t.Sev[st.Worst]
		label := st.Worst.Short()
		prefix := fmt.Sprintf("%d unacked · ", st.Unacked)
		if cols[2] >= 32 {
			label = st.Worst.Label()
			prefix = fmt.Sprintf("%d unacked · worst ", st.Unacked)
		}
		probSub = muted.Render(prefix) + fg(t.Sev[st.Worst]).Render(label)
	}
	count := fmt.Sprint(st.Active)
	if m.snapshot.Truncated {
		count += "+"
	}
	problems := m.tile(cols[2], "Problems", bigText(t, count, "", probColour), probSub)

	avgTile := func(w int, name string, avg model.Metric, peak *model.HostRow, metric func(*model.HostRow) model.Metric) []string {
		sub := muted.Render("no data")
		if peak != nil {
			v := metric(peak)
			sub = muted.Render("peak ") + fg(t.FG).Render(truncate(peak.Name, max(w-13, 4))) + fg(level(t, v.Value)).Render(" "+fmtPct(v))
		}
		colour := t.Muted
		if avg.Set {
			colour = level(t, avg.Value)
		}
		return m.tile(w, name, bigText(t, tilePct(avg), "%", colour), sub)
	}
	cpu := avgTile(cols[3], "Avg CPU", st.AvgCPU, st.PeakCPU, func(h *model.HostRow) model.Metric { return h.CPU })
	mem := avgTile(cols[4], "Avg Memory", st.AvgMem, st.PeakMem, func(h *model.HostRow) model.Metric { return h.Mem })
	return joinRow(hosts, avail, problems, cpu, mem)
}

func (m Model) severityPanel(w int, st model.Stats) []string {
	t := &m.theme
	inner := w - 2
	bar := fg(t.OK).Render(strings.Repeat("━", inner))
	if segs := model.SeverityBar(st.Counts, inner); segs != nil {
		var b strings.Builder
		for _, s := range segs {
			b.WriteString(fg(t.Sev[s.Severity]).Render(strings.Repeat("█", s.Cells)))
		}
		bar = b.String()
	}
	var legend strings.Builder
	for s := model.Disaster; s >= model.NotClassified; s-- {
		n := st.Counts[s]
		mark, label, num := t.Border, t.Muted, t.Muted
		if n > 0 {
			mark, label, num = t.Sev[s], t.FG, t.Sev[s]
		}
		legend.WriteString(fg(mark).Render("■ ") + fg(label).Render(s.Label()) + fg(num).Bold(true).Render(fmt.Sprintf(" %d     ", n)))
	}
	return panel(t, w, 4, panelOpts{title: title(t, "Problems by severity")}, []string{bar, legend.String()})
}

func (m Model) fleetChart(w, h int) []string {
	t := &m.theme
	label, other := "CPU", "show memory"
	metric := func(r *model.HostRow) ([]model.Point, model.Metric) { return r.CPUHist, r.CPU }
	if m.dashMem {
		label, other = "Memory", "show cpu"
		metric = func(r *model.HostRow) ([]model.Point, model.Metric) { return r.MemHist, r.Mem }
	}
	// The busiest six hosts: more lines than that is just noise.
	var hosts []*model.HostRow
	for i := range m.snapshot.Hosts {
		h := &m.snapshot.Hosts[i]
		if p, _ := metric(h); len(p) > 0 {
			hosts = append(hosts, h)
		}
	}
	slices.SortStableFunc(hosts, func(a, b *model.HostRow) int {
		_, av := metric(a)
		_, bv := metric(b)
		switch {
		case bv.Value > av.Value:
			return 1
		case bv.Value < av.Value:
			return -1
		}
		return 0
	})
	hosts = hosts[:min(len(hosts), 6)]
	series := make([]chartSeries, len(hosts))
	for i, h := range hosts {
		p, v := metric(h)
		series[i] = chartSeries{name: h.Name + " " + fmtPct(v), points: p, colour: t.Series[i%len(t.Series)]}
	}
	now := float64(m.now().Unix())
	o := panelOpts{
		title:  title(t, "Fleet "+label+" · last 30m"),
		bottom: fg(t.Accent).Bold(true).Render("m ") + fg(t.Muted).Render(other),
	}
	return timeChart(t, w, h, o, series, now-model.SparkWindow, now, 5, yPercent)
}

func (m Model) feed(w, h int) []string {
	t := &m.theme
	o := panelOpts{title: title(t, "Latest problems")}
	var problems []*model.Problem
	for i := range m.snapshot.Problems {
		if p := &m.snapshot.Problems[i]; !p.Suppressed {
			problems = append(problems, p)
		}
	}
	inner := w - 2
	if len(problems) == 0 {
		lines := make([]string, max((h-2-4)/2, 0))
		return panel(t, w, h, o, append(lines,
			center(fg(t.OK).Bold(true).Render("✓"), inner),
			center(title(t, "All clear"), inner),
			center(fg(t.Muted).Render("no active problems"), inner)))
	}
	slices.SortStableFunc(problems, func(a, b *model.Problem) int {
		switch {
		case b.Clock > a.Clock:
			return 1
		case b.Clock < a.Clock:
			return -1
		}
		return 0
	})
	now := m.now()
	var lines []string
	for _, p := range problems[:min(len(problems), (h-1)/2)] {
		ack := ""
		if p.Acknowledged {
			ack = fg(t.OK).Render(" ✓")
		}
		lines = append(lines,
			severityBadge(t, p.Severity)+" "+title(t, truncate(p.Host, max(inner-18, 1)))+ack+fg(t.Muted).Render("  "+fmtAge(p.Clock, now)+" ago"),
			"       "+fg(t.Muted).Render(truncate(p.Name, max(inner-8, 1))))
	}
	return panel(t, w, h, o, lines)
}

func (m Model) dashHosts(w, h int) []string {
	t := &m.theme
	hosts := m.filteredHosts()
	wide := w >= wideHosts
	spark := 14
	if wide {
		spark = 24
	}
	headers := []string{"", "Host", "CPU · 30m", "Memory"}
	cols := []int{1, 0, spark + 6, 16}
	if wide {
		headers = append(headers, "Disk /", "Load", "Network", "Uptime")
		cols = append(cols, 11, 5, 22, 8)
	}
	headers = append(headers, "Problems")
	cols = append(cols, 12)
	tb := table{
		headers: headers,
		widths:  flexWidths(w-2, cols, 14, 0),
		n:       len(hosts),
		sel:     m.hostSel,
		scroll:  m.scroll.dash,
		row: func(i int, base lipgloss.Style) []string {
			r := hosts[i]
			val := base.Foreground(valueFg(t, r))
			gap := base.Render(" ")
			muted := base.Foreground(t.Muted)
			cells := []string{
				statusDot(t, base, r),
				hostName(t, base, r, 200),
				sparkline(t, base, r.CPUHist, spark) + val.Render(fmt.Sprintf(" %5s", fmtPct(r.CPU))),
				meter(t, base, r.Mem, 10) + val.Render(fmt.Sprintf(" %5s", fmtPct(r.Mem))),
			}
			if wide {
				cells = append(cells,
					meter(t, base, r.Disk, 6)+base.Foreground(t.FG).Render(fmt.Sprintf(" %4s", fmtPct(r.Disk))),
					muted.Render(fmtMetric(r.Load, fmtLoad)),
					base.Foreground(t.Series[1]).Render("↓")+muted.Render(fmt.Sprintf("%9s", fmtMetric(r.NetIn, fmtBits)))+gap+
						base.Foreground(t.Series[2]).Render("↑")+muted.Render(fmt.Sprintf("%9s", fmtMetric(r.NetOut, fmtBits))),
					muted.Render(fmtMetric(r.Uptime, fmtUptime)))
			}
			return append(cells, problemCounts(t, base, r))
		},
	}
	return panel(t, w, h, panelOpts{title: title(t, "Hosts")}, tb.render(t, w-2, h-2))
}
