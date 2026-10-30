# Vogter: porting audit of zabterm

Milestone 1 deliverable. Audit date: 2026-10-29. Author: knightmare2600 (with Claude Code).

Vogter is an independent Go port of zabterm. It is not affiliated with, or endorsed by, the upstream
author.

Every claim below is marked with how it was established:

- **[src]**: read in the upstream source at the baseline commit.
- **[run]**: observed by running the upstream v0.1.2 release binary (see §3).
- **[doc]**: taken from the official Zabbix or Omarchy documentation or source, fetched on the audit date.
- **[unverified]**: not confirmed. A way to verify it is given.

---

## 1. Upstream baseline

| | |
|---|---|
| Repository | https://github.com/enderkus/zabterm |
| Baseline commit | `d57c64239fbdc65cdb6b926a62397c87a1bd7b04` (2026-09-30 15:58 +0100, "Add contributing and support sections, and a funding link") |
| Latest release | `v0.1.2` (tag on `63c1390`, published 2026-09-30). The two later commits change only `README.md` and docs |
| Crate version | `0.1.2`, Rust edition 2024 |
| History | 15 commits, 2026-09-26 → 2026-09-30; 3 tags (`v0.1.0`, `v0.1.1`, `v0.1.2`) |
| Contributors | Ender KUŞ (14 commits). One merged external PR (#1, `cf1ad3b`, by jpikkarainen@forcepoint.com, co-authored by an AI model) |
| Open issues / PRs | None at audit time (`gh issue list`, `gh pr list`) |
| Size | ~3,500 lines of Rust in 14 files under `src/` |
| Upstream tests | 4 unit tests: 3 in `src/theme.rs` (256-colour mapping, Alacritty parse, `colors.toml` parse) and 1 in `src/ui/fmt.rs` (formatters). No API, worker, app or UI tests |
| CI | `.github/workflows/ci.yml`: `cargo fmt --check`, `clippy -D warnings`, `cargo test` and `cargo build --release` on ubuntu-latest and macos-latest |
| Release CI | `.github/workflows/release.yml`: builds `x86_64`/`aarch64` Linux GNU and `aarch64`/`x86_64` Darwin tarballs (binary + README + LICENSE) |

The repository also holds a Jekyll GitHub Pages site (`docs/`), icons (`assets/`), a `.desktop` file
(`extras/`), an installer (`install.sh`) and `FUNDING.yml`. **[src]**

## 2. Licence and attribution

- `LICENSE` is the standard MIT text with a single line `Copyright (c) 2026 Ender KUŞ`. `Cargo.toml` says
  `license = "MIT"`. I read the file in full. **[src]**
- No file carries its own licence header. No vendored third-party code exists in the repository. **[src]**
- The icons (`assets/zabterm*.png`, `zabterm.svg`) and screenshots carry no separate licence. Presumption:
  they fall under the repository MIT licence. The name "zabterm" and the icon are upstream branding, so
  Vogter will use neither (it is a separate project).
- The Rust dependency licences (§12) bind only the Rust binary. Vogter copies no Rust crate code, so they
  create no obligation for Vogter.

**Obligations for Vogter:**

1. Ship upstream's MIT notice. Plan: `LICENSE` carries Vogter's own MIT notice, followed by upstream's full
   notice under a "Portions derived from zabterm" heading. A `NOTICE` (or README section) explains that
   Vogter is an independent Go port of zabterm at commit `d57c642`, with no affiliation or endorsement.
2. Keep upstream's copyright line verbatim.
3. Record Go dependency licences in `docs/dependencies.md` (all candidates are MIT or BSD-3-Clause, §12.3).
   BSD-3-Clause requires reproducing its notice in binary distributions. That is a release-packaging task
   for Milestone 6.
4. Port nothing from the upstream Jekyll site, icons, `install.sh` or `FUNDING.yml`. History is not
   imported.

**Open provenance point:** commit `cf1ad3b` says it was co-authored by an AI model. It was merged under the
repository's MIT licence, so it creates no extra obligation. Recorded for completeness only.

## 3. Build and run results

| Step | Result |
|---|---|
| Build from source | **Not done.** `cargo`/`rustc` are not installed here, and per the install boundary I did not install them |
| Run the release binary | **Done.** I downloaded `zabterm-v0.1.2-x86_64-unknown-linux-gnu.tar.gz` with `gh release download`. Its SHA-256 (`f5b947ff…fead08`) matches the asset digest GitHub records. I ran it under an isolated `HOME`/`XDG_CONFIG_HOME` in the scratchpad, so your real config was not touched |
| Binary | 7,060,472 bytes, stripped, LTO, dynamically linked to `libc`, `libm` and `libgcc_s` only |
| Mock server | A stdlib-only Go fixture JSON-RPC server (audit scaffolding, not part of Vogter) that logs every request, with the token redacted |

Exercised and observed **[run]**:

- `--help`, `--version` (`zabterm 0.1.2`), `config`, `init` (writes mode `0600`; a second `init` refuses
  without `--force`).
- `check`: with no config; with the placeholder token; with a valid token via `ZABTERM_URL`/`ZABTERM_TOKEN`;
  with a token the server rejects (`host.get: Invalid params. Not authorized.`); with an unknown profile;
  with a connection refused. Each failure exits 1.
- TUI under a pseudo-terminal (170×50, `COLORTERM=truecolor`) with scripted keys: Dashboard → `2` →
  `⏎` (host page) → `]` (range) → `i` (Latest data) → `esc` → `3` → `a`, type a message, `tab`, `⏎` →
  `q`. Username/password authentication was used.
- All screens rendered. The "Acknowledged" toast appeared. The ack dialog showed "trigger does not allow
  manual close". The alternate screen was entered and left once (terminal restored). Output used 24-bit SGR.

**Not exercised:** desktop notifications (disabled in the test config), the `o` browser opener, the Omarchy
live reload, macOS, and any real Zabbix server. No live Zabbix instance or credentials are available.

## 4. Upstream architecture

```
main.rs ── clap CLI ──► init / config / check (direct API calls)
   │
   └─ tui(): tokio runtime
        ├─ worker::spawn ─► task owning api::Client ─ Cmd {Refresh, Host, Ack} ◄── unbounded mpsc
        │                                           └─ Event {Connected, Snapshot, Host, Acked, Error} ──►
        └─ UI loop: select! { 250 ms tick → app.on_tick | worker event → app.on_event | key → app.on_key }
                    then ui::draw(frame, &mut app)   (immediate-mode ratatui redraw every iteration)
```

**[src]** File roles:

| File | Role |
|---|---|
| `src/main.rs` | CLI (`clap` derive), `check`, TUI event loop, terminal init/restore |
| `src/config.rs` | TOML schema, defaults, `init` template, credential resolution, URL normalisation |
| `src/api.rs` | JSON-RPC client (`reqwest`) and wire structs (all numbers kept as strings) |
| `src/model.rs` | `Severity`, `Availability`, `HostRow`, `Problem`, `Snapshot`, `HostDetail` |
| `src/worker.rs` | Poll loop, reconnect, snapshot assembly, sparkline store, host-detail assembly |
| `src/app.rs` | All UI state, key handling, filtering and sorting, notification diffing, browser opener |
| `src/theme.rs` | 8 palettes, role derivation, Omarchy reader, truecolor→256 fallback |
| `src/ui/*.rs` | Rendering: header/footer/splash/toasts/modals (`mod.rs`), `dashboard`, `hosts`, `host`, `problems`, `widgets` (panels, meters, badges, sparkline, big digits, braille chart), `fmt` (units) |

Data flow:

1. The worker waits for whichever comes first: a ticker (`refresh_interval`, minimum 2 s, missed ticks
   delayed) or a command.
2. With no client, it connects (`user.login` if needed, then `apiinfo.version`) and emits `Connected`. On
   failure it emits `Error` and waits for the next trigger.
3. `Refresh` builds a `Snapshot`. `Host` builds a `HostDetail`. `Ack` calls `event.acknowledge`, emits
   `Acked`, then builds a `Snapshot`.
4. When an error message contains `not authorized`, `session` or `re-login` (case-insensitive substring
   match), it drops the client so the next trigger reconnects.
5. The UI only reads the latest `Snapshot` and `HostDetail`. Every `Snapshot` received while the host page
   is open triggers a new `Host` command.

The worker processes one command at a time, so a slow call delays everything queued behind it. The
optional net/disk search is the exception: it runs with a 5 s budget (§6.3).

## 5. Feature inventory and parity checklist

Status key: **I** = implemented and observed working **[run]**; **S** = implemented, read in source only;
**P** = partial or defective (see §13); **D** = documented but not implemented.

| ID | Feature | Upstream location | Status | Go plan |
|---|---|---|---|---|
| **CLI** | | | | |
| C1 | `zabterm` launches TUI; fails fast on missing credentials before entering the TUI | `main.rs:56-81` | I | `cmd/vogter`: same order (load → profile → resolve auth → TUI) |
| C2 | `init [--force]`: writes commented template, mode 0600, refuses to overwrite | `main.rs:59`, `config.rs:202` | I (P: ignores `-c`, §13 D15) | `vogter init [--force]`, honouring `-c`. Atomic write; 0600 on Unix |
| C3 | `check`: prints config/profile/endpoint, then version, host count and problem count | `main.rs:83` | I | `vogter check`, plus distinct error classes (§6.5) |
| C4 | `config`: prints config path | `main.rs:65` | I | `vogter config` |
| C5 | `-p/--profile`, `-c/--config` (global, before or after subcommand) | `main.rs:20-25` | I | Same flags, accepted in either position |
| C6 | `-t/--theme` override for one run | `main.rs:28,74` | S | Same |
| C7 | `-V/--version`, `-h/--help` | clap | I | Same; version from `-ldflags` |
| **Config** | | | | |
| G1 | TOML with `default_profile`, `theme`, `refresh_interval`, `[notifications]`, `[profiles.*]` | `config.rs:58-99` | I | Identical schema (§7) |
| G2 | URL normalisation: appends `/api_jsonrpc.php` if missing; web URL strips it | `config.rs:114-121` | I (observed trailing `/` handled) | Same, using `net/url` |
| G3 | Credentials in order: env token → `token_env` → `token_cmd` (`sh -c`, trimmed) → `token` (not the placeholder) → `username`+`password` | `config.rs:123-148` | I (env, login, placeholder) / S (`token_env`, `token_cmd`) | Same order, with `VOGTER_TOKEN` (§7.3) |
| G4 | `ZABTERM_URL` alone creates profile `env` and makes it default | `config.rs:176` | I | `VOGTER_URL` |
| G5 | Missing config + no env → "no profiles configured" with guidance | `config.rs:180` | I | Same |
| G6 | `verify_tls = false` per profile | `config.rs:97`, `api.rs:25` | S | `tls.Config{InsecureSkipVerify}`, per profile only, with a visible warning |
| G7 | `refresh_interval` minimum 2 s | `worker.rs:42` | S (P: footer shows raw value, D16) | Clamp at load; display the effective value |
| **Dashboard** | | | | |
| B1 | 5 big-digit tiles: Hosts (up/down), Availability % of enabled (colour ≥99.9 ok / ≥90 warn / else err; subline: unreachable, maintenance or "agents reachable"), Problems (unsuppressed count coloured by worst; unacked + worst label, short label if tile <32 cols), Avg CPU, Avg Memory (excluding Down hosts; "peak" host) | `dashboard.rs:55-132` | I | `ui/dashboard.go`, big-digit glyph table ported verbatim |
| B2 | "Problems by severity" stacked bar (each non-zero severity ≥1 cell, largest absorbs rounding) and legend | `dashboard.rs:134-170` | S | Same algorithm, unit-tested |
| B3 | Fleet chart: 6 busiest hosts (by current value) over 30 min, braille lines, legend; `m` toggles CPU/memory | `dashboard.rs:172-194` | I | Hand-rolled braille chart widget (§14.4) |
| B4 | Latest problems feed: unsuppressed, newest first, 2 lines each, ✓ if acked, "All clear" when empty | `dashboard.rs:196-231` | I | Same |
| B5 | Host table: status dot, name, CPU sparkline (24 wide ≥130 cols, else 14), memory meter; wide adds Disk /, Load, Network ↓↑, Uptime; per-severity counters | `dashboard.rs:233-285` | I | Same, including the 130-column breakpoint |
| B6 | Header: logo badge, tabs with counts (problem count coloured by worst), `● live` pulse / spinner `sync` / `● offline`, profile, Zabbix version, latency ms, clock. Drops parts when narrow (3 tiers) | `ui/mod.rs:46-110` | I | Same tiers |
| B7 | Footer: per-view key hints, "updated Ns ago · every Ns" when room | `ui/mod.rs:112-155` | I | Same |
| B8 | Splash before first snapshot: logo, "connecting to …" or error + config path + retry hint | `ui/mod.rs:168-198` | I | Same layout, "VOGTER" glyphs |
| **Hosts** | | | | |
| H1 | Table: status, name, groups, interface address, CPU meter, memory meter, disk %, load, uptime, problem counters; selection marker `▌` | `hosts.rs:39-83` | I | `ui/hosts.go` |
| H2 | Filter `/` on name, address, group (case-insensitive substring) | `app.rs:267-281` | S | `model.FilterHosts`, unit-tested |
| H3 | Sort `s` cycles problems → cpu → memory → name (default problems: worst desc, count desc, name asc; cpu/mem desc with missing last) | `app.rs:38-56,282-289` | S | `model.SortHosts`, unit-tested |
| H4 | Preview: 30 min CPU and memory charts and an info panel (state, CPUs, Net ↓↑, uptime, interface error) | `hosts.rs:85-137` | I | Same |
| H5 | Status markers: `✕` disabled, `◆` maintenance, `●` up (green) / down (red), `○` unknown; disabled names struck through, down names red, stale values muted | `widgets.rs:50-78` | I | Same |
| **Host page** | | | | |
| P1 | Head: name, state pill, address, groups, facts (OS, Hostname, Arch, Agent, CPUs, Memory, Uptime from item keys), tabs, range selector, problem counters | `host.rs:28-82`, `worker.rs:320-335` | I | Same |
| P2 | Graphs tab: CPU, memory, network (busiest iface in/out), load 1m; ranges 15m/1h/6h/24h/7d via `[` `]`, default 1h | `host.rs:88-140`, `app.rs:14` | I | Same; history-fetch strategy changes (§13 D2, decision Q4) |
| P3 | Filesystems panel (`vfs.fs.dependent.size[*,pused]`, `/etc/*` excluded, sorted by mount) and host problems panel | `host.rs:142-168` | I | Same |
| P4 | Latest data tab: every item (name, key, formatted last value, age, time); `/` filters on name or key | `host.rs:171-207`, `app.rs:310-314` | I | Same; value formatting ported from `fmt.rs` with its test vectors |
| P5 | Back with `esc`/`h`/`backspace` to the originating view; header keeps the origin tab lit | `app.rs:448,511-515`, `ui/mod.rs:61` | I (`esc`) | Same |
| P6 | Page refreshes on every poll | `app.rs:202-204` | I (P: duplicated requests, D2) | Same cadence without duplicates |
| **Problems** | | | | |
| R1 | Table: severity badge, time, age, host, name, ack ✓, tags; sorted severity desc then newest; acked/suppressed dimmed | `problems.rs:41-91`, `worker.rs:213` | I | `ui/problems.go` |
| R2 | Min-severity filter `+`/`s` up, `-` down (wraps around); chip strip shows the scale | `app.rs:429-438`, `problems.rs:18-39` | S | Same (wrap-around kept) |
| R3 | Hide-acknowledged toggle `h` | `app.rs:439-442` | S | Same |
| R4 | Filter `/` on name, host, tags | `app.rs:292-308` | S | `model.FilterProblems` |
| R5 | Details pane: name, host, severity, started, event id, ack status, suppressed, opdata, tag pills | `problems.rs:93-141` | I | Same |
| R6 | Acknowledge `a`: dialog with optional message; `tab` toggles "close problem too" only when the trigger has `manual_close=1`; `⏎` confirms, `esc` cancels; refreshes after | `app.rs:418-428,494-506`, `api.rs:169-180` | I (`action: 6`) | Same. Confirmation is the dialog itself (explicit ⏎). Permission errors mapped to a clear toast (§6.5) |
| R7 | `⏎`/`l` opens the problem's host | `app.rs:413-417` | S | Same |
| **Cross-cutting** | | | | |
| X1 | Views `1` `2` `3`, `tab`/`shift-tab` cycle (not on the host page) | `app.rs:361-377` | I | Same |
| X2 | Movement `j k ↑ ↓`, `g G home end`, `ctrl-d ctrl-u pgdn pgup` (±10) | `app.rs:388-395` | S | Same |
| X3 | `r` refresh now (+ host detail on the host page) | `app.rs:521-526` | S | Same |
| X4 | `?` help modal (closes on `esc`/`?`/`q`/`⏎`); shows theme and config path | `ui/mod.rs:236-264` | S | Same |
| X5 | `q`/`ctrl-c` quit; terminal restored (also on panic, via `ratatui::init` hook) | `main.rs:102-126` | I | Bubble Tea restores on exit; add `recover` → restore → re-panic |
| X6 | `o` opens the Zabbix frontend: dashboard / `latest.view?hostids[]=` / `problem.view?hostids[]=` via `xdg-open`/`open`, detached and reaped; toast with the URL | `app.rs:582-601,617-622` | S | `internal/platform.OpenURL`; Windows added (§11) |
| X7 | Toasts: top right, max 4, 6 s lifetime, 4 rows each, kinds info/ok/error/problem(severity) | `app.rs:169-178`, `ui/mod.rs:200-225` | I | Same |
| X8 | Repeated identical API error raises only one toast | `app.rs:215-221` | S | Same |
| **Notifications** | | | | |
| N1 | New problems ≥ `min_severity`, unsuppressed, not present at start; max 3 per poll; toast ▲ in severity colour | `app.rs:227-236` | S | `notify.Diff`, pure and unit-tested |
| N2 | Resolved problems (eventid vanished) ≥ `min_severity`, unsuppressed; max 3 per poll | `app.rs:237-241` | S (P: false "resolved" beyond 500 problems, D10) | Same, with the D10 guard |
| N3 | Host Up→Down ("Unreachable", interface error or address, urgent) and Down→Up ("Back online"); not capped, not severity-filtered | `app.rs:242-254` | S | Same |
| N4 | Desktop: Linux `notify-send -a zabterm -u critical|normal`; macOS `osascript display notification`; failures ignored | `app.rs:604-613` | S | `platform.Notify`. Windows: disabled with a one-time toast (decision Q6) |
| N5 | `notifications.desktop=false` keeps toasts only | `app.rs:258` | S | Same |
| **Themes** | | | | |
| T1 | 8 built-in palettes (§9) | `theme.rs:98-110` | S | Values ported verbatim |
| T2 | Role derivation (muted, border, surface, severities, series) by RGB mixing | `theme.rs:205-229` | S | Same formulas, unit-tested against the Rust rounding |
| T3 | `auto`/`omarchy`: reads the Omarchy current theme (`colors.toml` with `colorN`, else `alacritty.toml`); falls back to tokyo-night | `theme.rs:114-160,192-198` | **P: does not work on Omarchy 4.x** (D4) | Support 3.x and 4.x layouts (§9.3) |
| T4 | Live reload: every 8 ticks (~2 s) compares the canonical dir and mtime; repaints and toasts | `app.rs:178-186`, `theme.rs:184-189` | S | Same, as a `tea.Tick` |
| T5 | `t` cycles `auto` (only if the Omarchy dir exists) + 8 built-ins; session only | `theme.rs:231-240` | S | Same |
| T6 | Truecolor unless `TERM_PROGRAM=Apple_Terminal` without truecolor `COLORTERM`; `ZABTERM_TRUECOLOR=0/1` forces it; 256-colour nearest-cube/grey mapping | `theme.rs:62-77,164-176` | S | Lip Gloss colour profile detection plus `VOGTER_TRUECOLOR` override; add `NO_COLOR` and 16-colour fallback (new) |
| T7 | Background never painted (keeps terminal transparency) | `theme.rs`, `ui/*` | S | Same |
| **Distribution** (not in scope for parity) | | | | |
| Z1 | `install.sh`, `.desktop` entry, icons, Homebrew tap | `install.sh`, `extras/` | S | Milestone 6 decision: `.desktop` file yes; no curl-pipe installer unless requested |

No documented feature is entirely missing (no **D** rows). trend data is never used: the README's "Show
trends in sparklines" (`a3f28b6`) means visual autoscaling, not `trend.get`. **[src]**

## 6. Zabbix API usage

### 6.1 Transport **[src][run]**

- `POST {url}/api_jsonrpc.php`, `Content-Type: application/json-rpc`, `User-Agent: zabterm/<ver>`.
- JSON-RPC 2.0 body `{jsonrpc, method, params, id}` with an incrementing id. No `auth` property in the
  body.
- `Authorization: Bearer <token>` on every call except `apiinfo.version` and `user.login`.
- 20 s whole-request timeout. TLS via rustls with platform roots. HTTP/2 enabled.
- Non-2xx → error `"{method}: HTTP {status}"`. A JSON `error` object → `"{method}: {message} {data}"`. A
  missing `result` → error. A shape mismatch → error. Errors are never turned into empty results, except
  in the three places listed in §13 (D6, net/disk search, sparkline seed).

### 6.2 Methods and exact parameters (captured from the running binary) **[run]**

| Method | When | Params |
|---|---|---|
| `user.login` | Connect, if using username/password | `{"username":…, "password":…}`. The result string is the session token, used as Bearer |
| `apiinfo.version` | Connect, `check` | `{}`, without the auth header |
| `host.get` | Every poll, `check` | `output:[hostid,name,status,maintenance_status,active_available]`, `selectInterfaces:[ip,dns,useip,port,available,type,error]`, `selectHostGroups:[name]`, `sortfield:"name"`. No limit |
| `problem.get` | Every poll, `check` | `output:[eventid,objectid,name,severity,clock,acknowledged,suppressed,opdata]`, `selectTags:[tag,value]`, `sortfield:["eventid"]`, `sortorder:"DESC"`, `limit:500` |
| `trigger.get` | Every poll (if any problems) | `triggerids:[…unique objectids]`, `output:[triggerid,description,manual_close]`, `selectHosts:[hostid,name]` |
| `item.get` (exact keys) | Every poll | `filter.key_:[system.cpu.util, vm.memory.util, system.cpu.load[all,avg1], system.uptime, system.cpu.num]`, `monitored:true`, `output:[itemid,hostid,name,key_,lastvalue,lastclock,units,value_type]` |
| `item.get` (prefix search) | Every poll, unless backing off | `search.key_:["net.if.in[","net.if.out[","vfs.fs.dependent.size["]`, `searchByAny:true`, `startSearch:true`, `monitored:true`, same output |
| `history.get` (seed) | Once per new CPU/MEM item | `history:0`, `itemids:[…]`, `time_from:now-1800`, `sortfield:"clock"`, `sortorder:"ASC"`. No limit |
| `item.get` (host) | Host page, every poll | `hostids:[id]`, same output, `sortfield:"name"`. No `monitored` filter, no limit |
| `history.get` (host) | Host page, every poll | One call per value type present among CPU/MEM/LOAD/busiest net in/out: `history:0` and/or `history:3`, `time_from:now-range`, sorted ASC. No limit, no `time_till` |
| `event.acknowledge` | `a` → `⏎` | `{"eventids":[id], "action":2|4?|1?, "message":"…"}`. Observed: `action:6` |

Not used: `trend.get`, `user.logout`, `event.get`, `hostgroup.get`, anything that writes configuration.
**[src]**

### 6.3 Request pattern per poll **[src][run]**

- `host.get`, `problem.get`, `item.get` (keys) and `item.get` (prefix) run concurrently. The prefix search
  is wrapped in a 5 s timeout. If it fails or times out, its columns go empty and it is skipped for 60 s
  (`EXTRA_BUDGET`/`EXTRA_RETRY`, `worker.rs:21-24,138-164`).
- After that comes `trigger.get` (sequential), then the optional seed `history.get`.
- Sparklines: a per-item ring buffer covering 30 minutes. It is seeded once from history, then extended
  with each poll's `lastvalue`/`lastclock` (only when the clock advances), and trimmed on push.
- Observed timing at startup: two full snapshots ran back to back (D1). On the host page with a 3 s
  interval, each poll produced two `item.get`(host) + `history.get` pairs (D2).

### 6.4 Response types (wire, all strings) **[src]**

`Host{hostid,name,status,maintenance_status,active_available?,interfaces[],hostgroups[]}`,
`Interface{ip,dns,useip,port,available,type,error?}`,
`RawProblem{eventid,objectid,name,severity,clock,acknowledged,suppressed,opdata?,tags[]{tag,value}}`,
`Trigger{triggerid,manual_close?,hosts[]{hostid,name}}`,
`Item{itemid,hostid,name,key_,lastvalue,lastclock,units,value_type}`, `HistoryPoint{itemid,clock,value}`.
Fields marked `?` default to empty when absent. All other fields are required: a missing one fails the
whole call ("unexpected response shape").

Semantic mapping **[src]**:

- Severity `0..5` (unparseable → 0).
- `status=="0"` → enabled. `maintenance_status=="1"` → maintenance.
- Availability: choose the agent interface (`type=="1"`), else the first interface. Down if that
  interface's `available=="2"` **or** `active_available=="2"`, else Up if either is `"1"`, else Unknown.
- `Item.value()` is `None` when `lastclock=="0"` or the value is unparseable. This is how "missing" stays
  distinct from zero, and Vogter keeps it.
- Numeric means `value_type` 0 (float) or 3 (unsigned).

### 6.5 Version compatibility **[doc]**

Upstream claims "Zabbix 7.x and 8.0" (README) and "Developed and tested against the 8.0 API … 7.0 and
later should work too" (guide). I checked each version-sensitive feature against the official manual
pages for 6.0, 6.4, 7.0, 7.4 and 8.0:

| Feature used | 6.0 | 6.4 | 7.0 | 7.4 | 8.0 |
|---|---|---|---|---|---|
| `Authorization: Bearer` header documented | no | yes | yes | yes | yes |
| `host.get` `selectHostGroups` | no (`selectGroups`) | yes | yes | yes | yes |
| `host` object `active_available` | no | yes | yes | yes | yes |
| `user.login` `username` param | yes | yes | yes | yes | yes |
| `apiinfo.version` must be unauthenticated | yes | yes | yes | yes | yes |
| `event.acknowledge` bitmask 1/2/4 (message required when 4 set; close needs `manual_close` and R/W rights) | — | — | yes (read) | yes (read) | — |
| `history.get` value types 0–5, `trend.get` (`itemids`, `time_from`, `time_till`, `limit`) | — | — | yes (read) | yes (read) | — |
| 7.4→8.0 breaking changes affecting zabterm's calls | | | | | none: only `history.get` `maxValueSize` for binary type 5 (unused), and removed `*.massupdate`/`hostinterface.*` writers (unused) |

Conclusions:

- The effective minimum is **6.4** by API surface. 6.4 has not been tested against a server; that is
  unverified, and only a live 6.4 instance can settle it.
- 6.0 LTS will fail at `host.get` (unknown parameter).
- **Zabbix 8.0 is not yet released.** zabbix.com release notes list `8.0.0rc3`, and the documentation
  selector marks 7.4 as "current". Upstream's "tested against 8.0" therefore refers to a pre-release.
- **Decided (2026-10-29): the primary target is Zabbix 7.0 LTS**, the current LTS release (8.0 is still a
  release candidate). Fixtures, request shapes and parameter choices are checked against the 7.0 manual
  first. 7.2, 7.4 and 8.0 are secondary: they are tracked for differences that affect Vogter's calls, and
  §6.5 has found none so far. Server version detection uses `apiinfo.version`. Below 7.0 Vogter warns
  that the version is untested. Below 6.4 it refuses to start, because the API features it needs
  (Bearer header, `selectHostGroups`, `active_available`) are missing. The comparison is pure and
  testable.

Vogter error classes (new, required by the brief). Upstream folds all of these into strings:

| Class | Detection |
|---|---|
| Transport | Dial, TLS, timeout or context error |
| HTTP | Non-2xx status (e.g. 502 from a proxy) |
| Decode | Body is not JSON-RPC |
| RPC | `error` object with `code`/`message`/`data` |
| Auth | RPC error whose data is "Not authorised"/"Session terminated…". Triggers re-login for sessions; reported as a credential problem for tokens |
| Permission | e.g. "No permissions to referred object or it does not exist" on `event.acknowledge` |
| Shape | Required field missing |
| Unsupported version | `apiinfo.version` below 6.4 |

Exact message strings must come from fixtures captured from a real server; that is unverified. Until
then, matching uses the documented `code` values plus substring fallback, as upstream does.

## 7. Configuration

### 7.1 Upstream schema **[src][run]**

```toml
default_profile = "local"     # optional; else first profile in sorted (BTreeMap) order
theme = "auto"                # auto|omarchy|tokyo-night|catppuccin|gruvbox|nord|everforest|rose-pine|kanagawa|matte-black
refresh_interval = 10         # seconds, effective minimum 2

[notifications]
desktop = true
min_severity = "average"      # also accepts nc/info/warn/avg, '-' or ' ' for '_', any case

[profiles.NAME]
url = "https://zabbix.example.com"   # required; /api_jsonrpc.php appended if absent
token = "…"
token_env = "VAR"
token_cmd = "pass show zabbix"       # sh -c, stdout trimmed, non-zero exit is an error
username = "Admin"
password = "zabbix"
verify_tls = true
```

Lenient parsing: unknown keys are ignored. An unknown theme silently becomes tokyo-night. An unknown
`min_severity` silently becomes average. **[src]**

### 7.2 Vogter schema

Same keys and semantics, so a zabterm file imports unchanged. Validation (new, reported by
`vogter check` and at start-up) produces errors or warnings for: unknown keys, unknown theme, unknown
severity, `refresh_interval < 2`, a profile without `url`, a profile without any credential, and more
than one credential source set (a warning; the precedence still applies).

### 7.3 Paths and environment

| | zabterm | Vogter |
|---|---|---|
| Config (Linux/macOS) | `$XDG_CONFIG_HOME/zabterm/config.toml` (only if absolute), else `~/.config/zabterm/config.toml` | `$XDG_CONFIG_HOME/vogter/config.toml`, else `~/.config/vogter/config.toml` (matches upstream convention, not `os.UserConfigDir` on macOS) |
| Config (Windows) | n/a | `%AppData%\vogter\config.toml` (`os.UserConfigDir`) |
| URL env | `ZABTERM_URL` | `VOGTER_URL` |
| Token env | `ZABTERM_TOKEN` | `VOGTER_TOKEN` |
| Colour env | `ZABTERM_TRUECOLOR` | `VOGTER_TRUECOLOR` (+ honour `NO_COLOR`) |

### 7.4 Migration from zabterm

Proposed `vogter import [--from PATH] [--force]`:

1. Default source: the zabterm path.
2. Parse and validate.
3. Write to the Vogter path with mode 0600, atomically (temp file + rename).
4. Refuse to overwrite without `--force`, showing the diff target path.
5. Never modify the source.
6. Print what was imported, with secrets shown as `<set>`.

The schema is identical, so this is a validated copy. Whether to also honour `ZABTERM_*` environment
variables as a fallback is decision Q5.

### 7.5 Secret handling

- Upstream never prints tokens (`check` prints URL and profile only). **[run]**
- The `token_cmd` stderr is included in error text. That is acceptable: it is not the secret.
- Vogter adds: a `Secret` type whose `String()`/`GoString()`/`slog` value is `<redacted>`; tokens are
  never placed in error strings; and a warning when the config file is group- or world-readable and
  contains `token` or `password`.

## 8. Keyboard and navigation **[src]**

Global (not in filter or modal mode):

| Key | Action |
|---|---|
| `1` `2` `3` | Dashboard / Hosts / Problems (also leaves the host page) |
| `tab` / `shift-tab` | Next / previous view (on the host page, `tab` toggles Graphs/Latest data instead) |
| `j` `↓` / `k` `↑` | Move 1 |
| `pgdn` `ctrl-d` / `pgup` `ctrl-u` | Move ±10 |
| `g` `home` / `G` `end` | Top / bottom |
| `/` | Start filter (Hosts, Problems, host page Latest data only) |
| `esc` | Host page: back. Elsewhere: clear that view's filter |
| `r` | Refresh now |
| `t` | Cycle theme |
| `o` | Open in frontend |
| `?` | Help |
| `q` / `ctrl-c` | Quit (`ctrl-c` works in every mode) |

Per view:

| View | Keys |
|---|---|
| Dashboard | `⏎`/`l` open the selected host (selection shared with the Hosts view); `m` CPU↔memory chart |
| Hosts | `⏎`/`l` open host; `s` cycle sort (resets selection) |
| Problems | `⏎`/`l` open host; `a` ack dialog; `+`/`s` raise and `-` lower min severity (wraps); `h` hide acked |
| Host page | `[` `]` range (clamped, no wrap); `tab`/`i` toggle tab; `h`/`backspace`/`esc` back |

Filter mode: printable characters append; `backspace` deletes; `⏎` keeps the filter; `esc` clears it and
exits; `↑`/`↓` still move.

Ack modal: characters append; `backspace` deletes; `tab` toggles close (only if allowed); `⏎` sends;
`esc` cancels.

Help modal: `esc`/`?`/`q`/`⏎` close it.

Selections are clamped after every snapshot. All keys act on key press only (repeat and release are
ignored).

## 9. Themes

### 9.1 Built-in palettes **[src]** (bg, fg, red, green, yellow, blue, magenta, cyan, bright_black, accent)

| Name | Values |
|---|---|
| tokyo-night | `#1a1b26 #c0caf5 #f7768e #9ece6a #e0af68 #7aa2f7 #bb9af7 #7dcfff #565f89 #7aa2f7` |
| catppuccin | `#1e1e2e #cdd6f4 #f38ba8 #a6e3a1 #f9e2af #89b4fa #f5c2e7 #94e2d5 #6c7086 #cba6f7` |
| gruvbox | `#282828 #ebdbb2 #fb4934 #b8bb26 #fabd2f #83a598 #d3869b #8ec07c #928374 #fe8019` |
| nord | `#2e3440 #d8dee9 #bf616a #a3be8c #ebcb8b #81a1c1 #b48ead #88c0d0 #616e88 #88c0d0` |
| everforest | `#2d353b #d3c6aa #e67e80 #a7c080 #dbbc7f #7fbbb3 #d699b6 #83c092 #7a8478 #a7c080` |
| rose-pine | `#191724 #e0def4 #eb6f92 #9ccfd8 #f6c177 #31748f #c4a7e7 #ebbcba #6e6a86 #c4a7e7` |
| kanagawa | `#1f1f28 #dcd7ba #e46876 #98bb6c #e6c384 #7e9cd8 #957fb8 #7fb4ca #727169 #7e9cd8` |
| matte-black | `#121212 #bebebe #d35f5f #8fa876 #ffc107 #e68e0d #b0a0c8 #8a8a8d #5c5c5f #f59e0b` |

### 9.2 Derived roles **[src]**

`mix(a,b,t)` is per-channel `round(a + (b-a)·t)`.

- muted = `mix(fg,bg,.45)`
- border = `mix(bright_black,bg,.25)`
- surface (selected row, pills) = `mix(bg,accent,.16)`
- ok = green; warn = yellow; err = red; info = blue
- sev = [`mix(bright_black,fg,.35)`, blue, yellow, `mix(yellow,red,.4)`, `mix(yellow,red,.75)`, red]
- series = [accent, green, magenta, cyan, yellow, red]
- Badge text = bg

Meter levels: ≥90 err, ≥70 warn, else ok. Sparkline colour is accent unless the last value is ≥70.

### 9.3 Omarchy integration: upstream defect found **[doc]**

Upstream reads `~/.config/omarchy/current/theme/` and expects either `colors.toml` with `background`,
`foreground`, `color1`–`color8` (`accent` optional), or `alacritty.toml`. It takes the theme name from
the canonicalised directory name. I checked this against the Omarchy source:

| Omarchy | Current theme dir | Theme files | zabterm 0.1.2 |
|---|---|---|---|
| v3.0.0 | `~/.config/omarchy/current/theme` | `alacritty.toml` shipped | works |
| v3.8.4 | `~/.config/omarchy/current/theme` | `colors.toml` with `color0`–`color15`; `theme.name` file | works |
| **v4.0.6 (latest stable, 2026-10-19)** and v4.5.0-rc2 | **`~/.local/state/omarchy/current/theme`** (real dir, created by `mv`); migration `1781043107.sh` moves and removes the old dir | `colors.toml` with **named keys** (`red`, `blue`, `muted`, `bright_*`, …, no `colorN`); `alacritty.toml` *generated* from `default/themed/alacritty.toml.tpl`; name in `~/.local/state/omarchy/current/theme.name` | **does not detect Omarchy.** `auto` silently becomes tokyo-night and `t` omits `auto` |

I have not observed this on an actual Omarchy 4 machine; it is derived from the Omarchy source at tag
`v4.0.6`. To verify on Omarchy 4: run `ls ~/.config/omarchy/current ~/.local/state/omarchy/current`, then
`zabterm -t auto` and check the help modal's theme name.

Vogter plan:

1. Look for the current theme in `~/.local/state/omarchy/current/theme`, then
   `~/.config/omarchy/current/theme`.
2. Parse `colors.toml` named keys (bright_black ← `muted`, as Omarchy's own Alacritty template maps it),
   then `colorN` keys, then `alacritty.toml`.
3. Take the name from `theme.name` if present, else the directory name.
4. Detect changes by mtime of `colors.toml`/`alacritty.toml`/`theme.name`.

Omarchy exists only on Linux; on other platforms, `auto` = tokyo-night as upstream.

## 10. Notifications **[src]**

Covered by N1–N5 in §5. Deduplication: `seen_events` is replaced with the current problem set on every
snapshot, so an event is announced at most once while it stays present. The first snapshot only seeds
it, so nothing is announced at start-up. Host transitions compare against the previous snapshot only.

Vogter keeps the same model as a pure function `Diff(prev, cur Snapshot, min Severity) []Note`, with
table tests.

## 11. Platform-specific behaviour

| Concern | Upstream Linux | Upstream macOS | Vogter Windows (new) |
|---|---|---|---|
| Desktop notification | `notify-send -a zabterm -u {critical|normal} title body` | `osascript -e 'display notification … with title "zabterm" subtitle …'` (string quoting uses Rust `{:?}`, D11) | Not supported initially; one toast explains it (decision Q6) |
| Open URL | `xdg-open` | `open` | `rundll32 url.dll,FileProtocolHandler <url>` (avoids `cmd` quoting of `&`); unverified until tested on Windows |
| `token_cmd` shell | `sh -c` | `sh -c` | `cmd /C` (decision Q6) |
| Omarchy | yes | n/a | n/a |
| Colour | truecolor | Apple Terminal → 256 | Windows Terminal truecolor; legacy console via Lip Gloss detection; unverified |
| Release targets | x86_64/aarch64 GNU | arm64/x86_64 | Proposed: linux/{amd64,arm64}, darwin/{amd64,arm64}, windows/amd64 (`CGO_ENABLED=0`, static) |

Only Linux can be tested here. macOS and Windows builds will be cross-compiled, but I will claim support
only after running them on real hosts or in CI runners.

## 12. Dependencies

### 12.1 Upstream direct Rust dependencies (licences from crates.io for the locked versions)

| Crate | Version | Licence | Purpose | Go replacement |
|---|---|---|---|---|
| anyhow | 1.0.104 | MIT OR Apache-2.0 | Error context | stdlib `errors`, `fmt.Errorf("%w")` |
| chrono | 0.4.45 | MIT OR Apache-2.0 | Local time formatting | stdlib `time` |
| clap | 4.6.7 | MIT OR Apache-2.0 | CLI parsing | stdlib `flag` with a small subcommand dispatcher |
| crossterm | 0.29.0 | MIT | Terminal I/O | Bubble Tea |
| futures | 0.3.34 | MIT OR Apache-2.0 | Event stream | goroutines and channels |
| ratatui | 0.30.2 | MIT | Layout, widgets, braille chart | Bubble Tea + Lip Gloss + own widgets |
| reqwest | 0.13.5 (rustls, http2, json) | MIT OR Apache-2.0 | HTTP client | stdlib `net/http`, `crypto/tls` |
| serde / serde_json | 1.0.229 / 1.0.151 | MIT OR Apache-2.0 | JSON | stdlib `encoding/json` |
| tokio | 1.53.1 (full) | MIT | Async runtime | goroutines, `context`, `time.Ticker` |
| toml | 1.1.6 (spec 1.1.0) | MIT OR Apache-2.0 | Config and Omarchy theme parsing | `github.com/BurntSushi/toml` |

Transitively, `Cargo.lock` holds **334 packages**, including `ring` 0.17.14 (Apache-2.0 AND ISC) and
`rustls` 0.23.45 (Apache-2.0 OR ISC OR MIT). I did not check the licence of each of the 334 transitive
crates, because none of their code is carried into Vogter.

### 12.2 Essential integrations

- **Terminal:** raw mode, alternate screen, key events, resize, Unicode width.
- **Notifications:** external `notify-send`/`osascript` processes (no library).
- **Configuration:** TOML.
- **OS:** `$HOME`/XDG lookup, file mode 0600, process spawning for `token_cmd`/opener/notifier.

All of these except the terminal and TOML are covered by the Go standard library.

### 12.3 Proposed Go dependencies (versions and `go` directives read from proxy.golang.org on 2026-10-29)

| Module | Version | Released | `go` directive | Licence | Purpose |
|---|---|---|---|---|---|
| `charm.land/bubbletea/v2` | v2.1.0 | 2026-10-08 | **1.26.0** | MIT | Event loop, input, rendering |
| `charm.land/lipgloss/v2` | v2.0.6 | 2026-08-11 | 1.25.0 | MIT | Styling, layout, colour-profile downsampling |
| `github.com/BurntSushi/toml` | v1.6.0 | 2025-12-18 | 1.18 | MIT | Config and Omarchy TOML (no dependencies) |

Not proposed:

- `charm.land/bubbles/v2`: its table and text input don't fit upstream's badge-in-cell rendering, and it
  pulls in clipboard, fuzzy, humanize, harmonica and heredoc.
- `ntcharts`: the braille chart is about 150 lines to write.
- `pelletier/go-toml/v2`: GitHub reports its licence as `NOASSERTION`; I would need to read its LICENSE
  before considering it.

Transitive modules (from the two Charm `go.mod` files): colorprofile, ultraviolet, x/ansi, x/term,
x/termios, x/windows, go-colorful, cancelreader, uniseg, go-runewidth, displaywidth, uax29, xo/terminfo,
go-udiff, golang.org/x/{sys,sync}. All are MIT except go-udiff and golang.org/x/* (BSD-3-Clause), per the
GitHub licence API. Roughly 18 modules against 334 crates. That is an estimate; `go mod graph` in
Milestone 2 will give the exact count.

**Toolchain blocker:**

- The local Go is **1.24.4**. Go 1.24 is out of support: go.dev lists 1.27.3 and 1.26.10 as the
  supported stable releases.
- Bubble Tea v2.1.0 needs Go ≥1.26.0 and Lip Gloss v2.0.6 needs ≥1.25.0.
- `GOTOOLCHAIN=auto` is set, so building would silently download a newer toolchain. That would breach
  the install boundary, so Vogter's Makefile will set `GOTOOLCHAIN=local`.
- Options (decision Q1):
  - (a) Install Go 1.26.x or 1.27.x, then use Charm v2. **Recommended.**
  - (b) Stay on 1.24 with Bubble Tea v1.3.10 (`go 1.24.0`) + Lip Gloss v1.1.0. That means the
    unsupported toolchain and the older Charm API, with a later rewrite to v2.

Bubble Tea v2.1.1 (2026-10-28) is too new to pin under the two-week rule; v2.1.0 is the proposed pin.

## 13. Upstream defects and quirks found

Default handling: preserve user-visible behaviour, fix defects, and document every deviation in
`docs/compatibility.md`. Items marked ⚑ change behaviour and need your decision (Q3).

| ID | Finding | Evidence | Proposed Vogter handling |
|---|---|---|---|
| D1 | Duplicate initial poll: `App::new` sends `Refresh` and the worker's interval also fires immediately | [run] two snapshots 1 ms apart | One initial poll |
| D2 ⚑ | Host page: every snapshot triggers another detail request, and `r` sends both, so detail is fetched twice per poll; the full range (up to 7 days of raw history) is re-read every poll | [run] | One detail fetch per poll. Incremental history (`time_from = last clock + 1`), append and trim. For 24h/7d, use `trend.get` (hourly) per decision Q4 |
| D3 | `user.login` sessions are never closed (`user.logout` is never called, even on quit or re-login). The Zabbix manual says to call `user.logout` | [run], [doc] | `user.logout` on quit and before re-login (best effort, 2 s timeout) |
| D4 | Omarchy 4.x not detected | §9.3 | Support both layouts |
| D5 | Theme name from canonical dir: on Omarchy 4 (real dir) it would read `omarchy:theme` | [doc] inferred | Read `theme.name` |
| D6 | A failed seed `history.get` is swallowed (`unwrap_or_default`), yet the items are marked seeded, so they are never retried | [src] `worker.rs:175-180` | Mark seeded only on success |
| D7 | Seed history always uses `history:0`; a CPU/MEM item of type 3 gets no seed | [src] `worker.rs:175` | Group by `value_type` as the host page does |
| D8 | A problem's host is the first host of its trigger; multi-host triggers are attributed to one host | [src] `worker.rs:197` | Keep, document |
| D9 | `problem.get limit:500`: counts and tiles silently cap at 500 | [src] | Keep the limit; show `500+` when hit |
| D10 | Over 500 problems, any problem pushed out of the top 500 is announced as "Resolved" | [src] | Suppress resolve notes when the limit was hit |
| D11 | macOS AppleScript strings built with Rust `{:?}` escaping (can emit `\u{…}`, which is not AppleScript syntax) | [src] | Proper AppleScript escaping (`\\`, `"`) and pass via `argv` |
| D12 | Host up/down notes are uncapped and ignore `min_severity` | [src] | Keep (documented upstream behaviour) |
| D13 | No `limit` on `host.get`/`item.get`(host)/`history.get` | [src] | Keep for `host.get`; history becomes incremental (D2) |
| D14 | An empty `ZABTERM_TOKEN` or empty `token_env` variable is accepted as a token | [src] | Treat empty as unset; error names the source |
| D15 | `init` always writes the default path, ignoring `-c` | [src] | Honour `-c` |
| D16 | Footer shows the configured interval, not the effective (≥2 s) one | [src] | Show effective |
| D17 | Unknown keys, themes and severities are silently ignored or defaulted | [src] | Warn (§7.2) |
| D18 | Re-login relies on substring matching of error text | [src] | Match on error class (§6.5) |
| D19 | Sparkline buffers are never pruned for removed items | [src] | Prune items absent from the current snapshot |
| D20 | The worker is single-threaded per command: an ack or host detail waits behind a slow poll | [src] | Polls and user actions on separate goroutines sharing the client; poll coalescing (no overlapping polls) |
| D21 | `o` on a problem without a host builds `hostids[]=` (empty) | [src] | Fall back to `problem.view` |

## 14. Proposed Go architecture

### 14.1 Layout

Simplified from the brief: one `zabbix` package instead of `api` + `zabbix`, and screens live inside
`ui` because they share widgets and state.

```
vogter/
├── cmd/vogter/main.go          # CLI dispatch: (no cmd) | init | check | config | import ; -p -c -t -V -h
├── internal/
│   ├── config/                 # schema, load, validate, paths, credential resolution, Secret, template, import
│   ├── zabbix/                 # JSON-RPC client (no TUI imports), typed methods, wire types, error classes, version
│   ├── model/                  # pure: Severity, Availability, HostRow, Problem, Snapshot assembly, filters, sorts,
│   │                           #   net/disk derivation, sparkline store
│   ├── poller/                 # goroutine owning the client: ticker, commands, budgets, back-off, re-login, history cache
│   ├── notify/                 # pure Diff(prev, cur) + desktop backends (build-tagged per OS)
│   ├── theme/                  # palettes, role derivation, Omarchy reader, colour-depth policy
│   ├── platform/               # OpenURL, shell for token_cmd, per-OS files
│   └── ui/                     # Bubble Tea model: app.go (state, keys), dashboard.go, hosts.go, host.go,
│                               #   problems.go, chrome.go (header/footer/splash/toasts/modals),
│                               #   widgets/ (braille chart, sparkline, meter, badge, big digits, table), fmt.go
├── configs/config.example.toml
├── docs/ porting-audit.md  architecture.md  compatibility.md  dependencies.md
├── testdata/                   # JSON fixtures (wire shapes from §6.4)
├── go.mod  go.sum  Makefile  README.md  LICENSE  NOTICE
```

### 14.2 Concurrency

- `poller.Run(ctx, client, cmds <-chan Cmd, out chan<- Event)`. `ui` bridges events with a `tea.Cmd`
  that blocks on `out` and re-arms after each message: the standard Bubble Tea pattern, with no UI-side
  blocking.
- Every request takes a `context.Context`. `http.Client` gets explicit dial, TLS handshake and
  response-header timeouts, plus a 20 s overall per-request deadline (upstream parity).
- The net/disk search gets a 5 s child deadline, with 60 s back-off on failure (parity). Cancelling the
  context also aborts the HTTP request, which upstream's `tokio::time::timeout` does too.
- Quitting cancels the root context, best-effort logs out, and restores the terminal.

### 14.3 State

The `ui` model mirrors upstream `App` fields (view, origin view, modal, toasts, selection per list,
filters, sort, min severity, hide-acked, dashboard metric, detail host, range, host tab, seen events).
Derived lists (`hosts()`, `problems()`, `items()`) become pure `model` functions so they can be
table-tested.

### 14.4 Rendering

- Lip Gloss styles are built once per theme.
- Panels use rounded borders with inline titles (Lip Gloss v2 border title support is unverified; the
  fallback is drawing the top border manually).
- The braille chart is a 2×4-dot canvas per cell with Bresenham lines, axis labels (start, mid, "now")
  and Y-axis "nice ceiling" (1/2/2.5/5×10ⁿ, capped at 100 for percent).
- The sparkline is ported verbatim (window-relative scaling, floor 8).
- Narrow terminals: the upstream breakpoints (130 cols for wide host tables, header tiers) are kept.
  Below ~80×24 a "terminal too small" notice is shown (new).
- ASCII fallback (new): `VOGTER_ASCII=1` replaces braille and block glyphs.

## 15. Test strategy

- **Unit tests (pure):** severity parse/name; availability matrix; root disk (`/` preferred, `/etc/*`
  skipped); net summing (`lo` and 2-param keys skipped); problem→host mapping; sort and filter for every
  mode; selection clamping; dashboard tile maths and the severity bar allocation; formatters (port
  upstream's vectors: `7.8 GiB`, `2.6 Kbps`, `5m`, `3d 2h`, `1.7%`, `10`); `nice_ceil`; sparkline glyph
  output; palette role derivation and 256-colour mapping (upstream's vectors: `(0,0,0)→16`,
  `(255,0,0)→196`, `(128,128,128)→244`); Omarchy parsing for 3.0, 3.8 and 4.x fixtures;
  config validation, precedence and placeholder; `Secret` redaction; notification `Diff` (first-snapshot
  seeding, cap of 3, min severity, suppressed, host transitions, D10 guard).
- **Client:** `httptest.Server` covering Bearer present/absent per method, the `user.login` flow, JSON-RPC
  error mapping into classes, non-2xx, malformed JSON, missing `result`, missing required field, optional
  fields absent, timeout and context cancellation, TLS verify on/off (with a self-signed `httptest` TLS
  server).
- **Poller:** fake clock; single initial poll (D1); net/disk budget and back-off; re-login on session
  expiry; no duplicate detail fetch (D2); incremental history.
- **UI:** drive `Update` with key messages and assert state transitions for every row in §8. Render
  golden snapshots at fixed sizes (ANSI stripped) for each screen.
- **Upstream comparison:** the mock server and fixtures from this audit become `testdata/`. Request
  parameters per method are compared order-insensitively against the log captured from zabterm 0.1.2
  (§6.2), with documented deviations (D1, D2, D3). Screen comparison is limited to key strings, because a
  cell-exact diff would need a terminal emulator library (not proposed).
- **Live Zabbix:** only when an instance and credentials are provided, behind a `-tags live` build tag and
  `VOGTER_LIVE_URL`/`VOGTER_LIVE_TOKEN`.

## 16. Outstanding uncertainties

1. Compatibility with 6.4 and 7.0 is checked against documentation only. Verify with a live instance or
   a `zabbix/zabbix-web-*` container (installing that is your call).
2. Real Zabbix error strings for an expired session, a revoked token and a permission denial on ack are
   unknown. Capture them from a live server.
3. Omarchy 4 behaviour (D4/D5) is derived from source, not observed.
4. Whether Lip Gloss v2.0.6 supports border titles natively is unverified; it decides the panel
   implementation approach, not feasibility.
5. Whether `BurntSushi/toml` v1.6.0 parses every TOML 1.1 construct that Rust `toml` 1.1 accepts (e.g.
   newlines in inline tables) is unverified. It matters only if users write such constructs; I'll add a
   test.
6. Windows terminal behaviour and the browser opener are unverified until run on Windows.
7. The exact Go module count and binary size will be measured in Milestone 2.

Resolved since the audit: (4) Lip Gloss v2.0.6 has no border titles, so panels draw their own frame.

## 17. Decisions needed before Milestone 2

| # | Question | Recommendation |
|---|---|---|
| Q0 | Zabbix target version | **Decided:** 7.0 LTS primary (§6.5) |
| Q1 | Toolchain: install Go 1.26.x/1.27.x (Charm v2), or stay on EOL 1.24.4 (Charm v1)? | **Decided:** install Go 1.27.3 from Debian trixie-backports (`golang-go` 2:1.27~1~bpo13+1 → `golang-1.27` 1.27.3-1~bpo13+1) |
| Q2 | Initialise a git repository in `~/vogter` now? (Nothing has been committed or initialised.) | Yes |
| Q3 | Accept the behaviour-changing fixes D2, D3, D9/D10, D14, D15, D17 as listed? | Yes |
| Q4 | 24h/7d host graphs: keep raw `history.get` (parity) or use `trend.get` (hourly)? | `trend.get` for ≥24h, raw below |
| Q5 | Honour `ZABTERM_URL`/`ZABTERM_TOKEN` as fallbacks, or only `VOGTER_*`? | Only `VOGTER_*`; `vogter import` covers migration |
| Q6 | Windows in scope for notifications and `token_cmd` (`cmd /C`)? | Opener and `token_cmd` yes; notifications disabled with an explanation |

## Appendix A: audit scaffolding (not part of Vogter)

These live in the session scratchpad and are not committed:

- the downloaded zabterm v0.1.2 binary (SHA-256 `f5b947ff7227f805659fea4a7eb035d67676ea8d37f5674cacad98602fcead08`)
- the mock server (`mockzbx`)
- the request logs (`req-check.jsonl`, `req-tui.jsonl`)
- the TUI typescript
- the Zabbix documentation pages
- the Omarchy v4.0.6 source

The request shapes from those logs are reproduced in §6.2. The fixtures will be recreated under
`testdata/` in Milestone 2.
