"""CLI entrypoint."""

from __future__ import annotations

import json
import logging
import sys
from pathlib import Path

import click

from usc_cli import __version__
from usc_cli.brightspace import (
    BrightspaceError,
    get_announcements,
    get_content_toc,
    get_courses,
    get_grades,
    get_whoami,
)
from usc_cli.client import SESSION_PATH, AuthError, USCClient, clear_session


def _setup_logging(verbose: bool) -> None:
    level = logging.DEBUG if verbose else logging.WARNING
    logging.basicConfig(
        format="%(levelname)s %(name)s: %(message)s",
        level=level,
        stream=sys.stderr,
    )


FORMAT_OPTION = click.option(
    "--format",
    "fmt",
    type=click.Choice(["json", "human"]),
    default="json",
    show_default=True,
    help="Output format. 'json' for agent/script use; 'human' for readable output.",
)


@click.group()
@click.version_option(version=__version__, prog_name="usc")
@click.option("-v", "--verbose", is_flag=True, default=False, help="Enable debug logging.")
@click.pass_context
def cli(ctx: click.Context, verbose: bool) -> None:
    """USC university services CLI."""
    ctx.ensure_object(dict)
    ctx.obj["verbose"] = verbose
    _setup_logging(verbose)


@cli.command()
@click.option("--username", "-u", envvar="USC_USERNAME", required=True, help="USC NetID")
@click.option(
    "--password",
    "-p",
    envvar="USC_PASSWORD",
    required=True,
    prompt=True,
    hide_input=True,
    help="USC password (or set USC_PASSWORD env var)",
)
@click.option(
    "--bypass-code",
    "-b",
    envvar="USC_DUO_BYPASS",
    required=True,
    prompt=True,
    hide_input=True,
    help="Duo bypass code (or set USC_DUO_BYPASS env var). Valid for unlimited uses within 1 week.",
)
@click.pass_context
def login(ctx: click.Context, username: str, password: str, bypass_code: str) -> None:
    """Authenticate with USC via SSO + Duo. Saves session cookies on-device."""
    try:
        with USCClient() as client:
            click.echo(f"Logging in as {username}...")
            client.login(username, password, bypass_code)
            click.echo(f"Logged in as {username}. Session saved to {SESSION_PATH}")
    except AuthError as e:
        click.echo(f"Auth failed: {e}", err=True)
        sys.exit(1)


@cli.command()
@FORMAT_OPTION
def status(fmt: str) -> None:
    """Show current session status and authenticated user info.

    Calls /whoami to verify the session is live.

    Output (JSON):
      {"session_path": str, "user": {"Identifier": str, "FirstName": str, "LastName": str, ...}}

    Or on failure:
      {"error": str}
    """
    if not SESSION_PATH.exists():
        msg = {"error": "No session found. Run `usc login` to authenticate."}
        click.echo(json.dumps(msg) if fmt == "json" else msg["error"], err=fmt != "json")
        sys.exit(1)

    with USCClient() as client:
        loaded = client.load_session()
        if not loaded:
            msg = {"error": "Session file exists but could not be loaded. Try `usc login` again."}
            click.echo(json.dumps(msg) if fmt == "json" else msg["error"], err=fmt != "json")
            sys.exit(1)
        try:
            user = get_whoami(client._http)
        except BrightspaceError as e:
            msg = {"error": str(e)}
            click.echo(json.dumps(msg) if fmt == "json" else msg["error"], err=fmt != "json")
            sys.exit(1)

    result = {"session_path": str(SESSION_PATH), "user": user}
    if fmt == "json":
        click.echo(json.dumps(result, indent=2))
    else:
        u = result["user"]
        click.echo(
            f"Logged in as {u.get('FirstName')} {u.get('LastName')} "
            f"({u.get('UniqueName')}) — session: {SESSION_PATH}"
        )


@cli.command()
def logout() -> None:
    """Clear the saved session from disk."""
    clear_session()
    click.echo("Session cleared.")


@cli.command()
@FORMAT_OPTION
@click.pass_context
def courses(ctx: click.Context, fmt: str) -> None:
    """List enrolled courses with their IDs.

    Each course includes: id, code, title, section.
    Use the id with `usc content <id>` to query course content.

    Output (JSON, one object):
      {"courses": [{"id": int, "code": str, "title": str, "section": str}, ...]}
    """
    try:
        with USCClient() as client:
            loaded = client.load_session()
            if not loaded:
                click.echo(
                    json.dumps({"error": "No session found. Run `usc login` to authenticate."}),
                    err=True,
                )
                sys.exit(1)
            result = get_courses(client._http)
    except (AuthError, BrightspaceError) as e:
        click.echo(json.dumps({"error": str(e)}), err=True)
        sys.exit(1)

    if fmt == "json":
        click.echo(json.dumps({"courses": result}, indent=2))
    else:
        if not result:
            click.echo("No course enrollments found.")
            return
        click.echo(f"{'ID':<10} {'Code':<12} {'Section':<25} Title")
        click.echo("-" * 80)
        for c in result:
            click.echo(
                f"{c['id']:<10} {(c['code'] or ''):<12} {(c['section'] or ''):<25} {c['title']}"
            )


@cli.command()
@click.argument("course_id", type=int)
@FORMAT_OPTION
@click.option("--flat", is_flag=True, default=False, help="Flatten all topics into a single list.")
@click.pass_context
def content(ctx: click.Context, course_id: int, fmt: str, flat: bool) -> None:
    """Show content structure (modules + topics) for a course.

    COURSE_ID is the numeric org-unit ID from `usc courses`.

    Output (JSON, one object):
      {
        "course_id": int,
        "modules": [
          {
            "id": int,
            "title": str,
            "topics": [{"id": int, "title": str, "type": int, "url": str, "due_date": str|null}],
            "modules": [...]
          }
        ]
      }

    With --flat, returns {"course_id": int, "topics": [flat list of all topics with module_title]}.
    """
    try:
        with USCClient() as client:
            loaded = client.load_session()
            if not loaded:
                click.echo(
                    json.dumps({"error": "No session found. Run `usc login` to authenticate."}),
                    err=True,
                )
                sys.exit(1)
            toc = get_content_toc(client._http, course_id)
    except (AuthError, BrightspaceError) as e:
        click.echo(json.dumps({"error": str(e)}), err=True)
        sys.exit(1)

    if flat:
        flat_topics: list[dict] = []

        def _flatten(modules: list, path: str = "") -> None:
            for m in modules:
                module_path = f"{path} > {m['title']}".lstrip(" > ")
                for t in m.get("topics", []):
                    flat_topics.append({**t, "module": module_path})
                _flatten(m.get("modules", []), module_path)

        _flatten(toc["modules"])
        result = {"course_id": course_id, "topics": flat_topics}
        if fmt == "json":
            click.echo(json.dumps(result, indent=2))
        else:
            click.echo(f"{'Module':<35} {'Title':<45} URL")
            click.echo("-" * 100)
            for t in flat_topics:
                mod = (t.get("module") or "")[:34]
                title = t["title"][:44]
                url = t.get("url", "")[:40]
                click.echo(f"{mod:<35} {title:<45} {url}")
        return

    if fmt == "json":
        click.echo(json.dumps(toc, indent=2))
    else:
        def _print_module(m: dict, indent: int = 0) -> None:
            pad = "  " * indent
            click.echo(f"{pad}[{m['id']}] {m['title']}")
            for t in m.get("topics", []):
                due = f" (due: {t['due_date']})" if t.get("due_date") else ""
                click.echo(f"{pad}  · {t['title']}{due}")
            for sub in m.get("modules", []):
                _print_module(sub, indent + 1)

        for mod in toc["modules"]:
            _print_module(mod)


@cli.command()
@click.argument("course_id", type=int)
@FORMAT_OPTION
@click.option(
    "--graded-only",
    is_flag=True,
    default=False,
    help="Only show items that have a score.",
)
@click.pass_context
def grades(ctx: click.Context, course_id: int, fmt: str, graded_only: bool) -> None:
    """Show grade items and scores for a course.

    COURSE_ID is the numeric org-unit ID from `usc courses`.

    Output (JSON, one object):
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
            "score": float | null,
            "score_max": float | null,
            "displayed_grade": str | null,
            "feedback": str | null,
            "last_modified": str | null
          },
          ...
        ]
      }
    """
    try:
        with USCClient() as client:
            loaded = client.load_session()
            if not loaded:
                click.echo(
                    json.dumps({"error": "No session found. Run `usc login` to authenticate."}),
                    err=True,
                )
                sys.exit(1)
            result = get_grades(client._http, course_id)
    except (AuthError, BrightspaceError) as e:
        click.echo(json.dumps({"error": str(e)}), err=True)
        sys.exit(1)

    grade_list = result["grades"]
    if graded_only:
        grade_list = [g for g in grade_list if g.get("score") is not None]
        result = {**result, "grades": grade_list}

    if fmt == "json":
        click.echo(json.dumps(result, indent=2))
    else:
        if not grade_list:
            click.echo("No grade items found.")
            return

        click.echo(f"{'Name':<35} {'Score':<15} {'Weight':<8} {'Modified':<14} Feedback")
        click.echo("-" * 100)
        for g in grade_list:
            score_str = g["displayed_grade"] or ("—" if g["score"] is None else str(g["score"]))
            weight_str = f"{g['weight']}%" if g["weight"] is not None else ""
            modified = (g["last_modified"] or "")[:10]
            feedback = (g["feedback"] or "")[:40]
            name = g["name"][:34]
            click.echo(f"{name:<35} {score_str:<15} {weight_str:<8} {modified:<14} {feedback}")


@cli.command()
@click.argument("course_id", type=int, required=False, default=None)
@FORMAT_OPTION
@click.option(
    "--since",
    default=None,
    metavar="DATETIME",
    help="ISO 8601 datetime — only return announcements on or after this date.",
)
@click.pass_context
def announcements(ctx: click.Context, course_id: int | None, fmt: str, since: str | None) -> None:
    """Show course announcements.

    With COURSE_ID: fetch announcements for that course.
    Without COURSE_ID: fetch announcements for all enrolled courses.

    Output (JSON):
      {
        "announcements": [
          {
            "course_id": int,
            "course_code": str | null,
            "id": int,
            "title": str,
            "body": str,
            "start_date": str | null,
            "end_date": str | null,
            "created_date": str | null,
            "last_modified_date": str | null,
            "is_pinned": bool,
            "is_hidden": bool,
            "attachments": [{"id": int, "name": str, "size": int}]
          },
          ...
        ]
      }
    """
    try:
        with USCClient() as client:
            loaded = client.load_session()
            if not loaded:
                click.echo(
                    json.dumps({"error": "No session found. Run `usc login` to authenticate."}),
                    err=True,
                )
                sys.exit(1)

            # Resolve course list
            if course_id is not None:
                course_list = [{"id": course_id, "code": None}]
            else:
                course_list = get_courses(client._http)

            all_items: list[dict] = []
            errors: list[dict] = []

            for course in course_list:
                cid = course["id"]
                code = course.get("code")
                try:
                    items = get_announcements(client._http, cid, since=since)
                    for item in items:
                        all_items.append({"course_id": cid, "course_code": code, **item})
                except BrightspaceError as e:
                    errors.append({"course_id": cid, "error": str(e)})

    except (AuthError, BrightspaceError) as e:
        click.echo(json.dumps({"error": str(e)}), err=True)
        sys.exit(1)

    result: dict = {"announcements": all_items}
    if errors:
        result["errors"] = errors

    if fmt == "json":
        click.echo(json.dumps(result, indent=2))
    else:
        if not all_items:
            click.echo("No announcements found.")
            if errors:
                for err in errors:
                    click.echo(f"  [course {err['course_id']}] {err['error']}", err=True)
            return

        # Group by course
        from itertools import groupby

        def _course_key(x: dict) -> str:
            return x.get("course_code") or str(x["course_id"])

        sorted_items = sorted(all_items, key=_course_key)
        for key, group in groupby(sorted_items, key=_course_key):
            click.echo(f"\n── {key} ──")
            for a in group:
                pinned = " [PINNED]" if a["is_pinned"] else ""
                date = (a.get("start_date") or a.get("created_date") or "")[:10]
                click.echo(f"  [{date}]{pinned} {a['title']}")
                if a["body"]:
                    # First 200 chars of body
                    body_preview = a["body"][:200].replace("\n", " ")
                    click.echo(f"    {body_preview}")

        if errors:
            click.echo("\nErrors:", err=True)
            for err in errors:
                click.echo(f"  [course {err['course_id']}] {err['error']}", err=True)


@cli.command()
@click.argument("url")
@click.option(
    "-o",
    "--output",
    "output_path",
    required=True,
    type=click.Path(writable=True, dir_okay=False),
    help="Local path to write the downloaded file.",
)
@click.pass_context
def download(ctx: click.Context, url: str, output_path: str) -> None:
    """Download a file from Brightspace using your session cookies.

    URL can be a full URL or a path relative to brightspace.usc.edu, e.g.:

      usc download /content/enforced/261076-.../hw7.pdf -o hw7.pdf

      usc download https://brightspace.usc.edu/content/enforced/.../hw7.pdf -o hw7.pdf
    """
    BRIGHTSPACE_BASE = "https://brightspace.usc.edu"

    if not url.startswith("http://") and not url.startswith("https://"):
        url = BRIGHTSPACE_BASE + url

    try:
        with USCClient() as client:
            loaded = client.load_session()
            if not loaded:
                click.echo(
                    json.dumps({"error": "No session found. Run `usc login` to authenticate."}),
                    err=True,
                )
                sys.exit(1)

            response = client._http.get(url)
            if response.status_code != 200:
                click.echo(
                    json.dumps({"error": f"Request failed with status {response.status_code}"}),
                    err=True,
                )
                sys.exit(1)

            output = Path(output_path)
            output.parent.mkdir(parents=True, exist_ok=True)
            output.write_bytes(response.content)

    except (AuthError, BrightspaceError) as e:
        click.echo(json.dumps({"error": str(e)}), err=True)
        sys.exit(1)

    click.echo(json.dumps({"path": str(output), "size": output.stat().st_size}))


if __name__ == "__main__":
    cli()
