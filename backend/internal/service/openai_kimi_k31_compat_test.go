package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Exercise the gateway boundaries, not just the effort normalizer: model
// mapping must happen before model-specific effort handling. The fixture
// accepts none so this test can verify that disabled thinking reaches upstream
// unchanged; real K3.1 accounts reject it rather than silently enabling it.
func TestKimiK31MappedReasoningAcrossProtocols(t *testing.T) {
	for _, model := range []string{"k3.1-pro", "k3.1-pro-highspeed"} {
		for _, protocol := range []string{"responses", "chat/completions", "messages"} {
			for _, effort := range []string{"max", "medium", "none"} {
				t.Run(fmt.Sprintf("%s/%s/%s", model, protocol, effort), func(t *testing.T) {
					body := map[string]any{"model": "public-kimi", "stream": false}
					switch protocol {
					case "responses":
						body["input"] = "hello"
						body["reasoning"] = map[string]string{"effort": effort}
					case "chat/completions":
						body["messages"] = []map[string]string{{"role": "user", "content": "hello"}}
						body["reasoning_effort"] = effort
					case "messages":
						body["messages"] = []map[string]string{{"role": "user", "content": "hello"}}
						body["max_tokens"] = 1000
						body["output_config"] = map[string]string{"effort": effort}
						if effort == "none" {
							body["thinking"] = map[string]string{"type": "disabled"}
							// Explicit disable must win over an otherwise valid max request.
							body["output_config"] = map[string]string{"effort": "max"}
						}
					}
					payload, err := json.Marshal(body)
					require.NoError(t, err)
					response := `data: {"type":"response.completed","response":{"id":"resp_kimi","model":"` + model + `","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":2}}}` + "\n\n"
					upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(response))}}
					svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
					account := rawGPT56ResponsesAPIKeyAccount("public-kimi", model)
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+protocol, strings.NewReader(string(payload)))
					SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
					var result *OpenAIForwardResult
					switch protocol {
					case "responses":
						result, err = svc.Forward(context.Background(), c, account, payload)
					case "chat/completions":
						result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, payload, "", "")
					case "messages":
						result, err = svc.ForwardAsAnthropic(context.Background(), c, account, payload, "", "")
					}
					require.NoError(t, err)
					require.NotNil(t, result)
					require.NotNil(t, upstream.lastReq)
					require.Equal(t, "/v1/responses", upstream.lastReq.URL.Path)
					require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String())
					require.Equal(t, effort, gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
				})
			}
		}
	}
}

func TestKimiK31NoneSurvivesBothRequestFilters(t *testing.T) {
	for _, model := range []string{"k3.1-pro", "k3.1-pro-highspeed"} {
		for _, requested := range []string{model, "public-kimi"} {
			t.Run(model+"/"+requested, func(t *testing.T) {
				account := rawGPT56ResponsesAPIKeyAccount(requested, model)
				body := map[string]any{"model": requested, "reasoning_effort": "none", "reasoning": map[string]any{"effort": "none"}}
				payload, err := json.Marshal(body)
				require.NoError(t, err)
				filtered, err := filterOpenAIResponsesNoneReasoningEffortForAccount(account, payload)
				require.NoError(t, err)
				require.JSONEq(t, string(payload), string(filtered))
				deleteOpenAIResponsesNoneReasoningEffortFromObject(account, body)
				filtered, err = json.Marshal(body)
				require.NoError(t, err)
				require.JSONEq(t, string(payload), string(filtered))
			})
		}
	}
}
