package lambdahttp

import (
	"encoding/base64"
	"io"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestResponseWriterToResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		write func(w http.ResponseWriter)
		check func(t *testing.T, resp events.APIGatewayV2HTTPResponse)
	}{
		{
			name:  "default status when WriteHeader not called",
			write: func(w http.ResponseWriter) { io.WriteString(w, "ok") },
			check: func(t *testing.T, resp events.APIGatewayV2HTTPResponse) {
				if resp.StatusCode != http.StatusOK {
					t.Errorf("status = %d, want 200", resp.StatusCode)
				}
				if resp.Body != "ok" {
					t.Errorf("body = %q, want ok", resp.Body)
				}
			},
		},
		{
			name:  "empty body, default status, not encoded",
			write: func(w http.ResponseWriter) {},
			check: func(t *testing.T, resp events.APIGatewayV2HTTPResponse) {
				if resp.StatusCode != http.StatusOK {
					t.Errorf("status = %d, want 200", resp.StatusCode)
				}
				if resp.IsBase64Encoded {
					t.Error("empty body should not be base64 encoded")
				}
			},
		},
		{
			name: "custom status and headers",
			write: func(w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Trace", "abc")
				w.WriteHeader(http.StatusCreated)
				io.WriteString(w, `{"ok":true}`)
			},
			check: func(t *testing.T, resp events.APIGatewayV2HTTPResponse) {
				if resp.StatusCode != http.StatusCreated {
					t.Errorf("status = %d, want 201", resp.StatusCode)
				}
				if resp.Headers["X-Trace"] != "abc" {
					t.Errorf("X-Trace = %q, want abc", resp.Headers["X-Trace"])
				}
				if resp.IsBase64Encoded {
					t.Error("json response should not be base64 encoded")
				}
			},
		},
		{
			name: "set-cookie routed to cookies field",
			write: func(w http.ResponseWriter) {
				http.SetCookie(w, &http.Cookie{Name: "a", Value: "1"})
				http.SetCookie(w, &http.Cookie{Name: "b", Value: "2"})
			},
			check: func(t *testing.T, resp events.APIGatewayV2HTTPResponse) {
				if len(resp.Cookies) != 2 {
					t.Fatalf("Cookies = %v, want 2 entries", resp.Cookies)
				}
				if _, ok := resp.Headers["Set-Cookie"]; ok {
					t.Error("Set-Cookie must not appear in Headers map")
				}
			},
		},
		{
			name: "binary content type is base64 encoded",
			write: func(w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Write([]byte{0xDE, 0xAD, 0xBE, 0xEF})
			},
			check: func(t *testing.T, resp events.APIGatewayV2HTTPResponse) {
				if !resp.IsBase64Encoded {
					t.Fatal("binary response should be base64 encoded")
				}
				decoded, err := base64.StdEncoding.DecodeString(resp.Body)
				if err != nil {
					t.Fatalf("body is not valid base64: %v", err)
				}
				if string(decoded) != string([]byte{0xDE, 0xAD, 0xBE, 0xEF}) {
					t.Errorf("decoded body = %v, want [222 173 190 239]", decoded)
				}
			},
		},
		{
			name:  "unknown content type, valid utf-8 stays text",
			write: func(w http.ResponseWriter) { io.WriteString(w, "plain text") },
			check: func(t *testing.T, resp events.APIGatewayV2HTTPResponse) {
				if resp.IsBase64Encoded {
					t.Error("valid utf-8 with no content-type should stay text")
				}
			},
		},
		{
			name:  "unknown content type, invalid utf-8 is encoded",
			write: func(w http.ResponseWriter) { w.Write([]byte{0xFF, 0xFE, 0xFD}) },
			check: func(t *testing.T, resp events.APIGatewayV2HTTPResponse) {
				if !resp.IsBase64Encoded {
					t.Error("invalid utf-8 with no content-type should be base64 encoded")
				}
			},
		},
		{
			name: "headers set after WriteHeader are ignored",
			write: func(w http.ResponseWriter) {
				w.Header().Set("X-Before", "kept")
				w.WriteHeader(http.StatusAccepted)
				w.Header().Set("X-After", "dropped")
			},
			check: func(t *testing.T, resp events.APIGatewayV2HTTPResponse) {
				if resp.Headers["X-Before"] != "kept" {
					t.Errorf("X-Before = %q, want kept", resp.Headers["X-Before"])
				}
				if got, ok := resp.Headers["X-After"]; ok {
					t.Errorf("X-After = %q, want it absent (set after WriteHeader)", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newResponseWriter()
			tt.write(w)
			tt.check(t, w.toResponse())
		})
	}
}

func TestIsTextContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		contentType string
		want        bool
	}{
		{"text/plain", true},
		{"text/html; charset=utf-8", true},
		{"application/json", true},
		{"application/vnd.api+json", true},
		{"application/xml", true},
		{"application/x-www-form-urlencoded", true},
		{"application/octet-stream", false},
		{"image/png", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			t.Parallel()
			if got := isTextContentType(tt.contentType); got != tt.want {
				t.Errorf("isTextContentType(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestShouldBase64Encode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		body        []byte
		want        bool
	}{
		{"empty body never encoded", "application/octet-stream", nil, false},
		{"text content type", "text/plain", []byte("hi"), false},
		{"binary content type", "image/png", []byte("hi"), true},
		{"no content type, valid utf-8", "", []byte("héllo"), false},
		{"no content type, invalid utf-8", "", []byte{0xFF}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldBase64Encode(tt.contentType, tt.body); got != tt.want {
				t.Errorf("shouldBase64Encode(%q, %v) = %v, want %v", tt.contentType, tt.body, got, tt.want)
			}
		})
	}
}
