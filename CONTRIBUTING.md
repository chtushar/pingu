# Contributing to pingu

## Prerequisites

- Go 1.25 or newer

## Everyday commands

```sh
make build        # build ./bin/pingu
make test         # run all tests
make test-race    # run tests with the race detector
make vet          # go vet ./...
make fmt          # format all code
make check        # fmt-check + vet + test + test-race + build (run before pushing)
```

## Conventions

- `gofmt`-formatted code; imports grouped stdlib / internal / third-party.
- Errors are wrapped with `fmt.Errorf("context: %w", err)`.
- Structured logging via `log/slog` (JSON to stderr); never `fmt.Println`
  for diagnostics. Secrets and message content stay out of logs.
- Standard library `testing` only; table-driven tests; black-box tests use
  `package <pkg>_test`.
- All public operations take `context.Context`; cancellation must reach
  provider streams and child processes.
- CLI exit codes: `0` success, `1` runtime/provider failure, `2`
  usage/config error, `130` interrupted.

## License

TODO: choose and add a LICENSE file before the first public release.
