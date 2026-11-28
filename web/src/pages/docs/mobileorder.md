---
layout: ../../layouts/Docs.astro
title: Mobile ordering
description: Browse USC dining menus, build carts, get quotes, and manage Transact mobile orders.
---

Shows USC dining menus and sends Transact mobile orders. Requires a USC sign-in. `usc` does not save carts or orders.

```sh
usc mobileorder account
usc mobileorder campus
usc mobileorder locations
usc mobileorder payments
usc mobileorder menu --location ID --fulfillment pickup --payment-method ID [--flat]
usc mobileorder item ITEM_ID --location ID --fulfillment pickup --payment-method ID
usc mobileorder checkout --location ID --fulfillment pickup --payment-method ID
usc mobileorder cart --location ID --fulfillment pickup --payment-method ID --item ITEM_ID [--select ITEM_ID:VALUE_ID] --asap
usc mobileorder quote --cart FILE
usc mobileorder order place --cart FILE --expected-total CENTS
usc mobileorder order continue ORDER_ID
usc mobileorder order check-in ORDER_ID --barcode BARCODE
usc mobileorder order cancel ORDER_ID
usc mobileorder orders [--pending]
```

## Order

1. Make the cart with `cart`.
2. Get the price with `quote`.
3. Send the order with `order place`.
4. Run `order continue` until the stage is `success`.

**Warning:** `order continue` can charge you.

```sh
usc mobileorder cart --location 123 --fulfillment pickup --payment-method 0 \
  --tender 456 --asap --item 789 --select 789:303 > cart.json
usc mobileorder quote --cart cart.json
usc mobileorder order place --cart cart.json --expected-total 385
usc mobileorder order continue ORDER_ID
```

## Cart JSON

To customize each copy of an item, write the cart file yourself. Give it to `--cart FILE`, or use `--cart -` for stdin.

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

The cart must contain `date` and `time`. An empty string sets the next available time. Add one item entry for each unit.

## Notes

- Payment method `0` is the campus card. The campus card also requires `--tender` from `checkout`.
- All prices are integer cents.
- An available item does not show that the location is open. Examine the location status.
- All copies of one item get the same selections.
- For a combo, select the child modifiers with the child item ID.
- Only the stage `success` shows that the order is complete.
- To add a payment card, use the official app.
- `mobileorder_order_unknown`: it is possible that the order exists. Examine `orders --pending` and `orders` before you try again. `usc` does not retry a purchase.

## Live tests

Live tests cover: sign-in, locations, menus, payment methods, orders, a quote, and the rejection of an incorrect expected total.

Live tests do not cover: a completed purchase, `continue`, cancel, check-in, combos, split payments, scheduled pickup, and delivery.
