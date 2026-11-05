package mapterm

import (
	"fmt"

	"github.com/knightmare2600/vogter/internal/mapterm/mvt"
)

// Classes, lowest first, so a road crossing a park keeps the road's colour.
const (
	ClassNone Class = iota
	ClassLand
	ClassPark
	ClassWater
	ClassBuilding
	ClassBoundary
	ClassRail
	ClassMinorRoad
	ClassMajorRoad
	ClassMotorway
	ClassWaterway
)

// Rule says how to draw a feature.
type Rule struct {
	Class   Class
	Fill    bool
	Pattern func(x, y int) bool
}

// Style chooses rules for features from the OpenMapTiles schema.
type Style struct {
	// Order is the layers to draw, bottom first.
	Order []string
	// LandBelow textures land below this zoom, so continents read as solid
	// against open sea; closer in, it would sit under every street.
	LandBelow float64
	Match     func(layer string, f *mvt.Feature, zoom float64) (Rule, bool)
}

func str(f *mvt.Feature, key string) string {
	if v, ok := f.Props[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

// noise is a fixed pseudo-random value for a dot, so fills look like texture
// rather than the stripes a regular pattern makes.
func noise(x, y int) uint32 {
	h := uint32(x)*0x9E3779B1 ^ uint32(y)*0x85EBCA77
	h ^= h >> 15
	h *= 0x2C1B3C6D
	return h ^ h>>12
}

// Sparse patterns keep large areas light, so lines stay readable over them.
func sparse(x, y int) bool      { return noise(x, y)%5 == 0 }
func scatter(x, y int) bool     { return noise(x, y)%11 == 0 }
func landTexture(x, y int) bool { return noise(x, y)%4 == 0 }
func woodland(x, y int) bool    { return noise(x, y)%22 == 0 }

// Fills reports whether a class is an area texture rather than a line.
func (c Class) Fills() bool { return c == ClassPark || c == ClassWater || c == ClassLand }

// DefaultStyle draws the OpenMapTiles layers OpenFreeMap serves.
var DefaultStyle = Style{
	Order:     []string{"landcover", "landuse", "park", "water", "waterway", "building", "boundary", "transportation"},
	LandBelow: 9,
	Match: func(layer string, f *mvt.Feature, zoom float64) (Rule, bool) {
		switch layer {
		case "water":
			// A shoreline, not a fill: open sea filled with texture is the
			// busiest part of any map that shows it.
			return Rule{Class: ClassWater}, true
		case "waterway":
			return Rule{Class: ClassWaterway}, true
		case "park":
			// This layer is protected areas, not parks: national parks,
			// reserves, Natura 2000 sites and the like, many of them at sea,
			// such as the wildlife reserve over Odense Fjord. Textured, they
			// cover sea and land alike. Town parks come from landcover.
			return Rule{}, false
		case "landcover":
			// Only what a reader would call green space is textured. Meadow,
			// grassland, heath, scrub, wetland and farmland cover most of
			// the countryside, and would texture the whole map.
			switch str(f, "class") {
			case "wood":
				return Rule{Class: ClassPark, Fill: true, Pattern: woodland}, true
			case "grass":
				switch str(f, "subclass") {
				case "park", "recreation_ground", "village_green", "garden", "golf_course":
					return Rule{Class: ClassPark, Fill: true, Pattern: scatter}, true
				}
			}
		case "landuse":
			if str(f, "class") == "cemetery" || str(f, "class") == "pitch" {
				return Rule{Class: ClassPark, Fill: true, Pattern: scatter}, true
			}
		case "building":
			// Below street level, outlines of every building merge into noise.
			if zoom >= 16 {
				return Rule{Class: ClassBuilding}, true
			}
		case "boundary":
			// Borders at sea, such as the ring around the Azores, would draw
			// lines across open water; state borders wait for zoom 5.
			if m, _ := f.Props["maritime"].(int64); m == 1 {
				return Rule{}, false
			}
			lvl, _ := f.Props["admin_level"].(int64)
			if lvl == 2 || (lvl > 2 && lvl <= 4 && zoom >= 5) {
				return Rule{Class: ClassBoundary}, true
			}
		case "transportation":
			switch str(f, "class") {
			case "motorway", "trunk":
				return Rule{Class: ClassMotorway}, true
			case "primary", "secondary", "tertiary":
				return Rule{Class: ClassMajorRoad}, true
			case "minor", "busway":
				if zoom >= 14 {
					return Rule{Class: ClassMinorRoad}, true
				}
			case "service":
				if zoom >= 15 {
					return Rule{Class: ClassMinorRoad}, true
				}
			case "path":
				// Pedestrian streets are streets: in an old town centre they
				// are most of them. Footpaths and cycleways wait for street level.
				if str(f, "subclass") == "pedestrian" && zoom >= 14 {
					return Rule{Class: ClassMinorRoad}, true
				}
				if zoom >= 16 {
					return Rule{Class: ClassMinorRoad}, true
				}
			case "track":
				if zoom >= 16 {
					return Rule{Class: ClassMinorRoad}, true
				}
			case "rail", "transit":
				return Rule{Class: ClassRail}, true
			}
		}
		return Rule{}, false
	},
}
