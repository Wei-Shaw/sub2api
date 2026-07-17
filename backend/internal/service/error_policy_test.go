//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// TestCheckErrorPolicy — 6 table-driven cases for the pure logic function
// ---------------------------------------------------------------------------

func TestCheckErrorPolicy(t *testing.T) {
	tests := []struct {
		name       string
		account    *Account
		statusCode int
		body       []byte
		expected   ErrorPolicyResult
REDACTED{
		{
			name: "no_policy_oauth_returns_none",
			account: &Account{
				ID:       1,
				Type:     AccountTypeOAuth,
				Platform: PlatformAntigravity,
				// no custom error codes, no temp rules
		REDACTED,
			statusCode: 500,
			body:       []byte(`"error"`),
			expected:   ErrorPolicyNone,
	REDACTED,
		{
			name: "custom_error_codes_hit_returns_matched",
			account: &Account{
				ID:       2,
				Type:     AccountTypeAPIKey,
				Platform: PlatformAntigravity,
		REDACTED
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(429), float64(500)REDACTED,
			REDACTED,
		REDACTED,
			statusCode: 500,
			body:       []byte(`"error"`),
			expected:   ErrorPolicyMatched,
	REDACTED,
		{
			name: "custom_error_codes_miss_returns_skipped",
			account: &Account{
				ID:       3,
				Type:     AccountTypeAPIKey,
				Platform: PlatformAntigravity,
		REDACTED
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(429), float64(500)REDACTED,
			REDACTED,
		REDACTED,
			statusCode: 503,
			body:       []byte(`"error"`),
			expected:   ErrorPolicySkipped,
	REDACTED,
		{
			name: "temp_unschedulable_hit_returns_temp_unscheduled",
			account: &Account{
				ID:       4,
				Type:     AccountTypeOAuth,
				Platform: PlatformAntigravity,
		REDACTED
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(503),
							"keywords":         []any{"overloaded"REDACTED,
							"duration_minutes": float64(10),
							"description":      "overloaded rule",
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			statusCode: 503,
			body:       []byte(`overloaded service`),
			expected:   ErrorPolicyTempUnscheduled,
	REDACTED,
		{
			name: "temp_unschedulable_401_first_hit_returns_temp_unscheduled",
			account: &Account{
				ID:       14,
				Type:     AccountTypeOAuth,
				Platform: PlatformAntigravity,
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
			expected:   ErrorPolicyTempUnscheduled,
	REDACTED,
		{
			// Antigravity 401 不走升级逻辑（由 applyErrorPolicy 的 temp_unschedulable_rules 自行控制），
			// second hit 仍然返回 TempUnscheduled。
			name: "temp_unschedulable_401_second_hit_antigravity_stays_temp",
			account: &Account{
				ID:                      15,
				Type:                    AccountTypeOAuth,
				Platform:                PlatformAntigravity,
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
			expected:   ErrorPolicyTempUnscheduled,
	REDACTED,
		{
			name: "temp_unschedulable_body_miss_returns_none",
			account: &Account{
				ID:       5,
				Type:     AccountTypeOAuth,
				Platform: PlatformAntigravity,
		REDACTED
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(503),
							"keywords":         []any{"overloaded"REDACTED,
							"duration_minutes": float64(10),
							"description":      "overloaded rule",
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			statusCode: 503,
			body:       []byte(`random msg`),
			expected:   ErrorPolicyNone,
	REDACTED,
		{
			name: "custom_error_codes_override_temp_unschedulable",
			account: &Account{
				ID:       6,
				Type:     AccountTypeAPIKey,
				Platform: PlatformAntigravity,
		REDACTED
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(503)REDACTED,
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(503),
							"keywords":         []any{"overloaded"REDACTED,
							"duration_minutes": float64(10),
							"description":      "overloaded rule",
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			statusCode: 503,
			body:       []byte(`overloaded`),
			expected:   ErrorPolicyMatched, // custom codes take precedence
	REDACTED,
		{
			name: "pool_mode_custom_error_codes_hit_returns_matched",
			account: &Account{
				ID:       7,
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
		REDACTED
					"pool_mode":                  true,
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(401), float64(403)REDACTED,
			REDACTED,
		REDACTED,
			statusCode: 401,
			body:       []byte(`unauthorized`),
			expected:   ErrorPolicyMatched,
	REDACTED,
		{
			name: "pool_mode_without_custom_error_codes_returns_skipped",
			account: &Account{
				ID:       8,
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
		REDACTED
					"pool_mode": true,
			REDACTED,
		REDACTED,
			statusCode: 401,
			body:       []byte(`unauthorized`),
			expected:   ErrorPolicySkipped,
	REDACTED,
		{
			name: "pool_mode_temp_unschedulable_hit_returns_temp_unscheduled",
			account: &Account{
				ID:       9,
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
		REDACTED
					"pool_mode":                  true,
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(http.StatusServiceUnavailable),
							"keywords":         []any{"unavailable"REDACTED,
							"duration_minutes": float64(30),
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			statusCode: http.StatusServiceUnavailable,
			body:       []byte(`Service temporarily unavailable`),
			expected:   ErrorPolicyTempUnscheduled,
	REDACTED,
		{
			name: "pool_mode_temp_unschedulable_miss_returns_skipped",
			account: &Account{
				ID:       10,
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
		REDACTED
					"pool_mode":                  true,
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(http.StatusServiceUnavailable),
							"keywords":         []any{"maintenance"REDACTED,
							"duration_minutes": float64(30),
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			statusCode: http.StatusServiceUnavailable,
			body:       []byte(`Service temporarily unavailable`),
			expected:   ErrorPolicySkipped,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &errorPolicyRepoStub{REDACTED
			svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)

			result := svc.CheckErrorPolicy(context.Background(), tt.account, tt.statusCode, tt.body)
			require.Equal(t, tt.expected, result, "unexpected ErrorPolicyResult")
	REDACTED)
REDACTED
REDACTED

func TestHandleUpstreamError_PoolModePolicies(t *testing.T) {
	t.Run("pool_mode_without_custom_error_codes_still_skips", func(t *testing.T) {
		repo := &errorPolicyRepoStub{REDACTED
		svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
		account := &Account{
			ID:       30,
			Type:     AccountTypeAPIKey,
			Platform: PlatformOpenAI,
	REDACTED
				"pool_mode": true,
		REDACTED,
	REDACTED

		shouldDisable := svc.HandleUpstreamError(context.Background(), account, 401, http.Header{REDACTED, []byte("unauthorized"))

		require.False(t, shouldDisable)
		require.Equal(t, 0, repo.setErrCalls)
		require.Equal(t, 0, repo.tempCalls)
REDACTED)

	t.Run("pool_mode_with_custom_error_codes_uses_local_error_policy", func(t *testing.T) {
		repo := &errorPolicyRepoStub{REDACTED
		svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
		account := &Account{
			ID:       31,
			Type:     AccountTypeAPIKey,
			Platform: PlatformOpenAI,
	REDACTED
				"pool_mode":                  true,
				"custom_error_codes_enabled": true,
				"custom_error_codes":         []any{float64(401)REDACTED,
		REDACTED,
	REDACTED

		shouldDisable := svc.HandleUpstreamError(context.Background(), account, 401, http.Header{REDACTED, []byte("unauthorized"))

		require.True(t, shouldDisable)
		require.Equal(t, 1, repo.setErrCalls)
		require.Equal(t, 0, repo.tempCalls)
REDACTED)

	t.Run("pool_mode_explicit_temp_rule_stops_scheduling", func(t *testing.T) {
		repo := &errorPolicyRepoStub{REDACTED
		svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
		account := &Account{
			ID:       32,
			Type:     AccountTypeAPIKey,
			Platform: PlatformOpenAI,
	REDACTED
				"pool_mode":                  true,
				"temp_unschedulable_enabled": true,
				"temp_unschedulable_rules": []any{
					map[string]any{
						"error_code":       float64(http.StatusServiceUnavailable),
						"keywords":         []any{"unavailable"REDACTED,
						"duration_minutes": float64(30),
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED

		shouldDisable := svc.HandleUpstreamError(
			context.Background(),
			account,
			http.StatusServiceUnavailable,
			http.Header{REDACTED,
			[]byte("Service temporarily unavailable"),
		)

		require.True(t, shouldDisable)
		require.Equal(t, 0, repo.setErrCalls)
		require.Equal(t, 1, repo.tempCalls)
REDACTED)

	t.Run("pool_mode_temp_rule_miss_still_skips", func(t *testing.T) {
		repo := &errorPolicyRepoStub{REDACTED
		svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
		account := &Account{
			ID:       33,
			Type:     AccountTypeAPIKey,
			Platform: PlatformOpenAI,
	REDACTED
				"pool_mode":                  true,
				"temp_unschedulable_enabled": true,
				"temp_unschedulable_rules": []any{
					map[string]any{
						"error_code":       float64(http.StatusServiceUnavailable),
						"keywords":         []any{"maintenance"REDACTED,
						"duration_minutes": float64(30),
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED

		shouldDisable := svc.HandleUpstreamError(
			context.Background(),
			account,
			http.StatusServiceUnavailable,
			http.Header{REDACTED,
			[]byte("Service temporarily unavailable"),
		)

		require.False(t, shouldDisable)
		require.Equal(t, 0, repo.setErrCalls)
		require.Equal(t, 0, repo.tempCalls)
REDACTED)
REDACTED

// ---------------------------------------------------------------------------
// TestApplyErrorPolicy — 4 table-driven cases for the wrapper method
// ---------------------------------------------------------------------------

func TestApplyErrorPolicy(t *testing.T) {
	tests := []struct {
		name              string
		account           *Account
		statusCode        int
		body              []byte
		expectedHandled   bool
		expectedStatus    int  // expected outStatus
		expectedSwitchErr bool // expect *AntigravityAccountSwitchError
		handleErrorCalls  int
REDACTED{
		{
			name: "none_not_handled",
			account: &Account{
				ID:       10,
				Type:     AccountTypeOAuth,
				Platform: PlatformAntigravity,
		REDACTED,
			statusCode:       500,
			body:             []byte(`"error"`),
			expectedHandled:  false,
			expectedStatus:   500, // passthrough
			handleErrorCalls: 0,
	REDACTED,
		{
			name: "skipped_handled_no_handleError",
			account: &Account{
				ID:       11,
				Type:     AccountTypeAPIKey,
				Platform: PlatformAntigravity,
		REDACTED
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(429)REDACTED,
			REDACTED,
		REDACTED,
			statusCode:       500, // not in custom codes
			body:             []byte(`"error"`),
			expectedHandled:  true,
			expectedStatus:   http.StatusInternalServerError, // skipped → 500
			handleErrorCalls: 0,
	REDACTED,
		{
			name: "matched_handled_calls_handleError",
			account: &Account{
				ID:       12,
				Type:     AccountTypeAPIKey,
				Platform: PlatformAntigravity,
		REDACTED
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(500)REDACTED,
			REDACTED,
		REDACTED,
			statusCode:       500,
			body:             []byte(`"error"`),
			expectedHandled:  true,
			expectedStatus:   500, // matched → original status
			handleErrorCalls: 1,
	REDACTED,
		{
			name: "temp_unscheduled_returns_switch_error",
			account: &Account{
				ID:       13,
				Type:     AccountTypeOAuth,
				Platform: PlatformAntigravity,
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
			body:              []byte(`overloaded`),
			expectedHandled:   true,
			expectedStatus:    503, // temp_unscheduled → original status
			expectedSwitchErr: true,
			handleErrorCalls:  0,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &errorPolicyRepoStub{REDACTED
			rlSvc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
			svc := &AntigravityGatewayService{
				rateLimitService: rlSvc,
		REDACTED

			var handleErrorCount int
			p := antigravityRetryLoopParams{
				ctx:     context.Background(),
				prefix:  "[test]",
				account: tt.account,
				handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
					handleErrorCount++
					return nil
			REDACTED,
				isStickySession: true,
		REDACTED

			handled, outStatus, retErr := svc.applyErrorPolicy(p, tt.statusCode, http.Header{REDACTED, tt.body)

			require.Equal(t, tt.expectedHandled, handled, "handled mismatch")
			require.Equal(t, tt.expectedStatus, outStatus, "outStatus mismatch")
			require.Equal(t, tt.handleErrorCalls, handleErrorCount, "handleError call count mismatch")

			if tt.expectedSwitchErr {
				var switchErr *AntigravityAccountSwitchError
				require.ErrorAs(t, retErr, &switchErr)
				require.Equal(t, tt.account.ID, switchErr.OriginalAccountID)
		REDACTED else {
				require.NoError(t, retErr)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestApplyErrorPolicy_GeminiRateLimitBypassesCustomSkip(t *testing.T) {
	repo := &stubAntigravityAccountRepo{REDACTED
	cache := &stubSmartRetryCache{REDACTED
	rlSvc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
	svc := &AntigravityGatewayService{
		rateLimitService: rlSvc,
		accountRepo:      repo,
		cache:            cache,
REDACTED

	account := &Account{
		ID:       31,
		Type:     AccountTypeAPIKey,
		Platform: PlatformAntigravity,
REDACTED
			"custom_error_codes_enabled": true,
			"custom_error_codes":         []any{float64(500)REDACTED,
	REDACTED,
REDACTED
	body := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-flash"REDACTED, "reason": "RATE_LIMIT_EXCEEDED"REDACTED,
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "15s"REDACTED
			]
	REDACTED
REDACTED`)
	p := antigravityRetryLoopParams{
		ctx:         context.Background(),
		prefix:      "[test]",
		account:     account,
		accountRepo: repo,
		groupID:     42,
		sessionHash: "gemini:sticky",
		handleError: func(context.Context, string, *Account, int, http.Header, []byte, string, int64, string, bool) *handleModelRateLimitResult {
			t.Fatal("model rate limit should be handled before custom error fallback")
			return nil
	REDACTED,
REDACTED

	handled, outStatus, retErr := svc.applyErrorPolicy(p, http.StatusTooManyRequests, http.Header{REDACTED, body)

	require.True(t, handled)
	require.Equal(t, http.StatusTooManyRequests, outStatus)
	require.NoError(t, retErr)
	require.Len(t, repo.modelRateLimitCalls, 2)
	require.Equal(t, "gemini-3-flash", repo.modelRateLimitCalls[0].modelKey)
	require.Equal(t, antigravityGeminiModelRateLimitKey, repo.modelRateLimitCalls[1].modelKey)
	require.Len(t, cache.deleteCalls, 1)
	require.Equal(t, int64(42), cache.deleteCalls[0].groupID)
	require.Equal(t, "gemini:sticky", cache.deleteCalls[0].sessionHash)
REDACTED

// ---------------------------------------------------------------------------
// errorPolicyRepoStub — minimal AccountRepository stub for error policy tests
// ---------------------------------------------------------------------------

type errorPolicyRepoStub struct {
	mockAccountRepoForGemini
	tempCalls    int
	setErrCalls  int
	lastErrorMsg string
REDACTED

func (r *errorPolicyRepoStub) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	r.tempCalls++
	return nil
REDACTED

func (r *errorPolicyRepoStub) SetError(ctx context.Context, id int64, errorMsg string) error {
	r.setErrCalls++
	r.lastErrorMsg = errorMsg
	return nil
REDACTED
