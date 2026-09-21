# Handshake client design

## Problem and observed contract

USC Handshake is an authenticated tenant application rather than a documented
public API. Browser inspection established the contract the student site uses:

- event categories and lists come from cursor-based GraphQL at `/stu/graphql`;
- full event records come from `GetEvent` at `/hs/graphql`;
- career fairs share the event-search connection under the `CareerFair` model;
- the career-fair detail page embeds its full record and sessions in
  `CareerFairsShowRoot` page data;
- the server accepts only `RELEVANCE` and `DATE` sort enums and exposes no event
  creation or posting timestamp.

The CLI must preserve the shared USC cookie session, JSON/error conventions,
and site-package boundary while making the site's actual filter vocabulary and
pagination usable without exposing GraphQL to callers.

## Selected shape: typed resource client

```go
type Search struct {
    Categories     []string
    Organizer      string
    Keyword        string
    Medium         string
    Date           string
    Sort           string
    PostedBySchool bool
    Limit          int
    After          string
}

type Page[T any] struct {
    Items      []T
    HasMore    bool
    NextCursor string
    CursorKind string
    Sort       string
}

func (c *Client) Categories(context.Context) ([]Category, error)
func (c *Client) Events(context.Context, Search) (Page[EventSummary], error)
func (c *Client) Event(context.Context, int) (EventDetail, error)
func (c *Client) CareerFairs(context.Context, Search) (Page[CareerFairSummary], error)
func (c *Client) CareerFair(context.Context, int) (CareerFairDetail, error)
```

`internal/handshake` owns endpoints, GraphQL documents, wire types, validation,
pagination, page-data parsing, and normalization. Category names and slugs are
resolved against the live category query, while numeric IDs remain accepted.
The CLI translates flags to `Search` and emits normalized domain records.

Organizer is not a server-side list filter. Host and employer names are checked
from each list result; unresolved candidates are hydrated from event or fair
detail so contact names and emails such as `vcareers@usc.edu` work correctly.

The API provides no posted timestamp and rejects posting-related sort enums.
For monitoring, `posted-desc` fetches the filtered result set and orders numeric
event IDs descending. Handshake IDs are the only observed creation-order signal,
so this is intentionally labeled `cursor_kind: "id"` rather than presented as
an exact timestamp sort. Native relevance/date pages retain GraphQL cursors.

This is a deep interface: five operations hide two GraphQL surfaces, embedded
page data, cursor mechanics, category IDs, organizer hydration, and session
expiry detection.

## Alternative: generic GraphQL passthrough

A generic `Query(document, variables)` client would be shorter and make newly
observed fields immediately accessible. It was rejected because every caller
would need to understand Handshake schema names, union fragments, endpoint
selection, cursor encoding, and frontend changes. It would also make CLI JSON
an accidental copy of private wire formats.

## Tradeoffs accepted

- Stable and discoverable JSON requires maintaining explicit event and fair
  types when Handshake changes its frontend contract.
- Exact organizer filtering may make detail requests for list candidates
  because the search operation omits contacts.
- `posted-desc` is useful for detecting newer IDs but is not an authoritative
  posted-at chronology; career-fair detail exposes exact `created_at` separately.
- Career-fair detail parsing is isolated because the live site does not request
  an equivalent complete GraphQL record.
