# usc-cli

## What is this?

`usc` is a JSON-first command-line client for USC student services.

Build it with Go 1.24 or later:

```sh
go build -o usc ./cmd/usc
```

## Go library

Every CLI feature is a library call; commands only parse flags and print JSON.

```sh
go get github.com/nsigel/usc-cli
```

| Package (under `github.com/nsigel/usc-cli/`) | Entry point |
| --- | --- |
| `brightspace` | `Open(ctx, auth.Options)` for course data and downloads |
| `handshake` | `Open(ctx, auth.Options)` for events and career fairs |
| `webreg` | `Open(ctx, auth.Options)` for Web Registration catalog, course bin, and checkout |
| `libcal` | `NewPublic()` for availability; `Open(auth.Options)` for booking |
| `classes` | `New()` for public course and section data |
| `auth` | `Options` for signing in; `Open(ctx, site.Name, Options)` for a raw SSO session; `SiteError` |
| `browser` | `SyncCookies(ctx, sessionFile, target)` for Chrome CDP sync |
| `site` | Site names used by `auth.Open` and `auth.SiteError` |
| `config` | `CredentialsPath()` and `SessionPath()`, the `usc` command's files |
| `skill` | `WriteUSC(writer)` for the embedded agent skill |

A program signs in with `auth.Options` and owns its session; nothing is shared
with the `usc` command unless you point at its files. Give it credentials in
one of two ways:

```go
// Username, password, and Duo bypass code directly:
options := auth.Options{Credentials: auth.Credentials{
    Username: netID, Password: password, BypassCode: bypass,
}}

// Or the credentials file that `usc auth login` saved:
file, err := config.CredentialsPath() // the usc command's file; any path works
options := auth.Options{CredentialsFile: file}

client, err := brightspace.Open(ctx, options)
courses, err := client.Courses(ctx)
```

By default the session's cookies live in memory, so each run signs in again.
Set `SessionFile` to a file your program owns to keep them between runs; later
runs then need credentials only when that session has expired:

```go
options.SessionFile = "/var/lib/myapp/usc-session.json"
```

Do not use the `usc` command's `session.json` as a `SessionFile`, and give each
concurrently running program its own file: a client is sequential and the
last writer wins.

The library never prompts and never reads environment variables. Every
authentication failure is an `*auth.SiteError` naming the site and wrapping
`auth.ErrCredentialsRequired`, `auth.ErrPasswordRejected`,
`auth.ErrBypassRejected`, or, when a site drops its session after the client
was opened, `auth.ErrSessionInvalid`. A rejected secret is submitted once; if
it came from `CredentialsFile` it is also removed from that file. Check for
these with `errors.As` and `errors.Is`.

Clients built with `New` take any `Do(*http.Request)` implementation using
`github.com/saucesteals/fhttp` types, which `auth.Session` provides.

Runnable examples:

```sh
go run ./examples/availability  # today's Leavey room intervals, no login needed
go run ./examples/brightspace  # enrollments, using the credentials usc auth login saved
go run ./examples/webreg       # the newest term's course bin, same credentials
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
| Assignment submissions and submitted-file downloads |
| Content downloads |

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

### Web Registration

| Feature |
| --- |
| Registration terms and per-term schools and program codes |
| Program or course catalog with sections, seats, clearance, and course-bin membership |
| myCourseBin with each section's scheduled/registered state and grade option |
| myCalendar meetings with conflict status |
| Add, schedule, unschedule, and remove course-bin sections |
| Grade-option changes |
| Checkout preview and explicitly confirmed submission |

### Agent skill

Print the bundled agent skill and pipe it into any agent skill system:

```sh
usc skill > SKILL.md
```

## How it works

### Authentication

```sh
usc auth login            # prompts for NetID, password, and Duo bypass code
usc auth status           # {"authenticated":true,"credentials_saved":true}
usc auth logout           # deletes the session and the saved credentials
```

`usc auth login` follows USC's SSO flow through Microsoft and Duo and saves
the credentials. One sign-in covers every site. After that, every command
signs itself in: it reuses the saved session and repeats the login with the
saved credentials when the session has expired. Only `usc auth login` prompts.

Two private (`0600`) files in the user configuration directory hold the sign-in:

| File | Contents |
| --- | --- |
| `session.json` | The cross-domain cookie session |
| `credentials.json` | NetID, password, and Duo bypass code |

| Platform | Directory |
| --- | --- |
| macOS | `~/Library/Application Support/usc/` |
| Linux | `$XDG_CONFIG_HOME/usc/`, or `~/.config/usc/` |
| Windows | `%AppData%\usc\` |

Set `USC_CONFIG_DIR` to use another directory.

[Duo](https://itservices.usc.edu/duo/) is USC's multi-factor service, and the
CLI completes it with a bypass code. To get one, open the [USC Duo bypass code
page](https://account.usc.edu/2fa/duo-bypass-code), sign in, and copy the code.

Credentials in the environment override saved values for any `usc` command, so
a shell script or CI job can skip `usc auth login` and save no credentials:

```sh
USC_USERNAME=netid USC_PASSWORD=password USC_DUO_BYPASS=123456789 \
usc brightspace courses
```

`usc auth login` only prompts on a terminal. Anywhere else it uses those
variables and reports which ones are missing.

If USC rejects the password or Duo rejects the bypass code, the command stops
after that single attempt and removes the rejected saved value, so the next
`usc auth login` prompts for a replacement. Secrets are never resubmitted
automatically and are never accepted as command-line flags.

### LibCal reservations

Leavey room discovery and schedules are public. `schedule` returns each space's
`intervals` with `start`, `end`, `available`, and the server's `status`;
`--end-date` is exclusive and defaults to the next day. `book` reserves exactly
the space and span you name, up to two hours, and never substitutes another.

```sh
usc libcal schedule --date 2026-10-02 --category rooms --min-capacity 6
usc libcal book --space 31391 --start 2026-10-02T15:00:00-07:00 \
  --end 2026-10-02T17:00:00-07:00 --accept-terms
```

Timestamps are RFC3339 with a UTC offset, as the schedule prints them. If
LibCal's checkout form asks a question, the error names the field and its
choices; answer with `--field NAME=VALUE`.

LibCal has no API that lists a patron's bookings, so the confirmation email is
the record of a reservation. A failed or interrupted booking releases its
temporary hold. If a booking's outcome is unknown, further bookings are
refused until `usc libcal release` acknowledges it.

The Go package exposes `Client.Schedule(ctx, ScheduleOptions)` and
`Client.Book(ctx, Selection, ReservationDetails)`.

### WebReg registration

WebReg keeps an active term in its server session, so every term-scoped command
takes `--term` (a code from `usc webreg terms`) and selects it first. Section IDs
are the numbers shown in `courses` and `bin`.

Registration is a two-step flow, as in the browser. Course-bin commands only
plan changes: `add` places a section in myCourseBin already scheduled,
`schedule` marks it for registration, and `unschedule` on a registered section
marks it to be dropped. `checkout` previews exactly what would be submitted.
`submit --confirm` sends it; WebReg does not process partial registrations, so a
failure such as a closed section changes nothing and is reported in `messages`.

```sh
usc webreg terms
usc webreg courses --term 20271 --course BUAD-306
usc webreg add 14790 --term 20271
usc webreg checkout --term 20271
usc webreg submit --term 20271 --confirm
```

`grade-option SECTION_ID CODE` accepts a code from the section's
`grade_option.choices`. Unregistered sections change immediately; registered
sections (`change_request: true`) submit a request that takes effect at checkout.

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
`ws://` / `wss://` debugger URL, and defaults to `127.0.0.1:9222`.

Start Chrome with remote debugging enabled, for example:

```sh
chromium --remote-debugging-port=9222
```

The command reports attempted/set/skipped counts and the resolved CDP endpoint.
It never prints cookie values.


### Command data

Brightspace commands read live course data directly from Brightspace's Valence
API at `brightspace.usc.edu`. Downloads are fetched from the authenticated
content URL returned by Brightspace; those URLs do not work without the saved
session. The CLI does not maintain a separate local cache of course data.

`usc classes` reads public course and section data from `classes.usc.edu`. It
does not use or require the saved USC session.

Handshake commands use the same operations as the student events experience at
`usc.joinhandshake.com`.
Lists return one page as `items` with `has_more` and `next_cursor`; pass
`next_cursor` to `--after` with the same filters for the next page. Category
filters accept IDs, names, or the slugs returned by `usc handshake categories`.
`--organizer` matches a host, employer, or contact name or email. Native relevance/date ordering uses Handshake's
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
| `usc auth login` | Sign in to USC and save the credentials that every later command reuses. |
| `usc auth status` | Report whether the saved session is valid and whether credentials are saved. |
| `usc auth logout` | Delete the saved USC session and credentials. |
| `usc browser sync [--cdp TARGET]` | Copy the saved USC session into Chrome over CDP. Never prints cookie values. |
| `usc brightspace whoami` | Show the authenticated Brightspace user. |
| `usc brightspace courses [--all]` | List enrolled classes; `--all` adds departments, semesters, and groups. |
| `usc brightspace content COURSE_ID [--flat]` | Show a course's content table of contents. |
| `usc brightspace grades COURSE_ID [--graded-only]` | Show grades for a course. |
| `usc brightspace announcements [COURSE_ID] [--since TIME]` | Show announcements for one course or all courses. |
| `usc brightspace assignments COURSE_ID` | List assignment folders for a course. |
| `usc brightspace submissions COURSE_ID FOLDER_ID [--download] [--output DIR]` | List your submissions to an assignment folder, or download every submitted file. |
| `usc brightspace download COURSE_ID TOPIC_ID [--output DIR]` | Download a content item. |
| `usc handshake categories` | List live event category IDs, names, and CLI slugs. |
| `usc handshake events [FILTERS]` | Search events. Filters: `--category`, `--organizer`, `--keyword`, `--medium`, `--date`, `--posted-by-school`, `--sort`, `--limit`, and `--after`. |
| `usc handshake event EVENT_ID` | Show the full event description, contacts, employers, location, and registration state. |
| `usc handshake career-fairs [FILTERS]` | Search career fairs with the event-list filters. |
| `usc handshake career-fair CAREER_FAIR_ID` | Show career-fair details and sessions. |
| `usc libcal categories` | List Leavey room and pod categories. |
| `usc libcal spaces [rooms\|pods\|all\|lvl1\|lvl2\|lvl3]` | List Leavey spaces. |
| `usc libcal space SPACE_ID` | Show one Leavey room or pod. |
| `usc libcal schedule [FILTERS]` | Show each space's available and unavailable intervals. Filters: `--date`, `--end-date`, `--category`, `--min-capacity`, `--max-capacity`. |
| `usc libcal book --space ID --start TIMESTAMP --end TIMESTAMP` | Book only the explicit selection; requires `--accept-terms`. |
| `usc libcal release` | Release unfinished CLI checkout state; never cancels confirmed reservations. |
| `usc webreg terms` | List registration terms and their codes. |
| `usc webreg programs --term TERM` | List schools and program codes. |
| `usc webreg courses --term TERM (--program CODE\|--course CODE)` | List courses with sections, seats, clearance, and course-bin membership. |
| `usc webreg bin --term TERM` | Show myCourseBin with scheduled/registered state and grade options. |
| `usc webreg calendar --term TERM` | Show weekly meetings with scheduled, registered, and conflict status. |
| `usc webreg add\|schedule\|unschedule\|remove SECTION_ID --term TERM` | Change a course-bin section. These plan changes; only `submit` registers or drops. |
| `usc webreg grade-option SECTION_ID CODE --term TERM` | Change a course-bin section's grade option. |
| `usc webreg checkout --term TERM` | Preview pending registrations and drops without submitting. |
| `usc webreg submit --term TERM --confirm` | Submit checkout; changes enrollment. |
| `usc classes TERM_CODE COURSE_CODE` | Show a public Schedule of Classes course and all of its sections. |
| `usc skill` | Print the bundled `SKILL.md` for use with agent skill systems. |
| `usc version` | Print the module version and commit this binary was built from. |

## Output format

Every command except `usc skill` writes JSON to stdout: exactly the value the
corresponding library call returns, so a list prints as a JSON array. `usc skill` writes its
raw Markdown so it can be piped directly into an agent skill system. JSON output
is pretty-printed when stdout is an interactive terminal and compact when it is
piped or redirected. Pass `--compact` to force single-line JSON or `--pretty` to
force indented JSON; the flags are mutually exclusive.

Errors are written as JSON to stderr and commands return exit code 1. When a
command resolves the error, the object names it in `action`:

```json
{"error":"brightspace: USC sign-in is required","action":"usc auth login"}
```

Only `usc auth login` prompts for input.

## License

MIT. See [LICENSE](LICENSE).
