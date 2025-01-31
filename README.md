# httpdump

HTTP dump middleware allows you to record and analyse HTTP server requests and responses. Most common use for this middleware is to log request and responses for debugging purposes. Also can be used for tracing or collecting metrics.

## Example

``` go
func Example() {
	setupDefaultLogger()

	dumpReq := func(r *http.Request, body []byte) {
		slog.Debug("HTTP request",
			"method", r.Method,
			"url", r.URL,
			"headers", r.Header,
			"body", string(body))
	}

	dumpResp := func(r *http.Response, body []byte, duration time.Duration) {
		slog.Debug("HTTP response",
			"method", r.Request.Method,
			"url", r.Request.URL,
			"status", r.StatusCode,
			"headers", r.Header,
			"body", string(body),
			"duration", duration)
	}

	m := httpdump.NewMiddleware(dumpReq, dumpResp)

	s := setupHTTPServer(m)

	c := s.Client()
	c.Post(s.URL+"/call_me", "text/plain", strings.NewReader("this is request body"))

	c.Get(s.URL + "/call_me")

	s.Close()

	// Output: time=now level=DEBUG msg="HTTP request" method=POST url=/call_me headers="map[Accept-Encoding:[gzip] Content-Length:[20] Content-Type:[text/plain] User-Agent:[Go-http-client/1.1]]" body="this is request body"
	// time=now level=DEBUG msg="HTTP response" method=POST url=/call_me status=200 headers=map[Content-Type:[text/plain]] body="this is response body" duration=1s
	// time=now level=DEBUG msg="HTTP request" method=GET url=/call_me headers="map[Accept-Encoding:[gzip] User-Agent:[Go-http-client/1.1]]" body=""
	// time=now level=DEBUG msg="HTTP response" method=GET url=/call_me status=405 headers="map[Allow:[POST] Content-Type:[text/plain; charset=utf-8] X-Content-Type-Options:[nosniff]]" body="Method Not Allowed\n" duration=1s
}
```
