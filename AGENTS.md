## Layout

```text
cmd/usc/             executable entrypoint
auth/                USC SSO (Microsoft, Duo), the cookie session, and saved credentials
brightspace/         Brightspace client, one file per resource
handshake/           Handshake events and career fairs
libcal/              Leavey Library availability and booking
webreg/              Web Registration
classes/             public Schedule of Classes
browser/             Chrome DevTools Protocol cookie sync
site/                site names and their sign-in pages
config/              the usc command's configuration file paths
skill/               embedded agent skill definition
internal/cli/        Cobra commands: parse flags, call a package, print JSON
internal/dom/        HTML helpers shared by the clients
examples/            runnable library examples
```

## Conventions

- Every feature is a library call first. A command only parses flags, calls
  the package, and prints exactly what it returns; neither logic nor output
  shapes live in `internal/cli`. Commands with nothing to return print a
  one-field acknowledgement.
- The library holds no ambient state. A program signs in with `auth.Options`
  (explicit credentials or a credentials file) and owns its session,
  which stays in memory unless it names a `SessionFile`. Only `internal/cli`
  knows about the usc configuration directory and the `USC_*` environment
  variables (`signIn` in `internal/cli/open.go`).
- Each service package has a `client.go` (construction, `Open`, HTTP, errors)
  and one file per resource or step. Wire types stay unexported.
- Authentication failures are `*auth.SiteError`; do not add per-package
  session errors. The library never names a `usc` command; `WriteError` in
  `internal/cli` adds the CLI's `action` hint.
- Commands emit JSON to stdout, except `usc skill`, which emits raw Markdown.
  JSON output is pretty in a terminal and single-line when piped; `--compact`
  and `--pretty` override detection.
- Errors are JSON on stderr and return exit code 1.
- One name per command and one flag per input: no aliases or shorthand forms.
- Never guess for the user: no default room, slot, term, or form answer.
- Only `usc auth login` may prompt, and only on a terminal.
- Never log passwords, bypass codes, or cookie values.
- Local state is limited to `session.json`, `credentials.json`, and LibCal's
  checkout recovery record. Do not add caches or histories.
- No unit tests will be written for this repo. You must verify functionality using live tests.

## Verify

```bash
gofmt -l .
go vet ./...
go build ./cmd/usc
```

## Before opening a pull request

Every time, in this order:

1. Run the comment-sicko pass in `.claude/agents/comment-sicko.md` over the
   diff against `main`: spawn it as a subagent where the harness supports
   that, otherwise read the file and carry out its instructions yourself.
   Delete what it condemns and fix every `MUST KILL` it flags.
2. Verify the change live: run the checks under Verify, then exercise the
   behaviour you changed for real and confirm it works.
3. If the change is visual, capture screenshots of it and attach them to the
   pull request with the `gh` CLI (it uploads attachments natively).
