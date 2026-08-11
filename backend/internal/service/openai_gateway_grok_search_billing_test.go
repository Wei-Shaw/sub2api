//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestForwardGrokResponses_PropagatesSearchCountFromJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"grok","input":"search something","tools":[{"type":"web_search"REDACTED],"stream":falseREDACTED`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	account := healthyGrokOAuthGatewayTestAccount(9901, "access-token")
	repo := &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{account.ID: accountREDACTEDREDACTED
	upstreamBody := `{
		"id":"resp_search_bill",
		"object":"response",
		"model":"grok-4.5",
		"status":"completed",
		"output":[
			{"type":"web_search_call","id":"ws1","call_id":"c1","status":"completed"REDACTED,
			{"type":"x_search_call","id":"xs1","call_id":"c2"REDACTED,
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED
		],
		"usage":{"input_tokens":10,"output_tokens":5REDACTED
REDACTED`
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(bytes.NewReader([]byte(upstreamBody))),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
REDACTED

	result, err := svc.forwardGrokResponses(context.Background(), c, account, body, "grok", false, time.Now())
REDACTED
	require.NotNil(t, result)
	require.Equal(t, 2, result.SearchCount, "Grok Responses must surface search tool calls for surcharge billing")
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
REDACTED

func TestForwardGrokResponses_PropagatesSearchCountFromSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"grok","input":"search","tools":[{"type":"web_search"REDACTED],"stream":trueREDACTED`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	account := healthyGrokOAuthGatewayTestAccount(9902, "access-token")
	repo := &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{account.ID: accountREDACTEDREDACTED
	// item.done + response.completed for same call_id must count once after wire-up.
	sse := "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"web_search_call\",\"id\":\"ws1\",\"call_id\":\"c1\"REDACTEDREDACTED\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_s\",\"status\":\"completed\",\"output\":[{\"type\":\"web_search_call\",\"id\":\"ws1\",\"call_id\":\"c1\"REDACTED],\"usage\":{\"input_tokens\":3,\"output_tokens\":1REDACTEDREDACTEDREDACTED\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body:       io.NopCloser(bytes.NewReader([]byte(sse))),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
REDACTED

	result, err := svc.forwardGrokResponses(context.Background(), c, account, body, "grok", true, time.Now())
REDACTED
	require.NotNil(t, result)
	require.Equal(t, 1, result.SearchCount, "stream SearchCount must be wired and deduped")
REDACTED

func TestGetSchedulableAccount_AppliesGrokFreeSoftGate(t *testing.T) {
	// Sticky/non-list path must not return over-gate free OAuth accounts once cache is warm.
	// First sticky hit fail-opens and schedules async refresh; subsequent hits use the cache.
	cfg := &config.Config{REDACTED
	cfg.Gateway.Grok.FreeQuotaSoftGateEnabled = true
	cfg.Gateway.Grok.FreeQuotaTokenLimit = 500_000
	cfg.Gateway.Grok.FreeQuotaSoftGatePercent = 95
	cfg.Gateway.Grok.FreeQuotaWindowHours = 24
	cfg.Gateway.Grok.FreeQuotaStatsCacheSeconds = 60

	account := healthyGrokOAuthGatewayTestAccount(8801, "tok")
	account.Credentials["subscription_tier"] = "free"
	account.Status = StatusActive
	account.Schedulable = true

	repo := &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: accountREDACTED,
REDACTED
	usageRepo := &grokFreeQuotaUsageRepoStub{stats: map[int64]*usagestats.AccountStats{
		account.ID: {Tokens: 480_000REDACTED, // above 95% of 500k
REDACTEDREDACTED
	// Clear shared gateway free-gate cache so this test is deterministic.
	gatewayGrokFreeQuotaGateCache.Range(func(key, _ any) bool {
		gatewayGrokFreeQuotaGateCache.Delete(key)
		return true
REDACTED)
	if root, ok := freeQuotaRefreshInFlight.Load(&gatewayGrokFreeQuotaGateCache); ok {
		if m, ok := root.(*sync.Map); ok {
			m.Delete(account.ID)
	REDACTED
REDACTED
	svc := &GatewayService{
		cfg:          cfg,
		accountRepo:  repo,
		usageLogRepo: usageRepo,
REDACTED

	// Miss: fail open + schedule refresh.
	got, err := svc.getSchedulableAccount(context.Background(), account.ID)
REDACTED
	require.NotNil(t, got, "first sticky hit fail-opens while free-gate stats refresh")

	require.Eventually(t, func() bool {
		got, err := svc.getSchedulableAccount(context.Background(), account.ID)
		return err == nil && got == nil
REDACTED, 2*time.Second, 10*time.Millisecond, "over free soft-gate sticky hit must miss after cache warm")
REDACTED

func TestOpenAIGetSchedulableAccount_AppliesGrokFreeSoftGate(t *testing.T) {
	// Legacy OpenAI-compatible sticky (advanced scheduler off) must free-gate Grok.
	cfg := &config.Config{REDACTED
	cfg.Gateway.Grok.FreeQuotaSoftGateEnabled = true
	cfg.Gateway.Grok.FreeQuotaTokenLimit = 500_000
	cfg.Gateway.Grok.FreeQuotaSoftGatePercent = 95
	cfg.Gateway.Grok.FreeQuotaWindowHours = 24
	cfg.Gateway.Grok.FreeQuotaStatsCacheSeconds = 60

	account := healthyGrokOAuthGatewayTestAccount(8802, "tok")
	account.Credentials["subscription_tier"] = "free"
	account.Status = StatusActive
	account.Schedulable = true

	repo := &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: accountREDACTED,
REDACTED
	usageRepo := &grokFreeQuotaUsageRepoStub{stats: map[int64]*usagestats.AccountStats{
		account.ID: {Tokens: 480_000REDACTED,
REDACTEDREDACTED
	openaiGrokFreeQuotaGateCache.Range(func(key, _ any) bool {
		openaiGrokFreeQuotaGateCache.Delete(key)
		return true
REDACTED)
	if root, ok := freeQuotaRefreshInFlight.Load(&openaiGrokFreeQuotaGateCache); ok {
		if m, ok := root.(*sync.Map); ok {
			m.Delete(account.ID)
	REDACTED
REDACTED
	svc := &OpenAIGatewayService{
		cfg:          cfg,
		accountRepo:  repo,
		usageLogRepo: usageRepo,
REDACTED

	got, err := svc.getSchedulableAccount(context.Background(), account.ID)
REDACTED
	require.NotNil(t, got, "first sticky hit fail-opens while free-gate stats refresh")

	require.Eventually(t, func() bool {
		got, err := svc.getSchedulableAccount(context.Background(), account.ID)
		return err == nil && got == nil
REDACTED, 2*time.Second, 10*time.Millisecond, "OpenAI legacy sticky must apply free soft-gate after cache warm")
REDACTED

func TestCountGrokNativeSearchCallsFromJSON_MessagesStyleBody(t *testing.T) {
	// Proves the same counter used by Anthropic-buffered Grok /v1/messages path.
	body := []byte(`{"id":"r1","output":[{"type":"web_search_call","id":"ws1"REDACTED,{"type":"message","role":"assistant"REDACTED]REDACTED`)
	require.Equal(t, 1, countGrokNativeSearchCallsFromJSONBytes(body))
REDACTED
