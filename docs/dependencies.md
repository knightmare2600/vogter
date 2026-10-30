# Dependencies

Measured on 2026-10-29 with Go 1.27.3 (`GOTOOLCHAIN=local`). Licences from the GitHub licence API
(SPDX ids), checked during the audit.

## Direct

| Module | Version | Licence | Purpose |
|---|---|---|---|
| `charm.land/bubbletea/v2` | v2.1.0 | MIT | Event loop, input, rendering, terminal restore, colour-profile detection |
| `charm.land/lipgloss/v2` | v2.0.6 | MIT | Styles, layout, borders |
| `github.com/BurntSushi/toml` | v1.6.0 | MIT | Config file (and Omarchy theme files from Milestone 5); no dependencies |
| `github.com/charmbracelet/colorprofile` | v0.4.3 | MIT | Forcing 24-bit or 256 colours via `VOGTER_TRUECOLOR`. It already arrives with Bubble Tea, so it adds nothing |
| `github.com/charmbracelet/x/ansi` | v0.11.9 | MIT | Clipping and wrapping styled text to a width. It already arrives with Bubble Tea, so it adds nothing |

Everything else uses the standard library: HTTP, TLS, JSON, contexts, process execution, CLI flags.

## Linked into the binary (`go version -m bin/vogter`)

18 modules: the five above, plus `charmbracelet/ultraviolet`, `charmbracelet/x/{term,termios,windows}`,
`clipperhouse/{displaywidth,uax29}`, `lucasb-eyer/go-colorful`, `mattn/go-runewidth`, `muesli/cancelreader`,
`rivo/uniseg`, `xo/terminfo` (all MIT) and `golang.org/x/{sys,sync}` (BSD-3-Clause).

`go list -m all` lists 24 modules. The six not linked (including `aymanbagabas/go-udiff` and
`charmbracelet/x/exp/golden`) are used only by dependencies' own tests.

## Size

| | Vogter (M3) | zabterm 0.1.2 |
|---|---|---|
| Linux amd64 binary | 8,978,592 bytes, static (`CGO_ENABLED=0`, `-s -w`) | 7,060,472 bytes, dynamic (libc, libm, libgcc_s) |
| Packages | 18 Go modules | 334 Rust crates |

The Vogter figure covers Milestone 3. I'll measure it again at each milestone.

## Toolchain

Bubble Tea v2.1.0 requires Go ≥ 1.26.0, so `go.mod` declares `go 1.26.0`. The Makefile exports
`GOTOOLCHAIN=local` so a build never downloads a toolchain.
