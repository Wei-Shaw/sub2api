package handler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIModelEffortSuffixNormalizationCoversCanonicalHTTPHandlers(t *testing.T) {
	tests := []struct {
		file         string
		function     string
		responsesAPI bool
		mappingToken string
	}{
		{file: "openai_gateway_handler.go", function: "Responses", responsesAPI: true, mappingToken: "ResolveChannelMappingAndRestrict("},
		{file: "gateway_handler_responses.go", function: "Responses", responsesAPI: true, mappingToken: "ResolveChannelMappingAndRestrict("},
		{file: "openai_chat_completions.go", function: "ChatCompletions", responsesAPI: false, mappingToken: "ResolveChannelMappingAndRestrict("},
		{file: "gateway_handler_chat_completions.go", function: "ChatCompletions", responsesAPI: false, mappingToken: "ResolveChannelMappingAndRestrict("},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			source := stripGoComments(goFunctionSource(t, tt.file, tt.function))
			normalizeToken := "NormalizeOpenAIModelEffortSuffix(body, false)"
			if tt.responsesAPI {
				normalizeToken = "NormalizeOpenAIModelEffortSuffix(body, true)"
			}

			normalizeIndex := strings.Index(source, normalizeToken)
			require.NotEqual(t, -1, normalizeIndex, "missing model effort suffix normalization")
			require.Contains(t, source[normalizeIndex:], "body = normalizedBody", "normalized body must replace the request body")
			require.Contains(t, source[normalizeIndex:], `reqModel := gjson.GetBytes(body, "model").String()`, "routing model must come from the normalized body")

			mappingIndex := strings.Index(source, tt.mappingToken)
			require.NotEqual(t, -1, mappingIndex, "missing channel mapping")
			require.Less(t, normalizeIndex, mappingIndex, "normalization must precede channel and account mapping")
		})
	}
}
