---
layout: ../../layouts/Docs.astro
title: Sign in
description: Sign in once on a local page; later commands sign in automatically.
---

Sign in one time. Later commands sign in automatically.

```sh
usc auth login --web
usc auth login --remote
usc auth login [--push-device DEVICE_ID | --mobile-code | --sms-device DEVICE_ID | --bypass]
usc auth status
usc auth logout
usc duo device create
usc duo device delete DEVICE_ID
usc bypass create
```

## Sign in

1. Run `usc auth login --web`.
2. Open the printed `url` in ten minutes or less.
3. Sign in with your NetID, password, and Duo.

Sign-in adds a Duo device for USC CLI. Later commands use this device.

An agent gives you the link. Or, the agent opens the link in its browser, and you control the browser. Do not put your password or Duo codes in chat.

Without `--web` or `--remote`, `usc auth login` lists your Duo methods. Run the command again with one of these methods.

## Sign in from a different device

Use `--remote` when `usc` runs on a machine where you cannot open a browser, for example an agent's cloud machine.

1. Run `usc auth login --remote`.
2. Open the printed `https://usc.sh/login/v1/` link on any device in ten minutes or less. Use the full link, with all the text after the `#`.
3. Read the warning and select Accept.
4. Sign in with your NetID, password, and Duo.

Only open a link that your own agent made for you. A link from a different person gives their machine access to your account.

Your browser encrypts your input with a key from the link. The key is the text after the `#`, and the browser does not send it to usc.sh. usc.sh sends only encrypted messages to the machine that runs `usc`. You must trust the page that usc.sh gives you, as you trust the `usc` program.

## Sign out

`usc auth logout` deletes the local credentials and cookies. It does not delete the Duo device. To delete the device, run `usc duo device delete DEVICE_ID`. `DEVICE_ID` is the `id` that sign-in prints.

`bypass create` can cancel your old bypass codes. Its output contains the new code.

## Credential location

`usc` keeps `credentials.json` and `session.json` in this directory:

- macOS: `~/Library/Application Support/usc/`
- Linux: `$XDG_CONFIG_HOME/usc/` or `~/.config/usc/`
- Windows: `%AppData%\usc\`

Keep this directory private. To use a different directory, set `USC_CONFIG_DIR`.
