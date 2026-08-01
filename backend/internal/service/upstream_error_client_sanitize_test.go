package service

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestExtractClientSafeUpstreamErrorMessage_UnwrapsNestedBodyAndRemovesProviderID(t *testing.T) {
	body := []byte(`{"error":{"message":"{\"error\":{\"type\":\"\\u003cnil\\u003e\",\"message\":\"Malformed request: please ensure the input adheres to the required specification. (request id: 202607281609465064538388268d9d67wbgXJSe)\"},\"type\":\"error\"}data: {\"type\":\"error\",\"error\":{\"type\":\"upstream_error\",\"message\":\"Upstream request failed\"}}","type":"upstream_error"}}`)

	rawMessage := ExtractUpstreamErrorMessage(body)
	require.Equal(t, "Malformed request: please ensure the input adheres to the required specification. (request id: 202607281609465064538388268d9d67wbgXJSe)", rawMessage)
	require.Equal(t, "Malformed request: please ensure the input adheres to the required specification.", ExtractClientSafeUpstreamErrorMessage(body))
}

func TestExtractClientSafeUpstreamErrorMessage_PreservesActionableDiagnostic(t *testing.T) {
	body := []byte(`{"error":{"message":"messages.3.content.0.text must not be empty; remove the empty block (request_id: req_01ABCDEF)"}}`)

	require.Equal(t,
		"messages.3.content.0.text must not be empty; remove the empty block",
		ExtractClientSafeUpstreamErrorMessage(body),
	)
}

func TestSanitizeClientUpstreamErrorMessage_CorrelationIDVariants(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "parenthesized request id", in: "invalid value (Request ID: abcdef123456)", want: "invalid value"},
		{name: "bracketed trace id", in: "invalid value [trace-id=trace_123456]", want: "invalid value"},
		{name: "bare correlation id suffix", in: "invalid value correlationId: corr-123456", want: "invalid value"},
		{name: "field name is diagnostic", in: "request_id is required", want: "request_id is required"},
		{name: "request id validation is diagnostic", in: "invalid field (request id: must be a UUID)", want: "invalid field (request id: must be a UUID)"},
		{name: "specific field detail", in: "tools.2.input_schema.type must be object", want: "tools.2.input_schema.type must be object"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, sanitizeClientUpstreamErrorMessage(tt.in))
		})
	}
}

func TestWriteAnthropicPassthroughResponseHeaders_PreservesGatewayRequestID(t *testing.T) {
	dst := http.Header{"X-Request-Id": []string{"gateway-request-id"}}
	src := http.Header{
		"Content-Type": []string{"text/event-stream"},
		"X-Request-Id": []string{"provider-request-id"},
	}
	filter := compileResponseHeaderFilter(&config.Config{})

	writeAnthropicPassthroughResponseHeaders(dst, src, filter)

	require.Equal(t, "gateway-request-id", dst.Get("X-Request-Id"))
	require.Equal(t, "text/event-stream", dst.Get("Content-Type"))
}
