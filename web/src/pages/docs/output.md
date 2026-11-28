---
layout: ../../layouts/Docs.astro
title: Output and errors
description: The JSON that usc prints, and how it reports errors.
---

Every command except `usc skill` prints JSON to stdout — the exact value returned by the corresponding Go library call, so lists print as JSON arrays.

Output is pretty-printed in terminals and single-line in pipes. Pass `--pretty` or `--compact` to override.

## Errors

Errors print as JSON to stderr with exit code 1. When applicable, `code` identifies the error condition and `action` names the command that resolves it:

```json
{"error":"brightspace: USC sign-in is required","action":"usc auth login"}
```

## Commands

| Command | Function |
| --- | --- |
| `usc auth login` / `status` / `logout` | Sign in, check the session, or sign out. [Sign in](/docs/auth/) |
| `usc duo device ...` | List, create, or delete Duo devices. [Sign in](/docs/auth/) |
| `usc brightspace ...` | Courses, content, grades, announcements, assignments, downloads. [Brightspace](/docs/brightspace/) |
| `usc webreg ...` | Terms, catalog, course bin, calendar, checkout. [WebReg](/docs/webreg/) |
| `usc handshake ...` | Events, career fairs, and fair registration. [Handshake](/docs/handshake/) |
| `usc directory ...` | Student Directory search. [Student Directory](/docs/directory/) |
| `usc libcal ...` | Leavey rooms, schedules, and bookings. [Leavey Library](/docs/libcal/) |
| `usc mobileorder ...` | Dining menus, carts, quotes, and orders. [Mobile ordering](/docs/mobileorder/) |
| `usc classes TERM COURSE` | Public course sections. [Schedule of Classes](/docs/classes/) |
| `usc skill` | Print the agent skill as Markdown |
| `usc version` | Print the version and commit |
