package poller

import (
	"context"
	"time"

	"github.com/knightmare2600/vogter/internal/model"
)

// mapLocationEvery paces inventory reads while the map is open, as host
// locations rarely change.
const mapLocationEvery = time.Minute

// MapLocations is the located hosts, for the map.
type MapLocations struct{ model.Locations }

func (MapLocations) isEvent() {}

// WatchMap turns host location reading on or off. It never blocks.
func (p *Poller) WatchMap(on bool) {
	p.mu.Lock()
	changed := p.mapping != on
	p.mapping = on
	p.mu.Unlock()
	if changed {
		p.pokeMap()
	}
}

func (p *Poller) pokeMap() {
	select {
	case p.mapPoke <- struct{}{}:
	default:
	}
}

// mapLoop reads host inventories while the map is shown, at most once a
// minute, and not at all while it is closed.
func (p *Poller) mapLoop(ctx context.Context, events chan<- Event) {
	var last time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.mapPoke:
		}
		p.mu.Lock()
		on, configured := p.mapping, p.Locations
		p.mu.Unlock()
		if !on {
			last = time.Time{}
			continue
		}
		if !last.IsZero() && p.Now().Sub(last) < mapLocationEvery {
			continue
		}
		inv, err := p.API.HostInventories(ctx)
		if err != nil {
			p.send(ctx, events, Failed{err})
			continue
		}
		last = p.Now()
		p.send(ctx, events, MapLocations{model.Locate(inv, configured)})
	}
}
