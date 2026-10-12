package ginprom

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// countingReadCloser
// ---------------------------------------------------------------------------

func TestCountingReadCloser_NilReceiver_BytesRead(t *testing.T) {
	var c *countingReadCloser
	if got := c.bytesRead(); got != 0 {
		t.Errorf("expected 0 for nil *countingReadCloser, got %d", got)
	}
}

func TestCountingReadCloser_ZeroBeforeRead(t *testing.T) {
	c := &countingReadCloser{ReadCloser: io.NopCloser(strings.NewReader("hello"))}
	if got := c.bytesRead(); got != 0 {
		t.Errorf("expected 0 before any Read, got %d", got)
	}
}

func TestCountingReadCloser_CountsFullRead(t *testing.T) {
	const data = "streaming body content"
	c := &countingReadCloser{ReadCloser: io.NopCloser(strings.NewReader(data))}

	got, err := io.ReadAll(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != data {
		t.Errorf("expected body %q to pass through unchanged, got %q", data, string(got))
	}
	if size := c.bytesRead(); size != int64(len(data)) {
		t.Errorf("expected bytesRead() == %d, got %d", len(data), size)
	}
}

func TestCountingReadCloser_EmptyBody(t *testing.T) {
	c := &countingReadCloser{ReadCloser: io.NopCloser(bytes.NewReader(nil))}

	if _, err := io.ReadAll(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size := c.bytesRead(); size != 0 {
		t.Errorf("expected 0, got %d", size)
	}
}

// CountsPartialRead verifies that bytes are counted as they are consumed —
// i.e. the size reflects what the handler actually read, not the full body,
// when the handler stops reading early (e.g. it only needs a header and
// ignores the rest of the payload).
func TestCountingReadCloser_CountsPartialRead(t *testing.T) {
	const data = "0123456789"
	c := &countingReadCloser{ReadCloser: io.NopCloser(strings.NewReader(data))}

	buf := make([]byte, 4)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 4 {
		t.Fatalf("expected to read 4 bytes, got %d", n)
	}
	if size := c.bytesRead(); size != 4 {
		t.Errorf("expected bytesRead() == 4 after partial read, got %d", size)
	}
}

// erroringReader returns some data together with a non-nil error on its
// final call, mirroring real-world readers (e.g. a dropped connection
// mid-body). countingReadCloser must still count the bytes it did deliver.
type erroringReader struct {
	data []byte
	sent bool
}

func (r *erroringReader) Read(p []byte) (int, error) {
	if r.sent {
		return 0, io.EOF
	}
	r.sent = true
	n := copy(p, r.data)
	return n, errors.New("connection reset")
}

func TestCountingReadCloser_CountsBytesOnErroredRead(t *testing.T) {
	c := &countingReadCloser{ReadCloser: io.NopCloser(&erroringReader{data: []byte("partial")})}

	buf := make([]byte, 32)
	n, err := c.Read(buf)
	if err == nil {
		t.Fatal("expected an error from the underlying reader")
	}
	if n != len("partial") {
		t.Fatalf("expected %d bytes, got %d", len("partial"), n)
	}
	if size := c.bytesRead(); size != int64(len("partial")) {
		t.Errorf("expected bytesRead() == %d even though Read errored, got %d", len("partial"), size)
	}
}
