"""CLI entrypoint."""

import sys

import click
import keyring
from rich.console import Console
from rich.table import Table

from usc_cli import __version__
from usc_cli.auth import AuthError

console = Console()

KEYRING_SERVICE = "usc-cli"
KEYRING_USERNAME_KEY = "username"
KEYRING_PASSWORD_KEY = "password"


@click.group()
@click.version_option(version=__version__, prog_name="usc-cli")
def cli() -> None:
    """USC university portal CLI — Brightspace D2L interaction."""


@cli.command()
@click.option("--username", "-u", envvar="USC_USERNAME", help="USC NetID username")
@click.option("--password", "-p", envvar="USC_PASSWORD", help="USC password")
@click.option(
    "--bypass-code",
    "-b",
    envvar="USC_DUO_BYPASS",
    prompt="Duo bypass code",
    help="Duo MFA bypass code",
)
@click.option("--save", is_flag=True, default=False, help="Save credentials to keychain")
def login(
    username: str | None,
    password: str | None,
    bypass_code: str,
    save: bool,
) -> None:
    """Authenticate with USC Brightspace via SAML SSO + Duo bypass code."""
    from usc_cli.client import USCClient

    # Resolve username
    if not username:
        username = keyring.get_password(KEYRING_SERVICE, KEYRING_USERNAME_KEY)
    if not username:
        username = click.prompt("USC NetID")

    # Resolve password
    if not password:
        password = keyring.get_password(KEYRING_SERVICE, KEYRING_PASSWORD_KEY)
    if not password:
        password = click.prompt("USC password", hide_input=True)

    if save:
        keyring.set_password(KEYRING_SERVICE, KEYRING_USERNAME_KEY, username)
        keyring.set_password(KEYRING_SERVICE, KEYRING_PASSWORD_KEY, password)
        console.print("[green]Credentials saved to keychain.[/green]")

    with console.status("[bold]Authenticating with USC Brightspace..."):
        try:
            client = USCClient()
            client.login(username, password, bypass_code)
        except AuthError as e:
            console.print(f"[red]Authentication failed:[/red] {e}")
            sys.exit(1)

    session = client.session
    console.print("[green bold]Login successful![/green bold]")
    console.print(f"  d2lSessionVal:       {session.get('d2lSessionVal', '')[:20]}...")
    console.print(f"  d2lSecureSessionVal: {session.get('d2lSecureSessionVal', '')[:20]}...")

    client.close()


@cli.command()
@click.option("--username", "-u", envvar="USC_USERNAME", help="USC NetID username")
@click.option("--password", "-p", envvar="USC_PASSWORD", help="USC password")
@click.option("--bypass-code", "-b", envvar="USC_DUO_BYPASS", prompt="Duo bypass code")
def courses(
    username: str | None,
    password: str | None,
    bypass_code: str,
) -> None:
    """List enrolled courses."""
    from usc_cli.client import USCClient

    username = username or keyring.get_password(KEYRING_SERVICE, KEYRING_USERNAME_KEY)
    password = password or keyring.get_password(KEYRING_SERVICE, KEYRING_PASSWORD_KEY)
    if not username:
        username = click.prompt("USC NetID")
    if not password:
        password = click.prompt("USC password", hide_input=True)

    with console.status("[bold]Authenticating..."):
        try:
            client = USCClient()
            client.login(username, password, bypass_code)
        except AuthError as e:
            console.print(f"[red]Auth failed:[/red] {e}")
            sys.exit(1)

    with console.status("[bold]Fetching enrollments..."):
        try:
            items = client.enrollments()
        except Exception as e:
            console.print(f"[red]Failed to fetch courses:[/red] {e}")
            sys.exit(1)

    table = Table(title="Enrolled Courses")
    table.add_column("Org Unit ID", style="dim")
    table.add_column("Course Name")
    table.add_column("Code")

    for item in items:
        ou = item.get("OrgUnit", {})
        table.add_row(
            str(ou.get("Id", "")),
            ou.get("Name", ""),
            ou.get("Code", ""),
        )

    console.print(table)
    client.close()


if __name__ == "__main__":
    cli()
