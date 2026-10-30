// Package integration runs the real client, poller and model together
// against a fixture server, and the real UI model on what comes out.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/knightmare2600/vogter/internal/config"
	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/poller"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

type server struct {
	mu    sync.Mutex
	calls map[string]int
}

func (s *server) count(m string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[m]
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	s.mu.Lock()
	s.calls[req.Method]++
	s.mu.Unlock()
	now := time.Now().Unix()
	item := func(id, key, val, vt string) string {
		return fmt.Sprintf(`{"itemid":%q,"hostid":"1","name":%q,"key_":%q,"lastvalue":%q,"lastclock":"%d","units":"","value_type":%q}`, id, key, key, val, now, vt)
	}
	res := map[string]string{
		"apiinfo.version":   `"7.0.12"`,
		"user.login":        `"session"`,
		"user.logout":       `true`,
		"host.get":          `[{"hostid":"1","name":"web","status":"0","maintenance_status":"0","active_available":"1","interfaces":[],"hostgroups":[{"name":"g"}]}]`,
		"problem.get":       fmt.Sprintf(`[{"eventid":"5","objectid":"9","name":"p","severity":"4","clock":"%d","acknowledged":"0","suppressed":"0","opdata":"","tags":[]}]`, now),
		"trigger.get":       `[{"triggerid":"9","manual_close":"1","hosts":[{"hostid":"1","name":"web"}]}]`,
		"item.get":          itemsFor(req.Params, item("1", model.KeyCPU, "40", "0"), item("2", `net.if.in["eth0"]`, "9", "3")),
		"history.get":       fmt.Sprintf(`[{"itemid":"1","clock":"%d","value":"30"}]`, now-60),
		"trend.get":         fmt.Sprintf(`[{"itemid":"1","clock":"%d","num":"60","value_min":"1","value_avg":"33","value_max":"90"}]`, now-7200),
		"event.acknowledge": `{"eventids":["5"]}`,
	}[req.Method]
	io.WriteString(w, `{"jsonrpc":"2.0","result":`+res+`,"id":1}`)
}

// itemsFor answers as item.get does: key filter, prefix search or one host.
func itemsFor(params json.RawMessage, cpu, net string) string {
	switch p := string(params); {
	case strings.Contains(p, `"search"`):
		return "[" + net + "]"
	case strings.Contains(p, `"filter"`):
		return "[" + cpu + "]"
	}
	return "[" + cpu + "," + net + "]"
}

func TestFullStack(t *testing.T) {
	s := &server{calls: map[string]int{}}
	srv := httptest.NewServer(s)
	defer srv.Close()

	client := zabbix.New(zabbix.Options{URL: srv.URL + "/api_jsonrpc.php", VerifyTLS: true})
	p := poller.New(client, config.Auth{Username: "u", Password: "p"}, time.Hour, poller.Fmt{})
	ctx, cancel := context.WithCancel(context.Background())
	cmds := make(chan poller.Cmd, 1)
	events := make(chan poller.Event, 16)
	done := make(chan struct{})
	go func() { defer close(done); p.Run(ctx, cmds, events) }()

	wait := func(match func(poller.Event) bool) poller.Event {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for {
			select {
			case e := <-events:
				if f, ok := e.(poller.Failed); ok {
					t.Fatalf("failed: %v", f.Err)
				}
				if match(e) {
					return e
				}
			case <-deadline:
				t.Fatal("timed out")
			}
		}
	}
	snap := wait(func(e poller.Event) bool { _, ok := e.(poller.Snapshot); return ok }).(poller.Snapshot)
	h := snap.Hosts[0]
	if h.CPU != model.Some(40) || h.NetIn != model.Some(9) || h.Problems[model.High] != 1 || len(h.CPUHist) != 2 {
		t.Errorf("%+v", h)
	}
	if pr := snap.Problems[0]; pr.Host != "web" || !pr.ManualClose {
		t.Errorf("%+v", pr)
	}

	p.Watch("1", 7*24*3600)
	d := wait(func(e poller.Event) bool { _, ok := e.(poller.Detail); return ok }).(poller.Detail)
	if len(d.CPU) != 2 || d.CPU[0].V != 33 || d.CPU[1].V != 30 {
		t.Errorf("trend then raw tail: %+v", d.CPU)
	}
	if err := p.Acknowledge(ctx, []string{"5"}, "seen", false); err != nil {
		t.Fatal(err)
	}
	cancel()
	<-done
	if s.count("user.logout") != 1 {
		t.Errorf("logouts = %d", s.count("user.logout"))
	}
	if s.count("event.acknowledge") != 1 || s.count("trend.get") != 1 {
		t.Errorf("%v", s.calls)
	}
}

func TestErrorStringsCarryNoCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"jsonrpc":"2.0","error":{"code":-32602,"message":"Invalid params.","data":"Not authorized."},"id":1}`)
	}))
	defer srv.Close()
	client := zabbix.New(zabbix.Options{URL: srv.URL, VerifyTLS: true})
	err := client.Login(context.Background(), "admin", "hunter2")
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Errorf("%v", err)
	}
}
