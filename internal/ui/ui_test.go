package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/platform"
	"github.com/knightmare2600/vogter/internal/poller"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

var testNow = time.Unix(1_800_000_000, 0)

type ackCall struct {
	ids     []string
	message string
	close   bool
}

func newModel(ack AckFunc) (Model, chan poller.Cmd) {
	cmds := make(chan poller.Cmd, 1)
	m := New(Options{Profile: "lab", Endpoint: "http://zbx/api_jsonrpc.php", ConfigPath: "/tmp/config.toml", Theme: "nord", Interval: 10 * time.Second, Cmds: cmds, Ack: ack})
	m.now = func() time.Time { return testNow }
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	return mm.(Model), cmds
}

func fixture() model.Snapshot {
	hist := []model.Point{{T: float64(testNow.Unix() - 600), V: 20}, {T: float64(testNow.Unix() - 300), V: 40}, {T: float64(testNow.Unix() - 60), V: 35}}
	return model.Snapshot{
		Hosts: []model.HostRow{
			{ID: "1", Name: "web-01", Groups: []string{"Linux servers", "Web"}, Address: "10.0.0.11:10050", Enabled: true, Availability: model.Up,
				CPU: model.Some(35), Mem: model.Some(61), Disk: model.Some(71), Load: model.Some(1.5), Uptime: model.Some(90000), NetIn: model.Some(12000), NetOut: model.Some(800),
				CPUHist: hist, MemHist: hist, Problems: [6]int{model.Average: 1}},
			{ID: "2", Name: "db-01", Groups: []string{"Linux servers"}, Address: "db-01.lan:10050", Enabled: true, Maintenance: true, Availability: model.Down,
				Error: "Get value from agent failed", CPU: model.Some(99), Problems: [6]int{model.High: 1}},
			{ID: "3", Name: "switch-01", Groups: []string{"Network"}, Address: "10.0.0.2:161", Enabled: false},
		},
		Problems: []model.Problem{
			{EventID: "502", Name: "Zabbix agent is not available", Severity: model.High, Clock: testNow.Unix() - 120, HostID: "2", Host: "db-01"},
			{EventID: "501", Name: "High CPU utilisation", Severity: model.Average, Clock: testNow.Unix() - 600, HostID: "1", Host: "web-01", ManualClose: true, Acknowledged: true, OpData: "Current: 93%", Tags: []string{"scope:performance"}},
			{EventID: "503", Name: "Backups late", Severity: model.Warning, Clock: testNow.Unix() - 9000, HostID: "1", Host: "web-01", Suppressed: true},
		},
		FetchedAt: testNow.Add(-3 * time.Second),
		Latency:   42 * time.Millisecond,
	}
}

func live(m Model) Model {
	mm, _ := m.Update(eventMsg{poller.Snapshot{Snapshot: fixture()}})
	return mm.(Model)
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "ctrl+d":
		return tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	return tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
}

func press(m Model, keys ...string) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		var mm tea.Model
		mm, cmd = m.Update(keyMsg(k))
		m = mm.(Model)
	}
	return m, cmd
}

func typed(m Model, s string) Model {
	for _, r := range s {
		m, _ = press(m, string(r))
	}
	return m
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;:]*[A-Za-z]`)

func plain(m Model) string { return ansiRe.ReplaceAllString(m.render(), "") }

func wants(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("screen lacks %q:\n%s", w, out)
		}
	}
}

func TestViewNavigation(t *testing.T) {
	m, _ := newModel(nil)
	steps := []struct {
		key  string
		want View
	}{
		{"2", Hosts}, {"3", Problems}, {"1", Dashboard}, {"tab", Hosts}, {"tab", Problems}, {"tab", Dashboard},
		{"shift+tab", Problems}, {"shift+tab", Hosts},
	}
	for _, s := range steps {
		m, _ = press(m, s.key)
		if m.view != s.want {
			t.Fatalf("after %q view = %v, want %v", s.key, m.view, s.want)
		}
	}
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		m, _ := newModel(nil)
		m, cmd := press(m, k)
		if !m.quitting || cmd == nil {
			t.Errorf("%s did not quit", k)
		}
	}
	m, _ := newModel(nil)
	m, _ = press(m, "?", "q")
	if m.help || m.quitting {
		t.Error("q in help must only close help")
	}
	m, _ = press(m, "?", "ctrl+c")
	if !m.quitting {
		t.Error("ctrl+c must quit from help")
	}
	m = live(m)
	m.quitting, m.help = false, false
	m, _ = press(m, "2", "/", "q")
	if m.quitting || m.hostFilter != "q" {
		t.Error("q while filtering is text")
	}
	m, _ = press(m, "ctrl+c")
	if !m.quitting {
		t.Error("ctrl+c must quit from the filter")
	}
}

func TestHelpSwallowsKeys(t *testing.T) {
	m, _ := newModel(nil)
	m, _ = press(m, "?", "2")
	if m.view != Dashboard || !m.help {
		t.Error("keys other than close must be ignored while help is open")
	}
	for _, k := range []string{"esc", "enter", "?"} {
		m, _ = press(m, "?")
		if !m.help {
			m, _ = press(m, "?")
		}
		m.help = true
		m, _ = press(m, k)
		if m.help {
			t.Errorf("%s must close help", k)
		}
	}
}

func TestRefreshSendsOnce(t *testing.T) {
	m, cmds := newModel(nil)
	m, _ = press(m, "r", "r", "r")
	if len(cmds) != 1 {
		t.Errorf("%d refreshes queued, want 1", len(cmds))
	}
	if !m.loading {
		t.Error("refresh must show the sync state")
	}
}

func TestThemeCycle(t *testing.T) {
	m, _ := newModel(nil)
	t.Setenv("HOME", t.TempDir())
	m, _ = press(m, "t")
	if m.theme.Name != "everforest" {
		t.Errorf("theme after nord = %q", m.theme.Name)
	}
	if len(m.toasts) != 1 || m.toasts[0].body != "everforest" {
		t.Errorf("toasts = %+v", m.toasts)
	}
}

func TestSplashAndErrors(t *testing.T) {
	m, _ := newModel(nil)
	wants(t, plain(m), "connecting to http://zbx/api_jsonrpc.php")

	err := &zabbix.Error{Kind: zabbix.KindAuth, Method: "host.get", Message: "Invalid params.", Data: "Not authorized."}
	for i := 0; i < 3; i++ {
		mm, _ := m.Update(eventMsg{poller.Failed{Err: err}})
		m = mm.(Model)
	}
	if len(m.toasts) != 1 {
		t.Errorf("a repeating error raised %d toasts, want 1", len(m.toasts))
	}
	wants(t, plain(m), "offline", "Not authorized.", "check the token", "retrying on the next refresh")

	v, _ := zabbix.ParseVersion("6.4.9")
	mm, _ := m.Update(eventMsg{poller.Connected{Version: v, Support: zabbix.SupportFor(v)}})
	m = live(mm.(Model))
	out := plain(m)
	wants(t, out, "live", "Zabbix 6.4.9", "42ms", "Hosts 3", "Problems 2", "updated 3s ago", "every 10s", "untested")
	if strings.Contains(out, "offline") {
		t.Error("a snapshot must clear the error state")
	}
}

func TestFailureKeepsLastSnapshot(t *testing.T) {
	m, _ := newModel(nil)
	m = live(m)
	mm, _ := m.Update(eventMsg{poller.Failed{Err: errors.New("boom")}})
	m = mm.(Model)
	wants(t, plain(m), "offline", "web-01")
}

func TestDashboard(t *testing.T) {
	m, _ := newModel(nil)
	m = live(m)
	wants(t, plain(m), "Availability", "Avg CPU", "Avg Memory", "Problems by severity", "Fleet CPU · last 30m", "show memory",
		"Latest problems", "1 down", "1 unreachable", "1 unacked · HIGH", "peak", "Zabbix agent is not available", "web-01", "CPU · 30m")
	m, _ = press(m, "m")
	wants(t, plain(m), "Fleet Memory · last 30m", "show cpu")
	if !containsBraille(plain(m)) {
		t.Error("chart drew nothing")
	}
}

func containsBraille(s string) bool {
	for _, r := range s {
		if r > 0x2800 && r <= 0x28ff {
			return true
		}
	}
	return false
}

func TestDashboardAllClear(t *testing.T) {
	m, _ := newModel(nil)
	snap := fixture()
	snap.Problems = nil
	mm, _ := m.Update(eventMsg{poller.Snapshot{Snapshot: snap}})
	wants(t, plain(mm.(Model)), "All clear", "all clear")
}

func TestMissingMetricsShowDash(t *testing.T) {
	m, _ := newModel(nil)
	m = live(m)
	m, _ = press(m, "2", "G")
	out := plain(m)
	wants(t, out, "switch-01", "disabled")
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "switch-01") && strings.Contains(line, "0.0%") {
			t.Errorf("a missing metric must not read as zero: %s", line)
		}
	}
}

func TestHostsView(t *testing.T) {
	m, _ := newModel(nil)
	m = live(m)
	m, _ = press(m, "2")
	wants(t, plain(m), "3 of 3 hosts", "problems", "Interface", "10.0.0.11:10050", "Get value from agent failed", "⏎ details", "CPU  99%", "maintenance")

	// Problems sort: db-01 (High) first, then web-01.
	m, _ = press(m, "j")
	wants(t, plain(m), "CPU  35%", "12.0 Kbps")

	m, _ = press(m, "s")
	if m.hostSort != model.SortCPU || m.hostSel != 0 {
		t.Errorf("sort = %v sel = %d", m.hostSort, m.hostSel)
	}
	m, _ = press(m, "/")
	m = typed(m, "NETW")
	if m.hostSel != 0 {
		t.Error("selection must stay in range")
	}
	wants(t, plain(m), "1 of 3 hosts", "⏎ keep  esc clear", "filter")
	m, _ = press(m, "enter")
	wants(t, plain(m), "switch-01")
	if m.editing || m.hostFilter != "NETW" {
		t.Errorf("enter must keep the filter: %+v", m.hostFilter)
	}
	m, _ = press(m, "esc")
	if m.hostFilter != "" {
		t.Error("esc outside editing clears the filter")
	}
	m, _ = press(m, "/", "x", "esc")
	if m.hostFilter != "" || m.editing {
		t.Error("esc while editing clears and leaves")
	}
}

func TestFilterText(t *testing.T) {
	m, _ := newModel(nil)
	m = live(m)
	m, _ = press(m, "2", "/", "a", "space", "é", "backspace", "backspace")
	if m.hostFilter != "a" {
		t.Errorf("filter = %q", m.hostFilter)
	}
	m, _ = press(m, "enter", "1", "/")
	if m.editing {
		t.Error("the dashboard has no filter")
	}
	mm, _ := m.Update(tea.PasteMsg{Content: "ignored"})
	m = mm.(Model)
	m, _ = press(m, "2", "/")
	mm, _ = m.Update(tea.PasteMsg{Content: "b\nc"})
	if got := mm.(Model).hostFilter; got != "ab c" {
		t.Errorf("paste: %q", got)
	}
}

func TestMovement(t *testing.T) {
	m, _ := newModel(nil)
	snap := fixture()
	for i := 0; i < 40; i++ {
		snap.Hosts = append(snap.Hosts, model.HostRow{ID: fmt.Sprint(100 + i), Name: fmt.Sprintf("n%02d", i), Enabled: true})
	}
	mm, _ := m.Update(eventMsg{poller.Snapshot{Snapshot: snap}})
	m = mm.(Model)
	m, _ = press(m, "2", "j", "down")
	if m.hostSel != 2 {
		t.Errorf("sel = %d", m.hostSel)
	}
	m, _ = press(m, "k", "up", "k")
	if m.hostSel != 0 {
		t.Errorf("sel = %d, must clamp at the top", m.hostSel)
	}
	m, _ = press(m, "pgdown", "ctrl+d")
	if m.hostSel != 20 {
		t.Errorf("sel = %d", m.hostSel)
	}
	m, _ = press(m, "G")
	if m.hostSel != 42 {
		t.Errorf("sel = %d", m.hostSel)
	}
	wants(t, plain(m), "n39") // scrolled into view
	m, _ = press(m, "home")
	if m.hostSel != 0 {
		t.Errorf("sel = %d", m.hostSel)
	}
	m, _ = press(m, "end", "g")
	if m.hostSel != 0 {
		t.Errorf("sel = %d", m.hostSel)
	}

	// A smaller snapshot pulls the selection back in range.
	m, _ = press(m, "G")
	mm, _ = m.Update(eventMsg{poller.Snapshot{Snapshot: fixture()}})
	if got := mm.(Model).hostSel; got != 2 {
		t.Errorf("clamped sel = %d", got)
	}
}

func TestProblemsView(t *testing.T) {
	m, _ := newModel(nil)
	m = live(m)
	m, _ = press(m, "3")
	wants(t, plain(m), "3 of 3 problems", "Not classified", "shown", "HIGH", "Severity", "Zabbix agent is not available", "Event #502", "unacknowledged", "acknowledge")

	m, _ = press(m, "j")
	wants(t, plain(m), "Opdata", "Current: 93%", "scope:performance", "Status acknowledged")

	m, _ = press(m, "+", "+", "+", "+")
	if m.minSeverity != model.High || m.problemSel != 0 {
		t.Errorf("min = %v sel = %d", m.minSeverity, m.problemSel)
	}
	wants(t, plain(m), "1 of 3 problems")
	m, _ = press(m, "s", "s")
	if m.minSeverity != model.NotClassified {
		t.Errorf("severity must wrap, got %v", m.minSeverity)
	}
	m, _ = press(m, "-")
	if m.minSeverity != model.Disaster {
		t.Errorf("severity must wrap down, got %v", m.minSeverity)
	}
	wants(t, plain(m), "Nothing to see here")
	m, _ = press(m, "+", "h")
	wants(t, plain(m), "2 of 3 problems", "hidden")
	m, _ = press(m, "/")
	m = typed(m, "perf")
	wants(t, plain(m), "0 of 3 problems")
}

func TestAcknowledge(t *testing.T) {
	var got ackCall
	ack := func(_ context.Context, ids []string, msg string, shut bool) error {
		got = ackCall{ids, msg, shut}
		return nil
	}
	m, cmds := newModel(ack)
	m = live(m)
	m, _ = press(m, "3", "a")
	wants(t, plain(m), "Acknowledge", "db-01 · Zabbix agent is not available", "trigger does not allow manual close", "confirm")
	m, _ = press(m, "tab")
	if m.ack.close {
		t.Error("tab must not arm close when the trigger forbids it")
	}
	m, _ = press(m, "esc")
	if m.ack != nil {
		t.Fatal("esc must cancel")
	}

	m, _ = press(m, "j", "a")
	wants(t, plain(m), "close problem too")
	m = typed(m, "on it")
	m, _ = press(m, "tab")
	m, cmd := press(m, "enter")
	if m.ack != nil || cmd == nil {
		t.Fatal("enter must send")
	}
	mm, _ := m.Update(cmd())
	m = mm.(Model)
	if len(got.ids) != 1 || got.ids[0] != "501" || got.message != "on it" || !got.close {
		t.Errorf("ack = %+v", got)
	}
	if len(cmds) != 1 {
		t.Error("a successful ack must refresh")
	}
	wants(t, plain(m), "Acknowledged", "1 problem")
}

func TestAcknowledgeFailure(t *testing.T) {
	perm := &zabbix.Error{Kind: zabbix.KindPermission, Method: "event.acknowledge", Message: "Application error.", Data: "No permissions"}
	m, cmds := newModel(func(context.Context, []string, string, bool) error { return perm })
	m = live(m)
	m, cmd := press(m, "3", "a", "enter")
	mm, _ := m.Update(cmd())
	m = mm.(Model)
	wants(t, plain(m), "Acknowledge failed: no permission", "event.acknowledge")
	if len(cmds) != 0 {
		t.Error("a failed ack must not refresh")
	}
	if m.loading {
		t.Error("a failed ack must clear the sync state")
	}
}

func TestToastsExpireAndCap(t *testing.T) {
	m, _ := newModel(nil)
	for range 6 {
		m, _ = press(m, "t")
	}
	if len(m.toasts) != maxToasts {
		t.Errorf("%d toasts", len(m.toasts))
	}
	later := testNow.Add(toastLife)
	m.now = func() time.Time { return later }
	mm, _ := m.Update(tickMsg(later))
	if n := len(mm.(Model).toasts); n != 0 {
		t.Errorf("%d toasts after expiry", n)
	}
}

func TestTruncatedProblems(t *testing.T) {
	m, _ := newModel(nil)
	snap := fixture()
	snap.Truncated = true
	mm, _ := m.Update(eventMsg{poller.Snapshot{Snapshot: snap}})
	m = mm.(Model)
	wants(t, plain(m), "Problems 2+")
	m, _ = press(m, "3")
	wants(t, plain(m), "of 3+ problems")
}

func TestEveryScreenFitsTheTerminal(t *testing.T) {
	sizes := [][2]int{{80, 24}, {100, 30}, {129, 35}, {130, 40}, {170, 50}, {240, 70}}
	for _, size := range sizes {
		for _, keys := range [][]string{{"1"}, {"2"}, {"3"}, {"3", "a"}, {"?"}, {"1", "t"}} {
			w, h := size[0], size[1]
			m, _ := newModel(func(context.Context, []string, string, bool) error { return nil })
			mm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			m = live(mm.(Model))
			m, _ = press(m, keys...)
			lines := strings.Split(m.render(), "\n")
			if len(lines) != h {
				t.Errorf("%dx%d %v: %d lines", w, h, keys, len(lines))
			}
			for i, l := range lines {
				if lw := lipgloss.Width(l); lw > w {
					t.Errorf("%dx%d %v: line %d is %d wide: %s", w, h, keys, i, lw, ansiRe.ReplaceAllString(l, ""))
				}
			}
		}
	}
}

func TestTooSmall(t *testing.T) {
	m, _ := newModel(nil)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	wants(t, plain(live(mm.(Model))), "terminal too small")
}

func TestDescribe(t *testing.T) {
	if got := describe(errors.New("plain")); got != "plain" {
		t.Error(got)
	}
}

func detailFixture() model.HostDetail {
	now := float64(testNow.Unix())
	pts := []model.Point{{T: now - 1800, V: 20}, {T: now - 60, V: 42.5}}
	return model.HostDetail{
		HostID: "1", Range: 3600, CPU: pts, Mem: pts, Load: []model.Point{{T: now - 60, V: 1.25}},
		NetIn: []model.Point{{T: now - 60, V: 12000}}, NetOut: []model.Point{{T: now - 60, V: 800}},
		Disks: []model.Disk{{Mount: "/", Used: 71}, {Mount: "/var", Used: 95}},
		Facts: []model.Fact{{Label: "OS", Value: "Debian 13"}, {Label: "CPUs", Value: "8"}},
		Items: []zabbix.Item{
			{ItemID: "1", Name: "CPU utilization", Key: "system.cpu.util", LastValue: "42.5", LastClock: fmt.Sprint(testNow.Unix() - 60), Units: "%", ValueType: "0"},
			{ItemID: "2", Name: "OS name", Key: "system.sw.os", LastValue: "Debian", LastClock: fmt.Sprint(testNow.Unix() - 3700), ValueType: "4"},
			{ItemID: "3", Name: "Never polled", Key: "custom.item", LastValue: "", LastClock: "0", ValueType: "0"},
		},
	}
}

type watched struct {
	id  string
	rng int64
}

func hostModel() (Model, *[]watched) {
	calls := &[]watched{}
	m, _ := newModel(nil)
	m.opts.Watch = func(id string, r int64) { *calls = append(*calls, watched{id, r}) }
	return live(m), calls
}

func withDetail(m Model) Model {
	mm, _ := m.Update(eventMsg{poller.Detail{HostDetail: detailFixture()}})
	return mm.(Model)
}

func TestOpenHostAndBack(t *testing.T) {
	m, calls := hostModel()
	m, _ = press(m, "2", "j", "enter") // problems sort: db-01, then web-01
	if m.view != Host || m.detailHost != "1" || m.backTo != Hosts {
		t.Fatalf("view=%v host=%q", m.view, m.detailHost)
	}
	if got := (*calls)[0]; got != (watched{"1", 3600}) {
		t.Errorf("watch = %+v", got)
	}
	wants(t, plain(m), "loading history…", "› web-01")
	m = withDetail(m)
	wants(t, plain(m), "available", "OS Debian 13", "Graphs", "Latest data", "range 15m 1h 6h 24h 7d", "CPU  42%  · 1h", "Load 1m  1.25", "12.0 Kbps", "Filesystems", "/var", "95%", "Problems 2", "High CPU utilisation")
	for _, key := range []string{"esc", "h", "backspace"} {
		m, _ = press(m, "2", "enter")
		*calls = nil
		m, _ = press(m, key)
		if m.view != Hosts {
			t.Errorf("%s: view = %v", key, m.view)
		}
		if len(*calls) != 1 || (*calls)[0].id != "" {
			t.Errorf("%s must stop the watch: %+v", key, *calls)
		}
	}
	m, _ = press(m, "3", "enter")
	if m.view != Host || m.backTo != Problems || m.detailHost != "2" {
		t.Fatalf("from problems: %v %v %q", m.view, m.backTo, m.detailHost)
	}
	m, _ = press(m, "1")
	if m.view != Dashboard {
		t.Error("view keys leave the host page")
	}
	m, _ = press(m, "enter")
	if m.view != Host || m.backTo != Dashboard {
		t.Error("dashboard opens the selected host")
	}
}

func TestRanges(t *testing.T) {
	m, calls := hostModel()
	m, _ = press(m, "2", "j", "enter")
	m = withDetail(m)
	*calls = nil
	m, _ = press(m, "[", "[", "[")
	if m.rangeIdx != 0 || len(*calls) != 1 || (*calls)[0].rng != 900 {
		t.Errorf("idx=%d calls=%+v, clamp at the bottom", m.rangeIdx, *calls)
	}
	mm, _ := m.Update(eventMsg{poller.Detail{HostDetail: detailFixture()}})
	m = mm.(Model)
	if m.detail.Range != 3600 {
		t.Log("old detail retained")
	}
	*calls = nil
	m, _ = press(m, "]", "]", "]", "]", "]", "]")
	if m.rangeIdx != len(ranges)-1 || len(*calls) != 4 {
		t.Errorf("idx=%d calls=%d, clamp at the top", m.rangeIdx, len(*calls))
	}
}

func TestLateDetailDropped(t *testing.T) {
	m, _ := hostModel()
	m, _ = press(m, "2", "enter") // db-01, id 2
	m = withDetail(m)             // detail for host 1
	if m.detail != nil {
		t.Error("detail for another host must be ignored")
	}
}

func TestLatestData(t *testing.T) {
	m, _ := hostModel()
	m, _ = press(m, "2", "j", "enter")
	m = withDetail(m)
	m, _ = press(m, "/")
	if m.editing {
		t.Error("the graphs tab has no filter")
	}
	m, _ = press(m, "tab")
	wants(t, plain(m), "Latest data  3 of 3", "system.cpu.util", "42.5%", "1m ago", "never", "1h 1m ago")
	m, _ = press(m, "j", "j", "j")
	if m.itemSel != 2 {
		t.Errorf("sel = %d", m.itemSel)
	}
	m, _ = press(m, "/")
	m = typed(m, "OS")
	wants(t, plain(m), "Latest data  1 of 3", "/OS", "OS name")
	if m.itemSel != 0 {
		t.Errorf("selection must stay in range: %d", m.itemSel)
	}
	m, _ = press(m, "enter", "i")
	if m.hostTab != tabGraphs {
		t.Error("i toggles the tab")
	}
	m, _ = press(m, "esc", "2", "enter")
	if m.itemFilter != "OS" {
		t.Error("returning to the same host keeps its filter")
	}
	m, _ = press(m, "esc", "k", "enter")
	if m.itemFilter != "" {
		t.Error("another host starts clean")
	}
}

func TestHostPageMissingData(t *testing.T) {
	m, _ := hostModel()
	m, _ = press(m, "2", "j", "enter")
	d := detailFixture()
	d.CPU, d.Mem, d.Load, d.NetIn, d.NetOut, d.Disks, d.Items, d.Facts = nil, nil, nil, nil, nil, nil, nil, nil
	mm, _ := m.Update(eventMsg{poller.Detail{HostDetail: d}})
	m = mm.(Model)
	wants(t, plain(m), "CPU  -", "no data yet", "no filesystems", "Network  ↓ -  ↑ -")
	if strings.Contains(plain(m), "CPU  0") {
		t.Error("missing metrics must not read as zero")
	}
}

func TestHostVanishes(t *testing.T) {
	m, _ := hostModel()
	m, _ = press(m, "2", "j", "enter")
	m = withDetail(m)
	snap := fixture()
	snap.Hosts = snap.Hosts[1:]
	mm, _ := m.Update(eventMsg{poller.Snapshot{Snapshot: snap}})
	if out := plain(mm.(Model)); !strings.Contains(out, "Host") {
		t.Error(out)
	}
}

func TestHostPageFits(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {100, 30}, {130, 40}, {170, 50}} {
		for _, keys := range [][]string{{}, {"tab"}, {"]", "]", "]"}} {
			m, _ := hostModel()
			mm, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m, _ = press(mm.(Model), "2", "j", "enter")
			m = withDetail(m)
			m, _ = press(m, keys...)
			lines := strings.Split(m.render(), "\n")
			if len(lines) != size[1] {
				t.Errorf("%v %v: %d lines", size, keys, len(lines))
			}
			for i, l := range lines {
				if lipgloss.Width(l) > size[0] {
					t.Errorf("%v %v: line %d too wide: %d", size, keys, i, lipgloss.Width(l))
				}
			}
		}
	}
}

func TestNotifications(t *testing.T) {
	type sent struct {
		title  string
		urgent bool
	}
	var got []sent
	m, _ := newModel(nil)
	m.opts.Desktop = true
	m.opts.MinSeverity = model.Average
	m.opts.Notify = func(title, _ string, urgent bool) error {
		got = append(got, sent{title, urgent})
		return nil
	}
	m = live(m)
	if len(got) != 0 || len(m.toasts) != 0 {
		t.Fatalf("the first snapshot announces nothing: %+v", got)
	}
	snap := fixture()
	snap.Problems = append(snap.Problems, model.Problem{EventID: "900", Name: "Disk full", Severity: model.Disaster, Host: "web-01", Clock: testNow.Unix()})
	for _, repeat := range []int{1, 2} {
		mm, _ := m.Update(eventMsg{poller.Snapshot{Snapshot: snap}})
		m = mm.(Model)
		if len(got) != 1 {
			t.Fatalf("poll %d: %+v", repeat, got)
		}
	}
	if got[0] != (sent{"Disaster · web-01", true}) {
		t.Errorf("%+v", got[0])
	}
	wants(t, plain(m), "▲ Disaster · web-01", "Disk full")

	m.opts.Desktop = false
	mm, _ := m.Update(eventMsg{poller.Snapshot{Snapshot: fixture()}})
	m = mm.(Model)
	if len(got) != 1 {
		t.Error("desktop off keeps toasts only")
	}
	wants(t, plain(m), "Resolved · web-01")
}

func TestNotifierMissingOrUnsupported(t *testing.T) {
	m, _ := newModel(nil)
	m.opts.Desktop = true
	m.opts.MinSeverity = model.Average
	m.opts.Notify = func(string, string, bool) error { return platform.ErrUnsupported }
	m = live(m)
	snap := fixture()
	snap.Problems = append(snap.Problems, model.Problem{EventID: "900", Severity: model.High}, model.Problem{EventID: "901", Severity: model.High})
	mm, _ := m.Update(eventMsg{poller.Snapshot{Snapshot: snap}})
	m = mm.(Model)
	n := 0
	for _, t := range m.toasts {
		if t.title == "Desktop notifications" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("unsupported notice shown %d times", n)
	}
}

func TestOpenInBrowser(t *testing.T) {
	var opened []string
	m, _ := newModel(nil)
	m.opts.WebURL = "https://z"
	m.opts.Open = func(u string) error { opened = append(opened, u); return nil }
	m = live(m)
	m, _ = press(m, "o", "2", "o", "3", "o")
	m, _ = press(m, "enter", "o")
	want := []string{
		"https://z/zabbix.php?action=dashboard.view",
		"https://z/zabbix.php?action=latest.view&hostids%5B%5D=2",
		"https://z/zabbix.php?action=problem.view&hostids%5B%5D=2",
		"https://z/zabbix.php?action=latest.view&hostids%5B%5D=2",
	}
	if strings.Join(opened, " ") != strings.Join(want, " ") {
		t.Errorf("%v", opened)
	}
	snap := fixture()
	snap.Problems[0].HostID = ""
	mm, _ := m.Update(eventMsg{poller.Snapshot{Snapshot: snap}})
	m = mm.(Model)
	m, _ = press(m, "3", "o")
	if got := opened[len(opened)-1]; got != "https://z/zabbix.php?action=problem.view" {
		t.Errorf("no host must not build an empty hostids (D21): %s", got)
	}
	m.opts.Open = func(string) error { return errors.New("no xdg-open") }
	m, _ = press(m, "o")
	wants(t, plain(m), "Could not open browser", "no xdg-open")
}

func TestThemeLiveReload(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".local", "state", "omarchy", "current")
	if err := os.MkdirAll(filepath.Join(dir, "theme"), 0o700); err != nil {
		t.Fatal(err)
	}
	colours := func(name, bg string) {
		body := fmt.Sprintf("background=%q\nforeground=\"#ffffff\"\nred=\"#ff0000\"\ngreen=\"#00ff00\"\nyellow=\"#ffff00\"\nblue=\"#0000ff\"\nmagenta=\"#ff00ff\"\ncyan=\"#00ffff\"\nmuted=\"#888888\"\n", bg)
		if err := os.WriteFile(filepath.Join(dir, "theme", "colors.toml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "theme.name"), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	colours("first", "#111111")
	m := New(Options{Theme: "auto", Cmds: make(chan poller.Cmd, 1)})
	m.now = func() time.Time { return testNow }
	if m.theme.Name != "omarchy:first" {
		t.Fatalf("%q", m.theme.Name)
	}
	colours("second", "#222222")
	m.frame = 7
	mm, _ := m.Update(tickMsg(testNow))
	m = mm.(Model)
	if m.theme.Name != "omarchy:second" {
		t.Errorf("theme = %q", m.theme.Name)
	}
	if len(m.toasts) != 1 {
		t.Errorf("toasts = %+v", m.toasts)
	}
}

func TestASCIIFallback(t *testing.T) {
	for _, keys := range [][]string{{"1"}, {"2"}, {"3"}, {"3", "a"}, {"?"}} {
		m, _ := newModel(func(context.Context, []string, string, bool) error { return nil })
		m = live(m)
		m, _ = press(m, keys...)
		out := ansiRe.ReplaceAllString(asciiOnly(m.render()), "")
		for _, r := range out {
			if r > 0x7e {
				t.Fatalf("%v: %q left in the output", keys, r)
			}
		}
		for i, l := range strings.Split(out, "\n") {
			if len([]rune(l)) > 140 {
				t.Errorf("%v: line %d grew to %d", keys, i, len([]rune(l)))
			}
		}
	}
}
