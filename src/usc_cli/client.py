"""HTTP client for USC Brightspace D2L API."""

from __future__ import annotations

import httpx


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

    def login(self, username: str, password: str) -> None:
        """Authenticate via USC Shibboleth SSO flow."""
        raise NotImplementedError("SSO login flow not yet implemented")

    def whoami(self) -> dict:
        """Return the current authenticated user profile."""
        resp = self._http.get(f"{self.D2L_API}/lp/1.0/users/whoami")
        resp.raise_for_status()
        return resp.json()

    def enrollments(self) -> list[dict]:
        """List current course enrollments."""
        resp = self._http.get(f"{self.D2L_API}/lp/1.0/enrollments/myenrollments/")
        resp.raise_for_status()
        return resp.json().get("Items", [])

    def close(self) -> None:
        self._http.close()

    def __enter__(self) -> USCClient:
        return self

    def __exit__(self, *exc) -> None:
        self.close()
