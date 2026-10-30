package zabbix

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type recorded struct {
	Method string
	Auth   string
	CT     string
	Params map[string]any
	Raw    json.RawMessage
}

// fake is a JSON-RPC endpoint answering from a method→result table.
type fake struct {
	mu      sync.Mutex
	calls   []recorded
	results map[string]string
	errors  map[string]string
	status  int
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		ID     any             `json:"id"`
	}
	_ = json.Unmarshal(body, &req)
	rec := recorded{Method: req.Method, Auth: r.Header.Get("Authorization"), CT: r.Header.Get("Content-Type"), Raw: req.Params}
	_ = json.Unmarshal(req.Params, &rec.Params)
	f.mu.Lock()
	f.calls = append(f.calls, rec)
	f.mu.Unlock()
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	id, _ := json.Marshal(req.ID)
	if e, ok := f.errors[req.Method]; ok {
		io.WriteString(w, `{"jsonrpc":"2.0","error":`+e+`,"id":`+string(id)+`}`)
		return
	}
	res, ok := f.results[req.Method]
	if !ok {
		res = "[]"
	}
	io.WriteString(w, `{"jsonrpc":"2.0","result":`+res+`,"id":`+string(id)+`}`)
}

func newFake(t *testing.T, f *fake) *Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return New(Options{URL: srv.URL + "/api_jsonrpc.php", VerifyTLS: true, UserAgent: "vogter/test"})
}

func TestBearerOnlyWhereAllowed(t *testing.T) {
	f := &fake{results: map[string]string{"apiinfo.version": `"7.0.12"`, "user.login": `"sess"`}}
	c := newFake(t, f)
	ctx := context.Background()
	c.SetToken("tok")
	if _, err := c.APIVersion(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Hosts(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Login(ctx, "u", "p"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Problems(ctx); err != nil {
		t.Fatal(err)
	}
	want := []struct{ method, auth string }{
		{"apiinfo.version", ""}, {"host.get", "Bearer tok"}, {"user.login", ""}, {"problem.get", "Bearer sess"},
	}
	for i, w := range want {
		got := f.calls[i]
		if got.Method != w.method || got.Auth != w.auth {
			t.Errorf("call %d = %s %q, want %s %q", i, got.Method, got.Auth, w.method, w.auth)
		}
		if got.CT != "application/json-rpc" {
			t.Errorf("Content-Type = %q", got.CT)
		}
	}
}

func TestLogoutOnlyForSessions(t *testing.T) {
	f := &fake{results: map[string]string{"user.login": `"sess"`, "user.logout": `true`}}
	c := newFake(t, f)
	ctx := context.Background()
	c.SetToken("api-token")
	if err := c.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Fatal("an API token must never be logged out")
	}
	if err := c.Login(ctx, "u", "p"); err != nil {
		t.Fatal(err)
	}
	if err := c.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	last := f.calls[len(f.calls)-1]
	if last.Method != "user.logout" || last.Auth != "Bearer sess" || string(last.Raw) != "[]" {
		t.Errorf("logout call = %+v", last)
	}
	if c.HasSession() {
		t.Error("session must be cleared")
	}
}

// Parameters must match those captured from zabterm 0.1.2 (porting-audit §6.2).
func TestRequestParams(t *testing.T) {
	f := &fake{}
	c := newFake(t, f)
	ctx := context.Background()
	_, _ = c.Hosts(ctx)
	_, _ = c.Problems(ctx)
	_, _ = c.Triggers(ctx, []string{"9001"})
	_, _ = c.ItemsByKey(ctx, nil, []string{"system.cpu.util"})
	_, _ = c.ItemsByPrefix(ctx, nil, []string{"net.if.in["})
	_, _ = c.HostItems(ctx, "10084")
	_, _ = c.History(ctx, []string{"1001"}, 0, 1000)
	_, _ = c.Trends(ctx, []string{"1001"}, 1000)
	_ = c.Acknowledge(ctx, []string{"502"}, "looking", false)
	_ = c.Acknowledge(ctx, []string{"502"}, "", true)

	want := []string{
		`{"output":["hostid","name","status","maintenance_status","active_available"],"selectHostGroups":["name"],"selectInterfaces":["ip","dns","useip","port","available","type","error"],"sortfield":"name"}`,
		`{"limit":500,"output":["eventid","objectid","name","severity","clock","acknowledged","suppressed","opdata"],"selectTags":["tag","value"],"sortfield":["eventid"],"sortorder":"DESC"}`,
		`{"output":["triggerid","description","manual_close"],"selectHosts":["hostid","name"],"triggerids":["9001"]}`,
		`{"filter":{"key_":["system.cpu.util"]},"monitored":true,"output":["itemid","hostid","name","key_","lastvalue","lastclock","units","value_type"]}`,
		`{"monitored":true,"output":["itemid","hostid","name","key_","lastvalue","lastclock","units","value_type"],"search":{"key_":["net.if.in["]},"searchByAny":true,"startSearch":true}`,
		`{"hostids":["10084"],"output":["itemid","hostid","name","key_","lastvalue","lastclock","units","value_type"],"sortfield":"name"}`,
		`{"history":0,"itemids":["1001"],"sortfield":"clock","sortorder":"ASC","time_from":1000}`,
		`{"itemids":["1001"],"output":["itemid","clock","num","value_min","value_avg","value_max"],"time_from":1000}`,
		`{"action":6,"eventids":["502"],"message":"looking"}`,
		`{"action":3,"eventids":["502"],"message":""}`,
	}
	if len(f.calls) != len(want) {
		t.Fatalf("%d calls, want %d", len(f.calls), len(want))
	}
	for i, w := range want {
		got, _ := json.Marshal(f.calls[i].Params) // re-marshal sorts keys
		if string(got) != w {
			t.Errorf("%s params\n got %s\nwant %s", f.calls[i].Method, got, w)
		}
	}
}

func TestEmptyIDsSkipCall(t *testing.T) {
	f := &fake{}
	c := newFake(t, f)
	ctx := context.Background()
	_, _ = c.Triggers(ctx, nil)
	_, _ = c.History(ctx, nil, 0, 0)
	_, _ = c.Trends(ctx, nil, 0)
	if len(f.calls) != 0 {
		t.Errorf("made %d calls with no ids", len(f.calls))
	}
}

func TestErrorKinds(t *testing.T) {
	cases := []struct {
		name string
		f    *fake
		want Kind
	}{
		{"http 502", &fake{status: 502}, KindHTTP},
		{"not json", &fake{results: map[string]string{"host.get": `<html>`}}, KindDecode},
		{"auth", &fake{errors: map[string]string{"host.get": `{"code":-32602,"message":"Invalid params.","data":"Not authorized."}`}}, KindAuth},
		{"session", &fake{errors: map[string]string{"host.get": `{"code":-32602,"message":"Invalid params.","data":"Session terminated, re-login, please."}`}}, KindAuth},
		{"permission", &fake{errors: map[string]string{"host.get": `{"code":-32500,"message":"Application error.","data":"No permissions to referred object or it does not exist!"}`}}, KindPermission},
		{"other rpc", &fake{errors: map[string]string{"host.get": `{"code":-32602,"message":"Invalid params.","data":"Incorrect method."}`}}, KindRPC},
		{"wrong shape", &fake{results: map[string]string{"host.get": `{"a":1}`}}, KindShape},
		{"missing id", &fake{results: map[string]string{"host.get": `[{"name":"x","status":"0"}]`}}, KindShape},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newFake(t, tc.f)
			hosts, err := c.Hosts(context.Background())
			if KindOf(err) != tc.want {
				t.Fatalf("kind = %v (%v), want %v", KindOf(err), err, tc.want)
			}
			if len(hosts) != 0 && tc.want != 0 {
				t.Error("a failed call must not return data")
			}
		})
	}
}

func TestRPCErrorMessage(t *testing.T) {
	f := &fake{errors: map[string]string{"host.get": `{"code":-32602,"message":"Invalid params.","data":"Not authorized."}`}}
	_, err := newFake(t, f).Hosts(context.Background())
	if err == nil || err.Error() != "host.get: Invalid params. Not authorized." {
		t.Errorf("err = %v", err)
	}
}

func TestTransportErrors(t *testing.T) {
	c := New(Options{URL: "http://127.0.0.1:1/api_jsonrpc.php", VerifyTLS: true})
	if _, err := c.APIVersion(context.Background()); KindOf(err) != KindTransport {
		t.Errorf("refused: kind = %v (%v)", KindOf(err), err)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer slow.Close()
	c = New(Options{URL: slow.URL, VerifyTLS: true, Timeout: 100 * time.Millisecond})
	start := time.Now()
	if _, err := c.APIVersion(context.Background()); KindOf(err) != KindTransport {
		t.Errorf("timeout: kind = %v (%v)", KindOf(err), err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("timeout not honoured")
	}

	c = New(Options{URL: slow.URL, VerifyTLS: true})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	if _, err := c.APIVersion(ctx); KindOf(err) != KindTransport {
		t.Errorf("cancel: kind = %v (%v)", KindOf(err), err)
	}
}

func TestTLSVerification(t *testing.T) {
	srv := httptest.NewTLSServer(&fake{results: map[string]string{"apiinfo.version": `"7.0.0"`}})
	defer srv.Close()
	strict := New(Options{URL: srv.URL, VerifyTLS: true})
	if _, err := strict.APIVersion(context.Background()); KindOf(err) != KindTransport {
		t.Errorf("self-signed certificate accepted with verification on: %v", err)
	}
	lax := New(Options{URL: srv.URL, VerifyTLS: false})
	if v, err := lax.APIVersion(context.Background()); err != nil || v != "7.0.0" {
		t.Errorf("verification off: %q, %v", v, err)
	}
}

func TestOptionalFieldsAndValue(t *testing.T) {
	f := &fake{results: map[string]string{
		"host.get": `[{"hostid":"1","name":"a","status":"0","maintenance_status":"0"}]`,
		"item.get": `[{"itemid":"1","hostid":"1","name":"cpu","key_":"system.cpu.util","lastvalue":"0","lastclock":"0","units":"%","value_type":"0"},
		              {"itemid":"2","hostid":"1","name":"cpu","key_":"system.cpu.util","lastvalue":"12.5","lastclock":"100","units":"%","value_type":"0"}]`,
	}}
	c := newFake(t, f)
	hosts, err := c.Hosts(context.Background())
	if err != nil || len(hosts) != 1 || hosts[0].ActiveAvailable != "" || hosts[0].Interfaces != nil {
		t.Fatalf("optional fields: %+v %v", hosts, err)
	}
	items, err := c.ItemsByKey(context.Background(), nil, []string{"system.cpu.util"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := items[0].Value(); ok {
		t.Error("an item that never reported must have no value, not zero")
	}
	if v, ok := items[1].Value(); !ok || v != 12.5 {
		t.Errorf("Value = %v, %v", v, ok)
	}
}

func TestInterfaceAddress(t *testing.T) {
	if got := (Interface{IP: "10.0.0.1", DNS: "h.lan", UseIP: "1", Port: "10050"}).Address(); got != "10.0.0.1:10050" {
		t.Error(got)
	}
	if got := (Interface{IP: "10.0.0.1", DNS: "h.lan", UseIP: "0", Port: "10050"}).Address(); got != "h.lan:10050" {
		t.Error(got)
	}
}

func TestErrorStringHasNoToken(t *testing.T) {
	f := &fake{errors: map[string]string{"host.get": `{"code":-32602,"message":"Invalid params.","data":"Not authorized."}`}}
	c := newFake(t, f)
	c.SetToken("super-secret-token")
	_, err := c.Hosts(context.Background())
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Error("token leaked into error text")
	}
}
