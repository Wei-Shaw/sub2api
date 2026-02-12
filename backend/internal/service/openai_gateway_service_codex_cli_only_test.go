package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubCodexRestrictionDetector struct {
	result CodexClientRestrictionDetectionResult
REDACTED

func (s *stubCodexRestrictionDetector) Detect(_ *gin.Context, _ *Account) CodexClientRestrictionDetectionResult {
	return s.result
REDACTED

func TestOpenAIGatewayService_GetCodexClientRestrictionDetector(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("使用注入的 detector", func(t *testing.T) {
		expected := &stubCodexRestrictionDetector{
			result: CodexClientRestrictionDetectionResult{Enabled: true, Matched: true, Reason: "stub"REDACTED,
	REDACTED
		svc := &OpenAIGatewayService{codexDetector: expectedREDACTED

		got := svc.getCodexClientRestrictionDetector()
		require.Same(t, expected, got)
REDACTED)

	t.Run("service 为 nil 时返回默认 detector", func(t *testing.T) {
		var svc *OpenAIGatewayService
		got := svc.getCodexClientRestrictionDetector()
		require.NotNil(t, got)
REDACTED)

	t.Run("service 未注入 detector 时返回默认 detector", func(t *testing.T) {
		svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: trueREDACTEDREDACTEDREDACTED
		got := svc.getCodexClientRestrictionDetector()
		require.NotNil(t, got)

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.Request.Header.Set("User-Agent", "curl/8.0")
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"codex_cli_only": trueREDACTEDREDACTED

		result := got.Detect(c, account)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonForceCodexCLI, result.Reason)
REDACTED)
REDACTED

func TestGetAPIKeyIDFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("context 为 nil", func(t *testing.T) {
		require.Equal(t, int64(0), getAPIKeyIDFromContext(nil))
REDACTED)

	t.Run("上下文没有 api_key", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		require.Equal(t, int64(0), getAPIKeyIDFromContext(c))
REDACTED)

	t.Run("api_key 类型错误", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Set("api_key", "not-api-key")
		require.Equal(t, int64(0), getAPIKeyIDFromContext(c))
REDACTED)

	t.Run("api_key 指针为空", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		var k *APIKey
		c.Set("api_key", k)
		require.Equal(t, int64(0), getAPIKeyIDFromContext(c))
REDACTED)

	t.Run("正常读取 api_key_id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Set("api_key", &APIKey{ID: 12345REDACTED)
		require.Equal(t, int64(12345), getAPIKeyIDFromContext(c))
REDACTED)
REDACTED

func TestLogCodexCLIOnlyDetection_NilSafety(t *testing.T) {
	// 不校验日志内容，仅保证在 nil 入参下不会 panic。
	require.NotPanics(t, func() {
		logCodexCLIOnlyDetection(context.TODO(), nil, 0, CodexClientRestrictionDetectionResult{Enabled: true, Matched: false, Reason: "test"REDACTED)
		logCodexCLIOnlyDetection(context.Background(), nil, 0, CodexClientRestrictionDetectionResult{Enabled: false, Matched: false, Reason: "disabled"REDACTED)
REDACTED)
REDACTED
