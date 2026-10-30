package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/knightmare2600/vogter/internal/model"
)

const problemDetailH = 8

func (m Model) problemsView(w, h int) []string {
	problems := m.filteredProblems()
	out := []string{m.problemChips(problems)}
	out = append(out, m.problemsTable(problems, w, max(h-1-problemDetailH, 6))...)
	return append(out, m.problemDetail(problems, w)...)
}

func (m Model) problemChips(problems []*model.Problem) string {
	t := &m.theme
	border := fg(t.Border)
	total := fmt.Sprint(len(m.snapshot.Problems))
	if m.snapshot.Truncated {
		total += "+"
	}
	var b strings.Builder
	b.WriteString(fg(t.FG).Bold(true).Render(fmt.Sprintf(" %d", len(problems))))
	b.WriteString(fg(t.Muted).Render(" of " + total + " problems"))
	b.WriteString(border.Render("   severity ≥ "))
	// A mini severity scale showing where the threshold sits.
	for s := model.NotClassified; s <= model.Disaster; s++ {
		c := t.Border
		if s >= m.minSeverity {
			c = t.Sev[s]
		}
		b.WriteString(fg(c).Render("■"))
	}
	b.WriteString(fg(t.FG).Render(" " + m.minSeverity.Label()))
	acked := "shown"
	if m.hideAcked {
		acked = "hidden"
	}
	b.WriteString(border.Render("   acked ") + pill(acked, t.FG, t.Surface))
	if m.problemFilter != "" {
		b.WriteString(border.Render("   filter ") + pill(m.problemFilter, t.BG, t.Accent))
	}
	return b.String()
}

func (m Model) problemsTable(problems []*model.Problem, w, h int) []string {
	t := &m.theme
	o := panelOpts{title: title(t, "Problems")}
	if len(problems) == 0 {
		inner := w - 2
		return panel(t, w, h, o, []string{"", "",
			center(fg(t.OK).Bold(true).Render("✓"), inner),
			center(title(t, "Nothing to see here"), inner),
			center(fg(t.Muted).Render("no problems match the current filters"), inner)})
	}
	now := m.now()
	tb := table{
		headers: []string{"Severity", "Time", "Age", "Host", "Problem", "Ack", "Tags"},
		widths:  flexWidths(w-2, []int{8, 12, 7, 18, 0, 3, 0}, 10, 1),
		marker:  true,
		gap:     true,
		n:       len(problems),
		sel:     m.problemSel,
		scroll:  m.scroll.problems,
		row: func(i int, base lipgloss.Style) []string {
			p := problems[i]
			text := t.FG
			if p.Suppressed || p.Acknowledged {
				text = t.Muted
			}
			ack := base.Foreground(t.Border).Render("·")
			if p.Acknowledged {
				ack = base.Foreground(t.OK).Render("✓")
			}
			return []string{
				severityBadge(t, p.Severity),
				base.Foreground(t.Muted).Render(fmtClock(p.Clock, now)),
				base.Foreground(t.FG).Render(fmtAge(p.Clock, now)),
				base.Foreground(text).Bold(true).Render(p.Host),
				base.Foreground(text).Render(p.Name),
				ack,
				base.Foreground(t.Border).Render(strings.Join(p.Tags, " ")),
			}
		},
	}
	return panel(t, w, h, o, tb.render(t, w-2, h-2))
}

func (m Model) problemDetail(problems []*model.Problem, w int) []string {
	t := &m.theme
	if m.problemSel >= len(problems) {
		return panel(t, w, problemDetailH, panelOpts{title: title(t, "Details")}, nil)
	}
	p := problems[m.problemSel]
	now := m.now()
	muted := fg(t.Muted)
	kv := func(k, v string) string { return muted.Render(k+" ") + fg(t.FG).Render(v) + "    " }
	lines := []string{
		fg(t.Sev[p.Severity]).Render("▲ ") + title(t, p.Name),
		"",
		kv("Host", p.Host) + kv("Severity", p.Severity.Label()) +
			kv("Started", fmt.Sprintf("%s (%s ago)", fmtClock(p.Clock, now), fmtAge(p.Clock, now))) + kv("Event", "#"+p.EventID),
	}
	status := muted.Render("Status ")
	if p.Acknowledged {
		status += fg(t.OK).Render("acknowledged")
	} else {
		status += fg(t.Warn).Render("unacknowledged")
	}
	if p.Suppressed {
		status += fg(t.Info).Render("  suppressed")
	}
	if p.OpData != "" {
		status += muted.Render("    Opdata ") + fg(t.FG).Render(p.OpData)
	}
	lines = append(lines, status)
	if len(p.Tags) > 0 {
		var tags strings.Builder
		tags.WriteString(muted.Render("Tags   "))
		for _, tag := range p.Tags {
			tags.WriteString(pill(tag, t.FG, t.Surface) + " ")
		}
		lines = append(lines, tags.String())
	}
	var wrapped []string
	for _, l := range lines {
		wrapped = append(wrapped, strings.Split(ansi.Wrap(l, w-2, ""), "\n")...)
	}
	accent := fg(t.Accent).Bold(true)
	bottom := accent.Render("a ") + muted.Render("acknowledge  ") + accent.Render("⏎ ") + muted.Render("host")
	return panel(t, w, problemDetailH, panelOpts{title: title(t, "Details"), bottom: bottom, border: t.Sev[p.Severity]}, wrapped)
}
