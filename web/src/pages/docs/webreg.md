---
layout: ../../layouts/Docs.astro
title: WebReg
description: Search sections, manage your course bin, and submit registration through Web Registration.
---

```sh
usc webreg terms
usc webreg programs --term TERM
usc webreg courses --term TERM --program CODE
usc webreg courses --term TERM --course CODE
usc webreg bin --term TERM
usc webreg calendar --term TERM
usc webreg add|schedule|unschedule|remove SECTION_ID --term TERM
usc webreg grade-option SECTION_ID CODE --term TERM
usc webreg checkout --term TERM
usc webreg submit --term TERM --confirm
```

`TERM` is a five-digit `code` from `usc webreg terms`, like `20271` for Spring 2027. `SECTION_ID` is a section `id` from `courses` or `bin`.

## Registering

Registration works the way it does in the browser: plan in the course bin, then check out.

1. `add` puts a section in your course bin, already scheduled.
2. `schedule` marks a section in the bin for registration. `unschedule` on a registered section marks it to be dropped.
3. `checkout` shows exactly what would be registered and dropped. It changes nothing.
4. `submit --confirm` sends it.

WebReg doesn't process partial registrations. If one section fails, for example because it closed, nothing changes and the reason is in `messages`.

```sh
usc webreg courses --term 20271 --course BUAD-306
usc webreg add 14790 --term 20271
usc webreg checkout --term 20271
usc webreg submit --term 20271 --confirm
```

## Calendar

`calendar` lists your weekly meetings with their scheduled and registered state. Meetings that overlap are marked `Conflicted`.

## Grade options

`grade-option SECTION_ID CODE` takes a code from the section's `grade_option.choices` in `bin`. Unregistered sections change right away. For registered sections (`change_request: true`) it files a request that takes effect at checkout.
