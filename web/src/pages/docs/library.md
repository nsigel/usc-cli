---
layout: ../../layouts/Docs.astro
title: Go library
description: Use the usc clients from your own Go program.
---

Each command calls a Go package. The CLI only reads the flags and prints the result. You can use the same packages in your program:

```sh
go get github.com/nsigel/usc-cli
```

The import path of each package is `github.com/nsigel/usc-cli/` and the package name. The packages in `internal/` are for the `usc` command only.

| Package | Entry point |
| --- | --- |
| `brightspace` | `Open(ctx, auth.Options)` |
| `webreg` | `Open(ctx, auth.Options)` |
| `handshake` | `Open(ctx, auth.Options)` |
| `directory` | `Open(ctx, auth.Options)` |
| `libcal` | `NewPublic()` for schedules, `Open(auth.Options)` for booking |
| `mobileorder` | `Open(ctx, auth.Options)` for authenticated calls; `New().USC(ctx)` for public campus information |
| `classes` | `New()` |
| `auth` | `Options`, `Open(ctx, auth.Site, Options)` for a raw SSO session, `SiteError` |
| `usc` (the module root) | `SkillMD`, `CredentialsPath()`, `SessionPath()` |

## Sign in

Pass credentials directly (with `DuoTOTPSecret` or `BypassCode`), or point to the file saved by `usc auth login`:

```go
options := auth.Options{Credentials: auth.Credentials{
	Username: netID, Password: password, DuoTOTPSecret: totpSecret,
}}

// or
file, err := usc.CredentialsPath()
options := auth.Options{CredentialsFile: file}

client, err := brightspace.Open(ctx, options)
courses, err := client.Courses(ctx)
```

By default, the client keeps session cookies in memory and signs in on each run. To reuse sessions across runs, give the client a session file that your program owns:

```go
options.SessionFile = "/var/lib/myapp/usc-session.json"
```

- Do not share `usc`'s own `session.json`.
- Give each concurrent process its own session file; clients are sequential and the last write wins.
- The library never prompts and never reads environment variables.

## Errors

Each sign-in failure is wrapped in an `*auth.SiteError` identifying the site. Use `errors.Is` to match the cause:

| Error | Meaning |
| --- | --- |
| `auth.ErrCredentialsRequired` | No credentials and no valid session |
| `auth.ErrPasswordRejected` | USC rejected the password |
| `auth.ErrBypassRejected` | Duo rejected the bypass code |
| `auth.ErrDuoPasscodeRejected` | Duo rejected the mobile passcode |
| `auth.ErrDuoSMSRejected` | Duo rejected the SMS code |
| `auth.ErrDuoPushRejected` | The user did not approve the Duo push |
| `auth.ErrDuoPushTimedOut` | The Duo push was not approved within 90 seconds; also matches `auth.ErrDuoPushRejected` |
| `auth.ErrSessionInvalid` | The site ended the session after the client opened |

A rejected password or bypass code is only sent once; if it came from `CredentialsFile`, the client removes it from that file.

## Mobile ordering

**Warning:** `mobileorder.ContinueOrder` can charge the user.

If the result is `OutcomeUnknown`, check pending and past orders before retrying.

## Examples

```sh
go run ./examples/availability   # today's Leavey rooms, no sign-in
go run ./examples/brightspace    # your enrollments
go run ./examples/webreg         # the newest term's course bin
```
