# Security policy

Remvyn stores SSH keys, server passwords and a password vault, so security reports are taken seriously and handled first.

## Reporting a vulnerability

**Please don't open a public issue, discussion or pull request for a security problem.**

Report it privately through GitHub: go to the repository's **Security** tab and choose **Report a vulnerability** ([direct link](https://github.com/Rajvira10/remvyn/security/advisories/new)). Only the maintainers can see it.

Include as much as you can:

- The affected version(s) and platform (macOS, Linux, Windows, Android).
- What an attacker needs (local access, a malicious server, write access to the sync repository, a malicious MCP client, …) and what they gain.
- Steps to reproduce, or a proof of concept.
- Any suggested fix.

What to expect:

- An acknowledgement within 3 working days.
- An assessment and a plan within 10 working days. Fixes for serious issues ship in a release as soon as they're ready.
- Credit in the release notes and the advisory, if you'd like it.

Please give us a reasonable chance to release a fix before disclosing publicly; we'll agree a date with you, normally within 90 days.

## Supported versions

Only the latest release gets security fixes. Update from [Releases](https://github.com/Rajvira10/remvyn/releases).

## Scope

In scope, for example:

- Reading secrets (passwords, private keys, passphrases, vault items, the GitHub token) without the master password, or while Remvyn is locked.
- Weaknesses in the encryption, key wrapping or sync format (`backend/security`, `backend/vaultsync`, the Android `crypto` package).
- Host-key verification bypasses, or anything that makes Remvyn trust a server key without the user confirming it.
- Command injection through file names, host fields, snippets or monitoring output.
- A malicious server, sync repository or MCP client getting code execution, secrets or access beyond what the user granted. This includes AI agents reaching hosts set to "No access", or bypassing "Ask first".
- Path traversal in SFTP, archive extraction or the editor.

Out of scope:

- Attacks that need malware already running as your user account with full access to your session (see "What Remvyn can't protect against" below).
- Without a master password, the vault key is stored in a `0600` file next to the database. That is documented behaviour, not a vulnerability; set a master password.
- Social engineering, physical access to an unlocked computer, and denial of service against your own machine.
- Missing hardening with no demonstrated impact (e.g. "the app isn't notarized").

## What Remvyn can't protect against

Being upfront about the limits:

- **Malware running as you.** It can log your master password as you type it, read Remvyn's memory while it's unlocked, or use the AI agent socket while Remvyn is unlocked. Keep "Ask first" on for servers that matter, and let auto-lock do its job.
- **A weak master password.** Synced data in GitHub is only as strong as the password that wraps its key, although Argon2id makes guessing slow.
- **A compromised server.** Once you're connected, the server sees everything you type into that session.

See the [Security section of the README](README.md#security) for how secrets are stored and synced.
