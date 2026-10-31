# Vægter

A keyboard-driven terminal UI for **Zabbix**, written in Go: hosts, problems, acknowledgements, graphs and your own
Zabbix dashboards, in a single static binary, with no browser in the loop.

Vægter is an independent port of [zabterm](https://github.com/enderkus/zabterm) by Ender KUŞ. It is not
affiliated with, or endorsed by, the zabterm author. See `NOTICE` and `LICENSE`.

## The name

*Vægter* (`[ˈvεgdʌ]`) is Danish for **watchman**: per
[Den Danske Ordbog](https://ordnet.dk/ddo/ordbog?query=v%C3%A6gter), the guard who kept order in a market town at night,
watched for fire and sang a verse on every stroke of the hour, and today the guard who makes the rounds of a workplace
after hours. Keeping watch through the night and raising the alarm is roughly the job this program does for a Zabbix
estate.

The project's name is Vægter, and that is what the app itself shows. *Vogter* is only the anglicised spelling for
GitHub, which also names the `VOGTER_*` variables and `~/.config/vogter/`. The binary and release files are `vaegter`,
the usual ASCII spelling of æ.

| Danish | English | Notes |
|---|---|---|
| vægter | watchman | the name; from Low German *wachter*, and related to *vagt*, watch |
| vogter | guard, keeper | the spelling used for GitHub, and a Danish word of its own from the same root |

The old job survives in [Ebeltoft](https://www.visitaarhus.com/aarhus/planlegg-reisen/nightwatchmen-old-ebeltoft-gdk606173):
watchmen are recorded there from the late 1600s, and they once patrolled every hour, calling out the time. These days
they set off from the Old Town Hall on summer evenings and stop at restaurants to sing. Vægter patrols every 10 seconds
by default and, sadly, does not sing. It only reads, too: its one change to Zabbix data is an acknowledgement you ask
for.

## Status

| | Linux | macOS | Windows |
|---|---|---|---|
| Build and unit tests (CI) | ✅ | ✅ | ✅ |
| Run against real Zabbix 7.0.32 in a test lab | ✅ | ❌ not yet tested | ❌ not yet tested |
| Run against a production Zabbix server | ❌ not yet tested | ❌ not yet tested | ❌ not yet tested |

Every zabterm feature is ported. See `docs/compatibility.md` for what has been verified, what has not, and how Vægter
differs from zabterm.

## Build

Go 1.26 or newer is required.

```sh
make            # gofmt check, go vet, go test -race, then build bin/vaegter
make cross      # dist/ binaries for linux, darwin and windows
make dist       # release binaries and archives, with SHA256SUMS
```

## Use

```sh
bin/vaegter init      # writes ~/.config/vogter/config.toml (mode 600)
vim ~/.config/vogter/config.toml
bin/vaegter check     # validates the config and tests the server
bin/vaegter           # start the UI; ? for keys, q to quit
```

Without a config file:

```sh
VOGTER_URL=https://zabbix.example.com VOGTER_TOKEN=... bin/vaegter
```

Coming from zabterm? `bin/vaegter import` copies your config across.

`bin/vaegter licences` prints Vægter's licence and those of the code it includes, so the binary can be shared alone.

Press `4` for Boards: your Zabbix global dashboards, read-only. Widgets a terminal cannot draw, such as maps, show a
placeholder, and `o` opens the page in Zabbix. See `docs/compatibility.md`.

Options: `-p NAME` picks a profile, `-c PATH` uses another config file, `-t THEME` overrides the theme.

The mouse wheel is off by default, because taking the mouse stops click-and-drag text selection. Set `mouse = true` in
the config, or press `M` while running. Lists scroll with `j k`, page up and down, and `ctrl-d ctrl-u`.

Credentials are tried in this order: `VOGTER_TOKEN`, `token_env`, `token_cmd`, `token`, then
`username`/`password`.

## Environment

| Variable | Effect |
|---|---|
| `VOGTER_URL`, `VOGTER_TOKEN` | Run without a config file; the token overrides the active profile |
| `VOGTER_TRUECOLOR` | `1` forces 24-bit colour, `0` forces 256 colours |
| `VOGTER_ASCII` | `1` uses only ASCII glyphs |
| `NO_COLOR` | Turns colour off, and beats `VOGTER_TRUECOLOR` |

## Zabbix versions

The target is Zabbix 7.0 LTS. Newer versions are supported. 6.4 runs with a warning that it is untested.
Anything older is refused.

## Documentation

- `docs/porting-audit.md`: audit of zabterm and the porting plan
- `docs/architecture.md`
- `docs/compatibility.md`: deliberate differences from zabterm
- `docs/dependencies.md`
- `docs/upstream-pr-notes.md`: fixes to offer upstream
