# usc-cli

CLI for programmatic interaction with USC's university portal (Brightspace D2L).

## Install

```bash
pip install -e ".[dev]"
```

## Usage

```bash
usc --help
usc login
usc courses
```

## Development

```bash
ruff check src/
pytest
```

## Architecture

- `src/usc_cli/cli.py` — Click CLI entrypoint
- `src/usc_cli/client.py` — HTTP client (httpx) for Brightspace D2L API
- Auth flow: USC Shibboleth SSO → D2L session (TBD)
