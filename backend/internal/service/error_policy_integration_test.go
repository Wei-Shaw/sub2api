//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mocks (scoped to this file by naming convention)
// ---------------------------------------------------------------------------

// epFixedUpstream returns a fixed response for every request.
type epFixedUpstream struct {
	statusCode int
	body       string
	calls      int
}

func (u *epFixedUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	u.calls++
	return &http.Response{
		StatusCode: u.statusCode,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(u.body)),
	}, nil
}

func (u *epFixedUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

// epAccountRepo records SetTempUnschedulable / SetError calls.
type epAccountRepo struct {
	mockAccountRepoForGemini
	tempCalls   int
	setErrCalls int
}

func (r *epAccountRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, _ string) error {
	r.tempCalls++
	return nil
}

func (r *epAccountRepo) SetError(_ context.Context, _ int64, _ string) error {
	r.setErrCalls++
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func saveAndSetBaseURLs(t *testing.T) {
	t.Helper()
	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvail := antigravity.DefaultURLAvailability
	antigravity.BaseURLs = []string{"https://ep-test.example"}
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)
	t.Cleanup(func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvail
	})
}

func newRetryParams(account *Account, upstream HTTPUpstream, handleError func(context.Context, string, *Account, int, http.Header, []byte, string, int64, string, bool) *handleModelRateLimitResult) antigravityRetryLoopParams {
	return antigravityRetryLoopParams{
		ctx:            context.Background(),
		prefix:         "[ep-test]",
		account:        account,
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"input":"test"}`),
		httpUpstream:   upstream,
		requestedModel: "claude-sonnet-4-5",
		handleError:    handleError,
	}
}

// ---------------------------------------------------------------------------
// TestRetryLoop_ErrorPolicy_CustomErrorCodes
// ---------------------------------------------------------------------------

func TestRetryLoop_ErrorPolicy_CustomErrorCodes(t *testing.T) {
	tests := []struct {
		name              string
		upstreamStatus    int
		upstreamBody      string
		customCodes       []any
		expectHandleError int
		expectUpstream    int
		expectStatusCode  int
	}{
		{
			name:              "429_in_custom_codes_matched",
			upstreamStatus:    429,
			upstreamBody:      `{"error":"rate limited"}`,
			customCodes:       []any{float64(429)},
			expectHandleError: 1,
			expectUpstream:    1,
			expectStatusCode:  429,
		},
		{
			name:              "429_not_in_custom_codes_skipped",
			upstreamStatus:    429,
			upstreamBody:      `{"error":"rate limited"}`,
			customCodes:       []any{float64(500)},
			expectHandleError: 0,
			expectUpstream:    1,
			expectStatusCode:  500,
		},
		{
			name:              "500_in_custom_codes_matched",
			upstreamStatus:    500,
			upstreamBody:      `{"error":"internal"}`,
			customCodes:       []any{float64(500)},
			expectHandleError: 1,
			expectUpstream:    1,
			expectStatusCode:  500,
		},
		{
			name:              "500_not_in_custom_codes_skipped",
			upstreamStatus:    500,
			upstreamBody:      `{"error":"internal"}`,
			customCodes:       []any{float64(429)},
			expectHandleError: 0,
			expectUpstream:    1,
			expectStatusCode:  500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saveAndSetBaseURLs(t)

			upstream := &epFixedUpstream{statusCode: tt.upstreamStatus, body: tt.upstreamBody}
			repo := &epAccountRepo{}
			rlSvc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

			account := &Account{
				ID:          100,
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAntigravity,
				Schedulable: true,
				Status:      StatusActive,
				Concurrency: 1,
				Credentials: map[string]any{
					"custom_error_codes_enabled": true,
					"custom_error_codes":         tt.customCodes,
				},
			}

			svc := &AntigravityGatewayService{rateLimitService: rlSvc}

			var handleErrorCount int
			p := newRetryParams(account, upstream, func(_ context.Context, _ string, _ *Account, _ int, _ http.Header, _ []byte, _ string, _ int64, _ string, _ bool) *handleModelRateLimitResult {
				handleErrorCount++
				return nil
			})

			result, err := svc.antigravityRetryLoop(p)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, result.resp)
			defer func() { _ = result.resp.Body.Close() }()

			require.Equal(t, tt.expectStatusCode, result.resp.StatusCode)
			require.Equal(t, tt.expectHandleError, handleErrorCount, "handleError call count")
			require.Equal(t, tt.expectUpstream, upstream.calls, "upstream call count")
		})
	}
}

// ---------------------------------------------------------------------------
// TestRetryLoop_ErrorPolicy_TempUnschedulable
// ---------------------------------------------------------------------------

func TestRetryLoop_ErrorPolicy_TempUnschedulable(t *testing.T) {
	tempRulesAccount := func(rules []any) *Account {
		return &Account{
			ID:          200,
			Type:        AccountTypeOAuth,
			Platform:    PlatformAntigravity,
			Schedulable: true,
			Status:      StatusActive,
			Concurrency: 1,
			Credentials: map[string]any{
				"temp_unschedulable_enabled": true,
				"temp_unschedulable_rules":   rules,
			},
		}
	}

	overloadedRule := map[string]any{
		"error_code":       float64(503),
		"keywords":         []any{"overloaded"},
		"duration_minutes": float64(10),
	}

	rateLimitRule := map[string]any{
		"error_code":       float64(429),
		"keywords":         []any{"rate limited keyword"},
		"duration_minutes": float64(5),
	}

	t.Run("503_overloaded_matches_rule", func(t *testing.T) {
		saveAndSetBaseURLs(t)

		upstream := &epFixedUpstream{statusCode: 503, body: `overloaded`}
		repo := &epAccountRepo{}
		rlSvc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		svc := &AntigravityGatewayService{rateLimitService: rlSvc}

		account := tempRulesAccount([]any{overloadedRule})
		p := newRetryParams(account, upstream, func(_ context.Context, _ string, _ *Account, _ int, _ http.Header, _ []byte, _ string, _ int64, _ string, _ bool) *handleModelRateLimitResult {
			t.Error("handleError should not be called for temp unschedulable")
			return nil
		})

		result, err := svc.antigravityRetryLoop(p)

		require.Nil(t, result)
		var switchErr *AntigravityAccountSwitchError
		require.ErrorAs(t, err, &switchErr)
		require.Equal(t, account.ID, switchErr.OriginalAccountID)
		require.Equal(t, 1, upstream.calls, "should not retry")
	})

	t.Run("429_rate_limited_keyword_matches_rule", func(t *testing.T) {
		saveAndSetBaseURLs(t)

		upstream := &epFixedUpstream{statusCode: 429, body: `rate limited keyword`}
		repo := &epAccountRepo{}
		rlSvc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		svc := &AntigravityGatewayService{rateLimitService: rlSvc}

		account := tempRulesAccount([]any{rateLimitRule})
		p := newRetryParams(account, upstream, func(_ context.Context, _ string, _ *Account, _ int, _ http.Header, _ []byte, _ string, _ int64, _ string, _ bool) *handleModelRateLimitResult {
			t.Error("handleError should not be called for temp unschedulable")
			return nil
		})

		result, err := svc.antigravityRetryLoop(p)

		require.Nil(t, result)
		var switchErr *AntigravityAccountSwitchError
		require.ErrorAs(t, err, &switchErr)
		require.Equal(t, account.ID, switchErr.OriginalAccountID)
		require.Equal(t, 1, upstream.calls, "should not retry")
	})

	t.Run("503_body_no_match_continues_default_retry", func(t *testing.T) {
		saveAndSetBaseURLs(t)

		upstream := &epFixedUpstream{statusCode: 503, body: `random`}
		repo := &epAccountRepo{}
		rlSvc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		svc := &AntigravityGatewayService{rateLimitService: rlSvc}

		account := tempRulesAccount([]any{overloadedRule})

		// Use a short-lived context: the backoff sleep (~1s) will be
		// interrupted, proving the code entered the default retry path
		// instead of breaking early via error policy.
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		p := newRetryParams(account, upstream, func(_ context.Context, _ string, _ *Account, _ int, _ http.Header, _ []byte, _ string, _ int64, _ string, _ bool) *handleModelRateLimitResult {
			return nil
		})
		p.ctx = ctx

		result, err := svc.antigravityRetryLoop(p)

		// Context cancellation during backoff proves default retry was entered
		require.Nil(t, result)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.GreaterOrEqual(t, upstream.calls, 1, "should have called upstream at least once")
	})
}

// ---------------------------------------------------------------------------
// TestRetryLoop_ErrorPolicy_NilRateLimitService
// ---------------------------------------------------------------------------

func TestRetryLoop_ErrorPolicy_NilRateLimitService(t *testing.T) {
	saveAndSetBaseURLs(t)

	upstream := &epFixedUpstream{statusCode: 429, body: `{"error":"rate limited"}`}
	// rateLimitService is nil — must not panic
	svc := &AntigravityGatewayService{rateLimitService: nil}

	account := &Account{
		ID:          300,
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Schedulable: true,
		Status:      StatusActive,
		Concurrency: 1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	p := newRetryParams(account, upstream, func(_ context.Context, _ string, _ *Account, _ int, _ http.Header, _ []byte, _ string, _ int64, _ string, _ bool) *handleModelRateLimitResult {
		return nil
	})
	p.ctx = ctx

	// Should not panic; enters the default retry path (eventually times out)
	result, err := svc.antigravityRetryLoop(p)

	require.Nil(t, result)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.GreaterOrEqual(t, upstream.calls, 1)
}

// ---------------------------------------------------------------------------
// TestRetryLoop_ErrorPolicy_NoPolicy_OriginalBehavior
// ---------------------------------------------------------------------------

func TestRetryLoop_ErrorPolicy_NoPolicy_OriginalBehavior(t *testing.T) {
	saveAndSetBaseURLs(t)

	upstream := &epFixedUpstream{statusCode: 429, body: `{"error":"rate limited"}`}
	repo := &epAccountRepo{}
	rlSvc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc := &AntigravityGatewayService{rateLimitService: rlSvc}

	// Plain OAuth account with no error policy configured
	account := &Account{
		ID:          400,
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Schedulable: true,
		Status:      StatusActive,
		Concurrency: 1,
	}

	var handleErrorCount int
	p := newRetryParams(account, upstream, func(_ context.Context, _ string, _ *Account, _ int, _ http.Header, _ []byte, _ string, _ int64, _ string, _ bool) *handleModelRateLimitResult {
		handleErrorCount++
		return nil
	})

	result, err := svc.antigravityRetryLoop(p)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.resp)
	defer func() { _ = result.resp.Body.Close() }()

	require.Equal(t, http.StatusTooManyRequests, result.resp.StatusCode)
	require.Equal(t, antigravityMaxRetries, upstream.calls, "should exhaust all retries")
	require.Equal(t, 1, handleErrorCount, "handleError should be called once after retries exhausted")
}

// ---------------------------------------------------------------------------
// epTrackingRepo — records SetRateLimited / SetError calls for verification.
// ---------------------------------------------------------------------------

type epTrackingRepo struct {
	mockAccountRepoForGemini
	rateLimitedCalls int
	rateLimitedID    int64
	setErrCalls      int
	setErrID         int64
	tempCalls        int
}

func (r *epTrackingRepo) SetRateLimited(_ context.Context, id int64, _ time.Time) error {
	r.rateLimitedCalls++
	r.rateLimitedID = id
	return nil
}

func (r *epTrackingRepo) SetError(_ context.Context, id int64, _ string) error {
	r.setErrCalls++
	r.setErrID = id
	return nil
}

func (r *epTrackingRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, _ string) error {
	r.tempCalls++
	return nil
}

// ---------------------------------------------------------------------------
// TestCustomErrorCode599_SkippedErrors_Return500_NoRateLimit
//
// 核心场景：自定义错误码设为 [599]（一个不会真正出现的错误码），
// 当上游返回 429/500/503/401 时：
//   - 返回给客户端的状态码必须是 500（而不是透传原始状态码）
//   - 不调用 SetRateLimited（不进入限流状态）
//   - 不调用 SetError（不停止调度）
//   - 不调用 handleError
// ---------------------------------------------------------------------------

func TestCustomErrorCode599_SkippedErrors_Return500_NoRateLimit(t *testing.T) {
	errorCodes := []int{429, 500, 503, 401, 403}

	for _, upstreamStatus := range errorCodes {
		t.Run(http.StatusText(upstreamStatus), func(t *testing.T) {
			saveAndSetBaseURLs(t)

			upstream := &epFixedUpstream{
				statusCode: upstreamStatus,
				body:       `{"error":"some upstream error"}`,
			}
			repo := &epTrackingRepo{}
			rlSvc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			svc := &AntigravityGatewayService{rateLimitService: rlSvc}

			account := &Account{
				ID:          500,
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAntigravity,
				Schedulable: true,
				Status:      StatusActive,
				Concurrency: 1,
				Credentials: map[string]any{
					"custom_error_codes_enabled": true,
					"custom_error_codes":         []any{float64(599)},
				},
			}

			var handleErrorCount int
			p := newRetryParams(account, upstream, func(_ context.Context, _ string, _ *Account, _ int, _ http.Header, _ []byte, _ string, _ int64, _ string, _ bool) *handleModelRateLimitResult {
				handleErrorCount++
				return nil
			})

			result, err := svc.antigravityRetryLoop(p)

			// 不应返回 error（Skipped 不触发账号切换）
			require.NoError(t, err, "should not return error")
			require.NotNil(t, result, "result should not be nil")
			require.NotNil(t, result.resp, "response should not be nil")
			defer func() { _ = result.resp.Body.Close() }()

			// 状态码必须是 500（不透传原始状态码）
			require.Equal(t, http.StatusInternalServerError, result.resp.StatusCode,
				"skipped error should return 500, not %d", upstreamStatus)

			// 不调用 handleError
			require.Equal(t, 0, handleErrorCount,
				"handleError should NOT be called for skipped errors")

			// 不标记限流
			require.Equal(t, 0, repo.rateLimitedCalls,
				"SetRateLimited should NOT be called for skipped errors")

			// 不停止调度
			require.Equal(t, 0, repo.setErrCalls,
				"SetError should NOT be called for skipped errors")

			// 只调用一次上游（不重试）
			require.Equal(t, 1, upstream.calls,
				"should call upstream exactly once (no retry)")
		})
	}
}
