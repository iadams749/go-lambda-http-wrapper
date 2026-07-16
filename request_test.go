package lambdahttp

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"slices"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

// newEvent builds a minimal valid API Gateway v2 event for tests.
func newEvent(method, path string) events.APIGatewayV2HTTPRequest {
	e := events.APIGatewayV2HTTPRequest{RawPath: path}
	e.RequestContext.HTTP.Method = method
	e.RequestContext.HTTP.Path = path
	e.RequestContext.HTTP.Protocol = "HTTP/1.1"
	e.RequestContext.HTTP.SourceIP = "192.0.2.1"
	e.RequestContext.DomainName = "example.com"
	return e
}

// captureRequest runs the adapter against event and returns the *http.Request
// the handler observed.
func captureRequest(t *testing.T, event events.APIGatewayV2HTTPRequest, opts ...Option) *http.Request {
	t.Helper()
	var got *http.Request
	h := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r })
	if _, err := New(h, opts...).Proxy(t.Context(), event); err != nil {
		t.Fatalf("Proxy returned error: %v", err)
	}
	if got == nil {
		t.Fatal("handler was never called")
	}
	return got
}

func TestNewHTTPRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		path   string
		setup  func(e *events.APIGatewayV2HTTPRequest)
		opts   []Option
		check  func(t *testing.T, r *http.Request)
	}{
		{
			name:   "method and path",
			method: http.MethodPost,
			path:   "/things/42",
			check: func(t *testing.T, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method = %q, want POST", r.Method)
				}
				if r.URL.Path != "/things/42" {
					t.Errorf("path = %q, want /things/42", r.URL.Path)
				}
			},
		},
		{
			name:   "multi-value query string",
			method: http.MethodGet,
			path:   "/search",
			setup:  func(e *events.APIGatewayV2HTTPRequest) { e.RawQueryString = "q=go&tag=a&tag=b" },
			check: func(t *testing.T, r *http.Request) {
				if got := r.URL.Query()["tag"]; !slices.Equal(got, []string{"a", "b"}) {
					t.Errorf("tag = %v, want [a b]", got)
				}
				if got := r.URL.Query().Get("q"); got != "go" {
					t.Errorf("q = %q, want go", got)
				}
				if r.RequestURI != "/search?q=go&tag=a&tag=b" {
					t.Errorf("RequestURI = %q", r.RequestURI)
				}
			},
		},
		{
			name:   "headers and explicit host",
			method: http.MethodGet,
			path:   "/",
			setup: func(e *events.APIGatewayV2HTTPRequest) {
				e.Headers = map[string]string{
					"Host":            "api.example.com",
					"X-Custom":        "value",
					"Accept-Encoding": "gzip, deflate", // comma must be preserved
				}
			},
			check: func(t *testing.T, r *http.Request) {
				if r.Host != "api.example.com" {
					t.Errorf("Host = %q, want api.example.com", r.Host)
				}
				if got := r.Header.Get("Host"); got != "" {
					t.Errorf("Host header = %q, want it removed from the header map", got)
				}
				if got := r.Header.Get("X-Custom"); got != "value" {
					t.Errorf("X-Custom = %q, want value", got)
				}
				if got := r.Header.Get("Accept-Encoding"); got != "gzip, deflate" {
					t.Errorf("Accept-Encoding = %q, want %q", got, "gzip, deflate")
				}
			},
		},
		{
			name:   "host falls back to domain name",
			method: http.MethodGet,
			path:   "/",
			check: func(t *testing.T, r *http.Request) {
				if r.Host != "example.com" {
					t.Errorf("Host = %q, want example.com (from DomainName)", r.Host)
				}
			},
		},
		{
			name:   "cookies rejoined into header",
			method: http.MethodGet,
			path:   "/",
			setup:  func(e *events.APIGatewayV2HTTPRequest) { e.Cookies = []string{"session=abc", "theme=dark"} },
			check: func(t *testing.T, r *http.Request) {
				if got := r.Header.Get("Cookie"); got != "session=abc; theme=dark" {
					t.Errorf("Cookie = %q", got)
				}
				if c, err := r.Cookie("session"); err != nil || c.Value != "abc" {
					t.Errorf("session cookie = %v, err = %v", c, err)
				}
			},
		},
		{
			name:   "remote addr and protocol",
			method: http.MethodGet,
			path:   "/",
			check: func(t *testing.T, r *http.Request) {
				if r.RemoteAddr != "192.0.2.1" {
					t.Errorf("RemoteAddr = %q, want 192.0.2.1", r.RemoteAddr)
				}
				if r.ProtoMajor != 1 || r.ProtoMinor != 1 {
					t.Errorf("Proto = %d.%d, want 1.1", r.ProtoMajor, r.ProtoMinor)
				}
			},
		},
		{
			name:   "plain text body",
			method: http.MethodPost,
			path:   "/",
			setup:  func(e *events.APIGatewayV2HTTPRequest) { e.Body = "hello body" },
			check: func(t *testing.T, r *http.Request) {
				if r.ContentLength != int64(len("hello body")) {
					t.Errorf("ContentLength = %d, want %d", r.ContentLength, len("hello body"))
				}
				if got, _ := io.ReadAll(r.Body); string(got) != "hello body" {
					t.Errorf("body = %q, want hello body", got)
				}
			},
		},
		{
			name:   "base64 encoded body",
			method: http.MethodPost,
			path:   "/",
			setup: func(e *events.APIGatewayV2HTTPRequest) {
				e.Body = base64.StdEncoding.EncodeToString([]byte{0x00, 0x01, 0x02})
				e.IsBase64Encoded = true
			},
			check: func(t *testing.T, r *http.Request) {
				got, _ := io.ReadAll(r.Body)
				if !bytes.Equal(got, []byte{0x00, 0x01, 0x02}) {
					t.Errorf("decoded body = %v, want [0 1 2]", got)
				}
			},
		},
		{
			name:   "base path stripped",
			method: http.MethodGet,
			path:   "/api/users",
			opts:   []Option{WithBasePath("/api")},
			check: func(t *testing.T, r *http.Request) {
				if r.URL.Path != "/users" {
					t.Errorf("path = %q, want /users", r.URL.Path)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newEvent(tt.method, tt.path)
			if tt.setup != nil {
				tt.setup(&e)
			}
			tt.check(t, captureRequest(t, e, tt.opts...))
		})
	}
}

func TestStripBasePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		basePath string
		path     string
		want     string
	}{
		{"no base path configured", "", "/api/users", "/api/users"},
		{"prefix stripped", "/api", "/api/users", "/users"},
		{"prefix absent leaves path", "/api", "/users", "/users"},
		{"exact match rooted at slash", "/api", "/api", "/"},
		{"mid-segment match leaves path", "/api", "/apiv2/users", "/apiv2/users"},
		{"trailing slash on base path ignored", "/api/", "/api/users", "/users"},
		{"root base path is a no-op", "/", "/users", "/users"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := New(nil, WithBasePath(tt.basePath))
			if got := a.stripBasePath(tt.path); got != tt.want {
				t.Errorf("stripBasePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
