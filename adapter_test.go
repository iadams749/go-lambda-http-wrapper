package lambdahttp

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

// discardLogger keeps expected panics and rejections out of the test output.
var discardLogger = slog.New(slog.DiscardHandler)

func TestProxy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		event      func() events.APIGatewayV2HTTPRequest
		handler    http.HandlerFunc
		wantStatus int
		wantCalled bool
	}{
		{
			name:       "handler runs and status is returned",
			event:      func() events.APIGatewayV2HTTPRequest { return newEvent(http.MethodGet, "/") },
			handler:    func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) },
			wantStatus: http.StatusTeapot,
			wantCalled: true,
		},
		{
			name: "malformed base64 body yields 400 without running handler",
			event: func() events.APIGatewayV2HTTPRequest {
				e := newEvent(http.MethodPost, "/")
				e.Body = "not!valid!base64!"
				e.IsBase64Encoded = true
				return e
			},
			handler:    func(http.ResponseWriter, *http.Request) {},
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
		{
			name:       "panic in handler is recovered as 500",
			event:      func() events.APIGatewayV2HTTPRequest { return newEvent(http.MethodGet, "/") },
			handler:    func(http.ResponseWriter, *http.Request) { panic("boom") },
			wantStatus: http.StatusInternalServerError,
			wantCalled: true,
		},
		{
			name:       "panic with ErrAbortHandler is recovered as 500",
			event:      func() events.APIGatewayV2HTTPRequest { return newEvent(http.MethodGet, "/") },
			handler:    func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) },
			wantStatus: http.StatusInternalServerError,
			wantCalled: true,
		},
		{
			name:       "invalid method yields 400 without running handler",
			event:      func() events.APIGatewayV2HTTPRequest { return newEvent("BAD METHOD", "/") },
			handler:    func(http.ResponseWriter, *http.Request) {},
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			called := false
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				tt.handler(w, r)
			})

			resp, err := New(h, WithLogger(discardLogger)).Proxy(t.Context(), tt.event())
			if err != nil {
				t.Fatalf("Proxy returned error: %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if called != tt.wantCalled {
				t.Errorf("handler called = %v, want %v", called, tt.wantCalled)
			}
		})
	}
}

// TestProxyEndToEnd exercises a realistic handler wired through an http.ServeMux
// to confirm routing, request body, and response body all survive the round trip.
func TestProxyEndToEnd(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /echo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		io.Copy(w, r.Body)
	})

	e := newEvent(http.MethodPost, "/echo")
	e.Body = "round trip"

	resp, err := New(mux).Proxy(t.Context(), e)
	if err != nil {
		t.Fatalf("Proxy returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if resp.Body != "round trip" {
		t.Errorf("body = %q, want round trip", resp.Body)
	}
}
