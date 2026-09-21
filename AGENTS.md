## Layout

```text
cmd/usc/              executable entrypoint
internal/auth/        USC SSO, Microsoft, Duo, and session-cookie engine
internal/browser/     Chrome DevTools Protocol cookie sync
internal/brightspace/ Brightspace API client
internal/classes/     public Schedule of Classes client
internal/cli/         Cobra commands and JSON formatting
internal/site/        supported USC site catalog
internal/skill/       embedded agent skill definition
```

## Conventions

- Commands emit JSON to stdout, except `usc skill`, which emits raw Markdown.
  JSON output is pretty in a terminal and compact when piped; `--json` and
  `--pretty` override detection.
- Errors are JSON on stderr and return exit code 1.
- Only authentication commands may prompt.
- Never log passwords, bypass codes, or cookie values.
- No unit tests will be written for this repo. You must verify functionality using live tests.
 
## Verify

```bash
gofmt -w cmd internal
go test ./...
go vet ./...
go build ./cmd/usc
```
