package mapterm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/knightmare2600/vogter/internal/mapterm/mvt"
)

func TestPanMovesByScreenFractions(t *testing.T) {
	m := New(odense, 15)
	m.SetSize(100, 30)
	before := Project(m.Center)
	m.Pan(0.5, 0)
	after := Project(m.Center)
	dots := (after.X - before.X) * m.View().scale()
	if !near(dots, 100, 0.01) {
		t.Errorf("half a 200-dot width is 100 dots, moved %.2f", dots)
	}
	m.Pan(0, -100)
	if w := Project(m.Center); w.Y < 0 || !m.Center.Valid() {
		t.Errorf("panning past the pole must stop at the edge: %v", m.Center)
	}
}

func TestZoomClamps(t *testing.T) {
	m := New(odense, 18.5)
	m.ZoomBy(5)
	if m.Zoom != MaxZoom {
		t.Error(m.Zoom)
	}
	m.ZoomBy(-100)
	if m.Zoom != MinZoom {
		t.Error(m.Zoom)
	}
}

func TestGoToAndBack(t *testing.T) {
	m := New(odense, 12)
	if m.GoTo(LatLon{200, 0}, 3) {
		t.Fatal("an invalid place must be refused")
	}
	if !m.GoTo(LatLon{51.5, -0.12}, 10) || m.Zoom != 10 {
		t.Fatal("goto")
	}
	m.GoTo(LatLon{40.7, -74}, 11)
	if !m.Back() || !near(m.Center.Lat, 51.5, 1e-9) {
		t.Errorf("%v", m.Center)
	}
	if !m.Back() || m.Center != odense || m.Zoom != 12 {
		t.Errorf("%v %v", m.Center, m.Zoom)
	}
	if m.Back() {
		t.Error("nothing left to go back to")
	}
	for range historyDepth + 10 {
		m.GoTo(odense, 5)
	}
	if len(m.history) != historyDepth {
		t.Errorf("history must stay bounded: %d", len(m.history))
	}
}

func TestFit(t *testing.T) {
	m := New(LatLon{}, 2)
	m.SetSize(100, 40)
	pts := []LatLon{{55.40, 10.38}, {55.38, 10.42}, {999, 0}}
	if !m.Fit(pts, 16) {
		t.Fatal("fit")
	}
	for _, p := range pts[:2] {
		if _, _, ok := m.Locate(p); !ok {
			t.Errorf("%v off screen at zoom %v", p, m.Zoom)
		}
	}
	if m.Zoom < 11 {
		t.Errorf("two points a few km apart must zoom in well: %v", m.Zoom)
	}
	one := New(LatLon{}, 2)
	one.SetSize(100, 40)
	one.Fit([]LatLon{odense}, 14)
	if one.Zoom != 14 || !near(one.Center.Lat, odense.Lat, 1e-9) || !near(one.Center.Lon, odense.Lon, 1e-9) {
		t.Errorf("one point centres at the cap: %v %v", one.Center, one.Zoom)
	}
	// Fiji's two sides straddle the date line; the fit must not span the world.
	fiji := New(LatLon{}, 2)
	fiji.SetSize(100, 40)
	fiji.Fit([]LatLon{{-17.7, 178.0}, {-16.5, -179.9}}, 16)
	if fiji.Zoom < 5 {
		t.Errorf("fitting across the date line took the long way: zoom %v", fiji.Zoom)
	}
	if New(LatLon{}, 2).Fit(nil, 10) {
		t.Error("nothing to fit")
	}
}

func TestRenderReusesTheFrame(t *testing.T) {
	tile := squareTile()
	calls := 0
	lookup := func(id TileID) (*mvt.Tile, bool) { calls++; return tile, true }
	m := New(LatLon{}, 0)
	m.SetSize(40, 20)
	f1, _ := m.Render(14, 1, lookup)
	n := calls
	f2, _ := m.Render(14, 1, lookup)
	if f1 != f2 || calls != n {
		t.Error("an unchanged view must not draw again")
	}
	if f3, _ := m.Render(14, 2, lookup); f3 == f1 {
		t.Error("a new tile must redraw")
	}
	m.SetSize(60, 20)
	if f4, _ := m.Render(14, 2, lookup); f4.Cols != 60 {
		t.Error("a resize must redraw at the new size")
	}
}

func TestHiddenLayers(t *testing.T) {
	tile := squareTile()
	lookup := func(TileID) (*mvt.Tile, bool) { return tile, true }
	m := New(LatLon{}, 0)
	m.SetSize(128, 64)
	f, _ := m.Render(14, 1, lookup)
	has := func(f *Frame, c Class) bool {
		for _, cell := range f.Cells {
			if cell.Class == c && cell.R != ' ' {
				return true
			}
		}
		return false
	}
	if !has(f, ClassWater) {
		t.Fatal("water shown")
	}
	m.ToggleLayer(LayerWater)
	if f, _ = m.Render(14, 1, lookup); has(f, ClassWater) || !has(f, ClassMotorway) {
		t.Error("hiding water must keep roads")
	}
}

func TestNextMode(t *testing.T) {
	m := New(odense, 10)
	seen := map[Mode]bool{}
	for range len(Modes) {
		seen[m.Mode] = true
		m.NextMode()
	}
	if len(seen) != len(Modes) || m.Mode != ModeBraille {
		t.Error(seen)
	}
}

// fakeSource returns a fixed tile, counting calls, failing on demand, and
// waiting on gate when one is set.
type fakeSource struct {
	calls atomic.Int32
	fail  atomic.Bool
	gate  chan struct{}
	data  []byte
}

func (f *fakeSource) MaxZoom() int { return 14 }

func (f *fakeSource) Tile(ctx context.Context, id TileID) ([]byte, error) {
	f.calls.Add(1)
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.fail.Load() {
		return nil, errors.New("HTTP 503")
	}
	return f.data, nil
}

func TestLoader(t *testing.T) {
	src := &fakeSource{}
	l := NewLoader(src, 1000)
	now := time.Unix(1_000_000, 0)
	l.Now = func() time.Time { return now }
	a, b := TileID{3, 1, 1}, TileID{3, 1, 2}
	reqs := l.Want([]TileID{a, b})
	if len(reqs) != 2 || l.Pending() != 2 {
		t.Fatal(reqs)
	}
	if again := l.Want([]TileID{a, b}); len(again) != 0 {
		t.Error("tiles already on their way are not asked for twice")
	}
	tile, err := l.Load(reqs[0])
	l.Done(reqs[0], tile, err)
	if _, ok := l.Lookup(a); !ok || l.Version() != 1 {
		t.Error("an arrived tile is cached and bumps the version")
	}

	// Scrolling b out of view cancels it.
	l.Want([]TileID{a})
	if reqs[1].Ctx.Err() == nil {
		t.Error("an obsolete fetch must be cancelled")
	}
	l.Done(reqs[1], nil, context.Canceled)
	if again := l.Want([]TileID{b}); len(again) != 1 {
		t.Error("a cancelled tile is not a failure and can be asked for again")
	}

	// A failure waits before retrying.
	c := TileID{3, 2, 2}
	src.fail.Store(true)
	r := l.Want([]TileID{c})[0]
	tile, err = l.Load(r)
	l.Done(r, tile, err)
	if len(l.Want([]TileID{c})) != 0 {
		t.Error("a failed tile must not be retried at once")
	}
	now = now.Add(retryAfter)
	if len(l.Want([]TileID{c})) != 1 {
		t.Error("a failed tile is retried after the wait")
	}
}

func TestLateResultDoesNotClobberANewerFetch(t *testing.T) {
	l := NewLoader(&fakeSource{}, 1000)
	id := TileID{4, 1, 1}
	old := l.Want([]TileID{id})[0]
	l.Want(nil)
	fresh := l.Want([]TileID{id})[0]
	l.Done(old, nil, context.Canceled)
	if l.Pending() != 1 || fresh.Ctx.Err() != nil {
		t.Error("the old fetch's result must leave the new one alone")
	}
}

func TestLoaderDecodesInTheBackground(t *testing.T) {
	src := &fakeSource{gate: make(chan struct{}), data: nil}
	l := NewLoader(src, 1000)
	r := l.Want([]TileID{{2, 1, 1}})[0]
	done := make(chan struct{})
	go func() {
		tile, err := l.Load(r)
		l.Done(r, tile, err)
		close(done)
	}()
	// The caller is never blocked by a slow fetch.
	if _, ok := l.Lookup(TileID{2, 1, 1}); ok {
		t.Fatal("not yet")
	}
	close(src.gate)
	<-done
	if _, ok := l.Lookup(TileID{2, 1, 1}); !ok {
		t.Error("arrived")
	}
}

func TestDiskCacheAndOfflineStart(t *testing.T) {
	ts := newTileServer(t, []byte{9, 9})
	dir := t.TempDir()
	d := NewDiskCache(dir, DefaultDiskBudget, NewHTTPSource(ts.srv.URL+"/planet", "vaegter/test"))
	ctx := context.Background()
	if b, err := d.Tile(ctx, TileID{0, 0, 0}); err != nil || len(b) != 2 {
		t.Fatalf("%v %v", b, err)
	}
	if b, _ := d.Tile(ctx, TileID{0, 0, 0}); len(b) != 2 || ts.hits.Load() != 1 {
		t.Errorf("a cached tile needs no request: %d", ts.hits.Load())
	}

	// A new process with the server down still draws what was cached.
	ts.failMeta.Store(true)
	offline := NewDiskCache(dir, DefaultDiskBudget, NewHTTPSource(ts.srv.URL+"/planet", "vaegter/test"))
	if b, err := offline.Tile(ctx, TileID{0, 0, 0}); err != nil || len(b) != 2 {
		t.Fatalf("offline: %v %v", b, err)
	}
	if ts.hits.Load() != 1 {
		t.Error("offline reads must come from disk")
	}
	info, err := os.Stat(filepath.Join(dir, "tilejson.json"))
	if err != nil {
		t.Fatalf("metadata saved: %v", err)
	}
	// Windows has no Unix permission bits; its files report 0666.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Errorf("metadata must be private: %v", info.Mode().Perm())
	}
}

func TestDiskCachePrunesOldestFirst(t *testing.T) {
	dir := t.TempDir()
	d := &DiskCache{Dir: dir, Budget: 25}
	old := time.Now().Add(-time.Hour)
	for i, name := range []string{"a.pbf", "b.pbf", "c.pbf"} {
		p := filepath.Join(dir, name)
		os.WriteFile(p, make([]byte, 10), 0o600)
		at := old.Add(time.Duration(i) * time.Minute)
		os.Chtimes(p, at, at)
	}
	if err := d.Prune(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.pbf")); err == nil {
		t.Error("the oldest tile goes first")
	}
	if _, err := os.Stat(filepath.Join(dir, "c.pbf")); err != nil {
		t.Error("the newest tile stays")
	}
}

func TestLocate(t *testing.T) {
	m := New(odense, 14)
	m.SetSize(80, 24)
	if col, row, ok := m.Locate(odense); !ok || col != 40 || row != 12 {
		t.Errorf("centre at %d,%d", col, row)
	}
	if _, _, ok := m.Locate(LatLon{-33.9, 151.2}); ok {
		t.Error("Sydney is not on a map of Odense")
	}
	if _, _, ok := m.Locate(LatLon{100, 0}); ok {
		t.Error("invalid")
	}
}

func TestDetailDrawsWhatACloserViewWould(t *testing.T) {
	// A tile covered in small buildings, so some fall in any view of it.
	var blocks [][]mvt.Pt
	for x := int32(0); x < 4096; x += 64 {
		for y := int32(0); y < 4096; y += 64 {
			blocks = append(blocks, []mvt.Pt{pt(x, y), pt(x+40, y), pt(x+40, y+40), pt(x, y+40), pt(x, y)})
		}
	}
	tile := &mvt.Tile{Layers: map[string]*mvt.Layer{"building": {Name: "building", Extent: 4096,
		Features: []mvt.Feature{{Type: mvt.Polygon, Geometry: blocks}}}}}
	asked := map[int]bool{}
	lookup := func(id TileID) (*mvt.Tile, bool) { asked[id.Z] = true; return tile, true }
	has := func(f *Frame) bool {
		for _, c := range f.Cells {
			if c.Class == ClassBuilding && c.R != ' ' {
				return true
			}
		}
		return false
	}
	m := New(odense, 15.4)
	m.SetSize(43, 12)
	if f, _ := m.Render(14, 1, lookup); has(f) {
		t.Fatal("buildings are hidden at zoom 15.4")
	}
	m.Detail = 0.6
	f, _ := m.Render(14, 1, lookup)
	if !has(f) {
		t.Error("with detail, buildings show as at zoom 16")
	}
	if m.View().Zoom != 15.4 {
		t.Error("detail must not change the area shown")
	}
	// Too far out to read, buildings stay hidden whatever the detail.
	far := New(odense, 13.4)
	far.SetSize(43, 12)
	far.Detail = 2.6
	if f, _ := far.Render(14, 1, lookup); has(f) {
		t.Error("buildings at zoom 13.4 would fill the panel")
	}
	if !asked[14] {
		t.Error("with detail, the deeper tiles are read")
	}
}
