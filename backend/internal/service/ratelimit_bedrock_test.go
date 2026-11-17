//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type bedrockErrorRepoStub struct {
	tempUnschedScopeRepoStub
	errorMessages []string
	rateLimitedAt []time.Time
}

func (r *bedrockErrorRepoStub) SetError(_ context.Context, _ int64, msg string) error {
	r.errorCalls++
	r.errorMessages = append(r.errorMessages, msg)
	return nil
}

func (r *bedrockErrorRepoStub) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.rateLimitedAt = append(r.rateLimitedAt, resetAt)
	return nil
}

type bedrockErrorUpstreamStub struct {
	status  int
	header  http.Header
	body    []byte
	calls   int
	lastURL string
}

func (u *bedrockErrorUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return u.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (u *bedrockErrorUpstreamStub) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.calls++
	u.lastURL = req.URL.String()
	header := http.Header{"Content-Type": []string{"application/json"}, "X-Amzn-Requestid": []string{"aws-req-1"}}
	for k, v := range u.header {
		header[k] = v
	}
	return &http.Response{StatusCode: u.status, Header: header, Body: io.NopCloser(bytes.NewReader(u.body))}, nil
}

func newBedrockErrorTestAccount(rules ...map[string]any) *Account {
	account := &Account{
		ID:          77,
		Name:        "bedrock-error-test",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeBedrock,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"auth_mode":  "apikey",
			"api_key":    "bedrock-key",
			"aws_region": "us-east-1",
		},
	}
	if len(rules) > 0 {
		raw := make([]any, 0, len(rules))
		for _, rule := range rules {
			raw = append(raw, rule)
		}
		account.Credentials["temp_unschedulable_enabled"] = true
		account.Credentials["temp_unschedulable_rules"] = raw
	}
	return account
}

type bedrockForwardOutcome struct {
	result   *ForwardResult
	err      error
	failover *UpstreamFailoverError
	recorder *httptest.ResponseRecorder
	upstream *bedrockErrorUpstreamStub
}

type bedrockForwardOptions struct {
	ctx         context.Context
	model       string
	stream      bool
	rateLimiter *RateLimitService
}

func runBedrockForward(t *testing.T, repo *bedrockErrorRepoStub, account *Account, upstream *bedrockErrorUpstreamStub, stream bool) bedrockForwardOutcome {
	t.Helper()
	return runBedrockForwardWith(t, repo, account, upstream, bedrockForwardOptions{stream: stream})
}

func runBedrockForwardWith(t *testing.T, repo *bedrockErrorRepoStub, account *Account, upstream *bedrockErrorUpstreamStub, opts bedrockForwardOptions) bedrockForwardOutcome {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	rateLimiter := opts.rateLimiter
	if rateLimiter == nil {
		rateLimiter = &RateLimitService{accountRepo: repo, cfg: cfg}
	}
	ctx := opts.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	model := opts.model
	if model == "" {
		model = "claude-sonnet-4-5"
	}
	svc := &GatewayService{cfg: cfg, httpUpstream: upstream, rateLimitService: rateLimiter}
	body := []byte(`{"model":"` + model + `","messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	parsed := &ParsedRequest{Body: NewRequestBodyRef(body), Model: model, Stream: opts.stream}

	result, err := svc.forwardBedrock(ctx, c, account, parsed, time.Now())
	out := bedrockForwardOutcome{result: result, err: err, recorder: rec, upstream: upstream}
	var failover *UpstreamFailoverError
	if errors.As(err, &failover) {
		out.failover = failover
	}
	return out
}

func bedrockErrorRule(statusCode int, keyword string, minutes int, scope string) map[string]any {
	return map[string]any{
		"error_code":       float64(statusCode),
		"keywords":         []any{keyword},
		"duration_minutes": float64(minutes),
		"scope":            scope,
	}
}

func TestBedrockUpstreamErrors_AccountBlockedDisablesAndFailsOver(t *testing.T) {
	repo := &bedrockErrorRepoStub{}
	upstream := &bedrockErrorUpstreamStub{status: http.StatusBadRequest, body: []byte(`{"message":"Operation not allowed"}`)}

	out := runBedrockForward(t, repo, newBedrockErrorTestAccount(), upstream, false)

	require.NotNil(t, out.failover, "封号必须换号，不能把 400 回给客户端")
	require.Equal(t, http.StatusBadRequest, out.failover.StatusCode)
	require.Equal(t, 1, repo.errorCalls)
	require.Contains(t, repo.errorMessages[0], "Operation not allowed")
	require.Zero(t, out.recorder.Body.Len())
}

func TestBedrockUpstreamErrors_UseCaseMissingDisablesAndFailsOver(t *testing.T) {
	repo := &bedrockErrorRepoStub{}
	upstream := &bedrockErrorUpstreamStub{status: http.StatusNotFound, body: []byte(`{"message":"Model use case details have not been submitted for this account. Fill out the Anthropic use case details form before using the model. If you have already filled out the form, try again in 15 minutes."}`)}

	out := runBedrockForward(t, repo, newBedrockErrorTestAccount(), upstream, false)

	require.NotNil(t, out.failover)
	require.Equal(t, http.StatusNotFound, out.failover.StatusCode)
	require.Equal(t, 1, repo.errorCalls)
	require.Empty(t, repo.modelRateLimitCalls, "用例未提交是账号级问题，不按模型不存在冷却")
	require.Zero(t, out.recorder.Body.Len())
}

func TestBedrockUpstreamErrors_TempUnschedRuleOverridesAccountLevelDefaults(t *testing.T) {
	repo := &bedrockErrorRepoStub{}
	upstream := &bedrockErrorUpstreamStub{status: http.StatusNotFound, body: []byte(`{"message":"Model use case details have not been submitted for this account."}`)}
	account := newBedrockErrorTestAccount(bedrockErrorRule(http.StatusNotFound, "use case details", 30, TempUnschedScopeAccount))

	out := runBedrockForward(t, repo, account, upstream, false)

	require.NotNil(t, out.failover)
	require.Zero(t, repo.errorCalls)
	require.Equal(t, 1, repo.tempCalls)
}

func TestBedrockUpstreamErrors_ThrottlingCoolsOnlyRequestedModel(t *testing.T) {
	for _, msg := range []string{
		"Too many requests, please wait before trying again.",
		"Too many tokens per day, please wait before trying again.",
	} {
		t.Run(msg, func(t *testing.T) {
			repo := &bedrockErrorRepoStub{}
			upstream := &bedrockErrorUpstreamStub{status: http.StatusTooManyRequests, body: []byte(`{"message":"` + msg + `"}`)}

			out := runBedrockForward(t, repo, newBedrockErrorTestAccount(), upstream, false)

			require.NotNil(t, out.failover)
			require.Equal(t, http.StatusTooManyRequests, out.failover.StatusCode)
			require.Equal(t, 1, upstream.calls, "Bedrock 429 不在同一个桶上重试")
			require.Empty(t, repo.rateLimitedAt, "429 不冷却整个账号")
			require.Len(t, repo.modelRateLimitCalls, 1)
			call := repo.modelRateLimitCalls[0]
			require.Equal(t, "claude-sonnet-4-5", call.scope, "冷却键与调度器读取的模型键一致")
			require.Equal(t, bedrockThrottlingReason, call.reason)
			require.WithinDuration(t, time.Now().Add(defaultRateLimit429CooldownSeconds*time.Second), call.resetAt, 2*time.Second)
		})
	}
}

func TestBedrockUpstreamErrors_DailyTokenRuleTakesPrecedenceOverThrottlingCooldown(t *testing.T) {
	repo := &bedrockErrorRepoStub{}
	upstream := &bedrockErrorUpstreamStub{status: http.StatusTooManyRequests, body: []byte(`{"message":"Too many tokens per day, please wait before trying again."}`)}
	account := newBedrockErrorTestAccount(bedrockErrorRule(http.StatusTooManyRequests, "tokens per day", 240, TempUnschedScopeModel))

	out := runBedrockForward(t, repo, account, upstream, false)

	require.NotNil(t, out.failover)
	require.Empty(t, repo.rateLimitedAt)
	require.Zero(t, repo.tempCalls)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "claude-sonnet-4-5", repo.modelRateLimitCalls[0].scope)
	require.WithinDuration(t, time.Now().Add(240*time.Minute), repo.modelRateLimitCalls[0].resetAt, 2*time.Second)
}

func TestBedrockThrottlingWithoutModelFallsBackToAccountCooldown(t *testing.T) {
	repo := &bedrockErrorRepoStub{}
	svc := &RateLimitService{accountRepo: repo}

	shouldDisable := svc.HandleUpstreamError(context.Background(), newBedrockErrorTestAccount(), http.StatusTooManyRequests, http.Header{}, []byte(`{"message":"Too many requests, please wait before trying again."}`))

	require.False(t, shouldDisable)
	require.Empty(t, repo.modelRateLimitCalls)
	require.Len(t, repo.rateLimitedAt, 1)
}

func TestBedrockUpstreamErrors_ContentFilterReturnsRequestErrorWithoutFailover(t *testing.T) {
	repo := &bedrockErrorRepoStub{}
	upstream := &bedrockErrorUpstreamStub{status: http.StatusInternalServerError, body: []byte(`{"message":"Output blocked by content filtering policy"}`)}

	out := runBedrockForward(t, repo, newBedrockErrorTestAccount(), upstream, false)

	require.Error(t, out.err)
	require.Nil(t, out.failover, "内容过滤换号结果相同，不换号")
	require.Equal(t, http.StatusBadRequest, out.recorder.Code)
	require.Equal(t, "invalid_request_error", gjson.Get(out.recorder.Body.String(), "error.type").String())
	require.Equal(t, "Output blocked by content filtering policy", gjson.Get(out.recorder.Body.String(), "error.message").String())
	require.Zero(t, repo.errorCalls)
	require.Zero(t, repo.tempCalls)
	require.Empty(t, repo.modelRateLimitCalls)
	require.Empty(t, repo.rateLimitedAt)
}

func TestBedrockUpstreamErrors_ServerErrorsStillFailOverWithoutPenalty(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{http.StatusInternalServerError, `{"message":"Internal server error"}`},
		{http.StatusServiceUnavailable, `{"message":"Bedrock is unable to process your request."}`},
	} {
		repo := &bedrockErrorRepoStub{}
		upstream := &bedrockErrorUpstreamStub{status: tc.status, body: []byte(tc.body)}

		out := runBedrockForward(t, repo, newBedrockErrorTestAccount(), upstream, false)

		require.NotNil(t, out.failover, "status %d", tc.status)
		require.Equal(t, tc.status, out.failover.StatusCode)
		require.Zero(t, repo.errorCalls)
		require.Zero(t, repo.tempCalls)
		require.Empty(t, repo.modelRateLimitCalls)
		require.Empty(t, repo.rateLimitedAt)
	}
}

func TestBedrockUpstreamErrors_OtherValidationErrorsReturnToClient(t *testing.T) {
	repo := &bedrockErrorRepoStub{}
	upstream := &bedrockErrorUpstreamStub{status: http.StatusBadRequest, body: []byte(`{"message":"messages.0.content.0.image.source.base64: image exceeds 5 MB maximum"}`)}

	out := runBedrockForward(t, repo, newBedrockErrorTestAccount(), upstream, false)

	require.Error(t, out.err)
	require.Nil(t, out.failover)
	require.Equal(t, http.StatusBadRequest, out.recorder.Code)
	require.Zero(t, repo.errorCalls)
}

func buildBedrockEventStreamFrame(headers map[string]string, payload []byte) []byte {
	var headersBuf bytes.Buffer
	for _, name := range []string{":event-type", ":exception-type", ":message-type"} {
		value, ok := headers[name]
		if !ok {
			continue
		}
		_ = headersBuf.WriteByte(byte(len(name)))
		_, _ = headersBuf.WriteString(name)
		_ = headersBuf.WriteByte(7)
		_ = binary.Write(&headersBuf, binary.BigEndian, uint16(len(value)))
		_, _ = headersBuf.WriteString(value)
	}
	headerBytes := headersBuf.Bytes()
	total := uint32(12 + len(headerBytes) + len(payload) + 4)

	var prelude bytes.Buffer
	_ = binary.Write(&prelude, binary.BigEndian, total)
	_ = binary.Write(&prelude, binary.BigEndian, uint32(len(headerBytes)))

	var frame bytes.Buffer
	_, _ = frame.Write(prelude.Bytes())
	_ = binary.Write(&frame, binary.BigEndian, crc32.ChecksumIEEE(prelude.Bytes()))
	_, _ = frame.Write(headerBytes)
	_, _ = frame.Write(payload)
	_ = binary.Write(&frame, binary.BigEndian, crc32.ChecksumIEEE(frame.Bytes()))
	return frame.Bytes()
}

func bedrockChunkFrame(event string) []byte {
	payload := []byte(`{"bytes":"` + base64.StdEncoding.EncodeToString([]byte(event)) + `"}`)
	return buildBedrockEventStreamFrame(map[string]string{":event-type": "chunk", ":message-type": "event"}, payload)
}

func TestForwardBedrockStreamInterruptionKeepsObservedUsage(t *testing.T) {
	var stream bytes.Buffer
	_, _ = stream.Write(bedrockChunkFrame(`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","usage":{"input_tokens":120,"cache_read_input_tokens":30,"output_tokens":1}}}`))
	_, _ = stream.Write(bedrockChunkFrame(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hel"}}`))
	_, _ = stream.Write(buildBedrockEventStreamFrame(map[string]string{
		":exception-type": "internalServerException",
		":message-type":   "exception",
	}, []byte(`{"message":"Overloaded"}`)))

	repo := &bedrockErrorRepoStub{}
	upstream := &bedrockErrorUpstreamStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"application/vnd.amazon.eventstream"}},
		body:   stream.Bytes(),
	}

	out := runBedrockForward(t, repo, newBedrockErrorTestAccount(), upstream, true)

	require.Error(t, out.err)
	require.Nil(t, out.failover, "已向客户端输出内容后不换号")
	require.Contains(t, out.err.Error(), "internalServerException")
	require.NotNil(t, out.result, "流中断时返回已观测到的 usage 供计费")
	require.Equal(t, 120, out.result.Usage.InputTokens)
	require.Equal(t, 30, out.result.Usage.CacheReadInputTokens)
	require.Equal(t, "claude-sonnet-4-5", out.result.Model)
	require.True(t, strings.HasPrefix(out.result.UpstreamModel, "us.anthropic.claude-sonnet-4-5"), out.result.UpstreamModel)
	require.Equal(t, "aws-req-1", out.result.RequestID)
	require.True(t, out.result.Stream)
	require.Contains(t, out.recorder.Body.String(), "message_start")
}

func TestBedrockUpstreamErrors_AccountBlockedMatchesWholeMessageOnly(t *testing.T) {
	for _, msg := range []string{"Operation not allowed", "operation not allowed.", "  OPERATION NOT ALLOWED  "} {
		repo := &bedrockErrorRepoStub{}
		upstream := &bedrockErrorUpstreamStub{status: http.StatusBadRequest, body: []byte(`{"message":"` + msg + `"}`)}

		out := runBedrockForward(t, repo, newBedrockErrorTestAccount(), upstream, false)

		require.NotNil(t, out.failover, msg)
		require.Equal(t, 1, repo.errorCalls, msg)
	}
}

func TestBedrockUpstreamErrors_EchoedRequestContentDoesNotDisableAccount(t *testing.T) {
	for _, echoed := range []string{"Operation not allowed", "credit balance", "organization has been disabled", "identity verification is required"} {
		t.Run(echoed, func(t *testing.T) {
			repo := &bedrockErrorRepoStub{}
			body := `{"message":"Malformed input request: #: extraneous key [` + echoed + `] is not permitted, please reformat your input and try again."}`
			upstream := &bedrockErrorUpstreamStub{status: http.StatusBadRequest, body: []byte(body)}

			out := runBedrockForward(t, repo, newBedrockErrorTestAccount(), upstream, false)

			require.Nil(t, out.failover, "请求错误不换号")
			require.Equal(t, http.StatusBadRequest, out.recorder.Code)
			require.Zero(t, repo.errorCalls, "回显进错误消息的请求内容不能触发停号")
			require.Zero(t, repo.tempCalls)
			require.Empty(t, repo.modelRateLimitCalls)
		})
	}
}

func TestBedrockUpstreamErrors_ChannelMappedModelCoolsSchedulingKey(t *testing.T) {
	ctx := WithGatewaySchedulingModel(context.Background(), "claude-sonnet-4-5")

	t.Run("throttling", func(t *testing.T) {
		repo := &bedrockErrorRepoStub{}
		upstream := &bedrockErrorUpstreamStub{status: http.StatusTooManyRequests, body: []byte(`{"message":"Too many requests, please wait before trying again."}`)}
		account := newBedrockErrorTestAccount()

		out := runBedrockForwardWith(t, repo, account, upstream, bedrockForwardOptions{ctx: ctx, model: "claude-sonnet-4-6"})

		require.NotNil(t, out.failover)
		require.Contains(t, upstream.lastURL, "claude-sonnet-4-6", "上游按渠道映射后的模型请求")
		require.Len(t, repo.modelRateLimitCalls, 1)
		scope := repo.modelRateLimitCalls[0].scope
		require.Equal(t, "claude-sonnet-4-5", scope, "冷却写在调度器按客户端模型读取的键上")
		setAccountModelRateLimitSnapshot(account, scope, time.Now().Add(time.Minute), bedrockThrottlingReason, time.Now())
		require.False(t, account.IsSchedulableForModelWithContext(context.Background(), "claude-sonnet-4-5"))
	})

	t.Run("model scoped rule", func(t *testing.T) {
		repo := &bedrockErrorRepoStub{}
		upstream := &bedrockErrorUpstreamStub{status: http.StatusTooManyRequests, body: []byte(`{"message":"Too many tokens per day, please wait before trying again."}`)}
		account := newBedrockErrorTestAccount(bedrockErrorRule(http.StatusTooManyRequests, "tokens per day", 240, TempUnschedScopeModel))

		out := runBedrockForwardWith(t, repo, account, upstream, bedrockForwardOptions{ctx: ctx, model: "claude-sonnet-4-6"})

		require.NotNil(t, out.failover)
		require.Len(t, repo.modelRateLimitCalls, 1)
		require.Equal(t, "claude-sonnet-4-5", repo.modelRateLimitCalls[0].scope)
	})
}

func TestBedrockUpstreamErrors_ThrottlingCooldownDisabledWritesNothing(t *testing.T) {
	repo := &bedrockErrorRepoStub{}
	settingRepo := newMockSettingRepo()
	settingRepo.data[SettingKeyRateLimit429CooldownSettings] = `{"enabled":false,"cooldown_seconds":12}`
	rateLimiter := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	rateLimiter.SetSettingService(NewSettingService(settingRepo, &config.Config{}))
	upstream := &bedrockErrorUpstreamStub{status: http.StatusTooManyRequests, body: []byte(`{"message":"Too many requests, please wait before trying again."}`)}

	out := runBedrockForwardWith(t, repo, newBedrockErrorTestAccount(), upstream, bedrockForwardOptions{rateLimiter: rateLimiter})

	require.NotNil(t, out.failover)
	require.Empty(t, repo.modelRateLimitCalls)
	require.Empty(t, repo.rateLimitedAt)
}

func TestBedrockUpstreamErrors_ContentFilterOnlyForRewrittenServerErrors(t *testing.T) {
	for _, status := range []int{501, 529} {
		repo := &bedrockErrorRepoStub{}
		upstream := &bedrockErrorUpstreamStub{status: status, body: []byte(`{"message":"Output blocked by content filtering policy"}`)}

		out := runBedrockForward(t, repo, newBedrockErrorTestAccount(), upstream, false)

		require.NotNil(t, out.failover, "status %d 按原有 5xx 语义换号", status)
		require.Equal(t, status, out.failover.StatusCode)
	}
	for _, status := range []int{500, 502, 503, 504} {
		repo := &bedrockErrorRepoStub{}
		upstream := &bedrockErrorUpstreamStub{status: status, body: []byte(`{"message":"Input blocked by content filtering policy"}`)}

		out := runBedrockForward(t, repo, newBedrockErrorTestAccount(), upstream, false)

		require.Nil(t, out.failover, "status %d", status)
		require.Equal(t, http.StatusBadRequest, out.recorder.Code, "status %d", status)
	}
}
