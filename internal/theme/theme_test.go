package theme

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestHex(t *testing.T) {
	for _, in := range []string{"#7aa2f7", "7aa2f7", "0x7aa2f7", `"#7aa2f7"`, "'#7aa2f7ff'"} {
		if c, err := Hex(in); err != nil || c != (RGB{0x7a, 0xa2, 0xf7}) {
			t.Errorf("Hex(%q) = %v, %v", in, c, err)
		}
	}
	for _, bad := range []string{"", "#fff", "#zzzzzz"} {
		if _, err := Hex(bad); err == nil {
			t.Errorf("Hex(%q) accepted", bad)
		}
	}
}

func TestMixRoundsLikeRust(t *testing.T) {
	// 0 + (255-0)*0.5 = 127.5 rounds away from zero to 128 in Rust.
	if got := (RGB{0, 0, 0}).Mix(RGB{255, 255, 255}, 0.5); got != (RGB{128, 128, 128}) {
		t.Errorf("Mix = %v", got)
	}
	if got := (RGB{10, 20, 30}).Mix(RGB{10, 20, 30}, 0.7); got != (RGB{10, 20, 30}) {
		t.Errorf("Mix of equal colours = %v", got)
	}
}

func TestDerivedRoles(t *testing.T) {
	th := Load("tokyo-night")
	p := builtins["tokyo-night"]
	// muted = fg.mix(bg, .45): c0caf5 → 1a1b26
	want := RGB{
		uint8(roundHalfAway(0xc0 + (0x1a-0xc0)*0.45)),
		uint8(roundHalfAway(0xca + (0x1b-0xca)*0.45)),
		uint8(roundHalfAway(0xf5 + (0x26-0xf5)*0.45)),
	}
	if th.Muted != want {
		t.Errorf("Muted = %v, want %v", th.Muted, want)
	}
	if th.Sev[5] != p.Red || th.Sev[1] != p.Blue || th.Series[0] != p.Accent {
		t.Error("severity or series roles wrong")
	}
	if th.Sev[3] != p.Yellow.Mix(p.Red, 0.4) || th.Sev[4] != p.Yellow.Mix(p.Red, 0.75) {
		t.Error("average/high severities wrong")
	}
}

func roundHalfAway(f float64) float64 {
	if f < 0 {
		return -roundHalfAway(-f)
	}
	return float64(int(f + 0.5))
}

func TestLoadFallbackAndCycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if Load("auto").Name != "tokyo-night" || Load("nonsense").Name != "tokyo-night" {
		t.Error("unknown themes must fall back to tokyo-night")
	}
	for _, n := range Builtin {
		if Load(n).Name != n {
			t.Errorf("Load(%q) lost its name", n)
		}
	}
	name := Builtin[0]
	for range Builtin {
		name = Next(name)
	}
	if name != Builtin[0] {
		t.Error("cycling all themes must return to the start")
	}
}

const colors4 = `
mode = "dark"
accent = "#7aa2f7"
muted = "#414868"
background = "#1a1b26"
foreground = "#a9b1d6"
red = "#f7768e"
yellow = "#e0af68"
green = "#9ece6a"
cyan = "#449dab"
blue = "#7aa2f7"
magenta = "#ad8ee6"
`

func colors38() string {
	s := "accent = \"#7aa2f7\"\nbackground = \"#1a1b26\"\nforeground = \"#a9b1d6\"\n"
	for i := 0; i < 16; i++ {
		s += fmt.Sprintf("color%d = \"#%02x%02x%02x\"\n", i, i*10, i*10, i*10)
	}
	return s
}

const alacritty = `
[colors.primary]
background = '#1a1b26'
foreground = '#a9b1d6'
[colors.normal]
black = '#32344a'
red = '#f7768e'
green = '#9ece6a'
yellow = '#e0af68'
blue = '#7aa2f7'
magenta = '#ad8ee6'
cyan = '#449dab'
white = '#787c99'
[colors.bright]
black = '#444b6a'
`

func TestParseThemeFiles(t *testing.T) {
	p, ok := parseColorsTOML(colors4)
	if !ok || p.Red != (RGB{0xf7, 0x76, 0x8e}) || p.BrightBlack != (RGB{0x41, 0x48, 0x68}) || p.Accent != (RGB{0x7a, 0xa2, 0xf7}) {
		t.Errorf("omarchy 4: %+v %v", p, ok)
	}
	// Vectors from zabterm's theme tests.
	p, ok = parseColorsTOML(colors38())
	if !ok || p.Accent != (RGB{0x7a, 0xa2, 0xf7}) || p.BrightBlack != (RGB{80, 80, 80}) {
		t.Errorf("omarchy 3.8: %+v %v", p, ok)
	}
	p, ok = parseAlacritty(alacritty)
	if !ok || p.Red != (RGB{0xf7, 0x76, 0x8e}) || p.BrightBlack != (RGB{0x44, 0x4b, 0x6a}) {
		t.Errorf("alacritty: %+v %v", p, ok)
	}
	for _, bad := range []string{"", "not toml [", `background = "#000000"`} {
		if _, ok := parseColorsTOML(bad); ok {
			t.Errorf("accepted %q", bad)
		}
		if _, ok := parseAlacritty(bad); ok {
			t.Errorf("accepted %q", bad)
		}
	}
}

func installTheme(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	themeDir := filepath.Join(home, filepath.FromSlash(dir), "theme")
	if err := os.MkdirAll(themeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for f, body := range files {
		if err := os.WriteFile(filepath.Join(themeDir, f), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(home, filepath.FromSlash(dir))
}

func TestAutoOmarchy4(t *testing.T) {
	dir := installTheme(t, ".local/state/omarchy/current", map[string]string{"colors.toml": colors4})
	if err := os.WriteFile(filepath.Join(dir, "theme.name"), []byte("tokyo-night\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	th := Load("auto")
	if th.Name != "omarchy:tokyo-night" {
		t.Errorf("name = %q", th.Name)
	}
	if th.Accent != (RGB{0x7a, 0xa2, 0xf7}) || th.Err != (RGB{0xf7, 0x76, 0x8e}) {
		t.Errorf("%+v", th)
	}
	if Next(th.Name) != Builtin[0] {
		t.Errorf("next after omarchy = %q", Next(th.Name))
	}
	if Next(Builtin[len(Builtin)-1]) != "auto" {
		t.Error("the cycle must include auto with Omarchy present")
	}
}

func TestAutoOmarchy3(t *testing.T) {
	installTheme(t, ".config/omarchy/current", map[string]string{"alacritty.toml": alacritty})
	if th := Load("omarchy"); th.Name != "omarchy:theme" {
		t.Errorf("name = %q", th.Name)
	}
}

func TestAutoWithoutOmarchy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if Load("auto").Name != "tokyo-night" {
		t.Error("no Omarchy means tokyo-night")
	}
	if OmarchySignature() != (Signature{}) {
		t.Error("no Omarchy means an empty signature")
	}
	if Next(Builtin[len(Builtin)-1]) != Builtin[0] {
		t.Error("the cycle must skip auto without Omarchy")
	}
}

func TestBrokenThemeFallsBack(t *testing.T) {
	installTheme(t, ".local/state/omarchy/current", map[string]string{"colors.toml": "garbage ["})
	if Load("auto").Name != "tokyo-night" {
		t.Error("an unreadable theme must fall back")
	}
}

func TestSignatureChanges(t *testing.T) {
	dir := installTheme(t, ".local/state/omarchy/current", map[string]string{"colors.toml": colors4})
	a := OmarchySignature()
	if err := os.WriteFile(filepath.Join(dir, "theme.name"), []byte("nord"), 0o600); err != nil {
		t.Fatal(err)
	}
	if OmarchySignature() == a {
		t.Error("a theme switch must change the signature")
	}
}
