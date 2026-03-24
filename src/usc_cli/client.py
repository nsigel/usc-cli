"""HTTP client for USC Brightspace D2L API."""

from __future__ import annotations

import httpx

from usc_cli.auth import AuthError
from usc_cli.auth import login as _do_login


class USCClient:
    """Handles authentication and API requests to Brightspace D2L."""

    BASE_URL = "https://brightspace.usc.edu"
    D2L_API = f"{BASE_URL}/d2l/api"

    def __init__(self) -> None:
        self._http = httpx.Client(
            base_url=self.BASE_URL,
            follow_redirects=True,
            timeout=30.0,
        )
        self._authenticated = False
        self._session: dict[str, str] = {}

    def login(self, username: str, password: str, bypass_code: str) -> None:
        """Authenticate via USC SAML SSO + Duo bypass code flow."""
        session = _do_login(username, password, bypass_code)
        self._session = session

        # Inject cookies into the persistent httpx client
        for name, value in session.get("_all_cookies", {}).items():
            self._http.cookies.set(name, value, domain="brightspace.usc.edu")

        # Set XSRF header if available
        if session.get("xsrf_token"):
            self._http.headers["X-Csrf-Token"] = session["xsrf_token"]

        self._authenticated = True

    @property
    def session(self) -> dict[str, str]:
        return self._session

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

    def _require_auth(self) -> None:
        if not self._authenticated:
            raise AuthError("Not authenticated. Call login() first.")

    def close(self) -> None:
        self._http.close()

    def __enter__(self) -> USCClient:
        return self

    def __exit__(self, *exc) -> None:
        self.close()
