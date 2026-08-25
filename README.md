# usc-cli

A small, JSON-first command-line foundation for USC student services.

The project will grow into focused clients for Brightspace, Web Registration,
OASIS, Advise USC, and other systems reached through USC authentication. These
systems share an institution, not an application protocol: Brightspace is D2L,
Advise USC is Salesforce, and USC's registrar applications have their own
contracts. The code keeps those implementations separate.

## Output

Every command writes JSON to stdout. Output is pretty-printed when stdout is
an interactive terminal and compact when it is piped or redirected. Use
`--json` to force compact output or `--pretty` to force pretty-printed output.
Errors are JSON on stderr and follow the same formatting choice.

## Build

```sh
go build -o usc ./cmd/usc
go vet ./...
```

```sh
$ ./usc sites webreg
{"name":"webreg","url":"https://webreg.usc.edu/","login_url":"https://webreg.usc.edu/auth/login?returnUrl=%2FTerms","login":"entra-oidc"}
```

Release builds can set the version without a source edit:

```sh
go build -ldflags '-X main.version=v0.1.0' -o usc ./cmd/usc
```

## Authentication

`auth login` defaults to WebReg and accepts `webreg`, `brightspace`, or
`advise` as an optional site. It first reuses the saved session; if USC needs
a new login, it uses the supplied credentials or prompts for the USC NetID,
password, and Duo bypass code. Pass `--fresh` to intentionally ignore the
saved session. A successful login saves the resulting cross-domain cookie
session.

```sh
usc auth login
usc auth status brightspace
usc auth logout
```

## Brightspace

Brightspace commands reuse the saved session and return JSON. Authenticate once,
then query the course data exposed by Brightspace's Valence API:

```sh
usc auth login brightspace
usc brightspace courses
usc brightspace content COURSE_ID --flat
usc brightspace grades COURSE_ID --graded-only
usc brightspace announcements [COURSE_ID] --since 2026-08-01T00:00:00Z
usc brightspace assignments COURSE_ID
```

For non-interactive use, provide credentials through the environment rather
than command-line arguments:

```sh
USC_USERNAME=netid \
USC_PASSWORD=password \
USC_DUO_BYPASS=123456789 \
usc auth login --non-interactive
```

Passwords and bypass codes are never written to disk. The cookie session lives
at the platform config location under `usc/session.json`; set
`USC_CONFIG_DIR` to override its directory. `auth logout` deletes it.

## Design

- `internal/site` is a descriptive catalog. A site has a stable name, an entry
  URL, and a login protocol.
- Each future site package owns its endpoints, payloads, and response types.
  There is intentionally no universal "USC API" interface.
- Authentication owns the cross-domain browser session used to complete
  Shibboleth, Microsoft, and Duo redirects. Cookies stay scoped to the domains
  that issued them; a site name is not a cookie boundary.
- Cobra and JSON formatting stay in `internal/cli`. Domain packages do not know
  about flags, terminals, or output formatting.

The login values are intentionally specific. Brightspace enters through
Microsoft SAML, WebReg uses Microsoft OpenID Connect, and Advise USC's
Salesforce tenant uses Shibboleth SAML. OASIS remains `legacy`; its former
student landing page now points users to Experience USC. These distinctions are
data, not branches spread across every command.

## Adding a site

Add its catalog entry, then create a package for its actual client only when the
first command needs it. Add shared machinery after two implementations prove it
is shared.
