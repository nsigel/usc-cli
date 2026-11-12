---
layout: ../../layouts/Docs.astro
title: Signing in
description: How usc signs in to USC through Microsoft and Duo, and where it keeps your session.
---

One sign-in covers every site. `usc auth login` goes through USC's Microsoft sign-in and Duo, then saves your credentials so later commands can sign in again when the session expires. Only `usc auth login` ever prompts.

```sh
usc auth login    # asks for NetID, password, and Duo bypass code
usc auth status   # {"authenticated":true,"credentials_saved":true}
usc auth logout   # deletes the session and the saved credentials
```

## Duo bypass code

`usc` can't approve a Duo push, so it uses a bypass code instead. Get one from the [USC Duo bypass code page](https://account.usc.edu/2fa/duo-bypass-code) and paste it when `usc auth login` asks.

## What's saved

Two files, readable only by you (`0600`):

| File | Contents |
| --- | --- |
| `session.json` | Cookies for the signed-in session |
| `credentials.json` | NetID, password, and Duo bypass code |

They live in:

| Platform | Directory |
| --- | --- |
| macOS | `~/Library/Application Support/usc/` |
| Linux | `$XDG_CONFIG_HOME/usc/` or `~/.config/usc/` |
| Windows | `%AppData%\usc\` |

Set `USC_CONFIG_DIR` to use another directory.

## Scripts and CI

Environment variables override the saved credentials for any command, so a script can run without `usc auth login` and without saving anything:

```sh
USC_USERNAME=ttrojan USC_PASSWORD=... USC_DUO_BYPASS=123456789 usc brightspace courses
```

Outside a terminal, `usc auth login` reads these variables instead of prompting and says which ones are missing.

## Rejected passwords and codes

If USC rejects the password or Duo rejects the bypass code, the command stops after that one attempt and deletes the rejected value from `credentials.json`. The next `usc auth login` asks for a new one. Secrets are never retried automatically and are never accepted as command-line flags.
