---
layout: ../../layouts/Docs.astro
title: Leavey Library
description: Find open study rooms and pods in Leavey Library and book them.
---

Room lists and schedules are public and work without signing in. Booking uses your USC sign-in.

```sh
usc libcal categories
usc libcal spaces [rooms|pods|all|lvl1|lvl2|lvl3]
usc libcal space SPACE_ID
usc libcal schedule [--date DATE] [--end-date DATE] [--category CATEGORY] [--min-capacity N] [--max-capacity N]
usc libcal book --space SPACE_ID --start TIMESTAMP --end TIMESTAMP --accept-terms [--field NAME=VALUE]
usc libcal release
```

## Schedule

`schedule` lists each space with its `intervals`: `start`, `end`, `available`, and LibCal's `status`.

| Flag | Values |
| --- | --- |
| `--date` | `today` (default), `tomorrow`, or `YYYY-MM-DD`, in Los Angeles time |
| `--end-date` | Exclusive end date. Defaults to the day after `--date`. |
| `--category` | `rooms`, `pods`, `all` (default), `lvl1`, `lvl2`, `lvl3` |
| `--min-capacity`, `--max-capacity` | Seats |

## Booking

`book` reserves exactly the space and times you give it, up to two hours, and never picks another room. Take `--start` from an available interval and `--end` from the end of the same or a later adjacent available interval, copied as printed.

```sh
usc libcal schedule --date 2026-11-14 --category rooms --min-capacity 6
usc libcal book --space 31391 --start 2026-11-14T15:00:00-08:00 \
  --end 2026-11-14T17:00:00-08:00 --accept-terms
```

`--accept-terms` accepts the [reservation terms](https://libcal.usc.edu/reserve/lvl1). If the checkout form asks a question, the error names the field and its choices; answer with `--field NAME=VALUE`.

LibCal has no way to list your bookings, so the confirmation email is your record. A failed booking releases its hold. If a booking's outcome is unknown, for example because the connection dropped, `usc` refuses new bookings until you check your email and run `usc libcal release`. `release` never cancels a confirmed reservation.
