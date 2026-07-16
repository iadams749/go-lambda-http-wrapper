package lambdahttp

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// Adapter wraps an http.Handler so it can serve AWS Lambda requests originating
// from API Gateway HTTP APIs (payload format v2) or Lambda Function URLs.
type Adapter struct {
	handler  http.Handler
	basePath string
	logger   *slog.Logger
}

// Option configures an Adapter.
type Option func(*Adapter)

// WithBasePath strips the given prefix from the request path before the request
// reaches the handler. This is useful when the API is served under a custom
// domain base path (for example "/api") that the handler should not see. The
// prefix only matches on segment boundaries: "/api" strips "/api/users" but
// leaves "/apiv2/users" untouched. A trailing slash on basePath is ignored.
func WithBasePath(basePath string) Option {
	return func(a *Adapter) { a.basePath = strings.TrimRight(basePath, "/") }
}

// WithLogger sets the logger the adapter uses to report recovered panics and
// rejected events. When not set, slog.Default() is used.
func WithLogger(logger *slog.Logger) Option {
	return func(a *Adapter) { a.logger = logger }
}

// New wraps an http.Handler in an Adapter.
func New(handler http.Handler, opts ...Option) *Adapter {
	a := &Adapter{handler: handler}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Proxy translates a Lambda event into an *http.Request, runs the wrapped
// handler, and translates the buffered result back into a Lambda response. Its
// signature satisfies lambda.Start.
//
// A panic in the handler is recovered, logged with its stack trace via slog,
// and reported to the caller as a 500 response rather than crashing the Lambda
// invocation. An event whose body cannot be decoded is logged and answered
// with a 400 without invoking the handler; in both cases the returned error is
// nil so the invocation itself still succeeds.
func (a *Adapter) Proxy(ctx context.Context, event events.APIGatewayV2HTTPRequest) (resp events.APIGatewayV2HTTPResponse, err error) {
	logger := a.logger
	if logger == nil {
		logger = slog.Default()
	}

	defer func() {
		if r := recover(); r != nil {
			// Like net/http, a panic with ErrAbortHandler aborts the response
			// without the stack-trace log.
			if r != http.ErrAbortHandler {
				logger.ErrorContext(ctx, "lambdahttp: recovered panic from handler",
					"panic", r,
					"stack", string(debug.Stack()))
			}
			resp = events.APIGatewayV2HTTPResponse{StatusCode: http.StatusInternalServerError}
			err = nil
		}
	}()

	req, err := a.newHTTPRequest(ctx, event)
	if err != nil {
		logger.WarnContext(ctx, "lambdahttp: rejecting malformed request", "error", err)
		return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusBadRequest}, nil
	}

	rw := newResponseWriter()
	a.handler.ServeHTTP(rw, req)

	return rw.toResponse(), nil
}

// Start hands the adapter to lambda.Start, using Proxy as the invocation
// handler. It blocks until the Lambda runtime shuts the process down.
func (a *Adapter) Start() {
	lambda.Start(a.Proxy)
}
