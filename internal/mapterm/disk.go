package mapterm

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// DefaultDiskBudget caps the disk cache.
const DefaultDiskBudget = 200 << 20

// pruneEvery is how many writes pass between size checks.
const pruneEvery = 64

// DiskCache keeps fetched tiles on disk, so the map works offline for any area
// seen before. Tiles are grouped by the source's current tile URL, which
// OpenFreeMap changes with each new build of its data, so a new build never
// mixes with an old one.
type DiskCache struct {
	Dir    string
	Budget int64
	Source *HTTPSource

	mu     sync.Mutex
	writes int
}

// NewDiskCache wraps a source with a cache in dir.
func NewDiskCache(dir string, budget int64, src *HTTPSource) *DiskCache {
	src.MetaPath = filepath.Join(dir, "tilejson.json")
	return &DiskCache{Dir: dir, Budget: budget, Source: src}
}

// MaxZoom is the wrapped source's.
func (d *DiskCache) MaxZoom() int { return d.Source.MaxZoom() }

func (d *DiskCache) path(template string, id TileID) string {
	sum := sha256.Sum256([]byte(template))
	return filepath.Join(d.Dir, fmt.Sprintf("%x", sum[:8]), fmt.Sprint(id.Z), fmt.Sprint(id.X), fmt.Sprintf("%d.pbf", id.Y))
}

// Tile reads from disk, fetching and storing on a miss. A cache that cannot
// be written only costs speed, so write errors are not reported.
func (d *DiskCache) Tile(ctx context.Context, id TileID) ([]byte, error) {
	tmpl, err := d.Source.tileTemplate(ctx)
	if err != nil {
		return nil, err
	}
	p := d.path(tmpl, id)
	if b, err := os.ReadFile(p); err == nil {
		now := time.Now()
		_ = os.Chtimes(p, now, now)
		return b, nil
	}
	b, err := d.Source.Tile(ctx, id)
	if err != nil {
		return nil, err
	}
	if os.MkdirAll(filepath.Dir(p), 0o700) == nil {
		tmp := p + ".tmp"
		if os.WriteFile(tmp, b, 0o600) == nil {
			_ = os.Rename(tmp, p)
		}
	}
	d.mu.Lock()
	d.writes++
	due := d.writes%pruneEvery == 1
	d.mu.Unlock()
	if due {
		go d.Prune()
	}
	return b, nil
}

// Prune removes the least recently used tiles until the cache fits its budget.
func (d *DiskCache) Prune() error {
	type entry struct {
		path string
		size int64
		at   time.Time
	}
	var all []entry
	var total int64
	err := filepath.WalkDir(d.Dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if e.IsDir() || filepath.Ext(p) != ".pbf" {
			return nil
		}
		info, err := e.Info()
		if err != nil {
			return nil
		}
		all = append(all, entry{p, info.Size(), info.ModTime()})
		total += info.Size()
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(all, func(i, j int) bool { return all[i].at.Before(all[j].at) })
	for _, e := range all {
		if total <= d.Budget {
			break
		}
		if os.Remove(e.path) == nil {
			total -= e.size
		}
	}
	return nil
}
