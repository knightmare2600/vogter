package mvt

import (
	"os"
	"testing"
)

// TestRealTile decodes a tile from MVT_SAMPLE when set; it is not part of CI.
func TestRealTile(t *testing.T) {
	path := os.Getenv("MVT_SAMPLE")
	if path == "" {
		t.Skip("MVT_SAMPLE not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tile, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	for name, l := range tile.Layers {
		pts := 0
		for _, f := range l.Features {
			for _, p := range f.Geometry {
				pts += len(p)
			}
		}
		t.Logf("%-20s extent=%d features=%d points=%d", name, l.Extent, len(l.Features), pts)
	}
}
