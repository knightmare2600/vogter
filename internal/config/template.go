package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Template is the starter config written by `vaegter init`.
const Template = `# Vægter: a terminal UI for Zabbix
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

# Scroll with the mouse wheel. Off by default, because taking the mouse stops
# click-and-drag text selection in most terminals. M toggles it while running.
mouse = false

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

// WriteTemplate writes Template to path (DefaultPath() when empty). It never
// replaces an existing file unless force is set.
func WriteTemplate(path string, force bool) (string, error) {
	if path == "" {
		path = DefaultPath()
	}
	if err := writeNew(path, []byte(Template), force); err != nil {
		if errors.Is(err, ErrExists) {
			return path, fmt.Errorf("%s %w (use --force to overwrite)", path, ErrExists)
		}
		return path, err
	}
	return path, nil
}

// writeNew writes data with mode 0600 through a temporary file, so a failure
// cannot leave a truncated file behind. It returns ErrExists rather than
// replace a file unless force is set.
func writeNew(path string, data []byte, force bool) error {
	if _, err := os.Stat(path); err == nil && !force {
		return ErrExists
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
