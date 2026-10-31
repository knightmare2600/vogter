package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/theme"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

// Vectors from zabterm's src/ui/fmt.rs tests.
func TestFormatsMatchUpstream(t *testing.T) {
	cases := map[string]string{
		fmtBytes(8393289728):               "7.8 GiB",
		fmtBits(2592):                      "2.6 Kbps",
		fmtDuration(337):                   "5m",
		fmtDuration(3*86400 + 7200 + 60):   "3d 2h",
		fmtPct(model.Some(1.7359)):         "1.7%",
		fmtNumber(10):                      "10",
		fmtDuration(45):                    "45s",
		fmtDuration(3600):                  "1h",
		fmtDuration(3900):                  "1h 5m",
		fmtDuration(86400):                 "1d",
		fmtDuration(-5):                    "0s",
		fmtPct(model.Metric{}):             "-",
		fmtPct(model.Some(99.96)):          "100%",
		fmtPct(model.Some(42.4)):           "42%",
		fmtNumber(123.456):                 "123.5",
		fmtNumber(1.5):                     "1.50",
		fmtBytes(512):                      "512 B",
		fmtBytes(1024 * 150):               "150 KiB",
		fmtMetric(model.Metric{}, fmtBits): "-",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestItemValue(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	cases := []struct {
		it   zabbix.Item
		want string
	}{
		{zabbix.Item{LastClock: "0"}, "-"},
		{zabbix.Item{LastClock: "1", ValueType: "1", LastValue: "a\nb"}, "a b"},
		{zabbix.Item{LastClock: "1", ValueType: "4"}, `""`},
		{zabbix.Item{LastClock: "1", ValueType: "0", LastValue: "12.34", Units: "%"}, "12.3%"},
		{zabbix.Item{LastClock: "1", ValueType: "3", LastValue: "2048", Units: "Bps"}, "2.0 KiB/s"},
		{zabbix.Item{LastClock: "1", ValueType: "3", LastValue: "5", Units: "rpm"}, "5 rpm"},
		{zabbix.Item{LastClock: "1", ValueType: "0", LastValue: "x"}, "x"},
	}
	for _, c := range cases {
		if got := fmtItemValue(c.it, now); got != c.want {
			t.Errorf("%+v: %q, want %q", c.it, got, c.want)
		}
	}
}

func TestNiceCeil(t *testing.T) {
	for in, want := range map[float64]float64{0: 1, 3: 5, 1: 1, 1.5: 2, 2.2: 2.5, 7: 10, 110: 200, 0.03: 0.05} {
		if got := niceCeil(in); got < want*0.999 || got > want*1.001 {
			t.Errorf("niceCeil(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestChartTop(t *testing.T) {
	s := func(v float64) []chartSeries { return []chartSeries{{points: []model.Point{{T: 1, V: v}}}} }
	if got := chartTop(s(95), 5, yPercent); got != 100 {
		t.Errorf("percent charts cap at 100: %v", got)
	}
	if got := chartTop(s(150), 5, yPercent); got != 200 {
		t.Errorf("over 100%% is allowed: %v", got)
	}
	if got := chartTop(s(0.1), 5, yPercent); got != 5 {
		t.Errorf("floor: %v", got)
	}
}

func TestYAxisLabels(t *testing.T) {
	if got := yPercent.label(2.5); got != "2.5%" {
		t.Error(got)
	}
	if got := yPercent.label(50); got != "50%" {
		t.Error(got)
	}
	if got := yBits.label(2000); got != "2.0 Kbps" {
		t.Error(got)
	}
	if got := yPlain.label(1.25); got != "1.25" {
		t.Error(got)
	}
}

func TestTimeChartSize(t *testing.T) {
	th := theme.Load("nord")
	pts := []model.Point{{T: 100, V: 10}, {T: 150, V: 90}, {T: 200, V: 40}, {T: 50, V: 99}}
	for _, size := range [][2]int{{30, 11}, {60, 20}, {12, 5}, {5, 3}} {
		out := timeChart(&th, size[0], size[1], panelOpts{}, []chartSeries{{"a", pts, th.Accent}, {"b", pts[:2], th.OK}}, 100, 200, 5, yPercent)
		if len(out) != size[1] {
			t.Errorf("%v: %d lines", size, len(out))
		}
		for _, l := range out {
			if lipgloss.Width(l) != size[0] {
				t.Errorf("%v: width %d", size, lipgloss.Width(l))
			}
		}
	}
	empty := strings.Join(timeChart(&th, 30, 8, panelOpts{}, []chartSeries{{name: "a"}}, 0, 1, 5, yPercent), "\n")
	if !strings.Contains(empty, "no data yet") {
		t.Error(empty)
	}
}

func TestCanvasDots(t *testing.T) {
	th := theme.Load("nord")
	c := newCanvas(2, 1)
	c.set(0, 0, th.Accent)
	c.set(1, 3, th.Accent)
	c.set(2, 1, th.OK)
	c.set(99, 99, th.OK) // clipped
	if c.dots[0] != 0x01|0x80 || c.dots[1] != 0x02 {
		t.Errorf("dots = %x", c.dots)
	}
	l := newCanvas(4, 1)
	l.line(0, 0, 7, 3, th.Accent)
	n := 0
	for _, d := range l.dots {
		if d != 0 {
			n++
		}
	}
	if n != 4 {
		t.Errorf("line touched %d cells", n)
	}
}

func TestSparkline(t *testing.T) {
	th := theme.Load("nord")
	base := lipgloss.NewStyle()
	strip := func(s string) string { return ansiRe.ReplaceAllString(s, "") }
	if got := strip(sparkline(&th, base, nil, 4)); got != "····" {
		t.Error(got)
	}
	flat := []model.Point{{V: 3}, {V: 3}}
	if got := strip(sparkline(&th, base, flat, 4)); len([]rune(got)) != 4 || !strings.HasPrefix(got, "  ") {
		t.Errorf("%q", got)
	}
	var many []model.Point
	for i := 0; i < 20; i++ {
		many = append(many, model.Point{V: float64(i * 5)})
	}
	got := []rune(strip(sparkline(&th, base, many, 8)))
	if len(got) != 8 || got[0] != '▁' || got[7] != '█' {
		t.Errorf("%q", string(got))
	}
}

func TestBigText(t *testing.T) {
	th := theme.Load("nord")
	strip := func(s string) string { return ansiRe.ReplaceAllString(s, "") }
	out := bigText(&th, "12", "%", th.FG)
	if len(out) != 3 {
		t.Fatal(len(out))
	}
	if got := strip(out[0]); got != "  ▀█  ▀▀█  " {
		t.Errorf("%q", got)
	}
	if got := strip(out[2]); got != "  ▀▀▀ ▀▀▀ %" {
		t.Errorf("%q", got)
	}
}

func TestMeter(t *testing.T) {
	th := theme.Load("nord")
	base := lipgloss.NewStyle()
	strip := func(s string) string { return ansiRe.ReplaceAllString(s, "") }
	for _, c := range []struct {
		m    model.Metric
		want string
	}{
		{model.Metric{}, "──────────"},
		{model.Some(0), "──────────"},
		{model.Some(0.1), "━─────────"},
		{model.Some(50), "━━━━━─────"},
		{model.Some(150), "━━━━━━━━━━"},
	} {
		if got := strip(meter(&th, base, c.m, 10)); got != c.want {
			t.Errorf("%+v: %q", c.m, got)
		}
	}
}

func TestScroller(t *testing.T) {
	var s scroller
	if a, b := s.window(0, 5, 20); a != 0 || b != 5 {
		t.Error(a, b)
	}
	if a, _ := s.window(7, 5, 20); a != 3 {
		t.Error(a)
	}
	// Moving up inside the view does not move it.
	if a, _ := s.window(5, 5, 20); a != 3 {
		t.Error(a)
	}
	if a, _ := s.window(1, 5, 20); a != 1 {
		t.Error(a)
	}
	s.offset = 15
	if a, b := s.window(2, 5, 4); a != 0 || b != 4 {
		t.Error(a, b)
	}
}

func TestSplit(t *testing.T) {
	if got := split(150, 37, 37, 26); got[0] != 56 || got[1] != 55 || got[2] != 39 {
		t.Errorf("%v, want zabterm's 56 55 39", got)
	}
	if got := split(100, 50, 50); got[0]+got[1] != 100 {
		t.Errorf("%v", got)
	}
}

func TestFmtAxisTime(t *testing.T) {
	ts := float64(time.Date(2026, 10, 24, 11, 3, 0, 0, time.Local).Unix())
	if got := fmtAxisTime(ts, 24*3600); got != "11:03" {
		t.Errorf("24h: %q", got)
	}
	if got := fmtAxisTime(ts, 7*24*3600); got != "Oct 24" {
		t.Errorf("7d: %q", got)
	}
}
