---
layout: ../../layouts/Docs.astro
title: Handshake
description: Search Handshake events and career fairs, read their details, and register for fair sessions.
---

Searches Handshake events and career fairs and registers for fair sessions. Requires a USC sign-in.

```sh
usc handshake categories
usc handshake events [FILTERS]
usc handshake event EVENT_ID
usc handshake career-fairs [FILTERS]
usc handshake career-fair CAREER_FAIR_ID
usc handshake career-fair-register SESSION_ID
usc handshake career-fair-cancel SESSION_ID
```

`events` and `career-fairs` use the same filters. `SESSION_ID` is a session `id` from `career-fair`.

For the next page, give `next_cursor` to `--after`. Use the same filters.

## Notes

- Handshake does not show the date that an event was posted. `--sort posted-desc` sorts by event ID. Event ID order is almost the same as post order. Use it to find new events.
- Some fairs require an attendee form. Complete the form on Handshake.

## Examples

```sh
usc handshake events --category employers --category networking --date next-30
usc handshake career-fairs --keyword engineering
usc handshake career-fair 65867
usc handshake career-fair-register 80258
```
