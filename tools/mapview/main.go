// Command mapview draws one map view to the terminal, for developing the map
// renderer. It fetches only the tiles that view needs.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/knightmare2600/vogter/internal/mapterm"
)

func main() {
	lat := flag.Float64("lat", 55.39593945, "centre latitude")
	lon := flag.Float64("lon", 10.38620684, "centre longitude")
	zoom := flag.Float64("z", 15, "zoom")
	cols := flag.Int("cols", 100, "columns")
	rows := flag.Int("rows", 30, "rows")
	plain := flag.Bool("plain", false, "no colour")
	modeName := flag.String("mode", "braille", "braille, unicode or ascii")
	paletteName := flag.String("palette", "green", "palette name")
	labels := flag.Bool("labels", true, "show names")
	detail := flag.Float64("detail", 0, "draw as this many zooms closer would")
	flag.String("cache", "", "disk cache directory")
	flag.String("url", mapterm.OpenFreeMap, "TileJSON URL")
	flag.Parse()

	cacheDir := flag.Lookup("cache").Value.String()
	tileJSON := flag.Lookup("url").Value.String()
	http := mapterm.NewHTTPSource(tileJSON, "vaegter-mapview/dev (+https://github.com/knightmare2600/vogter)")
	var src mapterm.Source = http
	if cacheDir != "" {
		src = mapterm.NewDiskCache(cacheDir, mapterm.DefaultDiskBudget, http)
	}
	loader := mapterm.NewLoader(src, 4_000_000)
	m := mapterm.New(mapterm.LatLon{Lat: *lat, Lon: *lon}, *zoom)
	m.SetSize(*cols, *rows)
	m.Detail = *detail
	_, missing := m.Render(src.MaxZoom(), loader.Version(), loader.Lookup)
	fetchStart := time.Now()
	var wg sync.WaitGroup
	for _, r := range loader.Want(missing) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t, err := loader.Load(r)
			if err != nil {
				fmt.Fprintln(os.Stderr, r.ID, err)
			}
			loader.Done(r, t, err)
		}()
	}
	wg.Wait()
	fetched := time.Since(fetchStart)
	start := time.Now()
	m.Mode, _ = mapterm.ParseMode(*modeName)
	m.Labels = *labels
	if p, ok := mapterm.PaletteByName(*paletteName); ok {
		m.Palette = p
	}
	frame, left := m.Render(src.MaxZoom(), loader.Version(), loader.Lookup)
	took := time.Since(start)
	if *plain {
		for row := range frame.Rows {
			var b strings.Builder
			for col := range frame.Cols {
				b.WriteRune(frame.At(col, row).R)
			}
			fmt.Println(b.String())
		}
	} else {
		fmt.Println(strings.Join(frame.Lines(m.Palette), "\n"))
	}
	placed := 0
	for _, c := range frame.Cells {
		if c.Label {
			placed++
		}
	}
	fmt.Fprintf(os.Stderr, "label-cells=%d ", placed)
	counts := map[mapterm.Class]int{}
	for _, c := range frame.Cells {
		if c.R != ' ' && !c.Label {
			counts[c.Class]++
		}
	}
	fmt.Fprintf(os.Stderr, "classes=%v ", counts)
	fmt.Fprintf(os.Stderr, "%s  tiles=%d fetch=%s draw=%s missing=%d\n", mapterm.Attribution, len(missing), fetched.Round(time.Millisecond), took.Round(time.Microsecond), len(left))
}
