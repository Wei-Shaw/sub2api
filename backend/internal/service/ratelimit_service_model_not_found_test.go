//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type modelNotFoundRateLimitCall struct {
	accountID int64
	scope     string
	resetAt   time.Time
	reason    string
REDACTED

type modelNotFoundAccountRepoStub struct {
	mockAccountRepoForGemini
	tempCalls           int
	modelRateLimitCalls []modelNotFoundRateLimitCall
	modelRateLimitErr   error
REDACTED

func (r *modelNotFoundAccountRepoStub) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	r.tempCalls++
	return nil
REDACTED

func (r *modelNotFoundAccountRepoStub) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time, reason ...string) error {
	call := modelNotFoundRateLimitCall{
		accountID: id,
		scope:     scope,
		resetAt:   resetAt,
REDACTED
	if len(reason) > 0 {
		call.reason = reason[0]
REDACTED
	r.modelRateLimitCalls = append(r.modelRateLimitCalls, call)
	return r.modelRateLimitErr
REDACTED

func TestRateLimitService_HandleUpstreamError_ModelNotFoundUsesModelRateLimit(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &RateLimitService{accountRepo: repoREDACTED
	account := openAIModelNotFoundTempAccount()

	handled := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusNotFound,
		http.Header{REDACTED,
		[]byte(`{"error":{"code":"model_not_found","message":"model not found"REDACTEDREDACTED`),
		"gpt-5.4",
	)

	require.True(t, handled)
	require.Zero(t, repo.tempCalls)
	require.Len(t, repo.modelRateLimitCalls, 1)
	call := repo.modelRateLimitCalls[0]
	require.Equal(t, account.ID, call.accountID)
	require.Equal(t, "gpt-5.4", call.scope)
	require.Equal(t, upstreamModelNotFoundReason, call.reason)
	require.WithinDuration(t, time.Now().Add(upstreamModelNotFoundCooldown), call.resetAt, 5*time.Second)
REDACTED

func TestRateLimitService_HandleUpstreamError_ModelNotFoundWriteFailureDoesNotTempUnschedule(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{modelRateLimitErr: errors.New("write failed")REDACTED
	svc := &RateLimitService{accountRepo: repoREDACTED
	account := openAIModelNotFoundTempAccount()

	handled := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusNotFound,
		http.Header{REDACTED,
		[]byte(`{"error":{"code":"model_not_found","message":"model not found"REDACTEDREDACTED`),
		"gpt-5.4",
	)

	require.True(t, handled)
	require.Zero(t, repo.tempCalls)
	require.Len(t, repo.modelRateLimitCalls, 1)
REDACTED

func TestRateLimitService_HandleUpstreamError_Bare404UsesModelScopedTempUnschedulableWhenModelKnown(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &RateLimitService{accountRepo: repoREDACTED
	account := openAIModelNotFoundTempAccount()

	handled := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusNotFound,
		http.Header{REDACTED,
		[]byte(`{"error":{"message":"endpoint not found"REDACTEDREDACTED`),
		"gpt-5.4",
	)

	require.True(t, handled)
	require.Zero(t, repo.tempCalls)
	require.Len(t, repo.modelRateLimitCalls, 1)
	call := repo.modelRateLimitCalls[0]
	require.Equal(t, account.ID, call.accountID)
	require.Equal(t, "gpt-5.4", call.scope)
	require.WithinDuration(t, time.Now().Add(10*time.Minute), call.resetAt, 5*time.Second)

	var state TempUnschedState
	require.NoError(t, json.Unmarshal([]byte(call.reason), &state))
	require.Equal(t, http.StatusNotFound, state.StatusCode)
	require.Equal(t, "not found", state.MatchedKeyword)
REDACTED

func TestRateLimitService_HandleUpstreamError_Bare404WithoutModelKeepsAccountTempUnschedulable(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &RateLimitService{accountRepo: repoREDACTED
	account := openAIModelNotFoundTempAccount()

	handled := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusNotFound,
		http.Header{REDACTED,
		[]byte(`{"error":{"message":"endpoint not found"REDACTEDREDACTED`),
	)

	require.True(t, handled)
	require.Equal(t, 1, repo.tempCalls)
	require.Empty(t, repo.modelRateLimitCalls)
REDACTED

func TestRateLimitService_HandleUpstreamError_ModelTempWriteFailureNeverWidensToAccount(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{modelRateLimitErr: errors.New("write failed")REDACTED
	svc := &RateLimitService{accountRepo: repoREDACTED
	account := openAIModelNotFoundTempAccount()

	handled := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusNotFound,
		http.Header{REDACTED,
		[]byte(`{"error":{"message":"endpoint not found"REDACTEDREDACTED`),
		"gpt-5.4",
	)

	require.True(t, handled)
	require.Zero(t, repo.tempCalls)
	require.Len(t, repo.modelRateLimitCalls, 1)
REDACTED

func TestRateLimitService_HandleTempUnschedulable_PoolModeWithoutCustomPolicySkipsState(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &RateLimitService{accountRepo: repoREDACTED
	account := openAIModelNotFoundTempAccount()
	account.Credentials["pool_mode"] = true

	handled := svc.HandleTempUnschedulable(
		context.Background(),
		account,
		http.StatusNotFound,
		[]byte(`{"error":{"message":"endpoint not found"REDACTEDREDACTED`),
		"gpt-5.4",
	)

	require.False(t, handled)
	require.Zero(t, repo.tempCalls)
	require.Empty(t, repo.modelRateLimitCalls)
REDACTED

func TestRateLimitService_HandleTempUnschedulable_PoolModeCustomPolicyUsesModelScope(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &RateLimitService{accountRepo: repoREDACTED
	account := openAIModelNotFoundTempAccount()
	account.Credentials["pool_mode"] = true
	account.Credentials["custom_error_codes_enabled"] = true
	account.Credentials["custom_error_codes"] = []any{float64(http.StatusNotFound)REDACTED

	handled := svc.HandleTempUnschedulable(
		context.Background(),
		account,
		http.StatusNotFound,
		[]byte(`{"error":{"message":"endpoint not found"REDACTEDREDACTED`),
		"gpt-5.4",
	)

	require.True(t, handled)
	require.Zero(t, repo.tempCalls)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "gpt-5.4", repo.modelRateLimitCalls[0].scope)
REDACTED

func TestRateLimitService_TempUnschedulableContextPreservesModelForPoolDependency(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &RateLimitService{accountRepo: repoREDACTED
	account := openAIModelNotFoundTempAccount()
	account.Credentials["pool_mode"] = true
	ctx := withTempUnschedulableModel(context.Background(), []string{"gpt-5.4"REDACTED)

	// #4496 calls tryTempUnschedulable from the pool-mode branch without an
	// explicit model argument. The request context must preserve the canonical
	// model so that combined behavior remains model-scoped after it lands.
	handled := svc.tryTempUnschedulable(
		ctx,
		account,
		http.StatusNotFound,
		[]byte(`{"error":{"message":"endpoint not found"REDACTEDREDACTED`),
	)

	require.True(t, handled)
	require.Zero(t, repo.tempCalls)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "gpt-5.4", repo.modelRateLimitCalls[0].scope)
REDACTED

func TestRateLimitService_HandleUpstreamError_CustomPolicyExclusionSkipsAllState(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &RateLimitService{accountRepo: repoREDACTED
	account := openAIModelNotFoundTempAccount()
	account.Credentials["custom_error_codes_enabled"] = true
	account.Credentials["custom_error_codes"] = []any{float64(http.StatusServiceUnavailable)REDACTED

	handled := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusNotFound,
		http.Header{REDACTED,
		[]byte(`{"error":{"message":"endpoint not found"REDACTEDREDACTED`),
		"gpt-5.4",
	)

	require.False(t, handled)
	require.Zero(t, repo.tempCalls)
	require.Empty(t, repo.modelRateLimitCalls)
REDACTED

func TestRateLimitService_HandleTempUnschedulable_AuthenticationFailureStaysAccountScoped(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &RateLimitService{accountRepo: repoREDACTED
	account := openAIModelNotFoundTempAccount()
	account.TempUnschedulableReason = "legacy non-JSON reason"
	account.Credentials["temp_unschedulable_rules"] = []any{
		map[string]any{
			"error_code":       float64(http.StatusUnauthorized),
			"keywords":         []any{"unauthorized"REDACTED,
			"duration_minutes": float64(10),
	REDACTED,
REDACTED

	handled := svc.HandleTempUnschedulable(
		context.Background(),
		account,
		http.StatusUnauthorized,
		[]byte(`{"error":{"message":"unauthorized"REDACTEDREDACTED`),
		"gpt-5.4",
	)

	require.True(t, handled)
	require.Equal(t, 1, repo.tempCalls)
	require.Empty(t, repo.modelRateLimitCalls)
REDACTED

func TestRateLimitService_ModelTempUnschedulableIsolatesSchedulerByModel(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &RateLimitService{accountRepo: repoREDACTED
	account := openAIModelNotFoundTempAccount()
	account.Credentials["model_mapping"] = map[string]any{
		"public-a":   "upstream-a",
		"upstream-a": "upstream-b",
REDACTED

	handled := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusNotFound,
		http.Header{REDACTED,
		[]byte(`{"error":{"message":"endpoint not found"REDACTEDREDACTED`),
		"upstream-a",
	)

	require.True(t, handled)
	require.Len(t, repo.modelRateLimitCalls, 1)
	call := repo.modelRateLimitCalls[0]
	require.Equal(t, "upstream-a", call.scope, "canonical upstream model must not be mapped a second time")

	account.Extra = map[string]any{
		modelRateLimitsKey: map[string]any{
			call.scope: map[string]any{
				"rate_limit_reset_at": call.resetAt.UTC().Format(time.RFC3339),
		REDACTED,
	REDACTED,
REDACTED

	require.False(t, account.IsSchedulableForModelWithContext(context.Background(), "public-a"))
	require.True(t, account.IsSchedulableForModelWithContext(context.Background(), "gpt-5.6-sol"))
REDACTED

func openAIModelNotFoundTempAccount() *Account {
REDACTED
		ID:          101,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
REDACTED
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(http.StatusNotFound),
					"keywords":         []any{"not found"REDACTED,
					"duration_minutes": float64(10),
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
REDACTED

func TestRateLimitService_HandleUpstreamError_CodexPlanGatedModelUsesModelRateLimit(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &RateLimitService{accountRepo: repoREDACTED
	account := openAICodexPlanGatedOAuthAccount()

	handled := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusBadRequest,
		http.Header{REDACTED,
		[]byte(`{"detail":"The 'gpt-5.6-sol' model is not supported when using Codex with a ChatGPT account."REDACTED`),
		"gpt-5.6-sol",
	)

	require.True(t, handled)
	require.Zero(t, repo.tempCalls)
	require.Len(t, repo.modelRateLimitCalls, 1)
	call := repo.modelRateLimitCalls[0]
	require.Equal(t, account.ID, call.accountID)
	require.Equal(t, "gpt-5.6-sol", call.scope)
	require.Equal(t, upstreamCodexPlanGatedModelReason, call.reason)
	require.WithinDuration(t, time.Now().Add(upstreamCodexPlanGatedModelCooldown), call.resetAt, 5*time.Second)
REDACTED

func TestRateLimitService_HandleUpstreamError_CodexPlanGatedModelRespectsModelMapping(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &RateLimitService{accountRepo: repoREDACTED
	account := openAICodexPlanGatedOAuthAccount()
	account.Credentials["model_mapping"] = map[string]any{"gpt-5.6-sol": "gpt-5.6-sol-upstream"REDACTED

	handled := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusBadRequest,
		http.Header{REDACTED,
		[]byte(`{"detail":"The 'gpt-5.6-sol-upstream' model is not supported when using Codex with a ChatGPT account."REDACTED`),
		"gpt-5.6-sol",
	)

	require.True(t, handled)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "gpt-5.6-sol-upstream", repo.modelRateLimitCalls[0].scope)
REDACTED

func TestRateLimitService_HandleUpstreamError_CodexPlanGatedModelIgnoresAPIKeyAccount(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &RateLimitService{accountRepo: repoREDACTED
	account := openAICodexPlanGatedOAuthAccount()
	account.Type = AccountTypeAPIKey

	handled := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusBadRequest,
		http.Header{REDACTED,
		[]byte(`{"detail":"The 'gpt-5.6-sol' model is not supported when using Codex with a ChatGPT account."REDACTED`),
		"gpt-5.6-sol",
	)

	require.False(t, handled)
	require.Empty(t, repo.modelRateLimitCalls)
REDACTED

func openAICodexPlanGatedOAuthAccount() *Account {
REDACTED
		ID:          202,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
REDACTEDREDACTED,
REDACTED
REDACTED
