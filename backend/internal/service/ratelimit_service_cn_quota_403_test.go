//go:build unit

package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// newCNQuotaExhaustedTestAccount 构造一个 Kimi Coding Plan 账号：
// account_mode=coding 使 IsCodingPlan() 为真，Extra 带 5h/weekly 窗口快照。
func newCNQuotaExhaustedTestAccount(resetIn time.Duration) *Account {
	extra := map[string]any{}
	if resetIn > 0 {
		extra[cnExtraKey(PlatformKimi, cnExtraSuffix5hUsed)] = 100
		extra[cnExtraKey(PlatformKimi, cnExtraSuffix5hReset)] = time.Now().Add(resetIn).UTC().Format(time.RFC3339)
		extra[cnExtraKey(PlatformKimi, cnExtraSuffixWeeklyReset)] = time.Now().Add(7 * 24 * time.Hour).UTC().Format(time.RFC3339)
	}
	return &Account{
		ID:          401,
		Platform:    PlatformKimi,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
		Extra:       extra,
	}
}

const kimiWeeklyQuotaExhaustedBody = `{"error":{"message":"You've reached your weekly (7-day) usage limit. Your quota will reset when the current 7-day window ends. To continue now, purchase extra usage or upgrade your plan: https://www.kimi.com/membership/subscription?tab=quota","type":"access_terminated_error"}}`

// runKimi403 以 403 + 给定响应体驱动 HandleUpstreamError，返回 shouldDisable。
func runKimi403(service *RateLimitService, account *Account, body string) bool {
	return service.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusForbidden,
		http.Header{},
		[]byte(body),
	)
}

func TestHandleUpstreamError_KimiQuotaExhausted403RateLimitedToWindowReset(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	blocker := &runtimeBlockRecorder{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetAccountRuntimeBlocker(blocker)
	account := newCNQuotaExhaustedTestAccount(2 * time.Hour)

	shouldDisable := runKimi403(service, account, kimiWeeklyQuotaExhaustedBody)

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.setErrorCalls, "quota exhaustion must never SetError")
	require.Equal(t, 0, repo.tempCalls)
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.Equal(t, account.ID, repo.lastRateLimitedID)
	require.True(t, repo.lastRateLimitedAt.After(time.Now()), "cooldown should target the future window reset")
	require.True(t, repo.lastRateLimitedAt.Before(time.Now().Add(3*time.Hour)), "should cool down to the nearer 5h reset, not weekly")
	require.Len(t, blocker.accounts, 1)
	require.Equal(t, cnQuotaExhaustedReasonPrefix, blocker.reasons[0])
}

func TestHandleUpstreamError_KimiQuotaExhausted403WithoutSnapshotFallsBackToTemp(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newCNQuotaExhaustedTestAccount(0)

	shouldDisable := runKimi403(service, account, kimiWeeklyQuotaExhaustedBody)

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 0, repo.rateLimitedCalls)
	require.Equal(t, 1, repo.tempCalls)
	require.Contains(t, repo.lastTempReason, cnQuotaExhaustedReasonPrefix)
	require.Greater(t, len(repo.lastTempReason), len(cnQuotaExhaustedReasonPrefix))
}

func TestHandleUpstreamError_KimiQuotaExhausted403MatchedByMessageOnly(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newCNQuotaExhaustedTestAccount(2 * time.Hour)
	// 无 error.type 字段，仅靠文案兜底匹配。
	body := `{"error":{"message":"You've reached your weekly (7-day) usage limit. Your quota will reset when the current 7-day window ends."}}`

	shouldDisable := runKimi403(service, account, body)

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 1, repo.rateLimitedCalls)
}

func TestHandleUpstreamError_KimiConcurrencyLimit403StillUsesConcurrencyPath(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newCNQuotaExhaustedTestAccount(2 * time.Hour)
	body := fmt.Sprintf(`{"error":{"message":%q,"type":"access_terminated_error"}}`, kimiConcurrentRequestLimitMessage)

	shouldDisable := runKimi403(service, account, body)

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 0, repo.rateLimitedCalls, "concurrency-limit message must keep its dedicated path")
	require.Equal(t, 1, repo.tempCalls)
	require.Contains(t, repo.lastTempReason, cnConcurrencyLimitReasonPrefix)
}

func TestHandleUpstreamError_KimiGeneric403StillEscalates(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{3}}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	account := newCNQuotaExhaustedTestAccount(2 * time.Hour)

	shouldDisable := runKimi403(service, account, `{"error":{"message":"policy rejected this request"}}`)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.setErrorCalls, "non-quota 403 keeps the generic escalation path")
	require.Equal(t, 0, repo.rateLimitedCalls)
}

func TestHandleUpstreamError_KimiNonCodingPlanQuotaMessageUsesGeneric403(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{3}}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	account := newCNQuotaExhaustedTestAccount(2 * time.Hour)
	account.Credentials = map[string]any{"account_mode": AccountModePayG}

	shouldDisable := runKimi403(service, account, kimiWeeklyQuotaExhaustedBody)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.setErrorCalls, "non-coding-plan account must keep the generic 403 escalation")
	require.Equal(t, 0, repo.rateLimitedCalls)
	require.Equal(t, 0, repo.tempCalls)
}

func TestHandleUpstreamError_NonCNAccessTerminated403KeepsGenericPath(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{1}}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	account := &Account{
		ID:       402,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}

	shouldDisable := runKimi403(service, account, kimiWeeklyQuotaExhaustedBody)

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.rateLimitedCalls, "non-CN platform must not enter the CN quota path")
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 1, repo.tempCalls)
	require.Contains(t, repo.lastTempReason, "(1/3)")
}

func TestIsCNProviderQuotaExhausted403_Classification(t *testing.T) {
	kimi := newCNQuotaExhaustedTestAccount(0)
	require.True(t, isCNProviderQuotaExhausted403(kimi, []byte(kimiWeeklyQuotaExhaustedBody), ""))
	require.True(t, isCNProviderQuotaExhausted403(kimi, []byte(`{"error":{"message":"You've reached your weekly (7-day) usage limit."}}`), "You've reached your weekly (7-day) usage limit."))
	require.False(t, isCNProviderQuotaExhausted403(kimi, []byte(`{"error":{"message":"policy rejected"}}`), "policy rejected"))
	require.False(t, isCNProviderQuotaExhausted403(kimi, nil, ""))

	openai := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.False(t, isCNProviderQuotaExhausted403(openai, []byte(kimiWeeklyQuotaExhaustedBody), ""))
	require.False(t, isCNProviderQuotaExhausted403(nil, []byte(kimiWeeklyQuotaExhaustedBody), ""))
}
