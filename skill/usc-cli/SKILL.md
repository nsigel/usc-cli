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

Use live `categories`, `spaces`, or `room` data for room names, IDs, kinds,
and actual capacities. `rooms` includes group rooms; `pods` is one-person
pods; `all` includes both. `lvl1`, `lvl2`, and `lvl3` select group-room
floors. Discovery defaults to all categories without ranking them.

Read `usc libcal schedule --date YYYY-MM-DD` (alias: `availability`).
Optional filters are `--category`, `--min-capacity`, `--max-capacity`,
or an inclusive `--capacity MIN-MAX` range such as `6-12`. `--end-date`
is exclusive; extend it to include dates across midnight.

The schedule is enough to choose a duration. Each entry in
`categories[].spaces[].intervals[]` has `start`, `end`, `available`,
and the server's `status`. Join adjacent available intervals for the same
space and choose a continuous reservation of **up to two hours**. For
"maximum time," select the longest suitable available span, capped at two
hours. Respect interval boundaries; gaps and unavailable intervals cannot
be included. Rank options using the user's constraints and preferences.

For a discovery request, return matching options. For an explicit booking
request, pass the chosen `--space`, `--start`, and `--end` directly to
`usc libcal book`, with `--accept-terms` when the user has agreed to the
terms. Booking retrieves end-time checksums and completes checkout through
HTTP requests internally. The CLI and Go package reject selections longer
than two hours before sending requests.

If checkout requires additional answers, its validation error identifies the
field and available choices. Retry with explicit `--field FIELD=VALUE`
answers; do not invent dropdown answers. `--name` and `--email` are
optional conveniences, overridden by explicit field answers.
Reservation terms are published at https://libcal.usc.edu/reserve/lvl1.

If authentication is required, run `usc auth login libcal --non-interactive`
without booking flags, refresh the schedule, and retry the same selection if
still available. Authentication never stages a room.

Pass full RFC3339 timestamps with UTC offsets. This example shows the normal
schedule-to-book flow across midnight; replace its dates and selection with
live values and use `--accept-terms` only after agreement.

```sh
usc libcal schedule --date 2026-10-02 --end-date 2026-10-04 --category rooms --min-capacity 6
usc libcal book --space 31391 --start 2026-10-02T23:00:00-07:00 \
  --end 2026-10-03T01:00:00-07:00 --accept-terms
```

- `usc auth login libcal` and `usc auth status libcal` establish/check shared USC
  SSO through the existing Handshake SAML entry. `book` completes LibCal's
  checkout-specific handoff. Only auth commands can prompt.
- `usc libcal reservations` lists upcoming reservations confirmed by this CLI;
  add `--all` for past records. `complete: false` means browser or older
  bookings may be absent. Check email for authoritative complete history;
  do not book again to check.
- `usc libcal release` releases unfinished CLI checkout state only.

### LibCal failure recovery

- Normal errors and Ctrl-C release temporary holds. The next booking recovers recorded abandoned checkouts automatically.
- For `libcal_duration_limit`, choose a span of at most two hours from the schedule.
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
