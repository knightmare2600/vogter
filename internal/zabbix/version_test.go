package zabbix

import "testing"

func TestParseVersion(t *testing.T) {
	cases := map[string][3]int{"7.0.12": {7, 0, 12}, "7.4": {7, 4, 0}, "8.0.0rc3": {8, 0, 0}, " 6.4.21 ": {6, 4, 21}}
	for in, want := range cases {
		v, err := ParseVersion(in)
		if err != nil || [3]int{v.Major, v.Minor, v.Patch} != want {
			t.Errorf("ParseVersion(%q) = %+v, %v", in, v, err)
		}
	}
	for _, bad := range []string{"", "seven", "7", "x.1"} {
		if _, err := ParseVersion(bad); err == nil {
			t.Errorf("ParseVersion(%q) accepted", bad)
		}
	}
}

func TestSupportFor(t *testing.T) {
	cases := map[string]Support{"7.0.0": Supported, "7.4.2": Supported, "8.0.0rc3": Supported, "6.4.9": Untested, "6.0.30": Unsupported, "5.4.0": Unsupported}
	for in, want := range cases {
		v, _ := ParseVersion(in)
		if got := SupportFor(v); got != want {
			t.Errorf("SupportFor(%s) = %v, want %v", in, got, want)
		}
		if (want == Supported) != (want.Describe(v) == "") {
			t.Errorf("Describe(%s) = %q", in, want.Describe(v))
		}
	}
}
