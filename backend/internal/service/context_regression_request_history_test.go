//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func labRequestGateway() (*OpenAIGatewayService, *httpUpstreamRecorder, *Account) {
	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}}}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-offline-fixture", "base_url": "https://fixture.invalid/v1"},
		Extra:       map[string]any{"openai_responses_mode": "force_responses", "openai_responses_supported": false},
	}
	return svc, upstream, account
}

func labRequestForward(t *testing.T, svc *OpenAIGatewayService, account *Account, body []byte, key string) {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, key, "")
	require.NoError(t, err)
	require.NotNil(t, result)
}

func labRequestHistoryBody(t *testing.T, model string, cycles int) []byte {
	t.Helper()
	msgs := []apicompat.AnthropicMessage{{Role: "user", Content: json.RawMessage(`"TASK_ROOT_4ad19: inspect every file, repair all faults, then report. Continue until finished."`)}}
	for i := 0; i < cycles; i++ {
		id := fmt.Sprintf("call_%d", i)
		msgs = append(msgs, apicompat.AnthropicMessage{Role: "assistant", Content: json.RawMessage(fmt.Sprintf(`[{"type":"tool_use","id":%q,"name":"read_file","input":{"path":"file_%d.go"}}]`, id, i))})
		content := fmt.Sprintf(`[{"type":"tool_result","tool_use_id":%q,"content":"file content %d"}]`, id, i)
		if i == cycles-1 {
			content = fmt.Sprintf(`[{"type":"tool_result","tool_use_id":%q,"content":"file content %d"},{"type":"text","text":"LATEST_TASK_b925: also fix the latest failing test now."}]`, id, i)
		}
		msgs = append(msgs, apicompat.AnthropicMessage{Role: "user", Content: json.RawMessage(content)})
	}
	body, err := json.Marshal(&apicompat.AnthropicRequest{Model: model, MaxTokens: 256, Messages: msgs, Tools: []apicompat.AnthropicTool{{Name: "read_file", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)}}})
	require.NoError(t, err)
	return body
}

func labRequestSave(t *testing.T, model string, l1, l2 []byte) {
	t.Helper()
	root := os.Getenv("LAB_REQUEST_ARTIFACT_DIR")
	if root == "" {
		return
	}
	dir := filepath.Join(root, strings.ReplaceAll(t.Name(), "/", "_"), model)
	require.NoError(t, os.MkdirAll(dir, 0755))
	for name, body := range map[string][]byte{"L1.anthropic.request.json": l1, "L2.responses.request.json": l2} {
		var formatted bytes.Buffer
		require.NoError(t, json.Indent(&formatted, body, "", "  "))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), formatted.Bytes(), 0644))
	}
}

func TestLabRequestStatelessFullHistory(t *testing.T) {
	for _, model := range []string{"gpt-6.1-sol", "gpt-6-sol"} {
		t.Run(model, func(t *testing.T) {
			svc, upstream, account := labRequestGateway()
			body := labRequestHistoryBody(t, model, 7)
			upstream.resp = openAICompatSSECompletedResponse("resp_fixture", model)
			labRequestForward(t, svc, account, body, "")
			labRequestSave(t, model, body, upstream.lastBody)
			require.Equal(t, "/v1/responses", upstream.lastReq.URL.Path)
			require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String())
			require.False(t, gjson.GetBytes(upstream.lastBody, "store").Bool())
			require.False(t, gjson.GetBytes(upstream.lastBody, "previous_response_id").Exists())
			require.Contains(t, string(upstream.lastBody), "LATEST_TASK_b925")
			require.Contains(t, string(upstream.lastBody), "TASK_ROOT_4ad19", "stateless replay must retain the unfinished root task")
			require.Contains(t, string(upstream.lastBody), `"call_id":"call_0"`)
		})
	}
}

func TestLabRequestExpiredContinuationFullHistory(t *testing.T) {
	for _, model := range []string{"gpt-6.1-sol", "gpt-6-sol"} {
		t.Run(model, func(t *testing.T) {
			svc, upstream, account := labRequestGateway()
			body := labRequestHistoryBody(t, model, 7)
			svc.bindOpenAICompatSessionResponseID(context.Background(), nil, account, "session-fixture", "resp_expired")
			if openAICompatContinuationEnabled(account, model) {
				upstream.responses = []*http.Response{{StatusCode: 404, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"previous_response_not_found","message":"Previous response not found"}}`))}, openAICompatSSECompletedResponse("resp_restored", model)}
			} else {
				upstream.resp = openAICompatSSECompletedResponse("resp_restored", model)
			}
			labRequestForward(t, svc, account, body, "session-fixture")
			labRequestSave(t, model, body, upstream.lastBody)
			require.False(t, gjson.GetBytes(upstream.lastBody, "previous_response_id").Exists())
			require.Contains(t, string(upstream.lastBody), "LATEST_TASK_b925")
			require.Contains(t, string(upstream.lastBody), "TASK_ROOT_4ad19", "missing response state fallback must replay the entire request")
		})
	}
}

func TestLabRequestMixedLatestTurnAndStateIsolation(t *testing.T) {
	svc, _, account := labRequestGateway()
	for _, model := range []string{"gpt-6.1-sol", "gpt-6-sol"} {
		var anthropic apicompat.AnthropicRequest
		require.NoError(t, json.Unmarshal(labRequestHistoryBody(t, model, 7), &anthropic))
		req, err := apicompat.AnthropicToResponses(&anthropic)
		require.NoError(t, err)
		trimAnthropicCompatResponsesInputToLatestTurn(req)
		require.Contains(t, string(req.Input), "LATEST_TASK_b925")
		require.Contains(t, string(req.Input), `"type":"function_call","call_id":"call_6"`)
		require.Contains(t, string(req.Input), `"type":"function_call_output","call_id":"call_6"`)
		require.NotContains(t, string(req.Input), `"call_id":"call_5"`)
	}
	svc.bindOpenAICompatSessionResponseID(context.Background(), nil, account, "session-a", "resp_a")
	require.Empty(t, svc.getOpenAICompatSessionResponseID(context.Background(), nil, account, "session-b"))
	other := *account
	other.ID = 2
	require.Empty(t, svc.getOpenAICompatSessionResponseID(context.Background(), nil, &other, "session-a"))
}

func TestLabRequestDigestForkMustNotInheritSiblingState(t *testing.T) {
	for _, model := range []string{"gpt-6.1-sol", "gpt-6-sol"} {
		t.Run(model, func(t *testing.T) {
			svc, upstream, account := labRequestGateway()
			first := []byte(fmt.Sprintf(`{"model":%q,"max_tokens":256,"messages":[{"role":"user","content":"same initial task"}]}`, model))
			upstream.resp = openAICompatSSECompletedResponse("resp_sibling_a", model)
			labRequestForward(t, svc, account, first, "")
			branch := []byte(fmt.Sprintf(`{"model":%q,"max_tokens":256,"messages":[{"role":"user","content":"same initial task"},{"role":"assistant","content":"different branch"},{"role":"user","content":"branch b task"}]}`, model))
			upstream.resp = openAICompatSSECompletedResponse("resp_branch_b", model)
			labRequestForward(t, svc, account, branch, "")
			labRequestSave(t, model, branch, upstream.lastBody)
			require.False(t, gjson.GetBytes(upstream.lastBody, "previous_response_id").Exists(), "content digest cannot prove server state is this branch's parent")
			require.Contains(t, string(upstream.lastBody), "different branch")
		})
	}
}

func TestLabRequestHistoryBoundary(t *testing.T) {
	for _, model := range []string{"gpt-6.1-sol", "gpt-6-sol"} {
		for _, count := range []int{11, 12, 13, 14} {
			t.Run(fmt.Sprintf("%s_%d", model, count), func(t *testing.T) {
				svc, upstream, account := labRequestGateway()
				messages := []apicompat.AnthropicMessage{{Role: "user", Content: json.RawMessage(`"ROOT_BOUNDARY_TASK: finish all required work before stopping."`)}}
				for i := 1; i < count; i++ {
					role := "user"
					if i%2 == 1 {
						role = "assistant"
					}
					messages = append(messages, apicompat.AnthropicMessage{Role: role, Content: json.RawMessage(fmt.Sprintf(`"checkpoint-%d"`, i))})
				}
				body, err := json.Marshal(&apicompat.AnthropicRequest{Model: model, MaxTokens: 256, Messages: messages})
				require.NoError(t, err)
				upstream.resp = openAICompatSSECompletedResponse("resp_boundary", model)
				labRequestForward(t, svc, account, body, "")
				labRequestSave(t, model, body, upstream.lastBody)
				require.Contains(t, string(upstream.lastBody), "ROOT_BOUNDARY_TASK")
				require.Contains(t, string(upstream.lastBody), fmt.Sprintf("checkpoint-%d", count-1))
			})
		}
	}
}
