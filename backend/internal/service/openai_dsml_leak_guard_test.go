//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
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

// 所有 fixture 均为按已核验形态合成的样本（生产捕获含用户内容，禁止入库）。

func dsmlGuardTestConfig() *config.Config {
	cfg := rawChatCompletionsTestConfig()
	cfg.Gateway.DSMLGuardEnabled = true
	cfg.Gateway.DSMLGuardModels = []string{"deepseek-v4"}
	cfg.Gateway.DSMLGuardMaxRetries = 1
	return cfg
}

func dsmlLeakOpsEvents(c *gin.Context) []*OpsUpstreamErrorEvent {
	v, ok := c.Get(OpsUpstreamErrorsKey)
	if !ok {
		return nil
	}
	events, _ := v.([]*OpsUpstreamErrorEvent)
	var out []*OpsUpstreamErrorEvent
	for _, ev := range events {
		if ev != nil && ev.Kind == dsmlGuardOpsKind {
			out = append(out, ev)
		}
	}
	return out
}

func dsmlStreamRequestBody() []byte {
	return []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"t"}}],"stream":true}`)
}

// runDSMLStreamFixture 起一条流式 raw CC 请求并返回客户端字节与上游记录。
func runDSMLStreamFixture(t *testing.T, cfg *config.Config, reqBody []byte, upstreamResponses ...*http.Response) (*httptest.ResponseRecorder, *httpUpstreamRecorder, *gin.Context, *OpenAIForwardResult, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(reqBody)))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{}
	if len(upstreamResponses) == 1 {
		upstream.resp = upstreamResponses[0]
	} else {
		upstream.responses = upstreamResponses
	}

	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	result, err := svc.forwardAsRawChatCompletions(context.Background(), c, rawChatCompletionsTestAccount(), reqBody, "")
	return rec, upstream, c, result, err
}

func dsmlSSEResponse(requestID string, lines ...string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{requestID}},
		Body:       io.NopCloser(strings.NewReader(strings.Join(lines, "\n") + "\n")),
	}
}

// dsmlLeakAttemptLines 复刻已核验的全漏形态：reasoning 正常直播 → content 以
// <thinking> 起头、参数体+闭合标签当正文流出 → finish_reason:"stop"，全程无 tool_calls。
func dsmlLeakAttemptLines(id string, promptTokens, completionTokens int) []string {
	return []string{
		`data: {"id":"` + id + `","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		``,
		`data: {"id":"` + id + `","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"reasoning_content":"let me think"},"finish_reason":null}]}`,
		``,
		`data: {"id":"` + id + `","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"<thinking>use weather tool"},"finish_reason":null}]}`,
		``,
		`data: {"id":"` + id + `","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"city=SEA</｜DSML｜tool_calls>"},"finish_reason":null}]}`,
		``,
		`data: {"id":"` + id + `","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: {"id":"` + id + `","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[],"usage":{"prompt_tokens":` + itoa(promptTokens) + `,"completion_tokens":` + itoa(completionTokens) + `,"total_tokens":` + itoa(promptTokens+completionTokens) + `}}`,
		``,
		`data: [DONE]`,
		``,
	}
}

func dsmlCleanAttemptLines(id string, promptTokens, completionTokens int) []string {
	return []string{
		`data: {"id":"` + id + `","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		``,
		`data: {"id":"` + id + `","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"reasoning_content":"second pass"},"finish_reason":null}]}`,
		``,
		`data: {"id":"` + id + `","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"the weather in SEA is cloudy"},"finish_reason":null}]}`,
		``,
		`data: {"id":"` + id + `","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: {"id":"` + id + `","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[],"usage":{"prompt_tokens":` + itoa(promptTokens) + `,"completion_tokens":` + itoa(completionTokens) + `,"total_tokens":` + itoa(promptTokens+completionTokens) + `}}`,
		``,
		`data: [DONE]`,
		``,
	}
}

func itoa(v int) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// ① 全漏 → 同账号重试拿到干净流，客户端字节序列正确。
func TestDSMLGuard_LeakRetriesOnSameAccountAndHealsStream(t *testing.T) {
	rec, upstream, c, result, err := runDSMLStreamFixture(t, dsmlGuardTestConfig(), dsmlStreamRequestBody(),
		dsmlSSEResponse("rid_leak_1", dsmlLeakAttemptLines("a1", 11, 7)...),
		dsmlSSEResponse("rid_clean_2", dsmlCleanAttemptLines("a2", 13, 9)...),
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	body := rec.Body.String()
	// 坏 attempt 的正文一个字节都不该到客户端。
	require.NotContains(t, body, "｜DSML｜")
	require.NotContains(t, body, "<thinking>")
	// attempt-1 的 reasoning 已直播（扣的只是正文通道）。
	require.Contains(t, body, `"reasoning_content":"let me think"`)
	// attempt-2 完整续写进同一条 SSE。
	require.Contains(t, body, `"reasoning_content":"second pass"`)
	require.Contains(t, body, "the weather in SEA is cloudy")
	// 客户端只收到最后一次 attempt 的 usage 与一个 [DONE]。
	require.Equal(t, 1, strings.Count(body, `"prompt_tokens"`))
	require.Contains(t, body, `"prompt_tokens":13`)
	require.Equal(t, 1, strings.Count(body, "data: [DONE]"))

	// 计费口径：最后一次 attempt。
	require.Equal(t, 13, result.Usage.InputTokens)
	require.Equal(t, 9, result.Usage.OutputTokens)

	// 同账号同请求体重发。
	require.Len(t, upstream.requests, 2)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, string(upstream.bodies[0]), string(upstream.bodies[1]))

	events := dsmlLeakOpsEvents(c)
	require.NotEmpty(t, events)
	foundRetry := false
	for _, ev := range events {
		if strings.Contains(ev.Message, "retried on same account") {
			foundRetry = true
		}
	}
	require.True(t, foundRetry, "expected a dsml_leak retry ops event, got: %+v", events)
}

// ①b 重试后 response-model 观察按 attempt 重置：坏 attempt 声明的模型串不得
// 让 conflict 锁存（否则 response_model 计费静默降级 baseline），最终观察到的
// 模型应来自干净 attempt。
func TestDSMLGuard_RetryResetsUpstreamResponseModelObservation(t *testing.T) {
	leakLines := dsmlLeakAttemptLines("a1", 11, 7)
	for i, line := range leakLines {
		leakLines[i] = strings.ReplaceAll(line, `"model":"deepseek-v4-flash"`, `"model":"deepseek-v4-flash-exp"`)
	}
	_, _, _, result, err := runDSMLStreamFixture(t, dsmlGuardTestConfig(), dsmlStreamRequestBody(),
		dsmlSSEResponse("rid_leak_1", leakLines...),
		dsmlSSEResponse("rid_clean_2", dsmlCleanAttemptLines("a2", 13, 9)...),
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, "deepseek-v4-flash", result.UpstreamResponseModel,
		"observed model must come from the clean attempt, not the discarded one")
	require.False(t, result.UpstreamResponseModelConflict,
		"discarded attempt's model declaration must not latch a billing conflict")
}

// ② 混合（标记与真 tool_calls 并存）→ 剔标记冲洗，不重试。
func TestDSMLGuard_MixedToolCallScrubsMarkerWithoutRetry(t *testing.T) {
	lines := []string{
		`data: {"id":"m1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		``,
		`data: {"id":"m1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"calling <｜DSML｜tool_calls>x=1</｜DSML｜tool_calls> now"},"finish_reason":null}]}`,
		``,
		`data: {"id":"m1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":""}}]},"finish_reason":null}]}`,
		``,
		`data: {"id":"m1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}
	rec, upstream, _, result, err := runDSMLStreamFixture(t, dsmlGuardTestConfig(), dsmlStreamRequestBody(),
		dsmlSSEResponse("rid_mixed", lines...))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 1, "mixed tool round must not retry")

	body := rec.Body.String()
	require.Contains(t, body, `"tool_calls"`)
	require.Contains(t, body, `"finish_reason":"tool_calls"`)
	require.NotContains(t, body, "｜DSML｜")
	require.Contains(t, body, "calling  now")
}

// ③ 干净长答案 → 128 字节后直通，字节与无防护完全一致（关键回归）。
func TestDSMLGuard_CleanLongAnswerBytesUnchanged(t *testing.T) {
	long := strings.Repeat("all good here ", 12) // ≥128 字节、无标记、无可疑前缀
	lines := []string{
		`data: {"id":"c1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		``,
		`data: {"id":"c1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"` + long + `"},"finish_reason":null}]}`,
		``,
		`data: {"id":"c1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"tail"},"finish_reason":null}]}`,
		``,
		`data: {"id":"c1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: {"id":"c1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`,
		``,
		`data: [DONE]`,
		``,
	}

	guardedRec, guardedUpstream, guardedCtx, result, err := runDSMLStreamFixture(t, dsmlGuardTestConfig(), dsmlStreamRequestBody(),
		dsmlSSEResponse("rid_clean", lines...))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, guardedUpstream.requests, 1)
	require.Empty(t, dsmlLeakOpsEvents(guardedCtx))

	plainRec, _, _, _, err := runDSMLStreamFixture(t, rawChatCompletionsTestConfig(), dsmlStreamRequestBody(),
		dsmlSSEResponse("rid_clean", lines...))
	require.NoError(t, err)
	require.Equal(t, plainRec.Body.String(), guardedRec.Body.String(), "guard must be byte-invisible on clean streams")
}

// ④ 两次全漏 → 耗尽，最后一次 attempt 原样放行，无错误帧。
func TestDSMLGuard_RetryExhaustedReleasesHeldOutputUnchanged(t *testing.T) {
	rec, upstream, c, result, err := runDSMLStreamFixture(t, dsmlGuardTestConfig(), dsmlStreamRequestBody(),
		dsmlSSEResponse("rid_leak_1", dsmlLeakAttemptLines("a1", 11, 7)...),
		dsmlSSEResponse("rid_leak_2", dsmlLeakAttemptLines("a2", 13, 9)...),
	)
	require.NoError(t, err, "exhausted retry must degrade, never error")
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)

	body := rec.Body.String()
	// 最后一次 attempt 的被扣输出原样放行（降级 = 今天的行为）。
	require.Equal(t, 1, strings.Count(body, "<thinking>use weather tool"))
	require.Contains(t, body, "</｜DSML｜tool_calls>")
	require.NotContains(t, body, `"error"`)
	require.Equal(t, 1, strings.Count(body, "data: [DONE]"))
	require.Equal(t, 13, result.Usage.InputTokens)
	require.Equal(t, 9, result.Usage.OutputTokens)

	events := dsmlLeakOpsEvents(c)
	foundExhausted := false
	for _, ev := range events {
		if strings.Contains(ev.Message, "persisted after") {
			foundExhausted = true
		}
	}
	require.True(t, foundExhausted, "expected an exhausted ops event, got: %+v", events)
}

// ⑤ observe 模式 → 只记录，字节零改动。
func TestDSMLGuard_ObserveModeDetectsWithoutChangingBytes(t *testing.T) {
	observeCfg := dsmlGuardTestConfig()
	observeCfg.Gateway.DSMLGuardObserve = true

	observedRec, observedUpstream, observedCtx, result, err := runDSMLStreamFixture(t, observeCfg, dsmlStreamRequestBody(),
		dsmlSSEResponse("rid_observe", dsmlLeakAttemptLines("a1", 11, 7)...))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, observedUpstream.requests, 1, "observe mode must not retry")

	plainRec, _, _, _, err := runDSMLStreamFixture(t, rawChatCompletionsTestConfig(), dsmlStreamRequestBody(),
		dsmlSSEResponse("rid_observe", dsmlLeakAttemptLines("a1", 11, 7)...))
	require.NoError(t, err)
	require.Equal(t, plainRec.Body.String(), observedRec.Body.String(), "observe mode must be byte-invisible")

	events := dsmlLeakOpsEvents(observedCtx)
	require.NotEmpty(t, events)
	foundObserve := false
	for _, ev := range events {
		if strings.Contains(ev.Message, "observe") {
			foundObserve = true
		}
	}
	require.True(t, foundObserve, "expected an observe-mode ops event, got: %+v", events)
}

// ⑥ 请求清洗：脏历史进 → 干净体出；user/tool 角色原样；observe 模式不改写。
func TestDSMLGuard_HistoryScrubStripsAssistantLeaks(t *testing.T) {
	reqBody := []byte(`{"model":"deepseek-v4-flash","stream":false,"messages":[{"role":"user","content":"why does ｜DSML｜ appear in my chat?"},{"role":"assistant","content":"<thinking>orphan params q=1</｜DSML｜tool_calls>"},{"role":"assistant","content":"before <｜DSML｜tool_calls>x=2</｜DSML｜tool_calls> after"},{"role":"tool","tool_call_id":"c1","content":"tool text ｜DSML｜ mention"}]}`)
	upstreamJSON := `{"id":"s1","object":"chat.completion","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	jsonResp := func() *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(upstreamJSON)),
		}
	}

	_, upstream, c, result, err := runDSMLStreamFixture(t, dsmlGuardTestConfig(), reqBody, jsonResp())
	require.NoError(t, err)
	require.NotNil(t, result)

	sent := upstream.lastBody
	messages := gjson.GetBytes(sent, "messages").Array()
	require.Len(t, messages, 4, "scrub must not add or drop messages")
	require.Equal(t, "why does ｜DSML｜ appear in my chat?", messages[0].Get("content").String(), "user content must stay untouched")
	require.Equal(t, "", messages[1].Get("content").String(), "fully-leaked assistant message keeps an empty string")
	require.Equal(t, "before  after", messages[2].Get("content").String())
	require.Equal(t, "tool text ｜DSML｜ mention", messages[3].Get("content").String(), "tool content must stay untouched")

	events := dsmlLeakOpsEvents(c)
	foundScrub := false
	for _, ev := range events {
		if strings.Contains(ev.Message, "dsml history scrub") {
			foundScrub = true
		}
	}
	require.True(t, foundScrub, "expected a history-scrub ops event, got: %+v", events)

	// observe 模式：请求体零改动。
	observeCfg := dsmlGuardTestConfig()
	observeCfg.Gateway.DSMLGuardObserve = true
	_, observeUpstream, observeCtx, _, err := runDSMLStreamFixture(t, observeCfg, reqBody, jsonResp())
	require.NoError(t, err)
	require.Equal(t, string(reqBody), string(observeUpstream.lastBody), "observe mode must not rewrite the request")
	require.NotEmpty(t, dsmlLeakOpsEvents(observeCtx), "observe mode still records the would-scrub event")
}

// ⑦ 非门控模型 / 无 tools / 非流式 → 检测器不激活，字节零改动。
func TestDSMLGuard_NotActivated(t *testing.T) {
	t.Run("other model", func(t *testing.T) {
		reqBody := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"t"}}],"stream":true}`)
		lines := dsmlLeakAttemptLines("n1", 3, 2)
		rec, _, c, _, err := runDSMLStreamFixture(t, dsmlGuardTestConfig(), reqBody, dsmlSSEResponse("rid_na1", lines...))
		require.NoError(t, err)
		require.Contains(t, rec.Body.String(), "</｜DSML｜tool_calls>", "non-gated model streams through unchanged")
		require.Empty(t, dsmlLeakOpsEvents(c))
	})

	t.Run("no tools", func(t *testing.T) {
		reqBody := []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"stream":true}`)
		lines := dsmlLeakAttemptLines("n2", 3, 2)
		rec, _, c, _, err := runDSMLStreamFixture(t, dsmlGuardTestConfig(), reqBody, dsmlSSEResponse("rid_na2", lines...))
		require.NoError(t, err)
		require.Contains(t, rec.Body.String(), "</｜DSML｜tool_calls>", "tool-less request streams through unchanged")
		require.Empty(t, dsmlLeakOpsEvents(c))
	})

	t.Run("non-streaming", func(t *testing.T) {
		reqBody := []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"t"}}],"stream":false}`)
		upstreamJSON := `{"id":"n3","object":"chat.completion","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"<thinking>leak</｜DSML｜tool_calls>"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
		rec, _, c, _, err := runDSMLStreamFixture(t, dsmlGuardTestConfig(), reqBody, &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(upstreamJSON)),
		})
		require.NoError(t, err)
		require.Contains(t, rec.Body.String(), "</｜DSML｜tool_calls>", "non-streaming responses are out of detector scope")
		require.Empty(t, dsmlLeakOpsEvents(c))
	})
}

// ⑧ 扣流中途上游断流 → 已扣内容冲洗放行（fail-open），无重试，无错误帧。
func TestDSMLGuard_MidHoldUpstreamAbortFlushesHeldLines(t *testing.T) {
	prefix := strings.Join([]string{
		`data: {"id":"x1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		``,
		`data: {"id":"x1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"<thinking>partial"},"finish_reason":null}]}`,
		``,
	}, "\n") + "\n"
	abortingBody := io.NopCloser(io.MultiReader(
		strings.NewReader(prefix),
		passthroughErrReadCloser{err: errors.New("upstream aborted mid-hold")},
	))

	rec, upstream, _, result, err := runDSMLStreamFixture(t, dsmlGuardTestConfig(), dsmlStreamRequestBody(), &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_abort"}},
		Body:       abortingBody,
	})
	require.NoError(t, err, "mid-hold abort must fail open, not error")
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 1, "no terminal frame means no leak verdict, so no retry")

	body := rec.Body.String()
	require.Contains(t, body, "<thinking>partial", "held lines must be flushed as-is on abort")
	require.NotContains(t, body, `"error"`)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
}

func TestScrubDSMLLeakFragments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		in          string
		want        string
		wantChanged bool
	}{
		{
			name:        "complete block removed",
			in:          "before <｜DSML｜tool_calls>x=1</｜DSML｜tool_calls> after",
			want:        "before  after",
			wantChanged: true,
		},
		{
			name:        "thinking head with orphan close tail stripped to empty",
			in:          "<thinking>params body q=1</｜DSML｜tool_calls>",
			want:        "",
			wantChanged: true,
		},
		{
			name:        "thinking head keeps trailing text after last close tag",
			in:          "<thinking>p</｜DSML｜tool_calls> trailing",
			want:        " trailing",
			wantChanged: true,
		},
		{
			name:        "orphan close tag cluster removed",
			in:          "text </｜DSML｜tool_calls></｜DSML｜end> tail",
			want:        "text  tail",
			wantChanged: true,
		},
		{
			name:        "double fullwidth bar spelling matched",
			in:          "a <｜｜DSML｜｜tool_calls>x</｜｜DSML｜｜tool_calls> b",
			want:        "a  b",
			wantChanged: true,
		},
		{
			name:        "no marker untouched",
			in:          "hello <thinking> world",
			want:        "hello <thinking> world",
			wantChanged: false,
		},
		{
			name:        "legit prose before late leak preserved",
			in:          "<thinking>plan</thinking>real answer <thinking>q=1</｜DSML｜tool_calls>",
			want:        "<thinking>plan</thinking>real answer ",
			wantChanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, changed := scrubDSMLLeakFragments(tt.in)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.wantChanged, changed)
		})
	}
}

func dsmlTestContentLine(text string) string {
	encoded, _ := json.Marshal(text)
	return `data: {"id":"t","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":` + string(encoded) + `},"finish_reason":null}]}`
}

const dsmlTestFinishLine = `data: {"id":"t","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`

func TestDSMLLeakGuard_HandleLineStateMachine(t *testing.T) {
	t.Parallel()

	t.Run("sniff releases long clean content", func(t *testing.T) {
		t.Parallel()
		g := newOpenAIDSMLLeakGuard(false)
		line := dsmlTestContentLine(strings.Repeat("a", 200))
		out := g.HandleLine(line)
		require.Equal(t, []string{line}, out, "clean ≥128B content releases immediately")
		require.False(t, g.Holding())
		require.False(t, g.LeakVerdict())
	})

	t.Run("thinking prefix holds past sniff window until clean terminal", func(t *testing.T) {
		t.Parallel()
		g := newOpenAIDSMLLeakGuard(false)
		line := dsmlTestContentLine("<thinking>" + strings.Repeat("b", 200))
		require.Empty(t, g.HandleLine(line), "suspicious prefix keeps holding past 128 bytes")
		require.True(t, g.Holding())
		out := g.HandleLine(dsmlTestFinishLine)
		require.Equal(t, []string{line, dsmlTestFinishLine}, out, "no marker by terminal → clean flush")
		require.False(t, g.LeakVerdict())
	})

	t.Run("marker split across chunks still detected", func(t *testing.T) {
		t.Parallel()
		g := newOpenAIDSMLLeakGuard(false)
		require.Empty(t, g.HandleLine(dsmlTestContentLine("abc ｜DS")))
		require.Empty(t, g.HandleLine(dsmlTestContentLine("ML｜ def")))
		require.Empty(t, g.HandleLine(dsmlTestFinishLine), "leak verdict swallows the terminal frame")
		require.True(t, g.LeakVerdict())
	})

	t.Run("late marker after release is observed but not recoverable", func(t *testing.T) {
		t.Parallel()
		g := newOpenAIDSMLLeakGuard(false)
		clean := dsmlTestContentLine(strings.Repeat("c", 200))
		require.Equal(t, []string{clean}, g.HandleLine(clean))
		late := dsmlTestContentLine("tail ｜DSML｜ garbage")
		require.Equal(t, []string{late}, g.HandleLine(late), "passthrough keeps streaming")
		require.True(t, g.LateMarkerSeen())
		require.False(t, g.LeakVerdict())
	})

	t.Run("nil guard passes lines through", func(t *testing.T) {
		t.Parallel()
		var g *openAIDSMLLeakGuard
		require.Equal(t, []string{"data: x"}, g.HandleLine("data: x"))
		require.False(t, g.LeakVerdict())
		require.False(t, g.Holding())
	})

	t.Run("prefix suspicion covers partial prefixes", func(t *testing.T) {
		t.Parallel()
		require.True(t, dsmlGuardPrefixSuspicious("<thi"), "partial prefix stays suspicious")
		require.True(t, dsmlGuardPrefixSuspicious("<thinking>x"))
		require.True(t, dsmlGuardPrefixSuspicious("<｜"))
		require.False(t, dsmlGuardPrefixSuspicious("normal text"))
		require.False(t, dsmlGuardPrefixSuspicious(""), "whitespace-only content must not be held forever")
	})

	t.Run("whitespace-leading content releases after sniff window", func(t *testing.T) {
		t.Parallel()
		g := newOpenAIDSMLLeakGuard(false)
		require.Empty(t, g.HandleLine(dsmlTestContentLine(strings.Repeat(" ", 64))), "first chunk enters hold")
		released := g.HandleLine(dsmlTestContentLine(strings.Repeat(" ", 64)))
		require.Len(t, released, 2, "128 whitespace bytes with empty prefix must sniff-release")
		require.False(t, g.Holding())
	})
}

// TestDSMLGuard_ResponsesLaneObserveVerdict 锁定 observe 模式的 Responses 事件解析：
// output_text.delta 里的标记 + 终态事件 → 判定泄漏；出现 function_call 事件则豁免。
func TestDSMLGuard_ResponsesLaneObserveVerdict(t *testing.T) {
	t.Parallel()

	t.Run("text leak with terminal event is a leak verdict", func(t *testing.T) {
		t.Parallel()
		g := newOpenAIDSMLLeakGuard(true)
		lines := []string{
			`data: {"type":"response.created","response":{"id":"resp_1"}}`,
			`data: {"type":"response.output_text.delta","delta":"<thinking>echo args</｜DSML｜tool_calls>"}`,
			`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed"}}`,
		}
		for _, line := range lines {
			require.Equal(t, []string{line}, g.HandleLine(line), "observe mode never withholds bytes")
		}
		require.True(t, g.LeakVerdict())
	})

	t.Run("structured function_call exempts the verdict", func(t *testing.T) {
		t.Parallel()
		g := newOpenAIDSMLLeakGuard(true)
		lines := []string{
			`data: {"type":"response.output_text.delta","delta":"prefix ｜DSML｜ noise"}`,
			`data: {"type":"response.output_item.added","item":{"type":"function_call","name":"lookup"}}`,
			`data: {"type":"response.completed","response":{"id":"resp_2"}}`,
		}
		for _, line := range lines {
			require.Equal(t, []string{line}, g.HandleLine(line))
		}
		require.False(t, g.LeakVerdict())
	})

	t.Run("no terminal event means no verdict", func(t *testing.T) {
		t.Parallel()
		g := newOpenAIDSMLLeakGuard(true)
		line := `data: {"type":"response.output_text.delta","delta":"<thinking>x</｜DSML｜tool_calls>"}`
		require.Equal(t, []string{line}, g.HandleLine(line))
		require.False(t, g.LeakVerdict())
	})
}

// 工具轮前导泄漏、标记被 chunk 边界拆开：单行 Contains 检不出完整标记，
// 必须整体拼接清洗后再放行（评审回归：跨 chunk 断裂标记曾原样漏给客户端）。
func TestDSMLGuard_SplitMarkerToolRoundScrubbedAcrossChunks(t *testing.T) {
	lines := []string{
		`data: {"id":"s1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		``,
		dsmlTestContentLine("<thinking>use weather "),
		``,
		dsmlTestContentLine("city=SEA</｜DS"),
		``,
		dsmlTestContentLine("ML｜tool_calls>"),
		``,
		`data: {"id":"s1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"weather","arguments":""}}]},"finish_reason":null}]}`,
		``,
		`data: {"id":"s1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}
	rec, upstream, _, result, err := runDSMLStreamFixture(t, dsmlGuardTestConfig(), dsmlStreamRequestBody(),
		dsmlSSEResponse("rid_split", lines...))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 1, "tool round must not retry")

	body := rec.Body.String()
	require.NotContains(t, body, "DSML", "no fragment of the split marker may reach the client")
	require.NotContains(t, body, "use weather", "leaked pre-tool text must be scrubbed")
	require.Contains(t, body, `"tool_calls"`)
	require.Contains(t, body, `"finish_reason":"tool_calls"`)
	require.Equal(t, 1, strings.Count(body, "data: [DONE]"))
}

// 重试流在 usage chunk 之前断掉：回退计上一 attempt 的 usage，不能计零
// （上游两次生成都已产生费用）。
func TestDSMLGuard_RetryTruncatedBeforeUsageFallsBackToPriorAttemptUsage(t *testing.T) {
	truncatedRetry := []string{
		`data: {"id":"a2","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		``,
		`data: {"id":"a2","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"partial answer before drop"},"finish_reason":null}]}`,
		``,
		// 连接在 finish/usage/[DONE] 之前断掉。
	}
	_, _, _, result, err := runDSMLStreamFixture(t, dsmlGuardTestConfig(), dsmlStreamRequestBody(),
		dsmlSSEResponse("rid_leak_1", dsmlLeakAttemptLines("a1", 11, 7)...),
		dsmlSSEResponse("rid_trunc_2", truncatedRetry...),
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 11, result.Usage.InputTokens, "must fall back to attempt-1 usage, not zero")
	require.Equal(t, 7, result.Usage.OutputTokens)
}

// 评审修复回归：shape-2 的标记跨 chunk 断裂（前半 <thinking>… 不含标记、后半才含
// 闭合标签）时，冲洗放行必须对拼接后的正文整体清洗，不能漏放前半的泄漏文本。
func TestDSMLGuard_MixedToolCallScrubsSplitMarkerAcrossChunks(t *testing.T) {
	lines := []string{
		`data: {"id":"m1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		``,
		`data: {"id":"m1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"<thinking>call weather"},"finish_reason":null}]}`,
		``,
		`data: {"id":"m1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"</｜DSML｜tool_call>"},"finish_reason":null}]}`,
		``,
		`data: {"id":"m1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{}"}}]},"finish_reason":null}]}`,
		``,
		`data: {"id":"m1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}
	rec, upstream, _, result, err := runDSMLStreamFixture(t, dsmlGuardTestConfig(), dsmlStreamRequestBody(),
		dsmlSSEResponse("rid_split", lines...))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 1, "tool round must not retry")

	body := rec.Body.String()
	require.NotContains(t, body, "｜DSML｜")
	require.NotContains(t, body, "call weather", "split leak halves must not reach the client")
	require.Contains(t, body, `"tool_calls"`)
	require.Contains(t, body, `"finish_reason":"tool_calls"`)
	require.Contains(t, body, "data: [DONE]")
}

// 评审修复回归：重试后必须换新 silent-refusal detector——坏 attempt 锁存的
// sawContent 不能吞掉 attempt-2 真 silent refusal 的零字节 failover 资格。
func TestDSMLGuard_RetrySilentRefusalStillFailsOver(t *testing.T) {
	reqBody := []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"` +
		strings.Repeat("x", openAISilentRefusalMinRequestBodyBytes) +
		`"}],"tools":[{"type":"function","function":{"name":"t"}}],"stream":true}`)
	// 无 reasoning 的全漏形态：客户端零字节（role 行被 refusal 门扣、正文被 guard 扣），
	// 零字节 failover 资格才可能保留到 attempt-2。
	noReasoningLeakLines := []string{
		`data: {"id":"s1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		``,
		`data: {"id":"s1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"<thinking>x</｜DSML｜tool_calls>"},"finish_reason":null}]}`,
		``,
		`data: {"id":"s1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
	}
	silentRefusalLines := []string{
		`data: {"id":"s2","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		``,
		`data: {"id":"s2","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":""},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
	}
	rec, upstream, _, result, err := runDSMLStreamFixture(t, dsmlGuardTestConfig(), reqBody,
		dsmlSSEResponse("rid_leak", noReasoningLeakLines...),
		dsmlSSEResponse("rid_silent", silentRefusalLines...),
	)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.True(t, IsOpenAISilentRefusalErrorBody(failoverErr.ResponseBody))
	require.Len(t, upstream.requests, 2)
	require.Empty(t, rec.Body.String(), "zero bytes may reach the client before a silent-refusal failover")
}

// 评审修复回归：attempt-2 在 usage chunk 之前断流时回退计 attempt-1 的 usage，
// 不允许上游双计费而网关计零；被扣的 attempt-2 正文照旧 fail-open 冲洗。
func TestDSMLGuard_TruncatedRetryFallsBackToPreviousAttemptUsage(t *testing.T) {
	truncatedLines := []string{
		`data: {"id":"t2","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		``,
		`data: {"id":"t2","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"partial answer"},"finish_reason":null}]}`,
		``,
	}
	rec, upstream, _, result, err := runDSMLStreamFixture(t, dsmlGuardTestConfig(), dsmlStreamRequestBody(),
		dsmlSSEResponse("rid_leak", dsmlLeakAttemptLines("t1", 7, 3)...),
		dsmlSSEResponse("rid_truncated", truncatedLines...),
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, 7, result.Usage.InputTokens, "fall back to attempt-1 usage")
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Contains(t, rec.Body.String(), `"content":"partial answer"`, "held lines flush fail-open on truncation")
}
