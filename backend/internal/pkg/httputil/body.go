package httputil

import (
	"bytes"
	"io"
	"net/http"
)

const (
	requestBodyReadInitCap    = 512
	requestBodyReadMaxInitCap = 1 << 20
)

// ReadRequestBodyWithPrealloc reads request body with preallocated buffer based on content length.
func ReadRequestBodyWithPrealloc(req *http.Request) ([]byte, error) {
	if req == nil || req.Body == nil {
		return nil, nil
REDACTED

	capHint := requestBodyReadInitCap
	if req.ContentLength > 0 {
		switch {
		case req.ContentLength < int64(requestBodyReadInitCap):
			capHint = requestBodyReadInitCap
		case req.ContentLength > int64(requestBodyReadMaxInitCap):
			capHint = requestBodyReadMaxInitCap
		default:
			capHint = int(req.ContentLength)
	REDACTED
REDACTED

	buf := bytes.NewBuffer(make([]byte, 0, capHint))
	if _, err := io.Copy(buf, req.Body); err != nil {
		return nil, err
REDACTED
	return buf.Bytes(), nil
REDACTED
