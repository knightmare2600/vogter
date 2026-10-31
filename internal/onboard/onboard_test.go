package onboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/knightmare2600/vogter/internal/config"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

const (
	testPassword = "correct-horse-battery"
	testSecret   = "generated-secret"
)

// fakeServer behaves like the Zabbix 7.0 token API as observed on 7.0.32.
type fakeServer struct {
	mu        sync.Mutex
	calls     []string
	version   string
	tokens    map[string]string
	nextID    int
	denied    bool
	rejectAll bool
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	_ = json.Unmarshal(body, &req)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req.Method)
	result, fail := `null`, ""
	switch req.Method {
	case "apiinfo.version":
		result = fmt.Sprintf("%q", f.version)
	case "user.login":
		var p struct{ Password string }
		_ = json.Unmarshal(req.Params, &p)
		if p.Password != testPassword {
			fail = `{"code":-32500,"message":"Application error.","data":"Incorrect user name or password or account is temporarily blocked."}`
		}
		result = `"session"`
	case "user.logout":
		result = `true`
	case "token.create":
		var p struct{ Name string }
		_ = json.Unmarshal(req.Params, &p)
		switch {
		case f.denied:
			fail = `{"code":-32500,"message":"Application error.","data":"No permissions to call \"token.create\"."}`
		case slices.Contains(mapValues(f.tokens), p.Name):
			fail = fmt.Sprintf(`{"code":-32602,"message":"Invalid params.","data":"API token \"%s\" already exists for userid \"1\"."}`, p.Name)
		default:
			f.nextID++
			id := fmt.Sprint(f.nextID)
			f.tokens[id] = p.Name
			result = fmt.Sprintf(`{"tokenids":[%q]}`, id)
		}
	case "token.generate":
		result = fmt.Sprintf(`[{"tokenid":"1","token":%q}]`, testSecret)
	case "token.delete":
		var ids []string
		_ = json.Unmarshal(req.Params, &ids)
		for _, id := range ids {
			delete(f.tokens, id)
		}
		result = `{"tokenids":[]}`
	case "host.get":
		if f.rejectAll || r.Header.Get("Authorization") != "Bearer "+testSecret {
			fail = `{"code":-32602,"message":"Invalid params.","data":"Not authorized."}`
		}
		result = `"3"`
	}
	if fail != "" {
		fmt.Fprintf(w, `{"jsonrpc":"2.0","error":%s,"id":1}`, fail)
		return
	}
	fmt.Fprintf(w, `{"jsonrpc":"2.0","result":%s,"id":1}`, result)
}

func mapValues(m map[string]string) []string {
	var out []string
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func setup(t *testing.T) (*Backend, *fakeServer, string) {
	t.Helper()
	f := &fakeServer{version: "7.0.32", tokens: map[string]string{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	b := New(filepath.Join(t.TempDir(), "config.toml"), "vaegter/test")
	b.Hostname = func() (string, error) { return "My.Laptop", nil }
	return b, f, srv.URL
}

func TestNormalise(t *testing.T) {
	for in, want := range map[string]string{
		"zabbix.example.com":                       "https://zabbix.example.com",
		"  http://z.example:8080/ ":                "http://z.example:8080",
		"https://z.example/zabbix/":                "https://z.example/zabbix",
		"https://z.example/zabbix/api_jsonrpc.php": "https://z.example/zabbix",
	} {
		if got, err := Normalise(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "   ", "ftp://z.example", "not a url", "https://"} {
		if got, err := Normalise(bad); err == nil {
			t.Errorf("%q: want an error, got %q", bad, got)
		}
	}
}

func TestSignInCreatesAndSavesAToken(t *testing.T) {
	b, f, url := setup(t)
	profile, err := b.SignIn(context.Background(), url, true, "kim.operator", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(mapValues(f.tokens), []string{"vaegter-my-laptop"}) {
		t.Errorf("%v", f.tokens)
	}
	if !slices.Contains(f.calls, "user.logout") {
		t.Error("the login session must be closed")
	}
	c, _, err := config.Load(b.Path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Profiles[profile].Token.Reveal() != testSecret {
		t.Error("token not saved")
	}
	raw, _ := os.ReadFile(b.Path)
	if strings.Contains(string(raw), testPassword) || strings.Contains(string(raw), "kim.operator") {
		t.Error("the login must never reach the config file")
	}
}

func TestSignInPicksAFreeTokenName(t *testing.T) {
	b, f, url := setup(t)
	f.tokens["9"] = "vaegter-my-laptop"
	if _, err := b.SignIn(context.Background(), url, true, "Admin", testPassword); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(mapValues(f.tokens), "vaegter-my-laptop-2") || !slices.Contains(mapValues(f.tokens), "vaegter-my-laptop") {
		t.Errorf("an existing token must be left alone: %v", f.tokens)
	}
}

func TestSignInFailuresLeaveNothingBehind(t *testing.T) {
	cases := []struct {
		name string
		prep func(*fakeServer, *Backend)
		pass string
		kind zabbix.Kind
	}{
		{"wrong password", nil, "nope", zabbix.KindAuth},
		{"no permission", func(f *fakeServer, _ *Backend) { f.denied = true }, testPassword, zabbix.KindPermission},
		{"token fails its test", func(f *fakeServer, _ *Backend) { f.rejectAll = true }, testPassword, zabbix.KindAuth},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, f, url := setup(t)
			if tc.prep != nil {
				tc.prep(f, b)
			}
			_, err := b.SignIn(context.Background(), url, true, "Admin", tc.pass)
			if zabbix.KindOf(err) != tc.kind {
				t.Fatalf("kind of %v", err)
			}
			if strings.Contains(err.Error(), tc.pass) {
				t.Error("the password leaked into an error")
			}
			if len(f.tokens) != 0 {
				t.Errorf("token left on the server: %v", f.tokens)
			}
			if _, err := os.Stat(b.Path); !errors.Is(err, os.ErrNotExist) {
				t.Error("nothing may be written after a failure")
			}
		})
	}
}

func TestFailedSaveRemovesTheToken(t *testing.T) {
	b, f, url := setup(t)
	if err := os.WriteFile(b.Path, []byte("[broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SignIn(context.Background(), url, true, "Admin", testPassword); err == nil {
		t.Fatal("want an error")
	}
	if len(f.tokens) != 0 || !slices.Contains(f.calls, "user.logout") {
		t.Errorf("%v %v", f.tokens, f.calls)
	}
}

func TestUseToken(t *testing.T) {
	b, f, url := setup(t)
	if _, err := b.UseToken(context.Background(), url, true, "wrong"); zabbix.KindOf(err) != zabbix.KindAuth {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat(b.Path); err == nil {
		t.Fatal("a rejected token must not be saved")
	}
	if _, err := b.UseToken(context.Background(), url, true, testSecret); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(f.calls, "user.login") {
		t.Error("a pasted token needs no login")
	}
}

func TestProbe(t *testing.T) {
	b, f, url := setup(t)
	if got, err := b.Probe(context.Background(), url, true); err != nil || got != "Zabbix 7.0.32" {
		t.Fatalf("%q %v", got, err)
	}
	f.version = "6.0.30"
	if _, err := b.Probe(context.Background(), url, true); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("%v", err)
	}
}

func TestTokenNameWithoutAHostname(t *testing.T) {
	b, _, _ := setup(t)
	b.Hostname = func() (string, error) { return "", errors.New("none") }
	if b.tokenName() != "vaegter" {
		t.Error(b.tokenName())
	}
}
