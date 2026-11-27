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
