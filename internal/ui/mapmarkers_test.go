package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/knightmare2600/vogter/internal/mapterm"
	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/poller"
)

func TestHostStatus(t *testing.T) {
	cases := []struct {
		row  model.HostRow
		want mapterm.Status
		anim mapterm.Animation
	}{
		{model.HostRow{Enabled: true, Availability: model.Up}, mapterm.StatusOK, mapterm.Still},
		{model.HostRow{Enabled: true, Availability: model.Up, Problems: [6]int{model.Warning: 1}}, mapterm.StatusWarning, mapterm.Still},
		{model.HostRow{Enabled: true, Availability: model.Up, Problems: [6]int{model.High: 1}}, mapterm.StatusCritical, mapterm.Still},
		{model.HostRow{Enabled: true, Availability: model.Down, Problems: [6]int{model.Disaster: 1}}, mapterm.StatusCritical, mapterm.Pulse},
		{model.HostRow{Enabled: true, Availability: model.Down}, mapterm.StatusUnknown, mapterm.Still},
		{model.HostRow{Enabled: false, Availability: model.Up}, mapterm.StatusUnknown, mapterm.Still},
		{model.HostRow{Enabled: true, Availability: model.Up, Problems: [6]int{model.Information: 1}}, mapterm.StatusOK, mapterm.Still},
	}
	for i, c := range cases {
		if s, a := hostStatus(&c.row); s != c.want || a != c.anim {
			t.Errorf("%d: %v %v", i, s, a)
		}
	}
}

// fixture hosts: web-01 (average problem, up), db-01 (high, down),
// switch-01 (disabled).
func located(t *testing.T) Model {
	t.Helper()
	m, _ := mapModel(t, true)
	var watched []bool
	m.opts.WatchMap = func(on bool) { watched = append(watched, on) }
	m, _ = press(m, "5")
	if len(watched) == 0 || !watched[len(watched)-1] {
		t.Fatalf("opening the map must start reading locations: %v", watched)
	}
	mm, _ := m.Update(eventMsg{poller.MapLocations{Locations: model.Locations{Total: 4, CityOnly: 1, Hosts: []model.HostLocation{
		{HostID: "1", Name: "web-01", Lat: 55.40, Lon: 10.38, Place: "Odense"},
		{HostID: "2", Name: "db-01", Lat: 55.68, Lon: 12.57, Place: "København"},
		{HostID: "3", Name: "switch-01", Lat: 56.16, Lon: 10.20},
	}}}})
	return mm.(Model)
}

func TestMarkersFromHosts(t *testing.T) {
	m := located(t)
	s := m.maps
	if len(s.markers) != 3 {
		t.Fatalf("%+v", s.markers)
	}
	if s.markers[0].Label != "db-01" || s.markers[0].Glyph != '■' {
		t.Errorf("worst first: %+v", s.markers[0])
	}
	if s.markers[1].Label != "web-01" || s.markers[1].Glyph != '▲' {
		t.Errorf("%+v", s.markers[1])
	}
	if !s.markers[0].Highlight {
		t.Error("the selection is highlighted")
	}
	wants(t, plain(m), "› db-01 (København)")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 36})
	m = mm.(Model)
	wants(t, plain(m), "3 hosts of 4 located", "1 with only a city")
	if m.opts.Map.Center == nil {
		t.Fatal("fixture has a start")
	}
}

func TestMapAutoFitsOnceWithoutAStart(t *testing.T) {
	m, _ := mapModel(t, true)
	m.opts.Map.Center = nil
	m, _ = press(m, "5")
	mm, _ := m.Update(eventMsg{poller.MapLocations{Locations: model.Locations{Total: 2, Hosts: []model.HostLocation{
		{HostID: "1", Name: "web-01", Lat: 55.40, Lon: 10.38}, {HostID: "2", Name: "db-01", Lat: 55.68, Lon: 12.57},
	}}}})
	m = mm.(Model)
	for _, mk := range m.maps.markers {
		if _, _, ok := m.maps.main.Locate(mk.Pos); !ok {
			t.Errorf("%s off screen after the fit", mk.Label)
		}
	}
	m, _ = press(m, "+")
	zoom := m.maps.main.Zoom
	mm, _ = m.Update(eventMsg{poller.MapLocations{Locations: m.maps.locations}})
	if mm.(Model).maps.main.Zoom != zoom {
		t.Error("a later refresh must not undo the user's view")
	}
}

func TestMapFiltersAndSelection(t *testing.T) {
	m := located(t)
	m, _ = press(m, "x")
	if len(m.maps.markers) != 2 {
		t.Errorf("problems only keeps db-01 and web-01: %d", len(m.maps.markers))
	}
	if !strings.Contains(m.locatedSummary(), "problems only") {
		t.Error(m.locatedSummary())
	}
	m, _ = press(m, "x", "/")
	m = typed(m, "web")
	m, _ = press(m, "enter")
	if len(m.maps.markers) != 1 || m.maps.markers[0].Label != "web-01" {
		t.Errorf("%+v", m.maps.markers)
	}
	m, _ = press(m, "esc")
	if m.mapFilter != "" || len(m.maps.markers) != 3 {
		t.Error("esc clears the filter")
	}
	// The selection followed web-01 through the filter.
	if m.maps.markers[m.maps.sel].Label != "web-01" {
		t.Fatalf("selection %q", m.maps.markers[m.maps.sel].Label)
	}
	m, _ = press(m, "n")
	if m.maps.markers[m.maps.sel].Label != "switch-01" || !m.maps.markers[m.maps.sel].Highlight || m.maps.markers[1].Highlight {
		t.Error("n moves the highlight to the next host")
	}
	if c := m.maps.main.Center; c.Lat != 56.16 {
		t.Errorf("n centres on it: %v", c)
	}
	m, _ = press(m, "p", "p")
	if m.maps.markers[m.maps.sel].Label != "db-01" {
		t.Error("p goes back, wrapping")
	}
}

func TestMapOpensTheHost(t *testing.T) {
	m := located(t)
	var watched []bool
	m.opts.WatchMap = func(on bool) { watched = append(watched, on) }
	m, _ = press(m, "enter")
	if m.view != Host || m.detailHost != "2" || m.backTo != Map {
		t.Fatalf("view %v host %q back %v", m.view, m.detailHost, m.backTo)
	}
	if len(watched) == 0 || watched[len(watched)-1] {
		t.Error("leaving the map stops reading locations")
	}
	m, _ = press(m, "esc")
	if m.view != Map || !watched[len(watched)-1] {
		t.Error("esc returns to the map and restarts the reading")
	}
}

func TestWidgetMarkers(t *testing.T) {
	all := []mapterm.Marker{{ID: "1", Highlight: true}, {ID: "2"}}
	if got := widgetMarkers(all, nil); len(got) != 2 {
		t.Error("no filter keeps all")
	}
	if got := widgetMarkers(all, []string{"1"}); len(got) != 1 || got[0].Highlight {
		t.Errorf("%+v", got)
	}
	if !all[0].Highlight {
		t.Error("the screen's markers are left alone")
	}
}

func TestMarkersOnScreen(t *testing.T) {
	m := located(t)
	m.maps.main.GoTo(mapterm.LatLon{Lat: 55.40, Lon: 10.38}, 12)
	m.frame = 0
	out := ansiRe.ReplaceAllString(m.View().Content, "")
	if !strings.Contains(out, "▲") {
		t.Error("web-01's warning marker is drawn at Odense")
	}
}
