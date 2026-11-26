//go:build unit

package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestNativeSearchHandlerErrorAccounting(t *testing.T) {
	testNativeSearchHandlerErrorAccounting(t, preContentHandlerFailed, false)
}

func TestNativeSearchHandlerCyberErrorAccounting(t *testing.T) {
	testNativeSearchHandlerErrorAccounting(t, strings.ReplaceAll(preContentHandlerFailed, "server_error", "cyber_policy"), true)
}

func testNativeSearchHandlerErrorAccounting(t *testing.T, failed string, cyber bool) {
	t.Helper()
	for _, path := range []string{"messages", "chat/completions", "responses", "passthrough"} {
		for _, stream := range []bool{false, true} {
			for _, completed := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%v/completed=%v", path, stream, completed), func(t *testing.T) {
					accounts := preContentAccounts(2)
					endpoint := path
					if path == "passthrough" {
						endpoint = "responses"
						for index := range accounts {
							accounts[index].Extra = map[string]any{"openai_passthrough": true}
						}
					}
					request := fmt.Sprintf(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"search"}],"stream":%v}`, stream)
					if endpoint == "responses" {
						request = fmt.Sprintf(`{"model":"gpt-5.4","input":"search","stream":%v}`, stream)
					}
					usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 3)}
					recorder, upstream := serveMessagesScenario(t, preContentFailoverScenario{
						accounts: accounts, path: "/v1/" + endpoint, requestBody: request, usageRepo: usageRepo,
						respond: func(call int) (*http.Response, error) {
							wire := preContentHandlerSuccess
							if call == 1 {
								wire = ""
								if completed {
									wire += `data: {"type":"response.web_search_call.completed","item_id":"ws_1","output_index":0}` + "\n\n"
								}
								wire += failed
							}
							return preContentSSEResponse(io.NopCloser(strings.NewReader(wire))), nil
						},
						afterServe: func(gateway *service.OpenAIGatewayService) {
							if completed || cyber {
								require.Zero(t, gateway.SnapshotOpenAIAccountSchedulerMetrics().AccountSwitchTotal)
							}
						},
					})
					if !completed && !cyber {
						require.Equal(t, 2, upstream.calls(), recorder.Body.String())
						return
					}
					require.Equal(t, 1, upstream.calls(), recorder.Body.String())
					require.NotContains(t, recorder.Body.String(), "answer from second account")
					require.Contains(t, recorder.Body.String(), "error")
					if !completed && cyber && endpoint == "responses" && !stream {
						return
					}
					select {
					case log := <-usageRepo.created:
						require.Zero(t, log.InputTokens)
						require.Zero(t, log.OutputTokens)
						require.Zero(t, log.CacheReadTokens)
						require.Zero(t, log.CacheCreationTokens)
						expectedCost := 0.0
						if completed {
							expectedCost = 0.01
						}
						require.InDelta(t, expectedCost, log.TotalCost, 1e-12)
					case <-time.After(3 * time.Second):
						t.Fatal("completed search error was not billed")
					}
					require.Empty(t, usageRepo.created)
				})
			}
		}
	}
}
