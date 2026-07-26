package openai

import (
	"errors"
	"fmt"
	"strings"
)

const (
	// UpstreamHTTP2StreamErrorCode is returned to OpenAI-compatible clients when
	// an upstream HTTP/2 response stream is reset after the request started.
	UpstreamHTTP2StreamErrorCode = "upstream_http2_stream_error"
	// UpstreamStreamReadErrorCode is the generic stream-read failure code.
	UpstreamStreamReadErrorCode = "upstream_stream_read_error"
)

// UpstreamStreamReadError is a sanitized classification of an upstream stream
// read failure for OpenAI-compatible client responses.
type UpstreamStreamReadError struct {
	cause         error
	clientCode    string
	clientMessage string
}

func (e *UpstreamStreamReadError) Error() string {
	return fmt.Sprintf("stream usage incomplete: %v", e.cause)
}

func (e *UpstreamStreamReadError) Unwrap() error { return e.cause }

// NewUpstreamStreamReadError wraps err with a stable client-facing classification.
func NewUpstreamStreamReadError(err error) error {
	code, message := classifyUpstreamStreamReadError(err)
	return &UpstreamStreamReadError{
		cause:         err,
		clientCode:    code,
		clientMessage: message,
	}
}

// UpstreamStreamReadErrorDetails returns the stable, sanitized client
// classification attached to an upstream stream read failure.
func UpstreamStreamReadErrorDetails(err error) (code, message string, ok bool) {
	var streamErr *UpstreamStreamReadError
	if !errors.As(err, &streamErr) || streamErr == nil {
		return "", "", false
	}
	return streamErr.clientCode, streamErr.clientMessage, true
}

func classifyUpstreamStreamReadError(err error) (code, message string) {
	if err != nil {
		lower := strings.ToLower(err.Error())
		// net/http's HTTP/2 stream error is unexported. Its stable text contains
		// "stream error: stream ID ..."; match only the transport signature and
		// never pass the original text to the client.
		if strings.Contains(lower, "stream error: stream id ") ||
			(strings.Contains(lower, "http2:") && strings.Contains(lower, "stream")) {
			return UpstreamHTTP2StreamErrorCode, "Upstream HTTP/2 stream failed"
		}
	}
	return UpstreamStreamReadErrorCode, "Upstream response stream was interrupted"
}
