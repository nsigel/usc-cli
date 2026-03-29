# AGENTS.md — usc-cli

Guidance for AI coding agents working in this repo.

## What This Is

Python CLI for USC Brightspace. Handles USC Shibboleth SSO login, persists session cookies, and exposes course data via clean JSON-first commands.

## Layout

```
src/usc_cli/
  cli.py          Click command definitions (entrypoint)
  client.py       httpx client, session persistence, auto-reauth logic
  auth.py         USC Shibboleth SSO + Duo bypass login flow
  brightspace.py  Brightspace Valence API calls (courses, content, grades, announcements)
tests/
  test_auth.py
  test_brightspace.py
  test_announcements.py
  test_auto_reauth.py
```

## Stack

- Python 3.10+, Click, httpx, Rich
- `pyproject.toml` only (no setup.py)
- Linting: ruff
- Tests: pytest + pytest-httpx

## Conventions

- All commands output **JSON to stdout** by default; `--format human` is opt-in
- Errors go to **stderr** as `{"error": "..."}` JSON; exit code 1 on failure
- No interactive prompts in non-auth commands
- Type hints everywhere
- New commands follow the pattern in `cli.py`: load session → call brightspace function → format output

## Adding a Command

1. Add any new API calls to `brightspace.py`
2. Register the Click command in `cli.py`
3. Document the JSON output schema in the docstring
4. Add tests in `tests/`

## Auth Flow (for context)

USC uses Shibboleth SSO → Duo MFA. The `auth.py` module drives this flow headlessly using a bypass code (a Duo feature that generates a short-lived OTP-style code). Session cookies are persisted to `~/.config/usc-cli/session.json`. All commands load cookies from disk.

Auto-reauth: if `USC_USERNAME`, `USC_PASSWORD`, and `USC_DUO_BYPASS` are in the environment, any 401 from Brightspace triggers a silent re-login and request retry. This is implemented in `_AuthRetryClient.send()` in `client.py`.

## Running Tests

```bash
pip install -e ".[dev]"
pytest
ruff check src/
```
