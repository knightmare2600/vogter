// Package model turns raw API responses into the view-friendly data the UI
// draws. Everything here is pure: no network, no terminal, no clock of its
// own, so each rule is covered by table tests.
package model

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/knightmare2600/vogter/internal/zabbix"
)

// Item keys the overview reads, from the standard Linux agent template.
const (
	KeyCPU    = "system.cpu.util"
	KeyMem    = "vm.memory.util"
	KeyLoad   = "system.cpu.load[all,avg1]"
	KeyUptime = "system.uptime"
	KeyNCPU   = "system.cpu.num"

	PrefixNetIn  = "net.if.in["
	PrefixNetOut = "net.if.out["
	PrefixDisk   = "vfs.fs.dependent.size["
)

// OverviewKeys are fetched by exact key on every poll.
var OverviewKeys = []string{KeyCPU, KeyMem, KeyLoad, KeyUptime, KeyNCPU}

// ExtraPrefixes are the optional net/disk key prefixes.
var ExtraPrefixes = []string{PrefixNetIn, PrefixNetOut, PrefixDisk}

// Severity is a Zabbix severity, 0 to 5.
type Severity int

// Severities in ascending order.
const (
	NotClassified Severity = iota
	Information
	Warning
	Average
	High
	Disaster
)

// SeverityFromCode parses a wire value; anything unknown is NotClassified.
func SeverityFromCode(code string) Severity {
	n, err := strconv.Atoi(code)
	if err != nil || n < 1 || n > 5 {
		return NotClassified
	}
	return Severity(n)
}

// Label is the full name.
func (s Severity) Label() string {
	return [...]string{"Not classified", "Information", "Warning", "Average", "High", "Disaster"}[s]
}

// Short is the badge text.
func (s Severity) Short() string {
	return [...]string{"N/C", "INFO", "WARN", "AVG", "HIGH", "DSTR"}[s]
}

// Availability is a host's reachability.
type Availability int

// Availability states.
const (
	Unknown Availability = iota
	Up
	Down
)

// Metric is a value that may be missing. A missing metric is shown as
// unavailable, never as zero.
type Metric struct {
	Value float64
	Set   bool
}

// Some wraps a known value.
func Some(v float64) Metric { return Metric{v, true} }

// Point is a time series sample: unix seconds and value.
type Point struct{ T, V float64 }

// HostRow is one host with everything the lists show.
type HostRow struct {
	ID           string
	Name         string
	Groups       []string
	Address      string
	Enabled      bool
	Maintenance  bool
	Availability Availability
	Error        string
	CPU, Mem     Metric
	Load, CPUs   Metric
	Uptime       Metric
	NetIn        Metric
	NetOut       Metric
	Disk         Metric
	CPUHist      []Point
	MemHist      []Point
	Problems     [6]int
}

// Worst is the highest severity with a problem, or -1 when clean.
func (h *HostRow) Worst() Severity {
	for s := Disaster; s >= NotClassified; s-- {
		if h.Problems[s] > 0 {
			return s
		}
	}
	return -1
}

// ProblemCount totals the per-severity counters.
func (h *HostRow) ProblemCount() int {
	n := 0
	for _, c := range h.Problems {
		n += c
	}
	return n
}

// Problem is one active problem joined with its host.
type Problem struct {
	EventID      string
	Name         string
	Severity     Severity
	Clock        int64
	Acknowledged bool
	Suppressed   bool
	HostID       string
	Host         string
	OpData       string
	Tags         []string
	ManualClose  bool
}

// Snapshot is one complete poll.
type Snapshot struct {
	Hosts    []HostRow
	Problems []Problem
	// Truncated is set when problem.get hit its limit, so the counts are
	// lower bounds and a vanished problem does not prove it was resolved.
	Truncated bool
	FetchedAt time.Time
	Latency   time.Duration
}

// Input is the raw material of a snapshot.
type Input struct {
	Hosts    []zabbix.Host
	Problems []zabbix.Problem
	Triggers []zabbix.Trigger
	Items    []zabbix.Item
	Extra    []zabbix.Item
	Series   *Series
}

// Build joins the raw responses into a Snapshot.
func Build(in Input) Snapshot {
	triggers := make(map[string]*zabbix.Trigger, len(in.Triggers))
	for i := range in.Triggers {
		triggers[in.Triggers[i].TriggerID] = &in.Triggers[i]
	}

	problems := make([]Problem, 0, len(in.Problems))
	counts := map[string]*[6]int{}
	for _, p := range in.Problems {
		out := Problem{
			EventID:      p.EventID,
			Name:         p.Name,
			Severity:     SeverityFromCode(p.Severity),
			Acknowledged: p.Acknowledged == "1",
			Suppressed:   p.Suppressed == "1",
			Host:         "?",
			OpData:       p.OpData,
		}
		out.Clock, _ = strconv.ParseInt(p.Clock, 10, 64)
		if t := triggers[p.ObjectID]; t != nil {
			out.ManualClose = t.ManualClose == "1"
			// A trigger spanning several hosts is attributed to the first.
			if len(t.Hosts) > 0 {
				out.HostID, out.Host = t.Hosts[0].HostID, t.Hosts[0].Name
			}
		}
		for _, t := range p.Tags {
			if t.Value == "" {
				out.Tags = append(out.Tags, t.Tag)
			} else {
				out.Tags = append(out.Tags, t.Tag+":"+t.Value)
			}
		}
		problems = append(problems, out)
		if !out.Suppressed {
			if counts[out.HostID] == nil {
				counts[out.HostID] = &[6]int{}
			}
			counts[out.HostID][out.Severity]++
		}
	}
	slices.SortStableFunc(problems, func(a, b Problem) int {
		return cmp.Or(cmp.Compare(b.Severity, a.Severity), cmp.Compare(b.Clock, a.Clock))
	})

	byHost := map[string][]*zabbix.Item{}
	for _, list := range [][]zabbix.Item{in.Items, in.Extra} {
		for i := range list {
			it := &list[i]
			byHost[it.HostID] = append(byHost[it.HostID], it)
		}
	}

	hosts := make([]HostRow, 0, len(in.Hosts))
	for i := range in.Hosts {
		h := &in.Hosts[i]
		items := byHost[h.HostID]
		row := HostRow{
			ID:          h.HostID,
			Name:        h.Name,
			Enabled:     h.Status == "0",
			Maintenance: h.MaintenanceStatus == "1",
			CPU:         value(items, KeyCPU),
			Mem:         value(items, KeyMem),
			Load:        value(items, KeyLoad),
			CPUs:        value(items, KeyNCPU),
			Uptime:      value(items, KeyUptime),
			NetIn:       sumNet(items, PrefixNetIn),
			NetOut:      sumNet(items, PrefixNetOut),
			Disk:        rootDisk(items),
		}
		for _, g := range h.HostGroups {
			row.Groups = append(row.Groups, g.Name)
		}
		if c := counts[h.HostID]; c != nil {
			row.Problems = *c
		}
		iface := mainInterface(h.Interfaces)
		row.Availability = availability(iface, h.ActiveAvailable)
		if iface != nil {
			row.Address, row.Error = iface.Address(), iface.Error
		}
		if in.Series != nil {
			if it := find(items, KeyCPU); it != nil {
				row.CPUHist = in.Series.Get(it.ItemID)
			}
			if it := find(items, KeyMem); it != nil {
				row.MemHist = in.Series.Get(it.ItemID)
			}
		}
		hosts = append(hosts, row)
	}
	return Snapshot{Hosts: hosts, Problems: problems, Truncated: len(in.Problems) >= zabbix.ProblemLimit}
}

// mainInterface is the agent interface, else whichever comes first.
func mainInterface(list []zabbix.Interface) *zabbix.Interface {
	for i := range list {
		if list[i].Type == "1" {
			return &list[i]
		}
	}
	if len(list) > 0 {
		return &list[0]
	}
	return nil
}

func availability(iface *zabbix.Interface, active string) Availability {
	passive := "0"
	if iface != nil {
		passive = iface.Available
	}
	switch {
	case passive == "2" || active == "2":
		return Down
	case passive == "1" || active == "1":
		return Up
	}
	return Unknown
}

func find(items []*zabbix.Item, key string) *zabbix.Item {
	for _, it := range items {
		if it.Key == key {
			return it
		}
	}
	return nil
}

func value(items []*zabbix.Item, key string) Metric {
	if it := find(items, key); it != nil {
		if v, ok := it.Value(); ok {
			return Some(v)
		}
	}
	return Metric{}
}

// TrafficIface extracts the interface from net.if.in["eth0"] style keys.
// Keys with a second parameter and the loopback are rejected.
func TrafficIface(key, prefix string) (string, bool) {
	inner, ok := strings.CutPrefix(key, prefix)
	if !ok {
		return "", false
	}
	inner, ok = strings.CutSuffix(inner, "]")
	if !ok || strings.Contains(inner, ",") {
		return "", false
	}
	name := strings.Trim(inner, `"`)
	return name, name != "lo"
}

func sumNet(items []*zabbix.Item, prefix string) Metric {
	var out Metric
	for _, it := range items {
		if _, ok := TrafficIface(it.Key, prefix); !ok {
			continue
		}
		if v, ok := it.Value(); ok {
			out = Some(out.Value + v)
		}
	}
	return out
}

// DiskMount extracts the mount from vfs.fs.dependent.size[/,pused].
func DiskMount(key string) (string, bool) {
	inner, ok := strings.CutPrefix(key, PrefixDisk)
	if !ok {
		return "", false
	}
	return strings.CutSuffix(inner, ",pused]")
}

// rootDisk is / if monitored, else the fullest real mount. Container bind
// mounts under /etc are noise.
func rootDisk(items []*zabbix.Item) Metric {
	var best Metric
	for _, it := range items {
		m, ok := DiskMount(it.Key)
		if !ok || strings.HasPrefix(m, "/etc/") {
			continue
		}
		v, ok := it.Value()
		if !ok {
			continue
		}
		if m == "/" {
			return Some(v)
		}
		if !best.Set || v >= best.Value {
			best = Some(v)
		}
	}
	return best
}

// HostSort is a hosts list order.
type HostSort int

// Host sort modes.
const (
	SortName HostSort = iota
	SortProblems
	SortCPU
	SortMemory
)

// Label names the sort for the UI.
func (s HostSort) Label() string {
	return [...]string{"name", "problems", "cpu", "memory"}[s]
}

// Next cycles problems → cpu → memory → name.
func (s HostSort) Next() HostSort { return (s + 1) % 4 }

// FilterHosts returns hosts matching needle (name, address or group,
// case-insensitive) in the given order.
func FilterHosts(hosts []HostRow, needle string, sort HostSort) []*HostRow {
	needle = strings.ToLower(needle)
	out := make([]*HostRow, 0, len(hosts))
	for i := range hosts {
		if h := &hosts[i]; needle == "" || hostMatches(h, needle) {
			out = append(out, h)
		}
	}
	desc := func(a, b Metric) int {
		av, bv := -1.0, -1.0
		if a.Set {
			av = a.Value
		}
		if b.Set {
			bv = b.Value
		}
		return cmp.Compare(bv, av)
	}
	switch sort {
	case SortName:
		slices.SortStableFunc(out, func(a, b *HostRow) int {
			return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		})
	case SortProblems:
		slices.SortStableFunc(out, func(a, b *HostRow) int {
			return cmp.Or(cmp.Compare(b.Worst(), a.Worst()), cmp.Compare(b.ProblemCount(), a.ProblemCount()), cmp.Compare(a.Name, b.Name))
		})
	case SortCPU:
		slices.SortStableFunc(out, func(a, b *HostRow) int { return desc(a.CPU, b.CPU) })
	case SortMemory:
		slices.SortStableFunc(out, func(a, b *HostRow) int { return desc(a.Mem, b.Mem) })
	}
	return out
}

func hostMatches(h *HostRow, needle string) bool {
	if strings.Contains(strings.ToLower(h.Name), needle) || strings.Contains(strings.ToLower(h.Address), needle) {
		return true
	}
	return slices.ContainsFunc(h.Groups, func(g string) bool { return strings.Contains(strings.ToLower(g), needle) })
}

// FilterProblems returns problems at or above min, optionally without the
// acknowledged ones, matching needle (name, host or tag).
func FilterProblems(problems []Problem, needle string, min Severity, hideAcked bool) []*Problem {
	needle = strings.ToLower(needle)
	out := make([]*Problem, 0, len(problems))
	for i := range problems {
		p := &problems[i]
		if p.Severity < min || (hideAcked && p.Acknowledged) {
			continue
		}
		if needle == "" || problemMatches(p, needle) {
			out = append(out, p)
		}
	}
	return out
}

func problemMatches(p *Problem, needle string) bool {
	if strings.Contains(strings.ToLower(p.Name), needle) || strings.Contains(strings.ToLower(p.Host), needle) {
		return true
	}
	return slices.ContainsFunc(p.Tags, func(t string) bool { return strings.Contains(strings.ToLower(t), needle) })
}

// Stats are the figures on the dashboard tiles.
type Stats struct {
	Total, Monitored, Up, Down, Maintenance int
	Availability                            Metric
	Active, Unacked                         int
	Worst                                   Severity // -1 with no active problems
	Counts                                  [6]int
	AvgCPU, AvgMem                          Metric
	PeakCPU, PeakMem                        *HostRow
}

// Summarise computes the dashboard tiles. Availability counts enabled
// hosts. Unreachable hosts only have stale values, so they stay out of the
// averages. Suppressed problems are not active.
func Summarise(s *Snapshot) Stats {
	st := Stats{Total: len(s.Hosts), Worst: -1}
	var live []*HostRow
	for i := range s.Hosts {
		h := &s.Hosts[i]
		if !h.Enabled {
			continue
		}
		st.Monitored++
		switch h.Availability {
		case Up:
			st.Up++
		case Down:
			st.Down++
		}
		if h.Maintenance {
			st.Maintenance++
		}
		if h.Availability != Down {
			live = append(live, h)
		}
	}
	if st.Monitored > 0 {
		st.Availability = Some(float64(st.Up) / float64(st.Monitored) * 100)
	}
	for i := range s.Problems {
		p := &s.Problems[i]
		if p.Suppressed {
			continue
		}
		st.Active++
		st.Counts[p.Severity]++
		if !p.Acknowledged {
			st.Unacked++
		}
		st.Worst = max(st.Worst, p.Severity)
	}
	st.AvgCPU, st.PeakCPU = aggregate(live, func(h *HostRow) Metric { return h.CPU })
	st.AvgMem, st.PeakMem = aggregate(live, func(h *HostRow) Metric { return h.Mem })
	return st
}

func aggregate(hosts []*HostRow, metric func(*HostRow) Metric) (Metric, *HostRow) {
	var (
		sum  float64
		n    int
		peak *HostRow
	)
	for _, h := range hosts {
		m := metric(h)
		if !m.Set {
			continue
		}
		sum += m.Value
		n++
		if peak == nil || m.Value >= metric(peak).Value {
			peak = h
		}
	}
	if n == 0 {
		return Metric{}, nil
	}
	return Some(sum / float64(n)), peak
}

// Segment is one run of the stacked severity bar.
type Segment struct {
	Severity Severity
	Cells    int
}

// SeverityBar splits width cells between the non-zero severities, worst
// first. Each gets at least one cell and the largest absorbs the rounding.
func SeverityBar(counts [6]int, width int) []Segment {
	total := 0
	for _, c := range counts {
		total += c
	}
	if total == 0 {
		return nil
	}
	var segs []Segment
	used, big := 0, 0
	for s := Disaster; s >= NotClassified; s-- {
		if counts[s] == 0 {
			continue
		}
		seg := Segment{s, max(counts[s]*width/total, 1)}
		segs = append(segs, seg)
		used += seg.Cells
		if seg.Cells >= segs[big].Cells {
			big = len(segs) - 1
		}
	}
	segs[big].Cells = max(segs[big].Cells+width-used, 0)
	return segs
}
