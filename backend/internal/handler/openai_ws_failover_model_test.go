package handler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIWSCurrentTurnFailoverUsesCurrentModel(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModeCtxPool, service.OpenAIWSIngressModeHTTPBridge} {
		for _, targetMode := range []string{mode, service.OpenAIWSIngressModePassthrough} {
			for _, tc := range []struct {
				name           string
				firstModel     string
				currentModel   string
				channelMapping map[string]string
			}{
				{name: "direct_model", firstModel: "gpt-5.1", currentModel: "gpt-5.2"},
				{
					name: "channel_alias", firstModel: "client-a", currentModel: "client-b",
					channelMapping: map[string]string{"client-a": "gpt-5.1", "client-b": "gpt-5.2"},
				},
			} {
				t.Run(mode+"_to_"+targetMode+"/"+tc.name, func(t *testing.T) {
					conn, repo, attempts := newOpenAIWSTurnBudgetSession(t, mode,
						[]string{"response.completed", "429", "response.completed", "response.completed"}, tc.channelMapping)
					repo.mu.Lock()
					repo.accounts[0].Credentials["model_mapping"] = map[string]any{"gpt-5.1": "gpt-5.1", "gpt-5.2": "gpt-5.2"}
					repo.accounts[1].Credentials["model_mapping"] = map[string]any{"gpt-5.1": "gpt-5.1"}
					repo.accounts[2].Credentials["model_mapping"] = map[string]any{"gpt-5.2": "gpt-5.2"}
					repo.accounts[2].Extra["openai_apikey_responses_websockets_v2_mode"] = targetMode
					repo.mu.Unlock()

					for turn, model := range []string{tc.firstModel, tc.currentModel, ""} {
						modelField := ""
						if model != "" {
							modelField = fmt.Sprintf(`,"model":%q`, model)
						}
						payload := fmt.Sprintf(`{"type":"response.create"%s,"instructions":"turn-%d","input":[{"role":"user","content":"hello"}]}`, modelField, turn+1)
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(payload)))
						_, event, err := conn.Read(ctx)
						cancel()
						require.NoError(t, err, "turn %d should complete on the same client connection", turn+1)
						require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
					}

					gotAccounts, gotTurns, gotModels := attempts()
					require.Equal(t, []int64{801, 801, 803, 803}, gotAccounts, "failover must skip the first-model-only account and select the current-model-only account")
					require.Equal(t, []string{"turn-1", "turn-2", "turn-2", "turn-3"}, gotTurns)
					require.Equal(t, []string{"gpt-5.1", "gpt-5.2", "gpt-5.2", "gpt-5.2"}, gotModels, "replay and subsequent turns must retain the current model and apply channel mapping")
				})
			}
		}
	}
}
