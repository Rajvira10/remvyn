# Remvyn

Remvyn is a desktop SSH client in the spirit of Termius. It saves your servers, opens tabbed terminals, transfers files over SFTP, manages SSH keys, and runs port-forwarding tunnels. It is built with Wails v2, Go, React, TypeScript, xterm.js and SQLite.

## Features

- **Hosts**: saved servers with labels, groups, colors, startup commands, and password, SSH key or SSH agent authentication. Search, duplicate, import from `~/.ssh/config` (keys referenced by `IdentityFile` are imported too), and quick connect with ⌘T.
- **Terminal**:
  - Tabbed xterm.js sessions (WebGL renderer) with a real PTY (`xterm-256color`, truecolor, Unicode 11) and live resize.
  - Copy and paste, find in scrollback, clickable links, 39 color schemes, and configurable font, cursor and scrollback. The palette button in the terminal toolbar switches scheme (with live preview on hover), text size and cursor without opening Settings.
  - Local terminal tabs: a shell on this computer (your login shell on macOS and Linux; PowerShell, or cmd, on Windows) from the ⌘T palette, the title bar or ⌘⌥T.
  - Split panes: split any terminal tab right (⌘D) or down (⌘⌥D) as often as you like. Each pane is its own session to the same host (or another local shell), with a draggable divider; ⌘⌥ + arrow keys move between panes, and ⌘W closes the focused pane.
  - Run a snippet from the terminal toolbar.
  - Dropped connections show a Reconnect overlay; keepalives detect dead links.
- **SFTP**:
  - Dual-pane browser, local and remote, with breadcrumbs, a typed path bar and hidden-file toggle.
  - Multi-select, drag and drop between panes, mkdir, rename, and recursive delete with confirmation.
  - Recursive uploads and downloads with progress, cancellation and overwrite confirmation. Downloads use a temp file and rename, so a cancelled download never leaves a truncated file behind.
  - Built-in editor: double-click a text file to edit it in a tab, with syntax highlighting, find and replace, and Ctrl/⌘+S. Saves warn if the file changed since you opened it.
  - Open with an installed editor (VS Code, Cursor, Sublime, JetBrains IDEs, …); remote files are uploaded back on every save.
  - Right-click menu: cut, copy, paste, duplicate, copy path, compress to .zip/.tar.gz, extract, new file, rename, permissions and delete.
- **Keychain**:
  - Generate Ed25519, ECDSA or RSA keys, optionally passphrase-protected, or import OpenSSH/PEM keys from a paste or a file.
  - Copy the public key, install it on a host (like `ssh-copy-id`), or export the private key with `0600` permissions.
- **Port forwarding**: local (`-L`), remote (`-R`) and dynamic SOCKS5 (`-D`) rules, with start/stop, auto-start at launch, live connection and byte counters, and automatic stop with an error if the SSH link drops.
- **Passwords**: a password vault for email and website logins, API tokens and secure notes, with a password generator, two-factor (TOTP) codes and extra fields. Encrypted like everything else and synced with the vault. See [Passwords](#passwords).
- **Snippets**: saved commands you can send to any open terminal.
- **Known hosts**: trust on first use with an explicit fingerprint check. A changed host key shows a man-in-the-middle warning with both fingerprints, and rejecting it is the default action. You can review and forget trusted hosts.
- **Monitoring**: live CPU, memory, disk, network, Docker containers, services and top processes beside the terminal, plus history, alerts, uptime checks and a multi-server dashboard. See [Monitoring](#monitoring).
- **Master password and app lock**: optional master password, auto-lock after inactivity, and ⌘L to lock. See [Master password and sync](#master-password-and-sync).
- **AI agents**: let Claude Code (or any MCP client) run commands and edit files on servers you choose. Remvyn makes the connections with its own saved credentials, so the agent never sees a key or password; you can approve each action. See [AI agents](#ai-agents-claude-code).
- **Sync**: keep your vault in sync between computers through a private GitHub repository, end-to-end encrypted. Sign in with GitHub; no server needed.
- **Settings**: theme, terminal appearance, keepalive and connect timeout, SFTP defaults, close confirmation, and clearing all data.

## Monitoring

Remvyn monitors Linux servers without an agent. It runs a short, read-only shell script over SSH (`/proc`, `df`, `ps`, `docker` and `systemctl`) and computes CPU and network rates from successive samples. Nothing is installed on the server.

**Live**
- A metrics panel beside every terminal (toolbar **Metrics** or ⌘I). It reuses the terminal's SSH connection, so there's no second login.
- CPU (with load average), memory and swap, every disk, network throughput, Docker containers (with start, stop and restart), systemd services (failed first) and the busiest processes.
- Live view for any host from **Monitoring**, sampled every 2 seconds while you look. Docker and services refresh every 10 seconds.

**Background**
- **Multi-server dashboard**: choose servers to monitor in the background (every 30 seconds by default) while Remvyn is open.
- **Historical metrics**: per-minute averages and CPU peaks, charted over 1 hour to 30 days, kept for 30 days by default (up to a year).
- **Custom alerts**: CPU, memory, swap, disk, load, failed services, stopped containers, an unreachable server, or a failing uptime check. Each rule has a threshold and a duration, sends a desktop notification and an in-app alert, and notifies again when it resolves.
- **Uptime monitoring**: HTTP(S) and TCP checks run from this computer, with response time, 24-hour, 7-day and 30-day availability and recent results.

Background monitoring connects without prompting: it uses an open terminal's connection if there is one, otherwise the host's saved password or key and an already-trusted host key. If something is missing, the dashboard says what to do.

**Limits:** data is only collected while Remvyn is running. Gaps in history mean the app was closed, and no alerts fire while it's closed. Only Linux servers are supported. Docker data needs the SSH user to be allowed to use Docker; service data needs systemd.

## Install

Remvyn was called Flux during development. On first launch it moves the old `Flux` data folder (hosts, keys, settings) to `Remvyn`, and vaults already synced to GitHub keep working.

Download from [Releases](https://github.com/Rajvira10/remvyn/releases):

- **macOS**: `Remvyn-<version>-macOS-universal.dmg` (Apple Silicon and Intel). Drag Remvyn to Applications. The app isn't notarized yet, so the first time, right-click it and choose **Open** (or run `xattr -dr com.apple.quarantine /Applications/Remvyn.app`).
- **Linux** (x86_64; Arch/Omarchy, Ubuntu 22.04+, Debian 12+, Fedora): `Remvyn-<version>-linux-x86_64.tar.gz`. Install GTK 3 and WebKitGTK 4.1 (Arch/Omarchy: `sudo pacman -S --needed gtk3 webkit2gtk-4.1`), unpack, and run `./install.sh` (per-user, no root). On Arch you can instead build a package with [`packaging/arch/PKGBUILD`](packaging/arch/PKGBUILD) and `makepkg -si`.
- **Windows** (10/11, x64): `Remvyn-<version>-windows-x64-setup.exe` installs Remvyn with a Start menu entry and installs the WebView2 runtime if it's missing. `Remvyn-<version>-windows-x64.zip` is the same app without an installer. Neither is code-signed yet, so SmartScreen may warn; choose **More info → Run anyway**.
- **Android**: `Remvyn-<version>-android.apk` (Android 8+). Open it on your phone and allow installing from your browser or file manager.

## Android app

A companion for when you're away from your computer. It signs in with GitHub, opens the same encrypted vault as the desktop (sync must be on there first), and gives you an SSH terminal to your hosts.

- Hosts with groups and colors, search, and saved passwords and keys (ed25519, ECDSA, RSA, passphrase-protected).
- Terminal: xterm-256color emulator with colors, scrollback, alternate screen (vim, htop, less), pinch to zoom, and an extra-keys row (esc, tab, ctrl, alt, arrows, home/end, page up/down). Snippets from the vault run from the menu.
- Host keys are checked against the vault's known hosts; new ones need confirmation, and a changed key is flagged.
- The vault stays encrypted on the phone and opens only with the master password; Remvyn locks after 5 minutes in the background. The GitHub token is sealed with the Android Keystore. Screenshots of the app are blocked.
- **Read-only vault for now**: add and edit hosts on the desktop. Host keys you trust on the phone are kept on the phone.
- Sessions can drop when Android suspends the app in the background (no foreground service yet). Agent-auth hosts can't be used on the phone.

Build it with Android Studio, or:

```bash
cd mobile/android
./gradlew assembleDebug                                   # app/build/outputs/apk/debug
REMVYN_SIGNING=/path/to/signing.properties ./gradlew assembleRelease
./gradlew testDebugUnitTest                               # opens a vault and keys made by the Go code
```

`signing.properties` holds `storeFile`, `storePassword`, `keyAlias` and `keyPassword`; keep it and the keystore out of the repo. `REMVYN_GITHUB_CLIENT_ID` / `REMVYN_GITHUB_APP_SLUG` build in a different GitHub App, like the desktop.

## Passwords

**Passwords** in the sidebar keeps the logins and secrets that aren't SSH credentials: email accounts, websites, API tokens, recovery codes.

- **Logins** have a username, password, website and an optional two-factor secret. Paste the setup key (or `otpauth://` link) a site offers under its QR code and Remvyn shows the current 6-digit code with a countdown.
- **API tokens** and **secure notes** for everything else. Any item can carry extra named fields (a client ID, an app password), each shown or masked.
- New logins get a generated password (20 characters by default, using the system's secure random generator). Change the length or the character sets in the generator.
- Secrets stay hidden until you click **Reveal**, and hide again after 30 seconds. **Copy** buttons put the value on the clipboard straight from the backend, and Remvyn clears a copied secret after 30 seconds (unless you've copied something else since) and when it locks.
- Every field of an item, including its name, is encrypted with the vault key (AES-256-GCM), stored as two sealed blobs: the details shown in lists and the secrets, which are decrypted only when you reveal, copy or edit them. Locking Remvyn makes them unreadable, and they sync to your other devices end-to-end encrypted. AI agents never see them.
- Set a master password to get the full benefit: without one, the vault key is stored on this computer.

## Master password and sync

**Master password.** Set one in Settings → Security. Remvyn then asks for it at launch, after auto-lock (15 minutes idle by default) and after ⌘L. Open terminals keep running while locked, but they get no input, and new connections, background monitoring and sync wait until you unlock. The data key that encrypts your secrets is wrapped with a key derived from the password (Argon2id, 64 MiB, 3 passes) and is only held in memory while unlocked; changing the password re-wraps it without re-encrypting anything. A forgotten password can't be recovered: the lock screen offers to erase local data and start over.

**Sync with GitHub.** Settings → Sync → *Sign in with GitHub*:

1. Remvyn shows a one-time code; approve it on github.com (GitHub's device flow, so there's no client secret and no Remvyn server).
2. Choose an empty **private** repository. If Remvyn can't see one yet, the dialog links to "new repository" and to the app's install page, where you grant access to just that repository.
3. Set (or confirm) your master password. Remvyn creates the vault and uploads everything, encrypted.

On another computer, sign in, choose the same repository and enter the same master password; its local data is merged in, and it adopts that master password.

What's synced: hosts (with saved passwords), keys (with passphrases), groups, snippets, forwarding rules, known hosts and password vault items. Settings and monitoring history stay per device.

How it works:
- The repository holds `vault.json` (a random sync key wrapped with your master password) and one file per item under `records/`. Each file is AES-256-GCM encrypted and bound to its path; file names are keyed hashes, so GitHub sees only how many items you have and when they change.
- Remvyn uses the Git Data API directly; no git binary or local clone.
- Each device remembers, per item, the version it last agreed with the repository, so it can tell local edits, remote edits and deletions apart. If both sides edited the same item, the newer edit wins. An edit beats a deletion, so nothing is lost silently.
- Sync runs a couple of seconds after a change, every minute, and when the window comes to the front. Commits that race another device are retried.
- Changing the master password re-wraps the key in the repository; other devices keep syncing and use the new password next time they join.
- "Clear all data" and "Reset" disconnect sync first, so a wipe never propagates.

**The GitHub App.** Builds use the public [remvyn-sync-rajvira10](https://github.com/apps/remvyn-sync-rajvira10) app, so anyone can sign in with their own GitHub account and pick their own private repository. It uses GitHub's device flow, doesn't expire tokens, and asks only for *Repository → Contents: read and write* on the repositories you choose. To use a different app, build it in:

```bash
wails build -ldflags "-X main.githubClientID=Iv23li... -X main.githubAppSlug=your-app-name"
```

or set `REMVYN_GITHUB_CLIENT_ID` and `REMVYN_GITHUB_APP_SLUG`. Users can also enter their own app in Settings → Sync. `REMVYN_GITHUB_WEB` / `REMVYN_GITHUB_API` point Remvyn at GitHub Enterprise.

To try sync locally without GitHub:

```bash
go run ./tools/devgithub        # fake GitHub on 127.0.0.1:2280 with octo/remvyn-vault
REMVYN_GITHUB_WEB=http://127.0.0.1:2280 REMVYN_GITHUB_API=http://127.0.0.1:2280 \
  REMVYN_GITHUB_CLIENT_ID=dev REMVYN_GITHUB_APP_SLUG=remvyn-dev REMVYN_DATA_DIR=/tmp/remvyn-a wails dev
```

## AI agents (Claude Code)

Remvyn includes an [MCP](https://modelcontextprotocol.io) server, so Claude Code can work on your servers without you putting SSH keys on the machine it runs on or pasting passwords into it.

```text
Claude Code ──MCP (stdio)──▶ remvyn mcp ──unix socket (0600)──▶ Remvyn app ──SSH──▶ your servers
```

1. Open **AI agents** in the sidebar, turn on **Allow AI agents**, and choose access per server:
   - **No access** (the default): the server is invisible to agents.
   - **Ask first**: every action appears in Remvyn with **Deny**, **Allow once** or **Allow for 30 min**. You also get a desktop notification. Unanswered requests are denied after 5 minutes.
   - **Always allow**: actions run without asking.
2. Run the command shown on that page once. On macOS it is:

   ```bash
   claude mcp add --scope user remvyn -- /Applications/Remvyn.app/Contents/MacOS/Remvyn mcp
   ```

   Other MCP clients use the same command (`<path to Remvyn> mcp`); the page shows the JSON config.
3. Ask Claude something like "check disk space and nginx status on prod-web".

Tools: `list_hosts`, `run_command` (non-interactive, exit code, stdout and stderr, timeout up to 30 minutes), `read_file`, `write_file` (atomic, keeps permissions) and `list_dir`, over SFTP.

Safeguards:

- The agent never receives a password, key or passphrase. Remvyn connects with the vault's credentials and returns only the output.
- An agent alone can never make Remvyn trust a new host key. When you approve an action on a server whose key isn't trusted yet, Remvyn shows you the usual fingerprint check; on "Always allow" servers an unknown key is refused.
- Turning access off, locking Remvyn or auto-lock stops running commands, refuses pending requests and closes agent connections immediately.
- Every action is recorded in the activity log, with its command and output encrypted like your passwords.
- The socket lives in the data folder and only your user account can open it. Access settings are per device and are never synced.

Remvyn has to be running for agents to reach it; otherwise the agent is told to ask you to open it.

## Run and build

```bash
# prerequisites: Go ≥ 1.26, Node ≥ 20
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails doctor

wails dev                     # hot-reload development
wails build                   # → build/bin/Remvyn.app (macOS)
wails build -platform darwin/universal   # Intel + Apple Silicon
packaging/linux/build.sh      # Linux x86_64 tarball, built in Docker (Ubuntu 22.04 base)
wails build -platform windows/amd64 -nsis -webview2 download   # Windows exe + installer (needs makensis)
open build/bin/Remvyn.app
```

Set `REMVYN_DATA_DIR` to use a different data folder. The default is the OS config dir plus `/Remvyn`, for example `~/Library/Application Support/Remvyn`.

### A local server to try it against

```bash
go run ./tools/devsshd -addr 127.0.0.1:2223 -user demo -password demo
```

This is the in-process test server: password auth, a tiny shell, SFTP over your real filesystem, and port forwarding. For a full shell, point Remvyn at any OpenSSH server.

## Keyboard shortcuts

On macOS these use ⌘. On Linux and Windows they use Ctrl+Shift, so plain Ctrl+W, Ctrl+K and so on still reach the remote shell.

| Action | Shortcut |
| --- | --- |
| Quick connect / new tab | ⌘T |
| Close tab (or the focused pane) | ⌘W |
| Local terminal | ⌘⌥T |
| Split right / down | ⌘D / ⌘⌥D |
| Focus the pane left / right / above / below | ⌘⌥ + arrow keys |
| Next / previous tab | ⌘] / ⌘[ |
| Vault / tab 1–9 | ⌘0 / ⌘1–9 |
| Copy / paste in terminal | ⌘C / ⌘V (Ctrl+Shift+C/V elsewhere) |
| Find in terminal / clear | ⌘F / ⌘K |
| Server metrics panel | ⌘I |
| Lock Remvyn (with a master password) | ⌘L |
| Next / previous color scheme | ⌘⌥] / ⌘⌥[ |
| Larger / smaller terminal text | ⌘= / ⌘- |
| Settings / shortcuts | ⌘, / ⌘/ |
| Save in the file editor | ⌘S |
| SFTP: select all / copy / cut / paste | ⌘A / ⌘C / ⌘X / ⌘V |
| SFTP: rename / delete | F2 / Delete or ⌘⌫ |

## Tests

```bash
go test -race ./...
cd frontend && npx tsc -b && npx oxlint src
```

`backend/testutil` runs a real SSH server in-process, so the end-to-end tests need no network or external `sshd`. They cover:

- PTY sessions: banner, input, resize, exit codes.
- Local shells: output, resize, exit status, closing, and refusing duplicate or invalid session IDs.
- Host-key trust on first use, and changed-key detection and rejection.
- Password flows: required, wrong, saved, and encrypted at rest.
- Key auth with an encrypted key, plus an unauthorized key being refused.
- Encrypted-key import and its passphrase handling.
- Recursive SFTP upload and download, byte-for-byte, plus rename, delete and transfer cancellation.
- File operations: local and remote text editing, copy and move, compress and extract round trips, zip-slip rejection, and shell quoting for remote commands.
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
  - Background monitoring, history, alerts firing and resolving with notifications, and uptime checks.
  - A blocked notifier never stalls alert evaluation.
- Master password: set, lock, wrong/right unlock, restart locked, change, remove, and reset after a forgotten password.
- Password vault: items encrypted at rest, create/edit/delete, locked while Remvyn is locked, syncing between devices, and TOTP codes against the RFC 6238 vectors.
- AI agents: off by default, "Ask first" approvals and their timeout, never trusting a new host key, the kill switch, blocking while locked, and a full run over MCP.
- Sync, against an in-process fake GitHub (device flow and Git Data API) with two and three devices:
  - Sign-in, denied sign-in, app not installed, inaccessible and public repositories.
  - Creating and joining a vault, wrong vault password, and that nothing readable reaches the repository.
  - Hosts, passwords, keys, forwards and known hosts arriving intact; edits and deletions in both directions; newer-edit-wins conflicts; idle passes making no commits; retrying after a racing push.
  - Sync pausing while locked, password changes, and "clear all data" not propagating.

## Architecture

```text
main.go, app.go        Wails entry point and the bound App facade (dialogs, structured errors)
backend/
  core/                service layer wiring everything together (no Wails dependency)
  sshconn/             dialing, auth methods, keepalive, host-key verification and prompt broker
  terminal/            PTY shell sessions (SSH or local) streaming base64 output events
  files/               SFTP and local file operations, recursive transfers with progress, remote text
                       editing, copy/move/compress/extract, open with an installed editor
  forward/             -L / -R / SOCKS5 -D forwarding
  keys/                key generation, parsing and passphrases
  sshconfig/           ~/.ssh/config parser
  storage/             SQLite (modernc.org/sqlite, pure Go)
  models/              shared data types
  ids/                 ID generation
  security/            AES-256-GCM vault for secrets; master-password key wrapping (Argon2id)
  totp/                two-factor codes (RFC 6238) for password vault items
  vaultsync/           GitHub sync: device-flow sign-in, Git Data API remote, encrypted records, merge engine
  mcp/                 MCP server for AI agents: stdio protocol, `remvyn mcp` bridge, app socket
  monitor/             agentless metrics collection (script, parser, per-host collectors)
  alerts/              alert rule evaluation, firing and resolving
  uptime/              HTTP/TCP uptime probes
  testutil/            in-process SSH server and fake GitHub for tests
tools/devsshd/         run the test server for manual testing
tools/devgithub/       run the fake GitHub for trying sync
packaging/             Linux tarball build (Docker) and Arch PKGBUILD
mobile/android/        Android companion app (Kotlin, Jetpack Compose, JSch)
branding/              logo, app icons and naming research (BRANDING.md)
frontend/src/          React + TypeScript + Tailwind + Zustand UI (xterm.js terminals)
```

Terminal tabs stay mounted while hidden, so switching tabs never loses scrollback. The frontend subscribes to a session's output events *before* asking the backend to connect, so the login banner is never missed.

## Security

- Passwords, private keys, key passphrases and password vault items are encrypted with AES-256-GCM before they are written to SQLite. Without a master password, the key lives in a separate `0600` file in the data folder, which protects against leaking the database file but not against someone who can read your home directory. With a master password, the key exists on disk only wrapped by an Argon2id-derived key, and in memory only while unlocked. Host names and other non-secret fields of hosts, keys and snippets are not encrypted; password vault items are encrypted in full, names included.
- Synced data is end-to-end encrypted with a key that only exists wrapped by your master password; GitHub stores ciphertext. Records are authenticated, so tampered or swapped files are rejected, but anyone with write access to the repository could delete synced items (and the deletion would sync); Git history keeps the old versions. The GitHub token and sync key are sealed with the local data key, so they're unusable while Remvyn is locked.
- Host keys are always verified. New hosts need explicit confirmation, and a changed key is flagged and rejected by default.
- Private keys never leave the Go backend; the UI only ever receives public metadata.
- AI agents get no access unless you turn it on and open individual servers. They never receive credentials, can't get a new host key trusted without you, and in "Ask first" mode every action waits for your approval. Output from a server can contain text written to mislead an agent (prompt injection), so prefer "Ask first" for servers that matter.
- Forwarding rules bind to `127.0.0.1` by default, and the UI warns before you bind to other interfaces.

## Platform support

| Platform | Status |
| --- | --- |
| macOS (Apple Silicon) | Built and tested: macOS 26, Go 1.26, Wails 2.17. |
| Linux, including Arch/Omarchy | x86_64 release build (WebKitGTK 4.1, built on Ubuntu 22.04 so it runs on newer distros). Launch-tested in an Arch container; not yet tried on real hardware. |
| Windows | x64 release build (cross-compiled from macOS with an NSIS installer). Not yet tested on real hardware. SSH agent auth is not implemented on Windows. Local terminals use ConPTY, which needs Windows 10 1809 or later. Closing the window quits Remvyn, which stops AI agent access. |

## Known limitations

- No jump hosts / ProxyJump, agent forwarding, X11 forwarding or Mosh yet. Hosts imported with `ProxyJump` are flagged during import.
- No session logging/recording.
- Sync needs a GitHub account and a private repository for the vault. Same-item conflicts keep the newer edit; there's no field-level merge. Hosts created separately on two devices with the same name are kept as two hosts. No Touch ID unlock yet.
- No team sharing.
- Monitoring only runs while Remvyn is open (no agent or tray mode yet) and supports Linux servers only.
- The "install public key" command assumes a POSIX shell on the server.
