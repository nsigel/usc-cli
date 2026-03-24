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
```

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
