---
layout: ../../layouts/Docs.astro
title: Schedule of Classes
description: Public course sections, times, seats, and instructors from classes.usc.edu.
---

```sh
usc classes TERM_CODE COURSE_CODE
```

Reads `classes.usc.edu`. No sign-in needed.

```sh
usc classes 20271 BUAD-306
```

Returns the course and every section: times, locations, seats, instructors, and syllabus links. `TERM_CODE` is five digits: the year, then `1` for spring, `2` for summer, `3` for fall.
