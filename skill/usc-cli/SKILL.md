---
name: usc-cli
description: "Use USC services through the usc CLI: courses, grades, assignments, registration, career events, library rooms, student directory, and dining orders."
---

# USC CLI

Run `usc` commands for the user. Read JSON results and use `--help` for arguments.

## Sign in

When a command needs sign-in, run `usc auth login --web --device-name NAME`,
with a name the user will recognize in their Duo devices. It prints a `url`
that works for ten minutes.

- If you run on the user's computer, tell them to open the `url` to log in.
- If you run on your own machine, open the `url` in your browser and tell the
  user to take control to log in.

Then run `usc auth status --wait URL` with that `url`; rerun it if your command
times out. If it fails or prints `"authenticated": false`, start over.

Never put credentials in chat. Never read, screenshot, or type into the
sign-in page.

## Courses and assignments

Use `usc brightspace courses`, then `content`, `assignments`, `grades`, or
`announcements` with the course ID. Use `content --flat` to find topic IDs for
`download`; use assignment IDs for `submissions`.

For public sections, seats, and schedules, use `usc classes TERM_CODE COURSE_CODE`.

## Course registration

Use `usc webreg terms` and `courses` to find term and section IDs.
Edit the course bin with `add`, `schedule`, `unschedule`, or `remove`.
Run `checkout` to review changes before `submit --confirm`.
Submitting registers scheduled sections and drops previously registered
sections that were unscheduled.

## Career events

Use `usc handshake events` or `career-fairs`. Fetch `career-fair FAIR_ID`
to get session IDs for `career-fair-register` or `career-fair-cancel`.
Follow `next_cursor` with `--after`, preserving the search filters.

## Student directory

Use `usc directory students` with `--query` or field filters.
Narrow the search when `truncated` is true. Use returned student IDs with
`usc directory student`.

## Library rooms

Use `usc libcal availability` for a fixed time or `schedule` for flexible times.
Book using returned space IDs and interval boundaries; bookings are at most
two hours. Supply the user's form answers and `--accept-terms`.

For cancellation, use the confirmation email's cancellation link with
`usc libcal bookings`, then cancel the selected booking ID.

## Dining orders

Use `usc mobileorder locations`, `payments`, `menu`, `item`, and `checkout`
to find choices. Build JSON with `cart`; use `--help` for modifiers and scheduling.
Money is integer cents. Item availability does not establish that a venue is open.

Quote the cart, review payment, pickup, and total, then use `order place` with
`--expected-total`. `order continue` may charge payment. Only stage `success`
confirms checkout. For `mobileorder_order_unknown`, inspect existing orders
before submitting again. Never automatically retry a purchase.
