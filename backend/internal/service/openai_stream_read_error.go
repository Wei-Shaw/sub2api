package service

import "github.com/Wei-Shaw/sub2api/internal/pkg/openai"

const (
	// OpenAIUpstreamHTTP2StreamErrorCode is returned to OpenAI-compatible clients
	// when an upstream HTTP/2 response stream is reset after the request started.
	OpenAIUpstreamHTTP2StreamErrorCode = openai.UpstreamHTTP2StreamErrorCode
	OpenAIUpstreamStreamReadErrorCode  = openai.UpstreamStreamReadErrorCode
)

func newOpenAIUpstreamStreamReadError(err error) error {
	return openai.NewUpstreamStreamReadError(err)
}

// OpenAIUpstreamStreamReadErrorDetails returns the stable, sanitized client
// classification attached to an upstream stream read failure.
func OpenAIUpstreamStreamReadErrorDetails(err error) (code, message string, ok bool) {
	return openai.UpstreamStreamReadErrorDetails(err)
}
