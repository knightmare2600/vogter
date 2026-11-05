package ui

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/knightmare2600/vogter/internal/mapterm"
	"github.com/knightmare2600/vogter/internal/model"
)

// hostStatus reads a host's marker state from the problems and availability
// the rest of Vægter already shows.
func hostStatus(h *model.HostRow) (mapterm.Status, mapterm.Animation) {
	switch w := h.Worst(); {
	case w == model.Disaster:
		return mapterm.StatusCritical, mapterm.Pulse
	case w == model.High:
		return mapterm.StatusCritical, mapterm.Still
	case w >= model.Warning:
		return mapterm.StatusWarning, mapterm.Still
	case !h.Enabled || h.Maintenance || h.Availability != model.Up:
		return mapterm.StatusUnknown, mapterm.Still
	}
	return mapterm.StatusOK, mapterm.Still
}

// rebuildMarkers turns located hosts into markers, worst first, keeping only
// those the map's filters allow. The selection follows its host.
func (m *Model) rebuildMarkers() {
	s := m.maps
	if s == nil || m.snapshot == nil {
		return
	}
	rows := map[string]*model.HostRow{}
	for _, r := range model.FilterHosts(m.snapshot.Hosts, m.mapFilter, model.SortName) {
		rows[r.ID] = r
	}
	pal := s.main.Palette
	selected := ""
	if s.sel < len(s.markers) {
		selected = s.markers[s.sel].ID
	}
	var out []mapterm.Marker
	for _, loc := range s.locations.Hosts {
		row, ok := rows[loc.HostID]
		if !ok {
			continue
		}
		status, anim := hostStatus(row)
		if s.onlyProblems && status > mapterm.StatusWarning {
			continue
		}
		out = append(out, mapterm.Marker{
			ID: loc.HostID, Pos: mapterm.LatLon{Lat: loc.Lat, Lon: loc.Lon}, Label: loc.Name,
			Glyph: mapterm.StatusGlyphs[status], Colour: pal.Status[status], Category: loc.Place,
			Rank: int(status), Animation: anim,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Rank != out[j].Rank {
			return out[i].Rank < out[j].Rank
		}
		return out[i].Label < out[j].Label
	})
	s.sel = 0
	for i := range out {
		if out[i].ID == selected {
			s.sel = i
		}
	}
	if s.sel < len(out) && m.view == Map {
		out[s.sel].Highlight = true
	}
	s.markers = out
}

// onLocations stores fresh locations, and fits the map to them the first
// time when the user has not moved it and the config sets no start.
func (m *Model) onLocations(l model.Locations) {
	s := m.maps
	s.locations, s.haveLocations = l, true
	m.rebuildMarkers()
	if !s.fitted && !s.moved && m.opts.Map.Center == nil && len(s.markers) > 0 {
		s.fitted = true
		w, h := m.mapArea()
		s.main.SetSize(w, h)
		m.fitMap()
	}
}

// widgetMarkers keeps the hosts a geomap widget names, or all of them.
func widgetMarkers(all []mapterm.Marker, hostIDs []string) []mapterm.Marker {
	if len(hostIDs) == 0 {
		return all
	}
	var out []mapterm.Marker
	for _, mk := range all {
		if slices.Contains(hostIDs, mk.ID) {
			mk.Highlight = false
			out = append(out, mk)
		}
	}
	return out
}

// locatedSummary is the status line's count of placed hosts.
func (m Model) locatedSummary() string {
	s := m.maps
	if !s.haveLocations {
		return "reading host locations"
	}
	var b strings.Builder
	b.WriteString(plural(len(s.locations.Hosts), "host") + " of " + itoa(s.locations.Total) + " located")
	if n := s.locations.CityOnly; n > 0 {
		b.WriteString(" (" + itoa(n) + " with only a city)")
	}
	if s.onlyProblems {
		b.WriteString(" · problems only")
	}
	if m.mapFilter != "" {
		b.WriteString(" · filter " + m.mapFilter)
	}
	return b.String()
}

func itoa(n int) string { return strconv.Itoa(n) }

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
