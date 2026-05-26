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
	svc := &GeminiMessagesCompatService{}

	tests := []struct {
		name       string
		statusCode int
		expected   bool
	}{
		{"401_failover", 401, true},
		{"403_failover", 403, true},
		{"429_failover", 429, true},
		{"529_failover", 529, true},
		{"500_failover", 500, true},
		{"502_failover", 502, true},
		{"503_failover", 503, true},
		{"400_no_failover", 400, false},
		{"404_no_failover", 404, false},
		{"422_no_failover", 422, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.shouldFailoverGeminiUpstreamError(tt.statusCode)
			require.Equal(t, tt.expected, got)
		})
	}
}

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
	}{
		{
			name: "gemini_apikey_custom_codes_hit",
			account: &Account{
				ID:       100,
				Type:     AccountTypeAPIKey,
				Platform: PlatformGemini,
				Credentials: map[string]any{
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(429), float64(500)},
				},
			},
			statusCode: 429,
			body:       []byte(`{"error":"rate limited"}`),
			expected:   ErrorPolicyMatched,
		},
		{
			name: "gemini_apikey_custom_codes_miss",
			account: &Account{
				ID:       101,
				Type:     AccountTypeAPIKey,
				Platform: PlatformGemini,
				Credentials: map[string]any{
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(429)},
				},
			},
			statusCode: 500,
			body:       []byte(`{"error":"internal"}`),
			expected:   ErrorPolicySkipped,
		},
		{
			name: "gemini_apikey_no_custom_codes_returns_none",
			account: &Account{
				ID:       102,
				Type:     AccountTypeAPIKey,
				Platform: PlatformGemini,
			},
			statusCode: 500,
			body:       []byte(`{"error":"internal"}`),
			expected:   ErrorPolicyNone,
		},
		{
			name: "gemini_apikey_temp_unschedulable_hit",
			account: &Account{
				ID:       103,
				Type:     AccountTypeAPIKey,
				Platform: PlatformGemini,
				Credentials: map[string]any{
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(503),
							"keywords":         []any{"overloaded"},
							"duration_minutes": float64(10),
						},
					},
				},
			},
			statusCode: 503,
			body:       []byte(`overloaded service`),
			expected:   ErrorPolicyTempUnscheduled,
		},
		{
			name: "gemini_apikey_temp_unschedulable_401_second_hit_returns_none",
			account: &Account{
				ID:                      105,
				Type:                    AccountTypeAPIKey,
				Platform:                PlatformGemini,
				TempUnschedulableReason: `{"status_code":401,"until_unix":1735689600}`,
				Credentials: map[string]any{
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(401),
							"keywords":         []any{"unauthorized"},
							"duration_minutes": float64(10),
						},
					},
				},
			},
			statusCode: 401,
			body:       []byte(`unauthorized`),
			expected:   ErrorPolicyNone,
		},
		{
			name: "gemini_custom_codes_override_temp_unschedulable",
			account: &Account{
				ID:       104,
				Type:     AccountTypeAPIKey,
				Platform: PlatformGemini,
				Credentials: map[string]any{
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(503)},
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(503),
							"keywords":         []any{"overloaded"},
							"duration_minutes": float64(10),
						},
					},
				},
			},
			statusCode: 503,
			body:       []byte(`overloaded`),
			expected:   ErrorPolicyMatched, // custom codes take precedence
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &errorPolicyRepoStub{}
			svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

			result := svc.CheckErrorPolicy(context.Background(), tt.account, tt.statusCode, tt.body)
			require.Equal(t, tt.expected, result)
		})
	}
}

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
	}{
		{
			name: "custom_codes_matched_429_failover",
			account: &Account{
				ID:       200,
				Type:     AccountTypeAPIKey,
				Platform: PlatformGemini,
				Credentials: map[string]any{
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(429)},
				},
			},
			statusCode:        429,
			respBody:          []byte(`{"error":"rate limited"}`),
			expectFailover:    true,
			expectHandleError: true,
		},
		{
			name: "custom_codes_skipped_500_no_failover",
			account: &Account{
				ID:       201,
				Type:     AccountTypeAPIKey,
				Platform: PlatformGemini,
				Credentials: map[string]any{
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(429)},
				},
			},
			statusCode:        500,
			respBody:          []byte(`{"error":"internal"}`),
			expectFailover:    false,
			expectHandleError: false,
		},
		{
			name: "temp_unschedulable_matched_failover",
			account: &Account{
				ID:       202,
				Type:     AccountTypeAPIKey,
				Platform: PlatformGemini,
				Credentials: map[string]any{
					"temp_unschedulable_enabled": true,
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(503),
							"keywords":         []any{"overloaded"},
							"duration_minutes": float64(10),
						},
					},
				},
			},
			statusCode:        503,
			respBody:          []byte(`overloaded`),
			expectFailover:    true,
			expectHandleError: true,
		},
		{
			name: "no_policy_429_failover_via_shouldFailover",
			account: &Account{
				ID:       203,
				Type:     AccountTypeAPIKey,
				Platform: PlatformGemini,
			},
			statusCode:           429,
			respBody:             []byte(`{"error":"rate limited"}`),
			expectFailover:       true,
			expectHandleError:    true,
			expectShouldFailover: true,
		},
		{
			name: "no_policy_400_no_failover",
			account: &Account{
				ID:       204,
				Type:     AccountTypeAPIKey,
				Platform: PlatformGemini,
			},
			statusCode:        400,
			respBody:          []byte(`{"error":"bad request"}`),
			expectFailover:    false,
			expectHandleError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &geminiErrorPolicyRepo{}
			rlSvc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			svc := &GeminiMessagesCompatService{
				accountRepo:      repo,
				rateLimitService: rlSvc,
			}

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
			headers := http.Header{}

			if svc.rateLimitService != nil {
				switch svc.rateLimitService.CheckErrorPolicy(ctx, account, statusCode, respBody) {
				case ErrorPolicySkipped:
					// Skipped → return error directly (no handleGeminiUpstreamError, no failover)
					gotFailover = false
					handleErrorCalled = false
					goto verify
				case ErrorPolicyMatched, ErrorPolicyTempUnscheduled:
					svc.handleGeminiUpstreamError(ctx, account, statusCode, headers, respBody)
					handleErrorCalled = true
					gotFailover = true
					goto verify
				}
			}

			// ErrorPolicyNone → original logic
			svc.handleGeminiUpstreamError(ctx, account, statusCode, headers, respBody)
			handleErrorCalled = true
			if svc.shouldFailoverGeminiUpstreamError(statusCode) {
				gotFailover = true
			}

		verify:
			require.Equal(t, tt.expectFailover, gotFailover, "failover mismatch")
			require.Equal(t, tt.expectHandleError, handleErrorCalled, "handleGeminiUpstreamError call mismatch")

			if tt.expectShouldFailover {
				require.True(t, svc.shouldFailoverGeminiUpstreamError(statusCode),
					"shouldFailoverGeminiUpstreamError should return true for status %d", statusCode)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestGeminiErrorPolicy_NilRateLimitService — verifies nil safety
// ---------------------------------------------------------------------------

func TestGeminiErrorPolicy_NilRateLimitService(t *testing.T) {
	svc := &GeminiMessagesCompatService{
		rateLimitService: nil,
	}

	// When rateLimitService is nil, error policy is skipped → falls through to
	// shouldFailoverGeminiUpstreamError (original logic).
	// Verify this doesn't panic and follows expected behavior.

	ctx := context.Background()
	account := &Account{
		ID:       300,
		Type:     AccountTypeAPIKey,
		Platform: PlatformGemini,
		Credentials: map[string]any{
			"custom_error_codes_enabled": true,
			"custom_error_codes":         []any{float64(429)},
		},
	}

	// The nil check should prevent CheckErrorPolicy from being called
	if svc.rateLimitService != nil {
		t.Fatal("rateLimitService should be nil for this test")
	}

	// shouldFailoverGeminiUpstreamError still works
	require.True(t, svc.shouldFailoverGeminiUpstreamError(429))
	require.False(t, svc.shouldFailoverGeminiUpstreamError(400))

	// handleGeminiUpstreamError should not panic with nil rateLimitService
	require.NotPanics(t, func() {
		svc.handleGeminiUpstreamError(ctx, account, 500, http.Header{}, []byte(`error`))
	})
}

// ---------------------------------------------------------------------------
// geminiErrorPolicyRepo — minimal AccountRepository stub for Gemini error
// policy tests. Embeds mockAccountRepoForGemini and adds tracking.
// ---------------------------------------------------------------------------

type geminiErrorPolicyRepo struct {
	mockAccountRepoForGemini
	setErrorCalls       int
	setRateLimitedCalls int
	setTempCalls        int
}

func (r *geminiErrorPolicyRepo) SetError(_ context.Context, _ int64, _ string) error {
	r.setErrorCalls++
	return nil
}

func (r *geminiErrorPolicyRepo) SetRateLimited(_ context.Context, _ int64, _ time.Time) error {
	r.setRateLimitedCalls++
	return nil
}

func (r *geminiErrorPolicyRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, _ string) error {
	r.setTempCalls++
	return nil
}
