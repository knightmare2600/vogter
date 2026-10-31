package zabbix

import "context"

// Request parameters match zabterm 0.1.2 as captured in porting-audit §6.2.

// ProblemLimit caps problem.get. Callers compare the result length against
// it to tell a full page from the whole set.
const ProblemLimit = 500

var itemOutput = []string{"itemid", "hostid", "name", "key_", "lastvalue", "lastclock", "units", "value_type"}

// APIVersion calls apiinfo.version (always unauthenticated).
func (c *Client) APIVersion(ctx context.Context) (string, error) {
	var v string
	err := c.Call(ctx, "apiinfo.version", map[string]any{}, &v)
	return only(v, err)
}

// Hosts returns every host the account can see.
func (c *Client) Hosts(ctx context.Context) ([]Host, error) {
	var out hostList
	err := c.Call(ctx, "host.get", map[string]any{
		"output":           []string{"hostid", "name", "status", "maintenance_status", "active_available"},
		"selectInterfaces": []string{"ip", "dns", "useip", "port", "available", "type", "error"},
		"selectHostGroups": []string{"name"},
		"sortfield":        "name",
	}, &out)
	return only(out, err)
}

// Problems returns up to ProblemLimit active problems, newest first.
func (c *Client) Problems(ctx context.Context) ([]Problem, error) {
	var out problemList
	err := c.Call(ctx, "problem.get", map[string]any{
		"output":     []string{"eventid", "objectid", "name", "severity", "clock", "acknowledged", "suppressed", "opdata"},
		"selectTags": []string{"tag", "value"},
		"sortfield":  []string{"eventid"},
		"sortorder":  "DESC",
		"limit":      ProblemLimit,
	}, &out)
	return only(out, err)
}

// Triggers returns the given triggers with their hosts and manual_close.
func (c *Client) Triggers(ctx context.Context, triggerIDs []string) ([]Trigger, error) {
	if len(triggerIDs) == 0 {
		return nil, nil
	}
	var out triggerList
	err := c.Call(ctx, "trigger.get", map[string]any{
		"triggerids":  triggerIDs,
		"output":      []string{"triggerid", "description", "manual_close"},
		"selectHosts": []string{"hostid", "name"},
	}, &out)
	return only(out, err)
}

// ItemsByKey returns monitored items whose key matches one of keys exactly,
// on all hosts when hostIDs is nil.
func (c *Client) ItemsByKey(ctx context.Context, hostIDs []string, keys []string) ([]Item, error) {
	p := map[string]any{
		"output":    itemOutput,
		"filter":    map[string]any{"key_": keys},
		"monitored": true,
	}
	if hostIDs != nil {
		p["hostids"] = hostIDs
	}
	var out itemList
	err := c.Call(ctx, "item.get", p, &out)
	return only(out, err)
}

// ItemsByPrefix returns monitored items whose key starts with any of
// prefixes. It is the slowest call on large instances; callers give it a
// short deadline.
func (c *Client) ItemsByPrefix(ctx context.Context, hostIDs []string, prefixes []string) ([]Item, error) {
	p := map[string]any{
		"output":      itemOutput,
		"search":      map[string]any{"key_": prefixes},
		"searchByAny": true,
		"startSearch": true,
		"monitored":   true,
	}
	if hostIDs != nil {
		p["hostids"] = hostIDs
	}
	var out itemList
	err := c.Call(ctx, "item.get", p, &out)
	return only(out, err)
}

// HostItems returns every item on one host, sorted by name.
func (c *Client) HostItems(ctx context.Context, hostID string) ([]Item, error) {
	var out itemList
	err := c.Call(ctx, "item.get", map[string]any{
		"hostids":   []string{hostID},
		"output":    itemOutput,
		"sortfield": "name",
	}, &out)
	return only(out, err)
}

// ItemsByID returns the given items.
func (c *Client) ItemsByID(ctx context.Context, itemIDs []string) ([]Item, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}
	var out itemList
	err := c.Call(ctx, "item.get", map[string]any{"itemids": itemIDs, "output": itemOutput}, &out)
	return only(out, err)
}

// ItemsMatching returns monitored items on hostIDs whose name matches any of
// patterns, in which * is a wildcard, as a dashboard graph selects them.
func (c *Client) ItemsMatching(ctx context.Context, hostIDs, patterns []string) ([]Item, error) {
	var out itemList
	err := c.Call(ctx, "item.get", map[string]any{
		"hostids":                hostIDs,
		"output":                 itemOutput,
		"search":                 map[string]any{"name": patterns},
		"searchByAny":            true,
		"searchWildcardsEnabled": true,
		"monitored":              true,
		"sortfield":              "name",
	}, &out)
	return only(out, err)
}

// ItemsNamed returns monitored items named exactly one of names, on all
// hosts when hostIDs is nil.
func (c *Client) ItemsNamed(ctx context.Context, hostIDs, names []string) ([]Item, error) {
	p := map[string]any{
		"output":    itemOutput,
		"filter":    map[string]any{"name": names},
		"monitored": true,
	}
	if hostIDs != nil {
		p["hostids"] = hostIDs
	}
	var out itemList
	err := c.Call(ctx, "item.get", p, &out)
	return only(out, err)
}

// Dashboards returns the global dashboards the account can see, with their
// pages and widgets.
func (c *Client) Dashboards(ctx context.Context) ([]Dashboard, error) {
	var out dashboardList
	err := c.Call(ctx, "dashboard.get", map[string]any{
		"output":      []string{"dashboardid", "name"},
		"selectPages": "extend",
		"sortfield":   "name",
	}, &out)
	return only(out, err)
}

// Count runs a counting get on method, such as template.get.
func (c *Client) Count(ctx context.Context, method string) (string, error) {
	p := map[string]any{"countOutput": true}
	if method == "item.get" || method == "trigger.get" {
		p["templated"] = false
	}
	var n string
	err := c.Call(ctx, method, p, &n)
	return only(n, err)
}

// History returns values of one value type (history.get accepts a single
// type per call) received at or after from, oldest first.
func (c *Client) History(ctx context.Context, itemIDs []string, valueType int, from int64) ([]HistoryPoint, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}
	var out historyList
	err := c.Call(ctx, "history.get", map[string]any{
		"itemids":   itemIDs,
		"history":   valueType,
		"time_from": from,
		"sortfield": "clock",
		"sortorder": "ASC",
	}, &out)
	return only(out, err)
}

// Trends returns hourly aggregates collected at or after from. Vogter uses
// them for ranges of 24 hours and longer instead of raw history.
func (c *Client) Trends(ctx context.Context, itemIDs []string, from int64) ([]TrendPoint, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}
	var out trendList
	err := c.Call(ctx, "trend.get", map[string]any{
		"itemids":   itemIDs,
		"time_from": from,
		"output":    []string{"itemid", "clock", "num", "value_min", "value_avg", "value_max"},
	}, &out)
	return only(out, err)
}

// Acknowledge bitmask values (event.acknowledge "action").
const (
	ActionClose       = 1
	ActionAcknowledge = 2
	ActionMessage     = 4
)

// Acknowledge acknowledges events, adding message when it is not empty and
// closing them when close is set. Zabbix requires a message whenever the
// message bit is set, so the bit is only set with one.
func (c *Client) Acknowledge(ctx context.Context, eventIDs []string, message string, close bool) error {
	action := ActionAcknowledge
	if message != "" {
		action |= ActionMessage
	}
	if close {
		action |= ActionClose
	}
	return c.Call(ctx, "event.acknowledge", map[string]any{"eventids": eventIDs, "action": action, "message": message}, nil)
}

// only drops a partly decoded result when the call failed, so a failure
// can never be mistaken for data.
func only[T any](out T, err error) (T, error) {
	if err != nil {
		var zero T
		return zero, err
	}
	return out, nil
}
