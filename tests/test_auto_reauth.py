"""Tests for automatic re-authentication on 401 responses.

Uses pytest-httpx to mock HTTP + monkeypatching to inject env credentials
and stub out the full login flow.
"""

from __future__ import annotations

import json
from unittest.mock import MagicMock, patch

import httpx
import pytest
from pytest_httpx import HTTPXMock

from usc_cli.client import USCClient, _credentials_from_env

BASE = "https://brightspace.usc.edu"
LP = "1.31"

WHOAMI_RESPONSE = {
    "Identifier": "345258",
    "FirstName": "Noah",
    "LastName": "Sigel",
    "Pronouns": "",
    "UniqueName": "4163587521",
    "ProfileIdentifier": "6b3ONDjivG",
}

FAKE_SESSION = [
    {"name": "d2lSessionVal", "value": "newsession", "domain": "brightspace.usc.edu", "path": "/"},
]

ENV_CREDS = {
    "USC_USERNAME": "nsigel",
    "USC_PASSWORD": "hunter2",
    "USC_DUO_BYPASS": "12345678",
}


# ---------------------------------------------------------------------------
# _credentials_from_env
# ---------------------------------------------------------------------------


def test_credentials_from_env_all_present(monkeypatch) -> None:
    monkeypatch.setenv("USC_USERNAME", "nsigel")
    monkeypatch.setenv("USC_PASSWORD", "hunter2")
    monkeypatch.setenv("USC_DUO_BYPASS", "bypass123")
    result = _credentials_from_env()
    assert result == ("nsigel", "hunter2", "bypass123")


def test_credentials_from_env_missing_one(monkeypatch) -> None:
    monkeypatch.setenv("USC_USERNAME", "nsigel")
    monkeypatch.setenv("USC_PASSWORD", "hunter2")
    monkeypatch.delenv("USC_DUO_BYPASS", raising=False)
    assert _credentials_from_env() is None


def test_credentials_from_env_all_missing(monkeypatch) -> None:
    monkeypatch.delenv("USC_USERNAME", raising=False)
    monkeypatch.delenv("USC_PASSWORD", raising=False)
    monkeypatch.delenv("USC_DUO_BYPASS", raising=False)
    assert _credentials_from_env() is None


# ---------------------------------------------------------------------------
# Auto-reauth on 401
# ---------------------------------------------------------------------------


def _stub_reauth(usc_client: USCClient, session_cookies: list[dict]) -> None:
    """Replace usc_client.reauth() with a stub that injects cookies and returns True."""

    def _fake_reauth() -> bool:
        usc_client._http._reset_cookies()
        for c in session_cookies:
            usc_client._http.cookies.set(
                c["name"], c["value"], domain=c.get("domain"), path=c.get("path")
            )
        usc_client._authenticated = True
        return True

    usc_client.reauth = _fake_reauth  # type: ignore[method-assign]


def test_401_triggers_reauth_and_retry(httpx_mock: HTTPXMock, monkeypatch, tmp_path) -> None:
    """A 401 on a Brightspace endpoint should reauth and retry, returning the retried response."""
    for k, v in ENV_CREDS.items():
        monkeypatch.setenv(k, v)

    # First call → 401, second call → 200
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/lp/{LP}/users/whoami",
        status_code=401,
    )
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/lp/{LP}/users/whoami",
        json=WHOAMI_RESPONSE,
    )

    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    from usc_cli import client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        with USCClient() as usc:
            usc.load_session()
            _stub_reauth(usc, FAKE_SESSION)
            resp = usc._http.get(f"{BASE}/d2l/api/lp/{LP}/users/whoami")
    finally:
        client_mod.SESSION_PATH = orig

    assert resp.status_code == 200
    assert resp.json()["FirstName"] == "Noah"


def test_no_reauth_when_creds_absent(httpx_mock: HTTPXMock, monkeypatch, tmp_path) -> None:
    """Without env credentials, a 401 should be returned as-is (no retry)."""
    monkeypatch.delenv("USC_USERNAME", raising=False)
    monkeypatch.delenv("USC_PASSWORD", raising=False)
    monkeypatch.delenv("USC_DUO_BYPASS", raising=False)

    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/lp/{LP}/users/whoami",
        status_code=401,
    )

    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    from usc_cli import client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        with USCClient() as usc:
            usc.load_session()
            resp = usc._http.get(f"{BASE}/d2l/api/lp/{LP}/users/whoami")
    finally:
        client_mod.SESSION_PATH = orig

    assert resp.status_code == 401


def test_reauth_loop_guard(httpx_mock: HTTPXMock, monkeypatch, tmp_path) -> None:
    """If reauth itself triggers a 401, we should not loop infinitely."""
    for k, v in ENV_CREDS.items():
        monkeypatch.setenv(k, v)

    # Both calls return 401 — simulates reauth producing bad cookies
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/lp/{LP}/users/whoami",
        status_code=401,
    )
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/lp/{LP}/users/whoami",
        status_code=401,
    )

    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    def _bad_reauth() -> bool:
        # "succeeds" but injects broken cookies — retry will still 401
        return True

    from usc_cli import client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        with USCClient() as usc:
            usc.load_session()
            usc.reauth = _bad_reauth  # type: ignore[method-assign]
            resp = usc._http.get(f"{BASE}/d2l/api/lp/{LP}/users/whoami")
    finally:
        client_mod.SESSION_PATH = orig

    # Should return the 401 from the retry rather than looping
    assert resp.status_code == 401


def test_non_brightspace_401_not_retried(httpx_mock: HTTPXMock, monkeypatch) -> None:
    """401 from non-Brightspace hosts should pass through without reauth."""
    for k, v in ENV_CREDS.items():
        monkeypatch.setenv(k, v)

    httpx_mock.add_response(
        method="GET",
        url="https://example.com/api",
        status_code=401,
    )

    reauth_called = []

    with USCClient() as usc:
        usc.reauth = lambda: reauth_called.append(True) or True  # type: ignore[method-assign]
        resp = usc._http.get("https://example.com/api")

    assert resp.status_code == 401
    assert not reauth_called
