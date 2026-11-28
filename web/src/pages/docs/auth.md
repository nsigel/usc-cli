---
layout: ../../layouts/Docs.astro
title: Sign in with NetID and Duo
description: Sign in once with your NetID and Duo in a browser or terminal. usc enrolls its own Duo device, so later commands authenticate automatically.
---

Sign in once. Browser sign-in (and terminal sign-in unless `--bypass` is used) enrolls a dedicated Duo TOTP device for `usc` so later commands authenticate automatically without prompting.

```sh
usc auth login --web [--device-name NAME]
usc auth login --remote [--device-name NAME]
usc auth login [--push-device DEVICE_ID | --mobile-code | --sms-device DEVICE_ID | --bypass] [--device-name NAME]
usc auth status [--wait URL]
usc auth logout [--delete-device]
usc duo device list
usc duo device create [--device-name NAME]
usc duo device delete [DEVICE_ID]
```

## Browser sign-in (`--web`)

Use `--web` on your own computer, or when an agent has a browser on its machine that you can take control of.

1. Run `usc auth login --web` (pass `--device-name NAME` to choose the name shown in your Duo devices).
2. Open the printed `http://127.0.0.1` URL within 10 minutes.
3. Sign in with your NetID, password, and Duo.

When an agent runs sign-in, it either opens the link in a browser you control or gives you the link. Never paste your USC password or Duo codes into chat.

## Remote browser sign-in (`--remote`)

Use `--remote` when `usc` runs on a remote VM, headless sandbox, or cloud agent whose `127.0.0.1` your browser cannot reach.

1. Run `usc auth login --remote` (optionally with `--device-name NAME`).
2. Open the printed `https://usc.sh/login/v1/#...` link on any device within 10 minutes.
3. Review the warning and click **Accept**.
4. Sign in with your NetID, password, and Duo.

Only open a `--remote` link created by your own agent. Your browser encrypts everything with AES-256-GCM using the key in the URL fragment, which is never sent to the server; `usc.sh` forwards only ciphertext to your `usc` process.

## Terminal and CI sign-in

Without `--web` or `--remote`, `usc auth login` signs in through your interactive terminal (or non-interactively via environment variables). Do not use this mode with chat agents.

- **Interactive terminal:** Prompts for your NetID, password, and a Duo method (Duo Push, text message, Duo Mobile code, or bypass code) to authorize enrolling a Duo TOTP device in one pass. You can also skip the menu with `--push-device DEVICE_ID`, `--mobile-code`, or `--sms-device DEVICE_ID`, or pass `--bypass` to sign in with a bypass code *without* enrolling a Duo device.
- **Scripts and CI:** Set `USC_USERNAME`, `USC_PASSWORD`, and either `USC_DUO_TOTP_SECRET` or `USC_DUO_BYPASS`. Environment variables override saved credentials for any command (`usc auth login` still saves `credentials.json`, and authenticated commands still update `session.json`).

## Duo devices, status, and sign out

- **Reuse vs. enrollment:** The first browser sign-in (or terminal sign-in without `--bypass`) creates a Duo TOTP device for `usc` and saves its secret and `duo_device_id` in `credentials.json`. Subsequent sign-ins for the same NetID reuse that saved Duo device instead of creating a duplicate.
- **Listing and deleting devices:** `usc duo device list` lists the Duo devices on your USC account (marking the saved CLI device with `"current": true`). `usc duo device delete [DEVICE_ID]` deletes a phone/TOTP device from Duo, defaulting to the saved CLI device if `DEVICE_ID` is omitted; deleting the saved CLI device also clears `duo_device_id` and `duo_totp_secret` from `credentials.json` so the next command will require signing in again.
- **Checking status:** `usc auth status` reports `authenticated`, `credentials_saved`, and `duo_device_id` (when enrolled from `credentials.json`). If session cookies have expired and a Duo TOTP device is saved, `status` refreshes the session automatically (without spending a bypass code) and returns an auth error if the saved credentials are rejected. Pass `--wait URL` to block until a `--web` or `--remote` sign-in page finishes.
- **Signing out:** `usc auth logout` deletes local `credentials.json` and `session.json`, leaving the Duo device on your USC account. Pass `--delete-device` to also delete the saved CLI Duo device from your USC account before removing local files.

## Credential location

`usc` stores `credentials.json` and `session.json` with `0600` permissions in:

- macOS: `~/Library/Application Support/usc/`
- Linux: `$XDG_CONFIG_HOME/usc/` or `~/.config/usc/`
- Windows: `%AppData%\usc\`

Keep this directory private. Set `USC_CONFIG_DIR` to use a different directory.
