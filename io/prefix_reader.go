package io

import (
	"io"
)

func NewPrefixReader(r io.Reader, prefixLen int) (*PrefixReader, error) {
	pr := &PrefixReader{
		cache: make([]byte, prefixLen),
	}

	var err error
	if r != nil {
		err = pr.Reset(r)
	}

	return pr, err
}

type PrefixReader struct {
	r      io.Reader
	read   int
	cached int
	err    error
	cache  []byte
}

func (cr *PrefixReader) Reset(r io.Reader) error {
	cr.r = r
	cr.read = 0
	n, err := io.ReadAtLeast(r, cr.cache, len(cr.cache))
	cr.cached = n
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	cr.err = err
	return err
}

func (cr *PrefixReader) Prefix() []byte {
	return cr.cache[:cr.cached]
}

func (cr *PrefixReader) Read(p []byte) (int, error) {
	l := cr.cached - cr.read
	c := 0
	if l > 0 {
		c = copy(p, cr.cache[cr.read:cr.cached])
		cr.read += c
		if c == len(p) {
			// even if we have cr.err let's not return it here
			// subsequent reads will return it
			return c, nil
		}
	}

	if cr.err != nil {
		return c, cr.err
	}

	n, err := cr.r.Read(p[c:])
	return n + c, err
}

func (cr *PrefixReader) WriteTo(w io.Writer) (n int64, err error) {
	if cr.cached-cr.read > 0 {
		nn, err := w.Write(cr.cache[cr.read:cr.cached])
		n = int64(nn)
		if err != nil {
			return int64(n), err
		}
	}

	if wt, ok := cr.r.(io.WriterTo); ok {
		a, err := wt.WriteTo(w)
		return a + n, err
	}

	a, err := io.Copy(w, cr.r)
	return a + n, err
}

func (cr *PrefixReader) Close() error {
	if c, ok := cr.r.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
