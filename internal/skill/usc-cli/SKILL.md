---
name: usc-cli
description: Access USC Brightspace, Handshake events and career fairs, Leavey Library room availability and bookings, and the public Schedule of Classes with the Go-based `usc` CLI. Use when retrieving authorized course data, searching career events, booking or checking study spaces, or checking current sections.
---

# USC CLI

## Preconditions

- Run `usc auth status` before every Brightspace request.
- Run `usc auth status handshake` before every Handshake request.
- LibCal discovery is public. Inspect room types and availability before booking; authenticate when the saved session is missing or the command reports `libcal_authentication_required`.
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

The Leavey catalog is stable enough to use directly:

| Category | Spaces |
| --- | --- |
| `lvl1`, group rooms | 31391: 113B (6); 35418: 113C (8); 31392: 113D (6); 31393: 113E (6); 31394: 113F (6) |
| `lvl2`, group rooms | 18361: 201A (5); 18362: 201B (5); 18363: 201C (5); 18364: 201E (5); 18365: 201F (5); 18366: 201G (5); 18367: 202B (5); 18368: 202C (5); 18369: 202D (5); 18370: 202E (5); 18371: 202F (5); 18372: 202G (5); 18373: 202H (5); 18360: 202I (12) |
| `lvl3`, group rooms | 18412: 301C (5); 18413: 301D (5); 18414: 301E (5); 18415: 301F (5); 18418: 302C (10) |
| `pods`, one person | 233278: 210-A; 233279: 210-B; 233280: 210-C; 209556: 310-A; 233276: 310-B; 233277: 310-C; 211079: 310-D |

Numbers in parentheses are capacities. `rooms` means all group-room floors;
`all` also includes pods. Use `categories`, `spaces`, or `room` only to refresh
or verify this catalog when the live site may have changed.

For every availability or booking request:

1. Run `usc libcal availability` with the user's date, time window, duration, room type, floor, and capacity constraints. For “maximum time,” try 120 minutes; if none match and the user means the longest available slot, retry 90, 60, then 30 minutes and stop at the first duration with results. Keep pods opt-in and include valid slots that cross midnight.
2. Rank the returned slots by the user's stated preferences. Treat date, time window, room type, minimum capacity, and floor as hard constraints. Then prefer the requested start time, requested duration, smallest sufficient capacity, and earlier time, in that order unless the user expressed another preference.
3. If the user asked only to find or check rooms, return the best matching options and do not book.
4. If the user explicitly asked to book, select a returned slot and book that exact slot with `--space ID --start HH:MM` plus the same date, duration, category, and capacity filters. Do not use broad earliest-match booking after presenting a specific option.
5. If authentication is required, run `usc auth login libcal --non-interactive`, refresh availability, and retry the same exact selection if it remains available. The returned confirmation is the source of truth.

Example for a group room tonight after 8 p.m., for the maximum two hours:

```sh
usc libcal availability --date today --after 20:00 --duration 120 --category rooms
usc libcal book --date today --after 20:00 --duration 120 --category rooms \
  --space 18364 --start 23:00 --accept-terms
```

- Group study rooms are the default. Add `--include-pods` or select `--category pods` only when one-person pods are acceptable; group rooms win ties at the same time.
- Capacity filters are bands: `--capacity 1-4|5-8|9-12`. Check each space's actual capacity against the group size. `--category lvl1|lvl2|lvl3` selects a floor.
- Dates and times use Los Angeles local time. Reservations are limited to two hours per day, one week in advance, and released if the patron does not arrive within ten minutes.
- Booking can require `--name`, `--email`, and repeatable `--field FIELD=VALUE` arguments. `--accept-terms` is an explicit confirmation; the command never prompts.
- `usc libcal reservations` lists upcoming reservations confirmed by this CLI; add `--all` for its past records. Its `complete: false` field means bookings made in a browser or before local recording was added may be absent. Availability is not the user's booking history. Check email for the authoritative complete history; do not book again to check.
- `usc libcal release` releases unfinished CLI checkout state only.
- `usc auth login libcal` and `usc auth status libcal` briefly stage and release a one-hour room hold. Avoid using them as public availability checks.

### LibCal failure recovery

- Normal errors and Ctrl-C release temporary holds. The next booking recovers recorded abandoned checkouts automatically.
- Errors include a stable `code` and an explanatory `error` string. For `libcal_slot_unavailable`, `libcal_selected_slot_unavailable`, or `libcal_stale_slot`, refresh availability. For `libcal_authentication_required`, authenticate as described above.
- For `libcal_checkout_busy`, wait for the other command. For `libcal_cleanup_failed`, retry `usc libcal release`.
- Never automatically retry `libcal_booking_unknown`: first check whether a confirmation email arrived. Then use `usc libcal release` to acknowledge the result before starting another booking. Release never cancels a confirmed reservation.
- A process killed before it receives a checkout ID may leave a hold until LibCal's server-side expiry.

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
