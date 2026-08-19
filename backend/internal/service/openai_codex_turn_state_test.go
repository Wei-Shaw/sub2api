package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newTurnStateTestContext(t *testing.T, apiKeyID int64, sessionID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if sessionID != "" {
		c.Request.Header.Set("session_id", sessionID)
	}
	if apiKeyID > 0 {
		c.Set("api_key", &APIKey{ID: apiKeyID})
	}
	return c, rec
}

func TestOpenAICodexTurnStateSeed(t *testing.T) {
	c, _ := newTurnStateTestContext(t, 7, "sess-1")
	require.Equal(t, "7\x00sess-1", openAICodexTurnStateSeed(c))

	// 连字符形式优先（Codex CLI 标准头）
	c.Request.Header.Set("session-id", "sess-hyphen")
	require.Equal(t, "7\x00sess-hyphen", openAICodexTurnStateSeed(c))

	// 无会话标识 → 不跟踪
	cNoSession, _ := newTurnStateTestContext(t, 7, "")
	require.Empty(t, openAICodexTurnStateSeed(cNoSession))

	require.Empty(t, openAICodexTurnStateSeed(nil))
}

func TestRelayOpenAICodexTurnState_SetsHeaderAndRecordsProvenance(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 42}
	c, _ := newTurnStateTestContext(t, 7, "sess-relay")

	upstream := http.Header{}
	upstream.Set("x-codex-turn-state", "blob-A")
	svc.relayOpenAICodexTurnState(c, account, upstream)

	require.Equal(t, "blob-A", c.Writer.Header().Get("X-Codex-Turn-State"))

	raw, ok := svc.openaiCodexTurnStateOrigins.Load("7\x00sess-relay")
	require.True(t, ok)
	origin, ok := raw.(openAICodexTurnStateOrigin)
	require.True(t, ok)
	require.Equal(t, int64(42), origin.accountID)
	require.True(t, origin.expiresAt.After(time.Now()))
}

func TestRelayOpenAICodexTurnState_ClearsStaleValueWhenUpstreamAbsent(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 7, "sess-stale")
	// 模拟上一 failover attempt 残留的值
	c.Writer.Header().Set("X-Codex-Turn-State", "blob-old")

	svc.relayOpenAICodexTurnState(c, &Account{ID: 43}, http.Header{})

	require.Empty(t, c.Writer.Header().Get("X-Codex-Turn-State"))
	_, ok := svc.openaiCodexTurnStateOrigins.Load("7\x00sess-stale")
	require.False(t, ok)
}

func TestStageOpenAICodexTurnState_StagedHeaders(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 9, "sess-staged")

	// nil 集合 + 上游有值 → 创建集合并写入，但此刻还不记录溯源
	var staged http.Header
	upstream := http.Header{}
	upstream.Set("x-codex-turn-state", "blob-B")
	stageOpenAICodexTurnState(&staged, upstream)
	require.NotNil(t, staged)
	require.Equal(t, "blob-B", staged.Get("X-Codex-Turn-State"))
	_, noted := svc.openaiCodexTurnStateOrigins.Load("9\x00sess-staged")
	require.False(t, noted, "暂存阶段不得记录溯源：该 attempt 仍可能 failover 丢弃")

	// 真正提交时才记录
	svc.noteStagedOpenAICodexTurnStateCommitted(c, &Account{ID: 44}, staged)
	raw, ok := svc.openaiCodexTurnStateOrigins.Load("9\x00sess-staged")
	require.True(t, ok)
	origin, ok := raw.(openAICodexTurnStateOrigin)
	require.True(t, ok)
	require.Equal(t, int64(44), origin.accountID)

	// 上游无值 → 清除已暂存的值；nil 集合保持 nil
	stageOpenAICodexTurnState(&staged, http.Header{})
	require.Empty(t, staged.Get("X-Codex-Turn-State"))
	var nilStaged http.Header
	stageOpenAICodexTurnState(&nilStaged, http.Header{})
	require.Nil(t, nilStaged)
}

// 首输出超时导致 attempt 被丢弃时，溯源不得被该 attempt 污染——否则后续
// 请求会把客户端持有的合法 blob 误判成跨账号回带而剥离。
func TestStagedTurnState_AbandonedAttemptDoesNotPoisonProvenance(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 11, "sess-abandoned")

	// 账号 A 的 attempt 暂存了 blob，但从未提交（首输出超时 → failover）
	var staged http.Header
	upstreamA := http.Header{}
	upstreamA.Set("x-codex-turn-state", "blob-A")
	stageOpenAICodexTurnState(&staged, upstreamA)

	// 账号 B 接手并真正提交
	svc.relayOpenAICodexTurnState(c, &Account{ID: 52}, upstreamA)

	// 客户端回带的 blob 来自 B，出站到 B 时不得被剥离
	h := http.Header{}
	h.Set("x-codex-turn-state", "blob-A")
	svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 52}, h)
	require.Equal(t, "blob-A", h.Get("x-codex-turn-state"))

	raw, ok := svc.openaiCodexTurnStateOrigins.Load("11\x00sess-abandoned")
	require.True(t, ok)
	origin, ok := raw.(openAICodexTurnStateOrigin)
	require.True(t, ok)
	require.Equal(t, int64(52), origin.accountID)
}

func TestNoteStagedOpenAICodexTurnStateCommitted_NoopWithoutState(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 12, "sess-nostate")

	svc.noteStagedOpenAICodexTurnStateCommitted(c, &Account{ID: 60}, nil)
	svc.noteStagedOpenAICodexTurnStateCommitted(c, &Account{ID: 60}, http.Header{"X-Request-Id": []string{"rid"}})

	_, ok := svc.openaiCodexTurnStateOrigins.Load("12\x00sess-nostate")
	require.False(t, ok)
}

func TestGuardOpenAICodexTurnStateEcho(t *testing.T) {
	newOutbound := func(state string) http.Header {
		h := http.Header{}
		if state != "" {
			h.Set("x-codex-turn-state", state)
		}
		return h
	}

	t.Run("same_account_keeps_echo", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		c, _ := newTurnStateTestContext(t, 7, "sess-g1")
		upstream := http.Header{}
		upstream.Set("x-codex-turn-state", "blob-A")
		svc.relayOpenAICodexTurnState(c, &Account{ID: 42}, upstream)

		h := newOutbound("blob-A")
		svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 42}, h)
		require.Equal(t, "blob-A", h.Get("x-codex-turn-state"))
	})

	t.Run("foreign_account_strips_echo", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		c, _ := newTurnStateTestContext(t, 7, "sess-g2")
		upstream := http.Header{}
		upstream.Set("x-codex-turn-state", "blob-A")
		svc.relayOpenAICodexTurnState(c, &Account{ID: 42}, upstream)

		// failover 换到账号 43：blob 由 42 铸造，必须剥离
		h := newOutbound("blob-A")
		svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 43}, h)
		require.Empty(t, h.Get("x-codex-turn-state"))
	})

	t.Run("no_provenance_passthrough", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		c, _ := newTurnStateTestContext(t, 7, "sess-g3")
		h := newOutbound("blob-unknown")
		svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 43}, h)
		require.Equal(t, "blob-unknown", h.Get("x-codex-turn-state"))
	})

	t.Run("expired_provenance_passthrough_and_pruned", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		c, _ := newTurnStateTestContext(t, 7, "sess-g4")
		svc.openaiCodexTurnStateOrigins.Store("7\x00sess-g4", openAICodexTurnStateOrigin{
			accountID: 42,
			expiresAt: time.Now().Add(-time.Minute),
		})
		h := newOutbound("blob-A")
		svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 43}, h)
		require.Equal(t, "blob-A", h.Get("x-codex-turn-state"))
		_, ok := svc.openaiCodexTurnStateOrigins.Load("7\x00sess-g4")
		require.False(t, ok)
	})

	t.Run("no_session_seed_noop", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		c, _ := newTurnStateTestContext(t, 7, "")
		h := newOutbound("blob-A")
		svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 43}, h)
		require.Equal(t, "blob-A", h.Get("x-codex-turn-state"))
	})

	t.Run("no_echo_noop", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		c, _ := newTurnStateTestContext(t, 7, "sess-g5")
		h := newOutbound("")
		svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 43}, h)
		require.Empty(t, h.Get("x-codex-turn-state"))
	})
}

func TestSweepOpenAICodexTurnStateOrigins_PrunesExpiredEntries(t *testing.T) {
	svc := &OpenAIGatewayService{}
	svc.openaiCodexTurnStateOrigins.Store("expired", openAICodexTurnStateOrigin{
		accountID: 1,
		expiresAt: time.Now().Add(-time.Minute),
	})
	svc.openaiCodexTurnStateOrigins.Store("alive", openAICodexTurnStateOrigin{
		accountID: 2,
		expiresAt: time.Now().Add(time.Hour),
	})

	// 计数器推进到触发清扫的边界
	svc.openaiCodexTurnStateWrites.Store(255)
	svc.sweepOpenAICodexTurnStateOrigins()

	_, expiredOK := svc.openaiCodexTurnStateOrigins.Load("expired")
	require.False(t, expiredOK)
	_, aliveOK := svc.openaiCodexTurnStateOrigins.Load("alive")
	require.True(t, aliveOK)
}

func TestWriteOpenAIPassthroughResponseHeaders_RelaysAndClearsTurnState(t *testing.T) {
	// filter=nil 走 content-type 兜底分支；turn-state 强制放行不依赖 filter。
	dst := http.Header{}
	src := http.Header{}
	src.Set("X-Codex-Turn-State", "blob-P")
	writeOpenAIPassthroughResponseHeaders(dst, src, nil)
	require.Equal(t, "blob-P", dst.Get("X-Codex-Turn-State"))

	// 上游缺失时清除残留（failover 换号防串扰）
	writeOpenAIPassthroughResponseHeaders(dst, http.Header{"Content-Type": []string{"application/json"}}, nil)
	require.Empty(t, dst.Get("X-Codex-Turn-State"))
}

func TestEnsureOpenAIRemoteCompactionV2BetaFeature(t *testing.T) {
	t.Run("absent_sets_feature", func(t *testing.T) {
		h := http.Header{}
		ensureOpenAIRemoteCompactionV2BetaFeature(h)
		require.Equal(t, "remote_compaction_v2", h.Get("x-codex-beta-features"))
	})

	t.Run("present_unchanged", func(t *testing.T) {
		h := http.Header{}
		h.Set("x-codex-beta-features", "responses_websockets_v2, remote_compaction_v2")
		ensureOpenAIRemoteCompactionV2BetaFeature(h)
		require.Equal(t, "responses_websockets_v2, remote_compaction_v2", h.Get("x-codex-beta-features"))
	})

	t.Run("other_tokens_merged", func(t *testing.T) {
		h := http.Header{}
		h.Set("x-codex-beta-features", "responses_websockets_v2")
		ensureOpenAIRemoteCompactionV2BetaFeature(h)
		require.Equal(t, "responses_websockets_v2,remote_compaction_v2", h.Get("x-codex-beta-features"))
	})

	t.Run("multi_line_values_merged_single_line", func(t *testing.T) {
		h := http.Header{}
		h.Add("x-codex-beta-features", "feature_a")
		h.Add("x-codex-beta-features", "feature_b")
		ensureOpenAIRemoteCompactionV2BetaFeature(h)
		require.Equal(t, []string{"feature_a,feature_b,remote_compaction_v2"}, h.Values("x-codex-beta-features"))
	})
}

// 对齐真实 Codex：该头是会话级常量，挂在 OAuth 的每个请求上，而不是只在
// 压缩回合出现（codex-rs build_model_client_beta_features_header）。
func TestApplyOpenAICodexBetaFeatures(t *testing.T) {
	oauthAccount := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKeyAccount := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	t.Run("oauth_plain_request_gets_default_codex_shape", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		h := http.Header{}
		applyOpenAICodexBetaFeatures(c, oauthAccount, h)
		require.Equal(t, "remote_compaction_v2", h.Get("x-codex-beta-features"),
			"OAuth 的普通请求也必须带会话级 beta 头")
	})

	t.Run("client_declared_header_preserved", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		h := http.Header{}
		h.Set("x-codex-beta-features", "some_other_feature")
		applyOpenAICodexBetaFeatures(c, oauthAccount, h)
		require.Equal(t, "some_other_feature", h.Get("x-codex-beta-features"),
			"客户端显式声明的能力集不得被网关改写（非空即视为用户已关闭 v2）")
	})

	t.Run("native_v2_forces_feature_even_when_client_trimmed_it", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		MarkOpenAINativeCompactionV2(c)
		h := http.Header{}
		h.Set("x-codex-beta-features", "some_other_feature")
		applyOpenAICodexBetaFeatures(c, oauthAccount, h)
		require.Contains(t, h.Get("x-codex-beta-features"), "remote_compaction_v2",
			"body 带 compaction_trigger 是实锤，必须确保 v2 在列")
		require.Contains(t, h.Get("x-codex-beta-features"), "some_other_feature")
	})

	t.Run("native_v2_applies_to_non_oauth_too", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		MarkOpenAINativeCompactionV2(c)
		h := http.Header{}
		applyOpenAICodexBetaFeatures(c, apiKeyAccount, h)
		require.Equal(t, "remote_compaction_v2", h.Get("x-codex-beta-features"))
	})

	t.Run("non_oauth_plain_request_untouched", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		h := http.Header{}
		applyOpenAICodexBetaFeatures(c, apiKeyAccount, h)
		require.Empty(t, h.Get("x-codex-beta-features"),
			"非 Codex 后端不做会话级注入")
	})

	t.Run("nil_account_plain_request_untouched", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		h := http.Header{}
		applyOpenAICodexBetaFeatures(c, nil, h)
		require.Empty(t, h.Get("x-codex-beta-features"))
	})
}

// WS 握手与 HTTP 出站必须给出同一份会话级 beta 头：真实 Codex 的
// build_websocket_headers 复用 build_responses_headers（client.rs），
// 两侧不一致还会让预热连接与实际请求落进不同的连接池兼容分桶。
func TestBuildOpenAIWSHeaders_CarriesSessionBetaFeatures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}
	decision := OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}

	build := func(t *testing.T, account *Account, clientBeta string) http.Header {
		t.Helper()
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
		if clientBeta != "" {
			c.Request.Header.Set("x-codex-beta-features", clientBeta)
		}
		headers, _, err := svc.buildOpenAIWSHeaders(
			context.Background(), c, account, "test-token", decision,
			true, "", "", "", "gpt-5.6-codex", "",
		)
		require.NoError(t, err)
		return headers
	}

	oauthAccount := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "test-account"},
	}

	headers := build(t, oauthAccount, "")
	require.Equal(t, "remote_compaction_v2", headers.Get("x-codex-beta-features"),
		"WS 握手也必须带会话级 beta 头")

	declared := build(t, oauthAccount, "some_other_feature")
	require.Equal(t, []string{"some_other_feature"}, declared.Values("x-codex-beta-features"),
		"客户端已声明时原样保留")

	apiKeyHeaders := build(t, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "")
	require.Empty(t, apiKeyHeaders.Get("x-codex-beta-features"),
		"非 Codex 后端不注入")
}
