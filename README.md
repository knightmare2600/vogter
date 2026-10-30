# Vogter

A keyboard-driven terminal UI for Zabbix, written in Go.

Vogter is an independent port of [zabterm](https://github.com/enderkus/zabterm) by Ender KUŞ. It is not
affiliated with, or endorsed by, the zabterm author. See `NOTICE` and `LICENSE`.

**Status: Milestone 5 of 6.** Every zabterm feature is ported. Only Linux has been run so far, and only
against a fixture server: see `docs/compatibility.md`.

## Build

Go 1.26 or newer is required.

```sh
make            # gofmt check, go vet, go test -race, then build bin/vogter
make cross      # dist/ binaries for linux, darwin and windows
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

Options: `-p NAME` picks a profile, `-c PATH` uses another config file, `-t THEME` overrides the theme.

Credentials are tried in this order: `VOGTER_TOKEN`, `token_env`, `token_cmd`, `token`, then
`username`/`password`.

## Zabbix versions

The target is Zabbix 7.0 LTS. Newer versions are supported. 6.4 runs with a warning that it is untested.
Anything older is refused.

## Documentation

- `docs/porting-audit.md`: audit of zabterm and the porting plan
- `docs/architecture.md`
- `docs/compatibility.md`: deliberate differences from zabterm
- `docs/dependencies.md`
- `docs/upstream-pr-notes.md`: fixes to offer upstream
