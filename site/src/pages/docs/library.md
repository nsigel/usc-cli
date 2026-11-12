---
layout: ../../layouts/Docs.astro
title: Go library
description: Use the usc clients from your own Go program.
---

Every command is a call into a Go package; the CLI only parses flags and prints the result. Use the same packages directly:

```sh
go get github.com/nsigel/usc-cli
```

| Package | Entry point |
| --- | --- |
| `brightspace` | `Open(ctx, auth.Options)` |
| `webreg` | `Open(ctx, auth.Options)` |
| `handshake` | `Open(ctx, auth.Options)` |
| `libcal` | `NewPublic()` for schedules, `Open(auth.Options)` for booking |
| `classes` | `New()` |
| `auth` | `Options`, `Open(ctx, site.Name, Options)` for a raw SSO session, `SiteError` |
| `browser` | `SyncCookies(ctx, sessionFile, target)` |
| `skill` | `WriteUSC(writer)` |

## Signing in

Pass credentials directly, or point at the file `usc auth login` saved:

```go
options := auth.Options{Credentials: auth.Credentials{
	Username: netID, Password: password, BypassCode: bypass,
}}

// or
file, err := config.CredentialsPath()
options := auth.Options{CredentialsFile: file}

client, err := brightspace.Open(ctx, options)
courses, err := client.Courses(ctx)
```

The session stays in memory, so each run signs in again. To reuse it between runs, give it a file your program owns:

```go
options.SessionFile = "/var/lib/myapp/usc-session.json"
```

Don't share the `usc` command's `session.json`, and give each concurrently running program its own file. A client is sequential and the last writer wins.

The library never prompts and never reads environment variables.

## Errors

Every sign-in failure is an `*auth.SiteError` naming the site. Check the cause with `errors.Is`:

| Error | Meaning |
| --- | --- |
| `auth.ErrCredentialsRequired` | No credentials were given and there's no valid session |
| `auth.ErrPasswordRejected` | USC rejected the password |
| `auth.ErrBypassRejected` | Duo rejected the bypass code |
| `auth.ErrSessionInvalid` | The site dropped the session after the client opened |

A rejected secret is submitted once. If it came from `CredentialsFile`, it's also removed from that file.

## Examples

```sh
go run ./examples/availability   # today's Leavey rooms, no sign-in
go run ./examples/brightspace    # your enrollments
go run ./examples/webreg         # the newest term's course bin
```
