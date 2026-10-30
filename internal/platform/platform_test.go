package platform

import (
	"slices"
	"testing"
)

func TestNotifyCommand(t *testing.T) {
	got := notifyCommand("linux", "T", "B", true)
	if !slices.Equal(got, []string{"notify-send", "-a", "vogter", "-u", "critical", "T", "B"}) {
		t.Errorf("%v", got)
	}
	if got := notifyCommand("linux", "T", "B", false); got[4] != "normal" {
		t.Errorf("%v", got)
	}
	hostile := `a" & do shell script "rm -rf ~`
	mac := notifyCommand("darwin", hostile, "b", false)
	if mac[0] != "osascript" || mac[len(mac)-2] != hostile {
		t.Errorf("%v", mac)
	}
	for _, a := range mac[:len(mac)-2] {
		if a == hostile {
			t.Error("text must not be part of the script (D11)")
		}
	}
	if notifyCommand("windows", "T", "B", false) != nil {
		t.Error("windows has no notifier")
	}
}

func TestOpenCommand(t *testing.T) {
	const u = "https://z/zabbix.php?a=1&b=2"
	for goos, want := range map[string]string{"linux": "xdg-open", "darwin": "open", "windows": "rundll32"} {
		got := openCommand(goos, u)
		if got[0] != want || got[len(got)-1] != u {
			t.Errorf("%s: %v", goos, got)
		}
	}
}
