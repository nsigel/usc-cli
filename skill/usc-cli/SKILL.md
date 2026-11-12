---
name: usc-cli
description: Access USC Brightspace, Handshake events and career fairs, Leavey Library room availability and bookings, Web Registration (course bin, calendar, and checkout), and the public Schedule of Classes with the Go-based `usc` CLI. Use when retrieving authorized course data, searching career events, booking or checking study spaces, planning or submitting registration, or checking current sections.
---

# USC CLI

Every command prints JSON to stdout and signs itself in. Errors are JSON on
stderr with exit status 1; the `error` text says what went wrong and what to do.

## Brightspace

```sh
usc brightspace courses [--all]
usc brightspace content COURSE_ID [--flat]
usc brightspace grades COURSE_ID [--graded-only]
usc brightspace announcements [COURSE_ID] [--since RFC3339_TIMESTAMP]
usc brightspace assignments COURSE_ID
usc brightspace submissions COURSE_ID FOLDER_ID [--download] [--output DIRECTORY]
usc brightspace download COURSE_ID TOPIC_ID [--output DIRECTORY]
usc brightspace whoami
```

- `COURSE_ID` is the `id` from `courses`, `TOPIC_ID` the `id` from `content --flat`, and `FOLDER_ID` the `id` from `assignments`.
- `announcements` without a course covers every enrolled course.

## Handshake

```sh
usc handshake categories
usc handshake events [--category NAME_OR_ID] [--organizer NAME_OR_EMAIL] [--keyword TEXT] [--medium all|virtual|in-person] [--date all|today|next-10|next-30|past-year] [--posted-by-school] [--sort relevance|date|posted-desc] [--limit N] [--after CURSOR]
usc handshake event EVENT_ID
usc handshake career-fairs [SAME_FLAGS_AS_EVENTS]
usc handshake career-fair CAREER_FAIR_ID
```

- Lists return `items`, `has_more`, and `next_cursor`; pass `next_cursor` to `--after` with the same flags for the next page.
- `--sort posted-desc` lists the most recently posted first.

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

- `TERM` is a `code` from `terms`; `SECTION_ID` is a section `id` from `courses` or `bin`.
- `add`, `schedule`, `unschedule`, and `remove` only edit the course bin. `submit` is what registers scheduled sections and drops registered sections that were unscheduled; `checkout` previews exactly that.
- `calendar` marks conflicting meetings as `Conflicted`.

## Leavey Library rooms

```sh
usc libcal spaces [rooms|pods|all|lvl1|lvl2|lvl3]
usc libcal schedule [--date today|tomorrow|YYYY-MM-DD] [--end-date YYYY-MM-DD] [--category CATEGORY] [--min-capacity N] [--max-capacity N]
usc libcal book --space SPACE_ID --start TIMESTAMP --end TIMESTAMP --accept-terms [--field NAME=VALUE]
```

- `schedule` lists each space's `intervals` with `start`, `end`, and `available`. To book, take `--start` from the start of an available interval and `--end` from the end of the same or a later adjacent available interval, at most two hours apart, copied as printed.
- `--accept-terms` accepts the reservation terms at https://libcal.usc.edu/reserve/lvl1.

```sh
usc libcal schedule --date 2026-10-02 --category rooms --min-capacity 6
usc libcal book --space 31391 --start 2026-10-02T15:00:00-07:00 --end 2026-10-02T17:00:00-07:00 --accept-terms
```

## Schedule of Classes

```sh
usc classes TERM_CODE COURSE_CODE
```

Public section times, seats, instructors, and syllabus links, for example `usc classes 20271 BUAD-306`.
