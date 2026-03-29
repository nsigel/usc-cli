# usc-cli

USC Brightspace from your terminal. Authenticate once, then query courses, grades, content, announcements, and download files — all scriptable and agent-friendly.

## Install

```bash
pip install -e ".[dev]"
```

## Auth

```bash
usc login                  # prompts for NetID, password, bypass code
usc status                 # verify session is live
usc logout                 # clear saved session
```

Session cookies are persisted to `~/.config/usc-cli/session.json`. All commands load this automatically — you don't re-login between calls.

**Bypass codes** replace Duo push during login. Get one at https://account.usc.edu/2fa/duo-bypass-code (requires identity verification). A code is valid for unlimited uses within 1 week.

Store credentials in your environment for fully unattended operation:

```bash
export USC_USERNAME=yournetid
export USC_PASSWORD=yourpassword
export USC_DUO_BYPASS=1234567
```

When all three are set, the CLI will **auto-reauth** on 401s — if your session expires mid-use, it re-logs in and retries the request transparently. No intervention needed until the bypass code expires (~weekly).

## Commands

```bash
usc courses                         # list enrolled courses + IDs
usc content <course_id>             # course modules and topic tree
usc content <course_id> --flat      # flat topic list with module path
usc grades <course_id>              # grade items and scores
usc grades <course_id> --graded-only
usc announcements                   # all courses
usc announcements <course_id>
usc announcements --since 2026-03-01
usc download <url> -o <path>        # download any Brightspace file by URL
```

All commands output JSON by default. Pass `--format human` for readable output.

### download

Accepts a full URL or a relative Brightspace path:

```bash
usc download /content/enforced/261076-.../hw7.pdf -o hw7.pdf
usc download https://brightspace.usc.edu/content/enforced/.../syllabus.pdf -o syllabus.pdf
```

URLs come from `usc content <id>` — each topic has a `url` field.

## Dev

```bash
ruff check src/
pytest
```

## Layout

```
src/usc_cli/
  cli.py          Click commands
  client.py       httpx client, session persistence, auto-reauth
  auth.py         USC Shibboleth SSO + Duo bypass login flow
~/.config/usc-cli/session.json   persisted cookies
```
