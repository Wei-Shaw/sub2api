package kiro

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClassifyInvalidModelIDIsNetworkNotAccountFault 是红线回归测试。
// 大陆直连 Kiro 必现 INVALID_MODEL_ID；若归类为账号故障，
// 首个请求就会把整个账号池禁掉。
func TestClassifyInvalidModelIDIsNetworkNotAccountFault(t *testing.T) {
	t.Parallel()

	body := []byte(`{"message":"Improperly formed request: INVALID_MODEL_ID"}`)

	// 它通常伴随 400 返回 —— body 检查必须优先于状态码判断。
	got := Classify(400, body)
	require.Equal(t, SignalNetworkRegion, got)
	require.NotEqual(t, SignalBadRequest, got, "不得误判为请求格式错误")

	// 换个状态码也一样。
	require.Equal(t, SignalNetworkRegion, Classify(403, body))

	// 网络/区域问题可以换端点重试，但绝不是账号的错。
	require.True(t, SignalNetworkRegion.Retryable())
	require.False(t, SignalNetworkRegion.Failoverable(),
		"换账号解决不了网络问题，不得触发账号转移")
}

// TestClassifyBadRequestIsNeitherRetryableNorFailoverable 是另一条红线：
// 400 意味着我们自己的请求构造有误，换账号同样失败，重试只会烧光整池。
func TestClassifyBadRequestIsNeitherRetryableNorFailoverable(t *testing.T) {
	t.Parallel()

	got := Classify(400, []byte(`{"message":"Improperly formed request"}`))
	require.Equal(t, SignalBadRequest, got)
	require.False(t, got.Retryable(), "400 重试只会重复失败")
	require.False(t, got.Failoverable(), "400 换账号一样失败，会烧光整池")
}

func TestClassifyAuthAndOverageAndRateLimit(t *testing.T) {
	t.Parallel()

	require.Equal(t, SignalAuthExpired, Classify(401, nil))
	require.Equal(t, SignalAuthExpired, Classify(403, nil))
	require.Equal(t, SignalOverage, Classify(402, nil))
	require.Equal(t, SignalRateLimited, Classify(429, nil))

	require.True(t, SignalAuthExpired.Retryable(), "刷新 token 后应重试一次")
	require.True(t, SignalRateLimited.Retryable(), "先换端点，端点耗尽再交给限流冷却")
	require.True(t, SignalRateLimited.Failoverable())
	require.False(t, SignalOverage.Retryable())
	require.True(t, SignalOverage.Failoverable(),
		"overage 是当前账号的订阅设置问题，池子里开了 overage 的其它账号能正常服务同一请求")
}

func TestClassifySuspensionAndCreditsExhausted(t *testing.T) {
	t.Parallel()

	require.Equal(t, SignalSuspended,
		Classify(403, []byte(`{"message":"Your subscription has been suspended"}`)))
	require.Equal(t, SignalCreditsExhausted,
		Classify(429, []byte(`{"message":"Monthly request limit reached, credits exhausted"}`)))

	require.False(t, SignalSuspended.Retryable())
	require.True(t, SignalSuspended.Failoverable(),
		"账号被停用是当前账号的问题，池子里其它未停用的账号能正常服务同一请求；"+
			"是否禁用当前账号是另一套独立机制，不应连带让当前请求失败")
	require.True(t, SignalCreditsExhausted.Failoverable(), "额度耗尽应换账号")
}

func TestClassifySuccessAndUnknown(t *testing.T) {
	t.Parallel()

	require.Equal(t, SignalOK, Classify(200, nil))
	require.Equal(t, SignalOK, Classify(204, nil))

	require.Equal(t, SignalUnknown, Classify(500, nil))
	require.True(t, SignalUnknown.Retryable(), "5xx 允许重试")
	require.True(t, SignalUnknown.Failoverable())
	require.False(t, SignalOK.Failoverable())
}

// TestClassify2xxIsNeverReclassifiedByBodyContent 是修复轮 2 Finding 1(a) 的回归测试。
// 成功响应的 body 里可能回显了请求文本或助手原文，其中出现敏感词（比如
// "suspended"）绝不能反过来把一次成功请求判成账号故障。
func TestClassify2xxIsNeverReclassifiedByBodyContent(t *testing.T) {
	t.Parallel()

	// body 命中停用短语（"has been suspended"），但状态码是 2xx —— 必须仍判 OK。
	body := []byte(`{"message":"heads up: this account has been suspended once before, but is currently fully active"}`)
	require.Equal(t, SignalOK, Classify(200, body))
}

// TestClassifySuspensionMarkerRequiresSpecificPhrase 是修复轮 2 Finding 1(b) 的
// 回归测试。停用判定必须用具体短语，不能用裸 "suspend" 子串匹配 —— 否则任何
// 提到 suspend 的请求文本或助手原文都会误伤健康账号。
func TestClassifySuspensionMarkerRequiresSpecificPhrase(t *testing.T) {
	t.Parallel()

	// 提到 suspend 但不是具体停用短语 —— 不得误判，应落到 403 的鉴权失效分支。
	got := Classify(403, []byte(`{"message":"unsuspend request rejected"}`))
	require.Equal(t, SignalAuthExpired, got)
	require.NotEqual(t, SignalSuspended, got, "裸 suspend 子串不应触发停用判定")

	// 具体停用短语仍然命中。
	require.Equal(t, SignalSuspended,
		Classify(403, []byte(`{"message":"Your subscription has been suspended"}`)))
}

// TestClassifyCreditsExhaustedPrecedesSuspensionMarker 是修复轮 2 Finding 2 的
// 回归测试：锁定 body 特征之间的优先级。一段文案同时命中额度耗尽与停用两类
// 关键词时，额度耗尽先判定并生效。
func TestClassifyCreditsExhaustedPrecedesSuspensionMarker(t *testing.T) {
	t.Parallel()

	body := []byte(`{"message":"account is suspended, monthly request limit reached"}`)
	require.Equal(t, SignalCreditsExhausted, Classify(403, body))
}

// TestClassifyUpstreamErrorNilIsUnknown 锁定 nil 安全：ClassifyUpstreamError(nil)
// 不得 panic，必须落到 SignalUnknown（跟 Classify 遇到无法识别状态码时的兜底一致）。
func TestClassifyUpstreamErrorNilIsUnknown(t *testing.T) {
	t.Parallel()

	require.Equal(t, SignalUnknown, ClassifyUpstreamError(nil))
}

// TestClassifyUpstreamErrorMessageMarkersPrecedeType 是 I6 的红线回归测试：
// exception 帧的 Message 特征串必须优先于 Type 判定。AWS event-stream 的
// ValidationException 常常伴随 INVALID_MODEL_ID，若先按 Type 判成
// SignalBadRequest，会掩盖"这其实是网络问题"这一事实——与 Classify 对
// 状态码 vs body 特征串的优先级要求同构。
func TestClassifyUpstreamErrorMessageMarkersPrecedeType(t *testing.T) {
	t.Parallel()

	err := &UpstreamError{
		Type:    "ValidationException",
		Message: "Improperly formed request: INVALID_MODEL_ID",
	}
	got := ClassifyUpstreamError(err)
	require.Equal(t, SignalNetworkRegion, got)
	require.NotEqual(t, SignalBadRequest, got, "不得让 Type 掩盖 Message 里的网络/区域特征串")
}

// TestClassifyUpstreamErrorCreditsPrecedesSuspensionMarker 复现
// TestClassifyCreditsExhaustedPrecedesSuspensionMarker 的顺序要求，但走
// ClassifyUpstreamError 路径——两条路径共用 classifyMarkers，顺序必须一致。
func TestClassifyUpstreamErrorCreditsPrecedesSuspensionMarker(t *testing.T) {
	t.Parallel()

	err := &UpstreamError{
		Type:    "ValidationException",
		Message: "account is suspended, monthly request limit reached",
	}
	require.Equal(t, SignalCreditsExhausted, ClassifyUpstreamError(err))
}

// TestClassifyUpstreamErrorTypeFallback 覆盖 Message 不含任何特征串时的
// Type-based 分类，逐个锁定 switch 的每条分支目标。
func TestClassifyUpstreamErrorTypeFallback(t *testing.T) {
	t.Parallel()

	cases := []struct {
		errType string
		want    Signal
	}{
		{"ThrottlingException", SignalRateLimited},
		{"TooManyRequestsException", SignalRateLimited},
		{"AccessDeniedException", SignalAuthExpired},
		{"UnauthorizedException", SignalAuthExpired},
		{"UnrecognizedClientException", SignalAuthExpired},
		{"ExpiredTokenException", SignalAuthExpired},
		{"ValidationException", SignalBadRequest},
		{"InvalidRequestException", SignalBadRequest},
		{"SerializationException", SignalBadRequest},
		{"BadRequestException", SignalBadRequest},
		{"SomeUnmappedException", SignalUnknown},
		{"", SignalUnknown},
	}
	for _, c := range cases {
		got := ClassifyUpstreamError(&UpstreamError{Type: c.errType, Message: "nothing interesting here"})
		require.Equal(t, c.want, got, "Type=%q", c.errType)
	}
}

func TestSignalStringIsStable(t *testing.T) {
	t.Parallel()

	// 这些字符串会进日志与告警，改动会破坏既有检索。
	require.Equal(t, "ok", SignalOK.String())
	require.Equal(t, "auth_expired", SignalAuthExpired.String())
	require.Equal(t, "overage", SignalOverage.String())
	require.Equal(t, "rate_limited", SignalRateLimited.String())
	require.Equal(t, "network_region", SignalNetworkRegion.String())
	require.Equal(t, "bad_request", SignalBadRequest.String())
	require.Equal(t, "suspended", SignalSuspended.String())
	require.Equal(t, "credits_exhausted", SignalCreditsExhausted.String())
	require.Equal(t, "unknown", SignalUnknown.String())
}
