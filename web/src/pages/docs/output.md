---
layout: ../../layouts/Docs.astro
title: Output and errors
description: The JSON that usc prints, and how it reports errors.
---

All commands except `usc skill` print JSON to stdout. The JSON is the value that the related Go library call returns. A list prints as a JSON array.

In a terminal, the output is indented. In a pipe, the output is on one line. To set the format, use `--pretty` or `--compact`.

## Errors

`usc` prints errors as JSON to stderr and exits with status 1. If a different command can correct the problem, `action` gives that command:

```json
{"error":"brightspace: USC sign-in is required","action":"usc auth login"}
```

## Commands

| Command | Function |
| --- | --- |
| `usc auth login` / `status` / `logout` | Sign in, examine the session, or sign out. [Sign in](/docs/auth/) |
| `usc duo device ...`, `usc bypass create` | Manage Duo devices and bypass codes. [Sign in](/docs/auth/) |
| `usc brightspace ...` | Courses, content, grades, announcements, assignments, downloads. [Brightspace](/docs/brightspace/) |
| `usc webreg ...` | Terms, catalog, course bin, calendar, checkout. [WebReg](/docs/webreg/) |
| `usc handshake ...` | Events, career fairs, and fair registration. [Handshake](/docs/handshake/) |
| `usc directory ...` | Student Directory search. [Student Directory](/docs/directory/) |
| `usc libcal ...` | Leavey rooms, schedules, and bookings. [Leavey Library](/docs/libcal/) |
| `usc mobileorder ...` | Dining menus, carts, quotes, and orders. [Mobile ordering](/docs/mobileorder/) |
| `usc classes TERM COURSE` | Public course sections. [Schedule of Classes](/docs/classes/) |
| `usc browser sync` | Copy the session into Chrome. [Browser sync](/docs/browser/) |
| `usc skill` | Print the agent skill as Markdown |
| `usc version` | Print the version and commit |
