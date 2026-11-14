# Mobile ordering integration map

Research date: 2026-11-13.

**Current verified result:** USC SSO, temporary-token exchange, account reads,
and location reads work live (31 locations returned). The exchange returns
`user.login_token` directly. Earlier encrypted-response assumptions below
were superseded: `Mediator.w`/token-save code encrypts tokens for Android
local storage, and rr1 migrates the older stored format. This Go client keeps
the returned bearer token in memory, so neither decoder nor the environment
API key is needed. Only the exchange HMAC protocol constant is required.
No credentials, tokens, or user identifiers are recorded in this document.
 This is a reverse-engineered contract, not a
claim of implemented or authenticated functionality.

## Evidence and current implementation

Source: APKPure download of `com.blackboard.mobileorder`, with client code
reporting version 2026.3.1, decompiled with jadx. Decompilation reported 40
errors, so missing code must not be treated as absent behavior. The APK is
outside the repository. No extracted keys, tokens, or application binaries
are included here. The APK signature has not been independently verified.

Live public discovery succeeded using bundle `com.transact.mobileorder`.
USC Campus Dining is campus 155, Los Angeles, America/Los_Angeles.
The legacy Blackboard bundle returns an app replacement notice.
An unauthenticated `getcampuslocations` call returned status ERROR,
reason TOKEN, message “Login Token was not supplied.” Authenticated response
shapes and purchase behavior remain unverified.

Implemented: `mobileorder.New().Campuses(ctx)` and `.USC(ctx)` in the library.
The CLI exposes USC discovery through `usc mobileorder campus`.

## Transport and authentication

Production origin: https://mobileorderprodapi.transactcampus.com/.
Resource calls use JSON POST under `/api_user/`. HTTP success alone is not
success: inspect the JSON `status`, `reason`, and `message` envelope.
Legacy request construction adds body `userid` and `campusid` (often strings).
Headers include `Content-Type`, `sessionid`, and, for signed-in users,
`login_token`, `userid`, and `campusid`. API-key injection and newer Retrofit
interceptors still need tracing; do not assume all endpoints share identical
headers. The directory endpoint works without an API key.

App evidence: `myobfuscated/h.java` methods R, S, g, h, w0; the Retrofit
interfaces tl, s1, vs1; registration activity listed below.

### USC SSO sequence

1. Read campus configuration. USC currently advertises provider D,
   `mealplan_signup_url=samllogin`, `mealplan_success_url=samlsuccess`.
2. The app opens `/api_user/samllogin` with campusid and the application API
   key. Follow the genuine identity-provider flow using the caller's session.
3. On the configured success page, the app reads hidden fields including
   `temp_token` and `from_reg`; older paths also read email/card/name fields.
4. Modern exchange: `registerwithcampusssotoken`, body `hash`, `temp_token`,
   `language=EN`, `os_type=1`, `campusid`, `app_bundle_name`.
5. The response supplies user/session information. Existing sessions use
   `loginwithtoken`; account reads use `getuser`.

Evidence: `menu/quor/features/register/home/RegisterCampusSSOActivity.java`
(M1, onCreate, J1, K1), and `h.java` H0/G0/s0. The hash helper and token
storage encryption require further analysis before implementation. Do not
reproduce legacy identity-field registration as a substitute for SSO.
The app also has email/password login and registration; these are separate
Transact credentials and must not be confused with USC credentials.

Repository integration: add site.MobileOrder and an auth adapter after
verifying the SSO entry and callback with a real authorized session. Return
`*auth.SiteError` on token rejection. Keep credentials/session ownership
explicit, and extend existing session serialization if necessary; do not
create a new token file or cache. Only auth login may prompt on a terminal.

## Resource contracts

All paths below are relative to `/api_user/`; parameters omit shared identity
fields. These are APK observations unless explicitly marked live above.

| Flow | Endpoint | Inputs / response evidence |
|---|---|---|
| Directory | getcampuseswithbundlename | app_bundle_name; campuses and bundle |
| Locations | getcampuslocations | campusid; locations and cafeterias |
| Menu | getmenu | locationid, target_time, target_date, retrieval_type, payment_method, ct_id, ct_id2; menu, location, menu_time_slots, menu_date_slots |
| Quote | calculatecart | complete cart payload; cart and possibly revised target date/time |
| Submit | processorderstaged | complete cart payload; exact completion protocol unresolved |
| Legacy submit | processorder | selected by FEA_PROCESSORDER_LEGACY_APICALL or FEA_LOC_PROCESSORDER_LEGACY_APICALL |
| Past orders | getpastorders | orders |
| Pending orders | getpendingorders | pending_orders |
| Cancel | cancelpendingorder | orderid |
| Check in | checkinorder | orderid, checkin_barcode |
| Receipt | emailorderreceipt | orderid; sends email |
| Payment setup | addcreditcardgeturl | hosted payment URL |
| Payment token | addcreditcard | bambora_token, card_last4, card_type |
| Meal plan link | addmealplantoken | payment_token |
| Default payment | changedefaultpaymentmethod | card_slot |
| Remove payment | removepaymentmethod | card_slot |
| Account | getuser, loginwithtoken, logout | user/session |
| Automated checkout | getautomatedcheckoutbarcode, getautomatedcheckouttenders | separate Retrofit DTOs; not ordinary cart checkout |

USC directory includes location-scoped tenders, with credit-card and student
meal-plan entries, meal-exchange section restrictions, schedules, and
availability flags. Directory configuration is not an authenticated balance
or guarantee that a tender is usable by the signed-in person.

## Cart schema and semantics

Evidence: `myobfuscated/vd1.java:getJSONForCart`, fn/gn/hn `getJson`.
Most top-level numeric fields are serialized as strings. Preserve wire types
separately from exported library types.

Cart includes locationid, coordinates, app/device metadata, push_token,
payment_method_default, grand_total, credit_total, promo_credit_total,
campaign_credit_total, punchcard_credit_total, servicefee, servicefee_tax,
subtotal, subtotal_tax, pickup_time_min/max, reorderid, promo_code,
code_description, campaign_title/description, redeem_punchid, upsell IDs and
message, special_comment, checkout_select_choiceids, target_time/date,
retrieval_type, pickupspotid, ct_id, ct_id2, cash_eq_swipes, meal_ex_swipes,
and items.

Each item contains numeric itemid and sectionid, boolean meal_ex_applied,
upsell IDs, and options. Each option contains optionid and values; each value
contains valueid, combo_itemid, and recursively nested combo_items. Quantity
representation still needs tracing: do not invent a quantity field.

Quote before submitting. Use server-returned totals and revised scheduling
information, with explicit user selections for items, modifiers, pickup,
tender, and required checkout questions. Currency units, rounding, modifier
constraints, quantity representation, and staged-order completion need live
validation. Do not assume retries of submit/cancel are idempotent. A timeout
after submission must report an unknown outcome and reconcile pending/past
orders before any new submission. No idempotency key has been established.

## Implementation sequence and validation gates

1. Trace request interceptors, hash generation, token handling, and staged
   order completion; finish DTO mapping for menu/options/order/payment.
2. Integrate genuine SSO with auth.Session and verify sign-in, expired-token
   handling, and in-memory/session-file behavior with an authorized account.
3. Implement account, locations, menus, tender availability, and order reads;
   verify them against the app for the same user and location.
4. Implement cart construction and quote. Verify required modifiers, combos,
   multiple quantities, unavailable items, time slots, and meal-plan pricing
   using live quote calls after confirming their side effects.
5. Implement explicit order submission and reconciliation, then cancel/checkin
   where supported. A real purchase test needs a chosen location, complete
   cart, pickup choice, and payment selection from the user.
6. Extend embedded skill and website docs only for implemented functionality.

Suggested library files: session.go, account.go, locations.go, menus.go,
cart.go, orders.go, payments.go, each with private wire structs. CLI commands
only parse inputs, invoke these methods, and print their return values.
No unit tests per repository policy. Run gofmt, go vet, go build and live
checks as each implementation stage is completed.

## Additional legacy endpoint inventory

Statically observed names in h.java; not live-tested or necessarily enabled
for USC. Includes writes and account deletion; this is an inventory only.

- `addcreditcard`
- `addcreditcardgeturl`
- `addmealplandirectauth`
- `addmealplantoken`
- `applypromoregistration`
- `cancelpendingorder`
- `changeaffiliationid`
- `changedefaultpaymentmethod`
- `changeemail`
- `changeemailreceipts`
- `changeinboxcategories`
- `changeloyaltyoptin`
- `changemarketingoptin`
- `changename`
- `changepassword`
- `changephoto`
- `changeprogramofstudy`
- `checkinorder`
- `emailorderreceipt`
- `getaffiliationswithcampus`
- `getcampuslocations`
- `getchallenges`
- `getcontests`
- `getfaqs`
- `getinboxcategories`
- `getinboxmessages`
- `getloyaltydata`
- `getmenu`
- `getpastorders`
- `getpendingorders`
- `getpointstransactions`
- `getpushmessages`
- `getrewards`
- `getuser`
- `ldaplogin`
- `loginwithemailandpassword`
- `loginwithtoken`
- `logout`
- `logoutanddelete`
- `openedmessage`
- `processorderstatuscheck`
- `purchasereward`
- `rateapp`
- `rateorder`
- `registerenterphonenumber`
- `registersendverificationemail`
- `registerwithcampussso`
- `registerwithcampusssotoken`
- `registerwithemail`
- `registerwithgoogle`
- `removepaymentmethod`
- `resetpassword`
- `submitcontestentry`
- `trackappevents`
- `validatepromocodeonetime`

## Authentication trace (follow-up)

### Live checks

`GET /api_user/samllogin?campusid=155` works without api_key: it redirects to
`shibboleth.usc.edu/idp/profile/SAML2/Redirect/SSO`, then serves Shibboleth's
session-storage bootstrap form. A probe using this repository's
`auth.OpenSession(auth.Options{}, auth.CookiePolicy{})` and `Authenticate`
advanced to the credential gate and returned `auth.ErrCredentialsRequired`.
No credentials were submitted. This establishes compatibility through the
pre-login handoff, not successful SSO or a successful token exchange.

### Exact modern exchange

After genuine SSO, require an HTTPS callback on the Transact origin and the
configured success path. Parse the hidden `temp_token` and `from_reg` fields.
The app calls `registerwithcampusssotoken` for the registration/sign-in branch
of this flow; do not blindly exchange a callback from a meal-plan-link branch.
`RegisterCampusSSOActivity.J1` distinguishes these cases.

The exchange body is:

- hash: lowercase hexadecimal HMAC-SHA256 of the UTF-8 temporary token,
  using the app protocol constant from `h.n0` (value omitted here).
- temp_token: the callback token, unchanged.
- language: EN; os_type: 1; campusid: 155.
- app_bundle_name: com.transact.mobileorder.
- userid: "0" when there is no signed-in user (added by h.g).

Legacy APICall transport (`z6` -> `eo1`) adds no API-key interceptor.
Its unauthenticated headers are Content-Type and sessionid. This conclusion
applies to that call path, not every Retrofit service. The API key is still
used by the token-decryption derivation described below.

### Modern token decoding

Response user field: `login_token_aes_encrypted`. The app decrypts it before
placing it in the `login_token` request header:

1. Concatenate decimal userid + registration_datetime + decimal userid +
   registration_datetime, without separators (uu0.a).
2. UTF-8 encode and Android Base64.DEFAULT encode that text (ph.a). Preserve
   Android's line wrapping and trailing newline; ordinary unwrapped base64
   is not necessarily equivalent.
3. PBKDF2-HMAC-SHA256: password is the environment API key, salt is UTF-8
   bytes of the base64 text, 65,536 iterations, output 256 bits (ih2.a).
4. Base64-decode the encrypted token, then AES-256-CBC decrypt with the
   fixed 16-byte protocol IV in bj0.a and PKCS#7-compatible padding
   (Java calls this PKCS5Padding). Interpret plaintext as UTF-8 (rr.a).
5. Keep plaintext private and send it only to the intended API origin.

This is application-protocol encryption, not Android Keystore encryption.
No device-private key is required by the traced code. Values of extracted
protocol keys are deliberately omitted from documentation. Legacy
`login_token_encrypted` uses a different l3 format and must not be fed into
the modern decoder. Modern decoding is statically established but has not
been tested with an actual login response.

### Session continuation and repository design

Authenticated legacy API requests send login_token, userid, campusid and
sessionid headers. `loginwithtoken` with on_launch="0" uses that identity;
this is session validation, not evidence of an independent refresh-token
protocol. Trace token rotation in its response before implementing renewal.
The sessionid value is initialized separately by the application, not taken
from a server auth token; it must not be treated as the authentication secret.

Use `auth.Session.Authenticate` directly in mobileorder.Open so the client
can consume the callback body: generic auth.Open currently discards it.
Add site.MobileOrder for attributed errors. Keep token decoding and private
identity state in mobileorder. Initially keeping the Transact identity in
memory and using saved SSO cookies for a new exchange avoids inventing
another local state file. If token persistence becomes necessary, extend
session.json explicitly rather than disguising bearer tokens as cookies.

Remaining live gate: complete genuine USC/Duo login, inspect callback field
names without printing values, perform the token exchange, decode the
returned token, and verify with getuser and getcampuslocations. Validate
TOKEN/expired-session handling and whether repeated SSO exchange has any
registration side effects. No authenticated access is claimed yet.

## Current implementation and verification

Implemented mobileorder.Open, Account, and Locations with CLI account and
locations commands. Live tests succeeded after refreshing the USC/Duo
session. Both commands independently completed SSO and token exchange;
getuser validated the account identity, and getcampuslocations returned 31
locations. Tokens stay in memory and are excluded from exported outputs.
No orders or payment changes were submitted. Invalid-token handling and
session-expiration behavior have not yet been live-tested.

The prior pre-login failure was a missing Duo bypass code; the subsequent
decoder failure exposed the incorrect assumption about encrypted response
fields. Removed the unused Android storage decoders and API key from the
implementation. gofmt, go vet, go build and git diff --check pass.

## Ordering commands

The client now implements Menu, PaymentMethods, Quote, Orders, PlaceOrder,
ContinueOrder, CancelOrder and CheckInOrder. See [usage](mobileorder-usage.md)
for explicit cart inputs and staged checkout semantics. Menu, payment-method,
quote, and order reads passed live checks; successful mutations remain
unverified. A wrong expected total was rejected before submission.
The live quote changed a requested tender/payment selection; submission now
requires exact agreement with quoted payment, tender, pickup and total.

CLI fulfillment now uses pickup/delivery/address names, translated in the
library to the app enum 0/1/2. Cart JSON uses the same names. Menu output
is normalized to visible item summaries; item details and checkout metadata
are exposed by separate library methods/commands. All three passed live
pickup reads. Raw wire retrieval_type remains internal to the API.
