// Package ui is the Bubble Tea program: application state, key handling
// and rendering.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/knightmare2600/vogter/internal/config"
	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/poller"
	"github.com/knightmare2600/vogter/internal/theme"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

// View is a top-level screen.
type View int

// Top-level screens.
const (
	Dashboard View = iota
	Hosts
	Problems
	Host
)

// hostTab is a host page tab.
type hostTab int

const (
	tabGraphs hostTab = iota
	tabItems
)

const (
	headerH    = 2
	footerH    = 1
	minWidth   = 80
	minHeight  = 24
	pageStep   = 10
	toastLife  = 6 * time.Second
	maxToasts  = 4
	ackTimeout = 25 * time.Second
)

var viewNames = [...]string{"Dashboard", "Hosts", "Problems"}

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type tickMsg time.Time

// eventMsg wraps a poller event.
type eventMsg struct{ poller.Event }

// ackedMsg reports the result of an acknowledge.
type ackedMsg struct {
	n   int
	err error
}

type toastKind int

const (
	toastInfo toastKind = iota
	toastOK
	toastError
)

type toast struct {
	kind  toastKind
	title string
	body  string
	born  time.Time
}

// ackDialog is the acknowledge modal.
type ackDialog struct {
	eventID, title string
	input          string
	close, canShut bool
}

// AckFunc sends an acknowledgement. It runs off the UI goroutine.
type AckFunc func(ctx context.Context, eventIDs []string, message string, close bool) error

// Options configures a Model.
type Options struct {
	Profile    string
	Endpoint   string
	ConfigPath string
	Theme      string
	Interval   time.Duration
	Events     <-chan poller.Event
	Cmds       chan<- poller.Cmd
	Ack        AckFunc
	// Watch selects the host whose detail the poller follows; an empty id
	// stops it. It must not block.
	Watch func(hostID string, rangeSecs int64)
}

// scrolls keeps table offsets. They live behind pointers because rendering
// updates them, like ratatui's TableState, and View has a value receiver.
type scrolls struct{ dash, hosts, problems, items *scroller }

// Model is the whole UI state.
type Model struct {
	opts   Options
	theme  theme.Theme
	now    func() time.Time
	scroll scrolls

	width, height int
	view          View
	help          bool
	ack           *ackDialog
	toasts        []toast
	frame         int

	version  string
	support  string
	err      string
	loading  bool
	snapshot *model.Snapshot

	hostFilter, problemFilter string
	editing                   bool
	hostSort                  model.HostSort
	minSeverity               model.Severity
	hideAcked                 bool
	dashMem                   bool
	hostSel, problemSel       int

	backTo     View
	detailHost string
	detail     *model.HostDetail
	rangeIdx   int
	hostTab    hostTab
	itemFilter string
	itemSel    int

	quitting bool
}

// New builds the model.
func New(o Options) Model {
	return Model{
		opts:     o,
		theme:    theme.Load(o.Theme),
		now:      time.Now,
		scroll:   scrolls{&scroller{}, &scroller{}, &scroller{}, &scroller{}},
		loading:  true,
		hostSort: model.SortProblems,
		rangeIdx: 1,
		backTo:   Hosts,
	}
}

func tick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) waitEvent() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.opts.Events
		if !ok {
			return nil
		}
		return eventMsg{ev}
	}
}

// Init starts the clock and the event listener.
func (m Model) Init() tea.Cmd {
	return tea.Batch(tick(), m.waitEvent())
}

// Update handles one message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tickMsg:
		m.frame++
		m.expireToasts()
		return m, tick()
	case eventMsg:
		m.onEvent(msg.Event)
		return m, m.waitEvent()
	case ackedMsg:
		if msg.err != nil {
			m.loading = false
			m.addToast(toastError, failTitle("Acknowledge failed", msg.err), msg.err.Error())
			return m, nil
		}
		plural := "s"
		if msg.n == 1 {
			plural = ""
		}
		m.addToast(toastOK, "Acknowledged", fmt.Sprintf("%d problem%s", msg.n, plural))
		m.refresh()
	case tea.KeyPressMsg:
		return m.onKey(msg)
	case tea.PasteMsg:
		m.paste(msg.Content)
	}
	return m, nil
}

func (m *Model) addToast(kind toastKind, title, body string) {
	list := append([]toast{{kind, title, body, m.now()}}, m.toasts...)
	m.toasts = list[:min(len(list), maxToasts)]
}

func (m *Model) expireToasts() {
	var keep []toast
	for _, t := range m.toasts {
		if m.now().Sub(t.born) < toastLife {
			keep = append(keep, t)
		}
	}
	m.toasts = keep
}

func (m *Model) onEvent(ev poller.Event) {
	switch ev := ev.(type) {
	case poller.Connected:
		m.version = ev.Version.String()
		m.support = ev.Support.Describe(ev.Version)
		m.err = ""
		if m.support != "" {
			m.addToast(toastInfo, "Zabbix version", m.support)
		}
	case poller.Snapshot:
		m.loading = false
		m.err = ""
		snap := ev.Snapshot
		m.snapshot = &snap
		m.clampSelection()
	case poller.Detail:
		// A late answer for another host or range is dropped.
		if ev.HostID == m.detailHost && ev.Range == ranges[m.rangeIdx].secs {
			d := ev.HostDetail
			m.detail = &d
			m.clampSelection()
		}
	case poller.Failed:
		m.loading = false
		msg := describe(ev.Err)
		// A repeating failure raises one toast, not one per poll.
		if m.err != msg {
			m.addToast(toastError, failTitle("Zabbix API", ev.Err), ev.Err.Error())
		}
		m.err = msg
	}
}

// describe turns an error into one line for the UI, adding a hint for the
// error classes a user can act on.
func describe(err error) string {
	switch zabbix.KindOf(err) {
	case zabbix.KindAuth:
		return err.Error() + " (check the token or credentials)"
	case zabbix.KindPermission:
		return err.Error() + " (the account lacks permission)"
	}
	return err.Error()
}

// failTitle names the cause in a toast title, which is short enough never
// to be cut off, unlike the error text under it.
func failTitle(what string, err error) string {
	switch zabbix.KindOf(err) {
	case zabbix.KindAuth:
		return what + ": credentials rejected"
	case zabbix.KindPermission:
		return what + ": no permission"
	case zabbix.KindTransport:
		return what + ": server unreachable"
	}
	return what
}

func (m *Model) refresh() {
	m.loading = true
	select {
	case m.opts.Cmds <- poller.Refresh:
	default:
		// A refresh is already queued.
	}
}

func (m Model) filteredHosts() []*model.HostRow {
	if m.snapshot == nil {
		return nil
	}
	return model.FilterHosts(m.snapshot.Hosts, m.hostFilter, m.hostSort)
}

func (m Model) filteredProblems() []*model.Problem {
	if m.snapshot == nil {
		return nil
	}
	return model.FilterProblems(m.snapshot.Problems, m.problemFilter, m.minSeverity, m.hideAcked)
}

func (m *Model) clampSelection() {
	m.hostSel = min(m.hostSel, max(len(m.filteredHosts())-1, 0))
	m.problemSel = min(m.problemSel, max(len(m.filteredProblems())-1, 0))
	m.itemSel = min(m.itemSel, max(len(m.filteredItems())-1, 0))
}

func (m *Model) moveSelection(delta int) {
	switch m.view {
	case Dashboard, Hosts:
		m.hostSel = max(min(m.hostSel+delta, len(m.filteredHosts())-1), 0)
	case Problems:
		m.problemSel = max(min(m.problemSel+delta, len(m.filteredProblems())-1), 0)
	case Host:
		if m.hostTab == tabItems {
			m.itemSel = max(min(m.itemSel+delta, len(m.filteredItems())-1), 0)
		}
	}
}

func (m *Model) watch(hostID string) {
	if m.opts.Watch != nil {
		m.opts.Watch(hostID, ranges[m.rangeIdx].secs)
	}
}

func (m *Model) openHost(id string) {
	if m.detailHost != id {
		m.detail = nil
		m.itemFilter = ""
		m.itemSel = 0
	}
	m.detailHost = id
	m.backTo = m.view
	m.view = Host
	m.hostTab = tabGraphs
	m.watch(id)
}

// show switches view, leaving the host page cleanly if it was open.
func (m *Model) show(v View) {
	if m.view == Host && v != Host {
		m.watch("")
	}
	m.view = v
}

func (m *Model) shiftRange(delta int) {
	next := max(min(m.rangeIdx+delta, len(ranges)-1), 0)
	if next != m.rangeIdx {
		m.rangeIdx = next
		m.watch(m.detailHost)
	}
}

// filter returns the active view's filter text, if it has one.
func (m *Model) filter() *string {
	switch m.view {
	case Hosts:
		return &m.hostFilter
	case Problems:
		return &m.problemFilter
	case Host:
		if m.hostTab == tabItems {
			return &m.itemFilter
		}
	}
	return nil
}

func (m *Model) paste(s string) {
	s = strings.NewReplacer("\n", " ", "\r", " ").Replace(s)
	switch {
	case m.ack != nil:
		d := *m.ack
		d.input += s
		m.ack = &d
	case m.editing:
		if f := m.filter(); f != nil {
			*f += s
			m.clampSelection()
		}
	}
}

func dropLast(s string) string {
	r := []rune(s)
	return string(r[:max(len(r)-1, 0)])
}

func (m Model) onKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := k.String()
	if k.Keystroke() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}
	switch {
	case m.help:
		switch key {
		case "esc", "?", "q", "enter":
			m.help = false
		}
		return m, nil
	case m.ack != nil:
		return m.onAckKey(k)
	case m.editing:
		m.onFilterKey(k)
		return m, nil
	}

	switch key {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "?":
		m.help = true
	case "1":
		m.show(Dashboard)
	case "2":
		m.show(Hosts)
	case "3":
		m.show(Problems)
	case "tab":
		if m.view == Host {
			return m.onViewKey(key)
		}
		m.view = (m.view + 1) % 3
	case "shift+tab":
		if m.view != Host {
			m.view = (m.view + 2) % 3
		}
	case "r":
		m.refresh()
	case "t":
		m.theme = theme.Load(theme.Next(m.theme.Name))
		m.addToast(toastInfo, "Theme", m.theme.Name)
	case "/":
		if m.filter() != nil {
			m.editing = true
		}
	case "esc":
		if m.view == Host {
			m.show(m.backTo)
		} else if f := m.filter(); f != nil {
			*f = ""
		}
	case "down", "j":
		m.moveSelection(1)
	case "up", "k":
		m.moveSelection(-1)
	case "pgdown", "ctrl+d":
		m.moveSelection(pageStep)
	case "pgup", "ctrl+u":
		m.moveSelection(-pageStep)
	case "g", "home":
		m.moveSelection(-1 << 30)
	case "G", "end":
		m.moveSelection(1 << 30)
	default:
		return m.onViewKey(key)
	}
	return m, nil
}

func (m Model) onViewKey(key string) (tea.Model, tea.Cmd) {
	switch m.view {
	case Dashboard, Hosts:
		switch key {
		case "enter", "l":
			if hs := m.filteredHosts(); m.hostSel < len(hs) {
				m.openHost(hs[m.hostSel].ID)
			}
		case "m":
			if m.view == Dashboard {
				m.dashMem = !m.dashMem
			}
		case "s":
			if m.view == Hosts {
				m.hostSort = m.hostSort.Next()
				m.hostSel = 0
			}
		}
	case Problems:
		switch key {
		case "enter", "l":
			if ps := m.filteredProblems(); m.problemSel < len(ps) && ps[m.problemSel].HostID != "" {
				m.openHost(ps[m.problemSel].HostID)
			}
		case "a":
			if ps := m.filteredProblems(); m.problemSel < len(ps) {
				p := ps[m.problemSel]
				m.ack = &ackDialog{eventID: p.EventID, title: p.Host + " · " + p.Name, canShut: p.ManualClose}
			}
		case "+", "s":
			m.minSeverity = (m.minSeverity + 1) % 6
			m.problemSel = 0
		case "-":
			m.minSeverity = (m.minSeverity + 5) % 6
			m.problemSel = 0
		case "h":
			m.hideAcked = !m.hideAcked
			m.problemSel = 0
		}
	case Host:
		switch key {
		case "[":
			m.shiftRange(-1)
		case "]":
			m.shiftRange(1)
		case "tab", "i":
			m.hostTab = 1 - m.hostTab
		case "h", "backspace":
			m.show(m.backTo)
		}
	}
	return m, nil
}

func (m *Model) onFilterKey(k tea.KeyPressMsg) {
	f := m.filter()
	switch k.String() {
	case "esc":
		*f = ""
		m.editing = false
	case "enter":
		m.editing = false
	case "backspace":
		*f = dropLast(*f)
	case "down":
		m.moveSelection(1)
	case "up":
		m.moveSelection(-1)
	default:
		*f += k.Text
	}
	m.clampSelection()
}

func (m Model) onAckKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	d := *m.ack
	switch k.String() {
	case "esc":
		m.ack = nil
		return m, nil
	case "tab":
		if d.canShut {
			d.close = !d.close
		}
	case "backspace":
		d.input = dropLast(d.input)
	case "enter":
		m.ack = nil
		m.loading = true
		ack := m.opts.Ack
		id, message, shut := d.eventID, strings.TrimSpace(d.input), d.close
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), ackTimeout)
			defer cancel()
			return ackedMsg{1, ack(ctx, []string{id}, message, shut)}
		}
	default:
		d.input += k.Text
	}
	m.ack = &d
	return m, nil
}

// View renders the screen.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "vogter · " + m.opts.Profile
	return v
}

func (m Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	bodyH := max(m.height-headerH-footerH, 0)
	var body []string
	switch {
	case m.snapshot == nil:
		body = strings.Split(lipgloss.Place(m.width, bodyH, lipgloss.Center, lipgloss.Center, m.splash()), "\n")
	case m.width < minWidth || m.height < minHeight:
		notice := fg(m.theme.Muted).Render(fmt.Sprintf("terminal too small: need %d×%d", minWidth, minHeight))
		body = strings.Split(lipgloss.Place(m.width, bodyH, lipgloss.Center, lipgloss.Center, notice), "\n")
	case m.view == Dashboard:
		body = m.dashboard(m.width, bodyH)
	case m.view == Hosts:
		body = m.hostsView(m.width, bodyH)
	case m.view == Host:
		body = m.hostView(m.width, bodyH)
	default:
		body = m.problemsView(m.width, bodyH)
	}
	body = body[:min(len(body), bodyH)]
	for len(body) < bodyH {
		body = append(body, "")
	}
	// Header and footer clip rather than wrap on a narrow terminal.
	head := strings.SplitN(m.header(), "\n", 2)[0]
	base := ansi.Truncate(head, m.width, "") + "\n\n" + strings.Join(body, "\n") + "\n" + ansi.Truncate(m.footer(), m.width, "")

	layers := []*lipgloss.Layer{lipgloss.NewLayer(base)}
	y := headerH
	for _, t := range m.toasts {
		if y+4 > headerH+bodyH {
			break
		}
		w := min(48, max(m.width-2, 8))
		layers = append(layers, lipgloss.NewLayer(strings.Join(m.toastBox(t, w), "\n")).X(m.width-w-1).Y(y).Z(1))
		y += 4
	}
	if box := m.modal(); box != "" {
		bw, bh := lipgloss.Size(box)
		layers = append(layers, lipgloss.NewLayer(box).X(max((m.width-bw)/2, 0)).Y(max((m.height-bh)/2, 0)).Z(2))
	}
	if len(layers) == 1 {
		return base
	}
	return lipgloss.NewCompositor(layers...).Render()
}

func (m Model) toastBox(t toast, w int) []string {
	th := &m.theme
	icon, colour := "●", th.Accent
	switch t.kind {
	case toastOK:
		icon, colour = "✓", th.OK
	case toastError:
		icon, colour = "✕", th.Err
	}
	return panel(th, w, 4, panelOpts{border: colour}, []string{
		fg(colour).Render(icon+" ") + title(th, truncate(t.title, w-6)),
		fg(th.Muted).Render(truncate(t.body, w-4)),
	})
}

func (m Model) modal() string {
	switch {
	case m.help:
		return m.helpBox()
	case m.ack != nil:
		return m.ackBox()
	}
	return ""
}

func (m Model) header() string {
	t := &m.theme
	s := lipgloss.NewStyle()
	left := []string{s.Foreground(t.BG).Background(t.Accent).Bold(true).Render(" ◆ vogter "), "  "}
	counts := [3]string{}
	countColour := t.OK
	if m.snapshot != nil {
		st := model.Summarise(m.snapshot)
		counts[1] = fmt.Sprintf(" %d", st.Total)
		counts[2] = fmt.Sprintf(" %d", st.Active)
		if m.snapshot.Truncated {
			counts[2] += "+"
		}
		if st.Worst >= 0 {
			countColour = t.Sev[st.Worst]
		}
	}
	for i, name := range viewNames {
		st := s.Foreground(t.Muted)
		// While on a host page, keep the list the user came from lit.
		if View(i) == m.view || (m.view == Host && View(i) == m.backTo) {
			st = s.Foreground(t.Accent).Bold(true).Underline(true)
		}
		c := t.Muted
		if View(i) == Problems {
			c = countColour
		}
		left = append(left, s.Foreground(t.Border).Render(fmt.Sprintf("%d ", i+1)), st.Render(name), s.Foreground(c).Bold(true).Render(counts[i]), "   ")
	}
	if m.view == Host {
		name := ""
		if r := m.detailRow(); r != nil {
			name = r.Name
		}
		left = append(left, fg(t.Border).Render("› "), fg(t.Accent).Bold(true).Underline(true).Render(name))
	}
	l := strings.Join(left, "")

	var dot, state string
	switch {
	case m.err != "":
		dot, state = fg(t.Err).Render("●"), fg(t.Err).Render(" offline")
	case m.loading:
		dot, state = fg(t.Accent).Render(spinner[m.frame%len(spinner)]), fg(t.Muted).Render(" sync")
	default:
		pulse := t.OK
		if m.frame%8 >= 4 {
			pulse = t.Muted
		}
		dot, state = fg(pulse).Render("●"), fg(t.Muted).Render(" live")
	}
	sep := fg(t.Border).Render("  ·  ")
	clock := fg(t.FG).Render(m.now().Format("15:04:05"))
	profile := fg(t.FG).Bold(true).Render(m.opts.Profile)
	var version, latency string
	if m.version != "" {
		version = fg(t.Muted).Render("Zabbix " + m.version)
	}
	if m.snapshot != nil {
		latency = fg(t.Muted).Render(fmt.Sprintf("%dms", m.snapshot.Latency.Milliseconds()))
	}
	// Drop the least important parts first when the terminal is narrow.
	var r string
	for _, tier := range [][]string{{profile, version, latency, clock}, {profile, clock}, {clock}} {
		r = dot + state
		for _, part := range tier {
			if part != "" {
				r += sep + part
			}
		}
		r += " "
		if lipgloss.Width(l)+lipgloss.Width(r) < m.width {
			break
		}
	}
	gap := max(m.width-lipgloss.Width(l)-lipgloss.Width(r), 0)
	return l + strings.Repeat(" ", gap) + r + "\n"
}

func (m Model) hints() [][2]string {
	switch m.view {
	case Hosts:
		return [][2]string{{"j/k", "move"}, {"⏎", "open"}, {"/", "filter"}, {"s", "sort"}, {"r", "refresh"}, {"?", "help"}}
	case Problems:
		return [][2]string{{"j/k", "move"}, {"a", "ack"}, {"⏎", "host"}, {"+/-", "severity"}, {"h", "hide acked"}, {"/", "filter"}, {"?", "help"}}
	}
	if m.view == Host {
		if m.hostTab == tabItems {
			return [][2]string{{"esc", "back"}, {"tab", "graphs"}, {"/", "filter"}, {"j/k", "move"}, {"?", "help"}}
		}
		return [][2]string{{"esc", "back"}, {"[ ]", "range"}, {"tab", "items"}, {"r", "refresh"}, {"?", "help"}}
	}
	return [][2]string{{"j/k", "select"}, {"⏎", "open"}, {"m", "cpu/mem"}, {"tab", "next"}, {"t", "theme"}, {"?", "help"}, {"q", "quit"}}
}

func (m Model) footer() string {
	t := &m.theme
	if m.editing {
		f := m.filter()
		return lipgloss.NewStyle().Foreground(t.BG).Background(t.Accent).Bold(true).Render(" / ") + " " +
			fg(t.FG).Render(*f) + fg(t.Accent).Render("▏") + fg(t.Muted).Render("   ⏎ keep  esc clear")
	}
	var b strings.Builder
	b.WriteString(" ")
	for _, h := range m.hints() {
		b.WriteString(fg(t.Accent).Bold(true).Render(h[0]))
		b.WriteString(fg(t.Muted).Render(" " + h[1] + "   "))
	}
	l := b.String()
	if m.snapshot == nil {
		return l
	}
	ago := max(int(m.now().Sub(m.snapshot.FetchedAt).Seconds()), 0)
	r := fg(t.Muted).Render(fmt.Sprintf("updated %ds ago", ago)) +
		fg(t.Border).Render(fmt.Sprintf("  every %ds ", int(m.opts.Interval.Seconds())))
	if lipgloss.Width(l)+lipgloss.Width(r) >= m.width {
		return l
	}
	return l + strings.Repeat(" ", m.width-lipgloss.Width(l)-lipgloss.Width(r)) + r
}

// logo spells VOGTER in the half-block style of the dashboard digits.
var logo = [6][3]string{
	{"█ █", "█ █", " ▀ "},
	{"█▀█", "█ █", "▀▀▀"},
	{"█▀▀", "█ █", "▀▀▀"},
	{"▀█▀", " █ ", " ▀ "},
	{"█▀▀", "█▀▀", "▀▀▀"},
	{"█▀▄", "█▀▄", "▀ ▀"},
}

func (m Model) splash() string {
	t := &m.theme
	var lines []string
	for row := 0; row < 3; row++ {
		var b strings.Builder
		for i, g := range logo {
			c := t.Accent
			if i >= 3 {
				c = t.Series[2]
			}
			b.WriteString(fg(c).Bold(true).Render(g[row]) + " ")
		}
		lines = append(lines, b.String())
	}
	lines = append(lines, "", fg(t.Muted).Render("a terminal for Zabbix"), "")
	if m.err != "" {
		lines = append(lines,
			fg(t.Err).Render("✕  "+truncate(m.err, max(m.width-8, 20))),
			"",
			fg(t.Muted).Render("config  ")+fg(t.FG).Render(config.Tilde(m.opts.ConfigPath)),
			fg(t.Muted).Render("retrying on the next refresh · r to retry now"))
	} else {
		lines = append(lines, fg(t.Accent).Render(spinner[m.frame%len(spinner)])+
			fg(t.Muted).Render("  connecting to "+m.opts.Endpoint))
	}
	return lipgloss.JoinVertical(lipgloss.Center, lines...)
}

func (m Model) helpBox() string {
	t := &m.theme
	sections := []struct {
		name string
		keys [][2]string
	}{
		{"Global", [][2]string{{"1 2 3 / tab", "switch view"}, {"r", "refresh now"}, {"t", "cycle theme"}, {"?", "this help"}, {"q", "quit"}}},
		{"Lists", [][2]string{{"j k ↑ ↓", "move"}, {"g G", "top / bottom"}, {"ctrl-d ctrl-u", "page"}, {"/", "filter"}, {"⏎", "drill into host"}}},
		{"Problems", [][2]string{{"a", "acknowledge (+ message, close)"}, {"+ -", "minimum severity"}, {"h", "hide acknowledged"}}},
		{"Host", [][2]string{{"[ ]", "time range 15m → 7d"}, {"tab", "graphs / latest data"}, {"esc", "back"}}},
	}
	lines := []string{""}
	for _, sec := range sections {
		lines = append(lines, "  "+title(t, sec.name))
		for _, k := range sec.keys {
			lines = append(lines, fg(t.Accent).Render(fmt.Sprintf("    %-16s", k[0]))+fg(t.Muted).Render(k[1]))
		}
		lines = append(lines, "")
	}
	lines = append(lines, fg(t.Border).Render("  theme ")+fg(t.Muted).Render(t.Name)+
		fg(t.Border).Render("   config ")+fg(t.Muted).Render(config.Tilde(m.opts.ConfigPath)))
	w := min(64, max(m.width-2, 10))
	return strings.Join(panel(t, w, min(len(lines)+2, max(m.height-2, 3)), panelOpts{title: fg(t.Accent).Bold(true).Render("Keys"), border: t.Accent}, lines), "\n")
}

func (m Model) ackBox() string {
	t := &m.theme
	d := m.ack
	w := min(70, max(m.width-2, 20))
	inner := w - 4
	muted := fg(t.Muted)
	field := panel(t, inner, 3, panelOpts{title: muted.Render("message")}, []string{fg(t.FG).Render(d.input) + fg(t.Accent).Render("▏")})
	opt := fg(t.Border).Render("trigger does not allow manual close")
	if d.canShut {
		mark := "□"
		if d.close {
			mark = "■"
		}
		opt = fg(t.Accent).Render(mark+" ") + muted.Render("close problem too") + fg(t.Border).Render("  (tab)")
	}
	accent := fg(t.Accent).Bold(true)
	lines := []string{" " + fg(t.FG).Render(truncate(d.title, inner)), "", ""}
	for _, l := range field {
		lines = append(lines, " "+l)
	}
	lines = append(lines, " "+opt, "", " "+accent.Render("⏎")+muted.Render(" confirm   ")+accent.Render("esc")+muted.Render(" cancel"))
	return strings.Join(panel(t, w, len(lines)+2, panelOpts{title: fg(t.Accent).Bold(true).Render("Acknowledge"), border: t.Accent}, lines), "\n")
}
