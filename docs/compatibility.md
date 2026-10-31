# Compatibility with zabterm

Where Vægter deliberately differs from zabterm 0.1.2. Defect IDs refer to `porting-audit.md` §13.
Updated as each milestone lands.

## Configuration and CLI

| Area | zabterm | Vægter |
|---|---|---|
| Config path | `~/.config/zabterm/config.toml` | `~/.config/vogter/config.toml` (`%AppData%\vogter` on Windows) |
| Environment | `ZABTERM_URL`, `ZABTERM_TOKEN`, `ZABTERM_TRUECOLOR` | `VOGTER_URL`, `VOGTER_TOKEN`, `VOGTER_TRUECOLOR`; the `ZABTERM_*` variables are not read |
| Schema | — | Identical keys and meaning |
| `init -c PATH` | Ignores `-c` (D15) | Writes to `PATH`, atomically, mode 0600 |
| Empty token value | Accepted (D14) | An empty `VOGTER_TOKEN` counts as unset; an empty `token_env` variable or empty `token_cmd` output is an error |
| Unknown keys, theme, severity | Silently ignored or defaulted (D17) | Same fallback values, plus a warning (shown by `vaegter check`) |
| Missing `default_profile` target, profile without `url` | Not reported at load (exact failure point unverified) | Error at load |
| Plain-text secret in a group/world-readable file | — | Warning with the `chmod` to fix it |
| `token_cmd` | `sh -c`, no time limit | `sh -c` (`cmd /C` on Windows), 2 minute limit |
| `refresh_interval` below 2 | Raised to 2, footer shows the configured value (D16) | Raised to 2 with a warning; footer shows 2 |
| `check` | Version, host and problem counts | Also auth source, TLS state, a version-support verdict, and a hint per error class |

## Zabbix API

| Area | zabterm | Vægter |
|---|---|---|
| Target version | "7.x and 8.0" | 7.0 LTS primary. 6.4 runs with an "untested" warning. Below 6.4 is refused |
| Start-up | Two polls (D1) | One poll |
| Concurrent refresh requests | Each queued and run | Coalesced into one follow-up poll |
| `user.login` sessions | Never closed (D3) | `user.logout` on quit and before re-login |
| Re-login trigger | Substring match on any error (D18) | Only when the error is classed as auth and the profile uses a session |
| Failed call with partial data | Fails the whole call | Same: a failed call never returns data |
| User-Agent | `zabterm/<ver>` | `vaegter/<ver>` |

## Screens

Compared with zabterm 0.1.2 on 2026-10-30 at 150×45: both binaries against `tools/mockzbx`, with the
same keys, using `tools/compare.sh` and `tools/cmpdiff.py`. The diff ignores the product name, clocks and
braille dots. Dashboard and hosts screens matched on every line, and problems screens differed only by the
`o web` hint. The host page matched except for chart data captured at a different moment. The 24h and 7d
charts differ by design (hourly trends). Polling requests matched exactly; host page requests differ as
described below. With the mock's `-flip` mode and a recording `notify-send`, both sent the same three
notifications with the same urgency.

| Area | zabterm | Vægter |
|---|---|---|
| Hosts table below 130 columns | Columns squeezed | Drops Groups, then Interface below 110 |
| Below 80×24 | No special handling (result not tested) | "terminal too small" notice |
| Over 500 problems (D9) | Count silently capped | Counts show `500+` |
| Error toasts | One text line, cut at 44 columns | Cause in the title (no permission, credentials rejected, server unreachable), error below |
| Bracketed paste | Not handled | Goes into the filter or ack message |
| Acknowledge (D20) | Queued behind a running poll | Runs at once on its own goroutine |
| Sparkline seed (D6, D7) | Failure never retried; float items only | Retried next poll; float and unsigned items |
| Sparkline buffers (D19) | Kept for removed items | Dropped |
| Host page fetches (D2) | Two per poll, each re-reading the whole range | One per poll, reading only samples newer than those held |
| 24h and 7d graphs | Raw `history.get` | Hourly `trend.get` averages plus raw history for the hours trends do not cover yet. An item without trends falls back to raw history |
| Host page and polls | Host page waits behind a running poll | Fetched on its own goroutine |
| Omarchy (D4, D5) | Only `~/.config/omarchy/current` and `colorN` keys, so Omarchy 4 silently gets tokyo-night | Also `~/.local/state/omarchy/current`, named keys and `theme.name` |
| Theme colour depth | Own nearest-256 mapping | Bubble Tea's colour-profile downsampling, so the 256-colour result can differ |
| "Resolved" past 500 problems (D10) | False "Resolved" notes | None while the problem list is truncated |
| macOS notification text (D11) | Quoted into AppleScript with Rust escapes | Passed as arguments (unverified: not run on macOS) |
| Notification app name | `zabterm` | `Vægter` |
| Windows notifications | No Windows build | One toast says they are unsupported; toasts continue |
| `vaegter import` | n/a | Validated copy of a zabterm config, comments kept, mode 0600, never overwrites without `--force` |
| Late detail for another host or range | Range mismatch shown briefly | Dropped |

## Parity with the audit checklist

Every row of the checklist in `porting-audit.md` §5 is implemented, apart from Z1. There is no `install.sh`
or Homebrew tap. Releases offer a bare binary, which prints its licences with `vaegter licences`, and an archive with the binary,
licences and a `.desktop` file.

## Platform support

| Platform | Built | Run | Status |
|---|---|---|---|
| Linux amd64 | yes | yes, against a fixture server | Supported |
| Linux arm64 | yes | no | Unverified |
| macOS amd64, arm64 | yes | no | Unverified |
| Windows amd64 | yes | no | Unverified: notifications are off by design |

`.github/workflows/ci.yml` runs the tests on Linux, macOS and Windows once the repository is on GitHub. It
has not run yet, and CI does not exercise a real terminal.

## Scrollbars and mouse

Vægter adds a scrollbar to any table that overflows; a table that fits looks as it does in zabterm. The mouse wheel is
an addition, off by default: `mouse = true` in the config or `M` while running. `Shift` plus the wheel pans a board
where the terminal passes that through.

## Zabbix dashboards

The `4 Boards` screen is new in Vægter; zabterm has no equivalent, so it adds a fourth tab to the header. It is read-only
and only calls the API while open. Read through `dashboard.get`, global dashboards only, on the 72-column grid. The layout
is always Zabbix's: a terminal narrower than 120 columns or too short for the page becomes a window onto it, with
scrollbars, `j k h l`, page up and down, and `ctrl-d ctrl-u`.

| Widget | Drawn as |
|---|---|
| `svggraph` | Braille chart. Item patterns and item lists are read. At most 12 series, the rest counted in the frame. Theme colours replace Zabbix's. `now-Nh` style periods only, otherwise 1h |
| `problems`, `problemsbysv` | Active problems, honouring `hostids` and `severities`. Other filters, such as groups and tags, are not applied |
| `clock` | Digital, always the local time of the terminal |
| `item` | Last value and description |
| `hostavail` | Interface counts of the chosen type |
| `tophosts` | Table by item name, sorted by the chosen column. Thresholds and bars are not drawn; columns that do not fit are dropped |
| `systeminfo` | Version and object counts. "Server is running" and required performance have no API |
| `geomap`, `map`, anything else | A placeholder pointing at the web page (`o`) |

Template dashboards are not read.

## Verified against a real server

Tested on Zabbix 7.0.32 with PostgreSQL and agent 2, using `tools/zabbix-lab.sh`:

- `vaegter check` with `token_cmd` and username/password; a bad token, bad password and wrong URL each gave the right hint.
- `go test -tags live ./internal/integration` passed.
- Dashboard, hosts, problems, latest data, acknowledge, and graphs at 1h, 24h and 7d (trends).
- Discovered `net.if` and `vfs.fs.dependent.size` keys, including a real ext4-backed `/data` mount in the filesystems
  panel. The agent's `/etc/*` container mounts were excluded as designed.
- `4 Boards` on the stock `Global view`, `Zabbix server health` and `Zabbix server` dashboards, at 150×45 and 80×24, including scrolling both ways.
- An expired password session: one poll showed offline, then Vægter logged in again unaided.
- Real wording of authentication and permission errors, from `host.get`, `user.login`, `event.acknowledge` and
  `host.create`, is classified correctly.

## Not verified

- Zabbix 6.4 and 8.0 have not run against a real server, including `4 Boards`.
- `svggraph` datasets that list item ids rather than patterns: unit-tested only, as the default dashboards use patterns.
- `tophosts` matches items by name, tested with one host. The lab's fed hosts use different item names.
- `dashboard.get` on an account with a large number of dashboards.
- Desktop notifications driven by a real server; they were tested on the mock.
- Omarchy 4 support is tested against files in its v4.0.6 format, not on an Omarchy machine.
- `osascript` argument passing has not run on macOS.
