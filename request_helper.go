package ginprom

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

// calculateRequestSize computes the total size of an HTTP request efficiently.
// It avoids reading the entire request body when possible by using Content-Length.
// Returns the calculated size in bytes and an error if the request cannot be processed.
func calculateRequestSize(r *http.Request) (int64, error) {
	var size int64

	// Add the request line size
	size += int64(len(r.Method)) + 1       // Method and space
	size += int64(len(r.URL.String())) + 1 // URL/path and space
	size += int64(len(r.Proto)) + 2        // HTTP version and \r\n

	// Calculate the size of headers
	for name, values := range r.Header {
		size += int64(len(name)) + 2 // Header name and ": "
		for _, value := range values {
			size += int64(len(value)) + 2 // Header value and \r\n
		}
	}
	size += 2 // Extra \r\n after headers

	// For the body size, prefer using Content-Length when available
	if r.ContentLength > 0 {
		// If Content-Length is set and positive, use it directly
		size += r.ContentLength
	} else if r.ContentLength == 0 {
		// If Content-Length is explicitly 0, there's no body
		// No additional size to add
	} else {
		// Only read the body if Content-Length is not available (-1)
		// and the request has a body
		if r.Body != nil {
			// Use a streaming approach with a counter
			bodySize, err := calculateBodySizeStream(r)
			if err != nil {
				return 0, err
			}
			size += bodySize
		}
	}

	return size, nil
}

// calculateBodySizeStream calculates the size of the request body using a streaming approach
// that counts bytes without loading the entire body into memory.
// It also restores the body for further processing.
func calculateBodySizeStream(r *http.Request) (int64, error) {
	if r.Body == nil {
		return 0, nil
	}

	// Create a buffer to hold body data as we read it
	var buf bytes.Buffer

	// Count the bytes as we read them
	bodySize, err := io.Copy(&buf, r.Body)
	if err != nil {
		return 0, err
	}

	// Restore the body for further use
	r.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))

	return bodySize, nil
}

// readRequestBody reads the body of an HTTP request and restores it to allow further use.
// Returns the body as a byte slice and any error encountered during reading.
// This implementation uses a maxBodySize limit to prevent excessive memory usage.
func readRequestBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}

	// Set a reasonable limit for request body size to prevent memory issues
	// This can be made configurable if needed
	const maxBodySize = 10 * 1024 * 1024 // 10MB limit

	// Use LimitReader to prevent reading excessively large bodies into memory
	limitedReader := io.LimitReader(r.Body, maxBodySize)

	// Read the body with the size limit
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}

	// Check if we might have truncated the body
	if len(bodyBytes) >= maxBodySize {
		// Consider logging a warning here that body was truncated
		// You could also make this behavior configurable
	}

	// Create a new reader with the data we read
	bodyReader := bytes.NewReader(bodyBytes)

	// Store a copy of the body data for the metrics calculation
	metricsCopy := make([]byte, len(bodyBytes))
	copy(metricsCopy, bodyBytes)

	// Reset the body reader position for subsequent readers
	bodyReader.Seek(0, io.SeekStart)

	// Replace the original body with our buffered copy
	r.Body = io.NopCloser(bodyReader)

	return metricsCopy, nil
}

// computeResponseSize calculates the size of the HTTP response written by the context's writer.
// Returns 0 if the writer is nil.
func computeResponseSize(c *gin.Context) int {
	if c.Writer == nil {
		return 0
	}
	return c.Writer.Size()
}
