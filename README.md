# go-lambda-http-wrapper

[![CI](https://github.com/iadams749/go-lambda-http-wrapper/actions/workflows/ci.yml/badge.svg)](https://github.com/iadams749/go-lambda-http-wrapper/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/iadams749/go-lambda-http-wrapper/branch/main/graph/badge.svg)](https://codecov.io/gh/iadams749/go-lambda-http-wrapper)
[![Go Reference](https://pkg.go.dev/badge/github.com/iadams749/go-lambda-http-wrapper.svg)](https://pkg.go.dev/github.com/iadams749/go-lambda-http-wrapper)
[![Go Report Card](https://goreportcard.com/badge/github.com/iadams749/go-lambda-http-wrapper)](https://goreportcard.com/report/github.com/iadams749/go-lambda-http-wrapper)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Run a standard library `net/http` handler on AWS Lambda without rewriting it for
the Lambda event model.

The package wraps any `http.Handler` and translates between AWS Lambda events and
`net/http` in-process: it converts the incoming event into an `*http.Request`,
runs your handler against a buffering `ResponseWriter`, and converts the buffered
result back into a Lambda response. Your handler stays completely unaware that it
is running on Lambda.

> **Supported event source:** API Gateway **HTTP API (payload format v2)** and
> **Lambda Function URLs** (`events.APIGatewayV2HTTPRequest`). Other sources
> (REST v1, ALB) are not handled yet — see [Roadmap](#roadmap).

## Install

```sh
go get github.com/iadams749/go-lambda-http-wrapper
```

## Quick start

```go
package main

import (
	"fmt"
	"net/http"

	lambdahttp "github.com/iadams749/go-lambda-http-wrapper"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "hello world")
	})

	// Start blocks, serving Lambda invocations through the mux.
	lambdahttp.New(mux).Start()
}
```

`New` returns an `*Adapter`. `Start` hands it to `lambda.Start`; if you need the
handler function directly (for custom wiring or testing) use `Proxy`:

```go
adapter := lambdahttp.New(mux)
resp, err := adapter.Proxy(ctx, event) // events.APIGatewayV2HTTPRequest -> events.APIGatewayV2HTTPResponse
```

## Features

- **Request translation** — method, path, raw query string (including
  multi-value params), headers, and cookies (rejoined from the v2 `Cookies`
  field into a `Cookie` header).
- **Body handling** — base64-encoded request bodies are decoded automatically.
- **Response translation** — status (defaulting to `200`), headers, and body.
  Multiple `Set-Cookie` headers are routed to the v2 response `Cookies` field.
- **Binary responses** — bodies are base64-encoded (with `isBase64Encoded` set)
  when the `Content-Type` is non-textual, falling back to a UTF-8 check when no
  type is present.
- **Panic recovery** — a panic in the handler is logged (with its stack trace)
  and becomes a `500` response instead of crashing the invocation. Recovered
  panics and rejected events are reported through `slog.Default()`, or a logger
  of your choice via `WithLogger`.
- **Access to the raw event** — retrieve the original event from the request
  context for data with no HTTP equivalent (authorizer claims, stage variables,
  request context).

### Accessing the original event

```go
func handler(w http.ResponseWriter, r *http.Request) {
	if event, ok := lambdahttp.RequestEvent(r.Context()); ok {
		claims := event.RequestContext.Authorizer.JWT.Claims
		_ = claims
	}
}
```

### Stripping a base path

When the API is served under a custom domain base path the handler should not
see, strip it with `WithBasePath`:

```go
lambdahttp.New(mux, lambdahttp.WithBasePath("/api")).Start()
// a request to /api/users reaches the handler as /users
```

The prefix only matches whole path segments: with base path `/api`, a request
to `/apiv2/users` is passed through unchanged.

## Development

Common tasks are wrapped in the `Makefile`:

| Target       | Description                                    |
| ------------ | ---------------------------------------------- |
| `make test`  | Run unit tests with the race detector + cover  |
| `make cover` | Write a coverage profile and open the report   |
| `make check` | Verify formatting (`fmt`) and run `go vet`     |
| `make tidy`  | Sync `go.mod` / `go.sum`                        |

CI runs the same checks plus [golangci-lint](https://golangci-lint.run) and
[govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) on every
push to `main` and every pull request, and uploads coverage to Codecov.

## Roadmap

- Additional event sources: API Gateway REST v1, ALB target groups.
- Response streaming.
- A local development server that speaks the same translation for `go run`.
- Framework adapter helpers.

## License

[MIT](LICENSE)
