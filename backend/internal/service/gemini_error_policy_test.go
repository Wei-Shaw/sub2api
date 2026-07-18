//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// TestShouldFailoverGeminiUpstreamError — verifies the failover decision
// for the ErrorPolicyNone path (original logic preserved).
// ---------------------------------------------------------------------------

func TestShouldFailoverGeminiUpstreamError(t *testing.T) {
	svc := &GeminiMessagesCompatService{REDACTED

	tests := []struct {
		name       string
		statusCode int
		expected   bool
REDACTED{
		{"401_failover", 401, trueREDACTED,
		{"403_failover", 403, trueREDACTED,
		{"429_failover", 429, trueREDACTED,
		{"529_failover", 529, trueREDACTED,
		{"500_failover", 500, trueREDACTED,
		{"502_failover", 502, trueREDACTED,
		{"503_failover", 503, trueREDACTED,
		{"400_no_failover", 400, falseREDACTED,
		{"404_no_failover", 404, falseREDACTED,
		{"422_no_failover", 422, falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.shouldFailoverGeminiUpstreamError(tt.statusCode)
			require.Equal(t, tt.expected, got)
	REDACTED)
REDACTED
REDACTED

// ---------------------------------------------------------------------------
// TestCheckErrorPolicy_GeminiAccounts — verifies CheckErrorPolicy works
// correctly for Gemini platform accounts (API Key type).
// ---------------------------------------------------------------------------

func TestCheckErrorPolicy_GeminiAccounts(t *testing.T) {
	tests := []struct {
		name       string
		account    *Account
		statusCode int
		body       []byte
		expected   ErrorPolicyResult
REDACTED{
		{
			name: "gemini_apikey_custom_codes_hit",
			account: &Account{
				ID:       100,
				Type:     AccountTypeAPIKey,
		REDACTED
		REDACTED
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(429), float64(500)REDACTED,
			REDACTED,
		REDACTED,
			statusCode: 429,
			body:       []byte(`{"error":"rate limited"REDACTED`),
			expected:   ErrorPolicyMatched,
	REDACTED,
		{
			name: "gemini_apikey_custom_codes_miss",
			account: &Account{
				ID:       101,
				Type:     AccountTypeAPIKey,
		REDACTED
		REDACTED
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(429)REDACTED,
			REDACTED,
		REDACTED,
			statusCode: 500,
			body:       []byte(`{"error":"internal"REDACTED`),
			expected:   ErrorPolicySkipped,
	REDACTED,
		{
			name: "gemini_apikey_no_custom_codes_returns_none",
			account: &Account{
				ID:       102,
				Type:     AccountTypeAPIKey,
		REDACTED
		REDACTED,
			statusCode: 500,
			body:       []byte(`{"error":"internal"REDACTED`),
			expected:   ErrorPolicyNone,
	REDACTED,
		{
			name: "gemini_apikey_temp_unschedulable_hit",
			account: &Account{
				ID:       103,
				Type:     AccountTypeAPIKey,
		REDACTED
		REDACTED
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(503),
							"keywords":         []any{"overloaded"REDACTED,
							"duration_minutes": float64(10),
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			statusCode: 503,
			body:       []byte(`overloaded service`),
			expected:   ErrorPolicyTempUnscheduled,
	REDACTED,
		{
			name: "gemini_apikey_temp_unschedulable_401_second_hit_returns_none",
			account: &Account{
				ID:                      105,
				Type:                    AccountTypeAPIKey,
				Platform:                PlatformGemini,
				TempUnschedulableReason: `{"status_code":401,"until_unix":1735689600REDACTED`,
		REDACTED
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(401),
							"keywords":         []any{"unauthorized"REDACTED,
							"duration_minutes": float64(10),
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			statusCode: 401,
			body:       []byte(`unauthorized`),
			expected:   ErrorPolicyNone,
	REDACTED,
		{
			name: "gemini_custom_codes_override_temp_unschedulable",
			account: &Account{
				ID:       104,
				Type:     AccountTypeAPIKey,
		REDACTED
		REDACTED
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(503)REDACTED,
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(503),
							"keywords":         []any{"overloaded"REDACTED,
							"duration_minutes": float64(10),
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			statusCode: 503,
			body:       []byte(`overloaded`),
			expected:   ErrorPolicyMatched, // custom codes take precedence
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &errorPolicyRepoStub{REDACTED
			svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)

			result := svc.CheckErrorPolicy(context.Background(), tt.account, tt.statusCode, tt.body)
			require.Equal(t, tt.expected, result)
	REDACTED)
REDACTED
REDACTED

// ---------------------------------------------------------------------------
// TestGeminiErrorPolicyIntegration — verifies the Gemini error handling
// paths produce the correct behavior for each ErrorPolicyResult.
//
// These tests simulate the inline error policy switch in handleClaudeCompat
// and forwardNativeGemini by calling the same methods in the same order.
// ---------------------------------------------------------------------------

func TestGeminiErrorPolicyIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name                 string
		account              *Account
		statusCode           int
		respBody             []byte
		expectFailover       bool // expect UpstreamFailoverError
		expectHandleError    bool // expect handleGeminiUpstreamError to be called
		expectShouldFailover bool // for None path, whether shouldFailover triggers
		expectModelScope     string
REDACTED{
		{
			name: "custom_codes_matched_429_failover",
			account: &Account{
				ID:       200,
				Type:     AccountTypeAPIKey,
		REDACTED
		REDACTED
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(429)REDACTED,
			REDACTED,
		REDACTED,
			statusCode:        429,
			respBody:          []byte(`{"error":"rate limited"REDACTED`),
			expectFailover:    true,
			expectHandleError: true,
	REDACTED,
		{
			name: "custom_codes_skipped_500_no_failover",
			account: &Account{
				ID:       201,
				Type:     AccountTypeAPIKey,
		REDACTED
		REDACTED
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(429)REDACTED,
			REDACTED,
		REDACTED,
			statusCode:        500,
			respBody:          []byte(`{"error":"internal"REDACTED`),
			expectFailover:    false,
			expectHandleError: false,
	REDACTED,
		{
			name: "temp_unschedulable_matched_failover",
			account: &Account{
				ID:       202,
				Type:     AccountTypeAPIKey,
		REDACTED
		REDACTED
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(503),
							"keywords":         []any{"overloaded"REDACTED,
							"duration_minutes": float64(10),
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			statusCode:        503,
			respBody:          []byte(`overloaded`),
			expectFailover:    true,
			expectHandleError: false,
			expectModelScope:  "gemini-2.5-pro",
	REDACTED,
		{
			name: "no_policy_429_failover_via_shouldFailover",
			account: &Account{
				ID:       203,
				Type:     AccountTypeAPIKey,
		REDACTED
		REDACTED,
			statusCode:           429,
			respBody:             []byte(`{"error":"rate limited"REDACTED`),
			expectFailover:       true,
			expectHandleError:    true,
			expectShouldFailover: true,
	REDACTED,
		{
			name: "no_policy_400_no_failover",
			account: &Account{
				ID:       204,
				Type:     AccountTypeAPIKey,
		REDACTED
		REDACTED,
			statusCode:        400,
			respBody:          []byte(`{"error":"bad request"REDACTED`),
			expectFailover:    false,
			expectHandleError: true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &geminiErrorPolicyRepo{REDACTED
			rlSvc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
			svc := &GeminiMessagesCompatService{
				accountRepo:      repo,
				rateLimitService: rlSvc,
		REDACTED

			writer := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

			// Simulate the Claude compat error handling path (same logic as native).
			// This mirrors the inline switch in handleClaudeCompat.
			var handleErrorCalled bool
			var gotFailover bool

			ctx := context.Background()
			statusCode := tt.statusCode
			respBody := tt.respBody
			account := tt.account
			headers := http.Header{REDACTED

			if svc.rateLimitService != nil {
				policy := svc.rateLimitService.CheckErrorPolicy(ctx, account, statusCode, respBody, "gemini-2.5-pro")
				switch policy {
				case ErrorPolicySkipped:
					// Skipped → return error directly (no handleGeminiUpstreamError, no failover)
					gotFailover = false
					handleErrorCalled = false
					goto verify
				case ErrorPolicyMatched:
					svc.handleGeminiUpstreamError(ctx, account, statusCode, headers, respBody)
					handleErrorCalled = true
					gotFailover = true
					goto verify
				case ErrorPolicyTempUnscheduled:
					handleErrorCalled = false
					gotFailover = true
					goto verify
			REDACTED
		REDACTED

			// ErrorPolicyNone → original logic
			svc.handleGeminiUpstreamError(ctx, account, statusCode, headers, respBody)
			handleErrorCalled = true
			if svc.shouldFailoverGeminiUpstreamError(statusCode) {
				gotFailover = true
		REDACTED

		verify:
			require.Equal(t, tt.expectFailover, gotFailover, "failover mismatch")
			require.Equal(t, tt.expectHandleError, handleErrorCalled, "handleGeminiUpstreamError call mismatch")
			if tt.expectModelScope != "" {
				require.Equal(t, 1, repo.setModelRateLimitedCalls)
				require.Equal(t, tt.expectModelScope, repo.lastModelScope)
				require.Zero(t, repo.setTempCalls)
				require.Zero(t, repo.setRateLimitedCalls, "model temp rule must not be widened into an account rate limit")
		REDACTED

			if tt.expectShouldFailover {
				require.True(t, svc.shouldFailoverGeminiUpstreamError(statusCode),
					"shouldFailoverGeminiUpstreamError should return true for status %d", statusCode)
		REDACTED
	REDACTED)
REDACTED
REDACTED

// ---------------------------------------------------------------------------
// TestGeminiErrorPolicy_NilRateLimitService — verifies nil safety
// ---------------------------------------------------------------------------

func TestGeminiErrorPolicy_NilRateLimitService(t *testing.T) {
	svc := &GeminiMessagesCompatService{
		rateLimitService: nil,
REDACTED

	// When rateLimitService is nil, error policy is skipped → falls through to
	// shouldFailoverGeminiUpstreamError (original logic).
	// Verify this doesn't panic and follows expected behavior.

	ctx := context.Background()
	account := &Account{
		ID:       300,
		Type:     AccountTypeAPIKey,
REDACTED
REDACTED
			"custom_error_codes_enabled": true,
			"custom_error_codes":         []any{float64(429)REDACTED,
	REDACTED,
REDACTED

	// The nil check should prevent CheckErrorPolicy from being called
	if svc.rateLimitService != nil {
		t.Fatal("rateLimitService should be nil for this test")
REDACTED

	// shouldFailoverGeminiUpstreamError still works
	require.True(t, svc.shouldFailoverGeminiUpstreamError(429))
	require.False(t, svc.shouldFailoverGeminiUpstreamError(400))

	// handleGeminiUpstreamError should not panic with nil rateLimitService
	require.NotPanics(t, func() {
		svc.handleGeminiUpstreamError(ctx, account, 500, http.Header{REDACTED, []byte(`error`))
REDACTED)
REDACTED

// ---------------------------------------------------------------------------
// geminiErrorPolicyRepo — minimal AccountRepository stub for Gemini error
// policy tests. Embeds mockAccountRepoForGemini and adds tracking.
// ---------------------------------------------------------------------------

func TestHandleGeminiUpstreamError_GoogleOneCapacityExhaustedUsesTierCooldown(t *testing.T) {
	repo := &rateLimit429AccountRepoStub{REDACTED
	quotaSvc := NewGeminiQuotaService(&config.Config{REDACTED, nil)
	rlSvc := NewRateLimitService(repo, nil, &config.Config{REDACTED, quotaSvc, nil)
	svc := &GeminiMessagesCompatService{
		accountRepo:      repo,
		rateLimitService: rlSvc,
REDACTED

	account := &Account{
		ID:       511,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED
			"oauth_type": "google_one",
			"tier_id":    "google_ai_pro",
	REDACTED,
REDACTED
	body := []byte(`{"error":{"code":429,"details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","domain":"cloudcode-pa.googleapis.com","metadata":{"model":"gemini-3.1-pro-preview"REDACTED,"reason":"MODEL_CAPACITY_EXHAUSTED"REDACTED],"message":"No capacity available for model gemini-3.1-pro-preview on the server","status":"RESOURCE_EXHAUSTED"REDACTEDREDACTED`)

	before := time.Now()
	svc.handleGeminiUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{REDACTED, body)
	after := time.Now()

	require.Equal(t, 1, repo.rateLimitCalls)
	require.Equal(t, int64(511), repo.lastRateLimitID)
	require.WithinDuration(t, before.Add(5*time.Minute), repo.lastRateLimitReset, 2*time.Second)
	require.True(t, repo.lastRateLimitReset.After(before))
	require.True(t, repo.lastRateLimitReset.Before(after.Add(5*time.Minute).Add(2*time.Second)))
REDACTED

type geminiErrorPolicyRepo struct {
	mockAccountRepoForGemini
	setErrorCalls            int
	setRateLimitedCalls      int
	setTempCalls             int
	setModelRateLimitedCalls int
	lastModelScope           string
REDACTED

func (r *geminiErrorPolicyRepo) SetError(_ context.Context, _ int64, _ string) error {
	r.setErrorCalls++
	return nil
REDACTED

func (r *geminiErrorPolicyRepo) SetRateLimited(_ context.Context, _ int64, _ time.Time) error {
	r.setRateLimitedCalls++
	return nil
REDACTED

func (r *geminiErrorPolicyRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, _ string) error {
	r.setTempCalls++
	return nil
REDACTED

func (r *geminiErrorPolicyRepo) SetModelRateLimit(_ context.Context, _ int64, scope string, _ time.Time, _ ...string) error {
	r.setModelRateLimitedCalls++
	r.lastModelScope = scope
	return nil
REDACTED
