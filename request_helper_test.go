package ginprom

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// calculateRequestSize
// ---------------------------------------------------------------------------

func TestCalculateRequestSize_NoUserInfo(t *testing.T) {
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	size, err := calculateRequestSize(req)
	if err != nil {
		t.Fatalf("calculateRequestSize returned error: %v", err)
	}

	expected := int64(len(req.Method)) + 1 + int64(len(req.URL.String())) + 1 + int64(len(req.Proto)) + 2 + 2
	if size != expected {
		t.Errorf("expected %d, got %d", expected, size)
	}
}

func TestCalculateRequestSize_WithUserInfo(t *testing.T) {
	req, err := http.NewRequest("GET", "http://user:pass@example.com/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	size, err := calculateRequestSize(req)
	if err != nil {
		t.Fatalf("calculateRequestSize returned error: %v", err)
	}

	expected := int64(len(req.Method)) + 1 + int64(len(req.URL.String())) + 1 + int64(len(req.Proto)) + 2 + 2
	if size != expected {
		t.Errorf("expected %d, got %d", expected, size)
	}
}

func TestCalculateRequestSize_WithHeaders(t *testing.T) {
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Custom-Header", "value")

	size, err := calculateRequestSize(req)
	if err != nil {
		t.Fatalf("calculateRequestSize returned error: %v", err)
	}
	if size <= 0 {
		t.Errorf("expected positive size, got %d", size)
	}
}

func TestCalculateRequestSize_WithPositiveContentLength(t *testing.T) {
	body := strings.NewReader("hello world")
	req, err := http.NewRequest("POST", "/test", body)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.ContentLength = 11

	size, err := calculateRequestSize(req)
	if err != nil {
		t.Fatalf("calculateRequestSize returned error: %v", err)
	}
	// Size must include at least the body length
	if size < 11 {
		t.Errorf("expected size >= 11, got %d", size)
	}
}

func TestCalculateRequestSize_WithZeroContentLength(t *testing.T) {
	req, err := http.NewRequest("POST", "/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.ContentLength = 0

	size, err := calculateRequestSize(req)
	if err != nil {
		t.Fatalf("calculateRequestSize returned error: %v", err)
	}
	if size < 0 {
		t.Errorf("expected non-negative size, got %d", size)
	}
}

func TestCalculateRequestSize_WithUnknownContentLengthAndBody(t *testing.T) {
	bodyData := "streaming body content"
	body := io.NopCloser(strings.NewReader(bodyData))
	req, err := http.NewRequest("POST", "/test", body)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.ContentLength = -1

	size, err := calculateRequestSize(req)
	if err != nil {
		t.Fatalf("calculateRequestSize returned error: %v", err)
	}
	if size < int64(len(bodyData)) {
		t.Errorf("expected size >= %d, got %d", len(bodyData), size)
	}
}

func TestCalculateRequestSize_WithNilBody(t *testing.T) {
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.ContentLength = -1
	req.Body = nil

	size, err := calculateRequestSize(req)
	if err != nil {
		t.Fatalf("calculateRequestSize returned error: %v", err)
	}
	if size < 0 {
		t.Errorf("expected non-negative size, got %d", size)
	}
}

// ---------------------------------------------------------------------------
// calculateBodySizeStream
// ---------------------------------------------------------------------------

func TestCalculateBodySizeStream_NilBody(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.Body = nil
	size, err := calculateBodySizeStream(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != 0 {
		t.Errorf("expected 0, got %d", size)
	}
}

func TestCalculateBodySizeStream_WithBody(t *testing.T) {
	data := "test body"
	req, _ := http.NewRequest("POST", "/", strings.NewReader(data))

	size, err := calculateBodySizeStream(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != int64(len(data)) {
		t.Errorf("expected %d, got %d", len(data), size)
	}

	// Body should be restored and readable
	restored, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read restored body: %v", err)
	}
	if string(restored) != data {
		t.Errorf("body not restored correctly: got %q", string(restored))
	}
}

func TestCalculateBodySizeStream_EmptyBody(t *testing.T) {
	req, _ := http.NewRequest("POST", "/", bytes.NewReader([]byte{}))

	size, err := calculateBodySizeStream(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != 0 {
		t.Errorf("expected 0, got %d", size)
	}
}

// ---------------------------------------------------------------------------
// readRequestBody
// ---------------------------------------------------------------------------

func TestReadRequestBody_NilBody(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	data, err := readRequestBody(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data != nil {
		t.Errorf("expected nil, got %v", data)
	}
}

func TestReadRequestBody_WithBody(t *testing.T) {
	payload := "request body content"
	req, _ := http.NewRequest("POST", "/", strings.NewReader(payload))

	data, err := readRequestBody(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != payload {
		t.Errorf("expected %q, got %q", payload, string(data))
	}

	// Body must still be readable after readRequestBody
	restored, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read restored body: %v", err)
	}
	if string(restored) != payload {
		t.Errorf("body not properly restored: got %q", string(restored))
	}
}

func TestReadRequestBody_EmptyBody(t *testing.T) {
	req, _ := http.NewRequest("POST", "/", bytes.NewReader([]byte{}))

	data, err := readRequestBody(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty slice, got %v", data)
	}
}

// ---------------------------------------------------------------------------
// computeResponseSize
// ---------------------------------------------------------------------------

func TestComputeResponseSize_NilWriter(t *testing.T) {
	c := &gin.Context{}
	size := computeResponseSize(c)
	if size != 0 {
		t.Errorf("expected 0 for nil writer, got %d", size)
	}
}

func TestComputeResponseSize_WithResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.String(http.StatusOK, "hello world")

	size := computeResponseSize(c)
	if size <= 0 {
		t.Errorf("expected positive size, got %d", size)
	}
}
