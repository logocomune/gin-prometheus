package ginprom

import (
	"io"
)

// countingReadCloser wraps an *http.Request's Body and counts the number of
// bytes read through it as the downstream handler consumes them.
//
// It exists to measure request body size for requests with unknown
// Content-Length (e.g. chunked transfer-encoding) without buffering the
// body in memory and without altering what the handler receives — the same
// principle Gin's own ResponseWriter already uses to count response bytes
// (see c.Writer.Size()).
//
// It must be installed on c.Request.Body *before* the handler runs (i.e.
// before c.Next()); reading it afterwards would either see an
// already-drained, already-closed body, or — for a body the handler never
// touched — require an unbounded read to find out how large it was, which
// is exactly the memory-exhaustion issue this type replaces.
type countingReadCloser struct {
	io.ReadCloser
	n int64
}

// Read implements io.Reader, delegating to the wrapped body and counting
// every byte that passes through, including on a non-nil error (some
// readers return a final n > 0 together with io.EOF).
func (c *countingReadCloser) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	c.n += int64(n)
	return n, err
}

// bytesRead returns the number of bytes read through the wrapper so far.
// Safe to call on a nil *countingReadCloser (returns 0), which keeps call
// sites simple when wrapping was skipped (e.g. request-size recording is
// disabled, or the request has no body).
func (c *countingReadCloser) bytesRead() int64 {
	if c == nil {
		return 0
	}
	return c.n
}
