# Compatibility with zabterm

Where Vogter deliberately differs from zabterm 0.1.2. Defect IDs refer to `porting-audit.md` §13.
Updated as each milestone lands.

## Configuration and CLI

| Area | zabterm | Vogter |
|---|---|---|
| Config path | `~/.config/zabterm/config.toml` | `~/.config/vogter/config.toml` (`%AppData%\vogter` on Windows) |
| Environment | `ZABTERM_URL`, `ZABTERM_TOKEN`, `ZABTERM_TRUECOLOR` | `VOGTER_URL`, `VOGTER_TOKEN`, `VOGTER_TRUECOLOR`; the `ZABTERM_*` variables are not read |
| Schema | — | Identical keys and meaning |
| `init -c PATH` | Ignores `-c` (D15) | Writes to `PATH`, atomically, mode 0600 |
| Empty token value | Accepted (D14) | An empty `VOGTER_TOKEN` counts as unset; an empty `token_env` variable or empty `token_cmd` output is an error |
| Unknown keys, theme, severity | Silently ignored or defaulted (D17) | Same fallback values, plus a warning (shown by `vogter check`) |
| Missing `default_profile` target, profile without `url` | Not reported at load (exact failure point unverified) | Error at load |
| Plain-text secret in a group/world-readable file | — | Warning with the `chmod` to fix it |
| `token_cmd` | `sh -c`, no time limit | `sh -c` (`cmd /C` on Windows), 2 minute limit |
| `refresh_interval` below 2 | Raised to 2, footer shows the configured value (D16) | Raised to 2 with a warning; footer shows 2 |
| `check` | Version, host and problem counts | Also auth source, TLS state, a version-support verdict, and a hint per error class |

## Zabbix API

| Area | zabterm | Vogter |
|---|---|---|
| Target version | "7.x and 8.0" | 7.0 LTS primary. 6.4 runs with an "untested" warning. Below 6.4 is refused |
| Start-up | Two polls (D1) | One poll |
| Concurrent refresh requests | Each queued and run | Coalesced into one follow-up poll |
| `user.login` sessions | Never closed (D3) | `user.logout` on quit and before re-login |
| Re-login trigger | Substring match on any error (D18) | Only when the error is classed as auth and the profile uses a session |
| Failed call with partial data | Fails the whole call | Same: a failed call never returns data |
| User-Agent | `zabterm/<ver>` | `vogter/<ver>` |

## Screens

Compared with zabterm 0.1.2 on 2026-10-30 at 150×45: both binaries against `tools/mockzbx`, with the
same keys, using `tools/compare.sh` and `tools/cmpdiff.py`. The diff ignores the product name, clocks and
braille dots. Dashboard and hosts screens matched on every line, and problems screens differed only by the
`o web` hint. The host page matched except for chart data captured at a different moment. The 24h and 7d
charts differ by design (hourly trends). Polling requests matched exactly; host page requests differ as
described below.

| Area | zabterm | Vogter |
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
| Late detail for another host or range | Range mismatch shown briefly | Dropped |

## Not yet ported (by milestone)

- M5: Omarchy themes, notifications, `vogter import`, browser opener (`o`).
