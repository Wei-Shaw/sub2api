package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// Clients such as Pi retry a mid-stream failure only when its message says so,
// as OpenAI's own transient errors do. Every transient classification must
// carry that guidance while keeping its stable code.
func TestClassifyOpenAIUpstreamStreamReadErrorCarriesRetryGuidance(t *testing.T) {
	cases := []struct {
		err  error
		code string
	}{
		{errors.New("stream error: stream ID 5; INTERNAL_ERROR; received from peer"), OpenAIUpstreamHTTP2StreamErrorCode},
		{errors.New(`http2: server sent GOAWAY and closed the connection; LastStreamID=1, ErrCode=INTERNAL_ERROR, debug=""`), OpenAIUpstreamHTTP2StreamErrorCode},
		{ErrOpenAIUpstreamStreamTruncated, OpenAIUpstreamStreamTruncatedCode},
		{errors.New("unexpected EOF"), OpenAIUpstreamStreamReadErrorCode},
	}
	for _, tc := range cases {
		code, message := classifyOpenAIUpstreamStreamReadError(tc.err)
		require.Equal(t, tc.code, code, tc.err.Error())
		require.Contains(t, message, "You can retry your request.", tc.err.Error())
		require.NotContains(t, message, "INTERNAL_ERROR")
		require.NotContains(t, message, "stream ID")
	}
	require.JSONEq(t,
		`{"error":{"type":"upstream_error","code":"upstream_stream_truncated","message":"Upstream response stream ended before completion. You can retry your request."}}`,
		string(openAIRawStreamTruncatedErrorBody(ErrOpenAIUpstreamStreamTruncated)))
}
