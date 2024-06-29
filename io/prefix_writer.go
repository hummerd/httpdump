package io

import (
	"io"
)

func NewPrefixWriter(w io.Writer, prefixLen int) *PrefixWriter {
	return &PrefixWriter{
		w:      w,
		cached: 0,
		cache:  make([]byte, prefixLen),
	}
}

type PrefixWriter struct {
	w      io.Writer
	cached int
	cache  []byte
}

func (pw *PrefixWriter) Prefix() []byte {
	return pw.cache[:pw.cached]
}

func (pw *PrefixWriter) Reset(w io.Writer) {
	pw.w = w
	pw.cached = 0
}

func (pw *PrefixWriter) Write(data []byte) (int, error) {
	l := len(pw.cache) - pw.cached
	if l > 0 {
		if l > len(data) {
			l = len(data)
		}
		n := copy(pw.cache[pw.cached:], data[:l])
		pw.cached += n
	}

	return pw.w.Write(data)
}

func (pw *PrefixWriter) ReadFrom(r io.Reader) (n int64, err error) {
	l := len(pw.cache) - pw.cached
	if l > 0 {
		nn, err := io.ReadAtLeast(r, pw.cache[pw.cached:], l)
		n = int64(nn)
		if nn > 0 {
			_, err = pw.w.Write(pw.cache[pw.cached : pw.cached+nn])
			if err != nil {
				return n, err
			}
		}

		pw.cached += nn

		if err != nil {
			if err == io.ErrUnexpectedEOF {
				return n, io.EOF
			}
			return n, err
		}
	}

	if rf, ok := pw.w.(io.ReaderFrom); ok {
		a, err := rf.ReadFrom(r)
		return a + n, err
	}

	a, err := io.Copy(pw.w, r)
	return a + n, err
}

func (pw *PrefixWriter) Close() error {
	if c, ok := pw.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
