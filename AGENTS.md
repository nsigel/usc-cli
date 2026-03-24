# AGENTS.md — usc-cli

## Overview
Python CLI for programmatic interaction with USC Brightspace D2L portal.

## Stack
- Python 3.10+, Click (CLI), httpx (HTTP), Rich (output), keyring (credential storage)
- src layout: `src/usc_cli/`
- Tooling: ruff (lint), pytest (test)

## Auth
- USC uses Shibboleth SSO which redirects through identity.usc.edu
- Goal: headless login flow that captures D2L session cookies
- Credentials stored via keyring (system keychain)

## Conventions
- All code in `src/usc_cli/`
- Use `pyproject.toml` only (no setup.py/cfg)
- Type hints everywhere
- ruff for formatting/linting
