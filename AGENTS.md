# AGENTS.md — usc-cli

This orphan branch is the Go foundation for a multi-site USC CLI.

## Conventions

- Keep site-specific HTTP contracts in separate packages.
- Keep shared types small; do not invent a common site API prematurely.
- Commands print JSON to stdout. Errors print JSON to stderr and exit 1.
- Only authentication commands may prompt.
- Never log credentials, MFA values, or cookies.
- Use the standard library unless a dependency clearly removes more code than
  it adds. Cobra is the command boundary.

## Verify

```sh
gofmt -w cmd internal
go vet ./...
go build ./cmd/usc
```
