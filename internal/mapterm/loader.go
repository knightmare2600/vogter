package mapterm

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/knightmare2600/vogter/internal/mapterm/mvt"
)

// retryAfter is how long a failed tile waits before it is asked for again, so
// an outage does not turn into a stream of requests.
const retryAfter = 30 * time.Second

// Loader keeps decoded tiles and tracks which are being fetched. It does no
// network work itself: Want says which tiles to start, the caller fetches each
// with Load off the UI goroutine, and Done records the result. Tiles that
// scroll out of view are cancelled, unless they already arrived.
type Loader struct {
	Source Source
	Now    func() time.Time

	mu       sync.Mutex
	cache    *Cache[*mvt.Tile]
	inflight map[TileID]fetch
	gen      int
	failed   map[TileID]time.Time
	added    int
}

// Failing counts tiles whose last fetch failed and are waiting to retry.
func (l *Loader) Failing() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.failed)
}

// NewLoader makes a loader with a memory budget, in points.
func NewLoader(src Source, budget int) *Loader {
	return &Loader{Source: src, Now: time.Now, cache: NewCache[*mvt.Tile](budget),
		inflight: map[TileID]fetch{}, failed: map[TileID]time.Time{}}
}

// Lookup returns a cached tile.
func (l *Loader) Lookup(id TileID) (*mvt.Tile, bool) { return l.cache.Get(id) }

// Version changes whenever a tile arrives, so a map knows to draw again.
func (l *Loader) Version() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.added
}

// Pending is the number of tiles being fetched.
func (l *Loader) Pending() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.inflight)
}

// Request is a tile to fetch, with a context cancelled if it is no longer
// wanted. Its generation tells a late result from a newer fetch of the same
// tile.
type Request struct {
	ID  TileID
	Ctx context.Context
	gen int
}

type fetch struct {
	cancel context.CancelFunc
	gen    int
}

// Want takes the tiles a view lacks and returns those to start fetching now.
// Fetches for tiles no longer wanted are cancelled.
func (l *Loader) Want(ids []TileID) []Request {
	l.mu.Lock()
	defer l.mu.Unlock()
	wanted := make(map[TileID]bool, len(ids))
	var out []Request
	now := l.Now()
	for _, id := range ids {
		wanted[id] = true
		if _, busy := l.inflight[id]; busy {
			continue
		}
		if at, ok := l.failed[id]; ok && now.Sub(at) < retryAfter {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		l.gen++
		l.inflight[id] = fetch{cancel, l.gen}
		out = append(out, Request{id, ctx, l.gen})
	}
	for id, f := range l.inflight {
		if !wanted[id] {
			f.cancel()
			delete(l.inflight, id)
		}
	}
	return out
}

// Load fetches and decodes one tile. It is safe to call from any goroutine.
func (l *Loader) Load(r Request) (*mvt.Tile, error) {
	raw, err := l.Source.Tile(r.Ctx, r.ID)
	if err != nil {
		return nil, err
	}
	return mvt.Decode(raw)
}

// Done records a fetch's result. A cancelled fetch is forgotten without
// counting as a failure. A tile that arrived is kept even if it was
// cancelled meanwhile, as it cost a request already.
func (l *Loader) Done(r Request, t *mvt.Tile, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	id := r.ID
	if f, ok := l.inflight[id]; ok && f.gen == r.gen {
		f.cancel()
		delete(l.inflight, id)
	}
	switch {
	case err == nil:
		delete(l.failed, id)
		l.cache.Put(id, t, Cost(t))
		l.added++
	case isCancel(err):
	default:
		l.failed[id] = l.Now()
	}
}

func isCancel(err error) bool { return errors.Is(err, context.Canceled) }
