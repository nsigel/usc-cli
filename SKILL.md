---
name: usc-cli
description: >
  Use the user's USC accounts through the usc CLI. Use this skill when the user
  asks what is due, about grades, announcements, or course files; whether a
  class or section has open seats; to plan or change registration; about career
  fairs and events; to look up a student; to find or book a Leavey Library room;
  or to order food from a USC dining location.
---

# USC CLI

Run `usc` commands for the user. Pass `--help` to any command to see its flags and arguments.

## Rules

- `usc` writes JSON to stdout. All money values are integer cents.
- Errors are JSON on stderr with exit code 1. Check `code`; if `action` is `"usc auth login"`, follow the Sign in section below, and for any other `action`, run that command.
- Get the user's explicit approval for the exact action before running any mutating command: `webreg submit`, `libcal book`, `libcal cancel`, `career-fair-register`, `career-fair-cancel`, `order place`, `order continue`, and `order cancel`.
- Never pick sections, rooms, times, menu modifiers, or form answers on the user's behalf. When more than one option fits, ask.
- Summarize results for the user instead of dumping raw JSON. Format money in dollars.

## Sign in

Run this flow when a command returns `"action": "usc auth login"` or `usc auth status` reports `"authenticated": false`:

1. Start a sign-in page with a `--device-name` the user will recognize in their Duo devices (such as your agent or machine name):
   - `usc auth login --web --device-name NAME` — use this when running on the user's computer, or when you have a browser on your machine that the user can control.
   - `usc auth login --remote --device-name NAME` — use this everywhere else (remote VMs, headless sandboxes, chat-only agents).
   The command prints a one-time `url` valid for 10 minutes.
2. Hand the `url` to the user:
   - If you have a browser the user can control, open `url` there and ask the user to take control.
   - Otherwise, send the full `url` to the user so they can open it on any device.
3. Wait for sign-in to complete with `usc auth status --wait URL`.
   - If your shell tool times out while the user is still signing in, run `usc auth status --wait URL` again.
   - If `usc auth status --wait URL` exits with `sign_in_page_closed` or a sign-in failure, the link is finished — restart at step 1. If it fails to reach the relay, report the error and stop.

Rules for sign-in:

- Never ask for or accept USC passwords, Duo codes, or session files in chat. Never read, screenshot, or type into the sign-in page.
- `--remote` end-to-end encrypts input for your machine through `usc.sh`. Do not create a tunnel or proxy. If `--remote` fails to reach `usc.sh`, report the error and stop; do not fall back to plain `usc auth login`, which prompts in a terminal.
- To check whether a local `127.0.0.1` page is still live, `GET http://127.0.0.1:PORT/healthz` returns `{"status":"ok","expires_at":...}` until sign-in completes or the page expires.

## Courses

`usc brightspace courses` returns course IDs for `content`, `grades`, `announcements`, and `assignments`. `content --flat` returns topic IDs for `download`; `assignments` returns folder IDs for `submissions`.

`usc classes TERM_CODE COURSE_CODE` shows public sections, seats, and meeting times without signing in.

## Registration

`usc webreg terms` lists term codes, and `courses` lists section IDs. Plan changes with `add`, `schedule`, `unschedule`, and `remove`, then show the `checkout` preview to the user before running `submit --confirm`.

`submit` registers all scheduled sections and drops any registered sections that are no longer scheduled.

## Career events

Use `usc handshake events` or `career-fairs`. `career-fair FAIR_ID` lists session IDs for `career-fair-register` and `career-fair-cancel`. Keep the same filters when paging with `--after`.

## Student directory

`usc directory students` returns student IDs for `student`.

## Library rooms

Use `usc libcal availability` for a fixed window or `schedule` for flexible times. When booking, pass the exact space ID and interval timestamps printed by `usc`.

To cancel, get the cancellation link from the user's confirmation email, list bookings with `libcal bookings LINK`, and cancel with `cancel LINK --booking ID`.

## Dining orders

Discover choices with `usc mobileorder locations`, `payments`, `menu`, `item`, and `checkout`. Build the cart with `cart`, price it with `quote`, and show the total, payment method, and pickup time to the user before running `order place`.

## Notes

- WebReg checkout is all-or-nothing: if one section fails, nothing changes, and `messages` explains why.
- On an already-registered section, `webreg grade-option` queues a change request that takes effect at checkout.
- Some Handshake fairs require an attendee form that the user must fill out on Handshake.
- LibCal does not list existing bookings; the confirmation email is the record.
- On `libcal_booking_unknown`, the booking may have gone through. Ask the user to check their email, then run `usc libcal release`.
- An available menu item does not mean the dining location is open; check location status first.
- **Warning:** `order continue` can charge the user. Only stage `success` confirms the order is complete.
- On `mobileorder_order_unknown`, the order may have gone through. Check `orders --pending` and `orders` before retrying; never automatically retry a purchase.
