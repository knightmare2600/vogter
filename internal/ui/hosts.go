package ui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/theme"
)

func (m Model) hostsView(w, h int) []string {
	hosts := m.filteredHosts()
	tableH := min(max(len(hosts)+4, 6), max(h-12, 6))
	out := []string{m.hostsInfo(hosts)}
	out = append(out, m.hostsTable(hosts, w, tableH)...)
	return append(out, m.hostPreview(hosts, w, max(h-1-tableH, 0))...)
}

func (m Model) hostsInfo(hosts []*model.HostRow) string {
	t := &m.theme
	border := fg(t.Border)
	s := fg(t.FG).Bold(true).Render(fmt.Sprintf(" %d", len(hosts))) +
		fg(t.Muted).Render(fmt.Sprintf(" of %d hosts", len(m.snapshot.Hosts))) +
		border.Render("   sort ") + pill(m.hostSort.Label(), t.FG, t.Surface)
	if m.hostFilter != "" {
		s += border.Render("   filter ") + pill(m.hostFilter, t.BG, t.Accent)
	}
	return s
}

func (m Model) hostsTable(hosts []*model.HostRow, w, h int) []string {
	t := &m.theme
	headers := []string{"", "Name", "Groups", "Interface", "CPU", "Memory", "Disk", "Load", "Uptime", "Problems"}
	cols := []int{1, 0, 0, 22, 14, 14, 5, 5, 8, 12}
	// Narrow terminals lose the groups, then the interface.
	drop := 0
	if w < 130 {
		drop = 1
	}
	if w < 110 {
		drop = 2
	}
	keep := func(i int) bool { return !(drop >= 1 && i == 2) && !(drop >= 2 && i == 3) }
	var hs []string
	var cs []int
	for i := range headers {
		if keep(i) {
			hs, cs = append(hs, headers[i]), append(cs, cols[i])
		}
	}
	tb := table{
		headers: hs,
		widths:  flexWidths(w-2, cs, 12, 1),
		marker:  true,
		gap:     true,
		n:       len(hosts),
		sel:     m.hostSel,
		scroll:  m.scroll.hosts,
		row: func(i int, base lipgloss.Style) []string {
			r := hosts[i]
			muted := base.Foreground(t.Muted)
			val := base.Foreground(valueFg(t, r))
			diskColour := t.Muted
			if r.Disk.Set {
				diskColour = level(t, r.Disk.Value)
			}
			all := []string{
				statusDot(t, base, r),
				hostName(t, base, r, 200),
				muted.Render(strings.Join(r.Groups, ", ")),
				muted.Render(r.Address),
				meter(t, base, r.CPU, 8) + val.Render(fmt.Sprintf(" %5s", fmtPct(r.CPU))),
				meter(t, base, r.Mem, 8) + val.Render(fmt.Sprintf(" %5s", fmtPct(r.Mem))),
				base.Foreground(diskColour).Render(fmtPct(r.Disk)),
				muted.Render(fmtMetric(r.Load, fmtLoad)),
				muted.Render(fmtMetric(r.Uptime, fmtUptime)),
				problemCounts(t, base, r),
			}
			var cells []string
			for i, c := range all {
				if keep(i) {
					cells = append(cells, c)
				}
			}
			return cells
		},
	}
	return panel(t, w, h, panelOpts{title: title(t, "Hosts")}, tb.render(t, w-2, h-2))
}

func hostState(t *theme.Theme, h *model.HostRow) (string, color.Color) {
	switch {
	case !h.Enabled:
		return "disabled", t.Muted
	case h.Maintenance:
		return "maintenance", t.Info
	case h.Availability == model.Up:
		return "available", t.OK
	case h.Availability == model.Down:
		return "unreachable", t.Err
	}
	return "unknown", t.Muted
}

func (m Model) hostPreview(hosts []*model.HostRow, w, h int) []string {
	t := &m.theme
	if m.hostSel >= len(hosts) || h < 4 {
		return nil
	}
	r := hosts[m.hostSel]
	cols := split(w, 37, 37, 26)
	now := float64(m.now().Unix())
	x0 := now - model.SparkWindow

	cpu := timeChart(t, cols[0], h, panelOpts{title: title(t, "CPU  "+fmtPct(r.CPU))},
		[]chartSeries{{"cpu", r.CPUHist, t.Series[0]}}, x0, now, 5, yPercent)
	mem := timeChart(t, cols[1], h, panelOpts{title: title(t, "Memory  "+fmtPct(r.Mem))},
		[]chartSeries{{"mem", r.MemHist, t.Series[2]}}, x0, now, 5, yPercent)

	state, colour := hostState(t, r)
	kv := func(k, v string) string { return fg(t.Muted).Render(fmt.Sprintf("%-8s", k)) + fg(t.FG).Render(v) }
	lines := []string{
		statusDot(t, lipgloss.NewStyle(), r) + " " + fg(colour).Bold(true).Render(state),
		"",
		kv("CPUs", fmtMetric(r.CPUs, func(v float64) string { return fmt.Sprintf("%.0f", v) })),
		kv("Net ↓", fmtMetric(r.NetIn, fmtBits)),
		kv("Net ↑", fmtMetric(r.NetOut, fmtBits)),
		kv("Uptime", fmtMetric(r.Uptime, fmtUptime)),
	}
	if r.Error != "" {
		lines = append(lines, "", fg(t.Err).Render(truncate(r.Error, max(cols[2]-4, 1))))
	}
	info := panel(t, cols[2], h, panelOpts{title: title(t, truncate(r.Name, 24)), bottom: fg(t.Muted).Render("⏎ details")}, lines)
	return joinRow(cpu, mem, info)
}
