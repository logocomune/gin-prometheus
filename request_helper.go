package ginprom

import (
	"bytes"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// calculateRequestSize computes the total size of an HTTP request, including method, URL, headers, and body content.
// Returns the calculated size in bytes and an error if the request body cannot be read successfully.
func calculateRequestSize(r *http.Request) (int64, error) {

	var size int64

	// Calculate the size of the request line (method, URL, and HTTP version)
	size += int64(len(r.Method)) + 1 // Method and space
	if r.URL.User != nil {
		size += int64(len(r.URL.String())) + 1 // URL and space
	}
	size += int64(len(r.Proto)) + 2 // HTTP version and \r\n

	// Calculate the size of headers
	for name, values := range r.Header {
		size += int64(len(name)) + 2 // Header name and ": "
		for _, value := range values {
			size += int64(len(value)) + 2 // Header value and \r\n
		}
	}
	size += 2 // Extra \r\n after headers

	// Calculate the size of the body, if present
	if r.Body != nil {
		bodyBytes, err := readRequestBody(r)
		if err != nil {
			return 0, err
		}
		size += int64(len(bodyBytes))
	}

	return size, nil
}

// readRequestBody reads the body of an HTTP request and restores it to allow further use.
// Returns the body as a byte slice and any error encountered during reading.
func readRequestBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}

	// Read the body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	// Restore the original body for further use
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	return bodyBytes, nil
}

// computeResponseSize calculates the size of the HTTP response written by the context's writer.
// Returns 0 if the writer is nil.
func computeResponseSize(c *gin.Context) int {
	if c.Writer == nil {
		return 0
	}
	return c.Writer.Size()
}
