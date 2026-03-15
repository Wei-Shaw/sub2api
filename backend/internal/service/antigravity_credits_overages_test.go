//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func TestClassifyAntigravity429(t *testing.T) {
	t.Run("明确配额耗尽", func(t *testing.T) {
		body := []byte(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"QUOTA_EXHAUSTED"REDACTEDREDACTED`)
		require.Equal(t, antigravity429QuotaExhausted, classifyAntigravity429(body))
REDACTED)

	t.Run("结构化限流", func(t *testing.T) {
		body := []byte(`{
			"error": {
				"status": "RESOURCE_EXHAUSTED",
				"details": [
					{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"REDACTED, "reason": "RATE_LIMIT_EXCEEDED"REDACTED,
					{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.5s"REDACTED
				]
		REDACTED
	REDACTED`)
		require.Equal(t, antigravity429RateLimited, classifyAntigravity429(body))
REDACTED)

	t.Run("未知429", func(t *testing.T) {
		body := []byte(`{"error":{"message":"too many requests"REDACTEDREDACTED`)
		require.Equal(t, antigravity429Unknown, classifyAntigravity429(body))
REDACTED)
REDACTED

func TestCanUseAntigravityCreditsOverages(t *testing.T) {
	activeUntil := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)

	t.Run("必须有运行态才可直接走 overages", func(t *testing.T) {
		account := &Account{
			ID:          1,
			Platform:    PlatformAntigravity,
			Status:      StatusActive,
			Schedulable: true,
			Extra: map[string]any{
				"allow_overages": true,
		REDACTED,
	REDACTED
		require.False(t, canUseAntigravityCreditsOverages(context.Background(), account, "claude-sonnet-4-5"))
REDACTED)

	t.Run("运行态有效时允许使用 overages", func(t *testing.T) {
		account := &Account{
			ID:          2,
			Platform:    PlatformAntigravity,
			Status:      StatusActive,
			Schedulable: true,
			Extra: map[string]any{
				"allow_overages": true,
				antigravityCreditsOveragesKey: map[string]any{
					"claude-sonnet-4-5": map[string]any{
						"active_until": activeUntil,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED
		require.True(t, canUseAntigravityCreditsOverages(context.Background(), account, "claude-sonnet-4-5"))
REDACTED)

	t.Run("credits 耗尽后不可继续使用 overages", func(t *testing.T) {
		account := &Account{
			ID:          3,
			Platform:    PlatformAntigravity,
			Status:      StatusActive,
			Schedulable: true,
			Extra: map[string]any{
				"allow_overages": true,
				antigravityCreditsOveragesKey: map[string]any{
					"claude-sonnet-4-5": map[string]any{
						"active_until": activeUntil,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED
		setCreditsExhausted(account.ID, time.Now().Add(time.Minute))
		t.Cleanup(func() { clearCreditsExhausted(account.ID) REDACTED)
		require.False(t, canUseAntigravityCreditsOverages(context.Background(), account, "claude-sonnet-4-5"))
REDACTED)
REDACTED

func TestHandleSmartRetry_QuotaExhausted_UsesCreditsAndStoresIndependentState(t *testing.T) {
	successResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"ok":trueREDACTED`)),
REDACTED
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{successRespREDACTED,
		errors:    []error{nilREDACTED,
REDACTED
	repo := &stubAntigravityAccountRepo{REDACTED
	account := &Account{
		ID:       101,
		Name:     "acc-101",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
		Extra: map[string]any{
			"allow_overages": true,
	REDACTED,
REDACTED
			"model_mapping": map[string]any{
				"claude-opus-4-6": "claude-sonnet-4-5",
		REDACTED,
	REDACTED,
REDACTED

	respBody := []byte(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"QUOTA_EXHAUSTED"REDACTEDREDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTED
	params := antigravityRetryLoopParams{
		ctx:            context.Background(),
		prefix:         "[test]",
		account:        account,
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"model":"claude-opus-4-6","request":{REDACTEDREDACTED`),
		httpUpstream:   upstream,
		accountRepo:    repo,
		requestedModel: "claude-opus-4-6",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED

	svc := &AntigravityGatewayService{REDACTED
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, []string{"https://ag-1.test"REDACTED)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.resp)
	require.Nil(t, result.switchError)
	require.Len(t, upstream.requestBodies, 1)
	require.Contains(t, string(upstream.requestBodies[0]), "enabledCreditTypes")
	require.Empty(t, repo.modelRateLimitCalls, "overages 成功后不应写入普通 model_rate_limits")
	require.Len(t, repo.extraUpdateCalls, 1)

	state, ok := account.Extra[antigravityCreditsOveragesKey].(map[string]any)
	require.True(t, ok)
	_, exists := state["claude-sonnet-4-5"]
	require.True(t, exists, "应使用最终映射模型写入独立 overages 运行态")
REDACTED

func TestHandleSmartRetry_RateLimited_DoesNotUseCredits(t *testing.T) {
	successResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"ok":trueREDACTED`)),
REDACTED
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{successRespREDACTED,
		errors:    []error{nilREDACTED,
REDACTED
	repo := &stubAntigravityAccountRepo{REDACTED
	account := &Account{
		ID:       102,
		Name:     "acc-102",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
		Extra: map[string]any{
			"allow_overages": true,
	REDACTED,
REDACTED

	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"REDACTED, "reason": "RATE_LIMIT_EXCEEDED"REDACTED,
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"REDACTED
			]
	REDACTED
REDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTED
	params := antigravityRetryLoopParams{
		ctx:          context.Background(),
		prefix:       "[test]",
		account:      account,
		accessToken:  "token",
		action:       "generateContent",
		body:         []byte(`{"model":"claude-sonnet-4-5","request":{REDACTEDREDACTED`),
		httpUpstream: upstream,
		accountRepo:  repo,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED

	svc := &AntigravityGatewayService{REDACTED
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, []string{"https://ag-1.test"REDACTED)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.resp)
	require.Len(t, upstream.requestBodies, 1)
	require.NotContains(t, string(upstream.requestBodies[0]), "enabledCreditTypes")
	require.Empty(t, repo.extraUpdateCalls)
	require.Empty(t, repo.modelRateLimitCalls)
REDACTED

func TestAntigravityRetryLoop_ActiveOverages_InjectsCreditsBody(t *testing.T) {
	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	defer func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
REDACTED()

	antigravity.BaseURLs = []string{"https://ag-1.test"REDACTED
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)

	activeUntil := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{REDACTED,
				Body:       io.NopCloser(strings.NewReader(`{"ok":trueREDACTED`)),
		REDACTED,
	REDACTED,
		errors: []error{nilREDACTED,
REDACTED
	account := &Account{
		ID:          103,
		Name:        "acc-103",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"allow_overages": true,
			antigravityCreditsOveragesKey: map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"active_until": activeUntil,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	svc := &AntigravityGatewayService{REDACTED
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:            context.Background(),
		prefix:         "[test]",
		account:        account,
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"model":"claude-sonnet-4-5","request":{REDACTEDREDACTED`),
		httpUpstream:   upstream,
		requestedModel: "claude-sonnet-4-5",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 1)
	require.Contains(t, string(upstream.requestBodies[0]), "enabledCreditTypes")
REDACTED

func TestAntigravityRetryLoop_ActiveOverages_ExplicitCreditErrorMarksExhausted(t *testing.T) {
	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	defer func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
REDACTED()

	antigravity.BaseURLs = []string{"https://ag-1.test"REDACTED
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)

	accountID := int64(104)
	activeUntil := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	repo := &stubAntigravityAccountRepo{REDACTED
	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{
			{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{REDACTED,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"Insufficient GOOGLE_ONE_AI credits"REDACTEDREDACTED`)),
		REDACTED,
	REDACTED,
		errors: []error{nilREDACTED,
REDACTED
	account := &Account{
		ID:          accountID,
		Name:        "acc-104",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"allow_overages": true,
			antigravityCreditsOveragesKey: map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"active_until": activeUntil,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	clearCreditsExhausted(accountID)
	t.Cleanup(func() { clearCreditsExhausted(accountID) REDACTED)

	svc := &AntigravityGatewayService{accountRepo: repoREDACTED
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:            context.Background(),
		prefix:         "[test]",
		account:        account,
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"model":"claude-sonnet-4-5","request":{REDACTEDREDACTED`),
		httpUpstream:   upstream,
		requestedModel: "claude-sonnet-4-5",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, result)
	require.True(t, isCreditsExhausted(accountID))
	require.Len(t, repo.extraUpdateCalls, 1, "应清理对应模型的 overages 运行态")
REDACTED
