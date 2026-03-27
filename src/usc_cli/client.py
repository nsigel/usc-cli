"""USC HTTP client with on-device session persistence."""

from __future__ import annotations

import json
import logging
import os
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


def _credentials_from_env() -> tuple[str, str, str] | None:
    """Return (username, password, bypass_code) from environment, or None if incomplete."""
    username = os.environ.get("USC_USERNAME", "").strip()
    password = os.environ.get("USC_PASSWORD", "").strip()
    bypass_code = os.environ.get("USC_DUO_BYPASS", "").strip()
    if username and password and bypass_code:
        return username, password, bypass_code
    return None


class USCClient:
    """Handles authentication and HTTP requests to USC services.

    Auto-reauth: if USC_USERNAME, USC_PASSWORD, and USC_DUO_BYPASS are set in the
    environment, any 401 response from Brightspace will automatically trigger a
    fresh login and a single retry of the original request. This keeps sessions
    alive without manual intervention.
    """

    def __init__(self) -> None:
        self._http = _AuthRetryClient(self)
        self._authenticated = False
        self._reauthing = False  # guard against reauth loops

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

    def reauth(self) -> bool:
        """Re-authenticate using env-var credentials. Returns True on success."""
        creds = _credentials_from_env()
        if not creds:
            logger.debug("Auto-reauth skipped: credentials not in environment")
            return False
        username, password, bypass_code = creds
        logger.info("Session expired — auto-reauthenticating as %s", username)
        # Fresh underlying client to avoid stale cookie state
        self._http._reset_cookies()
        auth = USCAuth(self._http)
        auth.login(username, password, bypass_code)
        self._authenticated = True
        _save_session(self._http)
        logger.info("Auto-reauth successful")
        return True

    def is_authenticated(self) -> bool:
        return self._authenticated

    def close(self) -> None:
        self._http.close()

    def __enter__(self) -> USCClient:
        return self

    def __exit__(self, *exc) -> None:
        self.close()


class _AuthRetryClient(httpx.Client):
    """httpx.Client subclass that intercepts 401 responses and retries after reauth."""

    def __init__(self, usc_client: USCClient) -> None:
        super().__init__(
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
        self._usc = usc_client

    def _reset_cookies(self) -> None:
        """Clear all cookies from the jar (used before reauth)."""
        self.cookies.clear()

    def send(self, request: httpx.Request, **kwargs) -> httpx.Response:  # type: ignore[override]
        response = super().send(request, **kwargs)

        # Only intercept 401 from Brightspace, and only if not already mid-reauth
        if (
            response.status_code == 401
            and "brightspace.usc.edu" in str(request.url)
            and not self._usc._reauthing
        ):
            self._usc._reauthing = True
            try:
                reauthd = self._usc.reauth()
            finally:
                self._usc._reauthing = False

            if reauthd:
                logger.debug("Retrying request after reauth: %s", request.url)
                # Rebuild request so it picks up fresh cookies
                retry = request.read()  # ensure body is available
                new_request = self.build_request(
                    method=request.method,
                    url=request.url,
                    headers=dict(request.headers),
                    content=retry,
                )
                response = super().send(new_request, **kwargs)

        return response
