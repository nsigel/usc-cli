"""Brightspace Valence API client.

Uses the session cookies saved by USCClient.login() — no OAuth registration needed.
All methods return plain Python dicts/lists (JSON-serializable). Callers decide
how to render.
"""

from __future__ import annotations

import re
from typing import Any

import httpx

BRIGHTSPACE_BASE = "https://brightspace.usc.edu"

# Valence LP and LE versions confirmed working against brightspace.usc.edu
LP_VER = "1.31"
LE_VER = "1.67"

# OrgUnit type names we care about
COURSE_OFFERING_TYPE = "Course Offering"


class BrightspaceError(Exception):
    """Raised when a Brightspace API call fails."""


def _require_ok(resp: httpx.Response, label: str) -> dict[str, Any] | list[Any]:
    if resp.status_code == 401:
        raise BrightspaceError("Session expired or invalid — run `usc login` to re-authenticate.")
    if resp.status_code == 403:
        raise BrightspaceError(f"{label}: access denied (403)")
    if not resp.is_success:
        raise BrightspaceError(f"{label}: HTTP {resp.status_code} — {resp.text[:200]}")
    return resp.json()


# ---------------------------------------------------------------------------
# Parsed course model
# ---------------------------------------------------------------------------

# Names from enrollments look like:
#   "20261_36921 HIST-107: Introduction to the History of Japan"
#   "20261_30108/30295 CSCI-170: Discrete Methods in Computer Science"
_COURSE_NAME_RE = re.compile(
    r"^(?P<section>\S+)\s+"
    r"(?P<code>[A-Z]+-\d+[A-Z]*):\s+"
    r"(?P<title>.+)$"
)


def _parse_course(enrollment: dict[str, Any]) -> dict[str, Any]:
    """Normalize a raw enrollment into a structured course dict."""
    ou = enrollment.get("OrgUnit", {})
    raw_name = ou.get("Name", "")
    ou_id = ou.get("Id")
    ou_type = ou.get("Type", {}).get("Name", "")
    role = enrollment.get("Role", {}).get("Name", "")

    m = _COURSE_NAME_RE.match(raw_name)
    if m:
        code = m.group("code")
        title = m.group("title")
        section = m.group("section")
    else:
        code = None
        title = raw_name
        section = None

    return {
        "id": ou_id,
        "code": code,
        "title": title,
        "section": section,
        "type": ou_type,
        "role": role,
        "raw_name": raw_name,
    }


# ---------------------------------------------------------------------------
# API functions
# ---------------------------------------------------------------------------


def get_whoami(http: httpx.Client) -> dict[str, Any]:
    """Return current user identity from /d2l/api/lp/.../users/whoami."""
    resp = http.get(f"{BRIGHTSPACE_BASE}/d2l/api/lp/{LP_VER}/users/whoami")
    return _require_ok(resp, "whoami")  # type: ignore[return-value]


def get_courses(
    http: httpx.Client,
    *,
    include_all: bool = False,
) -> list[dict[str, Any]]:
    """Return list of enrolled courses.

    By default only returns Course Offering type (excludes Groups, Organization).
    Pass include_all=True to return all enrollment types.
    """
    page_url = f"{BRIGHTSPACE_BASE}/d2l/api/lp/{LP_VER}/enrollments/myenrollments/"
    results: list[dict[str, Any]] = []

    while page_url:
        resp = http.get(page_url, params={"pageSize": 100} if "?" not in page_url else {})
        data = _require_ok(resp, "enrollments")
        assert isinstance(data, dict)
        for e in data.get("Items", []):
            parsed = _parse_course(e)
            if include_all or parsed["type"] == COURSE_OFFERING_TYPE:
                results.append(parsed)
        # Follow paging bookmark if present
        paging = data.get("PagingInfo", {})
        if paging.get("HasMoreItems") and paging.get("Bookmark"):
            page_url = (
                f"{BRIGHTSPACE_BASE}/d2l/api/lp/{LP_VER}/enrollments/myenrollments/"
                f"?bookmark={paging['Bookmark']}"
            )
        else:
            page_url = None  # type: ignore[assignment]

    return results


def get_content_toc(http: httpx.Client, course_id: int) -> dict[str, Any]:
    """Return the full table of contents for a course as a nested dict.

    Structure:
      {
        "course_id": int,
        "modules": [
          {
            "id": int,
            "title": str,
            "topics": [{"id": int, "title": str, "type": int, "url": str}, ...],
            "modules": [...]   # recursive
          },
          ...
        ]
      }
    """
    resp = http.get(f"{BRIGHTSPACE_BASE}/d2l/api/le/{LE_VER}/{course_id}/content/toc")
    data = _require_ok(resp, f"toc({course_id})")
    assert isinstance(data, dict)

    def _normalize_module(m: dict[str, Any]) -> dict[str, Any]:
        return {
            "id": m.get("ModuleId"),
            "title": m.get("Title", ""),
            "description": m.get("Description", {}).get("Text") if m.get("Description") else None,
            "topics": [_normalize_topic(t) for t in m.get("Topics", [])],
            "modules": [_normalize_module(sub) for sub in m.get("Modules", [])],
        }

    def _normalize_topic(t: dict[str, Any]) -> dict[str, Any]:
        return {
            "id": t.get("TopicId"),
            "title": t.get("Title", ""),
            "type": t.get("Type"),  # 1=File, 3=Link, etc.
            "url": t.get("Url", ""),
            "due_date": t.get("DueDate"),
        }

    return {
        "course_id": course_id,
        "modules": [_normalize_module(m) for m in data.get("Modules", [])],
    }


def get_grades(http: httpx.Client, course_id: int) -> dict[str, Any]:
    """Return grade items merged with the current user's grade values.

    Structure:
      {
        "course_id": int,
        "grades": [
          {
            "id": str,
            "name": str,
            "grade_type": str,
            "max_points": float | null,
            "weight": float | null,
            "is_bonus": bool,
            "exclude_from_final": bool,
            "score": float | null,         # PointsNumerator
            "score_max": float | null,     # PointsDenominator
            "displayed_grade": str | null,
            "feedback": str | null,        # instructor comment (plain text)
            "last_modified": str | null,   # ISO timestamp
          },
          ...
        ]
      }
    """
    # Fetch grade item definitions
    r_items = http.get(f"{BRIGHTSPACE_BASE}/d2l/api/le/{LE_VER}/{course_id}/grades/")
    items = _require_ok(r_items, f"grades/items({course_id})")
    assert isinstance(items, list)

    # Fetch user's grade values
    r_vals = http.get(
        f"{BRIGHTSPACE_BASE}/d2l/api/le/{LE_VER}/{course_id}/grades/values/myGradeValues/"
    )
    values_raw = _require_ok(r_vals, f"grades/values({course_id})")
    assert isinstance(values_raw, list)

    # Index values by GradeObjectIdentifier (string id)
    values_by_id: dict[str, dict[str, Any]] = {
        str(v["GradeObjectIdentifier"]): v for v in values_raw
    }

    grades = []
    for item in items:
        item_id = str(item.get("Id", ""))
        val = values_by_id.get(item_id, {})

        # Extract plain-text feedback from HTML comments
        feedback: str | None = None
        raw_comment = val.get("Comments", {})
        if raw_comment:
            text = raw_comment.get("Text", "").strip()
            feedback = text if text else None

        grades.append({
            "id": item_id,
            "name": item.get("Name", ""),
            "grade_type": item.get("GradeType"),
            "max_points": item.get("MaxPoints"),
            "weight": item.get("Weight"),
            "is_bonus": item.get("IsBonus", False),
            "exclude_from_final": item.get("ExcludeFromFinalGradeCalculation", False),
            "score": val.get("PointsNumerator"),
            "score_max": val.get("PointsDenominator"),
            "displayed_grade": val.get("DisplayedGrade"),
            "feedback": feedback,
            "last_modified": val.get("LastModified"),
        })

    return {"course_id": course_id, "grades": grades}


def get_dropbox_folders(http: httpx.Client, course_id: int) -> list[dict[str, Any]]:
    """Return dropbox/assignment folders for a course."""
    resp = http.get(f"{BRIGHTSPACE_BASE}/d2l/api/le/{LE_VER}/{course_id}/dropbox/folders/")
    data = _require_ok(resp, f"dropbox({course_id})")
    assert isinstance(data, list)
    return [
        {
            "id": f.get("Id"),
            "name": f.get("Name", ""),
            "due_date": f.get("DueDate"),
            "availability": f.get("Availability"),
            "grading_type": f.get("GradingType"),
            "score": f.get("Score"),
        }
        for f in data
    ]
