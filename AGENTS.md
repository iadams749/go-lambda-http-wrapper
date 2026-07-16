# AGENTS.md

Guidance for coding agents (Claude Code and others) working in this repository.

> **Note:** `CLAUDE.md` is a symlink to this file (`AGENTS.md`). Edit `AGENTS.md`
> directly; never replace the symlink with a copy.

## What this is

A single-package Go library (`lambdahttp`, module
`github.com/iadams749/go-lambda-http-wrapper`) that adapts a standard library
`http.Handler` so it can serve AWS Lambda invocations. Only the **API Gateway
HTTP API payload format v2 / Lambda Function URLs** event source
(`events.APIGatewayV2HTTPRequest`) is supported today.

## Commands

- `make test` — unit tests with race detector + coverage
- `make check` — gofmt verification (`fmt`) and `go vet`
- `make cover` — write `coverage.out` and open the HTML report
- `make tidy` — `go mod tidy`

Run a single test:

```sh
go test -run TestNewHTTPRequest/multi-value_query_string ./
```

Subtest names come from the table's `name` field; spaces become underscores in
`-run` patterns.

CI (`.github/workflows/`) runs `make check`, `make test`, golangci-lint
(config in `.golangci.yml`), and govulncheck on pushes to `main` and PRs.

## Architecture

The library is a bidirectional translator wrapped around the user's handler:

```
Lambda v2 event ──► *http.Request ──► [ http.Handler ] ──► responseWriter ──► v2 response
```

The round trip lives in `Adapter.Proxy` (`adapter.go`) and spans four files:

- **`request.go`** — `newHTTPRequest` builds the `*http.Request`. Key v2-specific
  decisions: duplicate headers arrive comma-joined and are **not** split (commas
  are valid in many header values); request cookies come from the dedicated
  `Cookies []string` field and are rejoined into a `Cookie` header; the base64
  body is decoded here, and a decode failure is what makes `Proxy` return 400.
- **`response.go`** — `responseWriter` buffers status/headers/body, then
  `toResponse` converts it. Two v2 quirks handled here: multiple `Set-Cookie`
  headers are routed to the response `Cookies` field (v2 has no multi-value
  header map), and body base64-encoding is decided by `shouldBase64Encode`
  (text Content-Type → plain; otherwise encode; empty Content-Type → UTF-8
  validity check).
- **`context.go`** — stashes the raw event on the request context so handlers can
  reach it via `RequestEvent(ctx)` (for authorizer claims, stage variables, etc.).
- **`adapter.go`** — public surface (`New`, `Proxy`, `Start`, `WithBasePath`,
  `WithLogger`) plus panic recovery: a handler panic is logged via slog and
  becomes a 500 rather than failing the invocation.

When adding a new event source, the pattern to preserve is: one translator file
per direction, with the source-specific field quirks documented inline as above.

## Conventions

- **Tests are table-driven wherever possible** — `tests := []struct{ name ... }`
  iterated with `for _, tt := range tests { t.Run(tt.name, ...) }`. Use per-case
  `setup`/`check`/`write` func fields when assertions vary. Reserve standalone
  test functions for cases that genuinely don't tabulate.
- Follow idiomatic Go throughout (error wrapping with `%w` and a `lambdahttp:`
  prefix, doc comments on exported identifiers).

## Use skills liberally

Prefer invoking a matching skill over doing the work ad hoc — overusing skills is
better than ignoring them. If a skill's trigger plausibly matches the task (e.g.
`verify` before committing a nontrivial change, `code-review` on a diff), invoke
it. When in doubt, invoke.

## Note: the `agent/` scaffolding tree

Skill-runtime scaffolding (`agent/`, `.agents/`, `skills-lock.json`) may be
present in the repo root. It is **not** part of this library. The
`ignore ./agent` directive in `go.mod` keeps Go tooling (`go build ./...`,
`go test ./...`, `go mod tidy`) away from its example `.go` files, and
`.gitignore` keeps it out of commits. The one tool that doesn't read that
directive is `gofmt`, so the Makefile's `fmt` target excludes the tree by path.
