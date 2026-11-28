---
layout: ../../layouts/Docs.astro
title: Leavey Library
description: Find open study rooms and pods in Leavey Library and book them.
---

Finds and books study rooms and pods in Leavey Library. Room lists and schedules do not require sign-in. Booking requires a USC sign-in.

```sh
usc libcal categories
usc libcal spaces [rooms|pods|all|lvl1|lvl2|lvl3]
usc libcal space SPACE_ID
usc libcal schedule [--date DATE] [--category CATEGORY] [--min-capacity N]
usc libcal availability --start TIMESTAMP --end TIMESTAMP
usc libcal book --space SPACE_ID --start TIMESTAMP --end TIMESTAMP --accept-terms [--field NAME=VALUE]
usc libcal release
usc libcal bookings CANCEL_LINK
usc libcal cancel CANCEL_LINK --booking BOOKING_ID
```

## Book

1. Run `schedule` to find an available space.
2. Copy `--start` and `--end` exactly as `schedule` prints them.
3. Run `book` with `--accept-terms`. This flag accepts the [reservation terms](https://libcal.usc.edu/reserve/lvl1).

```sh
usc libcal schedule --date 2026-11-14 --category rooms --min-capacity 6
usc libcal book --space 31391 --start 2026-11-14T15:00:00-08:00 \
  --end 2026-11-14T17:00:00-08:00 --accept-terms
```

## Cancel

1. Get the cancellation link from your confirmation email.
2. Run `bookings` with the link to list the bookings.
3. Run `cancel` with the link and one booking ID.

```sh
usc libcal bookings 'https://libcal.usc.edu/equipment/cancel?id=CODE'
usc libcal cancel 'https://libcal.usc.edu/equipment/cancel?id=CODE' --booking 134794022
```

## Notes

- LibCal cannot list your bookings. The confirmation email is your record.
- If a booking fails, `usc` releases its hold.
- If a booking outcome is unknown, `usc` blocks new bookings until you clear it. Check your email, then run `release` (it never cancels a confirmed booking).
- `availability` does not hold or book a space. A missing interval is unavailable, and an empty result does not mean the booking window has ended.
- Anyone with a cancellation link can cancel its bookings. The link does not require sign-in.
- `bookings` continues to list past and canceled bookings, with `cancelable: false`.
