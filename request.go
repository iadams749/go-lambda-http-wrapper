package lambdahttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

// newHTTPRequest converts an API Gateway v2 / Function URL event into an
// *http.Request suitable for passing to an http.Handler.
func (a *Adapter) newHTTPRequest(ctx context.Context, event events.APIGatewayV2HTTPRequest) (*http.Request, error) {
	body, err := decodeBody(event)
	if err != nil {
		return nil, err
	}

	path := a.stripBasePath(event.RawPath)
	target := path
	if event.RawQueryString != "" {
		target += "?" + event.RawQueryString
	}

	req, err := http.NewRequestWithContext(ctx, event.RequestContext.HTTP.Method, target, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("lambdahttp: building request: %w", err)
	}

	// v2 collapses duplicate headers into a single comma-joined value, so each
	// key maps to exactly one string. We intentionally do not split on commas:
	// several header values (dates, user agents) legitimately contain commas.
	for key, value := range event.Headers {
		req.Header.Set(key, value)
	}

	// Request cookies arrive in a dedicated field in v2, not in the header map.
	if len(event.Cookies) > 0 {
		req.Header.Set("Cookie", strings.Join(event.Cookies, "; "))
	}

	// http.NewRequestWithContext is client-oriented and leaves several
	// server-side fields unset; populate the ones handlers commonly read.
	req.RequestURI = target
	req.RemoteAddr = event.RequestContext.HTTP.SourceIP
	// Like net/http's server, promote the Host header to req.Host and remove it
	// from the header map.
	if host := req.Header.Get("Host"); host != "" {
		req.Host = host
		req.Header.Del("Host")
	} else {
		req.Host = event.RequestContext.DomainName
	}
	if proto := event.RequestContext.HTTP.Protocol; proto != "" {
		if major, minor, ok := http.ParseHTTPVersion(proto); ok {
			req.Proto, req.ProtoMajor, req.ProtoMinor = proto, major, minor
		}
	}
	req.ContentLength = int64(len(body))

	// Make the raw event retrievable from within the handler.
	return req.WithContext(withRequestEvent(req.Context(), event)), nil
}

// decodeBody returns the request body, base64-decoding it when the event marks
// it as binary.
func decodeBody(event events.APIGatewayV2HTTPRequest) ([]byte, error) {
	if event.Body == "" {
		return nil, nil
	}
	if !event.IsBase64Encoded {
		return []byte(event.Body), nil
	}
	decoded, err := base64.StdEncoding.DecodeString(event.Body)
	if err != nil {
		return nil, fmt.Errorf("lambdahttp: decoding base64 body: %w", err)
	}
	return decoded, nil
}

// stripBasePath removes the configured base path prefix from a request path,
// keeping the result rooted at "/". The prefix must end on a segment boundary:
// "/api" matches "/api" and "/api/users" but not "/apiv2/users".
func (a *Adapter) stripBasePath(path string) string {
	if a.basePath == "" {
		return path
	}
	trimmed := strings.TrimPrefix(path, a.basePath)
	switch {
	case trimmed == path:
		return path // prefix not present; leave untouched
	case trimmed == "":
		return "/"
	case strings.HasPrefix(trimmed, "/"):
		return trimmed
	default:
		return path // prefix matched mid-segment (e.g. /apiv2); leave untouched
	}
}
