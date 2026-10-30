package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// Environment variables Vogter reads.
const (
	EnvURL       = "VOGTER_URL"
	EnvToken     = "VOGTER_TOKEN"
	EnvTruecolor = "VOGTER_TRUECOLOR"
	EnvASCII     = "VOGTER_ASCII"
)

// Home is the user's home directory, or "." when it cannot be determined.
func Home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

// Dir is the config directory: $XDG_CONFIG_HOME/vogter when that is an
// absolute path, else ~/.config/vogter on Linux and macOS (zabterm's
// convention, deliberately not ~/Library on macOS), and %AppData%\vogter on
// Windows.
func Dir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(x) {
		return filepath.Join(x, "vogter")
	}
	if runtime.GOOS == "windows" {
		if d, err := os.UserConfigDir(); err == nil {
			return filepath.Join(d, "vogter")
		}
	}
	return filepath.Join(Home(), ".config", "vogter")
}

// DefaultPath is the config file used when -c is not given.
func DefaultPath() string {
	return filepath.Join(Dir(), "config.toml")
}

// Tilde shortens a path under the home directory to ~/...
func Tilde(path string) string {
	home := Home()
	if rel, err := filepath.Rel(home, path); err == nil && rel != "." && !filepath.IsAbs(rel) && rel[0] != '.' {
		return "~" + string(filepath.Separator) + rel
	}
	return path
}
