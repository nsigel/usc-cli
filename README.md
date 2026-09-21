# usc-cli

## What is this?

`usc` is a JSON-first command-line client for USC student services.

Build it with Go 1.24 or later:

```sh
go build -o usc ./cmd/usc
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

Passwords and bypass codes are never written to disk. Brightspace and Handshake
commands use the saved USC session to establish their own application sessions.

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
| `usc auth login [brightspace\|handshake]` | Sign in through USC SSO. Defaults to Brightspace and reuses the saved session unless `--fresh` is passed. |
| `usc auth status [brightspace\|handshake]` | Check whether the saved USC session can authenticate the selected service. |
| `usc auth logout` | Delete the saved USC session. |
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
