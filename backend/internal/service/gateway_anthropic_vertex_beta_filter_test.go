package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newVertexBetaTestContext(t *testing.T, anthropicBeta string) *gin.Context {
REDACTED
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	if anthropicBeta != "" {
		c.Request.Header.Set("Anthropic-Beta", anthropicBeta)
REDACTED
	return c
REDACTED

func newVertexServiceAccount(id int64) *Account {
REDACTED
		ID:       id,
		Platform: PlatformAnthropic,
REDACTED
REDACTED
			"project_id": "vertex-proj",
			"location":   "us-east5",
	REDACTED,
REDACTED
REDACTED

// 复刻线上 400：近期 Claude Code CLI 透传的整份 anthropic-beta header 里含 Vertex
// 不接受的 token（advisor-tool / prompt-caching-scope / redact-thinking /
// thinking-token-count）。Vertex builder 必须剥掉它们，否则上游 HTTP 400（issue #3358）。
// 本用例在 Commit 1 之前 FAIL、之后 PASS。
func TestVertexBetaFilter_StripsUnsupportedClaudeCodeTokens(t *testing.T) {
	c := newVertexBetaTestContext(t,
		"claude-code-20250219,oauth-2025-04-20,REDACTED,"+
			"advisor-tool-2026-03-01,prompt-caching-scope-2026-01-05,"+
			"redact-thinking-2026-02-12,thinking-token-count-2026-05-13,"+
			"context-management-2025-06-27")

	body := []byte(`{"model":"claude-opus-4-7","max_tokens":32,"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`)

	svc := &GatewayService{REDACTED
	req, _, err := svc.buildUpstreamRequest(
		context.Background(), c, newVertexServiceAccount(401), body,
		"vertex-token", "service_account", "claude-opus-4-7@20260417", false, false,
	)
REDACTED

	outBeta := getHeaderRaw(req.Header, "anthropic-beta")

	// Vertex 拒绝的 token 必须全部剥掉。
	for _, bad := range []string{
		"advisor-tool-2026-03-01",
		"prompt-caching-scope-2026-01-05",
		"redact-thinking-2026-02-12",
		"thinking-token-count-2026-05-13",
		// 客户端身份 beta：Vertex service_account 不需要，亦不在白名单。
		"claude-code-20250219",
		"oauth-2025-04-20",
REDACTED {
		require.False(t, anthropicBetaTokensContains(outBeta, bad),
			"token %q 必须被剥离；实际 outgoing beta=%q", bad, outBeta)
REDACTED

	// 白名单内的 token 必须保留。
	for _, keep := range []string{
		"REDACTED",
		"context-management-2025-06-27",
REDACTED {
		require.True(t, anthropicBetaTokensContains(outBeta, keep),
			"token %q 应保留；实际 outgoing beta=%q", keep, outBeta)
REDACTED
REDACTED

// 全部 token 都不受 Vertex 支持时，outgoing header 不应下发 anthropic-beta。
func TestVertexBetaFilter_DropsHeaderWhenAllUnsupported(t *testing.T) {
	c := newVertexBetaTestContext(t,
		"prompt-caching-scope-2026-01-05,redact-thinking-2026-02-12")

	body := []byte(`{"model":"claude-opus-4-7","max_tokens":32,"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`)

	svc := &GatewayService{REDACTED
	req, _, err := svc.buildUpstreamRequest(
		context.Background(), c, newVertexServiceAccount(402), body,
		"vertex-token", "service_account", "claude-opus-4-7@20260417", false, false,
	)
REDACTED

	require.Empty(t, getHeaderRaw(req.Header, "anthropic-beta"),
		"所有 token 被剥离后不应残留 anthropic-beta header")
REDACTED

// 能力维度 sanitize 以「最终 beta」为准：客户端只带不支持的 prompt-caching-scope（会被剥光），
// body 又带 context_management → 因最终 header 不含 context-management beta，body 字段必须 strip。
// 证明 sanitize 不再以原始 client 值为准（修复前用 clientBeta，会错误保留 context_management）。
func TestVertexBetaFilter_BodySanitizeKeysOnFinalBeta(t *testing.T) {
	c := newVertexBetaTestContext(t, "prompt-caching-scope-2026-01-05")

	body := []byte(`{"model":"claude-opus-4-7","context_management":{"edits":[{"type":"clear_thinking_20251015"REDACTED]REDACTED,"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`)

	svc := &GatewayService{REDACTED
	req, _, err := svc.buildUpstreamRequest(
		context.Background(), c, newVertexServiceAccount(403), body,
		"vertex-token", "service_account", "claude-opus-4-7@20260417", false, false,
	)
REDACTED

	got := readRequestBodyForTest(t, req)
	require.False(t, gjson.GetBytes(got, "context_management").Exists(),
		"最终 beta 不含 context-management 时 body.context_management 必须被 strip")
	require.Empty(t, getHeaderRaw(req.Header, "anthropic-beta"))
REDACTED

// BetaPolicy block 规则在 Vertex 路径同样生效：管理员 block 某 token，客户端带它 → 直接报错。
func TestVertexBetaFilter_BlocksViaBetaPolicy(t *testing.T) {
	settings := &BetaPolicySettings{
		Rules: []BetaPolicyRule{
			{
				BetaToken:    "context-management-2025-06-27",
				Action:       BetaPolicyActionBlock,
				Scope:        BetaPolicyScopeAll,
				ErrorMessage: "context management is blocked",
		REDACTED,
	REDACTED,
REDACTED
	raw, err := json.Marshal(settings)
REDACTED

	svc := &GatewayService{
		settingService: NewSettingService(
			&betaPolicySettingRepoStub{values: map[string]string{
				SettingKeyBetaPolicySettings: string(raw),
	REDACTED
			&config.Config{REDACTED,
		),
REDACTED

	c := newVertexBetaTestContext(t,
		"REDACTED,context-management-2025-06-27")
	body := []byte(`{"model":"claude-opus-4-7","max_tokens":32,"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`)

	_, _, err = svc.buildUpstreamRequest(
		context.Background(), c, newVertexServiceAccount(404), body,
		"vertex-token", "service_account", "claude-opus-4-7@20260417", false, false,
	)
REDACTED
	var blocked *BetaBlockedError
	require.True(t, errors.As(err, &blocked), "expected *BetaBlockedError, got %T", err)
	require.Equal(t, "context management is blocked", err.Error())
REDACTED

// filterVertexBetaTokens 单元测试：白名单过滤 + drop 集合 + 去重 + 空输入。
func TestFilterVertexBetaTokens(t *testing.T) {
	t.Run("whitelist filters unsupported", func(t *testing.T) {
		out := filterVertexBetaTokens(
			"REDACTED,prompt-caching-scope-2026-01-05,context-management-2025-06-27",
			nil,
		)
		require.Equal(t, "REDACTED,context-management-2025-06-27", out)
REDACTED)

	t.Run("drop set strips before whitelist", func(t *testing.T) {
		out := filterVertexBetaTokens(
			"REDACTED,context-management-2025-06-27",
			map[string]struct{REDACTED{"context-management-2025-06-27": {REDACTEDREDACTED,
		)
		require.Equal(t, "REDACTED", out)
REDACTED)

	t.Run("dedupe", func(t *testing.T) {
		out := filterVertexBetaTokens(
			"context-1m-2025-08-07,context-1m-2025-08-07",
			nil,
		)
		require.Equal(t, "context-1m-2025-08-07", out)
REDACTED)

	t.Run("empty input", func(t *testing.T) {
		require.Empty(t, filterVertexBetaTokens("", nil))
		require.Empty(t, filterVertexBetaTokens("prompt-caching-scope-2026-01-05", nil))
REDACTED)
REDACTED
