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

Give the credentials directly. Or, give the file that `usc auth login` saved:

```go
options := auth.Options{Credentials: auth.Credentials{
	Username: netID, Password: password, BypassCode: bypass,
}}

// or
file, err := usc.CredentialsPath()
options := auth.Options{CredentialsFile: file}

client, err := brightspace.Open(ctx, options)
courses, err := client.Courses(ctx)
```

The client keeps the session in memory. Each run signs in again. To keep the session between runs, give the client a file that your program owns:

```go
options.SessionFile = "/var/lib/myapp/usc-session.json"
```

- Do not share the `session.json` of the `usc` command.
- Give a different file to each program that runs at the same time. A client is sequential. The last write replaces the earlier writes.
- The library never prompts and never reads environment variables.

## Errors

Each sign-in failure is an `*auth.SiteError` that gives the site name. Use `errors.Is` to find the cause:

| Error | Meaning |
| --- | --- |
| `auth.ErrCredentialsRequired` | No credentials and no valid session |
| `auth.ErrPasswordRejected` | USC rejected the password |
| `auth.ErrBypassRejected` | Duo rejected the bypass code |
| `auth.ErrDuoPasscodeRejected` | Duo rejected the generated mobile passcode |
| `auth.ErrSessionInvalid` | The site stopped the session after the client opened |

The client sends a rejected secret only one time. If the secret came from `CredentialsFile`, the client also removes it from that file.

## Mobile ordering

**Warning:** `mobileorder.ContinueOrder` can charge the user.

If the result is `OutcomeUnknown`, do not try again immediately. First, examine the pending and past orders.

## Examples

```sh
go run ./examples/availability   # today's Leavey rooms, no sign-in
go run ./examples/brightspace    # your enrollments
go run ./examples/webreg         # the newest term's course bin
```
