---
layout: ../../layouts/Docs.astro
title: Output and errors
description: The JSON that usc prints, and how it reports errors.
---

Every command except `usc skill` prints JSON to stdout: exactly the value the matching Go library call returns. Lists print as JSON arrays.

JSON is indented in a terminal and single-line when piped or redirected. Force either with `--pretty` or `--compact`.

```sh
usc brightspace courses --compact | jq '.[].name'
```

## Errors

Errors are JSON on stderr, and the command exits with status 1. When another command fixes the problem, `action` names it:

```json
{"error":"brightspace: USC sign-in is required","action":"usc auth login"}
```

## Commands

| Command | What it does |
| --- | --- |
| `usc auth login` / `status` / `logout` | Sign in, check, or sign out. [Signing in](/docs/auth/) |
| `usc brightspace ...` | Courses, content, grades, announcements, assignments, downloads. [Brightspace](/docs/brightspace/) |
| `usc webreg ...` | Terms, catalog, course bin, calendar, checkout. [WebReg](/docs/webreg/) |
| `usc handshake ...` | Events and career fairs. [Handshake](/docs/handshake/) |
| `usc libcal ...` | Leavey rooms, schedules, booking. [Leavey Library](/docs/libcal/) |
| `usc classes TERM COURSE` | Public course sections. [Schedule of Classes](/docs/classes/) |
| `usc browser sync` | Copy the session into Chrome. [Browser sync](/docs/browser/) |
| `usc skill` | Print the agent skill as Markdown |
| `usc version` | Print the version and commit |
