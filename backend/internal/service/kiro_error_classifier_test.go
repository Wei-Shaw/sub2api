package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyKiroHTTPErrorBadRequestCategories(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "schema",
			body: `{"message":"Improperly formed request: inputSchema.properties must be an object"}`,
			want: kiroErrorBadRequestSchema,
		},
		{
			name: "tool pairing",
			body: `{"message":"tool_use must be paired with a matching tool_result"}`,
			want: kiroErrorBadRequestToolPairing,
		},
		{
			name: "invalid model id",
			body: `{"message":"invalid modelId: model not supported"}`,
			want: kiroErrorBadRequestInvalidModel,
		},
		{
			name: "invalid model upstream",
			body: `{"error":{"message":"Invalid model. Please select a different model to continue.","type":"upstream_error"}}`,
			want: kiroErrorBadRequestInvalidModel,
		},
		{
			name: "invalid model reason",
			body: `{"message":"model route unavailable","reason":"INVALID_MODEL_ID"}`,
			want: kiroErrorBadRequestInvalidModel,
		},
		{
			name: "auth",
			body: `{"error":"invalid_grant","message":"Invalid refresh token provided"}`,
			want: kiroErrorBadRequestAuth,
		},
		{
			name: "quota",
			body: `{"message":"resource has been exhausted"}`,
			want: kiroErrorBadRequestQuota,
		},
		{
			name: "unknown",
			body: `{"message":"bad request"}`,
			want: kiroErrorBadRequestUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			classification := classifyKiroHTTPError(http.StatusBadRequest, tt.body)
			require.Equal(t, tt.want, classification.Category)
			require.Equal(t, http.StatusBadRequest, classification.StatusCode)
			require.Equal(t, tt.body, classification.Message)
		})
	}
}
