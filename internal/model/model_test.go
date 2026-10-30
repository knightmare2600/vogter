package model

import (
	"testing"

	"github.com/knightmare2600/vogter/internal/zabbix"
)

func item(id, host, key, val string) zabbix.Item {
	return zabbix.Item{ItemID: id, HostID: host, Key: key, LastValue: val, LastClock: "100", ValueType: "0"}
}

func TestAvailability(t *testing.T) {
	cases := []struct {
		passive, active string
		want            Availability
	}{
		{"1", "0", Up}, {"0", "1", Up}, {"2", "1", Down}, {"1", "2", Down}, {"0", "0", Unknown}, {"", "", Unknown},
	}
	for _, c := range cases {
		s := Build(Input{Hosts: []zabbix.Host{{HostID: "1", ActiveAvailable: c.active, Interfaces: []zabbix.Interface{{Type: "1", Available: c.passive}}}}})
		if got := s.Hosts[0].Availability; got != c.want {
			t.Errorf("passive=%q active=%q: %v, want %v", c.passive, c.active, got, c.want)
		}
	}
}

func TestAgentInterfacePreferred(t *testing.T) {
	s := Build(Input{Hosts: []zabbix.Host{{HostID: "1", Interfaces: []zabbix.Interface{
		{Type: "2", Available: "2", IP: "10.0.0.2", UseIP: "1", Port: "161"},
		{Type: "1", Available: "1", IP: "10.0.0.1", UseIP: "1", Port: "10050", Error: ""},
	}}}})
	if h := s.Hosts[0]; h.Availability != Up || h.Address != "10.0.0.1:10050" {
		t.Errorf("%+v", h)
	}
}

func TestNoInterfaces(t *testing.T) {
	s := Build(Input{Hosts: []zabbix.Host{{HostID: "1"}}})
	if h := s.Hosts[0]; h.Availability != Unknown || h.Address != "" {
		t.Errorf("%+v", h)
	}
}

func TestMissingMetricsStayMissing(t *testing.T) {
	never := item("3", "1", KeyLoad, "0")
	never.LastClock = "0"
	s := Build(Input{
		Hosts: []zabbix.Host{{HostID: "1"}, {HostID: "2"}},
		Items: []zabbix.Item{item("1", "1", KeyCPU, "0"), never},
	})
	if h := s.Hosts[0]; !h.CPU.Set || h.CPU.Value != 0 {
		t.Errorf("a real zero must stay set: %+v", h.CPU)
	}
	if s.Hosts[0].Load.Set {
		t.Error("an item that never reported is missing, not zero")
	}
	h := s.Hosts[1]
	if h.CPU.Set || h.Mem.Set || h.Disk.Set || h.NetIn.Set {
		t.Errorf("a host without items has no metrics: %+v", h)
	}
}

func TestNetAndDisk(t *testing.T) {
	s := Build(Input{
		Hosts: []zabbix.Host{{HostID: "1"}, {HostID: "2"}, {HostID: "3"}},
		Extra: []zabbix.Item{
			item("1", "1", `net.if.in["eth0"]`, "100"),
			item("2", "1", `net.if.in["eth1"]`, "50"),
			item("3", "1", `net.if.in["lo"]`, "999"),
			item("4", "1", `net.if.in["eth0",dropped]`, "999"),
			item("5", "1", `net.if.out["eth0"]`, "7"),
			item("6", "1", "vfs.fs.dependent.size[/data,pused]", "90"),
			item("7", "1", "vfs.fs.dependent.size[/,pused]", "40"),
			item("8", "2", "vfs.fs.dependent.size[/data,pused]", "30"),
			item("9", "2", "vfs.fs.dependent.size[/var,pused]", "60"),
			item("10", "2", "vfs.fs.dependent.size[/etc/hosts,pused]", "99"),
			item("11", "3", "vfs.fs.dependent.size[/etc/resolv.conf,pused]", "99"),
		},
	})
	h := s.Hosts[0]
	if h.NetIn != Some(150) || h.NetOut != Some(7) {
		t.Errorf("net = %+v / %+v", h.NetIn, h.NetOut)
	}
	if h.Disk != Some(40) {
		t.Errorf("disk must prefer /: %+v", h.Disk)
	}
	if s.Hosts[1].Disk != Some(60) {
		t.Errorf("disk must be the fullest mount, skipping /etc: %+v", s.Hosts[1].Disk)
	}
	if s.Hosts[2].Disk.Set {
		t.Error("only /etc mounts means no disk value")
	}
}

func TestProblemJoin(t *testing.T) {
	s := Build(Input{
		Hosts: []zabbix.Host{{HostID: "1"}},
		Problems: []zabbix.Problem{
			{EventID: "1", ObjectID: "t1", Severity: "3", Clock: "100", Tags: []zabbix.Tag{{Tag: "scope", Value: "perf"}, {Tag: "bare"}}},
			{EventID: "2", ObjectID: "t1", Severity: "5", Clock: "50"},
			{EventID: "3", ObjectID: "t1", Severity: "3", Clock: "200", Suppressed: "1", Acknowledged: "1"},
			{EventID: "4", ObjectID: "gone", Severity: "9", Clock: "1"},
		},
		Triggers: []zabbix.Trigger{{TriggerID: "t1", ManualClose: "1", Hosts: []zabbix.TriggerHost{{HostID: "1", Name: "web"}, {HostID: "9", Name: "other"}}}},
	})
	var ids []string
	for _, p := range s.Problems {
		ids = append(ids, p.EventID)
	}
	if got := ids[0] + ids[1] + ids[2] + ids[3]; got != "2314" {
		t.Errorf("order = %v, want severity desc then newest", ids)
	}
	p := s.Problems[2]
	if p.Host != "web" || p.HostID != "1" || !p.ManualClose || len(p.Tags) != 2 || p.Tags[0] != "scope:perf" || p.Tags[1] != "bare" {
		t.Errorf("%+v", p)
	}
	orphan := s.Problems[3]
	if orphan.Host != "?" || orphan.Severity != NotClassified || orphan.ManualClose {
		t.Errorf("%+v", orphan)
	}
	h := s.Hosts[0]
	if h.Problems[Average] != 1 || h.Problems[Disaster] != 1 || h.ProblemCount() != 2 || h.Worst() != Disaster {
		t.Errorf("suppressed problems must not count: %+v", h.Problems)
	}
}

func TestTruncated(t *testing.T) {
	ps := make([]zabbix.Problem, zabbix.ProblemLimit)
	if !Build(Input{Problems: ps}).Truncated {
		t.Error("a full page must be flagged")
	}
	if Build(Input{Problems: ps[:10]}).Truncated {
		t.Error("a short page is the whole set")
	}
}

func names(hs []*HostRow) string {
	s := ""
	for _, h := range hs {
		s += h.Name + " "
	}
	return s
}

func TestHostSortAndFilter(t *testing.T) {
	hosts := []HostRow{
		{Name: "bravo", CPU: Some(10), Mem: Some(80), Address: "10.0.0.2:10050", Groups: []string{"Web"}},
		{Name: "Alpha", CPU: Some(90), Problems: [6]int{Warning: 3}},
		{Name: "delta", Mem: Some(10), Problems: [6]int{Disaster: 1}},
		{Name: "charlie", CPU: Some(0), Problems: [6]int{Warning: 5}},
	}
	cases := []struct {
		sort HostSort
		want string
	}{
		{SortName, "Alpha bravo charlie delta "},
		{SortProblems, "delta charlie Alpha bravo "},
		{SortCPU, "Alpha bravo charlie delta "}, // a real 0 sorts above missing
		{SortMemory, "bravo delta Alpha charlie "},
	}
	for _, c := range cases {
		if got := names(FilterHosts(hosts, "", c.sort)); got != c.want {
			t.Errorf("%s: %q, want %q", c.sort.Label(), got, c.want)
		}
	}
	for needle, want := range map[string]string{"ALPH": "Alpha ", "10.0.0.2": "bravo ", "web": "bravo ", "zzz": ""} {
		if got := names(FilterHosts(hosts, needle, SortName)); got != want {
			t.Errorf("filter %q = %q", needle, got)
		}
	}
	if s := SortProblems; s.Next() != SortCPU || s.Next().Next().Next() != SortName {
		t.Error("sort cycle wrong")
	}
}

func TestFilterProblems(t *testing.T) {
	ps := []Problem{
		{EventID: "1", Name: "High CPU", Host: "web", Severity: High},
		{EventID: "2", Name: "Disk", Host: "db", Severity: Warning, Acknowledged: true, Tags: []string{"scope:capacity"}},
		{EventID: "3", Name: "Info", Host: "lb", Severity: Information},
	}
	cases := []struct {
		needle string
		min    Severity
		hide   bool
		want   int
	}{
		{"", NotClassified, false, 3}, {"", Warning, false, 2}, {"", Warning, true, 1}, {"CAPAC", NotClassified, false, 1}, {"web", NotClassified, false, 1}, {"cpu", Disaster, false, 0},
	}
	for _, c := range cases {
		if got := FilterProblems(ps, c.needle, c.min, c.hide); len(got) != c.want {
			t.Errorf("%+v: got %d", c, len(got))
		}
	}
}

func TestSummarise(t *testing.T) {
	s := &Snapshot{
		Hosts: []HostRow{
			{Name: "a", Enabled: true, Availability: Up, CPU: Some(20), Mem: Some(50)},
			{Name: "b", Enabled: true, Availability: Up, CPU: Some(40)},
			{Name: "c", Enabled: true, Availability: Down, CPU: Some(100), Maintenance: true},
			{Name: "d", Enabled: true, Availability: Unknown},
			{Name: "e", Enabled: false, Availability: Up, CPU: Some(99)},
		},
		Problems: []Problem{
			{Severity: High}, {Severity: Warning, Acknowledged: true}, {Severity: Disaster, Suppressed: true},
		},
	}
	st := Summarise(s)
	if st.Total != 5 || st.Monitored != 4 || st.Up != 2 || st.Down != 1 || st.Maintenance != 1 {
		t.Errorf("%+v", st)
	}
	if st.Availability != Some(50) {
		t.Errorf("availability = %+v", st.Availability)
	}
	if st.AvgCPU != Some(30) || st.PeakCPU.Name != "b" || st.AvgMem != Some(50) {
		t.Errorf("down and disabled hosts stay out of the averages: %+v", st)
	}
	if st.Active != 2 || st.Unacked != 1 || st.Worst != High || st.Counts[Disaster] != 0 {
		t.Errorf("%+v", st)
	}
	empty := Summarise(&Snapshot{})
	if empty.Availability.Set || empty.AvgCPU.Set || empty.Worst != -1 || empty.PeakMem != nil {
		t.Errorf("%+v", empty)
	}
}

func TestSeverityBar(t *testing.T) {
	if SeverityBar([6]int{}, 80) != nil {
		t.Error("no problems, no segments")
	}
	for _, counts := range [][6]int{{1, 1, 1, 1, 1, 100}, {0, 0, 0, 3, 0, 0}, {7, 0, 3, 0, 1, 2}} {
		segs := SeverityBar(counts, 80)
		sum := 0
		for _, s := range segs {
			if s.Cells < 1 {
				t.Errorf("%v: empty segment %+v", counts, s)
			}
			sum += s.Cells
		}
		if sum != 80 {
			t.Errorf("%v: %d cells, want 80", counts, sum)
		}
	}
	if segs := SeverityBar([6]int{Warning: 1, Disaster: 1}, 10); segs[0].Severity != Disaster {
		t.Errorf("worst first: %+v", segs)
	}
}

func TestSeries(t *testing.T) {
	s := NewSeries()
	now := int64(10_000)
	s.Push("1", now-SparkWindow-1, 1, now)
	s.Push("1", now-100, 2, now)
	s.Push("1", now-100, 3, now) // same clock: ignored
	s.Push("1", now-200, 4, now) // older: ignored
	s.Push("1", now, 5, now)
	got := s.Get("1")
	if len(got) != 2 || got[0].V != 2 || got[1].V != 5 {
		t.Fatalf("%+v", got)
	}
	got[0].V = 99
	if s.Get("1")[0].V != 2 {
		t.Error("Get must copy")
	}
	s.MarkSeeded("1")
	s.Push("2", now, 1, now)
	s.Retain(map[string]bool{"2": true})
	if s.Seeded("1") || len(s.Get("1")) != 0 || len(s.Get("2")) != 1 {
		t.Error("Retain must drop other items")
	}
}

func TestPickCharted(t *testing.T) {
	items := []zabbix.Item{
		item("1", "1", KeyCPU, "1"),
		item("2", "1", `net.if.in["eth0"]`, "100"),
		item("3", "1", `net.if.in["eth1"]`, "900"),
		item("4", "1", `net.if.in["lo"]`, "9999"),
		item("5", "1", `net.if.out["eth0"]`, "1"),
		item("6", "1", `net.if.out["eth1"]`, "2"),
	}
	c := PickCharted(items)
	if c.CPU.ItemID != "1" || c.NetIn.ItemID != "3" || c.NetOut.ItemID != "6" || c.Mem != nil {
		t.Errorf("%+v", c)
	}
	if len(c.All()) != 3 {
		t.Errorf("%d", len(c.All()))
	}
	if PickCharted(nil).NetOut != nil {
		t.Error("no items, no charts")
	}
}

func TestDisksFactsAndItems(t *testing.T) {
	items := []zabbix.Item{
		item("1", "1", "vfs.fs.dependent.size[/var,pused]", "60"),
		item("2", "1", "vfs.fs.dependent.size[/,pused]", "40"),
		item("3", "1", "vfs.fs.dependent.size[/etc/hosts,pused]", "99"),
		item("4", "1", "system.sw.os", "Debian"),
		item("5", "1", "system.hostname", ""),
		item("6", "1", KeyNCPU, "8"),
		item("7", "1", "vm.memory.size[total]", "1024"),
		item("8", "1", KeyUptime, "60"),
	}
	d := Disks(items)
	if len(d) != 2 || d[0].Mount != "/" || d[1].Mount != "/var" {
		t.Errorf("%+v", d)
	}
	f := Facts(items, func(float64) string { return "1 KiB" }, func(int64) string { return "1m" })
	want := []Fact{{"OS", "Debian"}, {"CPUs", "8"}, {"Memory", "1 KiB"}, {"Uptime", "1m"}}
	if len(f) != len(want) {
		t.Fatalf("%+v", f)
	}
	for i := range want {
		if f[i] != want[i] {
			t.Errorf("%+v", f)
		}
	}
	if got := FilterItems(items, "OS"); len(got) != 3 { // sw.os, hostname, /etc/hosts
		t.Errorf("%d", len(got))
	}
	if got := FilterItems(items, ""); len(got) != len(items) {
		t.Errorf("%d", len(got))
	}
}

func TestMerge(t *testing.T) {
	old := []Point{{T: 10, V: 1}, {T: 20, V: 2}, {T: 30, V: 3}}
	got := Merge(old, []Point{{T: 20, V: 9}, {T: 40, V: 4}}, 15)
	want := []Point{{T: 20, V: 9}, {T: 40, V: 4}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("%+v", got)
	}
	if got := Merge(old, nil, 25); len(got) != 1 || got[0].T != 30 {
		t.Errorf("%+v", got)
	}
	if old[1].V != 2 {
		t.Error("Merge must not change its input")
	}
	if p, ok := ParsePoint("10", "1.5"); !ok || p != (Point{T: 10, V: 1.5}) {
		t.Errorf("%+v", p)
	}
	if _, ok := ParsePoint("x", "1"); ok {
		t.Error("bad clock accepted")
	}
}
