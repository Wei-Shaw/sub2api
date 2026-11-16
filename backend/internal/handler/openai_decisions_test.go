package handler

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestValidateOpenAIDecisionsRequest(t *testing.T) {
	question := `{"type":"predicate","name":"ok","instructions":"OK?"}`
	for _, tc := range []struct {
		name, body, message string
	}{
		{"text", `{"input":"hello","questions":[` + question + `]}`, ""},
		{"image", `{"input":[{"role":"user","content":[{"type":"input_image","image_url":"https://example.com/a.png"}]}],"questions":[` + question + `]}`, ""},
		{"explicit nonstream", `{"input":"hello","questions":[` + question + `],"stream":false}`, ""},
		{"stream", `{"input":"hello","questions":[` + question + `],"stream":true}`, "does not support streaming"},
		{"missing evidence", `{"questions":[` + question + `]}`, "input must be"},
		{"missing questions", `{"input":"hello"}`, "questions must be"},
		{"empty questions", `{"input":"hello","questions":[]}`, "questions must be"},
		{"object questions", `{"input":"hello","questions":{"q":{}}}`, "questions must be"},
		{"too many questions", fmt.Sprintf(`{"input":"x","questions":[%s]}`, strings.TrimSuffix(strings.Repeat(question+",", 401), ",")), "questions must be"},
		{"maximum questions", fmt.Sprintf(`{"input":"x","questions":[%s]}`, strings.TrimSuffix(strings.Repeat(question+",", 400), ",")), ""},
		{"array body", `[]`, "JSON object"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message := validateOpenAIDecisionsRequest([]byte(tc.body))
			if tc.message == "" {
				require.Empty(t, message)
			} else {
				require.Contains(t, message, tc.message)
			}
		})
	}
}

func TestDecisionsEndpointNormalization(t *testing.T) {
	for _, path := range []string{"/v1/decisions", "/v1/decisions/", "/decisions"} {
		require.Equal(t, EndpointDecisions, NormalizeInboundEndpoint(path))
		require.Equal(t, EndpointDecisions, DeriveUpstreamEndpoint(NormalizeInboundEndpoint(path), path, service.PlatformOpenAI))
	}
}
