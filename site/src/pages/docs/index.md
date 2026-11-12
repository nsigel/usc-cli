---
layout: ../../layouts/Docs.astro
title: Install
description: Install the usc command, sign in once, and give your agent the skill.
---

`usc` is a command-line client for USC's Brightspace, Web Registration, Handshake, Leavey Library room booking, and the public Schedule of Classes. Every command prints JSON, so an agent can run it and read the result.

## With an agent

Paste this into Claude Code, Codex, or any agent that can run shell commands:

```text
Set up the USC CLI for me: https://usc.sh/install.md
```

The agent installs `usc` and adds the skill. It signs you in with your NetID, password, and Duo bypass code, unless you run `usc auth login` in your own terminal first.

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
usc auth login
```

It asks for your NetID, password, and a Duo bypass code, then saves them on your machine. Every later command signs itself in. See [Signing in](/docs/auth/).

### 3. Add the skill

The skill is a short Markdown file that teaches an agent the commands and their arguments. Save it where your agent loads skills. For Claude Code:

```sh
mkdir -p ~/.claude/skills/usc-cli
usc skill > ~/.claude/skills/usc-cli/SKILL.md
```

## Try it

```sh
usc brightspace courses
usc libcal schedule --date tomorrow --category rooms --min-capacity 4
usc classes 20271 CSCI-270
```

## Update

Run the install command again. `usc version` prints the version you have.
