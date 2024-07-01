package httpdump

import (
	"bytes"
	stdio "io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hummerd/httpdump/io"
)

const (
	HeaderContentType = "Content-Type"
)

const (
	// Default limit for dumped body size (both for request and response).
	DefaultBodySize = 1024
)

const (
	MimeApplicationJSON   = "application/json"
	MimeApplicationLDJSON = "application/ld+json"
	MimeApplicationXML    = "application/xml"
	MimeApplicationXHTML  = "application/xhtml+xml"
	MimeApplicationForm   = "application/x-www-form-urlencoded"
	MimeTextXML           = "text/xml"
	MimeTextHTML          = "text/html"
	MimeTextPlain         = "text/plain"
)

// DefaultDumpedContentTypes is a list of content types that are dumped by default.
var DefaultDumpedContentTypes = []string{
	MimeApplicationJSON,
	MimeApplicationLDJSON,
	MimeApplicationXML,
	MimeApplicationXHTML,
	MimeApplicationForm,
	MimeTextXML,
	MimeTextHTML,
	MimeTextPlain,
}

type DumpRequestFunc func(rq *http.Request, body []byte)
type DumpResponseFunc func(rp *http.Response, body []byte, duration time.Duration)
type RequestFilterFunc func(r *http.Request) (dump, body bool)
type ResponseFilterFunc func(r *http.Request, headers http.Header, status int) (dump, body bool)

// FilterRequestBodyByContentType creates a new request filter that
// will disable request body dump for specified content types.
func FilterRequestBodyByContentType(contentTypes []string) RequestFilterFunc {
	return func(r *http.Request) (bool, bool) {
		return isDumpedContentType(r.Header, contentTypes)
	}
}

// FilterResponseBodyByContentType creates a new response filter that
// will disable response body dump for specified content types.
func FilterResponseBodyByContentType(contentTypes []string) ResponseFilterFunc {
	return func(_ *http.Request, headers http.Header, _ int) (bool, bool) {
		return isDumpedContentType(headers, contentTypes)
	}
}

// WithPathFilter creates a new option that excludes request and response by path.
func WithPathFilter(regexps ...*regexp.Regexp) Option {
	req := WithRequestPathFilter(regexps...)
	res := WithResponsePathFilter(regexps...)

	return func(m *Middleware) {
		req(m)
		res(m)
	}
}

// Option is a middleware option that allows to set or
// override default behaviour of middleware.
type Option func(*Middleware)

// WithRequestFilters creates a new option that adds specified request filters.
func WithRequestFilters(filters ...RequestFilterFunc) Option {
	return func(m *Middleware) {
		m.requestFilters = append(m.requestFilters, filters...)
	}
}

// WithRequestPathFilter creates a new option that excludes request by path.
func WithRequestPathFilter(regexps ...*regexp.Regexp) Option {
	f := func(r *http.Request) (bool, bool) {
		for _, re := range regexps {
			if re.MatchString(r.URL.Path) {
				return false, false
			}
		}

		return true, true
	}

	return func(m *Middleware) {
		m.requestFilters = append(m.requestFilters, f)
	}
}

// WithResponseFilters creates a new option that adds specified response filters.
func WithResponseFilters(filters ...ResponseFilterFunc) Option {
	return func(m *Middleware) {
		m.responseFilters = append(m.responseFilters, filters...)
	}
}

// WithResponsePathFilter creates a new option that excludes response by path.
func WithResponsePathFilter(regexps ...*regexp.Regexp) Option {
	f := func(r *http.Request, headers http.Header, status int) (bool, bool) {
		for _, re := range regexps {
			if re.MatchString(r.URL.Path) {
				return false, false
			}
		}

		return true, true
	}

	return func(m *Middleware) {
		m.responseFilters = append(m.responseFilters, f)
	}
}

// WithLimitedBody creates a new option that sets limit for dumped body size.
func WithLimitedBody(limit int) Option {
	if limit <= 0 {
		panic("httpdump: limit must be greater than 0")
	}

	return func(m *Middleware) {
		m.dumpedBodySize = limit
	}
}

type Middleware struct {
	enabled         *atomic.Bool
	requestFilters  []RequestFilterFunc
	dumpRequest     DumpRequestFunc
	responseFilters []ResponseFilterFunc
	dumpResponse    DumpResponseFunc
	writerPool      *sync.Pool
	readerPool      *sync.Pool
	dumpedBodySize  int
}

// Creates http wrapper/middleware that dumps request and response.
// See NewMiddleware for more details.
func NewMiddlewareWrapper(
	dumpRequest DumpRequestFunc,
	dumpResponse DumpResponseFunc,
	opts ...Option,
) func(http.Handler) http.Handler {
	m := NewMiddleware(dumpRequest, dumpResponse, opts...)
	return m.Wrap
}

// NewMiddleware creates a new middleware that dumps request and response.
// By default ont the all body is dumped but only first DefaultBodySize bytes are saved,
// to change this behaviour set custom body limit.
// By default only body for DefaultDumpedContentTypes is dumped for both request and response,
// to change this behaviour set custom request and response filters.
func NewMiddleware(
	dumpRequest DumpRequestFunc,
	dumpResponse DumpResponseFunc,
	opts ...Option,
) *Middleware {
	enabled := &atomic.Bool{}
	enabled.Store(true)

	m := &Middleware{
		enabled:         enabled,
		requestFilters:  []RequestFilterFunc{FilterRequestBodyByContentType(DefaultDumpedContentTypes)},
		responseFilters: []ResponseFilterFunc{FilterResponseBodyByContentType(DefaultDumpedContentTypes)},
		dumpRequest:     dumpRequest,
		dumpResponse:    dumpResponse,
		dumpedBodySize:  DefaultBodySize,
	}

	for _, opt := range opts {
		opt(m)
	}

	m.writerPool = &sync.Pool{
		New: func() any {
			return newCachedWriter(nil, m.dumpedBodySize)
		},
	}
	m.readerPool = &sync.Pool{
		New: func() any {
			r, _ := io.NewPrefixReader(nil, m.dumpedBodySize)
			return r
		},
	}

	return m
}

// Enabled returns middleware enabled state. Safe to call from multiple goroutines.
func (m *Middleware) Enabled() bool {
	return m.enabled.Load()
}

// SetEnabled sets middleware enabled state. Safe to call from multiple goroutines.
func (m *Middleware) SetEnabled(v bool) {
	m.enabled.Store(v)
}

// Wrap wraps http.Handler with dump middleware.
func (m *Middleware) Wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.Handle(h, w, r)
	})
}

func (m *Middleware) Handle(next http.Handler, w http.ResponseWriter, r *http.Request) {
	if !m.enabled.Load() {
		next.ServeHTTP(w, r)
		return
	}

	start := time.Now()

	dumpReq, dumpReqBody := m.needDumpRequest(r)

	if dumpReq {
		var reqBody []byte

		if dumpReqBody {
			cr := m.readerPool.Get().(*io.PrefixReader)
			defer m.readerPool.Put(cr)

			// it's ok to ignore error here
			// further call to cr.Read() will return that error to caller
			// and we expect it to be handled there
			_ = cr.Reset(r.Body)

			r.Body = cr

			reqBody = cr.Prefix()
		}

		m.dumpRequest(r, reqBody)
	}

	var cw *cachedWriter

	if m.dumpResponse != nil {
		cw = m.writerPool.Get().(*cachedWriter)
		defer m.writerPool.Put(cw)

		cw.Reset(w, r, m.responseFilters...)

		w = cw
	}

	next.ServeHTTP(w, r)

	if m.dumpResponse != nil {
		cw.EnsureFilterPassed()

		if cw.dumpResponse {
			respBody := cw.Prefix()

			resp := newDumpedResponse(r, cw.Status(), respBody, cw.Header())
			m.dumpResponse(resp, respBody, time.Since(start))
		}
	}
}

func (m *Middleware) needDumpRequest(r *http.Request) (dump, body bool) {
	if m.dumpRequest == nil {
		return false, false
	}

	return filterPassed(r, m.requestFilters)
}

func filterPassed(r *http.Request, filters []RequestFilterFunc) (dump, body bool) {
	b := true

	for _, f := range filters {
		dump, body := f(r)
		if !dump {
			return false, false
		}

		if !body {
			b = false
		}
	}

	return true, b
}

func newDumpedResponse(
	r *http.Request,
	status int,
	body []byte,
	headers http.Header,
) *http.Response {
	return &http.Response{
		Proto:      r.Proto,
		ProtoMajor: r.ProtoMajor,
		ProtoMinor: r.ProtoMinor,
		Request:    r,
		Status:     http.StatusText(status),
		StatusCode: status,
		Body:       stdio.NopCloser(bytes.NewReader(body)),
		Header:     headers,
	}
}

func isDumpedContentType(headers http.Header, contentTypes []string) (bool, bool) {
	ct := headers.Get(HeaderContentType)
	for _, mt := range contentTypes {
		if strings.HasPrefix(ct, mt) {
			return true, true
		}
	}
	return true, false
}

func newCachedWriter(w http.ResponseWriter, s int) *cachedWriter {
	return &cachedWriter{
		PrefixWriter: *io.NewPrefixWriter(w, s),
		w:            w,
	}
}

type cachedWriter struct {
	io.PrefixWriter
	w            http.ResponseWriter
	statusCode   int
	request      *http.Request
	written      bool
	dumpBody     bool
	dumpResponse bool
	filters      []ResponseFilterFunc
}

func (cw *cachedWriter) Status() int {
	sc := cw.statusCode
	// http.ResponseWriter defaults to 200 Ok
	// if no status code is set
	if sc == 0 {
		sc = http.StatusOK
	}
	return sc
}

func (cw *cachedWriter) Reset(w http.ResponseWriter, r *http.Request, filters ...ResponseFilterFunc) {
	cw.PrefixWriter.Reset(w)

	cw.w = w
	cw.statusCode = 0
	cw.request = r
	cw.written = false
	cw.dumpBody = false
	cw.dumpResponse = false
	cw.filters = filters
}

func (cw *cachedWriter) Header() http.Header {
	return cw.w.Header()
}

func (cw *cachedWriter) EnsureFilterPassed() {
	if cw.written {
		return
	}

	if cw.statusCode == 0 {
		cw.statusCode = http.StatusOK
	}

	d, b := cw.filtered(
		cw.request,
		cw.w.Header(),
		cw.statusCode,
	)

	cw.dumpBody = b
	cw.dumpResponse = d
	cw.written = true
}

func (cw *cachedWriter) Write(data []byte) (int, error) {
	cw.EnsureFilterPassed()

	if !cw.dumpBody {
		return cw.w.Write(data)
	}

	n, err := cw.PrefixWriter.Write(data)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (cw *cachedWriter) WriteHeader(statusCode int) {
	if cw.statusCode == 0 {
		cw.statusCode = statusCode
	}

	cw.w.WriteHeader(statusCode)
}

func (cw *cachedWriter) filtered(r *http.Request, headers http.Header, status int) (bool, bool) {
	b := true

	for _, f := range cw.filters {
		dump, body := f(r, headers, status)
		if !dump {
			return false, false
		}

		if !body {
			b = false
		}
	}

	return true, b
}
