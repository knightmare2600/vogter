package poller

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"sync"

	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

// trendRange is the shortest range drawn from hourly trends. Below it raw
// history is short enough to read.
const trendRange = 24 * 60 * 60

// trendBucket is the length of one trend sample in seconds.
const trendBucket = 3600

// Fmt keeps display formatting out of the poller.
type Fmt struct {
	Bytes    func(float64) string
	Duration func(int64) string
}

type watch struct {
	hostID string
	rng    int64
}

// itemCache holds one item's samples for the watched host and range.
type itemCache struct {
	trend, raw []model.Point
}

func (c *itemCache) points() []model.Point {
	return append(slices.Clone(c.trend), c.raw...)
}

// Watch selects the host the detail loop follows; an empty id stops it. It
// never blocks, so the UI can call it directly.
func (p *Poller) Watch(hostID string, rng int64) {
	p.mu.Lock()
	p.watching = watch{hostID, rng}
	p.mu.Unlock()
	p.pokeDetail()
}

func (p *Poller) pokeDetail() {
	select {
	case p.detailPoke <- struct{}{}:
	default:
		// A fetch is already due; it will read the latest watch.
	}
}

// detailLoop fetches the watched host once per poke. It runs apart from
// the poll loop, so opening a host does not wait behind a slow poll, and
// each poll costs one fetch (zabterm made two, and re-read the whole range
// every time, D2).
func (p *Poller) detailLoop(ctx context.Context, events chan<- Event) {
	var (
		current watch
		cache   map[string]*itemCache
	)
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.detailPoke:
		}
		p.mu.Lock()
		w := p.watching
		p.mu.Unlock()
		if w != current {
			current, cache = w, map[string]*itemCache{}
		}
		if w.hostID == "" {
			continue
		}
		d, err := p.detail(ctx, w, cache)
		if err != nil {
			p.send(ctx, events, Failed{err})
			continue
		}
		p.send(ctx, events, Detail{d})
	}
}

func (p *Poller) detail(ctx context.Context, w watch, cache map[string]*itemCache) (model.HostDetail, error) {
	items, err := p.API.HostItems(ctx, w.hostID)
	if err != nil {
		return model.HostDetail{}, err
	}
	charted := model.PickCharted(items)
	now := p.Now().Unix()
	from := now - w.rng
	if err := p.fill(ctx, charted.All(), cache, w.rng, from); err != nil {
		return model.HostDetail{}, err
	}
	series := func(it *zabbix.Item) []model.Point {
		if it == nil {
			return nil
		}
		return cache[it.ItemID].points()
	}
	d := model.HostDetail{
		HostID: w.hostID,
		Range:  w.rng,
		CPU:    series(charted.CPU),
		Mem:    series(charted.Mem),
		Load:   series(charted.Load),
		NetIn:  series(charted.NetIn),
		NetOut: series(charted.NetOut),
		Disks:  model.Disks(items),
		Items:  items,
	}
	if p.Fmt.Bytes != nil {
		d.Facts = model.Facts(items, p.Fmt.Bytes, p.Fmt.Duration)
	}
	return d, nil
}

// fill brings each item's cache up to date with as little reading as it
// can: only samples newer than those held.
func (p *Poller) fill(ctx context.Context, items []*zabbix.Item, cache map[string]*itemCache, rng, from int64) error {
	for _, it := range items {
		if cache[it.ItemID] == nil {
			cache[it.ItemID] = &itemCache{}
		}
	}
	if rng >= trendRange {
		if err := p.fillTrends(ctx, items, cache, from); err != nil {
			return err
		}
	}
	return p.fillRaw(ctx, items, cache, from)
}

func (p *Poller) fillTrends(ctx context.Context, items []*zabbix.Item, cache map[string]*itemCache, from int64) error {
	// The newest bucket is read again, because it may have been rewritten.
	since := oldestNewest(items, cache, from)
	var ids []string
	for _, it := range items {
		if it.Numeric() {
			ids = append(ids, it.ItemID)
		}
	}
	points, err := p.API.Trends(ctx, ids, since)
	if err != nil {
		return err
	}
	fresh := map[string][]model.Point{}
	for _, t := range points {
		if pt, ok := model.ParsePoint(t.Clock, t.ValueAvg); ok {
			fresh[t.ItemID] = append(fresh[t.ItemID], pt)
		}
	}
	for _, it := range items {
		f := fresh[it.ItemID]
		slices.SortFunc(f, func(a, b model.Point) int { return cmp.Compare(a.T, b.T) })
		c := cache[it.ItemID]
		c.trend = model.Merge(c.trend, f, float64(from))
	}
	return nil
}

func (p *Poller) fillRaw(ctx context.Context, items []*zabbix.Item, cache map[string]*itemCache, from int64) error {
	// Raw samples start where trends end; without trends, at the range's
	// start.
	start := func(c *itemCache) int64 {
		if n := len(c.trend); n > 0 {
			return int64(c.trend[n-1].T) + trendBucket
		}
		return from
	}
	byType := map[int][]*zabbix.Item{}
	for _, it := range items {
		if it.Numeric() {
			vt, _ := strconv.Atoi(it.ValueType)
			byType[vt] = append(byType[vt], it)
		}
	}
	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		errs []error
	)
	for vt, list := range byType {
		// history.get takes one time_from per call, so the call uses the
		// earliest the items need, and Merge drops what they already hold.
		since := int64(1<<62 - 1)
		ids := make([]string, 0, len(list))
		for _, it := range list {
			c := cache[it.ItemID]
			s := start(c)
			if n := len(c.raw); n > 0 {
				s = max(s, int64(c.raw[n-1].T)+1)
			}
			since = min(since, s)
			ids = append(ids, it.ItemID)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			points, err := p.API.History(ctx, ids, vt, since)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			fresh := map[string][]model.Point{}
			for _, h := range points {
				if pt, ok := model.ParsePoint(h.Clock, h.Value); ok {
					fresh[h.ItemID] = append(fresh[h.ItemID], pt)
				}
			}
			for _, it := range list {
				c := cache[it.ItemID]
				c.raw = model.Merge(c.raw, fresh[it.ItemID], float64(start(c)))
			}
		}()
	}
	wg.Wait()
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// oldestNewest is the earliest of the items' newest trend samples, or from
// when an item holds none yet.
func oldestNewest(items []*zabbix.Item, cache map[string]*itemCache, from int64) int64 {
	since := int64(1<<62 - 1)
	for _, it := range items {
		s := cache[it.ItemID].trend
		if len(s) == 0 {
			return from
		}
		since = min(since, int64(s[len(s)-1].T))
	}
	if since == 1<<62-1 {
		return from
	}
	return since
}
