package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractOpenAIResponsesErrorFields(t *testing.T) {
	tests := []struct {
		name            string
		payload         string
		transportStatus int
		wantCode        string
		wantType        string
		wantMessage     string
		wantSemantic    int
	}{
		{
			name:            "nested error",
			payload:         `{"error":{"type":"invalid_request","code":"cyber_policy","message":"nested"}}`,
			transportStatus: http.StatusBadRequest,
			wantCode:        "cyber_policy",
			wantType:        "invalid_request",
			wantMessage:     "nested",
			wantSemantic:    http.StatusBadRequest,
		},
		{
			name:            "response failed",
			payload:         `{"type":"response.failed","response":{"error":{"type":"invalid_request","code":"cyber_policy","message":"wrapped"}}}`,
			transportStatus: http.StatusOK,
			wantCode:        "cyber_policy",
			wantType:        "invalid_request",
			wantMessage:     "wrapped",
			wantSemantic:    http.StatusBadRequest,
		},
		{
			name:            "CPA legacy top level",
			payload:         `{"type":"error","code":"Cyber_Policy","message":"legacy","sequence_number":0}`,
			transportStatus: http.StatusOK,
			wantCode:        "Cyber_Policy",
			wantMessage:     "legacy",
			wantSemantic:    http.StatusBadRequest,
		},
		{
			name:            "top level event type is not semantic type",
			payload:         `{"type":"error","code":"provider_error","message":"failed"}`,
			transportStatus: http.StatusOK,
			wantCode:        "provider_error",
			wantMessage:     "failed",
			wantSemantic:    http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractOpenAIResponsesErrorFields([]byte(tt.payload), tt.transportStatus)
			require.Equal(t, tt.wantCode, got.Code)
			require.Equal(t, tt.wantType, got.Type)
			require.Equal(t, tt.wantMessage, got.Message)
			require.Equal(t, tt.transportStatus, got.TransportStatus)
			require.Equal(t, tt.wantSemantic, got.SemanticStatus)
		})
	}
}
