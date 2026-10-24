# Flux

Flux is a desktop SSH client in the spirit of Termius. It saves your servers, opens tabbed terminals, transfers files over SFTP, manages SSH keys, and runs port-forwarding tunnels. It is built with Wails v2, Go, React, TypeScript, xterm.js and SQLite.

## Features

- **Hosts**: saved servers with labels, groups, colors, startup commands, and password, SSH key or SSH agent authentication. Search, duplicate, import from `~/.ssh/config` (keys referenced by `IdentityFile` are imported too), and quick connect with ⌘T.
- **Terminal**:
  - Tabbed xterm.js sessions (WebGL renderer) with a real PTY (`xterm-256color`, truecolor, Unicode 11) and live resize.
  - Copy and paste, find in scrollback, clickable links, five color schemes, and configurable font, cursor and scrollback.
  - Run a snippet from the terminal toolbar.
  - Dropped connections show a Reconnect overlay; keepalives detect dead links.
- **SFTP**:
  - Dual-pane browser, local and remote, with breadcrumbs, a typed path bar and hidden-file toggle.
  - Multi-select, drag and drop between panes, mkdir, rename, and recursive delete with confirmation.
  - Recursive uploads and downloads with progress, cancellation and overwrite confirmation. Downloads use a temp file and rename, so a cancelled download never leaves a truncated file behind.
- **Keychain**:
  - Generate Ed25519, ECDSA or RSA keys, optionally passphrase-protected, or import OpenSSH/PEM keys from a paste or a file.
  - Copy the public key, install it on a host (like `ssh-copy-id`), or export the private key with `0600` permissions.
- **Port forwarding**: local (`-L`), remote (`-R`) and dynamic SOCKS5 (`-D`) rules, with start/stop, auto-start at launch, live connection and byte counters, and automatic stop with an error if the SSH link drops.
- **Snippets**: saved commands you can send to any open terminal.
- **Known hosts**: trust on first use with an explicit fingerprint check. A changed host key shows a man-in-the-middle warning with both fingerprints, and rejecting it is the default action. You can review and forget trusted hosts.
- **Monitoring**: live CPU, memory, disk, network, Docker containers, services and top processes beside the terminal (free), plus history, alerts, uptime checks and a multi-server dashboard with Flux Pro. See [Monitoring](#monitoring).
- **Settings**: theme, terminal appearance, keepalive and connect timeout, SFTP defaults, close confirmation, and clearing all data.

## Monitoring

Flux monitors Linux servers without an agent. It runs a short, read-only shell script over SSH (`/proc`, `df`, `ps`, `docker` and `systemctl`) and computes CPU and network rates from successive samples. Nothing is installed on the server.

**Free**
- A metrics panel beside every terminal (toolbar **Metrics** or ⌘I). It reuses the terminal's SSH connection, so there's no second login.
- CPU (with load average), memory and swap, every disk, network throughput, Docker containers (with start, stop and restart), systemd services (failed first) and the busiest processes.
- Live view for any host from **Monitoring**, sampled every 2 seconds while you look. Docker and services refresh every 10 seconds.

**Flux Pro**
- **Multi-server dashboard**: choose servers to monitor in the background (every 30 seconds by default) while Flux is open.
- **Historical metrics**: per-minute averages and CPU peaks, charted over 1 hour to 30 days, kept for 30 days by default (up to a year).
- **Custom alerts**: CPU, memory, swap, disk, load, failed services, stopped containers, an unreachable server, or a failing uptime check. Each rule has a threshold and a duration, sends a desktop notification and an in-app alert, and notifies again when it resolves.
- **Uptime monitoring**: HTTP(S) and TCP checks run from this computer, with response time, 24-hour, 7-day and 30-day availability and recent results.

Pro is a local switch (Settings → Flux Pro, or any upgrade button) until payments are added. Pro features are enforced in the backend, not just hidden in the UI.

Background monitoring connects without prompting: it uses an open terminal's connection if there is one, otherwise the host's saved password or key and an already-trusted host key. If something is missing, the dashboard says what to do.

**Limits:** data is only collected while Flux is running. Gaps in history mean the app was closed, and no alerts fire while it's closed. Only Linux servers are supported. Docker data needs the SSH user to be allowed to use Docker; service data needs systemd.

## Run and build

```bash
# prerequisites: Go ≥ 1.25, Node ≥ 20
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails doctor

wails dev                     # hot-reload development
wails build                   # → build/bin/Flux.app (macOS)
open build/bin/Flux.app
```

Set `FLUX_DATA_DIR` to use a different data folder. The default is the OS config dir plus `/Flux`, for example `~/Library/Application Support/Flux`.

### A local server to try it against

```bash
go run ./tools/devsshd -addr 127.0.0.1:2223 -user demo -password demo
```

This is the in-process test server: password auth, a tiny shell, SFTP over your real filesystem, and port forwarding. For a full shell, point Flux at any OpenSSH server.

## Keyboard shortcuts

On macOS these use ⌘. On Linux and Windows they use Ctrl+Shift, so plain Ctrl+W, Ctrl+K and so on still reach the remote shell.

| Action | Shortcut |
| --- | --- |
| Quick connect / new tab | ⌘T |
| Close tab | ⌘W |
| Next / previous tab | ⌘] / ⌘[ |
| Vault / tab 1–9 | ⌘0 / ⌘1–9 |
| Copy / paste in terminal | ⌘C / ⌘V (Ctrl+Shift+C/V elsewhere) |
| Find in terminal / clear | ⌘F / ⌘K |
| Server metrics panel | ⌘I |
| Settings / shortcuts | ⌘, / ⌘/ |

## Tests

```bash
go test -race ./...
cd frontend && npx tsc -b && npx oxlint src
```

`backend/testutil` runs a real SSH server in-process, so the end-to-end tests need no network or external `sshd`. They cover:

- PTY sessions: banner, input, resize, exit codes.
- Host-key trust on first use, and changed-key detection and rejection.
- Password flows: required, wrong, saved, and encrypted at rest.
- Key auth with an encrypted key, plus an unauthorized key being refused.
- Encrypted-key import and its passphrase handling.
- Recursive SFTP upload and download, byte-for-byte, plus rename, delete and transfer cancellation.
- Local, remote and SOCKS forwarding.
- Forward teardown when the connection drops.
- Public-key install.
- Cancelling a connection while its host-key prompt is open.
- Persistence across restarts.
- `~/.ssh/config` import.
- Monitoring:
  - Parsing real-world `/proc`, `df`, Docker and systemd output, including containers and counter resets.
  - Live monitoring that reuses the terminal connection; direct connections, and their errors when credentials or host keys are missing.
  - Container actions and command-injection guards.
  - Pro gating; background monitoring, history, alerts firing and resolving with notifications, and uptime checks.
  - A blocked notifier never stalls alert evaluation.

## Architecture

```text
main.go, app.go        Wails entry point and the bound App facade (dialogs, structured errors)
backend/
  core/                service layer wiring everything together (no Wails dependency)
  sshconn/             dialing, auth methods, keepalive, host-key verification and prompt broker
  terminal/            PTY shell sessions streaming base64 output events
  files/               SFTP and local file operations, recursive transfers with progress
  forward/             -L / -R / SOCKS5 -D forwarding
  keys/                key generation, parsing and passphrases
  sshconfig/           ~/.ssh/config parser
  storage/             SQLite (modernc.org/sqlite, pure Go)
  security/            AES-256-GCM vault for secrets
  testutil/            in-process SSH server for tests
tools/devsshd/         run the test server for manual testing
  monitor/             agentless metrics collection (script, parser, per-host collectors)
  alerts/              alert rule evaluation, firing and resolving
  uptime/              HTTP/TCP uptime probes
frontend/src/          React + TypeScript + Tailwind + Zustand UI (xterm.js terminals)
```

Terminal tabs stay mounted while hidden, so switching tabs never loses scrollback. The frontend subscribes to a session's output events *before* asking the backend to connect, so the login banner is never missed.

## Security

- Passwords, private keys and key passphrases are encrypted with AES-256-GCM before they are written to SQLite. The key lives in a separate `0600` file in the data folder. This protects against leaking the database file, not against someone who can read your home directory; OS keychain integration is a planned improvement.
- Host keys are always verified. New hosts need explicit confirmation, and a changed key is flagged and rejected by default.
- Private keys never leave the Go backend; the UI only ever receives public metadata.
- Forwarding rules bind to `127.0.0.1` by default, and the UI warns before you bind to other interfaces.

## Platform support

| Platform | Status |
| --- | --- |
| macOS (Apple Silicon) | Built and tested: macOS 26, Go 1.26, Wails 2.17. |
| Linux, including Arch | Not yet tested. Needs `gtk3` and `webkit2gtk` (Arch: `pacman -S gtk3 webkit2gtk-4.1`, then build with `-tags webkit2_41`). |
| Windows | Not yet tested. Needs the WebView2 runtime. SSH agent auth is not implemented on Windows. |

## Known limitations

- No jump hosts / ProxyJump, agent forwarding, X11 forwarding or Mosh yet. Hosts imported with `ProxyJump` are flagged during import.
- No local (non-SSH) terminal tabs, split panes, or session logging/recording.
- No cloud sync or team sharing; all data stays on this device.
- Monitoring only runs while Flux is open (no agent or tray mode yet), supports Linux servers only, and Pro has no payment or licensing yet.
- The "install public key" command assumes a POSIX shell on the server.
- The app icon is still the Wails default.
