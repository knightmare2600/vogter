// Command vogter is a keyboard-driven terminal UI for Zabbix, an
// independent Go port of zabterm.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/knightmare2600/vogter/internal/config"
	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/platform"
	"github.com/knightmare2600/vogter/internal/poller"
	"github.com/knightmare2600/vogter/internal/ui"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = ""

func appVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

const usage = `vogter: a terminal UI for Zabbix

Usage: vogter [OPTIONS] [COMMAND]

Commands:
  init    Write a starter config (default %s)
  check   Validate the config and test the active profile against Zabbix
  config  Print the config file path
  import  Copy an existing zabterm config into place

Options:
  -p, --profile <NAME>  Profile from the config file to use
  -c, --config <PATH>   Path to an alternative config file
  -t, --theme <THEME>   Override the theme for this run
      --force           With init or import: overwrite an existing file
      --from <PATH>     With import: the zabterm config (default %s)
  -h, --help            Print help
  -V, --version         Print version
`

type options struct {
	profile, configPath, theme, from string
	force, help, version             bool
}

func parseArgs(args []string, stderr io.Writer) (options, string, error) {
	var o options
	fs := flag.NewFlagSet("vogter", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	for _, n := range []string{"p", "profile"} {
		fs.StringVar(&o.profile, n, "", "")
	}
	for _, n := range []string{"c", "config"} {
		fs.StringVar(&o.configPath, n, "", "")
	}
	for _, n := range []string{"t", "theme"} {
		fs.StringVar(&o.theme, n, "", "")
	}
	for _, n := range []string{"h", "help"} {
		fs.BoolVar(&o.help, n, false, "")
	}
	for _, n := range []string{"V", "version"} {
		fs.BoolVar(&o.version, n, false, "")
	}
	fs.BoolVar(&o.force, "force", false, "")
	fs.StringVar(&o.from, "from", "", "")

	// Options may come before or after the command.
	if err := fs.Parse(args); err != nil {
		return o, "", err
	}
	cmd := fs.Arg(0)
	if cmd != "" {
		if err := fs.Parse(fs.Args()[1:]); err != nil {
			return o, "", err
		}
		if fs.NArg() > 0 {
			return o, "", fmt.Errorf("unexpected argument %q", fs.Arg(0))
		}
	}
	switch cmd {
	case "", "init", "check", "config", "import":
	default:
		return o, "", fmt.Errorf("unknown command %q", cmd)
	}
	if o.force && cmd != "init" && cmd != "import" {
		return o, "", errors.New("--force only applies to init and import")
	}
	if o.from != "" && cmd != "import" {
		return o, "", errors.New("--from only applies to import")
	}
	return o, cmd, nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	o, cmd, err := parseArgs(args, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "\x1b[31m✕\x1b[0m %v\n\n", err)
		fmt.Fprintf(stderr, usage, config.DefaultPath(), config.ZabtermPath())
		return 2
	}
	if o.help {
		fmt.Fprintf(stdout, usage, config.DefaultPath(), config.ZabtermPath())
		return 0
	}
	if o.version {
		fmt.Fprintln(stdout, "vogter", appVersion())
		return 0
	}
	path := o.configPath
	if path == "" {
		path = config.DefaultPath()
	}

	switch cmd {
	case "config":
		fmt.Fprintln(stdout, path)
		return 0
	case "init":
		p, err := config.WriteTemplate(path, o.force)
		if err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintf(stdout, "\x1b[32m✓\x1b[0m wrote %s\n  add your Zabbix URL and API token, then run \x1b[1mvogter check\x1b[0m\n", p)
		return 0
	case "check":
		return check(o, path, stdout, stderr)
	case "import":
		from := o.from
		if from == "" {
			from = config.ZabtermPath()
		}
		names, err := config.Import(from, path, o.force)
		if err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintf(stdout, "\x1b[32m✓\x1b[0m imported profiles %s from %s\n  into %s\n  run \x1b[1mvogter check\x1b[0m to test them\n",
			strings.Join(names, ", "), from, path)
		return 0
	}
	return tui(o, path, stderr)
}

func fail(w io.Writer, err error) int {
	fmt.Fprintf(w, "\x1b[31m✕\x1b[0m %v\n", err)
	return 1
}

func loadProfile(o options, path string, stderr io.Writer, showIssues bool) (*config.Config, *config.Profile, error) {
	c, issues, err := config.Load(path)
	if showIssues || err != nil {
		for _, i := range issues {
			fmt.Fprintf(stderr, "  \x1b[33m%s\x1b[0m\n", i)
		}
	}
	if err != nil {
		return nil, nil, err
	}
	p, err := c.Profile(o.profile)
	return c, p, err
}

func newClient(p *config.Profile) *zabbix.Client {
	return zabbix.New(zabbix.Options{URL: p.APIURL(), VerifyTLS: p.TLSVerify(), UserAgent: "vogter/" + appVersion()})
}

// check validates the config and runs the same calls as one poll.
func check(o options, path string, stdout, stderr io.Writer) int {
	c, p, err := loadProfile(o, path, stderr, true)
	if err != nil {
		return fail(stderr, err)
	}
	src := path
	if c.Path == "" {
		src += " (not found; using " + config.EnvURL + ")"
	}
	fmt.Fprintf(stdout, "  config   %s\n  profile  %s\n  endpoint %s\n", src, p.Name, p.APIURL())
	if !p.TLSVerify() {
		fmt.Fprintln(stdout, "  tls      certificate verification OFF")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	auth, err := p.ResolveAuth(ctx)
	if err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "  auth     %s\n", auth.Source)

	cl := newClient(p)
	raw, err := cl.APIVersion(ctx)
	if err != nil {
		return failAPI(stderr, err)
	}
	v, err := zabbix.ParseVersion(raw)
	if err != nil {
		return fail(stderr, err)
	}
	support := zabbix.SupportFor(v)
	if support == zabbix.Unsupported {
		return fail(stderr, errors.New(support.Describe(v)))
	}
	if auth.IsLogin() {
		if err := cl.Login(ctx, auth.Username, auth.Password.Reveal()); err != nil {
			return failAPI(stderr, err)
		}
		defer func() {
			lctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = cl.Logout(lctx)
		}()
	} else {
		cl.SetToken(auth.Token.Reveal())
	}
	hosts, err := cl.Hosts(ctx)
	if err != nil {
		return failAPI(stderr, err)
	}
	problems, err := cl.Problems(ctx)
	if err != nil {
		return failAPI(stderr, err)
	}
	more := ""
	if len(problems) == zabbix.ProblemLimit {
		more = "+"
	}
	if note := support.Describe(v); note != "" {
		fmt.Fprintf(stdout, "  \x1b[33mwarning: %s\x1b[0m\n", note)
	}
	fmt.Fprintf(stdout, "\x1b[32m✓\x1b[0m Zabbix %s · %d hosts · %d%s active problems\n", v, len(hosts), len(problems), more)
	return 0
}

func failAPI(w io.Writer, err error) int {
	hint := map[zabbix.Kind]string{
		zabbix.KindTransport:  "cannot reach the server: check url, DNS, firewall and certificates",
		zabbix.KindHTTP:       "the web server answered with an error: check url points at the Zabbix frontend",
		zabbix.KindDecode:     "the response is not the Zabbix API: check url points at the Zabbix frontend",
		zabbix.KindAuth:       "credentials rejected: the token may be expired, revoked or disabled",
		zabbix.KindPermission: "the account lacks permission for this call",
		zabbix.KindShape:      "unexpected response from Zabbix: please report your Zabbix version",
	}[zabbix.KindOf(err)]
	fmt.Fprintf(w, "\x1b[31m✕\x1b[0m %v\n", err)
	if hint != "" {
		fmt.Fprintf(w, "  %s\n", hint)
	}
	return 1
}

func colourProfile() []tea.ProgramOption {
	switch os.Getenv(config.EnvTruecolor) {
	case "1":
		return []tea.ProgramOption{tea.WithColorProfile(colorprofile.TrueColor)}
	case "0":
		return []tea.ProgramOption{tea.WithColorProfile(colorprofile.ANSI256)}
	}
	return nil
}

func tui(o options, path string, stderr io.Writer) int {
	c, p, err := loadProfile(o, path, stderr, false)
	if err != nil {
		return fail(stderr, err)
	}
	themeName := c.Theme
	if o.theme != "" {
		themeName = o.theme
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Fail fast on missing credentials instead of inside the TUI.
	auth, err := p.ResolveAuth(ctx)
	if err != nil {
		return fail(stderr, err)
	}

	interval := time.Duration(c.RefreshInterval) * time.Second
	events := make(chan poller.Event)
	cmds := make(chan poller.Cmd, 1)
	pl := poller.New(newClient(p), auth, interval, ui.PollerFmt)
	done := make(chan struct{})
	go func() {
		defer close(done)
		pl.Run(ctx, cmds, events)
	}()

	sev, _ := config.ParseSeverity(c.Notifications.MinSeverity)
	minSev := model.Severity(sev)
	m := ui.New(ui.Options{
		Profile:     p.Name,
		Endpoint:    p.APIURL(),
		ConfigPath:  path,
		Theme:       themeName,
		Interval:    interval,
		Events:      events,
		Cmds:        cmds,
		Ack:         pl.Acknowledge,
		Watch:       pl.Watch,
		WebURL:      p.WebURL(),
		Desktop:     c.Notifications.Desktop,
		MinSeverity: minSev,
		Notify:      platform.Notify,
		Open:        platform.OpenURL,
	})
	_, err = tea.NewProgram(m, colourProfile()...).Run()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
	if err != nil {
		return fail(stderr, err)
	}
	return 0
}
