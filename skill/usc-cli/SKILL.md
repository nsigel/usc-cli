---
name: usc-cli
description: Access USC Brightspace, Handshake events and career fairs, Leavey Library room availability and bookings, Web Registration (course bin, calendar, and checkout), and the public Schedule of Classes with the Go-based `usc` CLI. Use when retrieving authorized course data, searching career events, booking or checking study spaces, planning or submitting registration, or checking current sections.
---

# USC CLI

## Preconditions

- Commands sign themselves in with the saved USC session and credentials; do not check `usc auth status` first. Only when an error's `action` is `usc auth login`, ask the user to run it in a terminal (it prompts), then retry.
- LibCal discovery is public. Inspect room types and availability before booking.
- Treat authentication sessions, passwords, Duo codes, and cookies as secrets. Never print, store, or share them.
- Do not submit coursework or send messages.
- Only change WebReg state (`add`, `schedule`, `unschedule`, `remove`, `grade-option`) when the user asks. Only run `usc webreg submit --confirm` after showing the user `usc webreg checkout` and receiving explicit approval to change their enrollment.
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
usc brightspace submissions COURSE_ID FOLDER_ID [--download] [--output DIRECTORY]
usc brightspace download COURSE_ID TOPIC_ID [--output DIRECTORY]
```

- Use `content --flat` to locate a file's topic ID; download it by that ID.
- Use an `assignments` folder ID with `submissions`; `--download` saves every submitted file, prefixing repeated names with the submission ID.
- Downloads default to the current working directory; pass `--output` for another one.
- Course data is live; the CLI has no content cache.

## Handshake

```sh
usc handshake categories
usc handshake events [--category NAME_OR_ID] [--organizer NAME_OR_EMAIL] [--keyword TEXT] [--medium all|virtual|in-person] [--date all|today|next-10|next-30|past-year] [--posted-by-school] [--sort relevance|date|posted-desc] [--limit N] [--after CURSOR]
usc handshake event EVENT_ID
usc handshake career-fairs [SEARCH_FLAGS]
usc handshake career-fair CAREER_FAIR_ID
```

- Resolve readable category slugs with `handshake categories`; list filters also accept the numeric category IDs.
- Lists return `items`, `has_more`, and `next_cursor`. Continue by passing `next_cursor` to `--after` with the same filters and sort.
- Organizer matching checks host, employer, and detail contact names/emails.
- Handshake exposes no event posting timestamp. `posted-desc` uses descending numeric IDs as a new-event monitoring signal; its cursor kind is `id`.

## Web Registration

```sh
usc webreg terms
usc webreg programs --term TERM
usc webreg courses --term TERM --program CODE | --course CODE
usc webreg bin --term TERM
usc webreg calendar --term TERM
usc webreg add|schedule|unschedule|remove SECTION_ID --term TERM
usc webreg grade-option SECTION_ID CODE --term TERM
usc webreg checkout --term TERM
usc webreg submit --term TERM --confirm
```

- Get `TERM` codes from `webreg terms`; section IDs from `courses` or `bin`.
- Course-bin changes only plan registration. `add` places a section in the bin already scheduled. Checkout registers scheduled, unregistered sections and drops registered, unscheduled ones. Unscheduling a registered section therefore means dropping it at checkout.
- `clearance: "D"` sections need department clearance; `seats.closed` sections usually fail at checkout.
- Use `calendar` to spot `Conflicted` meetings before checkout.
- `submit` is all-or-nothing. On `succeeded: false`, report `messages`; nothing changed. Do not retry automatically.

## Leavey Library / LibCal

Use live `categories`, `spaces`, or `space SPACE_ID` data for room names, IDs, kinds,
and actual capacities. `rooms` includes group rooms; `pods` is one-person
pods; `all` includes both. `lvl1`, `lvl2`, and `lvl3` select group-room
floors. Discovery defaults to all categories without ranking them.

Read `usc libcal schedule --date YYYY-MM-DD` (`today` and `tomorrow` also work).
Optional filters are `--category`, `--min-capacity`, and `--max-capacity`.
`--end-date` is exclusive; extend it to include dates across midnight.

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
answers; do not invent answers.
Reservation terms are published at https://libcal.usc.edu/reserve/lvl1.

Pass full RFC3339 timestamps with UTC offsets. This example shows the normal
schedule-to-book flow across midnight; replace its dates and selection with
live values and use `--accept-terms` only after agreement.

```sh
usc libcal schedule --date 2026-10-02 --end-date 2026-10-04 --category rooms --min-capacity 6
usc libcal book --space 31391 --start 2026-10-02T23:00:00-07:00 \
  --end 2026-10-03T01:00:00-07:00 --accept-terms
```

- There is no command that lists existing reservations; LibCal exposes none.
  The confirmation email is the record. Do not book again to check.
- `usc libcal release` releases unfinished CLI checkout state only.

### LibCal failure recovery

- Normal errors and Ctrl-C release temporary holds. The next booking recovers recorded abandoned checkouts automatically.
- For `libcal_duration_limit`, choose a span of at most two hours from the schedule.
- Errors include a stable `code` and an explanatory `error` string. For `libcal_slot_unavailable`, `libcal_selected_slot_unavailable`, or `libcal_stale_slot`, refresh availability. For `libcal_authentication_required`, ask the user to run `usc auth login`, refresh the schedule, and retry the same selection if it is still available.
- For `libcal_checkout_busy`, wait for the other command. For `libcal_cleanup_failed`, retry `usc libcal release`.
- Never automatically retry `libcal_booking_unknown`: first check whether a confirmation email arrived. Then use `usc libcal release` to acknowledge the result before starting another booking. Release never cancels a confirmed reservation.
- A process killed before it receives a checkout ID may leave a hold until LibCal's server-side expiry.

## Browser sync

Run `usc browser sync [--cdp TARGET]` to copy the saved session into Chrome, then
use the site's student/SSO login in Chrome to reach Handshake or other USC apps.


## Public schedule data

```sh
usc classes TERM_CODE COURSE_CODE
```

- Use `usc classes` for public section times, availability, instructors, and syllabus links.
- This command does not require a saved USC session.

## Output and errors

- Data commands emit JSON to stdout. Use `--compact` for single-line output or `--pretty` for indented output.
- `usc skill` emits this file as raw Markdown.
- Errors are JSON on stderr and exit with status 1. If an error includes `action`, follow that recovery step when appropriate.
- Only `usc auth login` may prompt.
