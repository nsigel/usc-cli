---
name: usc-cli
description: >
  Use the user's USC accounts through the usc CLI. Use this skill when the user
  asks what is due, about grades, announcements, or course files; if a class or
  section has open seats; to plan or change registration; about career fairs
  and events; to find a student; to find or book a Leavey Library room; or to
  order food from a USC dining location.
---

# USC CLI

Run `usc` commands for the user. Use `--help` on a command to see its arguments.

## Rules

- `usc` writes JSON to stdout. All money values are integer cents.
- `usc` writes errors as JSON to stderr and exits with code 1. Read `code`.
  If the error has `action`, run that command.
- Before you run a command that changes the user's accounts, get the user's
  approval for that exact change. These commands are `webreg submit`,
  `libcal book`, `libcal cancel`, `career-fair-register`,
  `career-fair-cancel`, `order place`, `order continue`, and `order cancel`.
- Do not select sections, rooms, times, menu modifiers, or form answers for
  the user. If more than one choice is possible, ask the user.
- Give the user a summary. Do not show raw JSON. Show money in dollars.

## Sign in

Do these steps when an error has `"action": "usc auth login"`:

1. Run `usc auth login --web --device-name NAME`. Use a name that the user
   can identify in their Duo devices. The command prints a `url`. The `url`
   is valid for ten minutes.
2. Give the `url` to the user. The user opens it and signs in. If you have a
   browser that the user can control, you can open the `url` there.
3. Run `usc auth status --wait URL`. If your command times out, run it again.
4. If the command fails or prints `"authenticated": false`, start again at
   step 1.

Do not ask for credentials in chat. Do not accept credentials in chat. Do not
read, capture, or type into the sign-in page.

## Courses

`usc brightspace courses` gives the course IDs for `content`, `grades`,
`announcements`, and `assignments`. `content --flat` gives the topic IDs for
`download`. `assignments` gives the folder IDs for `submissions`.

`usc classes TERM_CODE COURSE_CODE` shows public sections, seats, and times.
It does not need sign-in.

## Registration

`usc webreg terms` gives the term codes. `courses` gives the section IDs.
Use `add`, `schedule`, `unschedule`, and `remove` to change the course bin.
Show the `checkout` preview to the user before you run `submit --confirm`.

`submit` registers the scheduled sections. It also drops the registered
sections that are not scheduled.

## Career events

Use `usc handshake events` or `career-fairs`. `career-fair FAIR_ID` gives the
session IDs for `career-fair-register` and `career-fair-cancel`. When you use
`--after` for the next page, keep the same filters.

## Student directory

`usc directory students` gives the student IDs for `student`.

## Library rooms

Use `usc libcal availability` for a fixed time. Use `schedule` for flexible
times. To book, copy the space ID and interval times exactly as `usc` prints
them.

To cancel, get the cancellation link from the user's confirmation email. Run
`libcal bookings LINK`. Then run `cancel LINK --booking ID`.

## Dining orders

Use `usc mobileorder locations`, `payments`, `menu`, `item`, and `checkout` to
find the choices. Use `cart` to make the cart. Then use `quote` to get the
price. Show the total, payment, and pickup time to the user before you run
`order place`.

## Notes

- WebReg submits all changes or no changes. If one section fails, WebReg
  changes nothing. `messages` gives the reason.
- On a registered section, `webreg grade-option` only sends a request. The
  change occurs at checkout.
- Some Handshake fairs require an attendee form. The user must complete the
  form on Handshake.
- LibCal cannot list the user's bookings. The confirmation email is the record.
- `libcal_booking_unknown`: it is possible that the booking exists. Tell the
  user to examine their email. Then run `usc libcal release`.
- An available menu item does not show that the location is open. Examine the
  location status.
- **Warning:** `order continue` can charge the user. Only the stage `success`
  shows that the order is complete.
- `mobileorder_order_unknown`: it is possible that the order exists. Examine
  `orders --pending` and `orders` before you try again. Do not automatically
  retry a purchase.
