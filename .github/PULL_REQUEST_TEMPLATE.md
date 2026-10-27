## What changed

<!-- What does this do for the user, and why? Link the issue: "Closes #123". -->

## How I tested it

<!-- Commands you ran, and what you tried by hand. Screenshots or a recording for UI changes. -->

- [ ] `go test -race ./...` and `go vet ./...` pass, `gofmt -l .` prints nothing
- [ ] `cd frontend && npx tsc -b && npx oxlint src` pass
- [ ] Android: `./gradlew testDebugUnitTest` passes (if `mobile/` changed)
- [ ] New behaviour has tests

Platforms tried: <!-- macOS / Linux / Windows / Android -->

## Checklist

- [ ] README updated (features, section, shortcuts, tests, architecture, limitations as needed)
- [ ] No secrets in plaintext on disk, in logs, or sent to the frontend without a reveal/copy
- [ ] Sync records still readable by older versions and the Android app (if the sync format changed)
