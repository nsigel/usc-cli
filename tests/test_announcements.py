"""Tests for announcements — get_announcements() and `usc announcements` CLI command."""

from __future__ import annotations

import json

import httpx
import pytest
from click.testing import CliRunner
from pytest_httpx import HTTPXMock

from usc_cli.brightspace import BrightspaceError, get_announcements
from usc_cli.cli import cli

BASE = "https://brightspace.usc.edu"
LP = "1.31"
LE = "1.67"
OU_ID = 262484
OU_ID_2 = 261076

FAKE_SESSION = [
    {"name": "d2lSessionVal", "value": "fakesession", "domain": "brightspace.usc.edu", "path": "/"},
    {"name": "d2lSecureSessionVal", "value": "fakesecure", "domain": "brightspace.usc.edu", "path": "/"},
]

ENROLLMENTS_TWO_COURSES = {
    "Items": [
        {
            "OrgUnit": {
                "Id": OU_ID,
                "Name": "20261_14927 BUAD-313: Advanced Operations Management and Analytics",
                "Type": {"Id": 3, "Code": "CourseOffering", "Name": "Course Offering"},
            },
            "Role": {"Id": 110, "Code": "Learner", "Name": "Student"},
        },
        {
            "OrgUnit": {
                "Id": OU_ID_2,
                "Name": "20261_30108/30295 CSCI-170: Discrete Methods in Computer Science",
                "Type": {"Id": 3, "Code": "CourseOffering", "Name": "Course Offering"},
            },
            "Role": {"Id": 110, "Code": "Learner", "Name": "Student"},
        },
    ],
    "PagingInfo": {"HasMoreItems": False, "Bookmark": None},
}

ANNOUNCEMENTS_BUAD = [
    {
        "Id": 101,
        "Title": "Midterm grades released",
        "Body": {"Text": "Midterm Part 1 and Part 2 grades are now available.", "Html": ""},
        "StartDate": "2026-03-18T08:00:00Z",
        "EndDate": None,
        "CreatedDate": "2026-03-18T07:55:00Z",
        "LastModifiedDate": "2026-03-18T07:55:00Z",
        "IsPinned": False,
        "IsHidden": False,
        "Attachments": [],
        "CreatedBy": 999,
        "LastModifiedBy": 999,
        "IsGlobal": False,
        "IsPublished": True,
    },
    {
        "Id": 102,
        "Title": "Office hours this week",
        "Body": {"Text": "", "Html": "<p>Office hours moved to Thursday 3pm.</p>"},
        "StartDate": "2026-03-20T00:00:00Z",
        "EndDate": "2026-03-27T23:59:00Z",
        "CreatedDate": "2026-03-20T09:00:00Z",
        "LastModifiedDate": "2026-03-20T09:00:00Z",
        "IsPinned": True,
        "IsHidden": False,
        "Attachments": [{"FileId": 55, "FileName": "schedule.pdf", "FileSize": 12345}],
        "CreatedBy": 999,
        "LastModifiedBy": 999,
        "IsGlobal": False,
        "IsPublished": True,
    },
]

ANNOUNCEMENTS_CSCI = [
    {
        "Id": 201,
        "Title": "HW6 deadline extended",
        "Body": {"Text": "HW6 is now due March 25.", "Html": ""},
        "StartDate": "2026-03-21T00:00:00Z",
        "EndDate": None,
        "CreatedDate": "2026-03-21T10:00:00Z",
        "LastModifiedDate": "2026-03-21T10:00:00Z",
        "IsPinned": False,
        "IsHidden": False,
        "Attachments": [],
        "CreatedBy": 888,
        "LastModifiedBy": 888,
        "IsGlobal": False,
        "IsPublished": True,
    },
]


def _client() -> httpx.Client:
    return httpx.Client(follow_redirects=True, timeout=10.0)


def _mock_announcements(httpx_mock: HTTPXMock, ou_id: int, payload: list) -> None:
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/le/{LE}/{ou_id}/news/",
        json=payload,
    )


def _mock_announcements_since(httpx_mock: HTTPXMock, ou_id: int, since: str, payload: list) -> None:
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/le/{LE}/{ou_id}/news/?since={since}",
        json=payload,
    )


def _mock_enrollments(httpx_mock: HTTPXMock) -> None:
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/lp/{LP}/enrollments/myenrollments/?pageSize=100",
        json=ENROLLMENTS_TWO_COURSES,
    )


# ---------------------------------------------------------------------------
# get_announcements — unit tests
# ---------------------------------------------------------------------------


def test_get_announcements_returns_list(httpx_mock: HTTPXMock) -> None:
    _mock_announcements(httpx_mock, OU_ID, ANNOUNCEMENTS_BUAD)
    result = get_announcements(_client(), OU_ID)
    assert len(result) == 2


def test_get_announcements_fields(httpx_mock: HTTPXMock) -> None:
    _mock_announcements(httpx_mock, OU_ID, ANNOUNCEMENTS_BUAD)
    result = get_announcements(_client(), OU_ID)
    a = result[0]
    assert a["id"] == 101
    assert a["title"] == "Midterm grades released"
    assert a["body"] == "Midterm Part 1 and Part 2 grades are now available."
    assert a["start_date"] == "2026-03-18T08:00:00Z"
    assert a["end_date"] is None
    assert a["created_date"] == "2026-03-18T07:55:00Z"
    assert a["is_pinned"] is False
    assert a["is_hidden"] is False
    assert a["attachments"] == []


def test_get_announcements_pinned_and_attachments(httpx_mock: HTTPXMock) -> None:
    _mock_announcements(httpx_mock, OU_ID, ANNOUNCEMENTS_BUAD)
    result = get_announcements(_client(), OU_ID)
    a = result[1]
    assert a["is_pinned"] is True
    assert len(a["attachments"]) == 1
    assert a["attachments"][0] == {"id": 55, "name": "schedule.pdf", "size": 12345}


def test_get_announcements_body_falls_back_to_html(httpx_mock: HTTPXMock) -> None:
    """When Text is empty, HTML tags should be stripped for the body."""
    _mock_announcements(httpx_mock, OU_ID, ANNOUNCEMENTS_BUAD)
    result = get_announcements(_client(), OU_ID)
    a = result[1]  # has empty Text, HTML body
    assert a["body"] == "Office hours moved to Thursday 3pm."


def test_get_announcements_empty_course(httpx_mock: HTTPXMock) -> None:
    _mock_announcements(httpx_mock, OU_ID, [])
    result = get_announcements(_client(), OU_ID)
    assert result == []


def test_get_announcements_since_param(httpx_mock: HTTPXMock) -> None:
    since = "2026-03-20T00:00:00Z"
    _mock_announcements_since(httpx_mock, OU_ID, since, [ANNOUNCEMENTS_BUAD[1]])
    result = get_announcements(_client(), OU_ID, since=since)
    assert len(result) == 1
    assert result[0]["id"] == 102


def test_get_announcements_401_raises(httpx_mock: HTTPXMock) -> None:
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/le/{LE}/{OU_ID}/news/",
        status_code=401,
    )
    with pytest.raises(BrightspaceError, match="Session expired"):
        get_announcements(_client(), OU_ID)


def test_get_announcements_403_raises(httpx_mock: HTTPXMock) -> None:
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/le/{LE}/{OU_ID}/news/",
        status_code=403,
    )
    with pytest.raises(BrightspaceError, match="access denied"):
        get_announcements(_client(), OU_ID)


# ---------------------------------------------------------------------------
# CLI — announcements command (single course)
# ---------------------------------------------------------------------------


def test_cli_announcements_single_course_json(httpx_mock: HTTPXMock, tmp_path) -> None:
    _mock_announcements(httpx_mock, OU_ID, ANNOUNCEMENTS_BUAD)
    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["announcements", str(OU_ID)])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 0
    data = json.loads(result.output)
    assert "announcements" in data
    assert len(data["announcements"]) == 2
    assert data["announcements"][0]["course_id"] == OU_ID
    assert data["announcements"][0]["course_code"] is None  # not resolved when single course_id given


def test_cli_announcements_single_course_human(httpx_mock: HTTPXMock, tmp_path) -> None:
    _mock_announcements(httpx_mock, OU_ID, ANNOUNCEMENTS_BUAD)
    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["announcements", str(OU_ID), "--format", "human"])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 0
    assert "Midterm grades released" in result.output
    assert "Office hours this week" in result.output
    assert "[PINNED]" in result.output


def test_cli_announcements_single_course_since(httpx_mock: HTTPXMock, tmp_path) -> None:
    since = "2026-03-20T00:00:00Z"
    _mock_announcements_since(httpx_mock, OU_ID, since, [ANNOUNCEMENTS_BUAD[1]])
    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["announcements", str(OU_ID), "--since", since])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 0
    data = json.loads(result.output)
    assert len(data["announcements"]) == 1
    assert data["announcements"][0]["id"] == 102


# ---------------------------------------------------------------------------
# CLI — announcements command (all courses)
# ---------------------------------------------------------------------------


def test_cli_announcements_all_courses_json(httpx_mock: HTTPXMock, tmp_path) -> None:
    _mock_enrollments(httpx_mock)
    _mock_announcements(httpx_mock, OU_ID, ANNOUNCEMENTS_BUAD)
    _mock_announcements(httpx_mock, OU_ID_2, ANNOUNCEMENTS_CSCI)
    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["announcements"])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 0
    data = json.loads(result.output)
    assert len(data["announcements"]) == 3  # 2 BUAD + 1 CSCI
    course_ids = {a["course_id"] for a in data["announcements"]}
    assert course_ids == {OU_ID, OU_ID_2}
    codes = {a["course_code"] for a in data["announcements"]}
    assert "BUAD-313" in codes
    assert "CSCI-170" in codes


def test_cli_announcements_all_courses_human(httpx_mock: HTTPXMock, tmp_path) -> None:
    _mock_enrollments(httpx_mock)
    _mock_announcements(httpx_mock, OU_ID, ANNOUNCEMENTS_BUAD)
    _mock_announcements(httpx_mock, OU_ID_2, ANNOUNCEMENTS_CSCI)
    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["announcements", "--format", "human"])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 0
    assert "BUAD-313" in result.output
    assert "CSCI-170" in result.output
    assert "HW6 deadline extended" in result.output


def test_cli_announcements_partial_error_continues(httpx_mock: HTTPXMock, tmp_path) -> None:
    """If one course 403s, others should still return; errors in result."""
    _mock_enrollments(httpx_mock)
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/le/{LE}/{OU_ID}/news/",
        status_code=403,
    )
    _mock_announcements(httpx_mock, OU_ID_2, ANNOUNCEMENTS_CSCI)
    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["announcements"])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 0  # still succeeds with partial results
    data = json.loads(result.output)
    assert len(data["announcements"]) == 1  # CSCI only
    assert "errors" in data
    assert data["errors"][0]["course_id"] == OU_ID


def test_cli_announcements_no_session(tmp_path) -> None:
    session_file = tmp_path / "session.json"

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["announcements"])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 1
