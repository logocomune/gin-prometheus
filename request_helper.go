package ginprom

import "io"

type countingReadCloser struct {
	io.ReadCloser
	bytesRead int64
}

func (body *countingReadCloser) Read(buffer []byte) (int, error) {
	count, err := body.ReadCloser.Read(buffer)
	body.bytesRead += int64(count)
	return count, err
}
