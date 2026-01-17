//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

type stubAntigravityUpstream struct {
	firstBase  string
	secondBase string
	calls      []string
REDACTED

func (s *stubAntigravityUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	url := req.URL.String()
	s.calls = append(s.calls, url)
	if strings.HasPrefix(url, s.firstBase) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{REDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"Resource has been exhausted"REDACTEDREDACTED`)),
	REDACTED, nil
REDACTED
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(strings.NewReader("ok")),
REDACTED, nil
REDACTED

type scopeLimitCall struct {
	accountID int64
	scope     AntigravityQuotaScope
	resetAt   time.Time
REDACTED

type rateLimitCall struct {
	accountID int64
	resetAt   time.Time
REDACTED

type stubAntigravityAccountRepo struct {
	AccountRepository
	scopeCalls []scopeLimitCall
	rateCalls  []rateLimitCall
REDACTED

func (s *stubAntigravityAccountRepo) SetAntigravityQuotaScopeLimit(ctx context.Context, id int64, scope AntigravityQuotaScope, resetAt time.Time) error {
	s.scopeCalls = append(s.scopeCalls, scopeLimitCall{accountID: id, scope: scope, resetAt: resetAtREDACTED)
	return nil
REDACTED

func (s *stubAntigravityAccountRepo) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	s.rateCalls = append(s.rateCalls, rateLimitCall{accountID: id, resetAt: resetAtREDACTED)
	return nil
REDACTED

func TestAntigravityRetryLoop_URLFallback_UsesLatestSuccess(t *testing.T) {
	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	defer func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
REDACTED()

	base1 := "https://ag-1.test"
	base2 := "https://ag-2.test"
	antigravity.BaseURLs = []string{base1, base2REDACTED
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)

	upstream := &stubAntigravityUpstream{firstBase: base1, secondBase: base2REDACTED
	account := &Account{
		ID:          1,
		Name:        "acc-1",
		Platform:    PlatformAntigravity,
		Schedulable: true,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED

	var handleErrorCalled bool
	result, err := antigravityRetryLoop(antigravityRetryLoopParams{
		prefix:      "[test]",
		ctx:         context.Background(),
		account:     account,
		proxyURL:    "",
		accessToken: "token",
		action:      "generateContent",
		body:        []byte(`{"input":"test"REDACTED`),
		quotaScope:  AntigravityQuotaScopeClaude,
		httpUpstream: upstream,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, quotaScope AntigravityQuotaScope) {
			handleErrorCalled = true
	REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.resp)
	defer func() { _ = result.resp.Body.Close() REDACTED()
	require.Equal(t, http.StatusOK, result.resp.StatusCode)
	require.False(t, handleErrorCalled)
	require.Len(t, upstream.calls, 2)
	require.True(t, strings.HasPrefix(upstream.calls[0], base1))
	require.True(t, strings.HasPrefix(upstream.calls[1], base2))

	available := antigravity.DefaultURLAvailability.GetAvailableURLs()
	require.NotEmpty(t, available)
	require.Equal(t, base2, available[0])
REDACTED

func TestAntigravityHandleUpstreamError_UsesScopeLimitWhenEnabled(t *testing.T) {
	t.Setenv(antigravityScopeRateLimitEnv, "true")
	repo := &stubAntigravityAccountRepo{REDACTED
	svc := &AntigravityGatewayService{accountRepo: repoREDACTED
	account := &Account{ID: 9, Name: "acc-9", Platform: PlatformAntigravityREDACTED

	body := buildGeminiRateLimitBody("3s")
	svc.handleUpstreamError(context.Background(), "[test]", account, http.StatusTooManyRequests, http.Header{REDACTED, body, AntigravityQuotaScopeClaude)

	require.Len(t, repo.scopeCalls, 1)
	require.Empty(t, repo.rateCalls)
	call := repo.scopeCalls[0]
	require.Equal(t, account.ID, call.accountID)
	require.Equal(t, AntigravityQuotaScopeClaude, call.scope)
	require.WithinDuration(t, time.Now().Add(3*time.Second), call.resetAt, 2*time.Second)
REDACTED

func TestAntigravityHandleUpstreamError_UsesAccountLimitWhenScopeDisabled(t *testing.T) {
	t.Setenv(antigravityScopeRateLimitEnv, "false")
	repo := &stubAntigravityAccountRepo{REDACTED
	svc := &AntigravityGatewayService{accountRepo: repoREDACTED
	account := &Account{ID: 10, Name: "acc-10", Platform: PlatformAntigravityREDACTED

	body := buildGeminiRateLimitBody("2s")
	svc.handleUpstreamError(context.Background(), "[test]", account, http.StatusTooManyRequests, http.Header{REDACTED, body, AntigravityQuotaScopeClaude)

	require.Len(t, repo.rateCalls, 1)
	require.Empty(t, repo.scopeCalls)
	call := repo.rateCalls[0]
	require.Equal(t, account.ID, call.accountID)
	require.WithinDuration(t, time.Now().Add(2*time.Second), call.resetAt, 2*time.Second)
REDACTED

func TestAccountIsSchedulableForModel_AntigravityRateLimits(t *testing.T) {
	now := time.Now()
	future := now.Add(10 * time.Minute)

	account := &Account{
		ID:          1,
		Name:        "acc",
		Platform:    PlatformAntigravity,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	account.RateLimitResetAt = &future
	require.False(t, account.IsSchedulableForModel("claude-sonnet-4-5"))
	require.False(t, account.IsSchedulableForModel("gemini-3-flash"))

	account.RateLimitResetAt = nil
	account.Extra = map[string]any{
		antigravityQuotaScopesKey: map[string]any{
			"claude": map[string]any{
				"rate_limit_reset_at": future.Format(time.RFC3339),
		REDACTED,
	REDACTED,
REDACTED

	require.False(t, account.IsSchedulableForModel("claude-sonnet-4-5"))
	require.True(t, account.IsSchedulableForModel("gemini-3-flash"))
REDACTED

func buildGeminiRateLimitBody(delay string) []byte {
	return []byte(fmt.Sprintf(`{"error":{"message":"too many requests","details":[{"metadata":{"quotaResetDelay":%qREDACTEDREDACTED]REDACTEDREDACTED`, delay))
REDACTED
