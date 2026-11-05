package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/knightmare2600/vogter/internal/mapterm"
	"github.com/knightmare2600/vogter/internal/mapterm/mvt"
	"github.com/knightmare2600/vogter/internal/model"
)

// MapOptions configures the map screen.
type MapOptions struct {
	// Enabled starts the map on; otherwise it waits to be turned on.
	Enabled bool
	// NewSource builds the tile source the first time the map is used, so
	// nothing is contacted while the map stays off.
	NewSource func() mapterm.Source
	Center    *mapterm.LatLon
	Zoom      float64
	Mode      mapterm.Mode
	Palette   mapterm.Palette
	Labels    bool
	Hidden    map[mapterm.Layer]bool
}

const (
	// mapPanStep is how far one key pans, as a share of the screen.
	mapPanStep = 0.15
	// mapFitMaxZoom stops a fit around one site at street level.
	mapFitMaxZoom = 16
	// mapMemory is the tile cache budget in points. City tiles measured 18,000
	// to 40,000 points each, so it holds about 100 to 200 of them.
	mapMemory = 4_000_000
	// mapUserAgent names Vægter to the tile provider, as its terms ask.
	mapUserAgent = "vaegter/%s (+https://github.com/knightmare2600/vogter)"
)

// MapUserAgent is the User-Agent for tile requests.
func MapUserAgent(version string) string { return fmt.Sprintf(mapUserAgent, version) }

// mapTileMsg carries one fetched tile back to the UI goroutine.
type mapTileMsg struct {
	req  mapterm.Request
	tile *mvt.Tile
	err  error
}

// mapState is the map screen's state, shared by pointer so that View, which
// has a value receiver, sees the same cached frames.
type mapState struct {
	on      bool
	source  mapterm.Source
	loader  *mapterm.Loader
	main    *mapterm.Map
	widgets map[string]*mapterm.Map

	prompting bool
	prompt    string
	layers    bool
	markers   []mapterm.Marker
	sel       int

	locations     model.Locations
	haveLocations bool
	// fitted and moved decide the one automatic fit to the hosts.
	fitted, moved bool
	onlyProblems  bool
}

func newMapState(o MapOptions) *mapState {
	center, zoom := mapterm.LatLon{Lat: 30}, 1.0
	if o.Center != nil && o.Center.Valid() {
		center, zoom = *o.Center, o.Zoom
	}
	mm := mapterm.New(center, zoom)
	mm.Mode, mm.Labels = o.Mode, o.Labels
	if o.Palette.Name != "" {
		mm.Palette = o.Palette
	}
	for l, h := range o.Hidden {
		mm.Hidden[l] = h
	}
	return &mapState{main: mm, widgets: map[string]*mapterm.Map{}}
}

// turnOn builds the source and loader on first use.
func (m *Model) turnMapOn() {
	s := m.maps
	if s.on {
		return
	}
	if s.loader == nil && m.opts.Map.NewSource != nil {
		s.source = m.opts.Map.NewSource()
		s.loader = mapterm.NewLoader(s.source, mapMemory)
	}
	s.on = s.loader != nil
	if m.opts.ASCII {
		s.main.Mode = mapterm.ModeASCII
	}
	m.watchMap()
}

// mapArea is the map's size: the whole body less its status line.
func (m Model) mapArea() (int, int) {
	return m.width, max(m.height-headerH-footerH-1, 1)
}

// mapSync draws whatever map is on screen from cache and starts fetching the
// tiles it lacks. It runs after every update; an unchanged view costs almost
// nothing, because its frame is cached.
func (m *Model) mapSync() tea.Cmd {
	s := m.maps
	if s == nil || !s.on || s.loader == nil {
		return nil
	}
	maxZoom := s.source.MaxZoom()
	var missing []mapterm.TileID
	switch {
	case m.view == Map && m.snapshot != nil:
		w, h := m.mapArea()
		s.main.SetSize(w, h)
		_, miss := s.main.Render(maxZoom, s.loader.Version(), s.loader.Lookup)
		missing = append(missing, miss...)
	case m.view == Boards:
		for _, g := range m.geomapWidgets() {
			_, miss := g.Render(maxZoom, s.loader.Version(), s.loader.Lookup)
			missing = append(missing, miss...)
		}
	}
	var cmds []tea.Cmd
	for _, r := range s.loader.Want(missing) {
		loader := s.loader
		cmds = append(cmds, func() tea.Msg {
			t, err := loader.Load(r)
			return mapTileMsg{r, t, err}
		})
	}
	return tea.Batch(cmds...)
}

func (m *Model) onMapTile(msg mapTileMsg) {
	if m.maps.loader != nil {
		m.maps.loader.Done(msg.req, msg.tile, msg.err)
	}
}

func (m Model) onMapKey(key string) (tea.Model, tea.Cmd) {
	s := m.maps
	if !s.on {
		if key == "e" {
			m.turnMapOn()
			if s.on {
				m.addToast(toastInfo, "Map on for this session", "tiles from OpenFreeMap · set enabled = true under [map] to keep it")
			}
		}
		return m, nil
	}
	mm := s.main
	switch key {
	case "left", "h", "right", "l", "+", "=", "-", "_", ":", "b", "f", "c", "n", "p":
		s.moved = true
	}
	switch key {
	case "enter":
		if s.sel < len(s.markers) {
			m.openHost(s.markers[s.sel].ID)
		}
	case "x":
		s.onlyProblems = !s.onlyProblems
		m.rebuildMarkers()
	case "left", "h":
		mm.Pan(-mapPanStep, 0)
	case "right", "l":
		mm.Pan(mapPanStep, 0)
	case "+", "=":
		mm.ZoomBy(1)
	case "-", "_":
		mm.ZoomBy(-1)
	case "b":
		if !mm.Back() {
			m.addToast(toastInfo, "Map", "nothing to go back to")
		}
	case "f":
		m.fitMap()
	case "c":
		if s.sel < len(s.markers) {
			mm.GoTo(s.markers[s.sel].Pos, max(mm.Zoom, 14))
		}
	case "n", "p":
		if len(s.markers) > 0 {
			step := 1
			if key == "p" {
				step = len(s.markers) - 1
			}
			s.markers[s.sel].Highlight = false
			s.sel = (s.sel + step) % len(s.markers)
			s.markers[s.sel].Highlight = true
			mm.GoTo(s.markers[s.sel].Pos, max(mm.Zoom, 12))
		}
	case "L":
		mm.Labels = !mm.Labels
	case "v":
		if m.opts.ASCII {
			m.addToast(toastInfo, "Map", "ASCII mode is on, so the map stays in ASCII")
		} else {
			mm.NextMode()
			m.addToast(toastInfo, "Map mode", mm.Mode.String())
		}
	case "P":
		m.nextMapPalette()
	case "y":
		s.layers = true
	case ":":
		s.prompting, s.prompt = true, ""
	}
	return m, nil
}

func (m *Model) nextMapPalette() {
	mm := m.maps.main
	for i, p := range mapterm.Palettes {
		if p.Name == mm.Palette.Name {
			mm.Palette = mapterm.Palettes[(i+1)%len(mapterm.Palettes)]
			for _, g := range m.maps.widgets {
				g.Palette = mm.Palette
			}
			m.addToast(toastInfo, "Map palette", mm.Palette.Name)
			return
		}
	}
	mm.Palette = mapterm.Palettes[0]
}

// fitMap shows every marker, or returns to the configured start.
func (m *Model) fitMap() {
	s := m.maps
	var pts []mapterm.LatLon
	for _, mk := range s.markers {
		pts = append(pts, mk.Pos)
	}
	if s.main.Fit(pts, mapFitMaxZoom) {
		return
	}
	if c := m.opts.Map.Center; c != nil && c.Valid() {
		s.main.GoTo(*c, m.opts.Map.Zoom)
		return
	}
	s.main.GoTo(mapterm.LatLon{Lat: 30}, 1)
}

// onMapLayerKey handles the layers menu: 1 to 5 toggle, anything else closes.
func (m Model) onMapLayerKey(key string) (tea.Model, tea.Cmd) {
	if n, err := strconv.Atoi(key); err == nil && n >= 1 && n <= len(mapterm.AllLayers) {
		m.maps.main.ToggleLayer(mapterm.AllLayers[n-1])
		return m, nil
	}
	m.maps.layers = false
	return m, nil
}

// onMapPromptKey edits the go-to prompt.
func (m Model) onMapPromptKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := m.maps
	switch k.String() {
	case "esc":
		s.prompting = false
	case "enter":
		s.prompting = false
		at, zoom, err := parseCoords(s.prompt, s.main.Zoom)
		if err != nil {
			m.addToast(toastError, "Go to", err.Error())
			return m, nil
		}
		s.main.GoTo(at, zoom)
	case "backspace":
		s.prompt = dropLast(s.prompt)
	default:
		s.prompt += k.Text
	}
	return m, nil
}

// parseCoords reads "lat, lon" or "lat lon zoom" in decimal degrees.
func parseCoords(s string, zoom float64) (mapterm.LatLon, float64, error) {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
	if len(fields) < 2 || len(fields) > 3 {
		return mapterm.LatLon{}, 0, fmt.Errorf("type latitude, longitude and an optional zoom, such as 55.396, 10.386, 14")
	}
	var v [3]float64
	for i, f := range fields {
		n, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return mapterm.LatLon{}, 0, fmt.Errorf("%q is not a number", f)
		}
		v[i] = n
	}
	at := mapterm.LatLon{Lat: v[0], Lon: v[1]}
	if !at.Valid() || at.Lat < -mapterm.MaxLat || at.Lat > mapterm.MaxLat {
		return mapterm.LatLon{}, 0, fmt.Errorf("%g, %g is not on the map", v[0], v[1])
	}
	if len(fields) == 3 {
		zoom = v[2]
	}
	return at, zoom, nil
}

func (m Model) mapView(w, h int) []string {
	t := &m.theme
	s := m.maps
	if !s.on {
		lines := []string{"",
			title(t, "The map is off"), "",
			fg(t.FG).Render("It draws OpenStreetMap data from OpenFreeMap. Drawing it sends the area you"),
			fg(t.FG).Render("view, and your IP address, to OpenFreeMap and its CDN, Cloudflare."), "",
			fg(t.Accent).Bold(true).Render("e") + fg(t.Muted).Render(" turn it on for this session   ") +
				fg(t.Muted).Render("or set ") + fg(t.FG).Render("enabled = true") + fg(t.Muted).Render(" under [map]"), "",
			fg(t.Border).Render("Map data © OpenStreetMap contributors (ODbL) · tiles © OpenMapTiles"),
		}
		out := make([]string, len(lines))
		for i, l := range lines {
			out[i] = center(l, w-2)
		}
		return panel(t, w, h, panelOpts{title: title(t, "Map")}, out)
	}
	mm := s.main
	frame, _ := mm.Render(s.source.MaxZoom(), s.loader.Version(), s.loader.Lookup)
	placed := mm.PlaceMarkers(s.markers)
	shown, colours := mapterm.Overlay(frame, placed, m.frame, mm.Mode == mapterm.ModeASCII)
	lines := shown.LinesWith(mm.Palette, colours)
	for len(lines) < h-1 {
		lines = append(lines, "")
	}
	lines = lines[:max(h-1, 0)]
	if s.layers {
		lines = m.overlayBox(lines, w, m.layersBox())
	}
	return append(lines, m.mapStatus(w))
}

func (m Model) mapStatus(w int) string {
	t := &m.theme
	s := m.maps
	mm := s.main
	if s.prompting {
		return lipgloss.NewStyle().Foreground(t.BG).Background(t.Accent).Bold(true).Render(" go to ") + " " +
			fg(t.FG).Render(s.prompt) + fg(t.Accent).Render("▏") + fg(t.Muted).Render("   lat, lon[, zoom]   ⏎ go  esc cancel")
	}
	ns, ew := "N", "E"
	lat, lon := mm.Center.Lat, mm.Center.Lon
	if lat < 0 {
		ns, lat = "S", -lat
	}
	if lon < 0 {
		ew, lon = "W", -lon
	}
	// Most useful first, as a narrow screen cuts from the end.
	parts := []string{fmt.Sprintf("z%.0f", mm.Zoom), fmt.Sprintf("%.4f%s %.4f%s", lat, ns, lon, ew)}
	if len(s.markers) > 0 && s.sel < len(s.markers) {
		mk := s.markers[s.sel]
		sel := "› " + mk.Label
		if mk.Category != "" {
			sel += " (" + mk.Category + ")"
		}
		parts = append(parts, sel)
	}
	if n := s.loader.Pending(); n > 0 {
		parts = append(parts, fmt.Sprintf("%s loading %d", spinner[m.frame%len(spinner)], n))
	}
	if n := s.loader.Failing(); n > 0 {
		parts = append(parts, fmt.Sprintf("%d tiles unavailable", n))
	}
	parts = append(parts, m.locatedSummary(), mm.Mode.String(), mm.Palette.Name)
	right := fg(t.Border).Render(mapterm.Attribution + " ")
	room := w - lipgloss.Width(right) - 2
	if room < 8 {
		// The credit is required, so it is the last thing to go.
		return pad(right, w)
	}
	left := " " + fg(t.Muted).Render(truncate(strings.Join(parts, " · "), room))
	return left + strings.Repeat(" ", max(w-lipgloss.Width(left)-lipgloss.Width(right), 1)) + right
}

func (m Model) layersBox() []string {
	t := &m.theme
	lines := []string{""}
	for i, l := range mapterm.AllLayers {
		mark := "■"
		if m.maps.main.Hidden[l] {
			mark = "□"
		}
		lines = append(lines, fmt.Sprintf(" %s %s %s", fg(t.Accent).Bold(true).Render(strconv.Itoa(i+1)), fg(t.Accent).Render(mark), fg(t.FG).Render(l.String())))
	}
	lines = append(lines, "", fg(t.Muted).Render(" any other key closes"))
	return panel(t, 30, len(lines)+2, panelOpts{title: title(t, "Layers"), border: t.Accent}, lines)
}

// overlayBox centres a small box over some lines.
func (m Model) overlayBox(lines []string, w int, box []string) []string {
	bw := lipgloss.Width(box[0])
	top := max((len(lines)-len(box))/2, 0)
	left := max((w-bw)/2, 0)
	layers := []*lipgloss.Layer{lipgloss.NewLayer(strings.Join(lines, "\n")), lipgloss.NewLayer(strings.Join(box, "\n")).X(left).Y(top).Z(1)}
	return strings.Split(lipgloss.NewCompositor(layers...).Render(), "\n")
}

// geomapWidgets sizes and returns the map of each geomap widget on the open
// board page, creating them at their saved view on first sight.
func (m *Model) geomapWidgets() map[int]*mapterm.Map {
	p := m.boardPageNow()
	s := m.maps
	if p == nil || s == nil {
		return nil
	}
	g := boardLayout(p, m.width, max(m.height-headerH-footerH, 0))
	out := map[int]*mapterm.Map{}
	for _, c := range g.cells {
		wg := p.Widgets[c.index]
		if wg.Type != "geomap" {
			continue
		}
		key := fmt.Sprintf("%s/%d/%d", m.boardID, m.boardPage, c.index)
		gm := s.widgets[key]
		if gm == nil {
			center, zoom := geomapView(wg.Get("default_view"))
			gm = mapterm.New(center, zoom)
			gm.Palette, gm.Labels = s.main.Palette, true
			if m.opts.ASCII {
				gm.Mode = mapterm.ModeASCII
			}
			s.widgets[key] = gm
		}
		// The bottom row of the frame shows the credit.
		gm.SetSize(max(c.w-2, 1), max(c.h-3, 1))
		out[c.index] = gm
	}
	return out
}

// geomapView reads a widget's "lat,lon,zoom" view, as Zabbix stores it.
func geomapView(v string) (mapterm.LatLon, float64) {
	if at, zoom, err := parseCoords(v, 3); err == nil {
		return at, zoom
	}
	return mapterm.LatLon{Lat: 30}, 1
}

func (m Model) geomapLines(index int, wg model.Widget, w, h int) ([]string, bool) {
	s := m.maps
	if s == nil || !s.on {
		return nil, false
	}
	gm := s.widgets[fmt.Sprintf("%s/%d/%d", m.boardID, m.boardPage, index)]
	if gm == nil {
		return nil, false
	}
	frame, _ := gm.Render(s.source.MaxZoom(), s.loader.Version(), s.loader.Lookup)
	placed := gm.PlaceMarkers(widgetMarkers(s.markers, wg.All("hostids")))
	shown, colours := mapterm.Overlay(frame, placed, m.frame, gm.Mode == mapterm.ModeASCII)
	lines := shown.LinesWith(gm.Palette, colours)
	credit := fg(m.theme.Border).Render(truncate("© OpenStreetMap", w))
	return append(lines, credit), true
}

// mapURL opens the same place on openstreetmap.org.
func (m Model) mapURL() string {
	mm := m.maps.main
	return fmt.Sprintf("https://www.openstreetmap.org/#map=%d/%.5f/%.5f", int(mm.Zoom+0.5), mm.Center.Lat, mm.Center.Lon)
}
