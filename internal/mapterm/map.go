package mapterm

import (
	"math"

	"github.com/knightmare2600/vogter/internal/mapterm/mvt"
)

// Layer is a group of features that can be shown or hidden.
type Layer int

// Toggleable layers.
const (
	LayerBuildings Layer = iota
	LayerRailways
	LayerParks
	LayerWater
	LayerBoundaries
)

// AllLayers in toggle order.
var AllLayers = []Layer{LayerBuildings, LayerRailways, LayerParks, LayerWater, LayerBoundaries}

func (l Layer) String() string {
	return [...]string{"buildings", "railways", "parks", "water", "boundaries"}[l]
}

var layerOf = map[Class]Layer{
	ClassBuilding: LayerBuildings, ClassRail: LayerRailways, ClassPark: LayerParks,
	ClassWater: LayerWater, ClassWaterway: LayerWater, ClassBoundary: LayerBoundaries,
}

// place is a position and zoom, for the back history.
type place struct {
	center LatLon
	zoom   float64
}

// historyDepth bounds the back history.
const historyDepth = 32

// Map is the state of one interactive map: where it looks, how it draws, and
// what it showed last, so an unchanged view is not drawn twice.
type Map struct {
	Cols, Rows int
	Center     LatLon
	Zoom       float64
	Mode       Mode
	Labels     bool
	Palette    Palette
	Style      Style
	// Hidden layers are not drawn.
	Hidden map[Layer]bool

	history []place

	// The last frame and what it was drawn from.
	frame    *Frame
	missing  []TileID
	drawnFor frameKey
}

type frameKey struct {
	cols, rows int
	center     LatLon
	zoom       float64
	mode       Mode
	labels     bool
	hidden     [8]bool
	tiles      int
	maxZoom    int
}

// New makes a map at a position.
func New(center LatLon, zoom float64) *Map {
	return &Map{Center: center, Zoom: clampZoom(zoom), Labels: true, Palette: Palettes[0], Style: DefaultStyle, Hidden: map[Layer]bool{}}
}

func clampZoom(z float64) float64 {
	if math.IsNaN(z) {
		return MinZoom
	}
	return math.Max(MinZoom, math.Min(MaxZoom, z))
}

// SetSize sets the map's size in terminal cells.
func (m *Map) SetSize(cols, rows int) { m.Cols, m.Rows = max(cols, 0), max(rows, 0) }

// View is the current view in dots.
func (m *Map) View() View {
	return View{Center: m.Center, Zoom: m.Zoom, W: m.Cols * CellW, H: m.Rows * CellH}
}

// Pan moves by a fraction of the screen: 1 is a whole width or height.
func (m *Map) Pan(dx, dy float64) {
	v := m.View()
	w := v.FromDot(float64(v.W)/2+dx*float64(v.W), float64(v.H)/2+dy*float64(v.H))
	w.Y = math.Max(0, math.Min(1, w.Y))
	m.Center = Unproject(w)
}

// ZoomBy changes zoom by steps, keeping the centre.
func (m *Map) ZoomBy(steps float64) { m.Zoom = clampZoom(m.Zoom + steps) }

// GoTo jumps to a position, remembering the current one for Back. An invalid
// position is ignored.
func (m *Map) GoTo(center LatLon, zoom float64) bool {
	if !center.Valid() {
		return false
	}
	m.remember()
	m.Center, m.Zoom = center, clampZoom(zoom)
	return true
}

func (m *Map) remember() {
	m.history = append(m.history, place{m.Center, m.Zoom})
	if len(m.history) > historyDepth {
		m.history = m.history[1:]
	}
}

// Back returns to the position before the last jump.
func (m *Map) Back() bool {
	if len(m.history) == 0 {
		return false
	}
	p := m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m.Center, m.Zoom = p.center, p.zoom
	return true
}

// Fit centres on the given positions and zooms in as far as shows them all,
// with a margin, never past maxZoom. Invalid positions are skipped. It
// reports false when none is valid.
func (m *Map) Fit(points []LatLon, maxZoom float64) bool {
	var ws []World
	for _, p := range points {
		if p.Valid() {
			ws = append(ws, Project(p))
		}
	}
	if len(ws) == 0 || m.Cols == 0 || m.Rows == 0 {
		return false
	}
	// The narrowest span in x, allowing for the date line: try each point as
	// the western edge.
	bestSpan, west := 2.0, 0.0
	for _, a := range ws {
		span := 0.0
		for _, b := range ws {
			span = math.Max(span, math.Mod(b.X-a.X+1, 1))
		}
		if span < bestSpan {
			bestSpan, west = span, a.X
		}
	}
	minY, maxY := 1.0, 0.0
	for _, w := range ws {
		minY, maxY = math.Min(minY, w.Y), math.Max(maxY, w.Y)
	}
	const margin = 0.8
	zoom := maxZoom
	if bestSpan > 0 || maxY > minY {
		zx := math.Log2(margin * float64(m.Cols*CellW) / TileDots / math.Max(bestSpan, 1e-12))
		zy := math.Log2(margin * float64(m.Rows*CellH) / TileDots / math.Max(maxY-minY, 1e-12))
		zoom = math.Min(maxZoom, math.Floor(math.Min(zx, zy)*4)/4)
	}
	center := Unproject(World{west + bestSpan/2, (minY + maxY) / 2})
	return m.GoTo(center, zoom)
}

// ToggleLayer shows or hides a layer.
func (m *Map) ToggleLayer(l Layer) { m.Hidden[l] = !m.Hidden[l] }

// NextMode cycles the drawing mode.
func (m *Map) NextMode() { m.Mode = Modes[(int(m.Mode)+1)%len(Modes)] }

func (m *Map) key(tiles, maxZoom int) frameKey {
	k := frameKey{m.Cols, m.Rows, m.Center, m.Zoom, m.Mode, m.Labels, [8]bool{}, tiles, maxZoom}
	for l, h := range m.Hidden {
		if int(l) < len(k.hidden) {
			k.hidden[l] = h
		}
	}
	return k
}

// Render draws the map from tiles lookup already has. tiles counts the tiles
// the caller holds; while it, the view and the settings are unchanged, the
// previous frame is returned without drawing again. It also returns the tiles
// still missing, nearest the centre first.
func (m *Map) Render(maxZoom, tiles int, lookup Lookup) (*Frame, []TileID) {
	k := m.key(tiles, maxZoom)
	if m.frame != nil && k == m.drawnFor {
		return m.frame, m.missing
	}
	v := m.View()
	c := NewCanvas(m.Cols, m.Rows)
	style := m.Style
	inner := style.Match
	style.Match = func(layer string, f *mvt.Feature, zoom float64) (Rule, bool) {
		r, ok := inner(layer, f, zoom)
		if ok {
			if l, has := layerOf[r.Class]; has && m.Hidden[l] {
				return Rule{}, false
			}
		}
		return r, ok
	}
	missing := Draw(c, v, maxZoom, lookup, style)
	f := Compose(c, m.Mode)
	if m.Labels {
		PlaceLabels(f, CollectLabels(v, maxZoom, lookup), m.Mode == ModeASCII)
	}
	m.frame, m.missing, m.drawnFor = f, missing, k
	return f, missing
}

// Locate gives the cell a position falls in, and whether it is on screen.
func (m *Map) Locate(p LatLon) (col, row int, ok bool) {
	if !p.Valid() {
		return 0, 0, false
	}
	x, y := m.View().ToDot(Project(p))
	col, row = int(math.Floor(x/CellW)), int(math.Floor(y/CellH))
	return col, row, col >= 0 && row >= 0 && col < m.Cols && row < m.Rows
}
