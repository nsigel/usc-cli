---
layout: ../../layouts/Docs.astro
title: Student Directory
description: Search the USC Student Directory by name, major, school, email, or phone.
---

Searches the [USC Student Directory](https://uscdirectory.usc.edu/web/directory/student/). Requires a USC sign-in.

```sh
usc directory students --query TEXT
usc directory students [--first-name NAME] [--last-name NAME] [--major TEXT] [--school TEXT] [--email TEXT] [--phone TEXT]
usc directory student STUDENT_ID
```

`STUDENT_ID` is an `id` from `students`.

## Notes

- Each value matches as a substring.
- The directory returns at most 100 matches. If `truncated` is `true`, narrow your filters.
- The output does not include fields that a student did not publish.
- `title`, `department`, and `division` occur only for students with a USC job.

## Examples

```sh
usc directory students --query smith
usc directory students --last-name smith --school Viterbi
usc directory student scfj7mn6
```
