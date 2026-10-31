package ui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/knightmare2600/vogter/internal/config"
	"github.com/knightmare2600/vogter/internal/onboard"
	"github.com/knightmare2600/vogter/internal/theme"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

// setupTimeout bounds each step, which may make several API calls.
const setupTimeout = 45 * time.Second

const setupWidth = 72

// SetupBackend does the work behind the welcome flow.
type SetupBackend interface {
	Probe(ctx context.Context, serverURL string, verifyTLS bool) (string, error)
	SignIn(ctx context.Context, serverURL string, verifyTLS bool, username, password string) (string, error)
	UseToken(ctx context.Context, serverURL string, verifyTLS bool, token string) (string, error)
}

type setupStep int

const (
	stepWelcome setupStep = iota
	stepServer
	stepSignIn
	stepDone
)

type setupField int

const (
	fieldUser setupField = iota
	fieldPassword
)

type (
	setupTick struct{}
	probedMsg struct {
		summary string
		err     error
	}
	connectedMsg struct {
		profile string
		err     error
	}
)

// SetupModel is the first-run welcome flow. The password lives only in this
// model, is cleared as soon as it has been used, and is never shown or saved.
type SetupModel struct {
	backend SetupBackend
	path    string
	ascii   bool
	theme   theme.Theme

	width, height int
	frame         int
	step          setupStep
	busy          string
	err           string

	url       string
	serverURL string
	summary   string
	verifyTLS bool

	pasting  bool
	field    setupField
	username string
	password string
	token    string

	profile string
}

// NewSetup builds the welcome flow, writing to path.
func NewSetup(backend SetupBackend, path, themeName string, ascii bool) SetupModel {
	return SetupModel{backend: backend, path: path, ascii: ascii, theme: theme.Load(themeName), verifyTLS: true}
}

// Profile is the saved profile, or "" when the user left without finishing.
func (m SetupModel) Profile() string {
	if m.step == stepDone {
		return m.profile
	}
	return ""
}

// Init starts the spinner clock.
func (m SetupModel) Init() tea.Cmd { return setupClock() }

func setupClock() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return setupTick{} })
}

// Update handles one message.
func (m SetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case setupTick:
		m.frame++
		return m, setupClock()
	case probedMsg:
		m.busy = ""
		if msg.err != nil {
			m.err = setupProblem(msg.err, false)
			return m, nil
		}
		m.summary, m.err, m.step = msg.summary, "", stepSignIn
	case connectedMsg:
		m.busy = ""
		m.password = ""
		if msg.err != nil {
			m.err = setupProblem(msg.err, m.pasting)
			return m, nil
		}
		m.token = ""
		m.profile, m.err, m.step = msg.profile, "", stepDone
	case tea.PasteMsg:
		m.paste(msg.Content)
	case tea.KeyPressMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m SetupModel) onKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := k.String()
	if k.Keystroke() == "ctrl+c" {
		m.password, m.token = "", ""
		return m, tea.Quit
	}
	if m.busy != "" {
		return m, nil
	}
	switch m.step {
	case stepWelcome:
		switch key {
		case "enter":
			m.step = stepServer
		case "q", "esc":
			return m, tea.Quit
		}
	case stepDone:
		if key == "enter" {
			return m, tea.Quit
		}
	case stepServer:
		switch key {
		case "esc":
			m.step, m.err = stepWelcome, ""
		case "tab":
			m.verifyTLS = !m.verifyTLS
		case "enter":
			return m.submitServer()
		default:
			m.edit(&m.url, k)
		}
	case stepSignIn:
		return m.onSignInKey(k)
	}
	return m, nil
}

func (m SetupModel) onSignInKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.step, m.err, m.password = stepServer, "", ""
	case "tab":
		m.pasting, m.err = !m.pasting, ""
	case "up":
		m.field = fieldUser
	case "down":
		m.field = fieldPassword
	case "enter":
		return m.submitSignIn()
	default:
		switch {
		case m.pasting:
			m.edit(&m.token, k)
		case m.field == fieldUser:
			m.edit(&m.username, k)
		default:
			m.edit(&m.password, k)
		}
	}
	return m, nil
}

func (m *SetupModel) edit(s *string, k tea.KeyPressMsg) {
	if k.String() == "backspace" {
		*s = dropLast(*s)
		return
	}
	*s += k.Text
}

func (m *SetupModel) paste(text string) {
	text = strings.TrimSpace(strings.NewReplacer("\n", "", "\r", "").Replace(text))
	switch {
	case m.busy != "":
	case m.step == stepServer:
		m.url += text
	case m.step == stepSignIn && m.pasting:
		m.token += text
	case m.step == stepSignIn && m.field == fieldUser:
		m.username += text
	case m.step == stepSignIn:
		m.password += text
	}
}

func (m SetupModel) submitServer() (tea.Model, tea.Cmd) {
	u, err := onboard.Normalise(m.url)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.serverURL, m.url, m.err, m.busy = u, u, "", "contacting "+u
	backend, verify := m.backend, m.verifyTLS
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
		defer cancel()
		summary, err := backend.Probe(ctx, u, verify)
		return probedMsg{summary, err}
	}
}

func (m SetupModel) submitSignIn() (tea.Model, tea.Cmd) {
	backend, u, verify := m.backend, m.serverURL, m.verifyTLS
	if m.pasting {
		token := strings.TrimSpace(m.token)
		if token == "" {
			m.err = "paste your API token first"
			return m, nil
		}
		m.err, m.busy = "", "testing the token"
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
			defer cancel()
			profile, err := backend.UseToken(ctx, u, verify, token)
			return connectedMsg{profile, err}
		}
	}
	if m.field == fieldUser && m.password == "" {
		m.field = fieldPassword
		return m, nil
	}
	if m.username == "" || m.password == "" {
		m.err = "enter your username and password"
		return m, nil
	}
	username, password := m.username, m.password
	m.err, m.busy = "", "creating your API token"
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
		defer cancel()
		profile, err := backend.SignIn(ctx, u, verify, username, password)
		return connectedMsg{profile, err}
	}
}

// setupProblem turns a failure into what the user can do about it.
func setupProblem(err error, pasting bool) string {
	text := err.Error()
	if strings.Contains(text, "x509") || strings.Contains(text, "certificate") {
		return "The certificate is not trusted. Press esc and then tab to stop checking it, if you trust this server."
	}
	var zerr *zabbix.Error
	if !errors.As(err, &zerr) {
		return text
	}
	switch zabbix.KindOf(err) {
	case zabbix.KindAuth:
		if pasting {
			return "Zabbix rejected that token. Check that it is complete, enabled and not expired."
		}
		return "Zabbix rejected that username or password. Too many tries can lock the account for a while."
	case zabbix.KindPermission:
		return "This account may not create API tokens. Ask an admin for that permission, or tab to paste a token."
	case zabbix.KindTransport:
		return "Cannot reach that address. Check the URL, DNS and any firewall."
	case zabbix.KindHTTP, zabbix.KindDecode:
		return "That address does not look like a Zabbix frontend. Check the URL."
	}
	return text
}

// name is the product name, without æ where only ASCII will do.
func (m SetupModel) name() string {
	if m.ascii {
		return "Vaegter"
	}
	return "Vægter"
}

// View renders the screen.
func (m SetupModel) View() tea.View {
	screen := m.render()
	if m.ascii {
		screen = asciiOnly(screen)
	}
	v := tea.NewView(screen)
	v.AltScreen = true
	v.WindowTitle = m.name() + " · welcome"
	return v
}

func (m SetupModel) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	t := &m.theme
	w := max(min(setupWidth, m.width-2), 20)
	var body []string
	var heading string
	switch m.step {
	case stepWelcome:
		heading, body = "Welcome", m.welcomeBody(w-4)
	case stepServer:
		heading, body = "Step 1 of 2 · Your Zabbix server", m.serverBody(w-4)
	case stepSignIn:
		heading, body = "Step 2 of 2 · Sign in", m.signInBody(w-4)
	default:
		heading, body = "All set", m.doneBody(w-4)
	}
	if m.busy != "" {
		body = append(body, "", fg(t.Accent).Render(spinner[m.frame%len(spinner)])+fg(t.Muted).Render("  "+truncate(m.busy, w-8)))
	} else if m.err != "" {
		wrapped := lipgloss.NewStyle().Width(w - 6).Render(m.err)
		for i, l := range strings.Split(wrapped, "\n") {
			mark := "  "
			if i == 0 {
				mark = "✕ "
			}
			body = append(body, fg(t.Err).Render(mark+strings.TrimRight(l, " ")))
		}
	}
	lines := make([]string, 0, len(body)+2)
	lines = append(lines, "")
	for _, l := range body {
		lines = append(lines, " "+l)
	}
	box := panel(t, w, len(lines)+3, panelOpts{title: fg(t.Accent).Bold(true).Render(heading), border: t.Accent}, lines)
	page := append(logoLines(t), box...)
	page = append(page, "", m.keys())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, lipgloss.JoinVertical(lipgloss.Center, page...))
}

func (m SetupModel) keys() string {
	var keys [][2]string
	switch m.step {
	case stepWelcome:
		keys = [][2]string{{"⏎", "start"}, {"q", "quit"}}
	case stepServer:
		keys = [][2]string{{"⏎", "continue"}, {"tab", "certificate check"}, {"esc", "back"}}
	case stepSignIn:
		keys = [][2]string{{"⏎", "continue"}, {"tab", "switch method"}, {"esc", "back"}}
		if !m.pasting {
			keys = append([][2]string{{"↑ ↓", "field"}}, keys...)
		}
	default:
		keys = [][2]string{{"⏎", "open dashboard"}}
	}
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(fg(m.theme.Accent).Bold(true).Render(k[0]))
		b.WriteString(fg(m.theme.Muted).Render(" " + k[1] + "   "))
	}
	return b.String()
}

func (m SetupModel) welcomeBody(int) []string {
	t := &m.theme
	return []string{
		fg(t.FG).Render("No Zabbix server is set up yet, so let's connect one."),
		"",
		fg(t.Muted).Render("You will need the address of your Zabbix frontend and a login."),
		fg(t.Muted).Render(m.name() + " creates its own API token and saves it privately to"),
		fg(t.FG).Render(config.Tilde(m.path)),
		"",
		fg(t.Muted).Render("Your password is used once and is never stored."),
	}
}

// inputLines draws one text field. secret hides its value.
func (m SetupModel) inputLines(name, value string, focused, secret bool, w int) []string {
	t := &m.theme
	shown := value
	if secret {
		shown = strings.Repeat("•", len([]rune(value)))
	}
	if r := []rune(shown); len(r) > w-4 {
		shown = "…" + string(r[len(r)-(w-5):])
	}
	border := t.Border
	if focused && m.busy == "" {
		border = t.Accent
		shown += "▏"
	}
	return panel(t, w, 3, panelOpts{title: fg(t.Muted).Render(name), border: border}, []string{fg(t.FG).Render(shown)})
}

func (m SetupModel) serverBody(w int) []string {
	t := &m.theme
	body := []string{fg(t.Muted).Render("The address you open Zabbix at in your browser."), ""}
	body = append(body, m.inputLines("server address", m.url, true, false, w)...)
	mark := "■"
	if !m.verifyTLS {
		mark = "□"
	}
	body = append(body, fg(t.Accent).Render(mark+" ")+fg(t.Muted).Render("check the server's certificate"))
	if !m.verifyTLS {
		body = append(body, fg(t.Warn).Render("  Only for a server you trust, such as one with a self-signed certificate."))
	}
	return body
}

func (m SetupModel) signInBody(w int) []string {
	t := &m.theme
	body := []string{fg(t.OK).Render("✓ ") + fg(t.FG).Render(truncate(m.summary, w-2)) + fg(t.Muted).Render("  ·  ") + fg(t.Muted).Render(truncate(m.serverURL, 30)), ""}
	if m.pasting {
		body = append(body, fg(t.Muted).Render("Paste an API token from User settings → API tokens."))
		return append(body, m.inputLines("API token", m.token, true, true, w)...)
	}
	body = append(body, fg(t.Muted).Render(m.name()+" uses these once, to create its own API token."))
	body = append(body, m.inputLines("username", m.username, m.field == fieldUser, false, w)...)
	body = append(body, m.inputLines("password", m.password, m.field == fieldPassword, true, w)...)
	return append(body, fg(t.Border).Render("Using MFA or SSO? Press tab to paste a token instead."))
}

func (m SetupModel) doneBody(int) []string {
	t := &m.theme
	return []string{
		fg(t.OK).Render("✓ ") + fg(t.FG).Render("Connected to "+m.serverURL),
		"",
		fg(t.Muted).Render("Profile  ") + fg(t.FG).Render(m.profile),
		fg(t.Muted).Render("Saved to ") + fg(t.FG).Render(config.Tilde(m.path)),
		"",
		fg(t.Muted).Render("Add another server at any time with ") + fg(t.FG).Render("vaegter setup"),
	}
}
