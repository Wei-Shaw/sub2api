//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const minimaxM31TestModel = "MiniMax-M3.1-Flash-Preview"

func minimaxM31TestBody(model, protocol, effort, thinking string, stream bool) []byte {
	body := map[string]any{"model": model, "stream": stream}
	switch protocol {
	case APIProtocolResponses:
		body["input"] = "hi"
		if effort != "" {
			body["reasoning"] = map[string]any{"effort": effort}
		}
	default:
		body["messages"] = []map[string]any{{"role": "user", "content": "hi"}}
		if thinking != "" {
			body["thinking"] = map[string]any{"type": thinking}
		}
		if protocol == APIProtocolAnthropic {
			body["max_tokens"] = 16384
			if effort != "" {
				body["output_config"] = map[string]any{"effort": effort}
			}
		} else if effort != "" {
			body["reasoning_effort"] = effort
		}
	}
	encoded, _ := json.Marshal(body)
	return encoded
}

func minimaxM31TestForward(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte, protocol string) (*OpenAIForwardResult, error) {
	switch protocol {
	case APIProtocolAnthropic:
		return svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
	case APIProtocolChatCompletions:
		return svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	default:
		return svc.Forward(context.Background(), c, account, body)
	}
}

func minimaxM31TestResponse(protocol, inbound string, stream bool) *http.Response {
	if protocol == APIProtocolAnthropic {
		if stream || inbound != APIProtocolAnthropic {
			return nativeAnthropicStreamResponse()
		}
		return nativeAnthropicBufferedResponse()
	}
	contentType := "text/event-stream"
	var body string
	if protocol == APIProtocolChatCompletions {
		if stream {
			body = "data: " + `{"id":"chat_1","choices":[{"index":0,"delta":{"content":"pong"}}]}` + "\n\ndata: " + `{"id":"chat_1","choices":[],"usage":{"prompt_tokens":9,"completion_tokens":3,"total_tokens":12}}` + "\n\ndata: [DONE]\n\n"
		} else {
			contentType = "application/json"
			body = `{"id":"chat_1","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":3,"total_tokens":12}}`
		}
	} else {
		response := `{"id":"resp_1","object":"response","model":"MiniMax-M3.1-Flash-Preview","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"pong"}]}],"usage":{"input_tokens":9,"output_tokens":3,"total_tokens":12}}`
		if !stream && inbound == APIProtocolResponses {
			contentType = "application/json"
			body = response
		} else {
			body = "event: response.completed\ndata: " + `{"type":"response.completed","response":` + response + "}\n\n"
		}
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestMiniMaxM31ReasoningProtocolMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	protocols := []string{APIProtocolAnthropic, APIProtocolChatCompletions, APIProtocolResponses}
	effortPaths := map[string]string{APIProtocolAnthropic: "output_config.effort", APIProtocolChatCompletions: "reasoning_effort", APIProtocolResponses: "reasoning.effort"}
	paths := map[string]string{APIProtocolAnthropic: "/v1/messages", APIProtocolChatCompletions: "/v1/chat/completions", APIProtocolResponses: "/v1/responses"}
	for _, inbound := range protocols {
		for _, outbound := range protocols {
			for _, effort := range []string{"", "low", "medium", "high", "xhigh", "max"} {
				for _, stream := range []bool{false, true} {
					for _, model := range []string{minimaxM31TestModel, "client-alias", "my-high", "my-max"} {
						t.Run(fmt.Sprintf("%s/%s/%s/stream=%t/%s", inbound, outbound, effort, stream, model), func(t *testing.T) {
							body := minimaxM31TestBody(model, inbound, effort, "", stream)
							account := nativeAnthropicTestAccount()
							account.Platform = PlatformMiniMax
							account.Credentials["api_protocol"] = outbound
							account.Credentials["base_url"] = "http://minimax.example"
							if model != minimaxM31TestModel {
								account.Credentials["model_mapping"] = map[string]any{model: minimaxM31TestModel}
							}
							upstream := &httpUpstreamRecorder{resp: minimaxM31TestResponse(outbound, inbound, stream)}
							svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
							result, err := minimaxM31TestForward(svc, adaptiveProtocolTestContext(paths[inbound], body), account, body, inbound)
							require.NoError(t, err)
							require.NotNil(t, upstream.lastReq)
							require.Equal(t, paths[outbound], upstream.lastReq.URL.Path)
							require.Equal(t, minimaxM31TestModel, gjson.GetBytes(upstream.lastBody, "model").String())
							require.Equal(t, effort, gjson.GetBytes(upstream.lastBody, effortPaths[outbound]).String())
							if effort == "" {
								require.False(t, gjson.GetBytes(upstream.lastBody, effortPaths[outbound]).Exists(), "omission must survive conversion")
							}
							require.NotNil(t, result.ReasoningEffort)
							wantEffort := effort
							if wantEffort == "" {
								wantEffort = "max"
							}
							require.Equal(t, wantEffort, *result.ReasoningEffort)
						})
					}
				}
			}
		}
	}
}

func TestMiniMaxM31RejectsInvalidThinkingBeforeConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, inbound := range []string{APIProtocolAnthropic, APIProtocolChatCompletions, APIProtocolResponses} {
		for _, outbound := range []string{APIProtocolAnthropic, APIProtocolChatCompletions, APIProtocolResponses} {
			for _, preference := range []string{"disabled", "none", "auto"} {
				t.Run(inbound+"/"+outbound+"/"+preference, func(t *testing.T) {
					effort, thinking := preference, ""
					if preference == "disabled" {
						effort, thinking = "max", "disabled"
						if inbound == APIProtocolResponses {
							effort = "none"
						}
					}
					body := minimaxM31TestBody("client-alias", inbound, effort, thinking, false)
					account := nativeAnthropicTestAccount()
					account.Platform = PlatformMiniMax
					account.Credentials["api_protocol"] = outbound
					account.Credentials["model_mapping"] = map[string]any{"client-alias": minimaxM31TestModel}
					upstream := &httpUpstreamRecorder{err: errors.New("unexpected upstream request")}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					c := adaptiveProtocolTestContext("/v1/test", body)
					_, err := minimaxM31TestForward(svc, c, account, body, inbound)
					require.Error(t, err)
					require.Nil(t, upstream.lastReq)
					require.Equal(t, http.StatusBadRequest, c.Writer.Status())
				})
			}
		}
	}
}

func TestMiniMaxM31ConfiguredReasoningLevels(t *testing.T) {
	descriptor := newConfiguredCodexModelDescriptor(minimaxM31TestModel)
	require.Equal(t, "max", *descriptor.DefaultReasoningLevel)
	var efforts []string
	for _, level := range descriptor.SupportedReasoningLevels {
		efforts = append(efforts, level.Effort)
	}
	require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, efforts)
}

func TestMiniMaxM31MappingAwayUsesDestinationSemantics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, outbound := range []string{APIProtocolAnthropic, APIProtocolChatCompletions, APIProtocolResponses} {
		t.Run(outbound, func(t *testing.T) {
			body := minimaxM31TestBody(minimaxM31TestModel, APIProtocolAnthropic, "", "disabled", false)
			account := nativeAnthropicTestAccount()
			account.Platform = PlatformMiniMax
			account.Credentials["api_protocol"] = outbound
			account.Credentials["base_url"] = "http://minimax.example"
			account.Credentials["model_mapping"] = map[string]any{minimaxM31TestModel: "MiniMax-M3"}
			upstream := &httpUpstreamRecorder{resp: minimaxM31TestResponse(outbound, APIProtocolAnthropic, false)}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			_, err := minimaxM31TestForward(svc, adaptiveProtocolTestContext("/v1/messages", body), account, body, APIProtocolAnthropic)
			require.NoError(t, err)
			require.Equal(t, "MiniMax-M3", gjson.GetBytes(upstream.lastBody, "model").String())
		})
	}
}

func TestMiniMaxM31OlderModelsKeepExistingDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, model := range []string{"MiniMax-M3", "MiniMax-M2.7"} {
		for _, inbound := range []string{APIProtocolAnthropic, APIProtocolChatCompletions, APIProtocolResponses} {
			for _, outbound := range []string{APIProtocolAnthropic, APIProtocolChatCompletions, APIProtocolResponses} {
				t.Run(model+"/"+inbound+"/"+outbound, func(t *testing.T) {
					body := minimaxM31TestBody(model, inbound, "", "", false)
					account := nativeAnthropicTestAccount()
					account.Platform = PlatformMiniMax
					account.Credentials["api_protocol"] = outbound
					account.Credentials["base_url"] = "http://minimax.example"
					upstream := &httpUpstreamRecorder{resp: minimaxM31TestResponse(outbound, inbound, false)}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					result, err := minimaxM31TestForward(svc, adaptiveProtocolTestContext("/v1/test", body), account, body, inbound)
					require.NoError(t, err)
					if inbound == APIProtocolAnthropic && outbound != APIProtocolAnthropic {
						require.NotNil(t, result.ReasoningEffort)
						require.Equal(t, "medium", *result.ReasoningEffort)
					} else {
						require.Nil(t, result.ReasoningEffort)
					}
				})
			}
		}
	}
}

func TestMiniMaxM31NoneSurvivesPlaceholderFiltering(t *testing.T) {
	account := nativeAnthropicTestAccount()
	account.Platform = PlatformMiniMax
	account.Credentials["model_mapping"] = map[string]any{"client-alias": minimaxM31TestModel}
	body := minimaxM31TestBody("client-alias", APIProtocolResponses, "none", "", false)
	filtered, err := filterOpenAIResponsesNoneReasoningEffortForAccount(account, body)
	require.NoError(t, err)
	require.JSONEq(t, string(body), string(filtered))
	bridged, err := prepareOpenAIWSHTTPBridgeBody(account, body)
	require.NoError(t, err)
	require.Equal(t, "none", gjson.GetBytes(bridged, "reasoning.effort").String())
	require.Error(t, validateMiniMaxM31Request(bridged, minimaxM31TestModel))
}

func TestMiniMaxM31ReasoningUsageLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, effort := range []string{"", "low", "medium", "high", "xhigh", "max"} {
		t.Run(effort, func(t *testing.T) {
			body := minimaxM31TestBody(minimaxM31TestModel, APIProtocolChatCompletions, effort, "", false)
			account := nativeAnthropicTestAccount()
			account.Platform = PlatformMiniMax
			account.Credentials["api_protocol"] = APIProtocolChatCompletions
			account.Credentials["base_url"] = "http://minimax.example"
			upstream := &httpUpstreamRecorder{resp: minimaxM31TestResponse(APIProtocolChatCompletions, APIProtocolChatCompletions, false)}
			forwarder := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			result, err := minimaxM31TestForward(forwarder, adaptiveProtocolTestContext("/v1/chat/completions", body), account, body, APIProtocolChatCompletions)
			require.NoError(t, err)
			result.RequestedReasoningEffort = CanonicalRequestedReasoningEffort(body)
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			err = svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{Result: result, APIKey: &APIKey{ID: 1}, User: &User{ID: 2}, Account: account})
			require.NoError(t, err)
			wantEffort := effort
			if wantEffort == "" {
				wantEffort = "max"
			}
			require.NotNil(t, usageRepo.lastLog)
			require.Equal(t, wantEffort, *usageRepo.lastLog.ReasoningEffort)
			require.Equal(t, wantEffort, *usageRepo.lastLog.RequestedReasoningEffort)
		})
	}
}
