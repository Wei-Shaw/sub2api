//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAI429FastPath_MarksOAuthAccountCoolingDown(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	apiKeyAccount := &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED

	shouldDisable := svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{REDACTED, nil)
	apiKeyShouldDisable := svc.handleOpenAIAccountUpstreamError(context.Background(), apiKeyAccount, http.StatusTooManyRequests, http.Header{REDACTED, nil)

	require.False(t, shouldDisable)
	require.False(t, apiKeyShouldDisable)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(apiKeyAccount))
REDACTED

// TestOpenAI429FastPath_SkipsSparkShadow 外审第8轮 P1:spark 影子被选中后若 /responses 返回 429,
// 不得按 global x-codex-* 信号写内存运行时熔断(否则 spark 被冷却到 global reset、单影子场景无可用账号)。
func TestOpenAI429FastPath_SkipsSparkShadow(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	parentID := int64(800)
	shadow := &Account{
		ID:              801,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
REDACTED
	normal := &Account{ID: 802, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED

	headers := http.Header{REDACTED
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "18000")
	headers.Set("x-codex-primary-window-minutes", "300")

	svc.markOpenAIOAuth429RateLimited(context.Background(), shadow, headers, nil)
	svc.markOpenAIOAuth429RateLimited(context.Background(), normal, headers, nil)

	require.False(t, svc.isOpenAIAccountRuntimeBlocked(shadow), "spark shadow must not be runtime-blocked by /responses global 429")
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(normal), "normal OpenAI OAuth account should still be runtime-blocked")
REDACTED

func TestOpenAIRuntimeBlock_AppliesToOpenAIAPIKeyWhenRateLimitServiceStopsScheduling(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	account := &Account{ID: 44, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED

	svc.BlockAccountScheduling(account, time.Time{REDACTED, "custom_error_code")

	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestOpenAIRuntimeBlock_DoesNotApplyToOtherPlatforms(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	account := &Account{ID: 45, Platform: PlatformGemini, Type: AccountTypeOAuthREDACTED

	svc.BlockAccountScheduling(account, time.Time{REDACTED, "custom_error_code")

	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestOpenAIRuntimeBlocker_IgnoresNonOpenAIFromRateLimitService(t *testing.T) {
	gateway := &OpenAIGatewayService{REDACTED
	repo := &rateLimitAccountRepoStub{REDACTED
	rateLimitService := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
	rateLimitService.SetAccountRuntimeBlocker(gateway)
	account := &Account{ID: 45, Platform: PlatformGemini, Type: AccountTypeOAuthREDACTED

	shouldDisable := rateLimitService.HandleUpstreamError(context.Background(), account, http.StatusForbidden, http.Header{REDACTED, []byte("forbidden"))

	require.True(t, shouldDisable)
	require.False(t, gateway.isOpenAIAccountRuntimeBlocked(account))
REDACTED

// 自 #4547（issue 4527 第4点）起，临时不可调度规则命中已知模型时按模型隔离：
// 只封 (账号, 模型) 对，不再账号级一刀切；未知模型仍走账号级兜底
// （见 TestOpenAITempUnschedulable_UnknownModelKeepsAccountRuntimeBlock）。
// 池模式规则仍然生效（issue 4470）：停止同账号重试并对命中模型设临时封锁。
func TestOpenAIPoolModeTempRule_StopsSameAccountRetryAndIsolatesBlockToModel(t *testing.T) {
	repo := &errorPolicyRepoStub{REDACTED
	rateLimitService := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
	gateway := &OpenAIGatewayService{
		cfg:              &config.Config{REDACTED,
		rateLimitService: rateLimitService,
REDACTED
	rateLimitService.SetAccountRuntimeBlocker(gateway)
	account := &Account{
		ID:          46,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
REDACTED
			"pool_mode":                    true,
			"pool_mode_retry_status_codes": []any{float64(http.StatusServiceUnavailable)REDACTED,
			"temp_unschedulable_enabled":   true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(http.StatusServiceUnavailable),
					"keywords":         []any{"unavailable"REDACTED,
					"duration_minutes": float64(30),
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	body := []byte(`{"error":{"message":"Service temporarily unavailable"REDACTEDREDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{REDACTED,
REDACTED

	failoverErr := gateway.failoverOpenAIUpstreamHTTPError(
		context.Background(),
		nil,
		account,
		resp,
		body,
		"Service temporarily unavailable",
		"gpt-5.4",
	)

	require.NotNil(t, failoverErr)
	require.False(t, failoverErr.RetryableOnSameAccount)
	require.Zero(t, repo.tempCalls)
	require.Equal(t, 0, repo.setErrCalls)
	require.Equal(t, StatusActive, account.Status)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "gpt-5.4", repo.modelRateLimitCalls[0].scope)
	require.False(t, gateway.isOpenAIAccountRuntimeBlocked(account))
	require.False(t, gateway.isOpenAIAccountRequestRuntimeBlocked(account, "gpt-5.5"))
REDACTED

func TestOpenAIModelNotFound_DoesNotRuntimeBlockWholeAccount(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &OpenAIGatewayService{
		rateLimitService: &RateLimitService{accountRepo: repoREDACTED,
REDACTED
	account := openAIModelNotFoundTempAccount()

	shouldDisable := svc.handleOpenAIAccountUpstreamError(
		context.Background(),
		account,
		http.StatusNotFound,
		http.Header{REDACTED,
		[]byte(`{"error":{"code":"model_not_found","message":"model not found"REDACTEDREDACTED`),
		"gpt-5.4",
	)

	require.True(t, shouldDisable)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Zero(t, repo.tempCalls)
	require.Len(t, repo.modelRateLimitCalls, 1)
REDACTED

func TestOpenAIModelTempUnschedulable_DoesNotRuntimeBlockWholeAccount(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &OpenAIGatewayService{
		rateLimitService: &RateLimitService{accountRepo: repoREDACTED,
REDACTED
	account := openAIModelNotFoundTempAccount()

	shouldDisable := svc.handleOpenAIAccountUpstreamError(
		context.Background(),
		account,
		http.StatusNotFound,
		http.Header{REDACTED,
		[]byte(`{"error":{"message":"endpoint not found"REDACTEDREDACTED`),
		"gpt-5.4",
	)

	require.True(t, shouldDisable)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Zero(t, repo.tempCalls)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "gpt-5.4", repo.modelRateLimitCalls[0].scope)
REDACTED

func TestOpenAIModelTempUnschedulable_WriteFailureDoesNotRuntimeBlockWholeAccount(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{modelRateLimitErr: errors.New("write failed")REDACTED
	svc := &OpenAIGatewayService{
		rateLimitService: &RateLimitService{accountRepo: repoREDACTED,
REDACTED
	account := openAIModelNotFoundTempAccount()

	shouldDisable := svc.handleOpenAIAccountUpstreamError(
		context.Background(),
		account,
		http.StatusNotFound,
		http.Header{REDACTED,
		[]byte(`{"error":{"message":"endpoint not found"REDACTEDREDACTED`),
		"gpt-5.4",
	)

	require.True(t, shouldDisable)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Zero(t, repo.tempCalls)
	require.Len(t, repo.modelRateLimitCalls, 1)
REDACTED

func TestOpenAIOAuth429_MatchingModelTempRuleAvoidsAccountRuntimeBlock(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &OpenAIGatewayService{
		rateLimitService: &RateLimitService{accountRepo: repoREDACTED,
REDACTED
	account := openAIModelNotFoundTempAccount()
	account.Type = AccountTypeOAuth
	account.Credentials["temp_unschedulable_rules"] = []any{
		map[string]any{
			"error_code":       float64(http.StatusTooManyRequests),
			"keywords":         []any{"model quota"REDACTED,
			"duration_minutes": float64(10),
	REDACTED,
REDACTED

	shouldDisable := svc.handleOpenAIAccountUpstreamError(
		context.Background(),
		account,
		http.StatusTooManyRequests,
		http.Header{REDACTED,
		[]byte(`{"error":{"message":"model quota exhausted"REDACTEDREDACTED`),
		"gpt-5.4",
	)

	require.True(t, shouldDisable)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "gpt-5.4", repo.modelRateLimitCalls[0].scope)
REDACTED

func TestOpenAIOAuth429_NonmatchingModelTempRuleKeepsAccountRuntimeBlock(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &OpenAIGatewayService{
		rateLimitService: &RateLimitService{accountRepo: repoREDACTED,
REDACTED
	account := openAIModelNotFoundTempAccount()
	account.Type = AccountTypeOAuth
	account.Credentials["temp_unschedulable_rules"] = []any{
		map[string]any{
			"error_code":       float64(http.StatusTooManyRequests),
			"keywords":         []any{"different marker"REDACTED,
			"duration_minutes": float64(10),
	REDACTED,
REDACTED

	shouldDisable := svc.handleOpenAIAccountUpstreamError(
		context.Background(),
		account,
		http.StatusTooManyRequests,
		http.Header{REDACTED,
		[]byte(`{"error":{"message":"global rate limit"REDACTEDREDACTED`),
		"gpt-5.4",
	)

	require.False(t, shouldDisable)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Empty(t, repo.modelRateLimitCalls)
REDACTED

func TestOpenAITempUnschedulable_UnknownModelKeepsAccountRuntimeBlock(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &OpenAIGatewayService{
		rateLimitService: &RateLimitService{accountRepo: repoREDACTED,
REDACTED
	account := openAIModelNotFoundTempAccount()

	shouldDisable := svc.handleOpenAIAccountUpstreamError(
		context.Background(),
		account,
		http.StatusNotFound,
		http.Header{REDACTED,
		[]byte(`{"error":{"message":"endpoint not found"REDACTEDREDACTED`),
	)

	require.True(t, shouldDisable)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Equal(t, 1, repo.tempCalls)
	require.Empty(t, repo.modelRateLimitCalls)
REDACTED

func TestOpenAIRuntimeBlock_DoesNotShortenExistingBlock(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	account := &Account{ID: 46, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	longUntil := time.Now().Add(10 * time.Minute)

	svc.BlockAccountScheduling(account, longUntil, "oauth_401")
	svc.BlockAccountScheduling(account, time.Time{REDACTED, "upstream_disable")

	value, ok := svc.openaiAccountRuntimeBlockUntil.Load(account.ID)
	require.True(t, ok)
	actualUntil, ok := value.(time.Time)
	require.True(t, ok)
	require.WithinDuration(t, longUntil, actualUntil, time.Second)
REDACTED

func TestOpenAIRuntimeBlock_ClearAccountSchedulingBlock(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	account := &Account{ID: 47, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED

	svc.BlockAccountScheduling(account, time.Now().Add(time.Minute), "429")
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))

	svc.ClearAccountSchedulingBlock(account.ID)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestShouldStopOpenAIOAuth429Failover_OnlyDuringStorm(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	apiKeyAccount := &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	var state OpenAIOAuth429FailoverState

	require.False(t, svc.ShouldStopOpenAIOAuth429Failover(account, http.StatusTooManyRequests, 1, &state))

	for i := 0; i < openAIOAuth429StormThreshold; i++ {
		svc.recordOpenAIOAuth429()
REDACTED

	require.True(t, svc.ShouldStopOpenAIOAuth429Failover(account, http.StatusTooManyRequests, 1, &state))
	require.False(t, svc.ShouldStopOpenAIOAuth429Failover(apiKeyAccount, http.StatusTooManyRequests, 1, &state))
	require.False(t, svc.ShouldStopOpenAIOAuth429Failover(account, http.StatusInternalServerError, 1, &state))
	require.False(t, svc.ShouldStopOpenAIOAuth429Failover(account, http.StatusTooManyRequests, 0, &state))
REDACTED

func TestShouldStopOpenAIOAuth429Failover_TracksOneGrokFollowupAttempt(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	account := &Account{ID: 44, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED
	apiKeyAccount := &Account{ID: 45, Platform: PlatformGrok, Type: AccountTypeAPIKeyREDACTED

	t.Run("429 then 500 stops after one followup", func(t *testing.T) {
		var state OpenAIOAuth429FailoverState
		require.False(t, svc.ShouldStopOpenAIOAuth429Failover(account, http.StatusTooManyRequests, 1, &state))
		require.True(t, svc.ShouldStopOpenAIOAuth429Failover(account, http.StatusInternalServerError, 2, &state))
REDACTED)

	t.Run("500 then 429 still allows one followup", func(t *testing.T) {
		var state OpenAIOAuth429FailoverState
		require.False(t, svc.ShouldStopOpenAIOAuth429Failover(account, http.StatusInternalServerError, 1, &state))
		require.False(t, svc.ShouldStopOpenAIOAuth429Failover(account, http.StatusTooManyRequests, 2, &state))
		require.True(t, svc.ShouldStopOpenAIOAuth429Failover(account, http.StatusBadGateway, 3, &state))
REDACTED)

	t.Run("OAuth 429 then API-key failure consumes the same followup", func(t *testing.T) {
		var state OpenAIOAuth429FailoverState
		require.False(t, svc.ShouldStopOpenAIOAuth429Failover(account, http.StatusTooManyRequests, 1, &state))
		require.True(t, svc.ShouldStopOpenAIOAuth429Failover(apiKeyAccount, http.StatusInternalServerError, 2, &state))
REDACTED)

	var state OpenAIOAuth429FailoverState
	require.False(t, svc.ShouldStopOpenAIOAuth429Failover(account, http.StatusTooManyRequests, 0, &state))
	require.False(t, svc.ShouldStopOpenAIOAuth429Failover(apiKeyAccount, http.StatusTooManyRequests, 2, &state))
REDACTED
