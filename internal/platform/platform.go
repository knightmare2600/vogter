// Package platform wraps the operating-system integrations: desktop
// notifications and opening a browser. All of it runs helper programs,
// detached from the terminal so they cannot disturb the UI.
package platform

import (
	"errors"
	"os/exec"
	"runtime"
)

// ErrUnsupported means the platform has no desktop notifications.
var ErrUnsupported = errors.New("desktop notifications are not supported on this platform")

func start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap it, so it does not linger as a zombie.
	go func() { _ = cmd.Wait() }()
	return nil
}

// notifyCommand builds the notifier for goos, or nil if there is none.
func notifyCommand(goos, title, body string, urgent bool) []string {
	switch goos {
	case "darwin":
		// Passing the text as arguments avoids quoting it into the script.
		return []string{"osascript", "-e", "on run argv", "-e",
			`display notification (item 2 of argv) with title "vogter" subtitle (item 1 of argv)`,
			"-e", "end run", title, body}
	case "windows":
		return nil
	}
	urgency := "normal"
	if urgent {
		urgency = "critical"
	}
	return []string{"notify-send", "-a", "vogter", "-u", urgency, title, body}
}

// Notify sends a desktop notification. A missing helper is an error the
// caller can ignore, as the in-app toast has already fired.
func Notify(title, body string, urgent bool) error {
	argv := notifyCommand(runtime.GOOS, title, body, urgent)
	if argv == nil {
		return ErrUnsupported
	}
	return start(argv[0], argv[1:]...)
}

func openCommand(goos, url string) []string {
	switch goos {
	case "darwin":
		return []string{"open", url}
	case "windows":
		// Not cmd /c start, which would need & in URLs quoted.
		return []string{"rundll32", "url.dll,FileProtocolHandler", url}
	}
	return []string{"xdg-open", url}
}

// OpenURL opens url in the default browser.
func OpenURL(url string) error {
	argv := openCommand(runtime.GOOS, url)
	return start(argv[0], argv[1:]...)
}
