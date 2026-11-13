---
layout: ../../layouts/Docs.astro
title: Brightspace
description: Courses, content, grades, announcements, assignments, and downloads from Brightspace.
---

Reads live data from Brightspace (`brightspace.usc.edu`). Nothing is cached.

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

| Argument | Where it comes from |
| --- | --- |
| `COURSE_ID` | `id` from `courses` |
| `TOPIC_ID` | `id` from `content --flat` |
| `FOLDER_ID` | `id` from `assignments` |

## Flags

| Flag | Effect |
| --- | --- |
| `courses --all` | Also list departments, semesters, and groups, not just classes |
| `content --flat` | One array of topics, each with its module path |
| `grades --graded-only` | Only items that have a score |
| `announcements --since` | Only announcements on or after an ISO-8601 time, like `2026-11-01T00:00:00-07:00` |
| `submissions --download` | Download every file you submitted instead of listing them |
| `--output` | Directory for downloads, default `.` |

`announcements` without a course covers every course you're enrolled in.

## Examples

```sh
# Everything posted this week
usc brightspace announcements --since 2026-11-09T00:00:00-08:00

# Find a file and download it
usc brightspace content 148230 --flat
usc brightspace download 148230 9081123 --output ~/Downloads
```
