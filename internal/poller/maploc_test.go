package poller

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/knightmare2600/vogter/internal/config"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

func TestMapLoop(t *testing.T) {
	api := &fakeAPI{inventory: []zabbix.HostInventory{
		{HostID: "1", Name: "a", Inventory: zabbix.Inventory{Lat: "55.4", Lon: "10.4"}},
		{HostID: "2", Name: "b"},
	}}
	var clock atomic.Int64
	clock.Store(1_000_000)
	p := New(api, config.Auth{Token: "t"}, time.Hour, Fmt{})
	p.Now = func() time.Time { return time.Unix(clock.Load(), 0) }
	p.Locations = map[string][2]float64{"b": {56, 10}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan Event, 4)
	go p.mapLoop(ctx, events)

	p.pokeMap()
	select {
	case e := <-events:
		t.Fatalf("closed map read inventory: %#v", e)
	case <-time.After(50 * time.Millisecond):
	}

	p.WatchMap(true)
	e := (<-events).(MapLocations)
	if len(e.Hosts) != 2 || e.Total != 2 {
		t.Fatalf("%+v", e)
	}
	// A poll soon after does not read again.
	p.pokeMap()
	time.Sleep(30 * time.Millisecond)
	if n := countCalls(api, "inventories"); n != 1 {
		t.Errorf("read %d times within a minute", n)
	}
	clock.Add(int64(mapLocationEvery / time.Second))
	p.pokeMap()
	<-events
	api.mu.Lock()
	api.inventoryErr = errors.New("down")
	api.mu.Unlock()
	clock.Add(int64(mapLocationEvery / time.Second))
	p.pokeMap()
	if _, ok := (<-events).(Failed); !ok {
		t.Error("a failed read is reported")
	}
}
