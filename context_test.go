package lambdahttp

import (
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestRequestEvent(t *testing.T) {
	t.Parallel()

	t.Run("present on request context from adapter", func(t *testing.T) {
		t.Parallel()
		e := newEvent(http.MethodGet, "/widgets")
		e.RouteKey = "GET /widgets"

		var (
			got events.APIGatewayV2HTTPRequest
			ok  bool
		)
		h := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			got, ok = RequestEvent(r.Context())
		})
		if _, err := New(h).Proxy(t.Context(), e); err != nil {
			t.Fatalf("Proxy returned error: %v", err)
		}

		if !ok {
			t.Fatal("RequestEvent returned ok = false, want true")
		}
		if got.RouteKey != "GET /widgets" {
			t.Errorf("RouteKey = %q, want GET /widgets", got.RouteKey)
		}
	})

	t.Run("absent on a bare context", func(t *testing.T) {
		t.Parallel()
		if _, ok := RequestEvent(t.Context()); ok {
			t.Error("RequestEvent returned ok = true for a bare context, want false")
		}
	})
}
