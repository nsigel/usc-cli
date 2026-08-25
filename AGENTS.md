## Layout

```text
cmd/usc/           executable entrypoint
internal/auth/     USC SSO, Microsoft, Duo, and session-cookie engine
internal/cli/      Cobra commands and JSON formatting
internal/config/   profile, credential, and session paths
```

## Conventions

- Commands emit JSON to stdout. Output is pretty in a terminal and compact when
  piped; `--json` and `--pretty` override detection.
- Errors are JSON on stderr and return exit code 1.
- Only authentication commands may prompt.
- Never log passwords, bypass codes, or cookie values.
- Profile directories use mode `0700`; files containing user data use `0600`.
- No unit tests will be written for this repo. You must verify functionality using live tests.
 
## Verify

```bash
gofmt -w cmd internal
go test ./...
go vet ./...
go build ./cmd/usc
```
