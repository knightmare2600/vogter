package model

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/knightmare2600/vogter/internal/zabbix"
)

// HostLocation is where a host is, and where that came from.
type HostLocation struct {
	HostID, Name string
	Lat, Lon     float64
	// Place is the inventory's location and city, for display.
	Place string
	// FromConfig marks a position set in Vægter's config, not Zabbix.
	FromConfig bool
}

// Locations is every located host, with counts of those that are not.
type Locations struct {
	Hosts []HostLocation
	// Total is every host seen; CityOnly is how many have a city in their
	// inventory but no coordinates, which only geocoding could place.
	Total, CityOnly int
}

// ParseCoordinate reads a latitude or longitude as people type them into
// inventory: decimal degrees with a point or a decimal comma, and an optional
// degree sign or compass letter (S and W are negative).
func ParseCoordinate(s string, lat bool) (float64, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return 0, false
	}
	sign := 1.0
	for _, suffix := range []string{"N", "S", "E", "W"} {
		if rest, ok := strings.CutSuffix(s, suffix); ok {
			if (suffix == "N" || suffix == "S") != lat {
				return 0, false
			}
			if suffix == "S" || suffix == "W" {
				sign = -1
			}
			s = strings.TrimSpace(rest)
			break
		}
	}
	s = strings.TrimSpace(strings.TrimSuffix(s, "°"))
	if strings.Count(s, ",") == 1 && !strings.Contains(s, ".") {
		s = strings.Replace(s, ",", ".", 1)
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	v *= sign
	limit := 180.0
	if lat {
		limit = 90
	}
	if v < -limit || v > limit {
		return 0, false
	}
	return v, true
}

// Locate combines inventory positions with those set in config. A config
// entry, keyed by host ID or by name, wins over the inventory.
func Locate(inv []zabbix.HostInventory, configured map[string][2]float64) Locations {
	out := Locations{Total: len(inv)}
	for _, h := range inv {
		place := strings.Join(nonEmpty(h.Inventory.Location, h.Inventory.City, h.Inventory.Country), ", ")
		loc := HostLocation{HostID: h.HostID, Name: h.Name, Place: place}
		if c, ok := configured[h.HostID]; ok {
			loc.Lat, loc.Lon, loc.FromConfig = c[0], c[1], true
		} else if c, ok := configured[h.Name]; ok {
			loc.Lat, loc.Lon, loc.FromConfig = c[0], c[1], true
		} else {
			lat, okLat := ParseCoordinate(h.Inventory.Lat, true)
			lon, okLon := ParseCoordinate(h.Inventory.Lon, false)
			if !okLat || !okLon {
				if h.Inventory.City != "" {
					out.CityOnly++
				}
				continue
			}
			loc.Lat, loc.Lon = lat, lon
		}
		out.Hosts = append(out.Hosts, loc)
	}
	sort.Slice(out.Hosts, func(i, j int) bool { return out.Hosts[i].Name < out.Hosts[j].Name })
	return out
}

func nonEmpty(ss ...string) []string {
	var out []string
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
