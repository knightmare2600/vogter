package theme

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// currentDirs are the places Omarchy keeps the active theme, newest first.
// Omarchy 4 moved it to ~/.local/state and removes the old directory, which
// zabterm 0.1.2 still looks in (D4).
var currentDirs = []string{".local/state/omarchy/current", ".config/omarchy/current"}

func omarchyDir() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	for _, d := range currentDirs {
		dir := filepath.Join(home, filepath.FromSlash(d))
		if st, err := os.Stat(filepath.Join(dir, "theme")); err == nil && st.IsDir() {
			return dir, true
		}
	}
	return "", false
}

// OmarchyPresent reports whether an active Omarchy theme exists.
func OmarchyPresent() bool {
	_, ok := omarchyDir()
	return ok
}

// omarchyName prefers Omarchy's theme.name file. In Omarchy 4 the theme
// directory is a real directory called "theme", so its name says nothing
// (D5). Older releases point it at the theme by symlink.
func omarchyName(dir string) string {
	if raw, err := os.ReadFile(filepath.Join(dir, "theme.name")); err == nil {
		if n := strings.TrimSpace(string(raw)); n != "" {
			return n
		}
	}
	if real, err := filepath.EvalSymlinks(filepath.Join(dir, "theme")); err == nil {
		return filepath.Base(real)
	}
	return "theme"
}

func loadOmarchy() (string, Palette, bool) {
	dir, ok := omarchyDir()
	if !ok {
		return "", Palette{}, false
	}
	themeDir := filepath.Join(dir, "theme")
	if raw, err := os.ReadFile(filepath.Join(themeDir, "colors.toml")); err == nil {
		if p, ok := parseColorsTOML(string(raw)); ok {
			return omarchyName(dir), p, true
		}
	}
	if raw, err := os.ReadFile(filepath.Join(themeDir, "alacritty.toml")); err == nil {
		if p, ok := parseAlacritty(string(raw)); ok {
			return omarchyName(dir), p, true
		}
	}
	return "", Palette{}, false
}

// parseColorsTOML reads Omarchy's colors.toml in either layout: named keys
// (Omarchy 4) or color0 to color15 (late 3.x).
func parseColorsTOML(raw string) (Palette, bool) {
	var t map[string]any
	if _, err := toml.Decode(raw, &t); err != nil {
		return Palette{}, false
	}
	get := func(k string) (RGB, bool) {
		s, ok := t[k].(string)
		if !ok {
			return RGB{}, false
		}
		c, err := Hex(s)
		return c, err == nil
	}
	bg, ok1 := get("background")
	fg, ok2 := get("foreground")
	if !ok1 || !ok2 {
		return Palette{}, false
	}
	keys := []string{"red", "green", "yellow", "blue", "magenta", "cyan", "muted"} // as Omarchy's Alacritty template maps them
	if _, named := get("red"); !named {
		keys = []string{"color1", "color2", "color3", "color4", "color5", "color6", "color8"}
	}
	vals := make([]RGB, len(keys))
	for i, k := range keys {
		v, ok := get(k)
		if !ok {
			return Palette{}, false
		}
		vals[i] = v
	}
	accent, ok := get("accent")
	if !ok {
		accent = vals[3]
	}
	return Palette{BG: bg, FG: fg, Red: vals[0], Green: vals[1], Yellow: vals[2], Blue: vals[3], Magenta: vals[4], Cyan: vals[5], BrightBlack: vals[6], Accent: accent}, true
}

func parseAlacritty(raw string) (Palette, bool) {
	var t struct {
		Colors map[string]map[string]string `toml:"colors"`
	}
	if _, err := toml.Decode(raw, &t); err != nil {
		return Palette{}, false
	}
	get := func(section, k string) (RGB, bool) {
		c, err := Hex(t.Colors[section][k])
		return c, err == nil
	}
	blue, ok := get("normal", "blue")
	if !ok {
		return Palette{}, false
	}
	p := Palette{Blue: blue, Accent: blue}
	for _, f := range []struct {
		dst        *RGB
		section, k string
	}{
		{&p.BG, "primary", "background"}, {&p.FG, "primary", "foreground"},
		{&p.Red, "normal", "red"}, {&p.Green, "normal", "green"}, {&p.Yellow, "normal", "yellow"},
		{&p.Magenta, "normal", "magenta"}, {&p.Cyan, "normal", "cyan"},
	} {
		v, ok := get(f.section, f.k)
		if !ok {
			return Palette{}, false
		}
		*f.dst = v
	}
	p.BrightBlack = blue
	if v, ok := get("bright", "black"); ok {
		p.BrightBlack = v
	}
	return p, true
}

// Signature is a cheap fingerprint of the active Omarchy theme, compared
// to follow theme switches. It is empty without Omarchy.
type Signature struct {
	Dir  string
	Time time.Time
	Name string
}

// OmarchySignature reads the current fingerprint.
func OmarchySignature() Signature {
	dir, ok := omarchyDir()
	if !ok {
		return Signature{}
	}
	s := Signature{Dir: dir, Name: omarchyName(dir)}
	for _, f := range []string{"colors.toml", "alacritty.toml"} {
		if st, err := os.Stat(filepath.Join(dir, "theme", f)); err == nil {
			s.Time = st.ModTime()
			break
		}
	}
	return s
}
