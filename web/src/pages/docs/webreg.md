---
layout: ../../layouts/Docs.astro
title: WebReg
description: Search sections, manage your course bin, and submit registration through Web Registration.
---

Reads and changes your registration in Web Registration. Requires a USC sign-in.

```sh
usc webreg terms
usc webreg programs --term TERM
usc webreg courses --term TERM --program CODE
usc webreg courses --term TERM --course CODE
usc webreg bin --term TERM
usc webreg calendar --term TERM
usc webreg add SECTION_ID --term TERM
usc webreg schedule SECTION_ID --term TERM
usc webreg unschedule SECTION_ID --term TERM
usc webreg remove SECTION_ID --term TERM
usc webreg grade-option SECTION_ID CODE --term TERM
usc webreg checkout --term TERM
usc webreg submit --term TERM --confirm
```

`SECTION_ID` is a section `id` from `courses` or `bin`.

## Register

1. Add the section to your course bin with `add`.
2. Run `checkout` to see the changes. `checkout` changes nothing.
3. Run `submit --confirm` to send the changes.

```sh
usc webreg add 14790 --term 20271
usc webreg checkout --term 20271
usc webreg submit --term 20271 --confirm
```

To drop a registered section, run `unschedule`. Then do steps 2 and 3.

## Notes

- WebReg submits all changes or no changes. If one section fails, WebReg changes nothing. `messages` gives the reason.
- On a registered section (`change_request: true`), `grade-option` only sends a request. The change occurs at checkout.
