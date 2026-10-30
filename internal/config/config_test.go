package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func clearEnv(t *testing.T) {
	t.Setenv(EnvURL, "")
	t.Setenv(EnvToken, "")
}

func TestLoadDefaultsAndProfiles(t *testing.T) {
	clearEnv(t)
	p := write(t, `
[profiles.b]
url = "https://b.example/"
token = "tb"
[profiles.a]
url = "https://a.example/zabbix/api_jsonrpc.php"
token = "ta"
`)
	c, issues, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Errorf("unexpected issues: %v", issues)
	}
	if c.Theme != "auto" || c.RefreshInterval != 10 || !c.Notifications.Desktop || c.Notifications.MinSeverity != "average" {
		t.Errorf("defaults not applied: %+v", c)
	}
	// No default_profile: first in sorted order, like zabterm's BTreeMap.
	prof, err := c.Profile("")
	if err != nil || prof.Name != "a" {
		t.Fatalf("got %v, %v; want profile a", prof, err)
	}
	if got := prof.APIURL(); got != "https://a.example/zabbix/api_jsonrpc.php" {
		t.Errorf("APIURL = %q", got)
	}
	if got := prof.WebURL(); got != "https://a.example/zabbix" {
		t.Errorf("WebURL = %q", got)
	}
	b, _ := c.Profile("b")
	if got := b.APIURL(); got != "https://b.example/api_jsonrpc.php" {
		t.Errorf("APIURL = %q", got)
	}
	if _, err := c.Profile("nope"); err == nil || !strings.Contains(err.Error(), "have: a, b") {
		t.Errorf("unknown profile error = %v", err)
	}
	if !b.TLSVerify() {
		t.Error("TLS verification must default to on")
	}
}

func TestLoadNoConfig(t *testing.T) {
	clearEnv(t)
	_, _, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if !errors.Is(err, ErrNoProfiles) {
		t.Fatalf("err = %v, want ErrNoProfiles", err)
	}
}

func TestEnvURLCreatesProfile(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvURL, "https://env.example")
	c, _, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.Profile("")
	if err != nil || p.Name != "env" || p.URL != "https://env.example" {
		t.Fatalf("got %+v, %v", p, err)
	}
}

func TestValidation(t *testing.T) {
	clearEnv(t)
	p := write(t, `
theme = "solarized"
refresh_interval = 1
colour = "blue"
[notifications]
min_severity = "loud"
[profiles.x]
url = "https://x"
token = "t"
username = "u"
password = "p"
verify_tls = false
`)
	c, issues, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.RefreshInterval != MinRefresh {
		t.Errorf("RefreshInterval = %d, want clamped to %d", c.RefreshInterval, MinRefresh)
	}
	all := fmt.Sprint(issues)
	for _, want := range []string{`unknown setting "colour"`, `unknown theme "solarized"`, "below the minimum", `"loud"`, "2 credential sources", "verification turned off"} {
		if !strings.Contains(all, want) {
			t.Errorf("issues lack %q: %s", want, all)
		}
	}
}

func TestValidationErrors(t *testing.T) {
	clearEnv(t)
	for name, body := range map[string]string{
		"missing default": "default_profile = \"nope\"\n[profiles.x]\nurl = \"https://x\"\n",
		"missing url":     "[profiles.x]\ntoken = \"t\"\n",
		"bad toml":        "[profiles.x\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Load(write(t, body)); err == nil {
				t.Error("want error")
			}
		})
	}
}

func TestResolveAuthOrder(t *testing.T) {
	clearEnv(t)
	ctx := context.Background()
	t.Setenv("VOGTER_TEST_TOKEN", "from-env-var")
	p := &Profile{Name: "x", TokenEnv: "VOGTER_TEST_TOKEN", TokenCmd: "echo from-cmd", Token: "from-file", Username: "u", Password: "pw"}

	t.Setenv(EnvToken, "from-vogter-token")
	a, err := p.ResolveAuth(ctx)
	if err != nil || a.Token.Reveal() != "from-vogter-token" {
		t.Fatalf("VOGTER_TOKEN must win: %v %v", a.Token.Reveal(), err)
	}
	t.Setenv(EnvToken, "")

	a, _ = p.ResolveAuth(ctx)
	if a.Token.Reveal() != "from-env-var" {
		t.Errorf("token_env must come next, got %q", a.Token.Reveal())
	}
	p.TokenEnv = ""
	if runtime.GOOS != "windows" {
		a, _ = p.ResolveAuth(ctx)
		if a.Token.Reveal() != "from-cmd" {
			t.Errorf("token_cmd must come next, got %q", a.Token.Reveal())
		}
	}
	p.TokenCmd = ""
	a, _ = p.ResolveAuth(ctx)
	if a.Token.Reveal() != "from-file" {
		t.Errorf("token must come next, got %q", a.Token.Reveal())
	}
	p.Token = placeholder
	a, _ = p.ResolveAuth(ctx)
	if !a.IsLogin() || a.Username != "u" || a.Password.Reveal() != "pw" {
		t.Errorf("placeholder token must fall through to login, got %+v", a)
	}
	p.Username = ""
	if _, err := p.ResolveAuth(ctx); err == nil || !strings.Contains(err.Error(), "no credentials") {
		t.Errorf("err = %v", err)
	}
}

func TestResolveAuthEmptyValues(t *testing.T) {
	clearEnv(t)
	t.Setenv("VOGTER_EMPTY", "")
	p := &Profile{Name: "x", TokenEnv: "VOGTER_EMPTY", Token: "from-file"}
	if _, err := p.ResolveAuth(context.Background()); err == nil {
		t.Error("an empty token_env variable must be an error, not an empty token (D14)")
	}
}

func TestTokenCmdFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	clearEnv(t)
	p := &Profile{Name: "x", TokenCmd: "echo oops >&2; exit 3"}
	_, err := p.ResolveAuth(context.Background())
	if err == nil || !strings.Contains(err.Error(), "oops") {
		t.Errorf("err = %v", err)
	}
	p.TokenCmd = "true"
	if _, err := p.ResolveAuth(context.Background()); err == nil {
		t.Error("empty output must be an error")
	}
}

func TestSecretNeverFormats(t *testing.T) {
	s := Secret("hunter2")
	a := Auth{Token: s, Password: s}
	for _, f := range []string{"%v", "%+v", "%#v", "%s"} {
		if out := fmt.Sprintf(f, a); strings.Contains(out, "hunter2") {
			t.Errorf("%s leaked the secret: %s", f, out)
		}
	}
	if s.Reveal() != "hunter2" {
		t.Error("Reveal must return the value")
	}
}

func TestWriteTemplate(t *testing.T) {
	clearEnv(t)
	p := filepath.Join(t.TempDir(), "sub", "config.toml")
	if _, err := WriteTemplate(p, false); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(p)
		if st.Mode().Perm() != 0o600 {
			t.Errorf("mode = %o, want 600", st.Mode().Perm())
		}
	}
	if _, err := WriteTemplate(p, false); !errors.Is(err, ErrExists) {
		t.Errorf("second write err = %v, want ErrExists", err)
	}
	if _, err := WriteTemplate(p, true); err != nil {
		t.Errorf("forced write: %v", err)
	}
	// The template must load, with the placeholder treated as no credential.
	c, issues, err := Load(p)
	if err != nil {
		t.Fatalf("template does not load: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("template has issues: %v", issues)
	}
	prof, _ := c.Profile("")
	if _, err := prof.ResolveAuth(context.Background()); err == nil {
		t.Error("placeholder token must not count as a credential")
	}
}

func TestPermissionWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX modes")
	}
	clearEnv(t)
	p := write(t, "[profiles.x]\nurl = \"https://x\"\ntoken = \"t\"\n")
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	_, issues, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fmt.Sprint(issues), "readable by other users") {
		t.Errorf("no permission warning: %v", issues)
	}
}

func TestParseSeverity(t *testing.T) {
	for in, want := range map[string]int{"not_classified": 0, "NC": 0, "info": 1, "Warning": 2, "avg": 3, "high": 4, "Disaster": 5, "not-classified": 0, "not classified": 0} {
		if got, ok := ParseSeverity(in); !ok || got != want {
			t.Errorf("ParseSeverity(%q) = %d, %v", in, got, ok)
		}
	}
	if _, ok := ParseSeverity("loud"); ok {
		t.Error("unknown name accepted")
	}
}

// configs/config.example.toml is generated by `make example`; it must not
// drift from the template that `vogter init` writes.
func TestExampleMatchesTemplate(t *testing.T) {
	got, err := os.ReadFile(filepath.Join("..", "..", "configs", "config.example.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != Template {
		t.Error("configs/config.example.toml is stale: run make example")
	}
}

func TestImport(t *testing.T) {
	dir := t.TempDir()
	from := filepath.Join(dir, "zabterm.toml")
	body := "# my notes\n[profiles.prod]\nurl = \"https://z\"\ntoken_cmd = \"pass show z\"\n[profiles.lab]\nurl = \"https://l\"\n"
	if err := os.WriteFile(from, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "vogter", "config.toml")
	names, err := Import(from, dest, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "lab,prod" {
		t.Errorf("%v", names)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != body {
		t.Error("the text, comments included, must be kept")
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(dest); st.Mode().Perm() != 0o600 {
			t.Errorf("mode %o", st.Mode().Perm())
		}
	}
	if err := os.WriteFile(dest, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(from, dest, false); !errors.Is(err, ErrExists) {
		t.Errorf("err = %v", err)
	}
	got, _ = os.ReadFile(dest)
	if string(got) != "mine" {
		t.Error("an existing config must not be replaced without --force")
	}
	if _, err := Import(from, dest, true); err != nil {
		t.Errorf("forced: %v", err)
	}
	for name, bad := range map[string]string{"parse": "[x", "empty": "theme = \"nord\"\n", "no url": "[profiles.a]\ntoken = \"t\"\n"} {
		p := filepath.Join(dir, name+".toml")
		if err := os.WriteFile(p, []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Import(p, filepath.Join(dir, name+"-out.toml"), false); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := Import(filepath.Join(dir, "missing"), dest, true); err == nil {
		t.Error("missing source accepted")
	}
}
