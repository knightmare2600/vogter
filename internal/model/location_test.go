package model

import (
	"encoding/json"
	"testing"

	"github.com/knightmare2600/vogter/internal/zabbix"
)

func TestParseCoordinate(t *testing.T) {
	good := map[string]float64{
		"55.39594": 55.39594, " 55,6761 ": 55.6761, "-33.86": -33.86, "33.86S": -33.86, "55.4 N": 55.4, "55.4°": 55.4,
	}
	for in, want := range good {
		if got, ok := ParseCoordinate(in, true); !ok || got != want {
			t.Errorf("%q: %v %v", in, got, ok)
		}
	}
	if got, ok := ParseCoordinate("10.38W", false); !ok || got != -10.38 {
		t.Errorf("west: %v", got)
	}
	for _, bad := range []string{"", "north", "91", "1,234.5", "10E", "NaN", "Inf"} {
		if _, ok := ParseCoordinate(bad, true); ok {
			t.Errorf("%q must be refused as a latitude", bad)
		}
	}
	if _, ok := ParseCoordinate("181", false); ok {
		t.Error("longitude range")
	}
}

// The inventory shapes are exactly those Zabbix 7.0.32 returned in the lab.
const inventoryJSON = `[
 {"hostid":"10683","name":"web-01","inventory":{"location_lat":"55.6761","location_lon":"12.5683","location":"","site_city":"København","site_country":""}},
 {"hostid":"10684","name":"web-02","inventory":[]},
 {"hostid":"10685","name":"db-01","inventory":{"location_lat":"55,6761","location_lon":"12,5683","location":"","site_city":"","site_country":""}},
 {"hostid":"10686","name":"cache-01","inventory":{"location_lat":"","location_lon":"","location":"","site_city":"Aarhus","site_country":""}},
 {"hostid":"10084","name":"Zabbix server","inventory":{"location_lat":"55.39594","location_lon":"10.38621","location":"Server room","site_city":"Odense","site_country":"Denmark"}}
]`

func TestLocate(t *testing.T) {
	var inv []zabbix.HostInventory
	if err := json.Unmarshal([]byte(inventoryJSON), &inv); err != nil {
		t.Fatalf("a disabled inventory's [] must decode: %v", err)
	}
	locs := Locate(inv, map[string][2]float64{"web-02": {56.15, 10.2}, "10685": {57.05, 9.92}})
	if locs.Total != 5 || locs.CityOnly != 1 || len(locs.Hosts) != 4 {
		t.Fatalf("%+v", locs)
	}
	by := map[string]HostLocation{}
	for _, h := range locs.Hosts {
		by[h.Name] = h
	}
	if h := by["Zabbix server"]; h.Lat != 55.39594 || h.Place != "Server room, Odense, Denmark" || h.FromConfig {
		t.Errorf("%+v", h)
	}
	if h := by["web-02"]; !h.FromConfig || h.Lat != 56.15 {
		t.Errorf("config by name places a host with no inventory: %+v", h)
	}
	if h := by["db-01"]; !h.FromConfig || h.Lat != 57.05 {
		t.Errorf("config by ID wins over inventory: %+v", h)
	}
	if locs.Hosts[0].Name != "Zabbix server" && locs.Hosts[0].Name > locs.Hosts[1].Name {
		t.Error("sorted by name")
	}
}
