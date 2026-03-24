"""CLI entrypoint."""

from __future__ import annotations

import json
import logging
import sys

import click

from usc_cli import __version__
from usc_cli.brightspace import BrightspaceError, get_content_toc, get_courses
from usc_cli.client import SESSION_PATH, AuthError, USCClient, clear_session


def _setup_logging(verbose: bool) -> None:
    level = logging.DEBUG if verbose else logging.WARNING
    logging.basicConfig(
        format="%(levelname)s %(name)s: %(message)s",
        level=level,
        stream=sys.stderr,
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
    help="Duo bypass code (or set USC_DUO_BYPASS env var)",
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
def status() -> None:
    """Show current session status."""
    if not SESSION_PATH.exists():
        click.echo("No session found. Run `usc login` to authenticate.")
        sys.exit(1)

    with USCClient() as client:
        loaded = client.load_session()
        if not loaded:
            click.echo("Session file exists but could not be loaded. Try `usc login` again.")
            sys.exit(1)
        click.echo(f"Session active. Cookies loaded from {SESSION_PATH}")


@cli.command()
def logout() -> None:
    """Clear the saved session from disk."""
    clear_session()
    click.echo("Session cleared.")


FORMAT_OPTION = click.option(
    "--format",
    "fmt",
    type=click.Choice(["json", "human"]),
    default="json",
    show_default=True,
    help="Output format. 'json' for agent/script use; 'human' for readable output.",
)


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


if __name__ == "__main__":
    cli()
