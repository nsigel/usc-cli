---
name: usc-cli
description: Access USC Brightspace, Handshake events and career fairs, Leavey Library room availability and bookings, and the public Schedule of Classes with the Go-based `usc` CLI. Use when retrieving authorized course data, searching career events, booking or checking study spaces, or checking current sections.
---

# USC CLI

## Preconditions

- Run `usc auth status` before every Brightspace request.
- Run `usc auth status handshake` before every Handshake request.
- Run `usc auth login libcal` before a LibCal booking; availability and room listings are public.
- Treat authentication sessions, passwords, Duo codes, and cookies as secrets. Never print, store, or share them.
- Do not submit coursework, alter enrollment, or send messages.
- Only book a room when the user explicitly requests a reservation. Include `--accept-terms` only when the user has agreed to the reservation terms.

## Locate a course

1. Run `usc brightspace courses`.
2. Match the course code and retain its numeric `id`.
3. Use that ID for Brightspace commands.

## Brightspace

```sh
usc brightspace whoami
usc brightspace courses [--all]
usc brightspace content COURSE_ID [--flat]
usc brightspace grades COURSE_ID [--graded-only]
usc brightspace announcements [COURSE_ID] [--since RFC3339_TIMESTAMP]
usc brightspace assignments COURSE_ID
usc brightspace download COURSE_ID TOPIC_ID [--output DIRECTORY] [--markdown] [--ocr]
```

- Use `content --flat` to locate a file's topic ID; download it by that ID.
- Downloads default to the current working directory. Pass `--output` when a different noncredential directory is required.
- `--markdown` requires Docling; `--ocr` additionally requires `--markdown`.
- Course data is live; the CLI has no content cache.

## Handshake

```sh
usc auth login handshake
usc handshake categories
usc handshake events [--category NAME_OR_ID] [--organizer NAME_OR_EMAIL] [--keyword TEXT] [--medium all|virtual|in-person] [--date all|today|next-10|next-30|past-year] [--posted-by-school] [--sort relevance|date|posted-desc] [--limit N] [--after CURSOR]
usc handshake event EVENT_ID
usc handshake career-fairs [SEARCH_FLAGS]
usc handshake career-fair CAREER_FAIR_ID
```

- Resolve readable category slugs with `handshake categories`; list filters also accept the numeric category IDs.
- Continue pagination by passing `pagination.next_cursor` to `--after` with the same filters and sort.
- Organizer matching checks host, employer, and detail contact names/emails.
- Handshake exposes no event posting timestamp. `posted-desc` uses descending numeric IDs as a new-event monitoring signal; its cursor kind is `id`.
- Event and fair data is live and requires the saved Handshake application session.

## Leavey Library / LibCal

```sh
usc libcal categories
usc libcal spaces rooms
usc libcal room SPACE_ID
usc libcal availability --date tomorrow --after 18:00 --duration 60
usc libcal availability --date tomorrow --after 18:00 --include-pods
usc auth login libcal
usc libcal book --date tomorrow --after 18:00 --duration 60 --accept-terms
```

- Group study rooms are the default. Add `--include-pods` or select `--category pods` only when one-person pods are acceptable; group rooms win ties at the same time.
- Use `--capacity 5-8` or `--capacity 9-12` for group rooms, and `--capacity 1-4` for pods. `--category lvl1|lvl2|lvl3` selects a floor.
- Dates and times use Los Angeles local time. Reservations are limited to two hours per day, one week in advance, and released if the patron does not arrive within ten minutes.
- Booking can require `--name`, `--email`, and repeatable `--field FIELD=VALUE` arguments. `--accept-terms` is an explicit confirmation; the command never prompts.
- Leavey discovery and availability are public; booking requires the USC SSO session. Do not assume a booking succeeded unless the command returns a confirmation object.
- LibCal exposes its SSO handoff only during checkout; `usc auth login libcal` and `usc auth status libcal` briefly stage and release a one-hour room hold without submitting a reservation.

Marshall EMS credential setup is `usc auth marshall user@marshall.usc.edu`. It stores Marshall credentials only; Marshall booking commands are not available yet. EMS currently uses a browser-style HTTP Negotiate/NTLM challenge.


## Browser sync

If `usc auth status` is valid, run `usc browser sync --cdp …`, then use the site's
student/SSO login in Chrome to reach Handshake or other USC apps.


## Public schedule data

```sh
usc classes TERM_CODE COURSE_CODE
usc sites [NAME]
```

- Use `usc classes` for public section times, availability, instructors, and syllabus links.
- This command does not require a saved USC session.

## Output and errors

- Data commands emit JSON to stdout. Use `--json` for compact output or `--pretty` for readable output.
- `usc skill` emits this file as raw Markdown.
- Errors are JSON on stderr and exit with status 1. If an error includes `action`, follow that recovery step when appropriate.
- Only authentication commands may prompt.

## Build and verification

```sh
gofmt -w cmd internal
go test ./...
go vet ./...
go build -o usc ./cmd/usc
```

Run live, authorized checks after changes; never expose authentication data in test output.
