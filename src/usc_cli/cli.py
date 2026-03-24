"""CLI entrypoint."""

from __future__ import annotations

import logging
import sys

import click

from usc_cli import __version__
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
    except Exception as e:
        click.echo(f"Unexpected error: {e}", err=True)
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


if __name__ == "__main__":
    cli()
