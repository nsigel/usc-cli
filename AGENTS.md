# AGENTS.md — usc-cli

## Overview
Python CLI for interacting with USC university services. Scope: USC SSO login and on-device session/cookie persistence so other commands can authenticate without re-login.

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

## Conventions
- All code in `src/usc_cli/`
- Use `pyproject.toml` only (no setup.py/cfg)
- Type hints everywhere
- ruff for formatting/linting
