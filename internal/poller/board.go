package poller

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

const (
	// maxGraphSeries keeps a wildcard graph readable in a terminal.
	maxGraphSeries = 12
	// topHostsLimit is the frontend's default row count.
	topHostsLimit = 10
	// boardSlowEvery paces lookups that change rarely and cost the server
	// more: which items a graph selects, and the system information counts.
	boardSlowEvery = time.Minute
)

type boardWatch struct {
	on   bool
	id   string
	page int
}

// boardState lives as long as one watch.
type boardState struct {
	graphs   map[int][]zabbix.Item
	graphsAt time.Time
	// cache holds samples per range, as two widgets may read one item over
	// different periods.
	cache  map[int64]map[string]*itemCache
	info   [][2]string
	infoAt time.Time
}

func newBoardState() *boardState {
	return &boardState{graphs: map[int][]zabbix.Item{}, cache: map[int64]map[string]*itemCache{}}
}

// WatchBoard makes the board loop follow the dashboard list and, with an id,
// one dashboard page. It never blocks, so the UI can call it directly.
func (p *Poller) WatchBoard(on bool, id string, page int) {
	p.mu.Lock()
	p.boarding = boardWatch{on, id, page}
	p.mu.Unlock()
	p.pokeBoard()
}

func (p *Poller) pokeBoard() {
	select {
	case p.boardPoke <- struct{}{}:
	default:
		// A fetch is already due; it will read the latest watch.
	}
}

// boardLoop runs apart from the poll loop, like detailLoop, and costs nothing
// while the Boards screen is closed.
func (p *Poller) boardLoop(ctx context.Context, events chan<- Event) {
	var current boardWatch
	state := newBoardState()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.boardPoke:
		}
		p.mu.Lock()
		w := p.boarding
		p.mu.Unlock()
		if w != current {
			current, state = w, newBoardState()
		}
		if !w.on {
			continue
		}
		list, err := p.API.Dashboards(ctx)
		if err != nil {
			p.send(ctx, events, Failed{err})
			continue
		}
		boards := model.BoardsFrom(list)
		p.send(ctx, events, Boards{boards})
		if page, ok := findPage(boards, w); ok {
			p.send(ctx, events, BoardData{p.boardData(ctx, w, page, state)})
		}
	}
}

func findPage(boards []model.Board, w boardWatch) (model.BoardPage, bool) {
	for _, b := range boards {
		if b.ID == w.id && w.page < len(b.Pages) {
			return b.Pages[w.page], true
		}
	}
	return model.BoardPage{}, false
}

func (p *Poller) boardData(ctx context.Context, w boardWatch, page model.BoardPage, st *boardState) model.BoardData {
	data := model.BoardData{BoardID: w.id, Page: w.page, Widgets: make([]model.WidgetData, len(page.Widgets))}
	var hosts []zabbix.Host
	var hostErr error
	if slices.ContainsFunc(page.Widgets, needsHosts) {
		hosts, hostErr = p.API.Hosts(ctx)
	}
	now := p.Now()
	slow := now.Sub(st.graphsAt) >= boardSlowEvery
	for i, wg := range page.Widgets {
		var d model.WidgetData
		var err error
		switch {
		case needsHosts(wg) && hostErr != nil:
			err = hostErr
		case wg.Type == "svggraph":
			d, err = p.graph(ctx, i, wg, hosts, st, slow)
		case wg.Type == "tophosts":
			d, err = p.topHosts(ctx, wg, hosts)
		case wg.Type == "hostavail":
			c := model.CountAvail(hosts, wg.All("interface_type"))
			d.Avail = &c
		case wg.Type == "item":
			d.Items, err = p.API.ItemsByID(ctx, wg.All("itemid"))
		case wg.Type == "systeminfo":
			d.Info = p.systemInfo(ctx, st)
		}
		if err != nil {
			d = model.WidgetData{Err: err.Error()}
		}
		data.Widgets[i] = d
	}
	if slow {
		st.graphsAt = now
	}
	return data
}

func needsHosts(w model.Widget) bool {
	return w.Type == "svggraph" || w.Type == "tophosts" || w.Type == "hostavail"
}

func (p *Poller) graph(ctx context.Context, idx int, w model.Widget, hosts []zabbix.Host, st *boardState, slow bool) (model.WidgetData, error) {
	items, known := st.graphs[idx]
	if slow || !known {
		var err error
		if items, err = p.graphItems(ctx, w, hosts); err != nil {
			return model.WidgetData{}, err
		}
		st.graphs[idx] = items
	}
	shown := items[:min(len(items), maxGraphSeries)]
	rng := model.ParseRange(w.Get("time_period.from"))
	cache := st.cache[rng]
	if cache == nil {
		cache = map[string]*itemCache{}
		st.cache[rng] = cache
	}
	ptrs := make([]*zabbix.Item, len(shown))
	for i := range shown {
		ptrs[i] = &shown[i]
	}
	if err := p.fill(ctx, ptrs, cache, rng, p.Now().Unix()-rng); err != nil {
		return model.WidgetData{}, err
	}
	d := model.WidgetData{Range: rng, Dropped: len(items) - len(shown)}
	names := map[string]string{}
	for _, h := range hosts {
		names[h.HostID] = h.Name
	}
	several := false
	for _, it := range shown {
		several = several || it.HostID != shown[0].HostID
	}
	for _, it := range shown {
		name := it.Name
		if several {
			name = names[it.HostID] + ": " + name
		}
		d.Series = append(d.Series, model.NamedSeries{Name: name, Units: it.Units, Points: cache[it.ItemID].points()})
	}
	return d, nil
}

// graphItems resolves each dataset, which names its items directly or by
// host and item patterns, to the numeric items it selects.
func (p *Poller) graphItems(ctx context.Context, w model.Widget, hosts []zabbix.Host) ([]zabbix.Item, error) {
	var out []zabbix.Item
	seen := map[string]bool{}
	for ds := range w.Datasets() {
		var items []zabbix.Item
		var err error
		if ids := w.Dataset(ds, "itemids"); len(ids) > 0 {
			items, err = p.API.ItemsByID(ctx, ids)
		} else if hostIDs := matchHosts(hosts, w.Dataset(ds, "hosts")); len(hostIDs) > 0 {
			items, err = p.API.ItemsMatching(ctx, hostIDs, w.Dataset(ds, "items"))
		}
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if it.Numeric() && !seen[it.ItemID] {
				seen[it.ItemID] = true
				out = append(out, it)
			}
		}
	}
	return out, nil
}

func matchHosts(hosts []zabbix.Host, patterns []string) []string {
	var out []string
	for _, h := range hosts {
		if slices.ContainsFunc(patterns, func(p string) bool { return model.Glob(p, h.Name) }) {
			out = append(out, h.HostID)
		}
	}
	return out
}

func (p *Poller) topHosts(ctx context.Context, w model.Widget, hosts []zabbix.Host) (model.WidgetData, error) {
	columns := model.TopColumns(w)
	var names []string
	for _, c := range columns {
		if !c.Host && c.Item != "" {
			names = append(names, c.Item)
		}
	}
	chosen := w.All("hostids")
	shown := map[string]string{}
	for _, h := range hosts {
		if len(chosen) == 0 || slices.Contains(chosen, h.HostID) {
			shown[h.HostID] = h.Name
		}
	}
	var ids []string
	if len(chosen) > 0 {
		ids = chosen
	}
	items, err := p.API.ItemsNamed(ctx, ids, names)
	if err != nil {
		return model.WidgetData{}, err
	}
	sortBy, _ := strconv.Atoi(w.Get("column"))
	limit := topHostsLimit
	if n, err := strconv.Atoi(w.Get("show_lines")); err == nil && n > 0 {
		limit = n
	}
	top := model.BuildTop(columns, sortBy, shown, items, limit)
	return model.WidgetData{Top: &top}, nil
}

// systemInfo shows what the API can answer. The frontend's "server is
// running" and required performance rows come from the server itself and
// have no API. A count that fails is left out.
func (p *Poller) systemInfo(ctx context.Context, st *boardState) [][2]string {
	if st.info != nil && p.Now().Sub(st.infoAt) < boardSlowEvery {
		return st.info
	}
	version, err := p.API.APIVersion(ctx)
	if err != nil {
		version = "-"
	}
	rows := []struct{ label, method string }{
		{"Hosts", "host.get"}, {"Templates", "template.get"}, {"Items", "item.get"}, {"Triggers", "trigger.get"}, {"Users", "user.get"},
	}
	counts := make([]string, len(rows))
	var wg sync.WaitGroup
	for i, r := range rows {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if n, err := p.API.Count(ctx, r.method); err == nil {
				counts[i] = n
			}
		}()
	}
	wg.Wait()
	info := [][2]string{{"Zabbix version", version}}
	for i, r := range rows {
		if counts[i] != "" {
			info = append(info, [2]string{r.label, counts[i]})
		}
	}
	st.info, st.infoAt = info, p.Now()
	return info
}
