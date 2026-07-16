package lambdahttp

import (
	"context"

	"github.com/aws/aws-lambda-go/events"
)

// contextKey is an unexported type so that keys placed on a request context by
// this package cannot collide with keys set by other packages.
type contextKey struct{ name string }

var eventContextKey = &contextKey{"request-event"}

// withRequestEvent stores the original Lambda event on the context.
func withRequestEvent(ctx context.Context, event events.APIGatewayV2HTTPRequest) context.Context {
	return context.WithValue(ctx, eventContextKey, event)
}

// RequestEvent returns the original API Gateway v2 event associated with the
// request context, if present. Handlers can use it to reach data that has no
// http.Request equivalent, such as authorizer claims, stage variables, or the
// full request context.
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//		if event, ok := lambdahttp.RequestEvent(r.Context()); ok {
//			// event.RequestContext.Authorizer.JWT.Claims, etc.
//		}
//	}
func RequestEvent(ctx context.Context) (events.APIGatewayV2HTTPRequest, bool) {
	event, ok := ctx.Value(eventContextKey).(events.APIGatewayV2HTTPRequest)
	return event, ok
}
