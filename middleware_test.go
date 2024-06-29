package httpdump_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hummerd/httpdump"
)

func TestMiddleware_FilterByPath(t *testing.T) {
	req, err := http.NewRequest(
		http.MethodPost,
		"http://example.com/somepath",
		http.NoBody)
	noerr(t, err)

	_, dump := dumpRequest(
		t,
		true,
		req,
		true,
		http.StatusOK,
		nil,
		nil,
		[]httpdump.Option{
			httpdump.WithExcludeByPathFilter(regexp.MustCompile("somepath")),
		})

	expectedResultDumped := &httpDumpResult{
		reqDumped:  false,
		respDumped: false,
	}

	compareDumpResult(t, dump, expectedResultDumped)
}

func TestMiddleware_EnabledMiddleware(t *testing.T) {
	reqBody := `{ "some": "json" }`

	reqBodyReader := strings.NewReader(reqBody)

	req, err := http.NewRequest(
		http.MethodPost,
		"http://example.com/somepath",
		reqBodyReader)
	noerr(t, err)

	req.Header.Set("Content-Type", httpdump.MimeApplicationJSON)

	respBody := "Welcome!"
	respHeaders := headers("Content-Type", "text/plain")

	m, dump := dumpRequest(
		t,
		true,
		req,
		true,
		http.StatusOK,
		[]byte(respBody),
		respHeaders,
		nil)

	e := m.Enabled()
	if !e {
		t.Errorf("Expected middleware to be enabled")
	}

	expectedResultDumped := &httpDumpResult{
		gotBody:    []byte(reqBody),
		reqDumped:  true,
		req:        req,
		reqBody:    []byte(reqBody),
		respDumped: true,
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     respHeaders,
		},
		respBody: []byte(respBody),
	}

	compareDumpResult(t, dump, expectedResultDumped)

	m.SetEnabled(false)

	// reset dump result and make sure it winn not be dumped again
	dump.reqDumped = false
	dump.respDumped = false

	reqBodyReader.Seek(0, io.SeekStart)

	_ = processWrappedRequest(
		t,
		m,
		req,
		http.StatusOK,
		[]byte(respBody),
		respHeaders)

	expectedResultEmpty := &httpDumpResult{
		gotBody:    []byte(reqBody),
		reqDumped:  false,
		respDumped: false,
	}

	compareDumpResult(t, dump, expectedResultEmpty)

	// now enable middleware again and check that it will dump request and response
	m.SetEnabled(true)

	reqBodyReader.Seek(0, io.SeekStart)

	_ = processWrappedRequest(
		t,
		m,
		req,
		http.StatusOK,
		[]byte(respBody),
		respHeaders)

	compareDumpResult(t, dump, expectedResultDumped)
}

func TestMiddleware_HandlePostRequestRequestDump(t *testing.T) {
	reqBody := `{ "some": "json" }`

	req, err := http.NewRequest(
		http.MethodPost,
		"http://example.com/somepath",
		strings.NewReader(reqBody))
	noerr(t, err)

	req.Header.Set("Content-Type", httpdump.MimeApplicationJSON)

	respBody := "Welcome!"
	respHeaders := headers("Content-Type", "text/plain")

	_, dump := dumpRequest(
		t,
		true,
		req,
		false,
		http.StatusOK,
		[]byte(respBody),
		respHeaders,
		nil)

	expectedResult := &httpDumpResult{
		gotBody:    []byte(reqBody),
		reqDumped:  true,
		req:        req,
		reqBody:    []byte(reqBody),
		respDumped: false,
	}

	compareDumpResult(t, dump, expectedResult)
}

func TestMiddleware_HandlePostRequestResponseDump(t *testing.T) {
	reqBody := `{ "some": "json" }`

	req, err := http.NewRequest(
		http.MethodPost,
		"http://example.com/somepath",
		strings.NewReader(reqBody))
	noerr(t, err)

	req.Header.Set("Content-Type", httpdump.MimeApplicationJSON)

	respBody := "Welcome!"
	respHeaders := headers("Content-Type", "text/plain")

	_, dump := dumpRequest(
		t,
		false,
		req,
		true,
		http.StatusOK,
		[]byte(respBody),
		respHeaders,
		nil)

	expectedResult := &httpDumpResult{
		gotBody:    []byte(reqBody),
		reqDumped:  false,
		respDumped: true,
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     respHeaders,
		},
		respBody: []byte(respBody),
	}

	compareDumpResult(t, dump, expectedResult)
}

func TestMiddleware_Handle_GetRequestFullDump(t *testing.T) {
	req, err := http.NewRequest(
		http.MethodGet,
		"http://example.com/somepath",
		http.NoBody)
	noerr(t, err)

	respBody := "Welcome!"
	respHeaders := headers("Content-Type", "text/plain")

	_, dump := dumpRequest(
		t,
		true,
		req,
		true,
		0,
		[]byte(respBody),
		respHeaders,
		nil)

	expectedResult := &httpDumpResult{
		gotBody:    nil,
		reqDumped:  true,
		req:        req,
		reqBody:    nil,
		respDumped: true,
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     respHeaders,
		},
		respBody: []byte(respBody),
	}

	compareDumpResult(t, dump, expectedResult)
}

func TestMiddleware_Handle_PostJSONFullDump(t *testing.T) {
	reqBody := `{ "some": "json" }`

	req, err := http.NewRequest(
		http.MethodPost,
		"http://example.com/somepath",
		strings.NewReader(reqBody))
	noerr(t, err)

	req.Header.Set("Content-Type", httpdump.MimeApplicationJSON)

	respBody := "Welcome!"
	respHeaders := headers("Content-Type", "text/plain")

	_, dump := dumpRequest(
		t,
		true,
		req,
		true,
		http.StatusOK,
		[]byte(respBody),
		respHeaders,
		nil)

	expectedResult := &httpDumpResult{
		gotBody:    []byte(reqBody),
		reqDumped:  true,
		req:        req,
		reqBody:    []byte(reqBody),
		respDumped: true,
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     respHeaders,
		},
		respBody: []byte(respBody),
	}

	compareDumpResult(t, dump, expectedResult)
}

func compareDumpResult(t *testing.T, actual, expected *httpDumpResult) {
	if !bytes.Equal(expected.gotBody, actual.gotBody) {
		t.Errorf("Expected request body %v, got %v", expected.gotBody, actual.gotBody)
	}

	if actual.reqDumped != expected.reqDumped {
		t.Errorf("Expected request dump %v, got %v", expected.reqDumped, actual.reqDumped)
	}

	if expected.reqDumped {
		if !bytes.Equal(expected.reqBody, actual.reqBody) {
			t.Errorf("Expected request body %v, got %v", expected.reqBody, actual.reqBody)
		}

		if expected.req.Method != actual.req.Method {
			t.Errorf("Expected request method %v, got %v", expected.req.Method, actual.req.Method)
		}

		if expected.req.URL.String() != actual.req.URL.String() {
			t.Errorf("Expected request URL %v, got %v", expected.req.URL, actual.req.URL)
		}

		if !reflect.DeepEqual(expected.req.Header, actual.req.Header) {
			t.Errorf("Expected request headers %v, got %v", expected.req.Header, actual.req.Header)
		}
	}

	if actual.respDumped != expected.respDumped {
		t.Errorf("Expected response dump %v, got %v", expected.respDumped, actual.respDumped)
	}

	if expected.respDumped {
		if !bytes.Equal(expected.respBody, actual.respBody) {
			t.Errorf("Expected response body %v, got %v", expected.respBody, actual.respBody)
		}

		if actual.respDuration == 0 {
			t.Errorf("Expected response duration > 0, got %v", actual.respDuration)
		}

		if expected.resp.StatusCode != actual.resp.StatusCode {
			t.Errorf("Expected response status code %v, got %v", expected.resp.StatusCode, actual.resp.StatusCode)
		}

		if !reflect.DeepEqual(expected.resp.Header, actual.resp.Header) {
			t.Errorf("Expected response headers %v, got %v", expected.resp.Header, actual.resp.Header)
		}

		if expected.resp.StatusCode != actual.resp.StatusCode {
			t.Errorf("Expected response status code %v, got %v", expected.resp.StatusCode, actual.resp.StatusCode)
		}
	}
}

func headers(kv ...string) http.Header {
	h := http.Header{}
	for i := 0; i < len(kv); i += 2 {
		h.Add(kv[i], kv[i+1])
	}
	return h
}

type httpDumpResult struct {
	gotBody      []byte
	reqDumped    bool
	req          *http.Request
	reqBody      []byte
	respDumped   bool
	resp         *http.Response
	respBody     []byte
	respDuration time.Duration
}

func newMiddleware(
	dumpRequest bool,
	dumpResponse bool,
	opts []httpdump.Option,
) (*httpdump.Middleware, *httpDumpResult) {
	result := &httpDumpResult{}

	dreq := func(rq *http.Request, body []byte) {
		result.reqDumped = true
		result.req = rq
		result.reqBody = body
	}

	dresp := func(rp *http.Response, body []byte, duration time.Duration) {
		result.respDumped = true
		result.resp = rp
		result.respBody = body
		result.respDuration = duration
	}

	m := httpdump.NewMiddleware(
		iif(dumpRequest, dreq, nil),
		iif(dumpResponse, dresp, nil),
		opts...,
	)

	return m, result
}

func processWrappedRequest(
	t *testing.T,
	m *httpdump.Middleware,
	req *http.Request,
	respStatus int,
	respBody []byte,
	respHeaders http.Header,
) []byte {
	var gotBody []byte

	wrappedHandler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gb, err := io.ReadAll(r.Body)
		noerr(t, err)

		gotBody = gb

		for k, vs := range respHeaders {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}

		if respStatus != 0 {
			w.WriteHeader(respStatus)
		}

		if len(respBody) > 0 {
			w.Write(respBody)
		}
	}))

	resp := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(resp, req)

	rr := resp.Result()

	specialStatusCase := respStatus == 0 && rr.StatusCode == http.StatusOK
	if rr.StatusCode != respStatus && !specialStatusCase {
		t.Errorf("Expected status code %d, got %d", respStatus, rr.StatusCode)
	}

	if !reflect.DeepEqual(rr.Header, respHeaders) {
		t.Errorf("Expected headers %v, got %v", respHeaders, rr.Header)
	}

	if !reflect.DeepEqual(resp.Body.Bytes(), respBody) {
		t.Errorf("Expected body %v, got %v", respBody, resp.Body.Bytes())
	}

	return gotBody
}

func dumpRequest(
	t *testing.T,
	dumpRequest bool,
	req *http.Request,
	dumpResponse bool,
	respStatus int,
	respBody []byte,
	respHeaders http.Header,
	opts []httpdump.Option,
) (*httpdump.Middleware, *httpDumpResult) {
	if respHeaders == nil {
		respHeaders = http.Header{}
	}

	m, result := newMiddleware(
		dumpRequest,
		dumpResponse,
		opts)

	gotBody := processWrappedRequest(
		t,
		m,
		req,
		respStatus,
		respBody,
		respHeaders)

	result.gotBody = gotBody

	return m, result
}

func iif[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

func noerr(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatal(err)
	}
}
