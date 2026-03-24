"""USC HTTP client with on-device session persistence."""

from __future__ import annotations

import json
import logging
from pathlib import Path

import httpx

from usc_cli.auth import AuthError, USCAuth  # noqa: F401 — re-export

logger = logging.getLogger(__name__)

SESSION_PATH = Path.home() / ".config" / "usc-cli" / "session.json"

USC_BASE = "https://my.usc.edu"


def _save_session(http: httpx.Client) -> None:
    """Persist the current cookie jar to disk."""
    SESSION_PATH.parent.mkdir(parents=True, exist_ok=True)
    cookies = [
        {
            "name": c.name,
            "value": c.value,
            "domain": c.domain,
            "path": c.path,
        }
        for c in http.cookies.jar
    ]
    SESSION_PATH.write_text(json.dumps(cookies, indent=2))
    SESSION_PATH.chmod(0o600)
    logger.debug("Session saved to %s (%d cookies)", SESSION_PATH, len(cookies))


def _load_session(http: httpx.Client) -> bool:
    """Load persisted cookies into the client. Returns True if cookies were loaded."""
    if not SESSION_PATH.exists():
        return False
    try:
        cookies = json.loads(SESSION_PATH.read_text())
        for c in cookies:
            http.cookies.set(c["name"], c["value"], domain=c.get("domain"), path=c.get("path"))
        logger.debug("Session loaded from %s (%d cookies)", SESSION_PATH, len(cookies))
        return bool(cookies)
    except Exception as exc:
        logger.warning("Failed to load session: %s", exc)
        return False


def clear_session() -> None:
    """Delete the persisted session file."""
    if SESSION_PATH.exists():
        SESSION_PATH.unlink()
        logger.debug("Session cleared: %s", SESSION_PATH)


class USCClient:
    """Handles authentication and HTTP requests to USC services."""

    def __init__(self) -> None:
        self._http = httpx.Client(
            follow_redirects=True,
            timeout=30.0,
            headers={
                "User-Agent": (
                    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                    "AppleWebKit/537.36 (KHTML, like Gecko) "
                    "Chrome/124.0.0.0 Safari/537.36"
                )
            },
        )
        self._authenticated = False

    def login(self, username: str, password: str, bypass_code: str) -> None:
        """Authenticate via USC Shibboleth SSO + Duo bypass code.

        Saves session cookies to disk on success so future commands can
        load them without re-authenticating.
        """
        auth = USCAuth(self._http)
        auth.login(username, password, bypass_code)
        self._authenticated = True
        _save_session(self._http)
        logger.info("Authenticated as %s — session saved", username)

    def load_session(self) -> bool:
        """Load persisted session from disk. Returns True if session was found."""
        loaded = _load_session(self._http)
        if loaded:
            self._authenticated = True
        return loaded

    def is_authenticated(self) -> bool:
        return self._authenticated

    def close(self) -> None:
        self._http.close()

    def __enter__(self) -> USCClient:
        return self

    def __exit__(self, *exc) -> None:
        self.close()
