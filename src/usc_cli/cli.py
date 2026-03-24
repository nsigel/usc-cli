"""CLI entrypoint."""

import click

from usc_cli import __version__


@click.group()
@click.version_option(version=__version__, prog_name="usc-cli")
def cli() -> None:
    """USC university portal CLI — Brightspace D2L interaction."""


@cli.command()
def login() -> None:
    """Authenticate with myUSC / Brightspace."""
    click.echo("Login not yet implemented.")


@cli.command()
def courses() -> None:
    """List enrolled courses."""
    click.echo("Courses not yet implemented.")


if __name__ == "__main__":
    cli()
