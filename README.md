# usc-cli

CLI for USC university services. Handles USC SSO login and persists your session on-device so other commands don't require re-authentication.

## Install

```bash
pip install -e ".[dev]"
```

## Usage

```bash
usc --help
usc login
usc status
usc courses
usc content <course_id>
usc grades <course_id>
usc announcements [course_id]
```

## Authentication

`usc login` requires your USC NetID, password, and a **Duo bypass code**.

Bypass codes are available at https://account.usc.edu/2fa/duo-bypass-code — generating one requires identity verification (you won't get a new code just by refreshing the page).

**Bypass code lifetime:** valid for unlimited uses within 1 week, then it expires. Store it in `USC_DUO_BYPASS` in your environment; you only need to regenerate it weekly (or if USC prompts you to during login).

## Development

```bash
ruff check src/
pytest
```

## Architecture

- `src/usc_cli/cli.py` — Click CLI entrypoint
- `src/usc_cli/client.py` — HTTP client (httpx); handles session persistence
- `src/usc_cli/auth.py` — USC Shibboleth SSO + Duo MFA login flow
- Session cookies stored at `~/.config/usc-cli/session.json`
- Auth flow: USC Shibboleth SSO → Duo MFA → session cookies saved on-device
