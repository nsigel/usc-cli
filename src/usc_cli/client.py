"""HTTP client for USC Brightspace D2L API."""

from __future__ import annotations

import logging

import httpx

from usc_cli.auth import AuthError, USCAuth  # noqa: F401 — re-export

logger = logging.getLogger(__name__)


class USCClient:
    """Handles authentication and API requests to Brightspace D2L."""

    BASE_URL = "https://brightspace.usc.edu"
    D2L_API = f"{BASE_URL}/d2l/api"

    def __init__(self) -> None:
        self._http = httpx.Client(
            base_url=self.BASE_URL,
            follow_redirects=True,
            timeout=30.0,
            headers={"User-Agent": (
                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                "AppleWebKit/537.36 (KHTML, like Gecko) "
                "Chrome/124.0.0.0 Safari/537.36"
            )},
        )
        self._authenticated = False

    def login(self, username: str, password: str, bypass_code: str) -> None:
        """Authenticate via USC Shibboleth SSO + Duo bypass code."""
        auth = USCAuth(self._http)
        auth.login(username, password, bypass_code)
        self._authenticated = True
        logger.info("Authenticated as %s", username)

    def _require_auth(self) -> None:
        if not self._authenticated:
            raise AuthError("Not authenticated — call login() first")

    def whoami(self) -> dict:
        """Return the current authenticated user profile."""
        self._require_auth()
        resp = self._http.get(f"{self.D2L_API}/lp/1.0/users/whoami")
        resp.raise_for_status()
        return resp.json()

    def enrollments(self) -> list[dict]:
        """List current course enrollments."""
        self._require_auth()
        resp = self._http.get(f"{self.D2L_API}/lp/1.0/enrollments/myenrollments/")
        resp.raise_for_status()
        return resp.json().get("Items", [])

    def close(self) -> None:
        self._http.close()

    def __enter__(self) -> USCClient:
        return self

    def __exit__(self, *exc) -> None:
        self.close()
