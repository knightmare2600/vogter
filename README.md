# Vogter

A keyboard-driven terminal UI for Zabbix, written in Go.

Vogter is an independent port of [zabterm](https://github.com/enderkus/zabterm) by Ender KUŞ. It is not
affiliated with, or endorsed by, the zabterm author. See `NOTICE` and `LICENSE`.

**Status:** every zabterm feature is ported. It has been run on Linux against a fixture server, not yet
against a real Zabbix or on macOS or Windows. See "Not verified" in `docs/compatibility.md`.

## Build

Go 1.26 or newer is required.

```sh
make            # gofmt check, go vet, go test -race, then build bin/vogter
make cross      # dist/ binaries for linux, darwin and windows
make dist       # release binaries and archives, with SHA256SUMS
```

## Use

```sh
bin/vogter init      # writes ~/.config/vogter/config.toml (mode 600)
vim ~/.config/vogter/config.toml
bin/vogter check     # validates the config and tests the server
bin/vogter           # start the UI; ? for keys, q to quit
```

Without a config file:

```sh
VOGTER_URL=https://zabbix.example.com VOGTER_TOKEN=... bin/vogter
```

Coming from zabterm? `bin/vogter import` copies your config across.

`bin/vogter licences` prints Vogter's licence and those of the code it includes, so the binary can be shared alone.

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
