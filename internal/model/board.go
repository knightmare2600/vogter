package model

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/knightmare2600/vogter/internal/zabbix"
)

// GridColumns is the width of the Zabbix dashboard grid.
const GridColumns = 72

// DefaultWidgetRange is the graph period when a widget does not set one that
// Vogter can read, and the Zabbix default.
const DefaultWidgetRange = 3600

// Board is a Zabbix dashboard.
type Board struct {
	ID, Name string
	Pages    []BoardPage
}

// BoardPage is one page of a dashboard.
type BoardPage struct {
	Name    string
	Widgets []Widget
}

// Widget is a dashboard widget, placed in grid units.
type Widget struct {
	Type, Name string
	X, Y, W, H int
	Fields     []zabbix.WidgetField
}

// Get returns a field's value, or "".
func (w Widget) Get(name string) string {
	for _, f := range w.Fields {
		if f.Name == name {
			return f.Value
		}
	}
	return ""
}

// All returns the values of a list field, "hostids" for hostids.0, hostids.1.
func (w Widget) All(name string) []string {
	var out []string
	for _, f := range w.Fields {
		if rest, ok := strings.CutPrefix(f.Name, name+"."); ok {
			if _, err := strconv.Atoi(rest); err == nil {
				out = append(out, f.Value)
			}
		}
	}
	return out
}

// Datasets is the number of ds.N datasets of a graph.
func (w Widget) Datasets() int {
	n := 0
	for _, f := range w.Fields {
		if rest, ok := strings.CutPrefix(f.Name, "ds."); ok {
			head, _, _ := strings.Cut(rest, ".")
			if i, err := strconv.Atoi(head); err == nil {
				n = max(n, i+1)
			}
		}
	}
	return n
}

// Dataset returns field name of dataset i, which is a list.
func (w Widget) Dataset(i int, name string) []string {
	return w.All(fmt.Sprintf("ds.%d.%s", i, name))
}

var widgetTitles = map[string]string{
	"clock":        "Clock",
	"geomap":       "Geomap",
	"hostavail":    "Host availability",
	"item":         "Item value",
	"map":          "Map",
	"problems":     "Problems",
	"problemsbysv": "Problems by severity",
	"svggraph":     "Graph",
	"systeminfo":   "System information",
	"tophosts":     "Top hosts",
}

// Title is the widget's name, or the frontend's default for its type.
func (w Widget) Title() string {
	if w.Name != "" {
		return w.Name
	}
	if t, ok := widgetTitles[w.Type]; ok {
		return t
	}
	return w.Type
}

// BoardsFrom converts dashboard.get rows. A widget without a usable size gets
// the smallest one, so bad data cannot hide it or divide by zero.
func BoardsFrom(list []zabbix.Dashboard) []Board {
	num := func(s string, least int) int {
		n, err := strconv.Atoi(s)
		if err != nil {
			return least
		}
		return max(n, least)
	}
	out := make([]Board, 0, len(list))
	for _, d := range list {
		b := Board{ID: d.DashboardID, Name: d.Name}
		for _, p := range d.Pages {
			page := BoardPage{Name: p.Name}
			for _, w := range p.Widgets {
				page.Widgets = append(page.Widgets, Widget{
					Type: w.Type, Name: w.Name, Fields: w.Fields,
					X: num(w.X, 0), Y: num(w.Y, 0), W: num(w.Width, 1), H: num(w.Height, 1),
				})
			}
			b.Pages = append(b.Pages, page)
		}
		out = append(out, b)
	}
	return out
}

// Widgets is the number of widgets on all pages.
func (b Board) Widgets() int {
	n := 0
	for _, p := range b.Pages {
		n += len(p.Widgets)
	}
	return n
}

// GridRows is the height of a page in grid units.
func (p BoardPage) GridRows() int {
	n := 1
	for _, w := range p.Widgets {
		n = max(n, w.Y+w.H)
	}
	return n
}

// ParseRange reads a "now-1h" period as seconds. Any other form, such as an
// absolute time or a rounded "now/d", gives DefaultWidgetRange.
func ParseRange(s string) int64 {
	rest, ok := strings.CutPrefix(s, "now-")
	if !ok || rest == "" {
		return DefaultWidgetRange
	}
	unit := map[byte]int64{'s': 1, 'm': 60, 'h': 3600, 'd': 86400, 'w': 7 * 86400, 'M': 30 * 86400, 'y': 365 * 86400}[rest[len(rest)-1]]
	n, err := strconv.ParseInt(rest[:len(rest)-1], 10, 64)
	if unit == 0 || err != nil || n <= 0 {
		return DefaultWidgetRange
	}
	return n * unit
}

// Glob matches name against a pattern in which * is a wildcard, ignoring
// case, as Zabbix matches a graph's host patterns.
func Glob(pattern, name string) bool {
	pattern, name = strings.ToLower(pattern), strings.ToLower(name)
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == name
	}
	if !strings.HasPrefix(name, parts[0]) {
		return false
	}
	name = name[len(parts[0]):]
	last := parts[len(parts)-1]
	for _, p := range parts[1 : len(parts)-1] {
		i := strings.Index(name, p)
		if i < 0 {
			return false
		}
		name = name[i+len(p):]
	}
	return strings.HasSuffix(name, last)
}

// WidgetProblems applies a widget's host and severity filters.
func WidgetProblems(w Widget, problems []Problem) []*Problem {
	hosts, sevs := w.All("hostids"), w.All("severities")
	var out []*Problem
	for i := range problems {
		p := &problems[i]
		if len(hosts) > 0 && !slices.Contains(hosts, p.HostID) {
			continue
		}
		if len(sevs) > 0 && !slices.Contains(sevs, strconv.Itoa(int(p.Severity))) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// NamedSeries is one line of a dashboard graph.
type NamedSeries struct {
	Name, Units string
	Points      []Point
}

// AvailCount counts a widget's host interfaces by state.
type AvailCount struct{ Up, Down, Unknown int }

// CountAvail counts the interfaces of the given types of enabled hosts, or of
// every type when types is empty.
func CountAvail(hosts []zabbix.Host, types []string) AvailCount {
	var c AvailCount
	for _, h := range hosts {
		if h.Status != "0" {
			continue
		}
		for _, i := range h.Interfaces {
			if len(types) > 0 && !slices.Contains(types, i.Type) {
				continue
			}
			switch i.Available {
			case "1":
				c.Up++
			case "2":
				c.Down++
			default:
				c.Unknown++
			}
		}
	}
	return c
}

// TopColumn is a column of a top hosts widget.
type TopColumn struct {
	Name, Item string
	// Host marks the column that shows the host name.
	Host bool
}

// TopRow is a host in a top hosts widget, with one cell per column.
type TopRow struct {
	Host  string
	Cells []*zabbix.Item
}

// Top is the table of a top hosts widget.
type Top struct {
	Columns []TopColumn
	Rows    []TopRow
}

// TopColumns reads a widget's columns. Column data 2 is the host name.
func TopColumns(w Widget) []TopColumn {
	var out []TopColumn
	for i := 0; ; i++ {
		name, ok := "", false
		for _, f := range w.Fields {
			if f.Name == fmt.Sprintf("columns.%d.name", i) {
				name, ok = f.Value, true
			}
		}
		if !ok {
			return out
		}
		out = append(out, TopColumn{
			Name: name,
			Item: w.Get(fmt.Sprintf("columns.%d.item", i)),
			Host: w.Get(fmt.Sprintf("columns.%d.data", i)) == "2",
		})
	}
}

// BuildTop lays items out by host and sorts by the widget's chosen column,
// highest first, keeping at most limit rows.
func BuildTop(columns []TopColumn, sortBy int, hosts map[string]string, items []zabbix.Item, limit int) Top {
	rows := map[string]*TopRow{}
	for i := range items {
		it := &items[i]
		name, ok := hosts[it.HostID]
		if !ok {
			continue
		}
		r := rows[it.HostID]
		if r == nil {
			r = &TopRow{Host: name, Cells: make([]*zabbix.Item, len(columns))}
			rows[it.HostID] = r
		}
		for c, col := range columns {
			if !col.Host && col.Item == it.Name {
				r.Cells[c] = it
			}
		}
	}
	top := Top{Columns: columns}
	for _, r := range rows {
		top.Rows = append(top.Rows, *r)
	}
	value := func(r TopRow) float64 {
		if sortBy >= len(r.Cells) || r.Cells[sortBy] == nil {
			return -1
		}
		v, _ := r.Cells[sortBy].Value()
		return v
	}
	slices.SortFunc(top.Rows, func(a, b TopRow) int {
		return cmp.Or(cmp.Compare(value(b), value(a)), cmp.Compare(a.Host, b.Host))
	})
	top.Rows = top.Rows[:min(len(top.Rows), limit)]
	return top
}

// WidgetData is what one widget shows. A widget that only needs the snapshot
// or the clock has none.
type WidgetData struct {
	Series []NamedSeries
	// Dropped counts graph series left out to keep the chart readable.
	Dropped int
	Range   int64
	Items   []zabbix.Item
	Top     *Top
	Avail   *AvailCount
	Info    [][2]string
	// Err is this widget's own failure; the others still draw.
	Err string
}

// BoardData is one fetch of a dashboard page, aligned with its widgets.
type BoardData struct {
	BoardID string
	Page    int
	Widgets []WidgetData
}
