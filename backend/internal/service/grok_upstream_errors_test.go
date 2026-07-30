//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIsGrokContentPolicyRejection(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
REDACTED{
		{
			name:   "new sensitive code",
			status: http.StatusForbidden,
			body:   `{"error":{"code":"new_sensitive","message":"image is sensitive"REDACTEDREDACTED`,
			want:   true,
	REDACTED,
		{
			name:   "content policy violation code",
			status: http.StatusForbidden,
			body:   `{"response":{"error":{"code":"content_policy_violation"REDACTEDREDACTEDREDACTED`,
			want:   true,
	REDACTED,
		{
			name:   "cyber policy code",
			status: http.StatusForbidden,
			body:   `{"error":{"code":"cyber_policy","message":"request rejected"REDACTEDREDACTED`,
			want:   true,
	REDACTED,
		{
			name:   "moderation feature unavailable",
			status: http.StatusForbidden,
			body:   `{"error":{"message":"The moderation feature is not available for this request"REDACTEDREDACTED`,
			want:   true,
	REDACTED,
		{
			name:   "explicit prompt moderation rejection",
			status: http.StatusForbidden,
			body:   `{"error":{"message":"request rejected by content moderation"REDACTEDREDACTED`,
			want:   true,
	REDACTED,
		{
			name:   "entitlement forbidden",
			status: http.StatusForbidden,
			body:   `{"error":{"message":"subscription required"REDACTEDREDACTED`,
			want:   false,
	REDACTED,
		{
			name:   "account policy suspension is not request policy",
			status: http.StatusForbidden,
			body:   `{"error":{"message":"account suspended due to policy violation"REDACTEDREDACTED`,
			want:   false,
	REDACTED,
		{
			name:   "structured account suspension overrides policy reason",
			status: http.StatusForbidden,
			body:   `{"error":{"code":"account_suspended","reason":"policy_violation","message":"account suspended due to policy violation"REDACTEDREDACTED`,
			want:   false,
	REDACTED,
		{
			name:   "ambiguous policy violation code is not enough",
			status: http.StatusForbidden,
			body:   `{"error":{"code":"policy_violation","message":"policy violation"REDACTEDREDACTED`,
			want:   false,
	REDACTED,
		{
			name:   "policy violation with request scoped message",
			status: http.StatusForbidden,
			body:   `{"error":{"code":"policy_violation","message":"request blocked by policy"REDACTEDREDACTED`,
			want:   true,
	REDACTED,
		{
			name:   "wrong status",
			status: http.StatusBadRequest,
			body:   `{"error":{"code":"new_sensitive"REDACTEDREDACTED`,
			want:   false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isGrokContentPolicyRejection(tt.status, []byte(tt.body)))
	REDACTED)
REDACTED
REDACTED

func TestGrokContentPolicy403DoesNotMutateOrFailover(t *testing.T) {
	repo := &grokQuotaAccountRepo{REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	account := &Account{ID: 4715, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED
	body := []byte(`{"error":{"code":"new_sensitive","message":"text is sensitive"REDACTEDREDACTED`)

	svc.handleGrokAccountUpstreamError(context.Background(), account, http.StatusForbidden, nil, body)

	require.Zero(t, repo.tempUnschedCalls)
	require.Zero(t, repo.rateLimitedCalls)
	require.Zero(t, repo.updateCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.False(t, svc.shouldFailoverGrokUpstreamError(http.StatusForbidden, body))

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	resp := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{REDACTEDREDACTED
	got := svc.failoverOpenAIUpstreamHTTPError(context.Background(), c, account, resp, body, "text is sensitive", "grok-4.5")
	require.Nil(t, got)
	require.Zero(t, repo.tempUnschedCalls)
REDACTED

func TestGrokContentPolicy403SharedErrorFallbackDoesNotMutate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"error":{"code":"content_filter","message":"prohibited content"REDACTEDREDACTED`)
	repo := &grokQuotaAccountRepo{REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	account := &Account{
		ID:       4719,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
REDACTED
			"custom_error_codes_enabled": true,
			"custom_error_codes":         []any{float64(http.StatusTooManyRequests)REDACTED,
	REDACTED,
REDACTED

	newContext := func() (*gin.Context, *httptest.ResponseRecorder) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		return c, recorder
REDACTED

	c, recorder := newContext()
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(string(body))),
REDACTED
	_, err := svc.handleErrorResponse(context.Background(), resp, c, account, nil, "grok-4.5")
REDACTED
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid_request_error")

	c, recorder = newContext()
	resp = &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(string(body))),
REDACTED
	_, err = svc.handleCompatErrorResponse(resp, c, account, writeChatCompletionsError, "grok-4.5")
REDACTED
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid_request_error")

	require.Zero(t, repo.tempUnschedCalls)
	require.Zero(t, repo.rateLimitedCalls)
	require.Zero(t, repo.updateCalls)
REDACTED

func TestGrokContentPolicy403MediaResponseBypassesCustomErrorCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"error":{"code":"new_sensitive","message":"image is sensitive"REDACTEDREDACTED`
	repo := &grokQuotaAccountRepo{REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	account := &Account{
		ID:       4720,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
REDACTED
			"custom_error_codes_enabled": true,
			"custom_error_codes":         []any{float64(http.StatusTooManyRequests)REDACTED,
	REDACTED,
REDACTED
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(body)),
REDACTED

	_, err := svc.handleGrokMediaErrorResponse(context.Background(), resp, c, account, "request-id", "grok-imagine")
REDACTED
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid_request_error")
	require.Zero(t, repo.tempUnschedCalls)
	require.Zero(t, repo.rateLimitedCalls)
	require.Zero(t, repo.updateCalls)
REDACTED

func TestGrokContentPolicySSEErrorDoesNotMutateOrFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &grokQuotaAccountRepo{REDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"error\",\"error\":{\"code\":\"new_sensitive\",\"message\":\"text is sensitive\"REDACTEDREDACTED\n\n",
		)),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{accountRepo: repo, httpUpstream: upstreamREDACTED
	account := &Account{ID: 4721, Platform: PlatformGrok, Type: AccountTypeOAuth, Concurrency: 1REDACTED
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	payload := []byte(`{"type":"response.create","model":"grok-4.5","input":"hi"REDACTED`)
	var writes [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "access-token", payload, len(payload),
		"grok-4.5", "", "", "", "cache-id", 1,
		func(message []byte) error {
			writes = append(writes, append([]byte(nil), message...))
			return nil
	REDACTED,
	)

REDACTED
	require.NotNil(t, result)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Len(t, writes, 1)
	require.Contains(t, string(writes[0]), "new_sensitive")
	require.Zero(t, repo.tempUnschedCalls)
	require.Zero(t, repo.rateLimitedCalls)
	require.Zero(t, repo.updateCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestHandleGrokAccountUpstreamErrorEntitlement403KeepsDefaultCooldown(t *testing.T) {
	repo := &grokQuotaAccountRepo{REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	account := &Account{ID: 4716, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED
	before := time.Now()

	svc.handleGrokAccountUpstreamError(
		context.Background(), account, http.StatusForbidden, nil,
		[]byte(`{"error":{"message":"subscription required"REDACTEDREDACTED`),
	)

	require.Equal(t, 1, repo.tempUnschedCalls)
	require.Equal(t, "grok access or entitlement denied", repo.lastTempUnschedReason)
	require.Greater(t, repo.lastTempUnschedUntil, before.Add(29*time.Minute))
	require.Less(t, repo.lastTempUnschedUntil, before.Add(31*time.Minute))
REDACTED

func TestHandleGrokAccountUpstreamErrorEntitlement403RespectsPoolMode(t *testing.T) {
	t.Run("pool mode keeps scheduling state", func(t *testing.T) {
		repo := &grokQuotaAccountRepo{REDACTED
		svc := &OpenAIGatewayService{accountRepo: repoREDACTED
		account := &Account{
			ID:       4722,
			Platform: PlatformGrok,
			Type:     AccountTypeAPIKey,
	REDACTED
				"pool_mode": true,
		REDACTED,
	REDACTED
		body := []byte(`{"error":{"message":"grok access or entitlement denied"REDACTEDREDACTED`)

		svc.handleGrokAccountUpstreamError(
			context.Background(), account, http.StatusForbidden, nil,
			body,
		)

		require.Zero(t, repo.tempUnschedCalls)
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
		require.Nil(t, account.TempUnschedulableUntil)
		require.Empty(t, account.TempUnschedulableReason)
		require.True(t, svc.shouldFailoverGrokUpstreamError(http.StatusForbidden, body))
		require.True(t, account.IsPoolModeRetryableStatus(http.StatusForbidden))
REDACTED)

	t.Run("explicit temporary rule still applies", func(t *testing.T) {
		repo := &grokQuotaAccountRepo{REDACTED
		svc := &OpenAIGatewayService{accountRepo: repoREDACTED
		account := &Account{
			ID:       4723,
			Platform: PlatformGrok,
			Type:     AccountTypeAPIKey,
	REDACTED
				"pool_mode":                  true,
				"temp_unschedulable_enabled": true,
				"temp_unschedulable_rules": []any{
					map[string]any{
						"error_code":       float64(http.StatusForbidden),
						"keywords":         []any{"entitlement denied"REDACTED,
						"duration_minutes": float64(7),
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED
		before := time.Now()

		svc.handleGrokAccountUpstreamError(
			context.Background(), account, http.StatusForbidden, nil,
			[]byte(`{"error":{"message":"grok access or entitlement denied"REDACTEDREDACTED`),
		)

		require.Equal(t, 1, repo.tempUnschedCalls)
		require.Equal(t, "grok configured forbidden rule", repo.lastTempUnschedReason)
		require.WithinDuration(t, before.Add(7*time.Minute), repo.lastTempUnschedUntil, time.Second)
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED)
REDACTED

func TestHandleGrokAccountUpstreamError403UsesConfiguredRule(t *testing.T) {
	repo := &grokQuotaAccountRepo{REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	account := &Account{
		ID:       4717,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
REDACTED
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(http.StatusForbidden),
					"keywords":         []any{"subscription"REDACTED,
					"duration_minutes": float64(7),
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	before := time.Now()

	svc.handleGrokAccountUpstreamError(
		context.Background(), account, http.StatusForbidden, nil,
		[]byte(`{"error":{"message":"subscription required"REDACTEDREDACTED`),
	)

	require.Equal(t, 1, repo.tempUnschedCalls)
	require.Greater(t, repo.lastTempUnschedUntil, before.Add(6*time.Minute))
	require.Less(t, repo.lastTempUnschedUntil, before.Add(8*time.Minute))
REDACTED

func TestHandleGrokAccountUpstreamError403ConfiguredUnmatchedKeepsDefaultCooldown(t *testing.T) {
	repo := &grokQuotaAccountRepo{REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	account := &Account{
		ID:       4718,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
REDACTED
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(http.StatusForbidden),
					"keywords":         []any{"different failure"REDACTED,
					"duration_minutes": float64(7),
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	svc.handleGrokAccountUpstreamError(
		context.Background(), account, http.StatusForbidden, nil,
		[]byte(`{"error":{"message":"subscription required"REDACTEDREDACTED`),
	)

	require.Equal(t, 1, repo.tempUnschedCalls)
	require.Equal(t, "grok access or entitlement denied", repo.lastTempUnschedReason)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED
