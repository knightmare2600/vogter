// Package poller owns the Zabbix client. It connects, polls on an interval
// and on demand, and reports results as events, so the UI never waits on
// the network.
package poller

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/knightmare2600/vogter/internal/config"
	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

// The net/disk prefix search can take far longer than the other calls on
// instances with thousands of hosts. Its columns are a bonus, so it gets a
// short budget and a failure leaves them empty instead of failing the poll.
// The server keeps running a query we gave up on, so it is then paused.
const (
	extraBudget = 5 * time.Second
	extraRetry  = time.Minute
)

// API is the subset of *zabbix.Client the poller uses; tests replace it.
type API interface {
	Login(ctx context.Context, username, password string) error
	Logout(ctx context.Context) error
	SetToken(token string)
	APIVersion(ctx context.Context) (string, error)
	Hosts(ctx context.Context) ([]zabbix.Host, error)
	Problems(ctx context.Context) ([]zabbix.Problem, error)
	Triggers(ctx context.Context, ids []string) ([]zabbix.Trigger, error)
	ItemsByKey(ctx context.Context, hostIDs, keys []string) ([]zabbix.Item, error)
	ItemsByPrefix(ctx context.Context, hostIDs, prefixes []string) ([]zabbix.Item, error)
	History(ctx context.Context, itemIDs []string, valueType int, from int64) ([]zabbix.HistoryPoint, error)
	Trends(ctx context.Context, itemIDs []string, from int64) ([]zabbix.TrendPoint, error)
	HostItems(ctx context.Context, hostID string) ([]zabbix.Item, error)
	ItemsByID(ctx context.Context, itemIDs []string) ([]zabbix.Item, error)
	ItemsMatching(ctx context.Context, hostIDs, patterns []string) ([]zabbix.Item, error)
	ItemsNamed(ctx context.Context, hostIDs, names []string) ([]zabbix.Item, error)
	Dashboards(ctx context.Context) ([]zabbix.Dashboard, error)
	HostInventories(ctx context.Context) ([]zabbix.HostInventory, error)
	Count(ctx context.Context, method string) (string, error)
	Acknowledge(ctx context.Context, eventIDs []string, message string, close bool) error
}

// Cmd asks the poller to do something.
type Cmd int

const (
	// Refresh polls now.
	Refresh Cmd = iota + 1
)

// Event is sent to the UI.
type Event interface{ isEvent() }

// Connected reports a successful login and the server version.
type Connected struct {
	Version zabbix.Version
	Support zabbix.Support
}

// Snapshot is one complete poll.
type Snapshot struct{ model.Snapshot }

// Detail is one fetch of the watched host.
type Detail struct{ model.HostDetail }

// Boards lists the dashboards.
type Boards struct{ List []model.Board }

// BoardData is one fetch of the open dashboard page.
type BoardData struct{ model.BoardData }

// Failed reports an error; the next trigger retries.
type Failed struct{ Err error }

func (Connected) isEvent() {}
func (Snapshot) isEvent()  {}
func (Detail) isEvent()    {}
func (Boards) isEvent()    {}
func (BoardData) isEvent() {}
func (Failed) isEvent()    {}

// Poller runs the poll loop.
type Poller struct {
	API      API
	Auth     config.Auth
	Interval time.Duration
	// Now is the clock, for tests.
	Now func() time.Time
	// Fmt formats host facts.
	Fmt Fmt

	connected  bool
	series     *model.Series
	extraRetry time.Time

	mu         sync.Mutex
	watching   watch
	detailPoke chan struct{}
	boarding   boardWatch
	boardPoke  chan struct{}
	mapping    bool
	mapPoke    chan struct{}

	// Locations from the config place hosts Zabbix has no coordinates for.
	Locations map[string][2]float64
}

// New builds a poller.
func New(api API, auth config.Auth, interval time.Duration, f Fmt) *Poller {
	return &Poller{API: api, Auth: auth, Interval: interval, Fmt: f, Now: time.Now, detailPoke: make(chan struct{}, 1), boardPoke: make(chan struct{}, 1), mapPoke: make(chan struct{}, 1)}
}

// Acknowledge runs outside the poll loop, so it does not wait behind a slow
// poll (zabterm queued it, D20). The client is safe for concurrent use.
func (p *Poller) Acknowledge(ctx context.Context, eventIDs []string, message string, close bool) error {
	return p.API.Acknowledge(ctx, eventIDs, message, close)
}

// Run polls until ctx is cancelled or cmds is closed. It polls once at
// start (zabterm polled twice, D1), then on every interval and every
// Refresh. Requests that arrive while a poll is running are coalesced into
// one follow-up poll. On exit it closes any user.login session.
func (p *Poller) Run(ctx context.Context, cmds <-chan Cmd, events chan<- Event) {
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); p.detailLoop(ctx, events) }()
	go func() { defer wg.Done(); p.boardLoop(ctx, events) }()
	go func() { defer wg.Done(); p.mapLoop(ctx, events) }()
	defer wg.Wait()
	defer func() {
		lctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.API.Logout(lctx)
	}()

	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()
	for {
		p.poll(ctx, events)
		p.pokeDetail()
		p.pokeBoard()
		p.pokeMap()
		// Drain anything that queued up during the poll: one follow-up
		// poll covers all of it.
		pending := false
	drain:
		for {
			select {
			case _, ok := <-cmds:
				if !ok {
					return
				}
				pending = true
			default:
				break drain
			}
		}
		if pending {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case _, ok := <-cmds:
			if !ok {
				return
			}
		case <-ticker.C:
		}
	}
}

func (p *Poller) send(ctx context.Context, events chan<- Event, e Event) {
	select {
	case events <- e:
	case <-ctx.Done():
	}
}

func (p *Poller) poll(ctx context.Context, events chan<- Event) {
	if !p.connected {
		ev, err := p.connect(ctx)
		if err != nil {
			p.send(ctx, events, Failed{err})
			return
		}
		p.connected = true
		p.send(ctx, events, ev)
	}

	started := p.Now()
	snap, err := p.snapshot(ctx)
	if err != nil {
		// An expired session is replaced by a fresh login on the next
		// trigger. A rejected API token stays rejected, so it is reported
		// as is.
		if zabbix.KindOf(err) == zabbix.KindAuth && p.Auth.IsLogin() {
			p.connected = false
		}
		p.send(ctx, events, Failed{err})
		return
	}
	snap.FetchedAt = p.Now()
	snap.Latency = snap.FetchedAt.Sub(started)
	p.send(ctx, events, Snapshot{snap})
}

func (p *Poller) snapshot(ctx context.Context) (model.Snapshot, error) {
	var (
		wg                        sync.WaitGroup
		in                        model.Input
		hostErr, probErr, itemErr error
		extraErr                  error
	)
	tryExtra := !p.Now().Before(p.extraRetry)
	wg.Add(3)
	go func() { defer wg.Done(); in.Hosts, hostErr = p.API.Hosts(ctx) }()
	go func() { defer wg.Done(); in.Problems, probErr = p.API.Problems(ctx) }()
	go func() { defer wg.Done(); in.Items, itemErr = p.API.ItemsByKey(ctx, nil, model.OverviewKeys) }()
	if tryExtra {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ectx, cancel := context.WithTimeout(ctx, extraBudget)
			defer cancel()
			in.Extra, extraErr = p.API.ItemsByPrefix(ectx, nil, model.ExtraPrefixes)
		}()
	}
	wg.Wait()
	for _, err := range []error{hostErr, probErr, itemErr} {
		if err != nil {
			return model.Snapshot{}, err
		}
	}
	if extraErr != nil {
		p.extraRetry = p.Now().Add(extraRetry)
	}

	seen := map[string]bool{}
	var ids []string
	for _, pr := range in.Problems {
		if !seen[pr.ObjectID] {
			seen[pr.ObjectID] = true
			ids = append(ids, pr.ObjectID)
		}
	}
	var err error
	if in.Triggers, err = p.API.Triggers(ctx, ids); err != nil {
		return model.Snapshot{}, err
	}

	p.updateSeries(ctx, in.Items)
	in.Series = p.series
	return model.Build(in), nil
}

// updateSeries seeds new CPU and memory items from history, once, then
// extends the buffers from the poll's last values. A failed seed only
// costs the sparkline's past, so it does not fail the poll; it is retried.
func (p *Poller) updateSeries(ctx context.Context, items []zabbix.Item) {
	if p.series == nil {
		p.series = model.NewSeries()
	}
	now := p.Now().Unix()
	keep := map[string]bool{}
	unseeded := map[int][]string{}
	var tracked []zabbix.Item
	for _, it := range items {
		if it.Key != model.KeyCPU && it.Key != model.KeyMem {
			continue
		}
		keep[it.ItemID] = true
		tracked = append(tracked, it)
		if !p.series.Seeded(it.ItemID) && it.Numeric() {
			vt, _ := strconv.Atoi(it.ValueType)
			unseeded[vt] = append(unseeded[vt], it.ItemID)
		}
	}
	p.series.Retain(keep)
	for vt, ids := range unseeded {
		points, err := p.API.History(ctx, ids, vt, now-model.SparkWindow)
		if err != nil {
			continue
		}
		for _, pt := range points {
			c, cerr := strconv.ParseInt(pt.Clock, 10, 64)
			v, verr := strconv.ParseFloat(pt.Value, 64)
			if cerr == nil && verr == nil {
				p.series.Push(pt.ItemID, c, v, now)
			}
		}
		for _, id := range ids {
			p.series.MarkSeeded(id)
		}
	}
	for _, it := range tracked {
		v, ok := it.Value()
		c, err := strconv.ParseInt(it.LastClock, 10, 64)
		if ok && err == nil {
			p.series.Push(it.ItemID, c, v, now)
		}
	}
}

func (p *Poller) connect(ctx context.Context) (Connected, error) {
	if p.Auth.IsLogin() {
		// Close a stale session before opening a new one.
		_ = p.API.Logout(ctx)
		if err := p.API.Login(ctx, p.Auth.Username, p.Auth.Password.Reveal()); err != nil {
			return Connected{}, err
		}
	} else {
		p.API.SetToken(p.Auth.Token.Reveal())
	}
	raw, err := p.API.APIVersion(ctx)
	if err != nil {
		return Connected{}, err
	}
	v, err := zabbix.ParseVersion(raw)
	if err != nil {
		return Connected{}, err
	}
	s := zabbix.SupportFor(v)
	if s == zabbix.Unsupported {
		return Connected{}, errors.New(s.Describe(v))
	}
	return Connected{Version: v, Support: s}, nil
}
