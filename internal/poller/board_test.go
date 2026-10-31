package poller

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/knightmare2600/vogter/internal/config"
	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

func graphDashboard() zabbix.Dashboard {
	return zabbix.Dashboard{DashboardID: "1", Name: "Health", Pages: []zabbix.DashboardPage{{Widgets: []zabbix.Widget{
		{Type: "svggraph", X: "0", Y: "0", Width: "36", Height: "5", Fields: []zabbix.WidgetField{
			{Name: "ds.0.hosts.0", Value: "Zabbix server"}, {Name: "ds.0.items.0", Value: "cache*"},
		}},
		{Type: "systeminfo", X: "36", Y: "0", Width: "36", Height: "5"},
		{Type: "hostavail", X: "0", Y: "5", Width: "36", Height: "2", Fields: []zabbix.WidgetField{{Name: "interface_type.0", Value: "1"}}},
	}}}}
}

func boardSetup(api *fakeAPI) *Poller {
	api.version = "7.0.32"
	p := New(api, config.Auth{Token: "t"}, time.Hour, Fmt{})
	p.Now = func() time.Time { return time.Unix(100_000, 0) }
	return p
}

func countCalls(api *fakeAPI, name string) int {
	api.mu.Lock()
	defer api.mu.Unlock()
	n := 0
	for _, c := range api.calls {
		if c == name {
			n++
		}
	}
	return n
}

func TestBoardDataFillsWidgets(t *testing.T) {
	api := &fakeAPI{
		hosts:      []zabbix.Host{{HostID: "10", Name: "Zabbix server", Status: "0", Interfaces: []zabbix.Interface{{Type: "1", Available: "1"}}}},
		boardItems: []zabbix.Item{{ItemID: "1", HostID: "10", Name: "cache used", Key: "k", ValueType: "0", Units: "%"}, {ItemID: "2", HostID: "10", Name: "text", Key: "t", ValueType: "4"}},
		history:    []zabbix.HistoryPoint{{ItemID: "1", Clock: "99000", Value: "5"}},
	}
	p := boardSetup(api)
	page := model.BoardsFrom([]zabbix.Dashboard{graphDashboard()})[0].Pages[0]
	st := newBoardState()
	d := p.boardData(context.Background(), boardWatch{true, "1", 0}, page, st)
	g := d.Widgets[0]
	if len(g.Series) != 1 || g.Series[0].Name != "cache used" || len(g.Series[0].Points) != 1 || g.Range != model.DefaultWidgetRange {
		t.Fatalf("text items must be skipped: %+v", g)
	}
	if d.Widgets[2].Avail == nil || d.Widgets[2].Avail.Up != 1 {
		t.Errorf("%+v", d.Widgets[2])
	}
	info := d.Widgets[1].Info
	if info[0] != [2]string{"Zabbix version", "7.0.32"} {
		t.Errorf("%v", info)
	}
	if slices.Contains(info, [2]string{"Users", ""}) || len(info) != 5 {
		t.Errorf("a failed count must be left out: %v", info)
	}

	// The next refresh reuses the slow lookups and reads only new samples.
	p.boardData(context.Background(), boardWatch{true, "1", 0}, page, st)
	if n := countCalls(api, "itemsmatching"); n != 1 {
		t.Errorf("patterns resolved %d times", n)
	}
	if n := countCalls(api, "count"); n != 5 {
		t.Errorf("counted %d times", n)
	}
}

func TestGraphCapsSeries(t *testing.T) {
	api := &fakeAPI{hosts: []zabbix.Host{{HostID: "10", Name: "Zabbix server", Status: "0"}}}
	for i := range maxGraphSeries + 3 {
		api.boardItems = append(api.boardItems, zabbix.Item{ItemID: string(rune('a' + i)), HostID: "10", Name: "n", Key: "k", ValueType: "0"})
	}
	p := boardSetup(api)
	page := model.BoardsFrom([]zabbix.Dashboard{graphDashboard()})[0].Pages[0]
	d := p.boardData(context.Background(), boardWatch{true, "1", 0}, page, newBoardState())
	if g := d.Widgets[0]; len(g.Series) != maxGraphSeries || g.Dropped != 3 {
		t.Fatalf("%d series, %d dropped", len(g.Series), g.Dropped)
	}
}

func TestWidgetFailureStaysInTheWidget(t *testing.T) {
	api := &fakeAPI{hostErrs: []error{errors.New("boom")}}
	p := boardSetup(api)
	page := model.BoardsFrom([]zabbix.Dashboard{graphDashboard()})[0].Pages[0]
	d := p.boardData(context.Background(), boardWatch{true, "1", 0}, page, newBoardState())
	if d.Widgets[0].Err != "boom" || d.Widgets[1].Err != "" || len(d.Widgets[1].Info) == 0 {
		t.Fatalf("%+v", d.Widgets)
	}
}

func TestBoardLoopFollowsTheWatch(t *testing.T) {
	api := &fakeAPI{boards: []zabbix.Dashboard{graphDashboard()}}
	p := boardSetup(api)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan Event, 8)
	go p.boardLoop(ctx, events)

	// Off, the loop costs nothing.
	p.WatchBoard(false, "", 0)
	select {
	case e := <-events:
		t.Fatalf("event while off: %#v", e)
	case <-time.After(50 * time.Millisecond):
	}
	if countCalls(api, "dashboards") != 0 {
		t.Fatal("fetched while off")
	}

	p.WatchBoard(true, "", 0)
	if _, ok := (<-events).(Boards); !ok {
		t.Fatal("no list")
	}
	p.WatchBoard(true, "1", 0)
	if _, ok := (<-events).(Boards); !ok {
		t.Fatal("no list")
	}
	if e, ok := (<-events).(BoardData); !ok || e.BoardID != "1" {
		t.Fatal("no data")
	}

	api.mu.Lock()
	api.boardErr = errors.New("nope")
	api.mu.Unlock()
	p.pokeBoard()
	if _, ok := (<-events).(Failed); !ok {
		t.Fatal("a failed list must be reported")
	}
}
