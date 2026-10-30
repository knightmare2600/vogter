package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"

	"github.com/knightmare2600/vogter/internal/config"
)

func TestParseArgs(t *testing.T) {
	cases := []struct {
		args    []string
		cmd     string
		profile string
		force   bool
		err     bool
	}{
		{args: nil},
		{args: []string{"check", "-p", "staging"}, cmd: "check", profile: "staging"},
		{args: []string{"--profile", "staging", "check"}, cmd: "check", profile: "staging"},
		{args: []string{"init", "--force"}, cmd: "init", force: true},
		{args: []string{"check", "--force"}, err: true},
		{args: []string{"frobnicate"}, err: true},
		{args: []string{"check", "extra"}, err: true},
		{args: []string{"--nope"}, err: true},
	}
	for _, tc := range cases {
		o, cmd, err := parseArgs(tc.args, io.Discard)
		if (err != nil) != tc.err {
			t.Errorf("%v: err = %v", tc.args, err)
			continue
		}
		if err == nil && (cmd != tc.cmd || o.profile != tc.profile || o.force != tc.force) {
			t.Errorf("%v: got cmd=%q profile=%q force=%v", tc.args, cmd, o.profile, o.force)
		}
	}
}

func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv(config.EnvURL, "")
	t.Setenv(config.EnvToken, "")
	return dir
}

func TestInitHonoursConfigFlag(t *testing.T) {
	dir := isolate(t)
	alt := filepath.Join(dir, "alt.toml")
	var out, errb bytes.Buffer
	if rc := run([]string{"-c", alt, "init"}, &out, &errb); rc != 0 {
		t.Fatalf("rc=%d %s", rc, errb.String())
	}
	if _, err := os.Stat(alt); err != nil {
		t.Errorf("init ignored -c (zabterm did, D15): %v", err)
	}
	if _, err := os.Stat(config.DefaultPath()); err == nil {
		t.Error("init wrote the default path as well")
	}
	if rc := run([]string{"-c", alt, "init"}, &out, &errb); rc != 1 {
		t.Error("second init must refuse")
	}
}

func TestVersionAndConfigCommands(t *testing.T) {
	isolate(t)
	var out bytes.Buffer
	if rc := run([]string{"-V"}, &out, io.Discard); rc != 0 || !strings.HasPrefix(out.String(), "vogter ") {
		t.Errorf("version: %d %q", rc, out.String())
	}
	out.Reset()
	if rc := run([]string{"config"}, &out, io.Discard); rc != 0 || strings.TrimSpace(out.String()) != config.DefaultPath() {
		t.Errorf("config: %d %q", rc, out.String())
	}
}

// fakeZabbix answers the calls `vogter check` makes.
func fakeZabbix(t *testing.T, version string, authFails bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		res := map[string]string{
			"apiinfo.version": `"` + version + `"`,
			"user.login":      `"sess"`,
			"user.logout":     `true`,
			"host.get":        `[{"hostid":"1","name":"a","status":"0","maintenance_status":"0"}]`,
			"problem.get":     `[]`,
		}[req.Method]
		if authFails && req.Method == "host.get" {
			io.WriteString(w, `{"jsonrpc":"2.0","error":{"code":-32602,"message":"Invalid params.","data":"Not authorized."},"id":1}`)
			return
		}
		io.WriteString(w, `{"jsonrpc":"2.0","result":`+res+`,"id":1}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCheck(t *testing.T) {
	isolate(t)
	srv := fakeZabbix(t, "7.0.12", false)
	t.Setenv(config.EnvURL, srv.URL)
	t.Setenv(config.EnvToken, "secret-token-value")
	var out, errb bytes.Buffer
	if rc := run([]string{"check"}, &out, &errb); rc != 0 {
		t.Fatalf("rc=%d out=%s err=%s", rc, out.String(), errb.String())
	}
	all := out.String() + errb.String()
	if !strings.Contains(all, "Zabbix 7.0.12 · 1 hosts · 0 active problems") {
		t.Errorf("output: %s", all)
	}
	if strings.Contains(all, "secret-token-value") {
		t.Error("check printed the token")
	}
}

func TestCheckFailures(t *testing.T) {
	isolate(t)
	cases := []struct {
		name, version string
		authFails     bool
		want          string
	}{
		{"auth", "7.0.12", true, "credentials rejected"},
		{"old server", "6.0.30", false, "not supported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := fakeZabbix(t, tc.version, tc.authFails)
			t.Setenv(config.EnvURL, srv.URL)
			t.Setenv(config.EnvToken, "t")
			var out, errb bytes.Buffer
			if rc := run([]string{"check"}, &out, &errb); rc != 1 {
				t.Fatalf("rc=%d", rc)
			}
			if !strings.Contains(errb.String(), tc.want) {
				t.Errorf("stderr lacks %q: %s", tc.want, errb.String())
			}
		})
	}
	t.Setenv(config.EnvURL, "http://127.0.0.1:1")
	t.Setenv(config.EnvToken, "t")
	var errb bytes.Buffer
	if rc := run([]string{"check"}, io.Discard, &errb); rc != 1 || !strings.Contains(errb.String(), "cannot reach the server") {
		t.Errorf("unreachable: rc=%d %s", rc, errb.String())
	}
}

func TestColourOverrides(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv(config.EnvTruecolor, "1")
	if len(colourProfile()) != 1 {
		t.Error("VOGTER_TRUECOLOR must force a profile")
	}
	t.Setenv("NO_COLOR", "1")
	if colourProfile() != nil {
		t.Error("NO_COLOR must win")
	}
	if p := colorprofile.Env([]string{"NO_COLOR=1", "TERM=xterm-256color", "COLORTERM=truecolor"}); p == colorprofile.TrueColor || p == colorprofile.ANSI256 {
		t.Errorf("detection ignored NO_COLOR: %v", p)
	}
}
