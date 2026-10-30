package model

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/knightmare2600/vogter/internal/zabbix"
)

// Disk is one filesystem's usage.
type Disk struct {
	Mount string
	Used  float64
}

// Fact is a labelled host property.
type Fact struct{ Label, Value string }

// HostDetail is everything the host page shows.
type HostDetail struct {
	HostID                        string
	Range                         int64
	CPU, Mem, Load, NetIn, NetOut []Point
	Disks                         []Disk
	Facts                         []Fact
	Items                         []zabbix.Item
}

// Charted holds the items behind the host page's graphs. Any may be nil.
type Charted struct {
	CPU, Mem, Load, NetIn, NetOut *zabbix.Item
}

// All lists the charted items that exist.
func (c Charted) All() []*zabbix.Item {
	var out []*zabbix.Item
	for _, it := range []*zabbix.Item{c.CPU, c.Mem, c.Load, c.NetIn, c.NetOut} {
		if it != nil {
			out = append(out, it)
		}
	}
	return out
}

func pick(items []zabbix.Item, key string) *zabbix.Item {
	for i := range items {
		if items[i].Key == key {
			return &items[i]
		}
	}
	return nil
}

// PickCharted chooses the graphed items. Network shows the busiest
// interface, since that is the interesting one.
func PickCharted(items []zabbix.Item) Charted {
	c := Charted{CPU: pick(items, KeyCPU), Mem: pick(items, KeyMem), Load: pick(items, KeyLoad)}
	best := -1.0
	var name string
	for i := range items {
		iface, ok := TrafficIface(items[i].Key, PrefixNetIn)
		if !ok {
			continue
		}
		v, _ := items[i].Value()
		if v >= best {
			best, name, c.NetIn = v, iface, &items[i]
		}
	}
	if c.NetIn == nil {
		return c
	}
	for i := range items {
		if iface, ok := TrafficIface(items[i].Key, PrefixNetOut); ok && iface == name {
			c.NetOut = &items[i]
			break
		}
	}
	return c
}

// Disks lists filesystems by mount, skipping container bind mounts under
// /etc and those with no value.
func Disks(items []zabbix.Item) []Disk {
	var out []Disk
	for _, it := range items {
		m, ok := DiskMount(it.Key)
		if !ok || strings.HasPrefix(m, "/etc/") {
			continue
		}
		if v, ok := it.Value(); ok {
			out = append(out, Disk{m, v})
		}
	}
	slices.SortStableFunc(out, func(a, b Disk) int { return cmp.Compare(a.Mount, b.Mount) })
	return out
}

// Facts collects the host properties shown in the page head. fmtBytes and
// fmtDuration keep display formatting in the UI.
func Facts(items []zabbix.Item, fmtBytes func(float64) string, fmtDuration func(int64) string) []Fact {
	var out []Fact
	for _, f := range [][2]string{{"OS", "system.sw.os"}, {"Hostname", "system.hostname"}, {"Arch", "system.sw.arch"}, {"Agent", "agent.version"}} {
		if it := pick(items, f[1]); it != nil && it.LastValue != "" {
			out = append(out, Fact{f[0], it.LastValue})
		}
	}
	if it := pick(items, KeyNCPU); it != nil {
		if v, ok := it.Value(); ok {
			out = append(out, Fact{"CPUs", fmt.Sprintf("%.0f", v)})
		}
	}
	if it := pick(items, "vm.memory.size[total]"); it != nil {
		if v, ok := it.Value(); ok {
			out = append(out, Fact{"Memory", fmtBytes(v)})
		}
	}
	if it := pick(items, KeyUptime); it != nil {
		if v, ok := it.Value(); ok {
			out = append(out, Fact{"Uptime", fmtDuration(int64(v))})
		}
	}
	return out
}

// FilterItems returns items whose name or key contains needle.
func FilterItems(items []zabbix.Item, needle string) []*zabbix.Item {
	needle = strings.ToLower(needle)
	out := make([]*zabbix.Item, 0, len(items))
	for i := range items {
		it := &items[i]
		if needle == "" || strings.Contains(strings.ToLower(it.Name), needle) || strings.Contains(strings.ToLower(it.Key), needle) {
			out = append(out, it)
		}
	}
	return out
}

// Merge appends fresh samples, which replace any old ones at or after their
// start, and drops samples before from. Both inputs are oldest first.
func Merge(old, fresh []Point, from float64) []Point {
	if len(fresh) > 0 {
		cut, _ := slices.BinarySearchFunc(old, fresh[0].T, func(p Point, t float64) int { return cmp.Compare(p.T, t) })
		old = old[:cut]
	}
	out := append(slices.Clone(old), fresh...)
	start, _ := slices.BinarySearchFunc(out, from, func(p Point, t float64) int { return cmp.Compare(p.T, t) })
	return out[start:]
}

// ParsePoint reads a wire clock and value; ok is false if either is bad.
func ParsePoint(clock, value string) (Point, bool) {
	c, err1 := strconv.ParseFloat(clock, 64)
	v, err2 := strconv.ParseFloat(value, 64)
	return Point{c, v}, err1 == nil && err2 == nil
}
