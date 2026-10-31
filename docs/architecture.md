# Architecture

```
cmd/vogter          CLI: (none) | init | check | config ; -p -c -t -V -h --force
internal/config     schema, load + validate, paths, credential resolution, Secret, init template
internal/zabbix     JSON-RPC client (no UI imports), typed methods, error kinds, version support
internal/model      pure: joins API responses into hosts and problems, filters, sorts, tiles, sparkline buffers
internal/poller     goroutine owning the client: connect, poll, net/disk budget, seed, coalesce, re-login, logout
internal/notify     pure diff of two snapshots into announcements
internal/platform   desktop notifications and browser opening
internal/theme      built-in palettes and Omarchy themes reduced to UI colour roles
internal/ui         Bubble Tea model: state, keys, screens, panels, tables, braille chart, toasts, modals
legal.go            embeds the licence texts for `vogter licences`
tools/mockzbx       fixture Zabbix server for development and comparison (not shipped)
tools/compare.sh    drives a binary under tmux for screen and request comparison
```

## Data flow

```
          cmds (buffered 1, non-blocking send)
   ui ─────────────────────────────────────────► poller ──► zabbix.Client ──► Zabbix
   ▲                                               │
   └──────────── events (Connected | Snapshot | Failed) ◄┘
```

- `poller.boardLoop` runs beside the poll and detail loops. It follows the Boards screen and makes no call while that
  screen is closed. Its widgets fail one at a time, so a failed widget does not blank the page.
- `poller.Run` polls once at start, then every `refresh_interval` and on each `Refresh`. Requests that
  arrive during a poll become one follow-up poll.
- The UI waits for events in a `tea.Cmd` and re-arms after each one, so rendering and key handling
  never wait on the network.
- All requests take a context. Quitting cancels it. The poller then closes any `user.login` session
  with a fresh 2 s context, and `main` waits up to 3 s for that before exiting.
- `zabbix.Client` returns `*zabbix.Error` with a `Kind` (transport, http, decode, rpc, auth, permission,
  shape). A failed call never returns data, so an error can't look like an empty inventory or a zero
  metric.
- The host page calls `Poller.Watch`. A separate detail loop fetches the watched host after each poll,
  keeping a per-item cache so each fetch reads only new samples. Ranges of 24h and up read `trend.get`,
  then raw history from the end of the last trend bucket.
- Acknowledge calls `Poller.Acknowledge` from a `tea.Cmd`, outside the poll loop. Success requests a
  refresh.
- Rendering builds each screen as a slice of lines. Lip Gloss has no border titles, so `panel` draws its
  own frame. Toasts and modals are Lip Gloss layers over the base screen.
- Credentials are `config.Secret`, which formats as `<redacted>` everywhere. `Reveal()` is called only
  where the value is handed to the client.
