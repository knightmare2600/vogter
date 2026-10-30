// Package config loads, validates and resolves Vogter's configuration: a
// TOML file with a few global settings and one or more named Zabbix
// profiles. The schema matches zabterm's so existing files import unchanged.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// MinRefresh is the shortest poll interval, in seconds. Shorter values are
// raised to it (zabterm does the same).
const MinRefresh = 2

// Themes accepted by the theme setting, besides "auto" and "omarchy".
var Themes = []string{"tokyo-night", "catppuccin", "gruvbox", "nord", "everforest", "rose-pine", "kanagawa", "matte-black"}

// Config is the whole file.
type Config struct {
	DefaultProfile  string              `toml:"default_profile"`
	Theme           string              `toml:"theme"`
	RefreshInterval int                 `toml:"refresh_interval"`
	Notifications   Notifications       `toml:"notifications"`
	Profiles        map[string]*Profile `toml:"profiles"`

	// Path is the file the config was read from; empty when none existed.
	Path string `toml:"-"`
}

// Notifications controls toasts and desktop notifications.
type Notifications struct {
	Desktop     bool   `toml:"desktop"`
	MinSeverity string `toml:"min_severity"`
}

// Profile describes one Zabbix server.
type Profile struct {
	Name      string `toml:"-"`
	URL       string `toml:"url"`
	Token     Secret `toml:"token"`
	TokenEnv  string `toml:"token_env"`
	TokenCmd  string `toml:"token_cmd"`
	Username  string `toml:"username"`
	Password  Secret `toml:"password"`
	VerifyTLS *bool  `toml:"verify_tls"`
}

// TLSVerify reports whether certificates are checked; true unless the
// profile sets verify_tls = false.
func (p *Profile) TLSVerify() bool {
	return p.VerifyTLS == nil || *p.VerifyTLS
}

// APIURL is the JSON-RPC endpoint, whether the user wrote the frontend URL
// or the full api_jsonrpc.php path.
func (p *Profile) APIURL() string {
	u := strings.TrimRight(p.URL, "/")
	if strings.HasSuffix(u, "api_jsonrpc.php") {
		return u
	}
	return u + "/api_jsonrpc.php"
}

// WebURL is the frontend base URL, used for "open in browser".
func (p *Profile) WebURL() string {
	u := strings.TrimRight(p.URL, "/")
	u = strings.TrimSuffix(u, "api_jsonrpc.php")
	return strings.TrimRight(u, "/")
}

func defaults() Config {
	return Config{
		Theme:           "auto",
		RefreshInterval: 10,
		Notifications:   Notifications{Desktop: true, MinSeverity: "average"},
	}
}

// Issue is a validation finding. Errors stop Vogter from starting; warnings
// are reported by `vogter check` and otherwise ignored.
type Issue struct {
	Error bool
	Msg   string
}

func (i Issue) String() string {
	if i.Error {
		return "error: " + i.Msg
	}
	return "warning: " + i.Msg
}

// ErrNoProfiles is returned when neither a config file nor VOGTER_URL
// provides a profile.
var ErrNoProfiles = errors.New("no profiles configured")

// Load reads the config at path (DefaultPath() when empty). A missing file is
// not an error by itself: VOGTER_URL alone is enough to run. The returned
// issues include unknown keys and out-of-range values.
func Load(path string) (*Config, []Issue, error) {
	if path == "" {
		path = DefaultPath()
	}
	c := defaults()
	var issues []Issue

	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		md, err := toml.Decode(string(raw), &c)
		if err != nil {
			return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		for _, k := range md.Undecoded() {
			issues = append(issues, Issue{Msg: fmt.Sprintf("unknown setting %q", k.String())})
		}
		c.Path = path
	case errors.Is(err, fs.ErrNotExist):
	default:
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}

	for name, p := range c.Profiles {
		if p == nil {
			p = &Profile{}
			c.Profiles[name] = p
		}
		p.Name = name
	}
	// VOGTER_URL alone is enough to run without any config file at all.
	if u := os.Getenv(EnvURL); u != "" {
		if c.Profiles == nil {
			c.Profiles = map[string]*Profile{}
		}
		c.Profiles["env"] = &Profile{Name: "env", URL: u}
		c.DefaultProfile = "env"
	}
	if len(c.Profiles) == 0 {
		return nil, issues, fmt.Errorf("%w.\n\n  Run `vogter init` to write a starter config to %s\n  or export %s and %s",
			ErrNoProfiles, path, EnvURL, EnvToken)
	}
	issues = append(issues, c.validate()...)
	if c.Path != "" {
		issues = append(issues, c.permissionIssues()...)
	}
	for _, i := range issues {
		if i.Error {
			return nil, issues, fmt.Errorf("invalid config: %s", i.Msg)
		}
	}
	c.RefreshInterval = max(c.RefreshInterval, MinRefresh)
	return &c, issues, nil
}

func (c *Config) validate() []Issue {
	var out []Issue
	if !validTheme(c.Theme) {
		out = append(out, Issue{Msg: fmt.Sprintf("unknown theme %q, using tokyo-night (have: auto, %s)", c.Theme, strings.Join(Themes, ", "))})
	}
	if c.RefreshInterval < MinRefresh {
		out = append(out, Issue{Msg: fmt.Sprintf("refresh_interval %d is below the minimum, using %d", c.RefreshInterval, MinRefresh)})
	}
	if _, ok := ParseSeverity(c.Notifications.MinSeverity); !ok {
		out = append(out, Issue{Msg: fmt.Sprintf("unknown notifications.min_severity %q, using average", c.Notifications.MinSeverity)})
	}
	if c.DefaultProfile != "" && c.Profiles[c.DefaultProfile] == nil {
		out = append(out, Issue{Error: true, Msg: fmt.Sprintf("default_profile %q does not exist (have: %s)", c.DefaultProfile, strings.Join(c.ProfileNames(), ", "))})
	}
	for _, name := range c.ProfileNames() {
		p := c.Profiles[name]
		if strings.TrimSpace(p.URL) == "" {
			out = append(out, Issue{Error: true, Msg: fmt.Sprintf("profile %q has no url", name)})
		}
		if n := p.credentialSources(); n > 1 {
			out = append(out, Issue{Msg: fmt.Sprintf("profile %q sets %d credential sources; the first in order token_env, token_cmd, token, username/password wins", name, n)})
		}
		if (p.Username == "") != p.Password.Empty() {
			out = append(out, Issue{Msg: fmt.Sprintf("profile %q sets only one of username and password", name)})
		}
		if !p.TLSVerify() {
			out = append(out, Issue{Msg: fmt.Sprintf("profile %q has TLS certificate verification turned off", name)})
		}
	}
	return out
}

func (p *Profile) credentialSources() int {
	n := 0
	for _, set := range []bool{p.TokenEnv != "", p.TokenCmd != "", !p.Token.Empty() && !p.Token.isPlaceholder(), p.Username != "" && !p.Password.Empty()} {
		if set {
			n++
		}
	}
	return n
}

func validTheme(name string) bool {
	if name == "auto" || name == "omarchy" {
		return true
	}
	for _, t := range Themes {
		if t == name {
			return true
		}
	}
	return false
}

// ProfileNames returns profile names in sorted order.
func (c *Config) ProfileNames() []string {
	names := make([]string, 0, len(c.Profiles))
	for n := range c.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Profile picks the named profile, else default_profile, else the first one
// in sorted order (zabterm's BTreeMap order).
func (c *Config) Profile(name string) (*Profile, error) {
	if name == "" {
		name = c.DefaultProfile
	}
	if name == "" {
		return c.Profiles[c.ProfileNames()[0]], nil
	}
	p := c.Profiles[name]
	if p == nil {
		return nil, fmt.Errorf("profile %q not found (have: %s)", name, strings.Join(c.ProfileNames(), ", "))
	}
	return p, nil
}

// ParseSeverity accepts the names zabterm accepts: full names and short
// forms, any case, with '-' or ' ' for '_'. It returns the Zabbix code 0–5.
func ParseSeverity(name string) (int, bool) {
	n := strings.NewReplacer("-", "_", " ", "_").Replace(strings.ToLower(name))
	switch n {
	case "not_classified", "nc":
		return 0, true
	case "information", "info":
		return 1, true
	case "warning", "warn":
		return 2, true
	case "average", "avg":
		return 3, true
	case "high":
		return 4, true
	case "disaster":
		return 5, true
	}
	return 3, false
}
