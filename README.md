# usc-cli

A JSON-first USC command-line client written in Go with Cobra. This branch is a
history-free rewrite; authentication and profile management are the first
implemented slice.

## Build

```bash
go build -o usc ./cmd/usc
go test ./...
```

## Fast path

No setup command or profile knowledge is required:

```bash
usc login
usc status
usc logout
```

`usc login` prompts for a USC NetID, password, and current Duo bypass code. It
automatically creates the `default` profile and saves only the resulting session.
All normal output is JSON; use `--format human` for terminal-oriented output.

For unattended login, use environment variables rather than command-line secrets:

```bash
export USC_USERNAME=yournetid
export USC_PASSWORD=yourpassword
export USC_DUO_BYPASS=123456789
usc login --non-interactive
```

## Profiles

Named profiles isolate usernames, credentials, and sessions:

```bash
usc profile add grad-school --username yournetid --use
usc login

usc --profile personal login
usc --profile grad-school status
usc profile list
```

The global `--profile` flag wins over `USC_PROFILE`, which wins over the active
profile. Without any of them, `default` is used. A named profile is also created
automatically by `usc --profile NAME login`.

Files live below the platform user-config directory (normally
`~/.config/usc-cli` on Linux and `~/Library/Application Support/usc-cli` on
macOS):

```text
config.json                         active profile name
profiles/<name>/profile.json        non-secret profile metadata
profiles/<name>/session.json        authenticated cookies
profiles/<name>/credentials.json    optional remembered credentials
```

Directories are created with mode `0700` and files with mode `0600`.
Passwords and bypass codes are not stored unless `usc login --remember` is
explicitly used. `usc logout` removes the session; `usc logout --forget` also
removes remembered credentials.

## Authentication command map

Implemented in this first slice:

| Command | Purpose |
| --- | --- |
| `usc login` | Resolve credentials, run USC SSO + Duo, and save the profile session |
| `usc status` | Verify the selected profile's session against WebReg |
| `usc logout [--forget]` | Clear the session and optionally remembered credentials |
| `usc profile add/list/use/show/remove` | Manage isolated profile directories |

The next auth-focused additions should be:

| Command | Purpose |
| --- | --- |
| `usc login --force` | Refresh a session even when its cookies still work |
| `usc auth doctor` | Diagnose config permissions, clock skew, network reachability, and SSO changes without printing secrets |
| `usc credentials set/forget` | Manage remembered credentials without performing a login |
| `usc session export/import` | Explicit, guarded session transfer for CI or another machine |

Course and WebReg data commands should be added only after these authentication
contracts and profile migration behavior are stable.
