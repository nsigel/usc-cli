# AGENTS.md — usc-cli

## Overview
Python CLI for interacting with USC university services. Scope: USC SSO login, on-device session/cookie persistence, and Brightspace course content access.

## Design Philosophy — Agents First
- All commands output **JSON by default** (machine-readable, predictable schema)
- `--format human` opt-in for human-readable tables/trees
- No interactive prompts in non-auth commands
- Errors always go to stderr as `{"error": "..."}` JSON; stdout is always valid JSON on success
- Exit code 1 on any error

## Stack
- Python 3.10+, Click (CLI), httpx (HTTP), Rich (output), keyring (credential storage)
- src layout: `src/usc_cli/`
- Tooling: ruff (lint), pytest (test)

## Auth
- USC uses Shibboleth SSO → login.usc.edu → Duo MFA (bypass code or TOTP)
- Session cookies are saved to `~/.config/usc-cli/session.json` after successful login
- All subsequent commands load cookies from disk — no re-authentication needed until session expires

## Session Storage
- Path: `~/.config/usc-cli/session.json`
- Format: httpx-compatible cookie jar (list of cookie dicts with domain, name, value, path, etc.)
- Credentials stored via keyring (system keychain)

## Commands
- `usc login` — authenticate via USC SSO + Duo, save session cookies
- `usc status` — check if session is loaded
- `usc logout` — clear session
- `usc courses [--format json|human]` — list enrolled courses with id, code, title, section
- `usc content <course_id> [--flat] [--format json|human]` — show module/topic tree; --flat for all topics in one list

## Modules
- `src/usc_cli/auth.py` — USC Shibboleth SSO + Duo MFA login flow
- `src/usc_cli/client.py` — httpx client + session cookie persistence
- `src/usc_cli/brightspace.py` — Brightspace Valence API (courses, content toc, dropbox)
- `src/usc_cli/cli.py` — Click entrypoint

## API Notes
- Brightspace cookie auth works without OAuth registration (session cookies from login flow)
- `brightspace.usc.edu` LP version: 1.31, LE version: 1.67
- Cookies saved to `~/.config/usc-cli/session.json`

## Conventions
- All code in `src/usc_cli/`
- Use `pyproject.toml` only (no setup.py/cfg)
- Type hints everywhere
- ruff for formatting/linting
