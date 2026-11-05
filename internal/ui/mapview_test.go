package ui

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/knightmare2600/vogter/internal/mapterm"
	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/poller"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

// fakeTiles serves one tile for every request: a water square and a motorway,
// encoded as a real vector tile.
type fakeTiles struct{ calls atomic.Int32 }

func (f *fakeTiles) MaxZoom() int { return 14 }

func (f *fakeTiles) Tile(context.Context, mapterm.TileID) ([]byte, error) {
	f.calls.Add(1)
	return testTile, nil
}

// testTile: layer "transportation" with one motorway across the middle.
var testTile = func() []byte {
	varint := func(b []byte, v uint64) []byte {
		for v >= 0x80 {
			b = append(b, byte(v)|0x80)
			v >>= 7
		}
		return append(b, byte(v))
	}
	field := func(b []byte, n int, data []byte) []byte {
		b = varint(b, uint64(n<<3|2))
		b = varint(b, uint64(len(data)))
		return append(b, data...)
	}
	packed := func(vs ...uint64) []byte {
		var b []byte
		for _, v := range vs {
			b = varint(b, v)
		}
		return b
	}
	zz := func(v int64) uint64 { return uint64((v << 1) ^ (v >> 63)) }
	var feat []byte
	feat = field(feat, 2, packed(0, 0))
	feat = append(feat, 3<<3, 2)
	feat = field(feat, 4, packed(1|1<<3, zz(0), zz(2048), 2|1<<3, zz(4096), zz(0)))
	var value []byte
	value = field(value, 1, []byte("motorway"))
	var layer []byte
	layer = field(layer, 1, []byte("transportation"))
	layer = field(layer, 2, feat)
	layer = field(layer, 3, []byte("class"))
	layer = field(layer, 4, value)
	return field(nil, 3, layer)
}()

func mapModel(t *testing.T, enabled bool) (Model, *fakeTiles) {
	t.Helper()
	src := &fakeTiles{}
	odense := mapterm.LatLon{Lat: 55.396, Lon: 10.386}
	m := New(Options{Profile: "lab", Theme: "nord", Cmds: make(chan poller.Cmd, 1), Map: MapOptions{
		Enabled: enabled, NewSource: func() mapterm.Source { return src }, Center: &odense, Zoom: 12, Labels: true,
	}})
	m.now = func() time.Time { return testNow }
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	m = live(mm.(Model))
	return m, src
}

// runAll runs a command and every command it batches, feeding the messages
// back in, as the program would.
func runAll(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0 && steps < 200; steps++ {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case nil:
		default:
			if _, tick := msg.(tickMsg); tick {
				continue
			}
			mm, next := m.Update(msg)
			m = mm.(Model)
			queue = append(queue, next)
		}
	}
	return m
}

func TestMapOffByDefaultContactsNobody(t *testing.T) {
	m, src := mapModel(t, false)
	m, cmd := press(m, "5")
	m = runAll(t, m, cmd)
	out := plain(m)
	wants(t, out, "5 Map", "The map is off", "your IP address", "Cloudflare", "© OpenStreetMap contributors")
	if src.calls.Load() != 0 || m.maps.loader != nil {
		t.Fatal("an off map must not even build its source")
	}
	m, cmd = press(m, "e")
	m = runAll(t, m, cmd)
	if !m.maps.on || src.calls.Load() == 0 {
		t.Fatal("e turns it on and fetches")
	}
	wants(t, plain(m), "Map on for this session")
}

func TestMapScreen(t *testing.T) {
	m, src := mapModel(t, true)
	if src.calls.Load() != 0 {
		t.Fatal("nothing is fetched until the map is shown")
	}
	m, cmd := press(m, "5")
	m = runAll(t, m, cmd)
	out := plain(m)
	wants(t, out, "z12", "55.3960N 10.3860E", "braille", "green", mapterm.Attribution)
	if src.calls.Load() == 0 || m.maps.loader.Pending() != 0 {
		t.Fatalf("tiles fetched and settled: %d calls, %d pending", src.calls.Load(), m.maps.loader.Pending())
	}
	for i, l := range strings.Split(out, "\n") {
		if lipgloss.Width(l) > 120 {
			t.Fatalf("line %d is %d wide", i, lipgloss.Width(l))
		}
	}
	before := m.maps.main.Center
	m, _ = press(m, "l", "l", "j")
	if m.maps.main.Center.Lon <= before.Lon || m.maps.main.Center.Lat >= before.Lat {
		t.Errorf("l pans east and j pans south: %v", m.maps.main.Center)
	}
	m, _ = press(m, "+", "+")
	if m.maps.main.Zoom != 14 {
		t.Errorf("zoom %v", m.maps.main.Zoom)
	}
	m, _ = press(m, "v")
	if m.maps.main.Mode != mapterm.ModeUnicode {
		t.Error("v cycles the mode")
	}
	m, _ = press(m, "P")
	if m.maps.main.Palette.Name != "solarized-dark" {
		t.Error(m.maps.main.Palette.Name)
	}
	m, _ = press(m, "L")
	if m.maps.main.Labels {
		t.Error("L toggles labels")
	}
	m, _ = press(m, "y")
	wants(t, plain(m), "Layers", "buildings")
	m, _ = press(m, "1", "x")
	if !m.maps.main.Hidden[mapterm.LayerBuildings] || m.maps.layers {
		t.Error("1 hides buildings and another key closes the menu")
	}
	if u := m.webURL(); !strings.HasPrefix(u, "https://www.openstreetmap.org/#map=14/") {
		t.Error(u)
	}
}

func TestMapGoToAndBack(t *testing.T) {
	m, _ := mapModel(t, true)
	m, _ = press(m, "5", ":")
	m = typed(m, "51.5074, -0.1278, 10")
	wants(t, plain(m), "go to", "51.5074, -0.1278, 10")
	m, _ = press(m, "enter")
	if c := m.maps.main.Center; c.Lat != 51.5074 || m.maps.main.Zoom != 10 {
		t.Fatalf("%v %v", c, m.maps.main.Zoom)
	}
	m, _ = press(m, "b")
	if m.maps.main.Center.Lat != 55.396 {
		t.Error("b goes back")
	}
	m, _ = press(m, ":")
	m = typed(m, "200 0")
	m, _ = press(m, "enter")
	wants(t, plain(m), "is not on the map")
	if m.maps.main.Center.Lat != 55.396 {
		t.Error("a bad place must not move the map")
	}
}

func TestParseCoords(t *testing.T) {
	for in, want := range map[string][3]float64{
		"55.396, 10.386": {55.396, 10.386, 7}, "55.396 10.386 14": {55.396, 10.386, 14}, "-33.86;151.2": {-33.86, 151.2, 7},
	} {
		at, z, err := parseCoords(in, 7)
		if err != nil || at.Lat != want[0] || at.Lon != want[1] || z != want[2] {
			t.Errorf("%q: %v %v %v", in, at, z, err)
		}
	}
	for _, bad := range []string{"", "55", "a b", "1 2 3 4", "89 0"} {
		if _, _, err := parseCoords(bad, 7); err == nil {
			t.Errorf("%q must fail", bad)
		}
	}
}

func TestMapWheelZooms(t *testing.T) {
	m, _ := mapModel(t, true)
	m, _ = press(m, "5", "M")
	m = wheel(m, tea.MouseWheelUp, 0)
	if m.maps.main.Zoom != 12.5 {
		t.Error(m.maps.main.Zoom)
	}
}

func TestMapASCII(t *testing.T) {
	m, _ := mapModel(t, true)
	m.opts.ASCII = true
	m.maps.main.Mode = mapterm.ModeASCII
	m, cmd := press(m, "5")
	m = runAll(t, m, cmd)
	for _, r := range ansiRe.ReplaceAllString(m.View().Content, "") {
		if r > 0x7e && r != '\n' {
			t.Fatalf("non-ASCII %q", r)
		}
	}
	m, _ = press(m, "v")
	if m.maps.main.Mode != mapterm.ModeASCII {
		t.Error("ASCII stays ASCII")
	}
}

func TestMapStatusKeepsTheCreditWhenNarrow(t *testing.T) {
	m, _ := mapModel(t, true)
	m, _ = press(m, "5")
	if s := ansiRe.ReplaceAllString(m.mapStatus(34), ""); !strings.Contains(s, mapterm.Attribution) {
		t.Errorf("%q", s)
	}
}

func TestAboutAndEgg(t *testing.T) {
	m, _ := mapModel(t, false)
	m, _ = press(m, "A")
	wants(t, plain(m), "About", "Vægter", "Ender KUŞ", "ODbL", mapterm.CopyrightURL)
	m, _ = press(m, "ø")
	if m.egg < 0 {
		t.Fatal("ø starts the message")
	}
	m, _ = press(m, "esc")
	m.frame += 3
	wants(t, plain(m), "shine the lights on me")
	m, _ = press(m, "£")
	if m.egg >= 0 {
		t.Error("£ stops it again")
	}
	m, _ = press(m, "3", "/")
	m = typed(m, "ø")
	if m.egg >= 0 || m.problemFilter != "ø" {
		t.Error("typing ø in a filter is just a letter")
	}
}

func TestGeomapWidget(t *testing.T) {
	m, src := mapModel(t, true)
	m, _ = press(m, "4")
	mm, _ := m.Update(eventMsg{poller.Boards{List: []model.Board{{ID: "1", Name: "Global view", Pages: []model.BoardPage{{Widgets: []model.Widget{
		{Type: "geomap", X: 0, Y: 0, W: 72, H: 8, Fields: []zabbix.WidgetField{{Name: "default_view", Value: "56.95225,24.11301,11"}}},
	}}}}}}})
	m = mm.(Model)
	m, cmd := press(m, "enter")
	m = runAll(t, m, cmd)
	if src.calls.Load() == 0 {
		t.Fatal("the widget fetches its tiles")
	}
	g := m.maps.widgets["1/0/0"]
	if g == nil || g.Center.Lat != 56.95225 || g.Zoom != 11 {
		t.Fatalf("%+v", g)
	}
	wants(t, plain(m), "© OpenStreetMap")
}
