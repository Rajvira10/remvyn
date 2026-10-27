# Contributing to Remvyn

Thanks for helping. Bug reports, fixes, features and docs are all welcome. This page covers how to get set up and what a pull request needs to get merged.

Found a security problem? Don't open an issue; follow [SECURITY.md](SECURITY.md) instead.

## Before you start

- **Small fixes** (typos, obvious bugs, a missing check): open a pull request straight away.
- **Features and larger changes**: open an issue first and describe what you want to build. Remvyn holds people's SSH keys and passwords, so changes to storage, sync, crypto, host-key checks or the AI agent bridge get a closer look, and agreeing on the approach up front saves you rework.
- Check the [Known limitations](README.md#known-limitations) and open issues; someone may already be on it.

## Set up

You need Go ≥ 1.26, Node ≥ 20 and the Wails CLI:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails doctor                  # checks the platform dependencies (WebKitGTK on Linux, WebView2 on Windows)
cd frontend && npm install && cd ..
wails dev                     # the app with hot reload
```

Point it at a throwaway data folder so you never touch your real vault:

```bash
REMVYN_DATA_DIR=/tmp/remvyn-dev wails dev
```

Something to connect to:

```bash
go run ./tools/devsshd -addr 127.0.0.1:2223 -user demo -password demo   # in-process SSH server
go run ./tools/devgithub                                                 # fake GitHub for sync, see README
```

The Android app needs Android Studio (or its bundled JDK 17) and lives in `mobile/android`; see [Android app](README.md#android-app).

## Make your change

The [Architecture](README.md#architecture) section maps the code. A few rules of thumb:

- **Business logic goes in `backend/core`**, not in `app.go`. `app.go` is a thin Wails facade; `core` has no Wails dependency so it can be tested directly.
- **Secrets never reach the frontend** unless the user asked to reveal or copy one. Private keys never leave the Go backend.
- **Anything written to disk that holds a secret is sealed** with the vault key (see `backend/security`). Don't add plaintext secret columns, logs or temp files.
- **Sync format changes need care.** Records written by one version must still open on the others, including the Android app. If you change a record's shape, keep reading the old one, and regenerate the phone's fixture:

  ```bash
  REMVYN_WRITE_FIXTURE=$PWD/mobile/android/app/src/test/resources/vault-fixture.json \
    go test ./backend/vaultsync -run TestWriteMobileFixture
  ```

- **Remote commands are quoted.** Anything that builds a shell command from a file name or user input must go through the existing quoting helpers (`shq` in `backend/files`), and needs a test with a hostile name.
- **Cross-platform.** Remvyn ships on macOS, Linux, Windows and Android. Use `filepath`, not hard-coded `/`; put OS-specific code behind build tags (`_unix.go`, `_windows.go`); and say in the PR which platforms you tried.
- **New dependencies** need a reason. Prefer the standard library and what's already in `go.mod` / `package.json`.
- **Match the code around you**: naming, comment density, error messages written for users ("The server didn't answer in time", not "dial tcp: i/o timeout").

## Test it

Run all of these before you push:

```bash
cd frontend && npm run build && cd ..   # the Go tests embed frontend/dist
go test -race ./...
go vet ./...
gofmt -l .                              # should print nothing
cd frontend && npx tsc -b && npx oxlint src
```

For Android changes:

```bash
cd mobile/android && ./gradlew testDebugUnitTest assembleDebug
```

- New behaviour needs a test. The end-to-end tests run a real SSH server in-process (`backend/testutil`), so most features can be tested without a network; look at `backend/core/core_test.go` for the pattern.
- UI changes: run the app and try them. Include a screenshot or short recording in the PR.

## Update the docs

Every user-visible change updates [README.md](README.md) in the same pull request: the Features list, the section for that area, the shortcuts table, the Tests list, the Architecture tree for new packages, and Known limitations if you removed one. Pull requests that change behaviour without touching the README will be asked to.

## Commits and pull requests

- Branch from `main`. One topic per pull request; split unrelated changes.
- Commit subjects are short, in the imperative, and say what changed for the user, e.g. `Add a built-in file editor and right-click actions to SFTP` or `Android: password vault with two-factor codes`. Use the body for the why.
- Fill in the pull request template: what changed, how you tested it, which platforms.
- Keep pull requests reviewable. A large feature is easier to land as a few steps.
- A maintainer may edit your branch (wording, small fixes) before merging, or squash it.

## Reporting bugs

Open an issue with the bug report template. The most useful reports include:

- Remvyn version (Settings, bottom of the page) and OS.
- What you did, what you expected, and what happened.
- For connection problems: the server's OS and SSH server (`ssh -V` on the server), the auth method, and the exact error text.

**Never paste private keys, passwords, tokens or your `remvyn.db` / data folder into an issue.** Redact host names and IPs if they're sensitive.

## License

Remvyn is [MIT licensed](LICENSE). By submitting a contribution you agree that it is licensed under the same terms, and that you have the right to submit it.

## Code of conduct

Everyone taking part is expected to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
