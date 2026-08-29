# usc-cli

A JSON-first command-line client for USC student services.

## Output

Every command writes JSON to stdout. Output is pretty-printed when stdout is
an interactive terminal and compact when it is piped or redirected. Use
`--json` to force compact output or `--pretty` to force pretty-printed output.
Errors are JSON on stderr and follow the same formatting choice.

## Build

```sh
go build -o usc ./cmd/usc
go vet ./...
```

## Authentication

`auth login` defaults to WebReg and accepts `webreg`, `brightspace`, or
`advise` as an optional site. It first reuses the saved session; if USC needs
a new login, it uses the supplied credentials or prompts for the USC NetID,
password, and Duo bypass code. Pass `--fresh` to intentionally ignore the
saved session. A successful login saves the resulting cross-domain cookie
session.

```sh
usc auth login
usc auth status brightspace
usc auth logout
```

## Brightspace

Brightspace commands reuse the saved session and return JSON. Authenticate once,
then query the course data exposed by Brightspace's Valence API:

```sh
usc auth login brightspace
usc brightspace courses
usc brightspace content COURSE_ID --flat
usc brightspace grades COURSE_ID --graded-only
usc brightspace announcements [COURSE_ID] --since 2026-08-01T00:00:00Z
usc brightspace assignments COURSE_ID
```

To download course content, first use `content COURSE_ID --flat` to find a
topic ID. Downloads use the saved Brightspace session; content URLs cannot be
downloaded without it.

```sh
usc brightspace download COURSE_ID TOPIC_ID
usc brightspace download COURSE_ID TOPIC_ID --markdown
usc brightspace download COURSE_ID TOPIC_ID --markdown --ocr
```

`--markdown` requires [Docling](https://docling-project.github.io/docling/):

```sh
pip install docling
```

OCR is disabled by default for born-digital course PDFs. Use `--ocr` only for
scanned documents.

For non-interactive use, provide credentials through the environment rather
than command-line arguments:

```sh
USC_USERNAME=netid \
USC_PASSWORD=password \
USC_DUO_BYPASS=123456789 \
usc auth login --non-interactive
```

Passwords and bypass codes are never written to disk. The cookie session lives
at the platform config location under `usc/session.json`; set
`USC_CONFIG_DIR` to override its directory. `auth logout` deletes it.
