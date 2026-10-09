package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func poolOpenAIAccount(id int64, groupID int64, priority int) *service.Account {
	return &service.Account{
		ID: id, Name: fmt.Sprintf("openai-%d", id), Platform: service.PlatformOpenAI,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
		Concurrency: 1, Priority: priority,
		AccountGroups: []service.AccountGroup{{AccountID: id, GroupID: groupID}},
		Credentials: map[string]any{
			"api_key":       "openai-key",
			"base_url":      "https://openai.example",
			"model_mapping": map[string]any{"my-model": "gpt-5.4"},
		},
	}
}

// openAIResponsesSSEOKResponse 是最小的 Responses 流式成功响应（openai 平台委派链以流式访问上游）。
func openAIResponsesSSEOKResponse(model string) *http.Response {
	body := "data: " + fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_pool_sse","object":"response","status":"completed","model":%q,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}],"usage":{"input_tokens":5,"output_tokens":3}}}`, model) + "\n\n"
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// 跨族账号池（generic handler）选中 openai 账号时同样受分组推理强度上限约束，
// 与纯 OpenAI 族池 / 单目标 openai 同语义。
func TestCompositeCrossFamilyPoolAppliesReasoningEffortCap(t *testing.T) {
	cases := []struct {
		name, path, body, effortPath string
		respond                      func() *http.Response
	}{
		{
			name:       "responses",
			path:       "/v1/responses",
			body:       `{"model":"my-model","stream":false,"input":"hello","reasoning":{"effort":"xhigh"}}`,
			effortPath: "reasoning.effort",
			respond:    func() *http.Response { return openAIResponsesOKResponse("gpt-5.4", 5, 3) },
		},
		{
			name:       "chat_completions",
			path:       "/v1/chat/completions",
			body:       `{"model":"my-model","stream":false,"messages":[{"role":"user","content":"hello"}],"reasoning_effort":"xhigh"}`,
			effortPath: "reasoning_effort",
			respond:    func() *http.Response { return openAIResponsesSSEOKResponse("gpt-5.4") },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newCompositePoolMessagesHarness(t, []*service.Account{poolOpenAIAccount(44101, 41011, 0)})
			h.group.MaxReasoningEffort = "medium"
			h.openAIUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response { return tc.respond() }

			c, rec := h.newRequest(t, tc.path, tc.body, service.PlatformAnthropic, service.PlatformOpenAI)
			if tc.path == "/v1/responses" {
				h.handler.Responses(c)
			} else {
				h.handler.ChatCompletions(c)
			}

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			calls := h.openAIUpstream.snapshot()
			require.Len(t, calls, 1)
			effort := gjson.GetBytes(calls[0].Body, "reasoning.effort").String()
			if effort == "" {
				effort = gjson.GetBytes(calls[0].Body, "reasoning_effort").String()
			}
			require.Equal(t, "medium", effort, "upstream body=%s", calls[0].Body)
		})
	}
}

// Messages 入口：委派 OpenAI 网关链时按分组上限压低 output_config.effort。
func TestCompositeCrossFamilyPoolMessagesAppliesReasoningEffortCap(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{poolOpenAIAccount(44201, 41011, 0)})
	h.group.MaxReasoningEffort = "medium"
	h.openAIUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response {
		return openAIResponsesSSEOKResponse("gpt-5.4")
	}

	c, rec := h.newRequest(t, "/v1/messages",
		`{"model":"my-model","max_tokens":64,"stream":false,"messages":[{"role":"user","content":"hello"}],"output_config":{"effort":"max"}}`,
		service.PlatformAnthropic, service.PlatformOpenAI)
	h.handler.Messages(c)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	calls := h.openAIUpstream.snapshot()
	require.Len(t, calls, 1)
	require.Equal(t, "medium", gjson.GetBytes(calls[0].Body, "reasoning.effort").String(), "upstream body=%s", calls[0].Body)
}
