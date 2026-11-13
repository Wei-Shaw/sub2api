//go:build unit

package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

const chatRetryCreated = `data: {"type":"response.created","response":{"id":"resp_retry","model":"gpt-6-astra","status":"in_progress","output":[]}}` + "\n\n"
const chatRetryOverload = `data: {"type":"response.failed","response":{"status":"failed","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}` + "\n\n"
const chatRetrySuccess = `data: {"type":"response.output_text.delta","delta":"recovered"}` + "\n\n" +
	`data: {"type":"response.completed","response":{"id":"resp_retry","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}}` + "\n\n"

type chatPreambleRetryUpstream struct {
	service.HTTPUpstream
	bodies     []string
	accountIDs []int64
	onFirst    func()
}

func (u *chatPreambleRetryUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.accountIDs = append(u.accountIDs, accountID)
	if len(u.accountIDs) == 1 && u.onFirst != nil {
		u.onFirst()
	}
	idx := len(u.accountIDs) - 1
	if idx >= len(u.bodies) {
		idx = len(u.bodies) - 1
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(u.bodies[idx])),
	}, nil
}

func TestChatPreambleRetry(t *testing.T) {
	for _, tc := range []struct {
		name         string
		first        string
		wantAttempts int
		wantStatus   int
		wantText     string
		cancel       bool
	}{
		{name: "overload before content retries then succeeds", first: chatRetryCreated + chatRetryOverload, wantAttempts: 2, wantStatus: 200, wantText: "recovered"},
		{name: "bare overload before content retries", first: chatRetryCreated + `data: {"type":"error","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}` + "\n\n", wantAttempts: 2, wantStatus: 200, wantText: "recovered"},
		{name: "text already emitted is not replayed", first: chatRetryCreated + `data: {"type":"response.output_text.delta","delta":"partial"}` + "\n\n" + chatRetryOverload, wantAttempts: 1, wantStatus: 200, wantText: "partial"},
		{name: "reasoning already emitted is not replayed", first: chatRetryCreated + `data: {"type":"response.reasoning_summary_text.delta","delta":"thinking"}` + "\n\n" + chatRetryOverload, wantAttempts: 1, wantStatus: 200, wantText: "thinking"},
		{name: "tool call already emitted is not replayed", first: chatRetryCreated + `data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":""}}` + "\n\n" + chatRetryOverload, wantAttempts: 1, wantStatus: 200, wantText: "call_1"},
		{name: "invalid input is not retried", first: chatRetryCreated + `data: {"type":"response.failed","response":{"status":"failed","error":{"code":"context_length_exceeded","message":"input exceeds the context window"}}}` + "\n\n", wantAttempts: 1, wantStatus: 502, wantText: "context window"},
		{name: "canceled client is not retried", first: chatRetryCreated + chatRetryOverload, wantAttempts: 1, cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			upstream := &chatPreambleRetryUpstream{bodies: []string{tc.first, chatRetryCreated + chatRetrySuccess}}
			if tc.cancel {
				upstream.onFirst = cancel
			}
			h := newOpenAIResponsesFailoverTestHandler(t, upstream)
			c, rec := newOpenAIResponsesFailoverTestContext(t, ctx)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-6-astra","messages":[{"role":"user","content":"hello"}],"stream":true}`)).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			h.ChatCompletions(c)
			require.Len(t, upstream.accountIDs, tc.wantAttempts)
			if tc.cancel {
				require.Empty(t, rec.Body.String())
				return
			}
			require.Equal(t, tc.wantStatus, rec.Code)
			require.Contains(t, rec.Body.String(), tc.wantText)
			if tc.wantAttempts > 1 {
				require.Equal(t, upstream.accountIDs[0], upstream.accountIDs[1], "capacity retries must stay on the same account first")
				require.NotContains(t, rec.Body.String(), "overloaded")
				require.Equal(t, 1, strings.Count(rec.Body.String(), `"role":"assistant"`))
				require.Equal(t, 1, strings.Count(rec.Body.String(), "data: [DONE]"))
			} else {
				require.NotContains(t, rec.Body.String(), "recovered")
			}
		})
	}
}

func TestChatPreambleRetryExhausted(t *testing.T) {
	upstream := &chatPreambleRetryUpstream{bodies: []string{chatRetryCreated + chatRetryOverload}}
	h := newOpenAIResponsesFailoverTestHandler(t, upstream)
	h.maxAccountSwitches = 0
	c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-6-astra","messages":[{"role":"user","content":"hello"}],"stream":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ChatCompletions(c)
	require.Len(t, upstream.accountIDs, 4, "initial attempt plus three bounded retries")
	require.GreaterOrEqual(t, rec.Code, 500)
	require.Contains(t, rec.Body.String(), "overloaded")
	require.NotContains(t, rec.Body.String(), `"role":"assistant"`)
	require.NotContains(t, rec.Body.String(), "[DONE]")
}
