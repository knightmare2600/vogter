package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/knightmare2600/vogter/internal/zabbix"
)

type fakeSetup struct {
	probeErr, connectErr error
	user, pass, token    string
}

func (f *fakeSetup) Probe(context.Context, string, bool) (string, error) {
	return "Zabbix 7.0.32", f.probeErr
}

func (f *fakeSetup) SignIn(_ context.Context, _ string, _ bool, user, pass string) (string, error) {
	f.user, f.pass = user, pass
	return "lab", f.connectErr
}

func (f *fakeSetup) UseToken(_ context.Context, _ string, _ bool, token string) (string, error) {
	f.token = token
	return "lab", f.connectErr
}

// drive sends keys, running any command they start, as the real program would.
func drive(t *testing.T, m SetupModel, keys ...string) SetupModel {
	t.Helper()
	for _, k := range keys {
		mm, cmd := m.Update(keyMsg(k))
		m = mm.(SetupModel)
		if cmd != nil {
			if _, quit := cmd().(tea.QuitMsg); quit {
				return m
			}
			mm, _ = m.Update(cmd())
			m = mm.(SetupModel)
		}
	}
	return m
}

func typedSetup(t *testing.T, m SetupModel, s string) SetupModel {
	t.Helper()
	for _, r := range s {
		m = drive(t, m, string(r))
	}
	return m
}

func newSetup(f *fakeSetup, w, h int) SetupModel {
	m := NewSetup(f, "/tmp/config.toml", "nord", false)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return mm.(SetupModel)
}

func setupText(m SetupModel) string { return ansiRe.ReplaceAllString(m.render(), "") }

func toSignIn(t *testing.T, m SetupModel) SetupModel {
	t.Helper()
	m = drive(t, m, "enter")
	m = typedSetup(t, m, "zabbix.example.com")
	return drive(t, m, "enter")
}

func TestSetupSignInFlow(t *testing.T) {
	f := &fakeSetup{}
	m := newSetup(f, 100, 40)
	wants(t, setupText(m), "Welcome", "never stored", "/tmp/config.toml")
	m = toSignIn(t, m)
	if m.step != stepSignIn || m.serverURL != "https://zabbix.example.com" {
		t.Fatalf("%+v", m.step)
	}
	wants(t, setupText(m), "Zabbix 7.0.32", "username", "password")
	m = typedSetup(t, m, "Admin")
	m = drive(t, m, "enter")
	m = typedSetup(t, m, "hunter2")
	out := setupText(m)
	wants(t, out, "•••••••")
	if strings.Contains(out, "hunter2") {
		t.Fatal("the password was shown")
	}
	m = drive(t, m, "enter")
	if f.user != "Admin" || f.pass != "hunter2" {
		t.Fatalf("%+v", f)
	}
	if m.step != stepDone || m.password != "" || m.Profile() != "lab" {
		t.Fatalf("step %v, password kept: %v", m.step, m.password != "")
	}
	wants(t, setupText(m), "All set", "Connected to https://zabbix.example.com", "vaegter setup")
}

func TestSetupPasteToken(t *testing.T) {
	f := &fakeSetup{}
	m := drive(t, toSignIn(t, newSetup(f, 100, 40)), "tab")
	m = drive(t, m, "enter")
	wants(t, setupText(m), "paste your API token first")
	mm, _ := m.Update(tea.PasteMsg{Content: "  abc123\n"})
	m = drive(t, mm.(SetupModel), "enter")
	if f.token != "abc123" || m.Profile() != "lab" {
		t.Fatalf("%q %q", f.token, m.Profile())
	}
}

func TestSetupStaysOnBadAddress(t *testing.T) {
	m := drive(t, newSetup(&fakeSetup{}, 100, 40), "enter", "enter")
	if m.step != stepServer {
		t.Fatal(m.step)
	}
	wants(t, setupText(m), "enter the address")
}

func TestSetupProblems(t *testing.T) {
	cases := []struct {
		err     error
		pasting bool
		want    string
	}{
		{&zabbix.Error{Kind: zabbix.KindAuth, Err: errors.New("x")}, false, "username or password"},
		{&zabbix.Error{Kind: zabbix.KindAuth, Err: errors.New("x")}, true, "rejected that token"},
		{&zabbix.Error{Kind: zabbix.KindPermission, Err: errors.New("x")}, false, "may not create API tokens"},
		{&zabbix.Error{Kind: zabbix.KindTransport, Err: errors.New("refused")}, false, "Cannot reach"},
		{&zabbix.Error{Kind: zabbix.KindHTTP, Err: errors.New("404")}, false, "does not look like a Zabbix"},
		{&zabbix.Error{Kind: zabbix.KindTransport, Err: errors.New("x509: unknown authority")}, false, "certificate is not trusted"},
		{errors.New("Zabbix 6.0 is not supported"), false, "not supported"},
	}
	for _, c := range cases {
		if got := setupProblem(c.err, c.pasting); !strings.Contains(got, c.want) {
			t.Errorf("%v: %q", c.err, got)
		}
	}
}

func TestSetupFailureClearsThePasswordAndAllowsARetry(t *testing.T) {
	f := &fakeSetup{connectErr: &zabbix.Error{Kind: zabbix.KindAuth, Err: errors.New("x")}}
	m := toSignIn(t, newSetup(f, 100, 40))
	m = typedSetup(t, m, "Admin")
	m = drive(t, m, "enter")
	m = typedSetup(t, m, "wrong")
	m = drive(t, m, "enter")
	if m.step != stepSignIn || m.password != "" || m.username != "Admin" {
		t.Fatalf("step %v user %q", m.step, m.username)
	}
	wants(t, setupText(m), "rejected that username")
	if m.Profile() != "" {
		t.Error("no profile until it succeeds")
	}
	f.connectErr = nil
	m = typedSetup(t, m, "right")
	if m = drive(t, m, "enter"); m.step != stepDone {
		t.Fatal(m.step)
	}
}

func TestSetupCertificateToggleAndBack(t *testing.T) {
	m := drive(t, newSetup(&fakeSetup{}, 100, 40), "enter", "tab")
	if m.verifyTLS {
		t.Fatal("tab must turn the check off")
	}
	wants(t, setupText(m), "Only for a server you trust")
	m = drive(t, m, "esc")
	if m.step != stepWelcome {
		t.Fatal(m.step)
	}
}

func TestSetupKeysAreIgnoredWhileBusy(t *testing.T) {
	m := toSignIn(t, newSetup(&fakeSetup{}, 100, 40))
	m.busy = "working"
	m = drive(t, m, "esc", "x")
	if m.step != stepSignIn || m.username != "" {
		t.Fatal("keys acted during a request")
	}
}

func TestSetupFitsSmallTerminals(t *testing.T) {
	m := toSignIn(t, newSetup(&fakeSetup{}, 80, 24))
	for i, l := range strings.Split(setupText(m), "\n") {
		if lipgloss.Width(l) > 80 {
			t.Fatalf("line %d is %d wide", i, lipgloss.Width(l))
		}
	}
	if n := lipgloss.Height(m.render()); n > 24 {
		t.Fatalf("%d lines high", n)
	}
}

func TestSetupASCII(t *testing.T) {
	m := NewSetup(&fakeSetup{}, "/tmp/config.toml", "nord", true)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(SetupModel)
	for i := range 3 {
		out := ansiRe.ReplaceAllString(m.View().Content, "")
		for _, r := range out {
			if r > 0x7e && r != '\n' {
				t.Fatalf("step %d: non-ASCII %q", i, r)
			}
		}
		if strings.Contains(out, "V?gter") {
			t.Fatalf("step %d has an unmapped glyph", i)
		}
		if i == 0 {
			wants(t, out, "Vaegter creates")
		}
		m.step++
	}
}

func TestSetupQuitWithoutSaving(t *testing.T) {
	m := drive(t, newSetup(&fakeSetup{}, 100, 40), "q")
	if m.Profile() != "" {
		t.Fatal(fmt.Sprintf("profile %q", m.Profile()))
	}
}
