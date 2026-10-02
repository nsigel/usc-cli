# usc-cli

## What is this?

`usc` is a JSON-first command-line client for USC student services.

Build it with Go 1.24 or later:

```sh
go build -o usc ./cmd/usc
```

## Go library

All service clients and shared authentication are importable without running
`usc`. Install the module in your Go project:

```sh
go get github.com/nsigel/usc-cli
```

| Package (under `github.com/nsigel/usc-cli/`) | Entry point |
| --- | --- |
| `brightspace` | `Open(ctx)` for authenticated course data and downloads |
| `handshake` | `Open(ctx)` for authenticated events and career fairs |
| `libcal` | `NewPublic()` for availability; `Open(ctx)` for booking |
| `classes` | `New()` for public course and section data |
| `auth` | `Open(ctx, site.Name, Options)` for USC SSO; `Login` / `LoginFresh` for explicit login |
| `browser` | `SyncCookies(ctx, sessionFile, target)` for Chrome CDP sync |
| `site` | `All()` / `Find(name)` for the site catalog |
| `config` | Shared session, credential, and reservation file paths |
| `skill` | `WriteUSC(writer)` for the embedded agent skill |

Only command parsing, terminal prompts, CLI output formatting, and the CLI's
Docling conversion wrapper remain internal. Brightspace's `Download` method
returns an authenticated stream that applications can save or convert themselves.

```go
client, err := brightspace.Open(ctx)
if err != nil {
    return err
}
courses, err := client.Courses(ctx, false)
```

`brightspace.Open`, `handshake.Open`, and `libcal.Open` share the CLI's session
path, including `USC_CONFIG_DIR`. Brightspace and Handshake establish their
application sessions through USC SSO when opened. LibCal defers authentication
until its checkout handoff; availability does not need login.

For a standalone application, use `OpenWithOptions` (available in all three
packages) with `auth.Options`:

```go
client, err := brightspace.OpenWithOptions(ctx, auth.Options{
    SessionFile: "/private/app/usc/session.json", // optional; defaults to the CLI path
    Credentials: auth.Credentials{
        Username: os.Getenv("USC_USERNAME"),
        Password: os.Getenv("USC_PASSWORD"),
        BypassCode: os.Getenv("USC_DUO_BYPASS"),
    },
})
```

The library never prompts, reads saved passwords, or implicitly loads credentials
from environment variables. Supply credentials explicitly as above when fresh
SSO is needed. Successful authentication persists cookies with private file
permissions; supplied passwords and bypass codes are not saved by these APIs.
Missing credentials return `auth.ErrCredentialsRequired`; LibCal translates this
to `libcal.ErrAuthenticationRequired`. Use `errors.Is` to handle these errors, or use the shared lightweight classifier
for the same JSON shape as the CLI:

```go
// import usc "github.com/nsigel/usc-cli"
info := usc.DescribeError(err, site.Brightspace)
// {"error":"credentials are required","action":"usc auth login"}
```

This only describes the error; it does not recover, retry, or initiate login.
If a later Brightspace or Handshake request returns `ErrSessionInvalid`, reopen
the client. API operations are not automatically replayed.

LibCal stores checkout recovery and booking history beside the selected session
file. `client.Reservations(includePast)` reads that client's history; the
package-level `libcal.Reservations` reads the default configuration directory.
Treat a client/session as sequential: concurrent use and concurrent writes to a
shared session file are not supported. Use separate session files for independent
workers. LibCal clients sharing a directory serialize checkout via its file lock.
Custom transports for `New` use `github.com/saucesteals/fhttp` request/response
types, as does `auth.Session`; these differ from the standard `net/http` types.

Runnable examples:

```sh
go run ./examples/availability  # today's Leavey room intervals, no login needed
go run ./examples/brightspace  # enrollments using saved SSO or explicit env credentials
```

## Features

### Brightspace

| Feature |
| --- |
| User identity |
| Enrollments |
| Course content |
| Grades |
| Announcements |
| Assignments |
| Content downloads |
| PDF-to-Markdown conversion with optional OCR |

### Schedule of Classes

| Feature |
| --- |
| Public course details and section availability |

### Handshake

| Feature |
| --- |
| Live event-category vocabulary |
| Event and career-fair search |
| Category, organizer, keyword, format, date, and USC-posted filters |
| Cursor pagination and newest-ID monitoring order |
| Full event descriptions, contacts, registration state, and locations |
| Full career-fair descriptions, contacts, counts, and sessions |

### Library spaces

| Feature |
| --- |
| Live Leavey room and pod inventory |
| Structured schedules with actual capacities and interval status |
| Explicit-slot booking and inspectable checkout forms |

### Agent skill

Print the bundled agent skill and pipe it into any agent skill system:

```sh
usc skill > SKILL.md
```

## How it works

### Authentication storage

`usc auth login [site]` follows USC's SSO flow through Microsoft and Duo, then saves
the cross-domain cookie session to the platform's user configuration directory:

| Platform | Default session path |
| --- | --- |
| macOS | `~/Library/Application Support/usc/session.json` |
| Linux | `$XDG_CONFIG_HOME/usc/session.json`, or `~/.config/usc/session.json` when `XDG_CONFIG_HOME` is unset |
| Windows | `%AppData%\usc\session.json` |

Set `USC_CONFIG_DIR` to replace the platform-specific `usc` directory. For
example, `USC_CONFIG_DIR=/path/to/config` stores the session at
`/path/to/config/session.json`.

### LibCal reservations

Leavey room discovery and schedules are public. The CLI exposes rooms and pods
without choosing a room, start, duration, or form answer. Agents inspect the
schedule, choose up to two hours of continuous availability, then book the exact
selection. The client handles checkout using direct HTTP requests.

`schedule` (alias `availability`) returns each category's `window_end` and
each space's metadata plus `intervals`: `start`, `end`, `available`, and
the original server `status`. Intervals retain the server's granularity;
missing intervals do not imply availability. The default category is `all`.
`--end-date` is exclusive and defaults to the next day; extend it when
inspecting a schedule across midnight. Capacity filters accept arbitrary bounds:
`--capacity 6-12`, `--min-capacity 6`, or `--max-capacity 12`.

The CLI and package reject any selection longer than two hours locally with
`libcal_duration_limit`, before making requests. This checks the selected
reservation's duration, not cumulative usage across other reservations.

Use full RFC3339 timestamps with UTC offsets, using the schedule's interval
boundaries. The timestamps below are illustrative; choose current
values from live responses.

```sh
usc libcal schedule --date 2026-10-02 --end-date 2026-10-04 --category rooms --min-capacity 6
# If authentication is required, sign in to USC SSO:
usc auth login libcal
# With terms accepted and any required field answers supplied:
usc libcal book --space 31391 --start 2026-10-02T23:00:00-07:00 \
  --end 2026-10-03T01:00:00-07:00 --accept-terms
usc libcal reservations
```

`book` requires `--space`, `--start`, and `--end`; it never selects a
replacement. Repeat `--field FIELD=VALUE` to supply explicit checkout answers.
Required dropdowns without a server-selected answer must be supplied; the CLI
never picks the first available option. Validation errors identify missing fields
and list available choices so the agent can correct the request. `--name` and `--email` are optional
identity conveniences, with explicit `--field` answers taking precedence.
Only authentication commands can prompt.

Booking reuses the saved USC SSO session. `usc auth login libcal` and
`usc auth status libcal` establish/check that shared session through the existing
Handshake SAML entry, without booking flags or room holds. Their status describes
shared USC SSO; `book` completes LibCal's checkout-specific authentication handoff.

The public Go package exposes `Client.Schedule(ctx, ScheduleOptions)` and
`Client.Book(ctx, Selection, ReservationDetails)`. `Selection.Validate()` checks
inputs without making requests.
Use `libcal.NewPublic()` for public reads and `libcal.Open(ctx)` to reuse
the CLI session. The former duration-filtered `Availability` and
`BookEarliest` APIs have been removed.

The `reservations` command reads private local history of bookings confirmed
by this CLI; its `complete` field is false and browser bookings may be absent.
History is stored at `$USC_CONFIG_DIR/libcal-reservations.json` (or the platform
configuration directory when that variable is unset).

Checkout follows the form returned by SSO instead of staging the same booking
again. Failed or interrupted commands release their temporary checkout using
LibCal's session-end endpoint. The CLI saves unfinished checkout IDs privately
and releases them before the next booking, with one checkout allowed at a time.
`usc libcal release` explicitly releases unfinished CLI checkout state; it does
not cancel confirmed reservations.

Errors include an actionable `error` string and a stable `code`, including
`libcal_slot_unavailable`, `libcal_stale_slot`, `libcal_checkout_expired`,
`libcal_authentication_required`, `libcal_booking_limit`,
`libcal_invalid_details`, `libcal_rate_limited`, `libcal_checkout_busy`,
`libcal_cleanup_failed`, `libcal_selected_slot_unavailable`, and
`libcal_booking_unknown`. If submission times out
or the process dies during submission, check the confirmation email before
retrying. Only then use `usc libcal release` to acknowledge the unknown result.
Never automatically retry a submission with an unknown outcome.

A hard kill or lost connection before LibCal returns a checkout ID cannot be
recovered locally; LibCal's temporary hold expiry remains the fallback.


### Marshall EMS credentials

Marshall EMS currently challenges clients with HTTP `Negotiate`/`NTLM`, which
the browser presents as a native authentication dialog. `usc auth marshall`
stores the Marshall email and password in `credentials.json`; it does not yet
provide Marshall booking commands or validate an EMS login. The password is
optional when a USC password is already saved, and is never accepted as a
command-line flag.

```sh
usc auth marshall user@marshall.usc.edu
# Or set USC_MARSHALL_EMAIL and optionally USC_MARSHALL_PASSWORD.
```


### Browser sync

`usc browser sync` copies the saved USC CLI session into Chrome over CDP. If CLI
SSO is still valid, sync then use the site's student/SSO login (e.g. Handshake
**Student Log-in**) to open any USC SSO app in the browser without re-entering
credentials.

```sh
usc browser sync
usc browser sync --cdp 9224
usc browser sync --cdp 127.0.0.1:9224
usc browser sync --cdp http://127.0.0.1:9224
```

`--cdp` accepts a port, `host:port`, an HTTP(S) CDP endpoint, or a full
`ws://` / `wss://` debugger URL. When omitted, the CLI uses `USC_CDP` if set,
otherwise `127.0.0.1:9222`. Some setups (including Grok Bot) expose CDP on
port `9224`.

Start Chrome with remote debugging enabled, for example:

```sh
chromium --remote-debugging-port=9222
```

The command reports attempted/set/skipped counts and the resolved CDP endpoint.
It never prints cookie values.


### Duo bypass code

[Duo](https://itservices.usc.edu/duo/) is USC's multi-factor authentication
service. The CLI uses a Duo bypass code to complete this second authentication
step.

To get one, open the [USC Duo bypass code
page](https://account.usc.edu/2fa/duo-bypass-code), sign in to USC SSO with your
usual Duo method or a passkey, then copy the bypass code. Paste it when
`usc auth login` asks for it.

For non-interactive login, provide all credentials through environment
variables:

```sh
USC_USERNAME=netid \
USC_PASSWORD=password \
USC_DUO_BYPASS=123456789 \
usc auth login handshake --non-interactive
```

After a successful login, the CLI saves the username, password, and bypass
code with mode `0600` in `credentials.json` inside the USC configuration
directory (for example, `~/Library/Application Support/usc/credentials.json`
on macOS). Saved credentials are used for later logins; `--username` and the
`USC_USERNAME`, `USC_PASSWORD`, and `USC_DUO_BYPASS` environment variables
override their corresponding saved values. Brightspace and Handshake commands
use the saved USC session to establish their own application sessions.

### Command data

Brightspace commands read live course data directly from Brightspace's Valence
API at `brightspace.usc.edu`. Downloads are fetched from the authenticated
content URL returned by Brightspace; those URLs do not work without the saved
session. The CLI does not maintain a separate local cache of course data.

`usc classes` reads public course and section data from `classes.usc.edu`. It
does not use or require the saved USC session.

Handshake commands use the same operations as the student events experience at
`usc.joinhandshake.com`. Run `usc auth login handshake` once before using them.
Category filters accept IDs, names, or the slugs returned by
`usc handshake categories`. Native relevance/date ordering uses Handshake's
opaque cursor. The site exposes no event posted timestamp or posted-time sort,
so `--sort posted-desc` orders numeric event IDs descending as a practical
new-event monitoring signal and returns an ID cursor.

```sh
usc handshake categories
usc handshake events --category employers --category networking --organizer vcareers@usc.edu
usc handshake events --category 2,4 --sort posted-desc --limit 25
usc handshake events --after NEXT_CURSOR
usc handshake event 2014893
usc handshake career-fairs --medium virtual
usc handshake career-fair 65867
```

## Command reference

| Command | Description |
| --- | --- |
| `usc auth login [brightspace\|handshake\|libcal]` | Sign in through USC SSO. Defaults to Brightspace; no booking flags required. |
| `usc auth status [brightspace\|handshake\|libcal]` | Check the saved session; LibCal checks shared USC SSO without staging a room. |
| `usc auth marshall [EMAIL]` | Save Marshall EMS credentials; the password defaults to the saved USC password. |
| `usc auth logout` | Delete the saved USC session. |
| `usc browser sync [--cdp TARGET]` | Copy the saved USC session into Chrome over CDP. Never prints cookie values. |
| `usc brightspace whoami` | Show the authenticated Brightspace user. |
| `usc brightspace courses [--all]` | List course enrollments. |
| `usc brightspace content COURSE_ID [--flat]` | Show a course's content table of contents. |
| `usc brightspace grades COURSE_ID [--graded-only]` | Show grades for a course. |
| `usc brightspace announcements [COURSE_ID] [--since TIME]` | Show announcements for one course or all courses. |
| `usc brightspace assignments COURSE_ID` | List assignment folders for a course. Alias: `dropbox`. |
| `usc brightspace download COURSE_ID TOPIC_ID [--output DIR] [--markdown] [--ocr]` | Download a content item, optionally converting a PDF to Markdown. Markdown conversion requires [Docling](https://docling-project.github.io/docling/) (`pip install docling`); `--ocr` requires `--markdown`. |
| `usc handshake categories` | List live event category IDs, names, and CLI slugs. |
| `usc handshake events [FILTERS]` | Search events. Filters: `--category`, `--organizer`, `--keyword`, `--medium`, `--date`, `--posted-by-school`, `--sort`, `--limit`, and `--after`. |
| `usc handshake event EVENT_ID` | Show the full event description, contacts, employers, location, and registration state. |
| `usc handshake career-fairs [FILTERS]` | Search career fairs with the event-list filters. Alias: `fairs`. |
| `usc handshake career-fair CAREER_FAIR_ID` | Show career-fair details and sessions. Alias: `fair`. |
| `usc libcal categories` | List Leavey room and pod categories. |
| `usc libcal spaces [rooms\|pods\|all\|lvl1\|lvl2\|lvl3]` | List Leavey spaces. |
| `usc libcal room SPACE_ID` | Show a Leavey space's details. |
| `usc libcal schedule [FILTERS]` | Return server intervals and actual capacities; alias: `availability`. |
| `usc libcal book --space ID --start TIMESTAMP --end TIMESTAMP` | Book only the explicit selection; requires `--accept-terms`. |
| `usc libcal reservations [--all]` | List upcoming or all reservations confirmed by this CLI. |
| `usc libcal release` | Release unfinished CLI checkout state; never cancels confirmed reservations. |
| `usc classes TERM_CODE COURSE_CODE` | Show a public Schedule of Classes course and all of its sections. |
| `usc sites [NAME]` | List supported USC sites, or show one site. |
| `usc skill` | Print the bundled `SKILL.md` for use with agent skill systems. |
| `usc version` | Print version information. |

## Output format

Every command except `usc skill` writes JSON to stdout. `usc skill` writes its
raw Markdown so it can be piped directly into an agent skill system. JSON output
is pretty-printed when stdout is an interactive terminal and compact when it is
piped or redirected. Pass `--json` to force compact JSON or `--pretty` to force
pretty-printed JSON; the flags are mutually exclusive.

Errors are written as JSON to stderr and commands return exit code 1. When the
CLI knows how to recover, the error object also includes an `action` command:

```json
{"error":"authentication required","action":"usc auth login"}
```

Only authentication commands prompt for input.

## License

This project does not currently include a license.
