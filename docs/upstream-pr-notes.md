# Notes for upstream zabterm pull requests

Defects found while porting, fixed in Vægter, to offer upstream as PRs at the end of the port. Locations
refer to zabterm commit `d57c642`. Nothing has been submitted yet.

| ID | Status in Vægter | Upstream location | Problem | Reproduction | Proposed Rust change |
|---|---|---|---|---|---|
| D1 | Fixed (M2) | `app.rs:160` + `worker.rs:50-56` | Two full polls at start-up: `App::new` sends `Cmd::Refresh` and the first `interval.tick()` fires immediately | Run against any server and log requests: two `host.get` within ~1 ms | Drop `app.send(Cmd::Refresh)` from `App::new`, or call `ticker.tick().await` once before the loop |
| D3 | Fixed (M2) | `api.rs:28-35`, `worker.rs:88-93`, `main.rs:126` | `user.login` sessions are never closed; each run and each re-login leaves a session behind (the Zabbix manual asks for `user.logout`) | `username`/`password` profile; start and quit; no `user.logout` request | Track whether the token is a session; call `user.logout` with `[]` on quit and before replacing the client |
| D14 | Fixed (M2) | `config.rs:124-130` | An empty `ZABTERM_TOKEN` or empty `token_env` variable is accepted as a token, giving a confusing auth error later | `ZABTERM_TOKEN= zabterm check` with a valid `token` in the file: fails with "Not authorized" | Treat empty values as unset (`Ok(t) if !t.is_empty()`) |
| D15 | Fixed (M2) | `main.rs:59-60`, `config.rs:202-203` | `zabterm -c other.toml init` writes the default path, not `other.toml` | Run it; `other.toml` is not created | Pass the resolved `config_path` into `write_template` |
| D16 | Fixed (M2) | `ui/mod.rs:149` | Footer shows the configured interval although the worker raises values below 2 to 2 | `refresh_interval = 1`; footer says "every 1s" | Clamp in `Config::load` instead of `worker::spawn` |
| D17 | Partly fixed (M2: warnings) | `config.rs:58-99`, `theme.rs:192-203`, `app.rs:123` | Unknown keys, theme names and severity names are silently ignored or defaulted | `theme = "gruvbx"` gives tokyo-night with no message | Have `check` report them, for example with `serde_ignored` or a second pass over `toml::Table` |
| D18 | Fixed (M2) | `worker.rs:88-93` | Re-login decided by substring match on any error text, including transport errors that mention "session" | Hard to trigger. Seen by reading the code | Match the JSON-RPC error data from `call()` explicitly |
| D6 | Fixed (M3) | `worker.rs:175-180` | A failed seed `history.get` is swallowed but the items are marked seeded, so they are never retried | Fail the first `history.get`; sparklines stay empty until the window fills | Extend `series.seeded` only when the call succeeded |
| D7 | Fixed (M3) | `worker.rs:175` | Seed history always asks for `history:0`, so a type 3 CPU or memory item gets no seed | Serve `system.cpu.util` with `value_type` 3 | Group unseeded items by `value_type`, as `host_detail` already does |
| D9 | Fixed (M3) | `api.rs:90`, `dashboard.rs` | Counts silently cap at `limit:500` | More than 500 active problems | Flag a full page and show `500+` |
| D19 | Fixed (M3) | `worker.rs:114-134` | Sparkline buffers are never dropped for removed items | Delete items during a long run; memory grows slowly | `retain` the maps against the current item ids |
| D20 | Fixed (M3) | `worker.rs:53-96` | One command at a time, so an acknowledge waits behind a slow poll | Slow server, press `a` then `⏎` during a poll | Spawn `acknowledge` as its own task; `Client` is already shared by reference |
| D2 | Fixed (M4) | `app.rs:202-204,521-526`, `worker.rs:290-353` | Each snapshot on the host page triggers another detail request as well as the poll's, and each one re-reads the whole range, up to 7 days of raw history | Open a host, pick 7d, log requests: `history.get` from now-7d every poll, twice | Cache per item, set `time_from` to the newest sample + 1, and use `trend.get` from 24h up |
| D4 | Fixed (M5) | `theme.rs:114-140,178-181` | Omarchy 4 moved the theme to `~/.local/state/omarchy/current` and switched `colors.toml` to named keys, so `auto` silently becomes tokyo-night. Derived from Omarchy v4.0.6 source, not seen on a machine | Omarchy 4: `zabterm`, then `?` shows the theme | Check both directories; read `red`, `green`, ... and `muted`, falling back to `colorN` |
| D5 | Fixed (M5) | `theme.rs:116` | Theme name comes from the canonical directory, which is `theme` on Omarchy 4 | As D4 | Read `current/theme.name` first |
| D10 | Fixed (M5) | `app.rs:237-241` | Past `limit:500`, a problem pushed off the page is announced as resolved | More than 500 problems, a newer one arrives | Skip resolve notes when either snapshot holds a full page |
| D11 | Fixed (M5, unverified on macOS) | `app.rs:607` | AppleScript text is built with Rust `{:?}`, which can emit `\\u{...}` escapes AppleScript does not know | Problem name with an emoji or control character | Pass the text as `osascript` arguments and read it from `argv` |
| D21 | Fixed (M5) | `app.rs:590-593` | `o` on a problem with no host builds an empty `hostids[]=` | Problem whose trigger has no host | Fall back to `problem.view` |
| D22 | Fixed (live test) | `ui/fmt.rs:50` | Chart time axis shows only HH:MM, so the 7d graph's labels do not say which day they mean | Open a host graph at the 7d range | Show the date on ranges beyond two days |

D8 (multi-host triggers), D12 and D13 are kept as they are; see `porting-audit.md` §13.
