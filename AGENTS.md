# Agent Notes

- This is a Go 1.26.3 project for a read-only, Supabase-authenticated MCP server fronting Intervals.icu.
- Main server entrypoint: `cmd/intervals-mcp`; CLI entrypoint: `cmd/intervals-cli`.
- Reuse existing config, Intervals client, and insights layers instead of duplicating API calls: `internal/config`, `internal/intervals`, `internal/insights`.
- Config loads `.env`; the Intervals API uses Basic Auth username `API_KEY` and password `INTERVALS_ICU_API_KEY`.
- Keep the server and CLI read-only unless explicitly asked otherwise. Never print secrets.
- Prefer small, focused changes that preserve existing package boundaries and tests.
- Use `GOCACHE=/tmp/go-build-cache` if the default Go cache is read-only.
- Use `-buildvcs=false` for builds/tests if VCS metadata causes issues.

Quality gates:

```sh
make fix
make fmt
make vet
make test
golangci-lint run
```

Run `golangci-lint run` only if it is installed.
