---
layout: ../../layouts/Docs.astro
title: Install
description: Install the usc command, sign in once, and give your agent the skill.
---

`usc` is a command-line client for USC sites: Brightspace, Web Registration, Handshake, the Student Directory, Leavey Library, mobile ordering, and the Schedule of Classes. Each command prints JSON. An agent runs the command and reads the result.

## With an agent

Give this text to an agent. The agent must be able to run commands and keep credentials private.

```text
Set up the USC CLI for me: https://usc.sh/install.md
```

The agent installs `usc`. Then it gives you a link to sign in. See [Sign in](/docs/auth/).

## By hand

### 1. Install

On macOS and Linux, run:

```sh
curl -fsSL https://usc.sh/install | sh
```

The script installs the latest [release](https://github.com/nsigel/usc-cli/releases) in `~/.local/bin`. Set `USC_INSTALL_DIR` to use another directory.

On all platforms with Go 1.24 or later, run:

```sh
go install github.com/nsigel/usc-cli/cmd/usc@latest
```

On Windows without Go, download `usc_<version>_windows_amd64.exe` from the releases page. Change its name to `usc.exe`.

### 2. Sign in

```sh
usc auth login --web
```

Open the printed `url` and sign in. See [Sign in](/docs/auth/).

### 3. Add the skill

The skill is a short Markdown file. It gives an agent the commands and the rules for their use. Give this text to your agent:

```text
Add the usc skill: save the output of `usc skill` as a skill named usc-cli.
```

Or, save the file yourself. For Claude Code, run:

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

## Hosts

`usc` connects to these hosts. Sign-in uses the Microsoft and Duo hosts. `api-*.duosecurity.com` is one host whose name comes from Duo during sign-in. `usc.sh` is for `usc auth login --remote` and the install script.

```text
account.usc.edu
brightspace.usc.edu
classes.usc.edu
libcal.usc.edu
uscdirectory.usc.edu
webreg.usc.edu
login.microsoftonline.com
login.live.com
us.azureauth.duosecurity.com
uw1.devicemanagement.duosecurity.com
api-*.duosecurity.com
libauth.com
usc.joinhandshake.com
mobileorderprodapi.transactcampus.com
usc.sh
```

## Update

Run the install command again. To see your version, run `usc version`.
