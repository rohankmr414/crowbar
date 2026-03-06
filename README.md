# 🔧 crowbar

A terminal UI for connecting to any **Source** or **Source 2** dedicated server over RCON. Execute commands with live autocomplete — all from your terminal.

Works with **CS:GO, CS2, TF2, Garry's Mod, L4D2, Rust**, and any other game using the Source RCON protocol.

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/license-MIT-blue)

## Features

- **Live autocomplete** — queries server with `find <prefix>` as you type to discover commands and cvars
- **Real-time log streaming** — receives server logs via UDP and automatically filters out polling noise
- **Raw multi-packet support** — custom TCP RCON parser completely bypasses the `cvarlist` truncation bug found in standard libraries
- **Quality of life** — command history (↑/↓), local display clearing (`Ctrl+L`), disconnected mode, and dynamic game-specific themes

## Install

```bash
# From source
go install github.com/rohankmr414/crowbar@latest

# Or clone and build
git clone https://github.com/rohankmr414/crowbar.git
cd crowbar
go build -o crowbar .
```

## Usage

```bash
crowbar -H <host> -p <port> -P <password> [-l <log-port>]
```

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--host` | `-H` | `127.0.0.1` | Server IP address |
| `--port` | `-p` | `27015` | Server RCON port |
| `--password` | `-P` | *(required)* | RCON password |
| `--log-port` | `-l` | `27115` | Local UDP port for receiving log stream |
| `--public-ip` | | *(auto-detected)* | Your public IP for log streaming |

### Examples

```bash
# Connect to a local server
crowbar -P myrconpassword

# Connect to a remote CS2 server
crowbar -H 192.168.1.100 -p 27015 -P secret

# Connect to a Garry's Mod server on a custom port
crowbar -H 10.0.0.5 -p 27025 -P secret

# Use a custom log port
crowbar -H 10.0.0.5 -P secret -l 27200
```

## Key Bindings

| Key | Action |
|-----|--------|
| `Enter` | Send command / accept autocomplete selection |
| `Tab` | Apply top autocomplete suggestion |
| `↑` / `↓` | Navigate command history or autocomplete list |
| `PgUp` / `PgDn` | Scroll log viewport |
| `Ctrl+L` | Clear local terminal viewport |
| `Esc` / `Ctrl+C` | Quit |

## How It Works

```
┌─────────────────────────────────────────┐
│  Log Viewport (scrollable)              │
│  ── server logs stream here via UDP ──  │
│  ── command responses appear here ──    │
├─────────────────────────────────────────┤
│  ❯ command input (with autocomplete)    │
└─────────────────────────────────────────┘
```

1. **RCON** — connects over TCP using the [Source RCON Protocol](https://developer.valvesoftware.com/wiki/Source_RCON_Protocol)
2. **Log streaming** — auto-detects your public IP and sends `logaddress_add` so the server streams logs via UDP
3. **Autocomplete** — creates one-shot RCON connections to run `find <prefix>`, discovering commands/cvars directly from the server
4. **TUI** — built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Elm-architecture)

## Built With

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) — Terminal styling
- [Bubbles](https://github.com/charmbracelet/bubbles) — TUI components (viewport, text input)
- [pflag](https://github.com/spf13/pflag) — POSIX-style CLI flags

## License

MIT
