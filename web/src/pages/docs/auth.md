---
layout: ../../layouts/Docs.astro
title: Signing in
description: Sign in once on a local page; later commands sign in on their own.
---

```sh
usc auth login --web
```

Open the printed `url` within ten minutes and sign in with your NetID,
password, and Duo. When an agent runs this, it gives you the link, or opens it
in its own browser for you to take control. Never paste your password or Duo
codes into chat.

Sign-in adds a Duo device for USC CLI, so later commands sign in on their own.

## From another device

```sh
usc auth login --remote
```

When `usc` runs on a machine you can't open a browser on, such as an agent's
cloud VM, this prints a `https://usc.sh/login/v1/` link that opens anywhere,
including your phone. The page first asks you to accept that the machine
which made the link will get access to your account.

Your browser encrypts what you type with a key held in the link after the `#`,
which is never sent to usc.sh. The site passes along only encrypted messages,
so your password is not readable in transit or in its logs. You are still
trusting the page usc.sh serves, as you trust the `usc` program itself.

Only open a link you asked your own agent for. A link someone else sends you
signs their machine in to your account.

## Using a terminal

```sh
usc auth login
```

Then rerun it with one of the methods it lists:

```sh
usc auth login --push-device DEVICE_ID
usc auth login --mobile-code
usc auth login --sms-device DEVICE_ID
```

`usc auth login --bypass` signs in with a
[bypass code](https://account.usc.edu/2fa/duo-bypass-code) instead of adding a device.

## Check or disconnect

```sh
usc auth status
usc duo device delete DEVICE_ID
usc auth logout
```

Logout removes local credentials and cookies but keeps the Duo device.

## Where credentials live

`credentials.json` and `session.json` are stored in:

- macOS: `~/Library/Application Support/usc/`
- Linux: `$XDG_CONFIG_HOME/usc/` or `~/.config/usc/`
- Windows: `%AppData%\usc\`

Keep this directory private. Set `USC_CONFIG_DIR` to choose another location.
