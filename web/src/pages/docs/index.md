---
layout: ../../layouts/Docs.astro
title: Install
description: Install the usc command, sign in once, and give your agent the skill.
---

`usc` is a command-line client for USC sites: Brightspace, Web Registration, Handshake, the Student Directory, Leavey Library, mobile ordering, and the Schedule of Classes. Each command prints JSON. An agent runs the command and reads the result.

## With an agent

Send this prompt to an agent that can run shell commands and store credentials privately:

```text
Set up the USC CLI for me: https://usc.sh/install.md
```

The agent installs `usc`, saves the skill, and gives you a link to sign in. See [Sign in](/docs/auth/).

To update `usc` and its skill later, send the same prompt again.

## By hand

### 1. Install

On macOS and Linux:

```sh
curl -fsSL https://usc.sh/install | sh
```

The script installs the latest [release](https://github.com/nsigel/usc-cli/releases) into `~/.local/bin`. Set `USC_INSTALL_DIR` to use another directory.

On any platform with Go 1.24 or later:

```sh
go install github.com/nsigel/usc-cli/cmd/usc@latest
```

On Windows without Go, download `usc_<version>_windows_amd64.exe` from the releases page and rename it to `usc.exe`.

### 2. Sign in

```sh
usc auth login --web
```

Open the printed `url` and sign in. See [Sign in](/docs/auth/) for `--remote`, terminal, and CI options.

### 3. Add the skill

The skill is a short Markdown file giving agents the commands and rules for using `usc`. Send this prompt to your agent:

```text
Add the usc skill: save the output of `usc skill` as a skill named usc-cli.
```

Or save it manually. For Claude Code:

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

Rerun the install command, then regenerate the skill with `usc skill`. To check your version, run `usc version`.
