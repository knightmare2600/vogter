package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/knightmare2600/vogter/internal/config"
	"github.com/knightmare2600/vogter/internal/mapterm"
	"github.com/knightmare2600/vogter/internal/poller"
	"github.com/knightmare2600/vogter/internal/ui"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

// mapZabbix answers the calls the map screen needs. Its host's inventory
// uses a decimal comma, as a Danish frontend stores it.
func mapZabbix(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		res := `[]`
		switch s := string(body); {
		case strings.Contains(s, `"apiinfo.version"`):
			res = `"7.0.32"`
		case strings.Contains(s, `"selectInventory"`):
			res = `[{"hostid":"1","name":"odense-core","inventory":{"location_lat":"55,39594","location_lon":"10,38621","location":"","site_city":"Odense","site_country":""}},
				{"hostid":"2","name":"no-place","inventory":[]}]`
		case strings.Contains(s, `"host.get"`):
			res = `[{"hostid":"1","name":"odense-core","status":"0","maintenance_status":"0","active_available":"1","interfaces":[{"type":"1","available":"1","ip":"10.0.0.1","dns":"","useip":"1","port":"10050","error":""}],"hostgroups":[]},
				{"hostid":"2","name":"no-place","status":"0","maintenance_status":"0","active_available":"1","interfaces":[],"hostgroups":[]}]`
		case strings.Contains(s, `"problem.get"`):
			res = fmt.Sprintf(`[{"eventid":"5","objectid":"9","name":"Disk full","severity":"4","clock":"%d","acknowledged":"0","suppressed":"0","opdata":"","tags":[]}]`, time.Now().Unix()-60)
		case strings.Contains(s, `"trigger.get"`):
			res = `[{"triggerid":"9","manual_close":"0","hosts":[{"hostid":"1","name":"odense-core"}]}]`
		}
		io.WriteString(w, `{"jsonrpc":"2.0","result":`+res+`,"id":1}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// tileServer serves a TileJSON and one road tile, recording User-Agents.
func tileServer(t *testing.T, tile []byte, hits *atomic.Int32, agent *atomic.Value) *httptest.Server {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent.Store(r.Header.Get("User-Agent"))
		if r.URL.Path == "/planet" {
			fmt.Fprintf(w, `{"tilejson":"3.0.0","maxzoom":14,"tiles":["%s/b1/{z}/{x}/{y}.pbf"]}`, srv.URL)
			return
		}
		hits.Add(1)
		w.Write(tile)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// roadTile is a vector tile with one motorway across it.
func roadTile() []byte {
	varint := func(b []byte, v uint64) []byte {
		for v >= 0x80 {
			b = append(b, byte(v)|0x80)
			v >>= 7
		}
		return append(b, byte(v))
	}
	field := func(b []byte, n int, data []byte) []byte {
		return append(varint(varint(b, uint64(n<<3|2)), uint64(len(data))), data...)
	}
	packed := func(vs ...uint64) (b []byte) {
		for _, v := range vs {
			b = varint(b, v)
		}
		return b
	}
	feat := field(nil, 2, packed(0, 0))
	feat = append(feat, 3<<3, 2)
	feat = field(feat, 4, packed(1|1<<3, 0, 4096, 2|1<<3, 8192, 0))
	layer := field(nil, 1, []byte("transportation"))
	layer = field(layer, 2, feat)
	layer = field(layer, 3, []byte("class"))
	layer = field(layer, 4, field(nil, 1, []byte("motorway")))
	return field(nil, 3, layer)
}

// runtime is a small stand-in for the Bubble Tea runtime: it runs commands
// concurrently and feeds their messages to Update. One message channel
// serves the whole test, as one program would.
type runtime struct{ msgs chan tea.Msg }

func (r *runtime) start(c tea.Cmd) {
	if c == nil {
		return
	}
	go func() {
		msg := c()
		if b, ok := msg.(tea.BatchMsg); ok {
			for _, cc := range b {
				r.start(cc)
			}
			return
		}
		if msg != nil {
			r.msgs <- msg
		}
	}()
}

// until applies messages until done holds.
func (r *runtime) until(t *testing.T, m tea.Model, done func(tea.Model) bool) tea.Model {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for !done(m) {
		select {
		case msg := <-r.msgs:
			var c tea.Cmd
			m, c = m.Update(msg)
			r.start(c)
		case <-deadline:
			t.Fatalf("timed out at:\n%s", screen(m))
		}
	}
	return m
}

func screen(m tea.Model) string { return ansi.Strip(m.View().Content) }

func TestMapEndToEnd(t *testing.T) {
	var hits atomic.Int32
	var agent atomic.Value
	tiles := tileServer(t, roadTile(), &hits, &agent)
	zbx := mapZabbix(t)
	cache := t.TempDir()

	client := zabbix.New(zabbix.Options{URL: zbx.URL + "/api_jsonrpc.php", VerifyTLS: true})
	p := poller.New(client, config.Auth{Token: "t"}, time.Hour, poller.Fmt{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmds := make(chan poller.Cmd, 1)
	events := make(chan poller.Event, 16)
	go p.Run(ctx, cmds, events)

	newSource := func() mapterm.Source {
		return mapterm.NewDiskCache(cache, mapterm.DefaultDiskBudget, mapterm.NewHTTPSource(tiles.URL+"/planet", ui.MapUserAgent("test")))
	}
	m := ui.New(ui.Options{Profile: "lab", Theme: "nord", Interval: time.Hour, Events: events, Cmds: cmds,
		Watch: p.Watch, WatchBoard: p.WatchBoard, WatchMap: p.WatchMap,
		Map: ui.MapOptions{Enabled: true, NewSource: newSource, Labels: true}})
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	rt := &runtime{msgs: make(chan tea.Msg, 64)}
	rt.start(tm.Init())
	tm = rt.until(t, tm, func(m tea.Model) bool { return strings.Contains(screen(m), "Problems 1") })

	tm, c := tm.Update(tea.KeyPressMsg{Code: '5', Text: "5"})
	rt.start(c)
	tm = rt.until(t, tm, func(m tea.Model) bool {
		s := screen(m)
		return strings.Contains(s, "› odense-core") && hits.Load() > 0 && !strings.Contains(s, "loading")
	})
	out := screen(tm)
	for _, want := range []string{"1 host of 2 located", "› odense-core (Odense)", mapterm.Attribution, "■"} {
		if !strings.Contains(out, want) {
			t.Errorf("screen lacks %q:\n%s", want, out)
		}
	}
	if ua, _ := agent.Load().(string); !strings.HasPrefix(ua, "vaegter/test (+https://github.com/") {
		t.Errorf("tile requests must name Vægter: %q", ua)
	}
	entries, _ := os.ReadDir(cache)
	if len(entries) < 2 {
		t.Errorf("tiles and the tile list are cached on disk: %v", entries)
	}
}
