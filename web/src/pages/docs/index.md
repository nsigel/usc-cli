---
layout: ../../layouts/Docs.astro
title: Install
description: Install the usc command, sign in once, and give your agent the skill.
---

`usc` is a command-line client for USC's Brightspace, Web Registration, Handshake, the Student Directory, Leavey Library room booking, USC mobile ordering, and the public Schedule of Classes. Every command prints JSON, so an agent can run it and read the result.

## With an agent

Tell an agent that can run commands and store credentials privately:

```text
Set up the USC CLI for me: https://usc.sh/install.md
```

Your agent installs USC CLI and gives you a link to sign in. See [Signing in](/docs/auth/).

## By hand

### 1. Install

macOS and Linux:

```sh
curl -fsSL https://usc.sh/install | sh
```

The script downloads the latest [release](https://github.com/nsigel/usc-cli/releases) for your platform into `~/.local/bin`. Set `USC_INSTALL_DIR` to put it somewhere else.

With Go 1.24 or later, on any platform:

```sh
go install github.com/nsigel/usc-cli/cmd/usc@latest
```

On Windows without Go, download `usc_<version>_windows_amd64.exe` from the releases page and rename it to `usc.exe`.

### 2. Sign in

```sh
usc auth login --web
```

Open the printed `url` and sign in. See [Signing in](/docs/auth/).

### 3. Add the skill

The skill is a short Markdown file that teaches an agent the commands and their arguments. Ask your agent:

```text
Add the usc skill: save the output of `usc skill` as a skill named usc-cli.
```

Or save it yourself. For Claude Code:

```sh
mkdir -p ~/.claude/skills/usc-cli
usc skill > ~/.claude/skills/usc-cli/SKILL.md
```

## Try it

```sh
usc brightspace courses
usc libcal schedule --date tomorrow --category rooms --min-capacity 4
usc classes 20271 CSCI-270
usc mobileorder locations
```

## Update

Run the install command again. `usc version` prints the version you have.

For dining, [browse menus, build a cart, and quote an order](/docs/mobileorder/).
