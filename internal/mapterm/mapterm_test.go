package mapterm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/knightmare2600/vogter/internal/mapterm/mvt"
)

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

var odense = LatLon{55.39593945, 10.38620684}

func TestProjectionRoundTrip(t *testing.T) {
	for _, p := range []LatLon{{0, 0}, odense, {-33.8568, 151.2153}, {84.9, -179.9}, {-84.9, 179.9}} {
		back := Unproject(Project(p))
		if !near(back.Lat, p.Lat, 1e-9) || !near(back.Lon, p.Lon, 1e-9) {
			t.Errorf("%v came back as %v", p, back)
		}
	}
	if w := Project(LatLon{0, 0}); w != (World{0.5, 0.5}) {
		t.Errorf("null island %v", w)
	}
	if w := Project(LatLon{90, 0}); !near(w.Y, 0, 1e-9) {
		t.Errorf("latitude must clamp at the top of the square: %v", w)
	}
}

func TestTileAt(t *testing.T) {
	// Checked against the standard slippy map formula, computed separately.
	if got := TileAt(Project(odense), 14); got != (TileID{14, 8664, 5150}) {
		t.Errorf("odense %v", got)
	}
	if got := TileAt(World{1.25, 0.5}, 2); got != (TileID{2, 1, 2}) {
		t.Errorf("x must wrap: %v", got)
	}
	if got := TileAt(World{0.5, 1.5}, 2); got.Y != 3 {
		t.Errorf("y must clamp: %v", got)
	}
	if p := (TileID{14, 8664, 5150}).Parent(12); p != (TileID{12, 2166, 1287}) {
		t.Errorf("parent %v", p)
	}
	if (TileID{3, 8, 0}).Valid() || !(TileID{3, 7, 7}).Valid() {
		t.Error("validity")
	}
}

func TestLatLonValid(t *testing.T) {
	for _, bad := range []LatLon{{91, 0}, {0, 181}, {math.NaN(), 0}, {0, math.Inf(1)}} {
		if bad.Valid() {
			t.Errorf("%v must be invalid", bad)
		}
	}
	if !odense.Valid() {
		t.Error("odense")
	}
}

func TestDataZoom(t *testing.T) {
	for _, c := range [][3]float64{{15.7, 14, 14}, {9.9, 14, 9}, {-1, 14, 0}} {
		if got := DataZoom(c[0], int(c[1])); got != int(c[2]) {
			t.Errorf("%v: %d", c, got)
		}
	}
}

func TestViewDotsRoundTrip(t *testing.T) {
	v := View{Center: odense, Zoom: 15.5, W: 200, H: 120}
	x, y := v.ToDot(Project(odense))
	if !near(x, 100, 1e-6) || !near(y, 60, 1e-6) {
		t.Fatalf("centre at %v,%v", x, y)
	}
	w := v.FromDot(13, 77)
	if x, y := v.ToDot(w); !near(x, 13, 1e-6) || !near(y, 77, 1e-6) {
		t.Errorf("%v %v", x, y)
	}
}

func TestViewTilesAcrossTheAntimeridian(t *testing.T) {
	v := View{Center: LatLon{0, 179.99}, Zoom: 3, W: 400, H: 100}
	tiles := v.Tiles(3)
	xs := map[int]bool{}
	for _, tl := range tiles {
		xs[tl.X] = true
		if !tl.Valid() {
			t.Errorf("invalid %v", tl)
		}
	}
	if !xs[0] || !xs[7] {
		t.Errorf("both sides of the date line: %v", tiles)
	}
	if first := tiles[0]; first.X != 7 && first.X != 0 {
		t.Errorf("nearest first: %v", tiles)
	}
}

func TestViewTilesWholeWorld(t *testing.T) {
	v := View{Center: LatLon{0, 0}, Zoom: 0.5, W: 2000, H: 2000}
	if n := len(v.Tiles(0)); n != 1 {
		t.Errorf("zoom 0 has one tile: %d", n)
	}
	if n := len((View{W: 0, H: 10}).Tiles(5)); n != 0 {
		t.Error("an empty view needs no tiles")
	}
}

func TestBraille(t *testing.T) {
	c := NewCanvas(2, 1)
	c.Set(0, 0, ClassPark)
	c.Set(1, 3, ClassMotorway)
	c.Set(2, 0, ClassPark)
	c.Set(-1, 0, ClassPark)
	c.Set(4, 0, ClassPark)
	d, class := c.Cell(0, 0)
	if Braille(d) != '⢁' || class != ClassMotorway {
		t.Errorf("%q %v", Braille(d), class)
	}
	if d, _ := c.Cell(1, 0); Braille(d) != '⠁' {
		t.Errorf("%q", Braille(d))
	}
	full := NewCanvas(1, 1)
	for y := range 4 {
		for x := range 2 {
			full.Set(x, y, ClassLand)
		}
	}
	if d, _ := full.Cell(0, 0); Braille(d) != '⣿' {
		t.Errorf("%q", Braille(d))
	}
}

func lit(c *Canvas) map[[2]int]bool {
	out := map[[2]int]bool{}
	for y := range c.H() {
		for x := range c.W() {
			if d, _ := c.Cell(x/CellW, y/CellH); d&bit[y%CellH][x%CellW] != 0 {
				out[[2]int{x, y}] = true
			}
		}
	}
	return out
}

func TestLine(t *testing.T) {
	c := NewCanvas(4, 2)
	c.Line(0, 0, 7, 7, ClassMajorRoad)
	got := lit(c)
	for i := range 8 {
		if !got[[2]int{i, i}] {
			t.Errorf("diagonal misses %d", i)
		}
	}
	if len(got) != 8 {
		t.Errorf("%d dots", len(got))
	}
	off := NewCanvas(4, 2)
	off.Line(-100, -5, 200, -5, ClassMajorRoad)
	if len(lit(off)) != 0 {
		t.Error("a line above the canvas must draw nothing")
	}
	long := NewCanvas(4, 2)
	long.Line(-1e9, 3, 1e9, 3, ClassMajorRoad)
	if len(lit(long)) != 8 {
		t.Errorf("a huge line is clipped to the canvas: %d", len(lit(long)))
	}
}

func TestFillWithHole(t *testing.T) {
	c := NewCanvas(5, 3) // 10×12 dots
	outer := []FPt{{0, 0}, {10, 0}, {10, 12}, {0, 12}, {0, 0}}
	hole := []FPt{{3, 3}, {7, 3}, {7, 9}, {3, 9}, {3, 3}}
	c.Fill([][]FPt{outer, hole}, ClassBuilding, nil)
	got := lit(c)
	if !got[[2]int{1, 1}] || !got[[2]int{9, 11}] {
		t.Error("the area must fill")
	}
	if got[[2]int{5, 6}] {
		t.Error("the hole must stay empty")
	}
	if len(got) != 120-24 {
		t.Errorf("%d dots", len(got))
	}
	sparse := NewCanvas(5, 3)
	sparse.Fill([][]FPt{outer}, ClassWater, func(x, y int) bool { return x%2 == 0 })
	if len(lit(sparse)) != 60 {
		t.Errorf("pattern: %d", len(lit(sparse)))
	}
}

func TestCacheEvictsLeastRecentlyUsed(t *testing.T) {
	c := NewCache[string](10)
	c.Put(TileID{1, 0, 0}, "a", 4)
	c.Put(TileID{1, 1, 0}, "b", 4)
	c.Get(TileID{1, 0, 0})
	c.Put(TileID{1, 0, 1}, "c", 4)
	if _, ok := c.Get(TileID{1, 1, 0}); ok {
		t.Error("b was least recently used")
	}
	if _, ok := c.Get(TileID{1, 0, 0}); !ok {
		t.Error("a was used recently")
	}
	c.Put(TileID{2, 0, 0}, "huge", 100)
	if _, ok := c.Get(TileID{2, 0, 0}); !ok || c.Len() != 1 {
		t.Errorf("the newest stays even over budget: %d", c.Len())
	}
	c.Put(TileID{2, 0, 0}, "again", 1)
	if v, _ := c.Get(TileID{2, 0, 0}); v != "again" || c.Len() != 1 {
		t.Error("replacing keeps one entry")
	}
}

// tileServer serves a TileJSON and counts requests and their concurrency.
type tileServer struct {
	srv                  *httptest.Server
	tileJSON, agents     atomic.Value
	hits, inFlight, peak atomic.Int32
	failMeta, slow       atomic.Bool
}

func newTileServer(t *testing.T, tile []byte) *tileServer {
	ts := &tileServer{}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.agents.Store(r.Header.Get("User-Agent"))
		if r.URL.Path == "/planet" {
			if ts.failMeta.Load() {
				http.Error(w, "down", http.StatusBadGateway)
				return
			}
			fmt.Fprintf(w, `{"tilejson":"3.0.0","maxzoom":14,"tiles":["%s/build1/{z}/{x}/{y}.pbf"]}`, ts.srv.URL)
			return
		}
		ts.hits.Add(1)
		n := ts.inFlight.Add(1)
		defer ts.inFlight.Add(-1)
		for p := ts.peak.Load(); n > p && !ts.peak.CompareAndSwap(p, n); p = ts.peak.Load() {
		}
		if ts.slow.Load() {
			time.Sleep(30 * time.Millisecond)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/0/0/0.pbf"):
			w.Write(tile)
		case strings.Contains(r.URL.Path, "/9/"):
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(ts.srv.Close)
	return ts
}

func TestHTTPSource(t *testing.T) {
	ts := newTileServer(t, []byte{1, 2, 3})
	s := NewHTTPSource(ts.srv.URL+"/planet", "vaegter/test")
	ctx := context.Background()
	b, err := s.Tile(ctx, TileID{0, 0, 0})
	if err != nil || string(b) != "\x01\x02\x03" {
		t.Fatalf("%v %v", b, err)
	}
	if ts.agents.Load() != "vaegter/test" {
		t.Error("the User-Agent must name the application")
	}
	if b, err := s.Tile(ctx, TileID{9, 1, 1}); err != nil || len(b) != 0 {
		t.Errorf("a missing tile is empty, not an error: %v %v", b, err)
	}
	if _, err := s.Tile(ctx, TileID{2, 1, 1}); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("%v", err)
	}
	if _, err := s.Tile(ctx, TileID{2, 9, 9}); err == nil {
		t.Error("a tile outside the world is refused")
	}
}

func TestHTTPSourceKeepsWorkingWhenMetadataFails(t *testing.T) {
	ts := newTileServer(t, []byte{1})
	s := NewHTTPSource(ts.srv.URL+"/planet", "vaegter/test")
	if _, err := s.Tile(context.Background(), TileID{0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	ts.failMeta.Store(true)
	s.loadedAt = time.Time{}
	if _, err := s.Tile(context.Background(), TileID{0, 0, 0}); err != nil {
		t.Errorf("a stale template must still be used: %v", err)
	}
	fresh := NewHTTPSource(ts.srv.URL+"/planet", "vaegter/test")
	if _, err := fresh.Tile(context.Background(), TileID{0, 0, 0}); err == nil {
		t.Error("with no template at all, the failure is reported")
	}
}

func TestHTTPSourceLimitsParallelRequests(t *testing.T) {
	ts := newTileServer(t, []byte{1})
	ts.slow.Store(true)
	s := NewHTTPSource(ts.srv.URL+"/planet", "vaegter/test")
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); s.Tile(context.Background(), TileID{0, 0, 0}) }()
	}
	wg.Wait()
	if p := ts.peak.Load(); p > maxParallel || p < 2 {
		t.Errorf("peak %d parallel requests", p)
	}
}

func TestHTTPSourceHonoursCancellation(t *testing.T) {
	ts := newTileServer(t, []byte{1})
	s := NewHTTPSource(ts.srv.URL+"/planet", "vaegter/test")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Tile(ctx, TileID{0, 0, 0}); !errors.Is(err, context.Canceled) {
		t.Errorf("%v", err)
	}
}

func pt(x, y int32) mvt.Pt { return mvt.Pt{X: x, Y: y} }

// squareTile is one tile holding a water square in its middle quarter and a
// road across its top.
func squareTile() *mvt.Tile {
	return &mvt.Tile{Layers: map[string]*mvt.Layer{
		"water": {Name: "water", Extent: 4096, Features: []mvt.Feature{{Type: mvt.Polygon,
			Geometry: [][]mvt.Pt{{pt(1024, 1024), pt(3072, 1024), pt(3072, 3072), pt(1024, 3072), pt(1024, 1024)}}}}},
		"transportation": {Name: "transportation", Extent: 4096, Features: []mvt.Feature{{Type: mvt.LineString,
			Props: map[string]any{"class": "motorway"}, Geometry: [][]mvt.Pt{{pt(0, 400), pt(4096, 400)}}}}},
	}}
}

func TestDrawPlacesFeatures(t *testing.T) {
	// Zoom 0 shows the whole world as one tile of 256×256 dots.
	v := View{Center: LatLon{0, 0}, Zoom: 0, W: 256, H: 256}
	c := NewCanvas(128, 64)
	tile := squareTile()
	missing := Draw(c, v, 14, func(id TileID) (*mvt.Tile, bool) { return tile, id == (TileID{0, 0, 0}) }, DefaultStyle)
	if len(missing) != 0 {
		t.Fatalf("%v", missing)
	}
	water := 0
	for row := 20; row < 44; row++ {
		for col := 40; col < 88; col++ {
			if d, cl := c.Cell(col, row); d != 0 && cl == ClassWater {
				water++
			}
		}
	}
	if water < 200 {
		t.Errorf("the middle must be textured as water: %d cells", water)
	}
	if _, cl := c.Cell(10, 6); cl != ClassMotorway {
		t.Errorf("road row %v", cl)
	}
	if d, _ := c.Cell(10, 50); d != 0 {
		t.Error("outside the features must be blank")
	}
}

func TestDrawUsesAParentWhileATileLoads(t *testing.T) {
	v := View{Center: Unproject(World{0.375, 0.375}), Zoom: 2, W: 64, H: 64}
	tile := squareTile()
	asked := map[TileID]bool{}
	c := NewCanvas(32, 16)
	missing := Draw(c, v, 14, func(id TileID) (*mvt.Tile, bool) {
		asked[id] = true
		return tile, id.Z == 0
	}, DefaultStyle)
	if len(missing) == 0 || missing[0].Z != 2 {
		t.Fatalf("%v", missing)
	}
	if !asked[TileID{0, 0, 0}] {
		t.Error("a parent must be tried")
	}
	if d, _ := c.Cell(16, 8); d == 0 {
		t.Error("the stand-in must draw")
	}
}

func TestMetresPerDot(t *testing.T) {
	if m := MetresPerDot(0, 0); !near(m, 156543.03, 0.01) {
		t.Errorf("%v", m)
	}
}
