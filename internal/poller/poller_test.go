package poller

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/knightmare2600/vogter/internal/config"
	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

type fakeAPI struct {
	mu        sync.Mutex
	calls     []string
	version   string
	hostErrs  []error // returned by successive Hosts calls, then nil
	logins    int
	logouts   int
	sessionUp bool

	items        []zabbix.Item
	triggerIDs   []string
	extraHang    bool
	extraErr     error
	historyErrs  []error
	historyTypes []int
	historyFrom  []int64
	history      []zabbix.HistoryPoint
	hostItems    []zabbix.Item
	trends       []zabbix.TrendPoint
	trendErr     error
	trendFrom    []int64
}

func (f *fakeAPI) log(s string) { f.mu.Lock(); f.calls = append(f.calls, s); f.mu.Unlock() }

func (f *fakeAPI) Login(context.Context, string, string) error {
	f.log("login")
	f.mu.Lock()
	f.logins++
	f.sessionUp = true
	f.mu.Unlock()
	return nil
}

func (f *fakeAPI) Logout(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sessionUp {
		f.calls = append(f.calls, "logout")
		f.logouts++
		f.sessionUp = false
	}
	return nil
}

func (f *fakeAPI) SetToken(string) { f.log("token") }

func (f *fakeAPI) APIVersion(context.Context) (string, error) {
	f.log("version")
	return f.version, nil
}

func (f *fakeAPI) Hosts(context.Context) ([]zabbix.Host, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "hosts")
	if len(f.hostErrs) > 0 {
		err := f.hostErrs[0]
		f.hostErrs = f.hostErrs[1:]
		return nil, err
	}
	return []zabbix.Host{{HostID: "1"}}, nil
}

func (f *fakeAPI) Problems(context.Context) ([]zabbix.Problem, error) {
	f.log("problems")
	return nil, nil
}

func (f *fakeAPI) Triggers(_ context.Context, ids []string) ([]zabbix.Trigger, error) {
	f.log("triggers")
	f.mu.Lock()
	f.triggerIDs = ids
	f.mu.Unlock()
	return nil, nil
}

func (f *fakeAPI) ItemsByKey(context.Context, []string, []string) ([]zabbix.Item, error) {
	f.log("items")
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.items, nil
}

func (f *fakeAPI) ItemsByPrefix(ctx context.Context, _, _ []string) ([]zabbix.Item, error) {
	f.log("extra")
	f.mu.Lock()
	hang, fail := f.extraHang, f.extraErr
	f.mu.Unlock()
	if hang {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if fail != nil {
		return nil, fail
	}
	return []zabbix.Item{{ItemID: "9", HostID: "1", Key: `net.if.in["eth0"]`, LastValue: "5", LastClock: "1", ValueType: "3"}}, nil
}

func (f *fakeAPI) History(_ context.Context, ids []string, vt int, from int64) ([]zabbix.HistoryPoint, error) {
	f.log("history")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.historyTypes = append(f.historyTypes, vt)
	f.historyFrom = append(f.historyFrom, from)
	if len(f.historyErrs) > 0 {
		err := f.historyErrs[0]
		f.historyErrs = f.historyErrs[1:]
		return nil, err
	}
	return f.history, nil
}

func (f *fakeAPI) Trends(_ context.Context, ids []string, from int64) ([]zabbix.TrendPoint, error) {
	f.log("trends")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.trendFrom = append(f.trendFrom, from)
	return f.trends, f.trendErr
}

func (f *fakeAPI) HostItems(context.Context, string) ([]zabbix.Item, error) {
	f.log("hostitems")
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hostItems, nil
}

func (f *fakeAPI) Acknowledge(context.Context, []string, string, bool) error {
	f.log("ack")
	return nil
}

func (f *fakeAPI) count(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == name {
			n++
		}
	}
	return n
}

func start(t *testing.T, api *fakeAPI, auth config.Auth, interval time.Duration) (chan Cmd, chan Event, context.CancelFunc, chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmds := make(chan Cmd, 1)
	events := make(chan Event, 16)
	done := make(chan struct{})
	p := New(api, auth, interval, Fmt{})
	go func() { defer close(done); p.Run(ctx, cmds, events) }()
	return cmds, events, cancel, done
}

func next(t *testing.T, events chan Event) Event {
	t.Helper()
	select {
	case e := <-events:
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
	}
	return nil
}

func TestSingleInitialPoll(t *testing.T) {
	api := &fakeAPI{version: "7.0.12"}
	_, events, cancel, done := start(t, api, config.Auth{Token: "t"}, time.Hour)
	if _, ok := next(t, events).(Connected); !ok {
		t.Fatal("first event must be Connected")
	}
	if _, ok := next(t, events).(Snapshot); !ok {
		t.Fatal("second event must be Snapshot")
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done
	if n := api.count("hosts"); n != 1 {
		t.Errorf("start-up made %d polls, want 1 (zabterm made 2, D1)", n)
	}
}

func TestRefreshCoalesces(t *testing.T) {
	api := &fakeAPI{version: "7.0.12"}
	cmds, events, cancel, done := start(t, api, config.Auth{Token: "t"}, time.Hour)
	next(t, events)
	next(t, events)
	cmds <- Refresh
	next(t, events)
	cancel()
	<-done
	if n := api.count("hosts"); n != 2 {
		t.Errorf("%d polls, want 2", n)
	}
}

func TestSessionReloginAndLogout(t *testing.T) {
	authErr := &zabbix.Error{Kind: zabbix.KindAuth, Method: "host.get", Message: "Invalid params.", Data: "Session terminated, re-login, please."}
	api := &fakeAPI{version: "7.0.12", hostErrs: []error{authErr}}
	cmds, events, cancel, done := start(t, api, config.Auth{Username: "u", Password: "p"}, time.Hour)
	next(t, events) // Connected
	if f, ok := next(t, events).(Failed); !ok || zabbix.KindOf(f.Err) != zabbix.KindAuth {
		t.Fatal("want Failed with an auth error")
	}
	cmds <- Refresh
	if _, ok := next(t, events).(Connected); !ok {
		t.Fatal("an expired session must reconnect")
	}
	next(t, events) // Snapshot
	cancel()
	<-done
	if api.logins != 2 {
		t.Errorf("logins = %d, want 2", api.logins)
	}
	if api.sessionUp || api.logouts < 1 {
		t.Error("the session must be closed on exit (D3)")
	}
}

func TestTokenAuthErrorDoesNotReconnect(t *testing.T) {
	authErr := &zabbix.Error{Kind: zabbix.KindAuth, Method: "host.get", Data: "Not authorized."}
	api := &fakeAPI{version: "7.0.12", hostErrs: []error{authErr}}
	cmds, events, cancel, done := start(t, api, config.Auth{Token: "t"}, time.Hour)
	next(t, events)
	next(t, events)
	cmds <- Refresh
	if _, ok := next(t, events).(Snapshot); !ok {
		t.Fatal("token auth must poll again without reconnecting")
	}
	cancel()
	<-done
	if n := api.count("version"); n != 1 {
		t.Errorf("version called %d times, want 1", n)
	}
}

func TestUnsupportedVersion(t *testing.T) {
	api := &fakeAPI{version: "6.0.30"}
	_, events, cancel, done := start(t, api, config.Auth{Token: "t"}, time.Hour)
	if _, ok := next(t, events).(Failed); !ok {
		t.Fatal("6.0 must be refused")
	}
	cancel()
	<-done
	if api.count("hosts") != 0 {
		t.Error("must not poll an unsupported server")
	}
}

func cpuItem(id, valueType string) zabbix.Item {
	return zabbix.Item{ItemID: id, HostID: "1", Key: model.KeyCPU, LastValue: "1", LastClock: "1", ValueType: valueType}
}

func direct(api *fakeAPI) *Poller {
	return New(api, config.Auth{Token: "t"}, time.Hour, Fmt{})
}

func TestExtraFailureKeepsSnapshotAndBacksOff(t *testing.T) {
	api := &fakeAPI{extraErr: errors.New("502")}
	now := time.Unix(1000, 0)
	p := direct(api)
	p.Now = func() time.Time { return now }
	if _, err := p.snapshot(context.Background()); err != nil {
		t.Fatalf("a failed net/disk search must not fail the poll: %v", err)
	}
	if _, err := p.snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := api.count("extra"); n != 1 {
		t.Errorf("searched %d times during back-off, want 1", n)
	}
	now = now.Add(extraRetry)
	api.extraErr = nil
	snap, err := p.snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n := api.count("extra"); n != 2 {
		t.Errorf("search not retried after the back-off: %d", n)
	}
	if !snap.Hosts[0].NetIn.Set {
		t.Error("extra items lost")
	}
}

func TestExtraBudgetDoesNotHoldUpPoll(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the 5 s budget")
	}
	api := &fakeAPI{extraHang: true}
	start := time.Now()
	if _, err := direct(api).snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < extraBudget || d > extraBudget+2*time.Second {
		t.Errorf("poll took %v, want about %v", d, extraBudget)
	}
}

func TestFailedSeedIsRetried(t *testing.T) {
	api := &fakeAPI{items: []zabbix.Item{cpuItem("1", "0")}, historyErrs: []error{errors.New("boom")}}
	p := direct(api)
	ctx := context.Background()
	if _, err := p.snapshot(ctx); err != nil {
		t.Fatalf("a failed seed must not fail the poll: %v", err)
	}
	_, _ = p.snapshot(ctx)
	_, _ = p.snapshot(ctx)
	if n := api.count("history"); n != 2 {
		t.Errorf("history called %d times, want 2: one failure, one success, then none (D6)", n)
	}
}

func TestSeedUsesItemValueType(t *testing.T) {
	api := &fakeAPI{items: []zabbix.Item{cpuItem("1", "0"), cpuItem("2", "3"), cpuItem("3", "4")}}
	if _, err := direct(api).snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	slices.Sort(api.historyTypes)
	if !slices.Equal(api.historyTypes, []int{0, 3}) {
		t.Errorf("history types = %v, want float and unsigned only (D7)", api.historyTypes)
	}
}

func TestTriggerIDsUnique(t *testing.T) {
	api := &problemAPI{fakeAPI: &fakeAPI{}}
	p := New(api, config.Auth{}, time.Hour, Fmt{})
	if _, err := p.snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(api.triggerIDs) != 2 {
		t.Errorf("trigger ids = %v, want 2 unique", api.triggerIDs)
	}
}

type problemAPI struct{ *fakeAPI }

func (problemAPI) Problems(context.Context) ([]zabbix.Problem, error) {
	return []zabbix.Problem{{ObjectID: "a"}, {ObjectID: "b"}, {ObjectID: "a"}}, nil
}

func TestAcknowledgeDoesNotWaitForPoll(t *testing.T) {
	api := &fakeAPI{}
	if err := direct(api).Acknowledge(context.Background(), []string{"1"}, "", false); err != nil {
		t.Fatal(err)
	}
	if api.count("ack") != 1 {
		t.Error("ack not sent")
	}
}

func TestIntervalPolls(t *testing.T) {
	api := &fakeAPI{version: "7.0.12"}
	_, events, cancel, done := start(t, api, config.Auth{Token: "t"}, 30*time.Millisecond)
	for i := 0; i < 4; i++ {
		next(t, events)
	}
	cancel()
	<-done
	if n := api.count("hosts"); n < 3 {
		t.Errorf("%d polls, want at least 3", n)
	}
}
