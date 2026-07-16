// Package lambdahttp adapts a standard library http.Handler so it can serve
// AWS Lambda requests originating from API Gateway HTTP APIs (payload format
// v2) or Lambda Function URLs.
//
// The adapter is a bidirectional translator: it converts the incoming Lambda
// event into an *http.Request, runs your handler against a buffering
// ResponseWriter, and converts the buffered result back into a Lambda
// response. Your handler code stays completely unaware that it is running on
// Lambda.
//
//	func main() {
//		mux := http.NewServeMux()
//		mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
//			fmt.Fprintln(w, "hello world")
//		})
//		lambdahttp.New(mux).Start()
//	}
package lambdahttp
