package lambdahttp

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-lambda-go/events"
)

// responseWriter is an http.ResponseWriter that buffers the handler's output so
// it can be converted into a Lambda response after the handler returns.
type responseWriter struct {
	header      http.Header
	snapshot    http.Header // header as of WriteHeader; nil until then
	body        bytes.Buffer
	status      int
	wroteHeader bool
}

func newResponseWriter() *responseWriter {
	return &responseWriter{header: make(http.Header)}
}

func (w *responseWriter) Header() http.Header { return w.header }

func (w *responseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	// Like net/http, header changes made after WriteHeader have no effect, so
	// freeze the map here.
	w.snapshot = w.header.Clone()
	w.wroteHeader = true
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(b)
}

// toResponse converts the buffered response into an API Gateway v2 response.
func (w *responseWriter) toResponse() events.APIGatewayV2HTTPResponse {
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}

	header := w.header
	if w.wroteHeader {
		header = w.snapshot
	}

	resp := events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    make(map[string]string),
	}

	for key, values := range header {
		// Multiple Set-Cookie headers must go in the dedicated Cookies field;
		// v2 has no multi-value header map.
		if http.CanonicalHeaderKey(key) == "Set-Cookie" {
			resp.Cookies = append(resp.Cookies, values...)
			continue
		}
		resp.Headers[key] = strings.Join(values, ",")
	}

	body := w.body.Bytes()
	if shouldBase64Encode(header.Get("Content-Type"), body) {
		resp.Body = base64.StdEncoding.EncodeToString(body)
		resp.IsBase64Encoded = true
	} else {
		resp.Body = string(body)
	}

	return resp
}

// shouldBase64Encode reports whether a response body must be base64-encoded to
// survive the trip through Lambda. Text payloads are sent as-is; anything else
// is encoded. When the Content-Type is unknown we fall back to a UTF-8 check.
func shouldBase64Encode(contentType string, body []byte) bool {
	if len(body) == 0 {
		return false
	}
	if contentType != "" {
		return !isTextContentType(contentType)
	}
	return !utf8.Valid(body)
}

// isTextContentType reports whether a Content-Type represents textual data that
// can be transmitted without base64 encoding.
func isTextContentType(contentType string) bool {
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		contentType = contentType[:i] // drop parameters like "; charset=utf-8"
	}
	contentType = strings.TrimSpace(strings.ToLower(contentType))

	switch {
	case strings.HasPrefix(contentType, "text/"):
		return true
	case strings.HasSuffix(contentType, "+json"), strings.HasSuffix(contentType, "+xml"):
		return true
	}

	switch contentType {
	case "application/json",
		"application/xml",
		"application/javascript",
		"application/x-www-form-urlencoded":
		return true
	}
	return false
}
