package handler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIHTTPHandlersPreserveRequestedModelForRouting(t *testing.T) {
	tests := []struct {
		file     string
		function string
	}{
		{file: "openai_gateway_handler.go", function: "Responses"},
		{file: "gateway_handler_responses.go", function: "Responses"},
		{file: "openai_chat_completions.go", function: "ChatCompletions"},
		{file: "gateway_handler_chat_completions.go", function: "ChatCompletions"},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			source := stripGoComments(goFunctionSource(t, tt.file, tt.function))

			require.NotContains(t, source, "NormalizeOpenAIModelEffortSuffix(", "handler must not rewrite a real model ID that ends with an effort-like suffix")
		modelIndex := strings.Index(source, "reqModel := modelResult.String()")
		mappingIndex := strings.Index(source, "ResolveChannelMappingAndRestrict(")
		require.NotEqual(t, -1, modelIndex, "routing model must come directly from the request")
		require.NotEqual(t, -1, mappingIndex, "missing channel mapping")
		require.Less(t, modelIndex, mappingIndex)
		})
	}
}
