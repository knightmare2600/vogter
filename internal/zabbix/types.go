package zabbix

import (
	"fmt"
	"strconv"
)

// Zabbix returns nearly every number as a string, so wire types keep the
// strings and parse them where they are used.

// Host is a host.get result row.
type Host struct {
	HostID            string      `json:"hostid"`
	Name              string      `json:"name"`
	Status            string      `json:"status"`
	MaintenanceStatus string      `json:"maintenance_status"`
	ActiveAvailable   string      `json:"active_available"`
	Interfaces        []Interface `json:"interfaces"`
	HostGroups        []Named     `json:"hostgroups"`
}

// Interface is a host interface.
type Interface struct {
	IP        string `json:"ip"`
	DNS       string `json:"dns"`
	UseIP     string `json:"useip"`
	Port      string `json:"port"`
	Available string `json:"available"`
	Type      string `json:"type"`
	Error     string `json:"error"`
}

// Address is ip:port or dns:port, whichever the interface uses.
func (i Interface) Address() string {
	a := i.DNS
	if i.UseIP == "1" {
		a = i.IP
	}
	return a + ":" + i.Port
}

// Named is any object with only a name selected.
type Named struct {
	Name string `json:"name"`
}

// Problem is a problem.get result row.
type Problem struct {
	EventID      string `json:"eventid"`
	ObjectID     string `json:"objectid"`
	Name         string `json:"name"`
	Severity     string `json:"severity"`
	Clock        string `json:"clock"`
	Acknowledged string `json:"acknowledged"`
	Suppressed   string `json:"suppressed"`
	OpData       string `json:"opdata"`
	Tags         []Tag  `json:"tags"`
}

// Tag is a problem tag.
type Tag struct {
	Tag   string `json:"tag"`
	Value string `json:"value"`
}

// Trigger is a trigger.get result row.
type Trigger struct {
	TriggerID   string        `json:"triggerid"`
	ManualClose string        `json:"manual_close"`
	Hosts       []TriggerHost `json:"hosts"`
}

// TriggerHost is a host selected through trigger.get.
type TriggerHost struct {
	HostID string `json:"hostid"`
	Name   string `json:"name"`
}

// Item is an item.get result row.
type Item struct {
	ItemID    string `json:"itemid"`
	HostID    string `json:"hostid"`
	Name      string `json:"name"`
	Key       string `json:"key_"`
	LastValue string `json:"lastvalue"`
	LastClock string `json:"lastclock"`
	Units     string `json:"units"`
	ValueType string `json:"value_type"`
}

// Value is the last value as a number. ok is false when the item has never
// reported or is not numeric, so a missing metric is never shown as zero.
func (i Item) Value() (v float64, ok bool) {
	if i.LastClock == "0" || i.LastClock == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(i.LastValue, 64)
	return v, err == nil
}

// Numeric reports whether the item stores float (0) or unsigned (3) values.
func (i Item) Numeric() bool { return i.ValueType == "0" || i.ValueType == "3" }

// HistoryPoint is a history.get result row.
type HistoryPoint struct {
	ItemID string `json:"itemid"`
	Clock  string `json:"clock"`
	Value  string `json:"value"`
}

// TrendPoint is a trend.get result row (hourly aggregates).
type TrendPoint struct {
	ItemID   string `json:"itemid"`
	Clock    string `json:"clock"`
	Num      string `json:"num"`
	ValueMin string `json:"value_min"`
	ValueAvg string `json:"value_avg"`
	ValueMax string `json:"value_max"`
}

// Dashboard is a dashboard.get result row with its pages selected.
type Dashboard struct {
	DashboardID string          `json:"dashboardid"`
	Name        string          `json:"name"`
	Pages       []DashboardPage `json:"pages"`
}

// DashboardPage is one page of a dashboard.
type DashboardPage struct {
	Name    string   `json:"name"`
	Widgets []Widget `json:"widgets"`
}

// Widget sits on a grid 72 columns wide.
type Widget struct {
	Type   string        `json:"type"`
	Name   string        `json:"name"`
	X      string        `json:"x"`
	Y      string        `json:"y"`
	Width  string        `json:"width"`
	Height string        `json:"height"`
	Fields []WidgetField `json:"fields"`
}

// WidgetField is one setting of a widget.
type WidgetField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Result slices check their required fields after decoding.

type hostList []Host
type problemList []Problem
type triggerList []Trigger
type itemList []Item
type historyList []HistoryPoint
type trendList []TrendPoint
type dashboardList []Dashboard

func required(what string, i int, fields ...string) error {
	for _, f := range fields {
		if f == "" {
			return fmt.Errorf("%s %d is missing a required field", what, i)
		}
	}
	return nil
}

func (l hostList) validate() error {
	for i, h := range l {
		if err := required("host", i, h.HostID, h.Status); err != nil {
			return err
		}
	}
	return nil
}

func (l problemList) validate() error {
	for i, p := range l {
		if err := required("problem", i, p.EventID, p.ObjectID, p.Severity, p.Clock); err != nil {
			return err
		}
	}
	return nil
}

func (l triggerList) validate() error {
	for i, t := range l {
		if err := required("trigger", i, t.TriggerID); err != nil {
			return err
		}
	}
	return nil
}

func (l itemList) validate() error {
	for i, it := range l {
		if err := required("item", i, it.ItemID, it.HostID, it.Key, it.ValueType); err != nil {
			return err
		}
	}
	return nil
}

func (l historyList) validate() error {
	for i, p := range l {
		if err := required("history point", i, p.ItemID, p.Clock); err != nil {
			return err
		}
	}
	return nil
}

func (l trendList) validate() error {
	for i, p := range l {
		if err := required("trend point", i, p.ItemID, p.Clock); err != nil {
			return err
		}
	}
	return nil
}

func (l dashboardList) validate() error {
	for i, d := range l {
		if err := required("dashboard", i, d.DashboardID); err != nil {
			return err
		}
	}
	return nil
}
