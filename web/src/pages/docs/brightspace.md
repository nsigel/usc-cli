---
layout: ../../layouts/Docs.astro
title: Brightspace
description: Courses, content, grades, announcements, assignments, and downloads from Brightspace.
---

Reads live data from Brightspace (`brightspace.usc.edu`). Requires a USC sign-in. `usc` does not cache data.

```sh
usc brightspace whoami
usc brightspace courses [--all]
usc brightspace content COURSE_ID [--flat]
usc brightspace grades COURSE_ID [--graded-only]
usc brightspace announcements [COURSE_ID] [--since TIME]
usc brightspace assignments COURSE_ID
usc brightspace submissions COURSE_ID FOLDER_ID [--download] [--output DIR]
usc brightspace download COURSE_ID TOPIC_ID [--output DIR]
```

## IDs

| Argument | Source |
| --- | --- |
| `COURSE_ID` | `id` from `courses` |
| `TOPIC_ID` | `id` from `content --flat` |
| `FOLDER_ID` | `id` from `assignments` |

## Examples

```sh
# Show all announcements from this week
usc brightspace announcements --since 2026-11-09T00:00:00-08:00

# Find a file and download it
usc brightspace content 148230 --flat
usc brightspace download 148230 9081123 --output ~/Downloads
```
