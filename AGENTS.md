## Layout

```text
cmd/usc/              executable entrypoint
auth/                public USC SSO, Microsoft, Duo, and session-cookie engine
browser/             public Chrome DevTools Protocol cookie sync
brightspace/         public Brightspace API client
classes/             public Schedule of Classes client
handshake/           public Handshake API client
libcal/              public library availability and booking client
config/              shared configuration paths
internal/cli/         Cobra commands and JSON formatting
site/                supported USC site catalog
skill/               embedded agent skill definition
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
gofmt -w cmd internal auth browser brightspace classes handshake libcal config site skill
go test ./...
go vet ./...
go build ./cmd/usc
```
