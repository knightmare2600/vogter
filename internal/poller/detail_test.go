package poller

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/knightmare2600/vogter/internal/config"
	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

func hostItem(id, key, vt string) zabbix.Item {
	return zabbix.Item{ItemID: id, HostID: "1", Name: key, Key: key, LastValue: "1", LastClock: "1", ValueType: vt}
}

func detailSetup(api *fakeAPI) (*Poller, map[string]*itemCache) {
	p := New(api, config.Auth{Token: "t"}, time.Hour, Fmt{})
	p.Now = func() time.Time { return time.Unix(100_000, 0) }
	return p, map[string]*itemCache{}
}

func TestShortRangeReadsOnlyNewHistory(t *testing.T) {
	api := &fakeAPI{
		hostItems: []zabbix.Item{hostItem("1", model.KeyCPU, "0")},
		history:   []zabbix.HistoryPoint{{ItemID: "1", Clock: "99000", Value: "5"}, {ItemID: "1", Clock: "99060", Value: "6"}},
	}
	p, cache := detailSetup(api)
	w := watch{"1", 3600}
	d, err := p.detail(context.Background(), w, cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.CPU) != 2 {
		t.Fatalf("%+v", d.CPU)
	}
	api.history = []zabbix.HistoryPoint{{ItemID: "1", Clock: "99120", Value: "7"}}
	d, err = p.detail(context.Background(), w, cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.CPU) != 3 || d.CPU[2].V != 7 {
		t.Fatalf("%+v", d.CPU)
	}
	if got := api.historyFrom; got[0] != 96400 || got[1] != 99061 {
		t.Errorf("history from = %v, want the full range then only what is new (D2)", got)
	}
	if api.count("trends") != 0 {
		t.Error("short ranges must not read trends")
	}
}

func TestLongRangeUsesTrendsPlusRawTail(t *testing.T) {
	api := &fakeAPI{
		hostItems: []zabbix.Item{hostItem("1", model.KeyCPU, "0")},
		trends: []zabbix.TrendPoint{
			{ItemID: "1", Clock: "93600", ValueAvg: "10"},
			{ItemID: "1", Clock: "90000", ValueAvg: "20"}, // out of order
		},
		history: []zabbix.HistoryPoint{{ItemID: "1", Clock: "97300", Value: "30"}},
	}
	p, cache := detailSetup(api)
	d, err := p.detail(context.Background(), watch{"1", 86400}, cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.CPU) != 3 || d.CPU[0].V != 20 || d.CPU[1].V != 10 || d.CPU[2].V != 30 {
		t.Fatalf("%+v", d.CPU)
	}
	if api.trendFrom[0] != 13600 {
		t.Errorf("first trend read from %d", api.trendFrom[0])
	}
	if api.historyFrom[0] != 97200 {
		t.Errorf("raw tail must start where trends end: %d", api.historyFrom[0])
	}
	if _, err := p.detail(context.Background(), watch{"1", 86400}, cache); err != nil {
		t.Fatal(err)
	}
	if api.trendFrom[1] != 93600 {
		t.Errorf("second trend read from %d, want the newest bucket only", api.trendFrom[1])
	}
}

func TestItemWithoutTrendsFallsBackToHistory(t *testing.T) {
	api := &fakeAPI{
		hostItems: []zabbix.Item{hostItem("1", model.KeyCPU, "0")},
		history:   []zabbix.HistoryPoint{{ItemID: "1", Clock: "50000", Value: "1"}},
	}
	p, cache := detailSetup(api)
	d, err := p.detail(context.Background(), watch{"1", 86400}, cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.CPU) != 1 || api.historyFrom[0] != 13600 {
		t.Errorf("%+v from %v", d.CPU, api.historyFrom)
	}
}

func TestDetailErrorsAreReported(t *testing.T) {
	api := &fakeAPI{hostItems: []zabbix.Item{hostItem("1", model.KeyCPU, "0")}, trendErr: errors.New("boom")}
	p, cache := detailSetup(api)
	if _, err := p.detail(context.Background(), watch{"1", 86400}, cache); err == nil {
		t.Error("a failed trend read must not look like an empty chart")
	}
	api = &fakeAPI{hostItems: []zabbix.Item{hostItem("1", model.KeyCPU, "0")}, historyErrs: []error{errors.New("boom")}}
	p, cache = detailSetup(api)
	if _, err := p.detail(context.Background(), watch{"1", 3600}, cache); err == nil {
		t.Error("a failed history read must not look like an empty chart")
	}
}

func TestUnsupportedTypesSkipped(t *testing.T) {
	api := &fakeAPI{hostItems: []zabbix.Item{hostItem("1", model.KeyCPU, "4")}}
	p, cache := detailSetup(api)
	d, err := p.detail(context.Background(), watch{"1", 86400}, cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.CPU) != 0 || api.count("history") != 0 {
		t.Errorf("text items have no history to chart: %+v", d.CPU)
	}
}

func TestWatchLoop(t *testing.T) {
	api := &fakeAPI{version: "7.0.12", hostItems: []zabbix.Item{hostItem("1", model.KeyCPU, "0")}}
	ctx, cancel := context.WithCancel(context.Background())
	p := New(api, config.Auth{Token: "t"}, time.Hour, Fmt{})
	cmds := make(chan Cmd, 1)
	events := make(chan Event, 16)
	done := make(chan struct{})
	go func() { defer close(done); p.Run(ctx, cmds, events) }()
	waitFor := func(want string) Event {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			select {
			case e := <-events:
				if fmt.Sprintf("%T", e) == want {
					return e
				}
			case <-deadline:
				t.Fatalf("no %s", want)
			}
		}
	}
	waitFor("poller.Snapshot")
	p.Watch("1", 3600)
	if d := waitFor("poller.Detail").(Detail); d.HostID != "1" || d.Range != 3600 {
		t.Errorf("%+v", d)
	}
	// The first poll's poke can land beside Watch's; let it finish, so the
	// count below is of one poll alone.
	time.Sleep(100 * time.Millisecond)
	before := api.count("hostitems")
	cmds <- Refresh
	waitFor("poller.Snapshot")
	waitFor("poller.Detail")
	if got := api.count("hostitems") - before; got != 1 {
		t.Errorf("%d detail fetches for one poll, want 1 (zabterm made 2, D2)", got)
	}
	p.Watch("", 0)
	time.Sleep(30 * time.Millisecond)
	before = api.count("hostitems")
	cmds <- Refresh
	waitFor("poller.Snapshot")
	time.Sleep(30 * time.Millisecond)
	if api.count("hostitems") != before {
		t.Error("leaving the host page must stop detail fetches")
	}
	cancel()
	<-done
}
