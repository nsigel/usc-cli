"""Tests for brightspace.py API layer and CLI grades/courses/content commands.

Uses pytest-httpx to intercept all outbound HTTP — no real network needed.
"""

from __future__ import annotations

import json

import httpx
import pytest
from click.testing import CliRunner
from pytest_httpx import HTTPXMock

from usc_cli.brightspace import (
    BrightspaceError,
    get_content_toc,
    get_courses,
    get_grades,
)
from usc_cli.cli import cli

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

BASE = "https://brightspace.usc.edu"
LP = "1.31"
LE = "1.67"

OU_ID = 262484  # BUAD-313

# Minimal enrollment payload (two courses + one group)
ENROLLMENTS_PAGE = {
    "Items": [
        {
            "OrgUnit": {
                "Id": 258945,
                "Name": "20261_36921 HIST-107: Introduction to the History of Japan",
                "Type": {"Id": 3, "Code": "CourseOffering", "Name": "Course Offering"},
            },
            "Role": {"Id": 110, "Code": "Learner", "Name": "Student"},
        },
        {
            "OrgUnit": {
                "Id": 262484,
                "Name": "20261_14927 BUAD-313: Advanced Operations Management and Analytics",
                "Type": {"Id": 3, "Code": "CourseOffering", "Name": "Course Offering"},
            },
            "Role": {"Id": 110, "Code": "Learner", "Name": "Student"},
        },
        {
            # Group — should be excluded by default
            "OrgUnit": {
                "Id": 267755,
                "Name": "Section 20261_14927",
                "Type": {"Id": 9, "Code": "Group", "Name": "Group"},
            },
            "Role": {"Id": 110, "Code": "Learner", "Name": "Student"},
        },
    ],
    "PagingInfo": {"HasMoreItems": False, "Bookmark": None},
}

# Minimal TOC payload
TOC_PAYLOAD = {
    "Modules": [
        {
            "ModuleId": 9679907,
            "Title": "Syllabus & Office Hours",
            "Description": {"Text": "Course syllabus", "Html": "<p>Course syllabus</p>"},
            "Topics": [
                {
                    "TopicId": 1001,
                    "Title": "Syllabus PDF",
                    "Type": 1,
                    "Url": "/content/enforced/262484/syllabus.pdf",
                    "DueDate": None,
                }
            ],
            "Modules": [],
        },
        {
            "ModuleId": 9679908,
            "Title": "Lectures",
            "Description": None,
            "Topics": [],
            "Modules": [
                {
                    "ModuleId": 9679910,
                    "Title": "Week 1",
                    "Description": None,
                    "Topics": [
                        {
                            "TopicId": 1002,
                            "Title": "Lecture 1 Slides",
                            "Type": 1,
                            "Url": "/content/enforced/262484/lec1.pdf",
                            "DueDate": None,
                        }
                    ],
                    "Modules": [],
                }
            ],
        },
    ]
}

# Grade item definitions
GRADE_ITEMS = [
    {
        "Id": 1402692,
        "Name": "Assignment #1",
        "GradeType": "Numeric",
        "MaxPoints": 100.0,
        "Weight": 10.0,
        "IsBonus": False,
        "ExcludeFromFinalGradeCalculation": False,
        "CategoryId": 0,
        "Description": {"Text": "", "Html": ""},
        "ShortName": "",
        "GradeSchemeId": None,
        "GradeSchemeUrl": "/d2l/api/le/1.67/262484/grades/schemes/0",
        "AssociatedTool": {"ToolId": 2000, "ToolItemId": 522166},
        "IsHidden": False,
        "CanExceedMaxPoints": False,
    },
    {
        "Id": 1433753,
        "Name": "Assignment #2",
        "GradeType": "Numeric",
        "MaxPoints": 100.0,
        "Weight": 10.0,
        "IsBonus": False,
        "ExcludeFromFinalGradeCalculation": False,
        "CategoryId": 0,
        "Description": {"Text": "", "Html": ""},
        "ShortName": "",
        "GradeSchemeId": None,
        "GradeSchemeUrl": "/d2l/api/le/1.67/262484/grades/schemes/0",
        "AssociatedTool": {"ToolId": 2000, "ToolItemId": 522167},
        "IsHidden": False,
        "CanExceedMaxPoints": False,
    },
    {
        # Ungraded item — no matching value entry
        "Id": 9999999,
        "Name": "Final Exam",
        "GradeType": "Numeric",
        "MaxPoints": 200.0,
        "Weight": 30.0,
        "IsBonus": False,
        "ExcludeFromFinalGradeCalculation": False,
        "CategoryId": 0,
        "Description": {"Text": "", "Html": ""},
        "ShortName": "",
        "GradeSchemeId": None,
        "GradeSchemeUrl": "/d2l/api/le/1.67/262484/grades/schemes/0",
        "AssociatedTool": None,
        "IsHidden": False,
        "CanExceedMaxPoints": False,
    },
]

# Grade values (user scores)
GRADE_VALUES = [
    {
        "GradeObjectIdentifier": "1402692",
        "GradeObjectName": "Assignment #1",
        "GradeObjectType": 1,
        "GradeObjectTypeName": "Numeric",
        "PointsNumerator": 92.0,
        "PointsDenominator": 100.0,
        "WeightedNumerator": None,
        "WeightedDenominator": None,
        "DisplayedGrade": "92 / 100",
        "Comments": {"Text": "Good work", "Html": "<p>Good work</p>"},
        "PrivateComments": {"Text": "", "Html": ""},
        "LastModified": "2026-02-10T15:37:05.447Z",
        "LastModifiedBy": None,
        "ReleasedDate": None,
    },
    {
        "GradeObjectIdentifier": "1433753",
        "GradeObjectName": "Assignment #2",
        "GradeObjectType": 1,
        "GradeObjectTypeName": "Numeric",
        "PointsNumerator": 96.0,
        "PointsDenominator": 100.0,
        "WeightedNumerator": None,
        "WeightedDenominator": None,
        "DisplayedGrade": "96 / 100",
        "Comments": {"Text": "", "Html": ""},
        "PrivateComments": {"Text": "", "Html": ""},
        "LastModified": "2026-02-25T17:03:37.000Z",
        "LastModifiedBy": None,
        "ReleasedDate": None,
    },
]


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _client() -> httpx.Client:
    return httpx.Client(follow_redirects=True, timeout=10.0)


def _mock_enrollments(httpx_mock: HTTPXMock) -> None:
    # get_courses appends ?pageSize=100 on first call
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/lp/{LP}/enrollments/myenrollments/?pageSize=100",
        json=ENROLLMENTS_PAGE,
    )


def _mock_toc(httpx_mock: HTTPXMock) -> None:
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/le/{LE}/{OU_ID}/content/toc",
        json=TOC_PAYLOAD,
    )


def _mock_grades(httpx_mock: HTTPXMock) -> None:
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/le/{LE}/{OU_ID}/grades/",
        json=GRADE_ITEMS,
    )
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/le/{LE}/{OU_ID}/grades/values/myGradeValues/",
        json=GRADE_VALUES,
    )


# Fake session file for CLI tests
FAKE_SESSION = [
    {"name": "d2lSessionVal", "value": "fakesession", "domain": "brightspace.usc.edu", "path": "/"},
    {"name": "d2lSecureSessionVal", "value": "fakesecure", "domain": "brightspace.usc.edu", "path": "/"},
]


# ---------------------------------------------------------------------------
# get_courses
# ---------------------------------------------------------------------------


def test_get_courses_returns_course_offerings_only(httpx_mock: HTTPXMock) -> None:
    _mock_enrollments(httpx_mock)
    result = get_courses(_client())
    assert len(result) == 2
    assert all(c["type"] == "Course Offering" for c in result)


def test_get_courses_parses_fields(httpx_mock: HTTPXMock) -> None:
    _mock_enrollments(httpx_mock)
    result = get_courses(_client())
    hist = next(c for c in result if c["id"] == 258945)
    assert hist["code"] == "HIST-107"
    assert hist["title"] == "Introduction to the History of Japan"
    assert hist["section"] == "20261_36921"
    assert hist["role"] == "Student"


def test_get_courses_include_all_returns_groups(httpx_mock: HTTPXMock) -> None:
    _mock_enrollments(httpx_mock)
    result = get_courses(_client(), include_all=True)
    assert len(result) == 3
    types = {c["type"] for c in result}
    assert "Group" in types


def test_get_courses_401_raises(httpx_mock: HTTPXMock) -> None:
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/lp/{LP}/enrollments/myenrollments/?pageSize=100",
        status_code=401,
    )
    with pytest.raises(BrightspaceError, match="Session expired"):
        get_courses(_client())


def test_get_courses_500_raises(httpx_mock: HTTPXMock) -> None:
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/lp/{LP}/enrollments/myenrollments/?pageSize=100",
        status_code=500,
        text="Internal Server Error",
    )
    with pytest.raises(BrightspaceError, match="HTTP 500"):
        get_courses(_client())


def test_get_courses_unparseable_name(httpx_mock: HTTPXMock) -> None:
    """A name that doesn't match the pattern should still return a result."""
    payload = {
        "Items": [
            {
                "OrgUnit": {
                    "Id": 1,
                    "Name": "Some weird course name with no pattern",
                    "Type": {"Id": 3, "Code": "CourseOffering", "Name": "Course Offering"},
                },
                "Role": {"Id": 110, "Code": "Learner", "Name": "Student"},
            }
        ],
        "PagingInfo": {"HasMoreItems": False, "Bookmark": None},
    }
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/lp/{LP}/enrollments/myenrollments/?pageSize=100",
        json=payload,
    )
    result = get_courses(_client())
    assert len(result) == 1
    assert result[0]["code"] is None
    assert result[0]["title"] == "Some weird course name with no pattern"


# ---------------------------------------------------------------------------
# get_content_toc
# ---------------------------------------------------------------------------


def test_get_content_toc_structure(httpx_mock: HTTPXMock) -> None:
    _mock_toc(httpx_mock)
    toc = get_content_toc(_client(), OU_ID)
    assert toc["course_id"] == OU_ID
    assert len(toc["modules"]) == 2


def test_get_content_toc_topics_normalized(httpx_mock: HTTPXMock) -> None:
    _mock_toc(httpx_mock)
    toc = get_content_toc(_client(), OU_ID)
    syllabus = toc["modules"][0]
    assert syllabus["id"] == 9679907
    assert syllabus["title"] == "Syllabus & Office Hours"
    assert len(syllabus["topics"]) == 1
    topic = syllabus["topics"][0]
    assert topic["id"] == 1001
    assert topic["title"] == "Syllabus PDF"
    assert topic["url"] == "/content/enforced/262484/syllabus.pdf"
    assert topic["due_date"] is None


def test_get_content_toc_nested_modules(httpx_mock: HTTPXMock) -> None:
    _mock_toc(httpx_mock)
    toc = get_content_toc(_client(), OU_ID)
    lectures = toc["modules"][1]
    assert len(lectures["modules"]) == 1
    week1 = lectures["modules"][0]
    assert week1["title"] == "Week 1"
    assert len(week1["topics"]) == 1


def test_get_content_toc_401_raises(httpx_mock: HTTPXMock) -> None:
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/le/{LE}/{OU_ID}/content/toc",
        status_code=401,
    )
    with pytest.raises(BrightspaceError, match="Session expired"):
        get_content_toc(_client(), OU_ID)


# ---------------------------------------------------------------------------
# get_grades
# ---------------------------------------------------------------------------


def test_get_grades_merges_items_and_values(httpx_mock: HTTPXMock) -> None:
    _mock_grades(httpx_mock)
    result = get_grades(_client(), OU_ID)
    assert result["course_id"] == OU_ID
    assert len(result["grades"]) == 3  # 2 scored + 1 ungraded


def test_get_grades_scored_item(httpx_mock: HTTPXMock) -> None:
    _mock_grades(httpx_mock)
    result = get_grades(_client(), OU_ID)
    a1 = next(g for g in result["grades"] if g["name"] == "Assignment #1")
    assert a1["score"] == 92.0
    assert a1["score_max"] == 100.0
    assert a1["displayed_grade"] == "92 / 100"
    assert a1["weight"] == 10.0
    assert a1["max_points"] == 100.0
    assert a1["feedback"] == "Good work"
    assert a1["last_modified"] == "2026-02-10T15:37:05.447Z"
    assert a1["is_bonus"] is False
    assert a1["exclude_from_final"] is False


def test_get_grades_ungraded_item(httpx_mock: HTTPXMock) -> None:
    _mock_grades(httpx_mock)
    result = get_grades(_client(), OU_ID)
    final = next(g for g in result["grades"] if g["name"] == "Final Exam")
    assert final["score"] is None
    assert final["displayed_grade"] is None
    assert final["feedback"] is None
    assert final["last_modified"] is None
    assert final["weight"] == 30.0


def test_get_grades_empty_feedback_is_none(httpx_mock: HTTPXMock) -> None:
    """Empty comment Text should normalize to None, not empty string."""
    _mock_grades(httpx_mock)
    result = get_grades(_client(), OU_ID)
    a2 = next(g for g in result["grades"] if g["name"] == "Assignment #2")
    assert a2["feedback"] is None


def test_get_grades_401_raises(httpx_mock: HTTPXMock) -> None:
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/le/{LE}/{OU_ID}/grades/",
        status_code=401,
    )
    with pytest.raises(BrightspaceError, match="Session expired"):
        get_grades(_client(), OU_ID)


def test_get_grades_values_error_raises(httpx_mock: HTTPXMock) -> None:
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/le/{LE}/{OU_ID}/grades/",
        json=GRADE_ITEMS,
    )
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/le/{LE}/{OU_ID}/grades/values/myGradeValues/",
        status_code=403,
    )
    with pytest.raises(BrightspaceError, match="access denied"):
        get_grades(_client(), OU_ID)


# ---------------------------------------------------------------------------
# CLI — courses command
# ---------------------------------------------------------------------------


def test_cli_courses_json_output(httpx_mock: HTTPXMock, tmp_path) -> None:
    _mock_enrollments(httpx_mock)
    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    runner = CliRunner()
    with runner.isolated_filesystem():
        import usc_cli.client as client_mod
        orig = client_mod.SESSION_PATH
        client_mod.SESSION_PATH = session_file
        try:
            result = runner.invoke(cli, ["courses"])
        finally:
            client_mod.SESSION_PATH = orig

    assert result.exit_code == 0
    data = json.loads(result.output)
    assert "courses" in data
    assert len(data["courses"]) == 2
    codes = {c["code"] for c in data["courses"]}
    assert "HIST-107" in codes
    assert "BUAD-313" in codes


def test_cli_courses_human_output(httpx_mock: HTTPXMock, tmp_path) -> None:
    _mock_enrollments(httpx_mock)
    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["courses", "--format", "human"])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 0
    assert "HIST-107" in result.output
    assert "BUAD-313" in result.output


def test_cli_courses_no_session(tmp_path) -> None:
    session_file = tmp_path / "session.json"  # does not exist

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["courses"])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 1


# ---------------------------------------------------------------------------
# CLI — content command
# ---------------------------------------------------------------------------


def test_cli_content_json_output(httpx_mock: HTTPXMock, tmp_path) -> None:
    _mock_toc(httpx_mock)
    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["content", str(OU_ID)])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 0
    data = json.loads(result.output)
    assert data["course_id"] == OU_ID
    assert len(data["modules"]) == 2


def test_cli_content_flat(httpx_mock: HTTPXMock, tmp_path) -> None:
    _mock_toc(httpx_mock)
    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["content", str(OU_ID), "--flat"])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 0
    data = json.loads(result.output)
    assert "topics" in data
    # 1 topic in Syllabus + 1 nested in Lectures > Week 1
    assert len(data["topics"]) == 2
    titles = {t["title"] for t in data["topics"]}
    assert "Syllabus PDF" in titles
    assert "Lecture 1 Slides" in titles


def test_cli_content_flat_module_breadcrumb(httpx_mock: HTTPXMock, tmp_path) -> None:
    _mock_toc(httpx_mock)
    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["content", str(OU_ID), "--flat"])
    finally:
        client_mod.SESSION_PATH = orig

    data = json.loads(result.output)
    nested = next(t for t in data["topics"] if t["title"] == "Lecture 1 Slides")
    assert nested["module"] == "Lectures > Week 1"


# ---------------------------------------------------------------------------
# CLI — grades command
# ---------------------------------------------------------------------------


def test_cli_grades_json_output(httpx_mock: HTTPXMock, tmp_path) -> None:
    _mock_grades(httpx_mock)
    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["grades", str(OU_ID)])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 0
    data = json.loads(result.output)
    assert data["course_id"] == OU_ID
    assert len(data["grades"]) == 3


def test_cli_grades_graded_only(httpx_mock: HTTPXMock, tmp_path) -> None:
    _mock_grades(httpx_mock)
    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["grades", str(OU_ID), "--graded-only"])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 0
    data = json.loads(result.output)
    # Final Exam has no score — should be excluded
    assert len(data["grades"]) == 2
    assert all(g["score"] is not None for g in data["grades"])


def test_cli_grades_human_output(httpx_mock: HTTPXMock, tmp_path) -> None:
    _mock_grades(httpx_mock)
    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["grades", str(OU_ID), "--format", "human"])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 0
    assert "Assignment #1" in result.output
    assert "92 / 100" in result.output
    assert "Final Exam" in result.output


def test_cli_grades_no_session(tmp_path) -> None:
    session_file = tmp_path / "session.json"

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["grades", str(OU_ID)])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 1


# ---------------------------------------------------------------------------
# CLI — status command
# ---------------------------------------------------------------------------

WHOAMI_RESPONSE = {
    "Identifier": "345258",
    "FirstName": "Noah",
    "LastName": "Sigel",
    "Pronouns": "",
    "UniqueName": "4163587521",
    "ProfileIdentifier": "6b3ONDjivG",
}


def test_cli_status_json_output(httpx_mock: HTTPXMock, tmp_path) -> None:
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/lp/{LP}/users/whoami",
        json=WHOAMI_RESPONSE,
    )
    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["status"])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 0
    data = json.loads(result.output)
    assert data["user"]["FirstName"] == "Noah"
    assert data["user"]["UniqueName"] == "4163587521"
    assert "session_path" in data


def test_cli_status_human_output(httpx_mock: HTTPXMock, tmp_path) -> None:
    httpx_mock.add_response(
        method="GET",
        url=f"{BASE}/d2l/api/lp/{LP}/users/whoami",
        json=WHOAMI_RESPONSE,
    )
    session_file = tmp_path / "session.json"
    session_file.write_text(json.dumps(FAKE_SESSION))

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["status", "--format", "human"])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 0
    assert "Noah Sigel" in result.output
    assert "4163587521" in result.output


def test_cli_status_expired_session(httpx_mock: HTTPXMock, tmp_path, monkeypatch) -> None:
    # Clear env creds so auto-reauth does not trigger and hit unmocked login URLs
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

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["status"])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 1


def test_cli_status_no_session(tmp_path) -> None:
    session_file = tmp_path / "session.json"

    runner = CliRunner()
    import usc_cli.client as client_mod
    orig = client_mod.SESSION_PATH
    client_mod.SESSION_PATH = session_file
    try:
        result = runner.invoke(cli, ["status"])
    finally:
        client_mod.SESSION_PATH = orig

    assert result.exit_code == 1
