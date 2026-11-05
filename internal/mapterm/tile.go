package mapterm

import (
	"fmt"
	"math"
)

// TileDots is how many canvas dots one tile spans at its own zoom. With braille
// cells two dots wide, a tile is 128 terminal columns across.
const TileDots = 256

// TileID names a tile in the usual z/x/y scheme.
type TileID struct{ Z, X, Y int }

func (t TileID) String() string { return fmt.Sprintf("%d/%d/%d", t.Z, t.X, t.Y) }

// Valid reports whether the tile exists at its zoom.
func (t TileID) Valid() bool {
	n := 1 << t.Z
	return t.Z >= 0 && t.Z <= 30 && t.X >= 0 && t.X < n && t.Y >= 0 && t.Y < n
}

// Parent is the tile at a lower zoom that contains this one.
func (t TileID) Parent(z int) TileID {
	if z >= t.Z {
		return t
	}
	s := uint(t.Z - z)
	return TileID{z, t.X >> s, t.Y >> s}
}

// TileAt is the tile holding a world position at zoom z.
func TileAt(w World, z int) TileID {
	n := float64(int(1) << z)
	x := int(math.Floor(w.X * n))
	y := int(math.Floor(w.Y * n))
	return wrap(TileID{z, x, y})
}

// wrap brings x into range and clamps y, as the world repeats east to west but
// not north to south.
func wrap(t TileID) TileID {
	n := 1 << t.Z
	t.X = ((t.X % n) + n) % n
	t.Y = max(0, min(n-1, t.Y))
	return t
}

// DataZoom is the tile zoom to draw a view zoom from. Zooming past the
// source's deepest level enlarges its tiles rather than asking for ones that
// do not exist.
func DataZoom(zoom float64, maxZoom int) int {
	return max(0, min(int(math.Floor(zoom)), maxZoom))
}
