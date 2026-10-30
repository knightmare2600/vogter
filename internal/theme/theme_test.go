package theme

import "testing"

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
