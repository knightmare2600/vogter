// Command mockzbx is a fixture Zabbix JSON-RPC endpoint for development and
// for comparing Vægter with zabterm. It logs every request as JSON lines,
// with the credential redacted. Response shapes match porting-audit §6.4.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var mu sync.Mutex

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "listen address")
	logPath := flag.String("log", "", "request log file (JSON lines)")
	flip := flag.Duration("flip", 0, "after this long, a new problem appears, one resolves and web-01 goes down")
	flag.Parse()
	started := time.Now()
	flipped := func() bool { return *flip > 0 && time.Since(started) > *flip }
	var out io.Writer = io.Discard
	if *logPath != "" {
		f, err := os.Create(*logPath)
		if err != nil {
			log.Fatal(err)
		}
		out = f
	}
	http.HandleFunc("/api_jsonrpc.php", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			ID     any             `json:"id"`
		}
		_ = json.Unmarshal(body, &req)
		authz := r.Header.Get("Authorization")
		scheme, _, _ := strings.Cut(authz, " ")
		line, _ := json.Marshal(map[string]any{
			"t":             time.Now().Format(time.RFC3339Nano),
			"method":        req.Method,
			"authorization": scheme,
			"user_agent":    r.Header.Get("User-Agent"),
			"params":        req.Params,
		})
		mu.Lock()
		fmt.Fprintln(out, string(line))
		mu.Unlock()

		id, _ := json.Marshal(req.ID)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(authz, "badtoken") {
			fmt.Fprintf(w, `{"jsonrpc":"2.0","error":{"code":-32602,"message":"Invalid params.","data":"Not authorized."},"id":%s}`, id)
			return
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","result":%s,"id":%s}`, result(req.Method, req.Params, flipped()), id)
	})
	log.Fatal(http.ListenAndServe(*addr, nil))
}

const hosts = `[
 {"hostid":"10084","name":"web-01","status":"0","maintenance_status":"0","active_available":"1",
  "interfaces":[{"ip":"10.0.0.11","dns":"","useip":"1","port":"10050","available":"1","type":"1","error":""}],
  "hostgroups":[{"name":"Linux servers"},{"name":"Web"}]},
 {"hostid":"10085","name":"db-01","status":"0","maintenance_status":"0","active_available":"0",
  "interfaces":[{"ip":"10.0.0.21","dns":"db-01.lan","useip":"0","port":"10050","available":"2","type":"1","error":"Get value from agent failed: cannot connect"}],
  "hostgroups":[{"name":"Linux servers"}]},
 {"hostid":"10086","name":"lb-01","status":"0","maintenance_status":"1","active_available":"1",
  "interfaces":[{"ip":"10.0.0.5","dns":"","useip":"1","port":"10050","available":"1","type":"1","error":""}],
  "hostgroups":[{"name":"Network"}]},
 {"hostid":"10087","name":"switch-01","status":"1","maintenance_status":"0","active_available":"0",
  "interfaces":[{"ip":"10.0.0.2","dns":"","useip":"1","port":"161","available":"0","type":"2","error":""}],
  "hostgroups":[{"name":"Network"}]}
]`

const triggers = `[
 {"triggerid":"9001","description":"High CPU","manual_close":"1","hosts":[{"hostid":"10084","name":"web-01"}]},
 {"triggerid":"9002","description":"Agent","manual_close":"0","hosts":[{"hostid":"10085","name":"db-01"}]},
 {"triggerid":"9003","description":"Backup","manual_close":"0","hosts":[{"hostid":"10086","name":"lb-01"}]}
]`

func item(id, host, name, key, value, units, vtype string, clock int64) string {
	return fmt.Sprintf(`{"itemid":%q,"hostid":%q,"name":%q,"key_":%q,"lastvalue":%q,"lastclock":"%d","units":%q,"value_type":%q}`,
		id, host, name, key, value, clock, units, vtype)
}

func result(method string, params json.RawMessage, flipped bool) string {
	now := time.Now().Unix()
	switch method {
	case "apiinfo.version":
		return `"7.0.12"`
	case "user.login":
		return `"0123456789abcdef0123456789abcdef"`
	case "user.logout":
		return `true`
	case "host.get":
		if flipped {
			return strings.Replace(strings.Replace(hosts, `"available":"1","type":"1","error":""`, `"available":"2","type":"1","error":"Connection refused"`, 1), `"active_available":"1"`, `"active_available":"0"`, 1)
		}
		return hosts
	case "trigger.get":
		return triggers
	case "event.acknowledge":
		return `{"eventids":["501"]}`
	case "problem.get":
		if flipped {
			return fmt.Sprintf(`[
 {"eventid":"504","objectid":"9001","name":"Disk full","severity":"5","clock":"%d","acknowledged":"0","suppressed":"0","opdata":"","tags":[]},
 {"eventid":"501","objectid":"9001","name":"High CPU utilisation","severity":"3","clock":"%d","acknowledged":"1","suppressed":"0","opdata":"","tags":[]}
]`, now-5, now-600)
		}
		return fmt.Sprintf(`[
 {"eventid":"503","objectid":"9003","name":"Backup is late","severity":"2","clock":"%d","acknowledged":"0","suppressed":"1","opdata":"","tags":[]},
 {"eventid":"502","objectid":"9002","name":"Zabbix agent is not available","severity":"4","clock":"%d","acknowledged":"0","suppressed":"0","opdata":"","tags":[]},
 {"eventid":"501","objectid":"9001","name":"High CPU utilisation","severity":"3","clock":"%d","acknowledged":"1","suppressed":"0","opdata":"Current: 93%%","tags":[{"tag":"scope","value":"performance"},{"tag":"bare","value":""}]}
]`, now-90000, now-120, now-600)
	case "item.get":
		var p struct {
			Search  *struct{} `json:"search"`
			HostIDs []string  `json:"hostids"`
		}
		_ = json.Unmarshal(params, &p)
		if len(p.HostIDs) == 1 {
			return "[" + strings.Join([]string{
				item("1001", "10084", "CPU utilization", "system.cpu.util", "42.5", "%", "0", now),
				item("1002", "10084", "Memory utilization", "vm.memory.util", "61.2", "%", "0", now),
				item("1003", "10084", "Load", "system.cpu.load[all,avg1]", "1.25", "", "0", now),
				item("1004", "10084", "Uptime", "system.uptime", "93784", "uptime", "3", now),
				item("1005", "10084", "CPUs", "system.cpu.num", "8", "", "3", now),
				item("1006", "10084", "OS", "system.sw.os", "Debian 13", "", "1", now),
				item("1007", "10084", "Total memory", "vm.memory.size[total]", "8393289728", "B", "3", now),
				item("2001", "10084", "eth0 in", `net.if.in[\"eth0\"]`, "12000", "bps", "3", now),
				item("2002", "10084", "eth0 out", `net.if.out[\"eth0\"]`, "800", "bps", "3", now),
				item("2003", "10084", "/: used", "vfs.fs.dependent.size[/,pused]", "71.5", "%", "0", now),
			}, ",") + "]"
		}
		if p.Search != nil {
			return "[" + strings.Join([]string{
				item("2001", "10084", "eth0 in", `net.if.in["eth0"]`, "12000", "bps", "3", now),
				item("2002", "10084", "eth0 out", `net.if.out["eth0"]`, "800", "bps", "3", now),
				item("2003", "10084", "/: used", "vfs.fs.dependent.size[/,pused]", "71.5", "%", "0", now),
				item("2004", "10086", "/: used", "vfs.fs.dependent.size[/,pused]", "93", "%", "0", now),
			}, ",") + "]"
		}
		return "[" + strings.Join([]string{
			item("1001", "10084", "CPU utilization", "system.cpu.util", "42.5", "%", "0", now),
			item("1002", "10084", "Memory utilization", "vm.memory.util", "61.2", "%", "0", now),
			item("1003", "10084", "Load", "system.cpu.load[all,avg1]", "1.25", "", "0", now),
			item("1004", "10084", "Uptime", "system.uptime", "93784", "uptime", "3", now),
			item("1005", "10084", "CPUs", "system.cpu.num", "8", "", "3", now),
			item("1011", "10085", "CPU utilization", "system.cpu.util", "97", "%", "0", now-3000),
			item("1012", "10085", "Memory utilization", "vm.memory.util", "55", "%", "0", now-3000),
			item("1021", "10086", "CPU utilization", "system.cpu.util", "75", "%", "0", now),
			item("1022", "10086", "Memory utilization", "vm.memory.util", "30", "%", "0", now),
		}, ",") + "]"
	case "history.get":
		var p struct {
			ItemIDs  []string `json:"itemids"`
			TimeFrom int64    `json:"time_from"`
		}
		_ = json.Unmarshal(params, &p)
		var rows []string
		// One sample a minute on the minute, so re-reads see the same data.
		for i, id := range p.ItemIDs {
			for c := (now - 1740) / 60 * 60; c <= now; c += 60 {
				if c >= p.TimeFrom {
					rows = append(rows, fmt.Sprintf(`{"itemid":%q,"clock":"%d","value":"%d","ns":"0"}`, id, c, 20+(c/60*7+int64(i)*13)%50))
				}
			}
		}
		return "[" + strings.Join(rows, ",") + "]"
	case "trend.get":
		var p struct {
			ItemIDs  []string `json:"itemids"`
			TimeFrom int64    `json:"time_from"`
		}
		_ = json.Unmarshal(params, &p)
		var rows []string
		for i, id := range p.ItemIDs {
			for c := (now-7*86400)/3600*3600 + 3600; c < now-3600; c += 3600 {
				if c >= p.TimeFrom {
					rows = append(rows, fmt.Sprintf(`{"itemid":%q,"clock":"%d","num":"60","value_min":"1","value_avg":"%d","value_max":"90"}`, id, c, 20+(c/3600*7+int64(i)*13)%50))
				}
			}
		}
		return "[" + strings.Join(rows, ",") + "]"
	}
	return `[]`
}
