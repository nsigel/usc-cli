"""CLI entrypoint."""

from __future__ import annotations

import logging
import sys

import click

from usc_cli import __version__
from usc_cli.client import AuthError, USCClient


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
    """USC university portal CLI — Brightspace D2L interaction."""
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
    """Authenticate with USC Brightspace via SSO + Duo bypass code."""
    try:
        with USCClient() as client:
            click.echo(f"Logging in as {username}...")
            client.login(username, password, bypass_code)
            click.echo(f"Login successful. Authenticated as {username}.")
    except AuthError as e:
        click.echo(f"Auth failed: {e}", err=True)
        sys.exit(1)


@cli.command()
@click.option("--username", "-u", envvar="USC_USERNAME", required=True, help="USC NetID")
@click.option(
    "--password",
    "-p",
    envvar="USC_PASSWORD",
    required=True,
    prompt=True,
    hide_input=True,
)
@click.option(
    "--bypass-code",
    "-b",
    envvar="USC_DUO_BYPASS",
    required=True,
    prompt=True,
    hide_input=True,
)
@click.pass_context
def courses(ctx: click.Context, username: str, password: str, bypass_code: str) -> None:
    """List enrolled courses."""
    try:
        with USCClient() as client:
            client.login(username, password, bypass_code)
            items = client.enrollments()
            if not items:
                click.echo("No enrollments found.")
                return
            for item in items:
                org = item.get("OrgUnit", {})
                click.echo(
                    f"{org.get('Code', '?'):12s}  {org.get('Name', '?')}  "
                    f"(id={org.get('Id', '?')})"
                )
    except AuthError as e:
        click.echo(f"Auth failed: {e}", err=True)
        sys.exit(1)


if __name__ == "__main__":
    cli()
