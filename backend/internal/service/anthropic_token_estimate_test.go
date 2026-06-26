package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEstimateAnthropicMessagesTokens(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"system":"abcd",
		"messages":[
			{"role":"user","content":[{"type":"text","text":"abcdefgh"}]},
			{"role":"assistant","content":[{"type":"tool_use","name":"toolname","input":{"city":"Paris"}}]},
			{"role":"user","content":[{"type":"tool_result","content":"done"}]}
		],
		"tools":[{"name":"lookup","description":"abcdefgh","input_schema":{"type":"object"}}]
	}`)

	require.Positive(t, EstimateAnthropicMessagesTokens(body))
	require.Equal(t, 19, EstimateAnthropicMessagesTokens(body))
}
