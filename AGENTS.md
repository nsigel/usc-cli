- Put behavior and result types in library packages. CLI commands parse inputs,
  call the library, and print its result.
- Libraries receive credentials and session paths through `auth.Options`.
  Only `internal/cli` reads configuration paths and `USC_*` variables.
- Put a package in `internal/` unless a Go program outside this module uses it.
  The root package `usc` holds the skill and the configuration paths.
- Keep wire types unexported and resource code in separate files.
- Wrap authentication failures in `*auth.SiteError`; CLI action hints belong
  in `cli.WriteError`.
- Output JSON to stdout; errors go to stderr with exit code 1.
  `usc skill` emits Markdown. Default to pretty JSON in terminals and compact
  JSON in pipes; honor `--pretty` and `--compact`.
- No command aliases or shorthand flags.
- Require explicit booking slots, terms, and form answers.
- Only `usc auth login` may prompt: in a terminal, or on its `--web` or `--remote` page.
- Never log credentials, activation secrets, or cookies.
- Persist only `session.json`, `credentials.json`, and LibCal checkout recovery.
- Verify with live tests; do not add unit tests.
- Write documentation, the skill, and `install.md` in ASD-STE100 Simplified
  Technical English: active voice, present tense, one instruction per sentence.

Before finishing, run:

```sh
gofmt -l .
go vet ./...
go build ./cmd/usc
```
