//go:build unit

package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 普通分组（ProfitControlEnabled=false）的 handler 级回归：真实 OpenAI
// /v1/responses 与 /v1/chat/completions + 真实 failover 循环，出站在 HTTPUpstream
// 边界脚本化。账号、会话和响应体均为合成数据。
//
// 契约：可迁移 HTTP 会话的候选必须跟着**确认成功**的账号走；只被选中、随后
// 失败的账号不得夺走它。旧 sticky 键仍按官方 eager 语义维护。

// normalGroupEagerSessionBinding 返回 7 号分组下那条**会话**旧 sticky 绑定，
// 跳过 response / http-response-owner 这两类既有的响应归属键。
func normalGroupEagerSessionBinding(c *stickyGatewayTestCache) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, accountID := range c.bindings {
		if !strings.HasPrefix(key, "binding:7:openai:") ||
			strings.HasPrefix(key, "binding:7:openai:response:") ||
			strings.HasPrefix(key, "binding:7:openai:http-response-owner:") {
			continue
		}
		return accountID, true
	}
	return 0, false
}

type openAINormalGroupStickyCase struct{ name, path, body, response string }

func openAINormalGroupStickyCases() []openAINormalGroupStickyCase {
	return []openAINormalGroupStickyCase{
		{
			"responses", "/v1/responses",
			`{"model":"gpt-test","input":"hello","stream":false}`,
			`{"id":"resp_test","status":"completed","model":"gpt-test","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`,
		},
		{
			"chat", "/v1/chat/completions",
			`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}],"stream":false}`,
			"",
		},
	}
}

func (tc openAINormalGroupStickyCase) okScript() stickyScript {
	if tc.name == "chat" {
		return stickyScript{statusCode: http.StatusOK, streamBody: "data: " +
			`{"type":"response.completed","response":{"id":"resp_test","status":"completed","model":"gpt-test","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"}
	}
	return stickyScript{statusCode: http.StatusOK, jsonBody: tc.response}
}

// A 上游失败、failover 到 B 成功后，下一个同会话请求必须直接从 B 开始；
// 旧 sticky 键仍由官方 eager 绑定维护。
func TestOpenAILegacyStickySuccessNormalGroupHTTPFailover(t *testing.T) {
	for _, tc := range openAINormalGroupStickyCases() {
		t.Run(tc.name, func(t *testing.T) {
			ok := tc.okScript()
			upstream := &stickyUpstream{scripts: map[int64][]stickyScript{
				1: {stickyFailScript(), ok, ok},
				2: {ok, ok, ok},
			}}
			cache := newStickyGatewayTestCache()
			router := newOpenAILegacySuccessFixture(t, upstream, cache, false)

			require.Equal(t, http.StatusOK, requestOpenAILegacySuccess(router, tc.path, tc.body).Code)
			require.Equal(t, []int64{1, 2}, upstream.hitOrder(), "首次请求 A 失败后 failover 到 B")
			require.EqualValues(t, 2, cache.preference(t).AccountID, "只有真正成功的账号才写成功偏好")

			upstream.resetHits()
			require.Equal(t, http.StatusOK, requestOpenAILegacySuccess(router, tc.path, tc.body).Code)
			require.Equal(t, []int64{2}, upstream.hitOrder(), "下一个同会话请求首跳必须是 failover 成功的 B")

			// 普通分组只接管候选来源：旧 sticky 键仍被 eager 写入。
			accountID, bound := normalGroupEagerSessionBinding(cache)
			require.True(t, bound, "普通分组的旧键写入不得被成功偏好接管")
			require.EqualValues(t, 2, accountID, "旧 sticky 键仍按官方 eager 语义维护")
		})
	}
}

// 只被选中、随后失败的账号不得夺走候选归属：A 成功过之后，一次两个账号都失败
// 的请求结束后，再下一个请求仍必须回到确认成功过的 A。
func TestOpenAILegacyStickySuccessNormalGroupFailedAttemptHTTP(t *testing.T) {
	for _, tc := range openAINormalGroupStickyCases() {
		t.Run(tc.name, func(t *testing.T) {
			ok := tc.okScript()
			fail := stickyFailScript()
			upstream := &stickyUpstream{scripts: map[int64][]stickyScript{
				1: {ok, fail, ok},
				2: {fail, fail, fail},
			}}
			cache := newStickyGatewayTestCache()
			router := newOpenAILegacySuccessFixture(t, upstream, cache, false)

			// 请求 1：A 直接成功，成为这条会话确认成功过的账号。
			require.Equal(t, http.StatusOK, requestOpenAILegacySuccess(router, tc.path, tc.body).Code)
			require.Equal(t, []int64{1}, upstream.hitOrder())
			require.EqualValues(t, 1, cache.preference(t).AccountID)

			// 请求 2：A、B 全部失败，整条请求失败，不得提交任何成功偏好。
			upstream.resetHits()
			require.NotEqual(t, http.StatusOK, requestOpenAILegacySuccess(router, tc.path, tc.body).Code)
			require.Equal(t, []int64{1, 2}, upstream.hitOrder())
			require.EqualValues(t, 1, cache.preference(t).AccountID, "全失败不得改写成功偏好")

			// 请求 3：候选归属仍属于上一次真正成功的 A。
			upstream.resetHits()
			require.Equal(t, http.StatusOK, requestOpenAILegacySuccess(router, tc.path, tc.body).Code)
			require.Equal(t, []int64{1}, upstream.hitOrder(),
				"只被选中、随后失败的 B 不得夺走候选归属")
		})
	}
}

// 普通分组全部候选失败时不得留下会被反复续命的成功偏好。
func TestOpenAILegacyStickySuccessNormalGroupHTTPAllFail(t *testing.T) {
	upstream := &stickyUpstream{scripts: map[int64][]stickyScript{
		1: {stickyFailScript()},
		2: {stickyFailScript()},
	}}
	cache := newStickyGatewayTestCache()
	router := newOpenAILegacySuccessFixture(t, upstream, cache, false)

	rec := requestOpenAILegacySuccess(router, "/v1/responses", `{"model":"gpt-test","input":"hello"}`)
	require.GreaterOrEqual(t, rec.Code, 400)
	require.Equal(t, []int64{1, 2}, upstream.hitOrder())
	require.Zero(t, cache.preferenceCount(), "全部候选失败不得留下成功偏好")
}
