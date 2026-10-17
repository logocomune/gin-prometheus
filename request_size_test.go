package ginprom

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMiddlewareUnknownRequestSize(t *testing.T) {
	const payload = "chunked upload payload"
	tests := []struct {
		name        string
		readBytes   int
		knownLength bool
		nilBody     bool
		emptyBody   bool
		replaceBody bool
		disabled    bool
		filtered    bool
	}{
		{name: "complete", readBytes: len(payload)},
		{name: "partial", readBytes: 4},
		{name: "unread", readBytes: 0},
		{name: "known length unread", knownLength: true},
		{name: "nil body", nilBody: true},
		{name: "empty body", emptyBody: true},
		{name: "handler replaces body", readBytes: len(payload), replaceBody: true},
		{name: "disabled", readBytes: 4, disabled: true},
		{name: "filtered", readBytes: 4, filtered: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metrics := newTestMetrics()
			router := gin.New()
			options := []Option{WithRecordRequestSize(!test.disabled)}
			if test.filtered {
				options = append(options, WithFilterRoutes([]string{"/upload"}))
			}
			router.Use(MiddlewareWithMetrics(metrics, options...))
			originalBody := io.NopCloser(strings.NewReader(payload))
			if test.emptyBody {
				originalBody = http.NoBody
			}
			router.POST("/upload", func(c *gin.Context) {
				if test.nilBody {
					if c.Request.Body != nil {
						t.Fatal("nil body was wrapped")
					}
					c.Status(http.StatusOK)
					return
				}
				_, wrapped := c.Request.Body.(*countingReadCloser)
				wantWrapped := !test.knownLength && !test.disabled && !test.filtered
				if wrapped != wantWrapped {
					t.Fatalf("body wrapped = %v, want %v", wrapped, wantWrapped)
				}
				data, err := io.ReadAll(io.LimitReader(c.Request.Body, int64(test.readBytes)))
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != payload[:test.readBytes] {
					t.Fatalf("body changed: %q", data)
				}
				if test.replaceBody {
					c.Request.Body = http.NoBody
				}
				c.Status(http.StatusOK)
			})
			request := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(payload))
			request.Body = originalBody
			if test.nilBody {
				request.Body = nil
			}
			if !test.knownLength {
				request.ContentLength = -1
			}
			router.ServeHTTP(httptest.NewRecorder(), request)
			if test.disabled || test.filtered {
				remainder, err := io.ReadAll(originalBody)
				if err != nil || string(remainder) != payload[test.readBytes:] {
					t.Fatalf("middleware consumed body: %q, %v", remainder, err)
				}
			}
			families, err := metrics.Registry.Gather()
			if err != nil {
				t.Fatal(err)
			}
			for _, family := range families {
				if family.GetName() != "http_request_size_bytes" {
					continue
				}
				if test.disabled || test.filtered {
					t.Fatal("request size recorded despite disabled recording or filtering")
				}
				if len(family.Metric) != 1 {
					t.Fatalf("expected one request-size series, got %d", len(family.Metric))
				}
				histogram := family.Metric[0].GetHistogram()
				wantSize := test.readBytes
				if test.knownLength {
					wantSize = len(payload)
				}
				if histogram.GetSampleCount() != 1 || histogram.GetSampleSum() != float64(wantSize) {
					t.Fatalf("request-size count/sum = %d/%g, want 1/%d", histogram.GetSampleCount(), histogram.GetSampleSum(), wantSize)
				}
				return
			}
			if !test.disabled && !test.filtered {
				t.Fatal("request-size histogram missing")
			}
		})
	}
}

type failingRequestBody struct {
	readError  error
	closeError error
	closed     bool
}

func (body *failingRequestBody) Read(buffer []byte) (int, error) {
	return copy(buffer, "partial"), body.readError
}

func (body *failingRequestBody) Close() error {
	body.closed = true
	return body.closeError
}

func TestCountingReadCloserErrors(t *testing.T) {
	for _, readError := range []error{io.EOF, errors.New("connection interrupted")} {
		t.Run(readError.Error(), func(t *testing.T) {
			original := &failingRequestBody{readError: readError, closeError: errors.New("close failed")}
			counter := &countingReadCloser{ReadCloser: original}
			buffer := make([]byte, 32)
			count, err := counter.Read(buffer)
			if err != readError || count != 7 || counter.bytesRead != 7 || string(buffer[:count]) != "partial" {
				t.Fatalf("read did not preserve bytes/error: %d, %v, %q", count, err, buffer[:count])
			}
			if err := counter.Close(); err != original.closeError || !original.closed {
				t.Fatalf("Close not delegated: %v", err)
			}
		})
	}
}

func checkCountingReads(payload []byte, chunkSize uint8) error {
	counter := &countingReadCloser{ReadCloser: io.NopCloser(bytes.NewReader(payload))}
	if counter.bytesRead != 0 {
		return errors.New("nonzero initial count")
	}
	buffer := make([]byte, int(chunkSize)+1)
	var received []byte
	for {
		count, err := counter.Read(buffer)
		received = append(received, buffer[:count]...)
		if counter.bytesRead != int64(len(received)) {
			return fmt.Errorf("count %d differs from bytes delivered %d", counter.bytesRead, len(received))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	if !bytes.Equal(received, payload) {
		return errors.New("body changed during reads")
	}
	return counter.Close()
}

func TestCountingReadCloserProperty(t *testing.T) {
	property := func(payload []byte, chunkSize uint8) bool {
		return checkCountingReads(payload, chunkSize) == nil
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}

func FuzzCountingReadCloser(f *testing.F) {
	f.Add([]byte("chunked upload payload"), uint8(3))
	f.Add([]byte{}, uint8(0))
	f.Add([]byte{0, 255, 13, 10}, uint8(255))
	f.Fuzz(func(t *testing.T, payload []byte, chunkSize uint8) {
		if err := checkCountingReads(payload, chunkSize); err != nil {
			t.Fatal(err)
		}
	})
}

func TestMiddlewareEmptyPath(t *testing.T) {
	metrics := newTestMetrics()
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	context.Request.URL.Path = ""
	MiddlewareWithMetrics(metrics)(context)
	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetName() == "path" && label.GetValue() != "/unmatched/*" {
					t.Fatalf("unexpected fallback label: %q", label.GetValue())
				}
			}
		}
	}
}

func TestHandleMetricsLargeStatusCode(t *testing.T) {
	metrics := newTestMetrics()
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	context.Status(1000)
	handleMetricsWithCollection(context, applyOpt(), "/", "/", time.Now(), metrics, nil)
	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetName() == "status_code" && label.GetValue() != "1000" {
					t.Fatalf("unexpected status label: %q", label.GetValue())
				}
			}
		}
	}
}
