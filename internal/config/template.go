package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Template is the starter config written by `vogter init`.
const Template = `# Vogter: a terminal UI for Zabbix
#
# Everything here is optional except a profile with a url and some way to
# authenticate. VOGTER_URL and VOGTER_TOKEN override the active profile,
# handy for one-off sessions.

# Profile used when --profile is not given.
default_profile = "local"

# "auto" follows your current Omarchy theme (and live-reloads when you switch
# it), falling back to tokyo-night everywhere else. Built-ins: tokyo-night,
# catppuccin, gruvbox, nord, everforest, rose-pine, kanagawa, matte-black.
theme = "auto"

# Seconds between API polls (minimum 2).
refresh_interval = 10

[notifications]
# Desktop notifications (notify-send on Linux, Notification Center on macOS)
# for new and resolved problems and for hosts going down or coming back.
desktop = true
# Ignore anything below this: not_classified, information, warning,
# average, high, disaster.
min_severity = "average"

[profiles.local]
url = "http://localhost:8080"
# Create one in Zabbix: User settings -> API tokens.
token = "` + placeholder + `"

# Prefer not to keep secrets in plain text? Pick one of these instead:
# token_env = "ZABBIX_TOKEN"
# token_cmd = "pass show zabbix/local"      # or: op read op://vault/zabbix/token
#
# Or classic username/password (a session is opened with user.login):
# username = "Admin"
# password = "zabbix"

# Self-signed certificate on the frontend?
# verify_tls = false

# [profiles.prod]
# url = "https://zabbix.example.com"
# token_cmd = "op read op://ops/zabbix-prod/token"
`

// ErrExists is returned by WriteTemplate when the file is already there and
// force is false.
var ErrExists = errors.New("already exists")

// WriteTemplate writes Template to path (DefaultPath() when empty) with mode
// 0600. It never replaces an existing file unless force is set, and it
// writes to a temporary file first so a failure cannot leave a truncated
// config behind.
func WriteTemplate(path string, force bool) (string, error) {
	if path == "" {
		path = DefaultPath()
	}
	if _, err := os.Stat(path); err == nil && !force {
		return path, fmt.Errorf("%s %w (use --force to overwrite)", path, ErrExists)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return path, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return path, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return path, err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		tmp.Close()
		return path, err
	}
	if _, err := tmp.WriteString(Template); err != nil {
		tmp.Close()
		return path, err
	}
	if err := tmp.Close(); err != nil {
		return path, err
	}
	return path, os.Rename(tmp.Name(), path)
}
