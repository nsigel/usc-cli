---
layout: ../../layouts/Docs.astro
title: Schedule of Classes
description: Public course sections, times, seats, and instructors from classes.usc.edu.
---

Reads the public Schedule of Classes (`classes.usc.edu`). Does not require sign-in. Returns the course and all its sections: times, locations, seats, instructors, and syllabus links.

```sh
usc classes TERM_CODE COURSE_CODE
```

`TERM_CODE` is the year and one digit: `1` for spring, `2` for summer, or `3` for fall.

## Examples

```sh
usc classes 20271 BUAD-306
```
