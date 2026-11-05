package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/knightmare2600/vogter/internal/mapterm"
)

// The hidden message, toggled by ø or £.
const eggMessage = "Shine on, shine the lights on me, In all of my life, so that much more I see... Jeg har mit på det tørre!"

// isEggKey is ø, the original operator's marker, or £ for a UK keyboard.
func isEggKey(k tea.KeyPressMsg) bool { return k.Text == "ø" || k.Text == "£" }

func (m *Model) toggleEgg() {
	if m.egg >= 0 {
		m.egg = -1
		return
	}
	m.egg = m.frame
}

// eggLine scrolls the message right to left across the footer.
func (m Model) eggLine(w int) string {
	runes := []rune(eggMessage + "        ")
	off := (m.frame - m.egg) % len(runes)
	var b strings.Builder
	for i := range max(w-2, 1) {
		b.WriteRune(runes[(off+i)%len(runes)])
	}
	return " " + fg(m.theme.Accent).Render(b.String())
}

func (m Model) aboutBox() string {
	t := &m.theme
	name := m.brand("Vægter", "Vaegter")
	version := m.opts.Version
	if version == "" {
		version = "dev"
	}
	muted, body, accent := fg(t.Muted), fg(t.FG), fg(t.Accent).Bold(true)
	lines := []string{
		"",
		"  " + accent.Render(name) + muted.Render("  "+version),
		"  " + body.Render("A terminal for Zabbix: the Danish word for a watchman."),
		"",
		"  " + accent.Render("Ported from"),
		"  " + body.Render("zabterm by Ender KUŞ, under the MIT licence."),
		"",
		"  " + accent.Render("Map"),
		"  " + body.Render("Map data © OpenStreetMap contributors, under the ODbL:"),
		"  " + muted.Render(mapterm.CopyrightURL),
		"  " + body.Render("Tiles © OpenMapTiles, served by OpenFreeMap."),
		"",
		"  " + muted.Render("vaegter licences prints every licence in full."),
		"",
		"  " + fg(t.Border).Render("esc close"),
	}
	w := min(66, max(m.width-2, 10))
	return strings.Join(panel(t, w, min(len(lines)+2, max(m.height-2, 3)), panelOpts{title: accent.Render("About"), border: t.Accent}, lines), "\n")
}
