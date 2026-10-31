package model

import (
	"reflect"
	"testing"

	"github.com/knightmare2600/vogter/internal/zabbix"
)

func field(pairs ...string) []zabbix.WidgetField {
	var out []zabbix.WidgetField
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, zabbix.WidgetField{Name: pairs[i], Value: pairs[i+1]})
	}
	return out
}

func TestBoardsFromSurvivesBadSizes(t *testing.T) {
	b := BoardsFrom([]zabbix.Dashboard{{DashboardID: "1", Name: "x", Pages: []zabbix.DashboardPage{{Widgets: []zabbix.Widget{
		{Type: "clock", X: "60", Y: "2", Width: "12", Height: "4"},
		{Type: "item", X: "bad", Y: "-3", Width: "0", Height: ""},
	}}}}})
	got := b[0].Pages[0].Widgets
	if got[0].X != 60 || got[0].W != 12 || got[1].X != 0 || got[1].Y != 0 || got[1].W != 1 || got[1].H != 1 {
		t.Fatalf("%+v", got)
	}
	if b[0].Pages[0].GridRows() != 6 || b[0].Widgets() != 2 {
		t.Errorf("rows %d widgets %d", b[0].Pages[0].GridRows(), b[0].Widgets())
	}
}

func TestWidgetFields(t *testing.T) {
	w := Widget{Type: "svggraph", Fields: field("ds.0.hosts.0", "a", "ds.0.items.0", "cpu", "ds.0.items.1", "mem", "ds.2.color", "fff", "hostids.0", "1", "hostids.x", "no")}
	if w.Datasets() != 3 {
		t.Errorf("datasets %d", w.Datasets())
	}
	if got := w.Dataset(0, "items"); !reflect.DeepEqual(got, []string{"cpu", "mem"}) {
		t.Errorf("%v", got)
	}
	if got := w.All("hostids"); !reflect.DeepEqual(got, []string{"1"}) {
		t.Errorf("%v", got)
	}
	if w.Title() != "Graph" || (Widget{Type: "odd"}).Title() != "odd" || (Widget{Name: "Mine"}).Title() != "Mine" {
		t.Error("title")
	}
}

func TestParseRange(t *testing.T) {
	for in, want := range map[string]int64{
		"now-1h": 3600, "now-30m": 1800, "now-7d": 604800, "now-1w": 604800, "now-45s": 45,
		"": DefaultWidgetRange, "now": DefaultWidgetRange, "now/d": DefaultWidgetRange, "2026-01-01 00:00:00": DefaultWidgetRange, "now-xh": DefaultWidgetRange, "now-0h": DefaultWidgetRange, "now-5q": DefaultWidgetRange,
	} {
		if got := ParseRange(in); got != want {
			t.Errorf("%q: %d", in, got)
		}
	}
}

func TestGlob(t *testing.T) {
	cases := []struct {
		p, s string
		want bool
	}{
		{"Zabbix server", "zabbix SERVER", true},
		{"Zabbix server", "Zabbix server 2", false},
		{"web-*", "web-01", true},
		{"*cache*% used", "Value cache % used", true},
		{"*cache*% used", "cache free", false},
		{"Utilization of * data collector *", "Utilization of poller data collector processes, in %", true},
		{"a*a", "a", false},
		{"*", "", true},
	}
	for _, c := range cases {
		if Glob(c.p, c.s) != c.want {
			t.Errorf("%q %q", c.p, c.s)
		}
	}
}

func TestWidgetProblems(t *testing.T) {
	problems := []Problem{{EventID: "1", HostID: "a", Severity: High}, {EventID: "2", HostID: "b", Severity: Warning}}
	if got := WidgetProblems(Widget{}, problems); len(got) != 2 {
		t.Errorf("no filter: %d", len(got))
	}
	if got := WidgetProblems(Widget{Fields: field("hostids.0", "b")}, problems); len(got) != 1 || got[0].EventID != "2" {
		t.Errorf("%+v", got)
	}
	if got := WidgetProblems(Widget{Fields: field("severities.0", "4")}, problems); len(got) != 1 || got[0].EventID != "1" {
		t.Errorf("%+v", got)
	}
}

func TestCountAvail(t *testing.T) {
	hosts := []zabbix.Host{
		{Status: "0", Interfaces: []zabbix.Interface{{Type: "1", Available: "1"}, {Type: "2", Available: "2"}}},
		{Status: "0", Interfaces: []zabbix.Interface{{Type: "1", Available: "0"}}},
		{Status: "1", Interfaces: []zabbix.Interface{{Type: "1", Available: "2"}}},
	}
	if got := CountAvail(hosts, []string{"1"}); got != (AvailCount{Up: 1, Unknown: 1}) {
		t.Errorf("%+v", got)
	}
	if got := CountAvail(hosts, nil); got != (AvailCount{Up: 1, Down: 1, Unknown: 1}) {
		t.Errorf("%+v", got)
	}
}

func TestBuildTop(t *testing.T) {
	w := Widget{Fields: field("columns.0.name", "Host", "columns.0.data", "2", "columns.1.name", "CPU", "columns.1.data", "1", "columns.1.item", "cpu")}
	cols := TopColumns(w)
	if len(cols) != 2 || !cols[0].Host || cols[1].Item != "cpu" {
		t.Fatalf("%+v", cols)
	}
	hosts := map[string]string{"1": "low", "2": "high", "3": "empty"}
	items := []zabbix.Item{
		{HostID: "1", Name: "cpu", LastValue: "5", LastClock: "1"},
		{HostID: "2", Name: "cpu", LastValue: "50", LastClock: "1"},
		{HostID: "9", Name: "cpu", LastValue: "99", LastClock: "1"},
	}
	top := BuildTop(cols, 1, hosts, items, 1)
	if len(top.Rows) != 1 || top.Rows[0].Host != "high" {
		t.Fatalf("%+v", top.Rows)
	}
}
