package httpdump_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/hummerd/httpdump"
)

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

func setupDefaultLogger() {
	logh := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// to male log output stable, replace time and duration attributes
			switch a.Key {
			case slog.TimeKey:
				return slog.Attr{Key: a.Key, Value: slog.StringValue("now")}
			case "duration":
				return slog.Attr{Key: a.Key, Value: slog.StringValue("1s")}
			default:
				return a
			}
		},
	})

	slog.SetDefault(slog.New(logh))
}

func setupHTTPServer(m *httpdump.Middleware) *httptest.Server {
	mux := http.NewServeMux()
	mux.Handle("POST /call_me", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("this is response body"))
	}))

	h := m.Wrap(mux)

	return httptest.NewServer(h)
}
