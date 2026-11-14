# Ordering from the terminal

Sign in with `usc auth login`. Mobile-order commands reuse USC SSO and keep
the Transact bearer token in memory. They do not save carts or order history.

## Browse and choose

```sh
usc mobileorder locations
usc mobileorder payments
usc mobileorder menu --location LOCATION_ID --fulfillment pickup --payment-method PAYMENT_ID
usc mobileorder orders
usc mobileorder orders --pending
```

`menu` returns location status and visible sections with item IDs, names,
prices and availability. Hidden/deleted entries and internal POS metadata
are omitted. Prices are cents; modifiers can change the final price.

```sh
usc mobileorder item ITEM_ID --location LOCATION_ID --fulfillment pickup --payment-method PAYMENT_ID
usc mobileorder checkout --location LOCATION_ID --fulfillment pickup --payment-method PAYMENT_ID
```

`item` returns a description and modifier choices, including required
minimums, maximums and combo item IDs. Fetch a combo item by its ID to see
its own modifier choices. `checkout` returns time/date slots, location-specific
tenders, pickup spots and checkout questions. Item availability does not
mean the location is currently open; check the location status too.

`--fulfillment` accepts `pickup`, `delivery`, or `address`. Payment 0 selects
campus card; 1 and 2 identify saved cards. For campus payment, supply the
selected tender with `--tender` when browsing and `tender_id` in the cart.
Delivery/address purchasing needs location-specific details and has not been
verified; pickup is the live-tested flow. The menu's tender list may differ
by payment context; a quote can return a different effective tender.

## Build and quote a cart

No hand-written JSON is needed for ordinary orders:

```sh
usc mobileorder cart --location LOCATION_ID --fulfillment pickup \
  --payment-method PAYMENT_ID --tender TENDER_ID --asap \
  --item ITEM_ID --select ITEM_ID:VALUE_ID > cart.json
usc mobileorder quote --cart cart.json
```

Omit `--tender` when it is not applicable. Repeat `--item` for additional
items or multiple units. Repeat `--select ITEM_ID:VALUE_ID` for each chosen
modifier value from `item ITEM_ID`. The library supplies section/option IDs
from the current menu and checks required modifier counts. It never picks
default choices. Repeated copies of the same item share its selections;
use the JSON input below if copies need different customization.

For a combo, select its value on the parent item and specify any child
modifiers with the child item ID shown in item details. Required child
choices are checked too. Use `--checkout-choice ID`, `--pickup-spot ID` and
`--comment TEXT` when required. Choose `--asap` explicitly, or provide both
`--date` and `--time` from checkout slots.

Cart output also pipes directly into `usc mobileorder quote --cart -`.
The client stores no cart itself; redirecting to a file is optional and makes
it easy to reuse exactly the same selections when placing the order.

### Advanced JSON input

Supply a JSON object with your explicit selections via `--cart FILE`, or
`--cart -` for stdin. Example shape (IDs below are placeholders):

```json
{
  "location_id": 123,
  "fulfillment": "pickup",
  "payment_method": 0,
  "tender_id": 456,
  "secondary_tender_id": 0,
  "date": "",
  "time": "",
  "pickup_spot_id": 0,
  "comment": "",
  "checkout_choices": [],
  "items": [{
    "itemid": 789,
    "sectionid": 101,
    "meal_ex_applied": false,
    "options": [{
      "optionid": 202,
      "values": [{"valueid": 303, "combo_itemid": 0, "combo_items": []}]
    }]
  }]
}
```

Date and time keys are required. Empty strings explicitly request the app's
immediate/default-time behavior; use returned slot values for a scheduled
pickup. Repeat item entries for multiple units. Combo values contain nested
items with their own options. Select required modifiers and checkout answers
yourself; the client does not choose defaults from the menu.

```sh
usc mobileorder quote --cart cart.json
```

Inspect total, pickup time, payment_method_default, ct_id, ct_id2 and any
banner messages in the returned cart. A quote is not a reservation and does
not guarantee a currently closed location will accept a purchase.

## Place and track

```sh
usc mobileorder order place --cart cart.json --expected-total TOTAL_IN_CENTS
usc mobileorder order continue ORDER_ID
usc mobileorder orders --pending
usc mobileorder order check-in ORDER_ID --barcode ORDER_BARCODE
usc mobileorder order cancel ORDER_ID
```

`place` requotes immediately, requires an exact total match, and rejects
server changes to payment or pickup selections before sending a single
staged submission. Review a changed quote before editing and resubmitting.
`continue` advances the staged purchase and **may process payment**. It is
not a read-only order-status operation. Only stage `success` confirms
checkout success; intermediate stages need further continuation. A `failed`
stage is not success. Use `orders` to read fulfillment status; order output
includes its check-in barcode when supplied by the service.

An unknown submission/continuation outcome is reported as
`mobileorder_order_unknown`. Inspect pending and past orders before any new
submission; do not blindly retry a purchase. The client performs no automatic
purchase retries. Cancellation and check-in are explicit actions on an
existing order. Configure or add payment cards through the official app;
this client lists existing methods only.

## Verification

Live-verified: SSO, account, locations, menus, existing payment methods, past
and pending orders, a single-item quote, and rejection of a mismatched
expected total before submission. The sample fries quote was 385 cents;
prices and availability change.

Not live-verified: successful purchase/continuation, cancellation, check-in,
combo purchases, split payments, or scheduled fulfillment. These operations
are implemented from the app contract but need a user-selected real order
for end-to-end verification. No purchase was made during implementation.
