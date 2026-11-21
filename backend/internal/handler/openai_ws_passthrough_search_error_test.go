package handler

import (
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesWebSocketNativeSearchAbnormalCloseBilling(t *testing.T) {
	for _, turn := range []int{1, 2} {
		t.Run(fmt.Sprintf("turn=%d", turn), func(t *testing.T) {
			testCase := openAIResponsesWSUsageLogCase{
				firstPayload:       `{"type":"response.create","model":"gpt-5.4","stream":true}`,
				ingressMode:        service.OpenAIWSIngressModePassthrough,
				abnormalSearchTurn: turn,
			}
			if turn == 2 {
				testCase.secondPayload = testCase.firstPayload
			}
			got := runOpenAIResponsesWebSocketUsageLogCase(t, testCase)
			require.Len(t, got.logs, turn)
			partial := got.logs[turn-1]
			require.Zero(t, partial.InputTokens)
			require.Zero(t, partial.OutputTokens)
			require.Zero(t, partial.CacheCreationTokens)
			require.Zero(t, partial.CacheReadTokens)
			require.InDelta(t, 0.01, partial.TotalCost, 1e-12)
			if turn == 2 {
				require.Equal(t, 2, got.logs[0].InputTokens)
				require.Equal(t, 1, got.logs[0].OutputTokens)
			}
		})
	}
}
