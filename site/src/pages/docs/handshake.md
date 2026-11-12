---
layout: ../../layouts/Docs.astro
title: Handshake
description: Search Handshake events and career fairs and read their details.
---

```sh
usc handshake categories
usc handshake events [FILTERS]
usc handshake event EVENT_ID
usc handshake career-fairs [FILTERS]
usc handshake career-fair CAREER_FAIR_ID
```

## Filters

`events` and `career-fairs` take the same filters.

| Flag | Values |
| --- | --- |
| `--category` | Name, slug, or ID from `categories`. Repeat it or separate with commas. |
| `--organizer` | Host, employer, or contact name or email |
| `--keyword` | Title or keyword search |
| `--medium` | `all`, `virtual`, `in-person` |
| `--date` | `all`, `today`, `next-10`, `next-30`, `past-year` |
| `--posted-by-school` | Only events USC posted |
| `--sort` | `relevance`, `date`, `posted-desc` |
| `--limit` | Results per page, 1 to 100, default 30 |
| `--after` | `next_cursor` from the previous page |

## Pages

Lists return `items`, `has_more`, and `next_cursor`. For the next page, pass `next_cursor` to `--after` with the same filters.

Handshake doesn't expose when an event was posted. `--sort posted-desc` orders by event ID, newest first, which tracks posting order closely enough to watch for new events.

## Examples

```sh
usc handshake events --category employers --category networking --date next-30
usc handshake events --organizer vcareers@usc.edu --medium virtual
usc handshake career-fairs --keyword engineering
usc handshake career-fair 65867
```
