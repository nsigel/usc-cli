---
name: usc-cli
description: Access USC Brightspace, Handshake events and career fairs, and the public Schedule of Classes with the Go-based `usc` CLI. Use when retrieving authorized course data, searching career events, reading full event or fair details, or checking current sections.
---

# USC CLI

## Preconditions

- Run `usc auth status` before every Brightspace request.
- Run `usc auth status handshake` before every Handshake request.
- Treat authentication sessions, passwords, Duo codes, and cookies as secrets. Never print, store, or share them.
- Do not submit coursework, alter enrollment, or send messages.

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
