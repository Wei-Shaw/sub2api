//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type grokCredentialHandlerRepo struct {
	service.AccountRepository
	mu             sync.Mutex
	accounts       []service.Account
	setErrorIDs    []int64
	setTempIDs     []int64
	rateLimitIDs   []int64
	updateExtraIDs []int64
	selectionCalls int
	setErrorErr    error
	setTempErr     error
	missingOnGet   map[int64]bool
REDACTED

func (r *grokCredentialHandlerRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.selectionCalls++
	out := make([]service.Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.Platform == platform && account.IsSchedulable() {
			out = append(out, account)
	REDACTED
REDACTED
	return out, nil
REDACTED

func (r *grokCredentialHandlerRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, _ int64, platform string) ([]service.Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
REDACTED

func (r *grokCredentialHandlerRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
REDACTED

func (r *grokCredentialHandlerRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.missingOnGet[id] {
		return nil, nil
REDACTED
	for _, account := range r.accounts {
		if account.ID == id {
			copy := account
			copy.Credentials = cloneCredentialMap(account.Credentials)
			return &copy, nil
	REDACTED
REDACTED
	return nil, nil
REDACTED

func (r *grokCredentialHandlerRepo) SetError(_ context.Context, id int64, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setErrorIDs = append(r.setErrorIDs, id)
	if r.setErrorErr != nil {
		return r.setErrorErr
REDACTED
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			r.accounts[i].Status = service.StatusError
			r.accounts[i].Schedulable = false
			r.accounts[i].ErrorMessage = message
	REDACTED
REDACTED
	return nil
REDACTED

func (r *grokCredentialHandlerRepo) SetTempUnschedulable(_ context.Context, id int64, until time.Time, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setTempIDs = append(r.setTempIDs, id)
	if r.setTempErr != nil {
		return r.setTempErr
REDACTED
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			value := until
			r.accounts[i].TempUnschedulableUntil = &value
	REDACTED
REDACTED
	return nil
REDACTED

func (r *grokCredentialHandlerRepo) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rateLimitIDs = append(r.rateLimitIDs, id)
	for i := range r.accounts {
		if r.accounts[i].ID != id {
			continue
	REDACTED
		now := time.Now()
		r.accounts[i].RateLimitedAt = &now
		value := resetAt
		r.accounts[i].RateLimitResetAt = &value
REDACTED
	return nil
REDACTED

func (r *grokCredentialHandlerRepo) SetRateLimitedIfLater(ctx context.Context, id int64, resetAt time.Time) error {
	r.mu.Lock()
	for i := range r.accounts {
		if r.accounts[i].ID == id && r.accounts[i].RateLimitResetAt != nil && !resetAt.After(*r.accounts[i].RateLimitResetAt) {
			r.mu.Unlock()
			return nil
	REDACTED
REDACTED
	r.mu.Unlock()
	return r.SetRateLimited(ctx, id, resetAt)
REDACTED

func (r *grokCredentialHandlerRepo) SetGrokCredentialErrorIfMatch(
	_ context.Context,
	id int64,
	snapshot service.GrokCredentialMutationSnapshot,
	message string,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.accounts {
		account := &r.accounts[i]
		if account.ID != id || !handlerGrokCredentialSnapshotMatches(account, snapshot) {
			continue
	REDACTED
		r.setErrorIDs = append(r.setErrorIDs, id)
		if r.setErrorErr != nil {
			return false, r.setErrorErr
	REDACTED
		account.Status = service.StatusError
		account.Schedulable = false
		account.ErrorMessage = message
		return true, nil
REDACTED
	return false, nil
REDACTED

func (r *grokCredentialHandlerRepo) SetGrokCredentialTempUnschedulableIfMatch(
	_ context.Context,
	id int64,
	snapshot service.GrokCredentialMutationSnapshot,
	until time.Time,
	_ string,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.accounts {
		account := &r.accounts[i]
		if account.ID != id || !handlerGrokCredentialSnapshotMatches(account, snapshot) {
			continue
	REDACTED
		r.setTempIDs = append(r.setTempIDs, id)
		if r.setTempErr != nil {
			return false, r.setTempErr
	REDACTED
		value := until
		account.TempUnschedulableUntil = &value
		return true, nil
REDACTED
	return false, nil
REDACTED

func handlerGrokCredentialSnapshotMatches(account *service.Account, snapshot service.GrokCredentialMutationSnapshot) bool {
	if account == nil {
		return false
REDACTED
	credentialsJSON, err := json.Marshal(account.Credentials)
	return err == nil && account.IsGrokOAuth() && account.IsSchedulable() && string(credentialsJSON) == snapshot.CredentialsJSON &&
		handlerGrokCredentialProxyIDsEqual(account.ProxyID, snapshot.ProxyID)
REDACTED

func handlerGrokCredentialProxyIDsEqual(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
REDACTED
	return *left == *right
REDACTED

func (r *grokCredentialHandlerRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updateExtraIDs = append(r.updateExtraIDs, id)
	for i := range r.accounts {
		if r.accounts[i].ID != id {
			continue
	REDACTED
		if r.accounts[i].Extra == nil {
			r.accounts[i].Extra = map[string]any{REDACTED
	REDACTED
		for key, value := range updates {
			r.accounts[i].Extra[key] = value
	REDACTED
REDACTED
	return nil
REDACTED

func (r *grokCredentialHandlerRepo) errorIDs() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int64(nil), r.setErrorIDs...)
REDACTED

func (r *grokCredentialHandlerRepo) selectorCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.selectionCalls
REDACTED

func (r *grokCredentialHandlerRepo) rateLimitedAccountIDs() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int64(nil), r.rateLimitIDs...)
REDACTED

type grokCredentialHandlerTokenCache struct {
	service.GrokTokenCache
	mu        sync.Mutex
	deleteErr error
REDACTED

func (c *grokCredentialHandlerTokenCache) GetAccessToken(context.Context, string) (string, error) {
	return "", errors.New("not cached")
REDACTED

func (c *grokCredentialHandlerTokenCache) SetAccessToken(context.Context, string, string, time.Duration) error {
	return nil
REDACTED

func (c *grokCredentialHandlerTokenCache) DeleteAccessToken(context.Context, string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.deleteErr
REDACTED

func (c *grokCredentialHandlerTokenCache) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return true, nil
REDACTED

func (c *grokCredentialHandlerTokenCache) ReleaseRefreshLock(context.Context, string) error {
	return nil
REDACTED

func cloneCredentialMap(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
REDACTED
	return cloned
REDACTED

type grokCredentialHandlerRefresher struct {
	mode    string
	started chan struct{REDACTED
	once    sync.Once
REDACTED

func (r *grokCredentialHandlerRefresher) CacheKey(account *service.Account) string {
	return service.GrokTokenCacheKey(account)
REDACTED

func (r *grokCredentialHandlerRefresher) CanRefresh(account *service.Account) bool {
	return account != nil && account.IsGrokOAuth()
REDACTED

func (r *grokCredentialHandlerRefresher) NeedsRefresh(account *service.Account, _ time.Duration) bool {
	return account != nil && (account.ID == 801 || r.mode == "all_revoked")
REDACTED

func (r *grokCredentialHandlerRefresher) Refresh(ctx context.Context, _ *service.Account) (map[string]any, error) {
	switch r.mode {
	case "revoked", "all_revoked", "mutation_set_error", "mutation_cache":
		return nil, infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "invalid_grant")
	case "provider":
		return nil, infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "invalid_client")
	case "cancel":
		r.once.Do(func() { close(r.started) REDACTED)
		<-ctx.Done()
		return nil, ctx.Err()
	case "transient", "mutation_temp":
		return nil, errors.New("temporary refresh transport failure")
	default:
		return nil, nil
REDACTED
REDACTED

type grokCredentialHandlerUpstream struct {
	service.HTTPUpstream
	mu            sync.Mutex
	hits          []int64
	requestURLs   []string
	authorization []string
	failAccountID int64
	rateLimitIDs  map[int64]bool
	failureStatus map[int64]int
	cancelRequest context.CancelFunc
REDACTED

func (u *grokCredentialHandlerUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	var requestBody []byte
	if req.Body != nil {
		requestBody, _ = io.ReadAll(req.Body)
REDACTED
	u.mu.Lock()
	u.hits = append(u.hits, accountID)
	u.requestURLs = append(u.requestURLs, req.URL.String())
	u.authorization = append(u.authorization, req.Header.Get("Authorization"))
	failAccountID := u.failAccountID
	rateLimited := u.rateLimitIDs[accountID]
	failureStatus := u.failureStatus[accountID]
	cancelRequest := u.cancelRequest
	u.mu.Unlock()
	if rateLimited {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Content-Type": []string{"application/json"REDACTED,
				"Retry-After":  []string{"60"REDACTED,
		REDACTED,
			Body: io.NopCloser(bytes.NewBufferString(`{"error":{"message":"rate limited"REDACTEDREDACTED`)),
	REDACTED, nil
REDACTED
	if failureStatus > 0 {
		return &http.Response{
			StatusCode: failureStatus,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error":{"message":"upstream unavailable"REDACTEDREDACTED`)),
	REDACTED, nil
REDACTED
	if accountID == failAccountID {
		if cancelRequest != nil {
			cancelRequest()
	REDACTED
		return &http.Response{
			StatusCode: http.StatusPaymentRequired,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error":{"message":"payment required"REDACTEDREDACTED`)),
	REDACTED, nil
REDACTED
	if bytes.Contains(requestBody, []byte(`"stream":true`)) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
			Body: io.NopCloser(bytes.NewBufferString(
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_healthy\",\"model\":\"grok-4.5\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1REDACTEDREDACTEDREDACTED\n\n",
			)),
	REDACTED, nil
REDACTED
	if strings.Contains(req.URL.Path, "/chat/completions") {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body: io.NopCloser(bytes.NewBufferString(
				`{"id":"chatcmpl_healthy","object":"chat.completion","model":"grok-4.5","choices":[{"index":0,"message":{"role":"assistant","content":"ok"REDACTED,"finish_reason":"stop"REDACTED],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2REDACTEDREDACTED`,
			)),
	REDACTED, nil
REDACTED
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body: io.NopCloser(bytes.NewBufferString(
			`{"id":"resp_healthy","object":"response","model":"grok-4.5","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTED`,
		)),
REDACTED, nil
REDACTED

func (u *grokCredentialHandlerUpstream) accountHits() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.hits...)
REDACTED

func (u *grokCredentialHandlerUpstream) requests() ([]string, []string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.requestURLs...), append([]string(nil), u.authorization...)
REDACTED

func TestResponsesCredentialFailoverLoop(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("revoked account selects healthy account", func(t *testing.T) {
		h, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "revoked")
		defer cleanup()
		_ = h

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":falseREDACTED`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Contains(t, recorder.Body.String(), "resp_healthy")
		require.Equal(t, []int64{801REDACTED, repo.errorIDs())
		require.Equal(t, []int64{802REDACTED, upstream.accountHits())
		requestURLs, authorization := upstream.requests()
		require.Equal(t, []string{xai.DefaultCLIBaseURL + "/responses"REDACTED, requestURLs)
		require.Equal(t, []string{"Bearer healthy-access"REDACTED, authorization)
REDACTED)

	t.Run("provider configuration stops before healthy account", func(t *testing.T) {
		h, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "provider")
		defer cleanup()

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":falseREDACTED`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		require.Contains(t, recorder.Body.String(), service.GrokCredentialUnavailableClientMessage)
		require.Empty(t, repo.errorIDs())
		require.Empty(t, upstream.accountHits())
		require.Equal(t, 1, repo.selectorCalls())
		require.Zero(t, h.gatewayService.SnapshotOpenAIAccountSchedulerMetrics().RuntimeStatsAccountCount,
			"provider-scoped auth failure must not penalize the selected account")
REDACTED)

	t.Run("parent cancellation stops before healthy account", func(t *testing.T) {
		_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "cancel")
		defer cleanup()

		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":falseREDACTED`)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		done := make(chan struct{REDACTED)
		go func() {
			defer close(done)
			router.ServeHTTP(recorder, req)
	REDACTED()

		select {
		case <-time.After(2 * time.Second):
			t.Fatal("credential refresh did not start")
		case <-findHandlerRefresherStarted(router):
			cancel()
	REDACTED
		select {
		case <-time.After(2 * time.Second):
			t.Fatal("handler did not stop after cancellation")
		case <-done:
	REDACTED

		require.Empty(t, repo.errorIDs())
		require.Empty(t, upstream.accountHits())
REDACTED)

	t.Run("post-mapping cancellation stops before scheduler mutation or reselection", func(t *testing.T) {
		h, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "postmap_cancel")
		defer cleanup()
		ctx, cancel := context.WithCancel(context.Background())
		upstream.mu.Lock()
		upstream.failAccountID = 801
		upstream.cancelRequest = cancel
		upstream.mu.Unlock()

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":falseREDACTED`)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)

		require.Equal(t, []int64{801REDACTED, upstream.accountHits())
		require.Empty(t, repo.errorIDs())
		require.Equal(t, 1, repo.selectorCalls())
		require.Zero(t, h.gatewayService.SnapshotOpenAIAccountSchedulerMetrics().RuntimeStatsAccountCount)
REDACTED)

	t.Run("pre-cancelled request never invokes an account selector", func(t *testing.T) {
		tests := []struct {
			name   string
			method string
			path   string
			body   string
	REDACTED{
			{name: "responses", method: http.MethodPost, path: "/openai/v1/responses", body: `{"model":"grok","input":"hello","stream":falseREDACTED`REDACTED,
			{name: "messages", method: http.MethodPost, path: "/openai/v1/messages", body: `{"model":"grok","max_tokens":16,"messages":[{"role":"user","content":"hello"REDACTED]REDACTED`REDACTED,
			{name: "chat completions", method: http.MethodPost, path: "/openai/v1/chat/completions", body: `{"model":"grok","messages":[{"role":"user","content":"hello"REDACTED],"stream":falseREDACTED`REDACTED,
			{name: "grok media", method: http.MethodGet, path: "/openai/v1/videos/request-1"REDACTED,
	REDACTED
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "revoked")
				defer cleanup()
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				recorder := httptest.NewRecorder()
				req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body)).WithContext(ctx)
				req.Header.Set("Content-Type", "application/json")

				router.ServeHTTP(recorder, req)

				require.Zero(t, repo.selectorCalls())
				require.Empty(t, upstream.accountHits())
		REDACTED)
	REDACTED
REDACTED)

	t.Run("credential state mutation failures stop before reselection", func(t *testing.T) {
		for _, mode := range []string{"mutation_set_error", "mutation_temp", "mutation_cache"REDACTED {
			t.Run(mode, func(t *testing.T) {
				_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, mode)
				defer cleanup()

				recorder := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":falseREDACTED`))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(recorder, req)

				require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
				require.Contains(t, recorder.Body.String(), service.GrokCredentialUnavailableClientMessage)
				require.Empty(t, upstream.accountHits())
				require.Equal(t, 1, repo.selectorCalls())
		REDACTED)
	REDACTED
REDACTED)

	t.Run("missing credential provider stops before upstream or reselection", func(t *testing.T) {
		_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "nil_provider")
		defer cleanup()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":falseREDACTED`))
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
		require.Contains(t, recorder.Body.String(), service.GrokCredentialUnavailableClientMessage)
		require.Equal(t, 1, repo.selectorCalls())
		require.Empty(t, upstream.accountHits())
		require.Empty(t, repo.errorIDs())
REDACTED)
REDACTED

func TestResponsesGrok429FailoverIsBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("first rate limited account selects healthy account", func(t *testing.T) {
		_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "first_429")
		defer cleanup()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":falseREDACTED`))
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Contains(t, recorder.Body.String(), "resp_healthy")
		require.Equal(t, []int64{801, 802REDACTED, upstream.accountHits())
		require.Equal(t, []int64{801REDACTED, repo.rateLimitedAccountIDs())
REDACTED)

	t.Run("two rate limited accounts stop without sweeping the pool", func(t *testing.T) {
		_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "all_429")
		defer cleanup()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":falseREDACTED`))
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusTooManyRequests, recorder.Code, recorder.Body.String())
		require.Equal(t, []int64{801, 802REDACTED, upstream.accountHits())
		require.Equal(t, []int64{801, 802REDACTED, repo.rateLimitedAccountIDs())
		require.NotContains(t, recorder.Body.String(), "expired")
		require.NotContains(t, recorder.Body.String(), "healthy-access")
		require.NotContains(t, recorder.Body.String(), "rate limited")
REDACTED)
REDACTED

func TestResponsesGrok429FailoverHandlesMixedStatuses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("429 then 500 stops after the bounded followup", func(t *testing.T) {
		_, _, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "mixed_429_500")
		defer cleanup()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":falseREDACTED`))
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusBadGateway, recorder.Code, recorder.Body.String())
		require.Equal(t, []int64{801, 802REDACTED, upstream.accountHits())
		require.NotContains(t, recorder.Body.String(), "upstream unavailable")
REDACTED)

	t.Run("500 then 429 permits one healthy followup", func(t *testing.T) {
		_, _, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "mixed_500_429")
		defer cleanup()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":falseREDACTED`))
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Equal(t, []int64{801, 802, 803REDACTED, upstream.accountHits())
REDACTED)

	t.Run("OAuth 429 then API-key failure cannot bypass the bound", func(t *testing.T) {
		_, _, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "oauth_429_apikey_500")
		defer cleanup()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":falseREDACTED`))
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusBadGateway, recorder.Code, recorder.Body.String())
		require.Equal(t, []int64{801, 802REDACTED, upstream.accountHits())
REDACTED)
REDACTED

func TestGrokMedia429FailoverIsBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("first 429 selects one healthy followup", func(t *testing.T) {
		_, _, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "first_429")
		defer cleanup()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/videos/generations", bytes.NewBufferString(`{"model":"grok-imagine-video","prompt":"waves"REDACTED`))
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Equal(t, []int64{801, 802REDACTED, upstream.accountHits())
REDACTED)

	t.Run("second 429 stops without sweeping a third account", func(t *testing.T) {
		_, _, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "all_429")
		defer cleanup()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/videos/generations", bytes.NewBufferString(`{"model":"grok-imagine-video","prompt":"waves"REDACTED`))
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusTooManyRequests, recorder.Code, recorder.Body.String())
		require.Equal(t, []int64{801, 802REDACTED, upstream.accountHits())
		require.NotContains(t, recorder.Body.String(), "rate limited")
REDACTED)
REDACTED

func TestGrokOAuthCredentialFailoverAcrossHTTPHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	endpoints := []struct {
		name   string
		method string
		path   string
		body   string
REDACTED{
		{name: "messages", method: http.MethodPost, path: "/openai/v1/messages", body: `{"model":"grok","max_tokens":16,"messages":[{"role":"user","content":"hello"REDACTED]REDACTED`REDACTED,
		{name: "chat completions", method: http.MethodPost, path: "/openai/v1/chat/completions", body: `{"model":"grok","messages":[{"role":"user","content":"hello"REDACTED],"stream":falseREDACTED`REDACTED,
		{name: "chat completions raw fallback", method: http.MethodPost, path: "/openai/v1/chat/completions", body: `{"model":"grok","messages":[{"role":"user","content":"hello"REDACTED],"stop":["END"],"stream":falseREDACTED`REDACTED,
		{name: "grok media", method: http.MethodPost, path: "/openai/v1/videos/generations", body: `{"model":"grok-imagine-video","prompt":"waves"REDACTED`REDACTED,
REDACTED

	for _, endpoint := range endpoints {
		t.Run(endpoint.name+" revoked selects healthy", func(t *testing.T) {
			_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "revoked")
			defer cleanup()
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(endpoint.method, endpoint.path, bytes.NewBufferString(endpoint.body))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(recorder, req)

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			require.Equal(t, []int64{801REDACTED, repo.errorIDs())
			require.Equal(t, []int64{802REDACTED, upstream.accountHits())
	REDACTED)

		t.Run(endpoint.name+" all accounts exhausted safely", func(t *testing.T) {
			_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "all_revoked")
			defer cleanup()
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(endpoint.method, endpoint.path, bytes.NewBufferString(endpoint.body))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(recorder, req)

			require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
			require.Contains(t, recorder.Body.String(), service.GrokCredentialUnavailableClientMessage)
			require.NotContains(t, recorder.Body.String(), "revoked-refresh")
			require.NotContains(t, recorder.Body.String(), "healthy-refresh")
			require.Equal(t, []int64{801, 802REDACTED, repo.errorIDs())
			require.Empty(t, upstream.accountHits())
	REDACTED)
REDACTED
REDACTED

func TestGrokOAuthMissingSelectedRowRetriesHealthyAccountWithoutMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "missing_row")
	defer cleanup()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":falseREDACTED`))
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, []int64{802REDACTED, upstream.accountHits())
	require.Empty(t, repo.errorIDs())
	require.Empty(t, repo.setTempIDs)
REDACTED

func TestResponsesWebSocketCredentialFailoverLoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dial := func(t *testing.T, router *gin.Engine) (*coderws.Conn, func()) {
	REDACTED
		server := httptest.NewServer(router)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/openai/v1/responses", nil)
		cancel()
	REDACTED
		return conn, func() {
			_ = conn.CloseNow()
			server.Close()
	REDACTED
REDACTED
	writeFirst := func(t *testing.T, conn *coderws.Conn) {
	REDACTED
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"grok","input":"hello","stream":falseREDACTED`)))
REDACTED

	t.Run("revoked account selects healthy account", func(t *testing.T) {
		_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "revoked")
		defer cleanup()
		conn, closeConn := dial(t, router)
		defer closeConn()
		writeFirst(t, conn)

		readCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, payload, err := conn.Read(readCtx)
		cancel()
	REDACTED
		require.Contains(t, string(payload), "resp_healthy")
		require.Equal(t, []int64{801REDACTED, repo.errorIDs())
		require.Equal(t, 2, repo.selectorCalls())
		require.Equal(t, []int64{802REDACTED, upstream.accountHits())
REDACTED)

	t.Run("provider configuration stops", func(t *testing.T) {
		_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "provider")
		defer cleanup()
		conn, closeConn := dial(t, router)
		defer closeConn()
		writeFirst(t, conn)

		readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, _, err := conn.Read(readCtx)
		cancel()
		var closeErr coderws.CloseError
		require.ErrorAs(t, err, &closeErr)
		require.Contains(t, closeErr.Reason, service.GrokCredentialUnavailableClientMessage)
		require.Equal(t, 1, repo.selectorCalls())
		require.Empty(t, upstream.accountHits())
REDACTED)

	t.Run("parent cancellation prevents reselection", func(t *testing.T) {
		_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "cancel")
		defer cleanup()
		conn, closeConn := dial(t, router)
		writeFirst(t, conn)
		select {
		case <-findHandlerRefresherStarted(router):
		case <-time.After(2 * time.Second):
			t.Fatal("credential refresh did not start")
	REDACTED
		closeConn()

		require.Eventually(t, func() bool { return repo.selectorCalls() == 1 REDACTED, 2*time.Second, 20*time.Millisecond)
		require.Empty(t, repo.errorIDs())
		require.Empty(t, upstream.accountHits())
REDACTED)
REDACTED

var handlerRefresherStarted sync.Map

func findHandlerRefresherStarted(router *gin.Engine) <-chan struct{REDACTED {
	value, _ := handlerRefresherStarted.Load(router)
	return value.(chan struct{REDACTED)
REDACTED

func newGrokCredentialFailoverHandler(t *testing.T, mode string) (*OpenAIGatewayHandler, *grokCredentialHandlerRepo, *grokCredentialHandlerUpstream, *gin.Engine, func()) {
REDACTED
	groupID := int64(901)
	accounts := []service.Account{
		{
			ID: 801, Name: "revoked", Platform: service.PlatformGrok, Type: service.AccountTypeOAuth,
			Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: 1,
	REDACTED
				"access_token": "expired", "refresh_token": "revoked-refresh",
				"expires_at": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
		REDACTED,
	REDACTED,
		{
			ID: 802, Name: "healthy", Platform: service.PlatformGrok, Type: service.AccountTypeOAuth,
			Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: 2,
	REDACTED
				"access_token": "healthy-access", "refresh_token": "healthy-refresh",
				"expires_at": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		REDACTED,
	REDACTED,
REDACTED
	if mode == "postmap_cancel" || mode == "first_429" || mode == "all_429" || mode == "mixed_429_500" || mode == "mixed_500_429" || mode == "oauth_429_apikey_500" {
		accounts[0].Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
REDACTED
	if mode == "all_429" || mode == "mixed_429_500" || mode == "mixed_500_429" || mode == "oauth_429_apikey_500" {
		accounts = append(accounts, service.Account{
			ID: 803, Name: "untried-healthy", Platform: service.PlatformGrok, Type: service.AccountTypeOAuth,
			Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: 3,
	REDACTED
				"access_token": "untried-healthy-access", "refresh_token": "untried-healthy-refresh",
				"expires_at": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		REDACTED,
	REDACTED)
REDACTED
	if mode == "oauth_429_apikey_500" {
		accounts[1].Type = service.AccountTypeAPIKey
		accounts[1].Credentials = map[string]any{"api_key": "third-party-key"REDACTED
REDACTED
	if mode == "all_revoked" {
		accounts[1].Credentials["expires_at"] = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
REDACTED
	repo := &grokCredentialHandlerRepo{accounts: accounts, missingOnGet: map[int64]bool{REDACTEDREDACTED
	if mode == "missing_row" {
		repo.missingOnGet[801] = true
REDACTED
	if mode == "mutation_set_error" {
		repo.setErrorErr = errors.New("database write failed")
REDACTED
	if mode == "mutation_temp" {
		repo.setTempErr = errors.New("database write failed")
REDACTED
	refresher := &grokCredentialHandlerRefresher{mode: mode, started: make(chan struct{REDACTED)REDACTED
	tokenCache := &grokCredentialHandlerTokenCache{REDACTED
	if mode == "mutation_cache" {
		tokenCache.deleteErr = errors.New("cache delete failed")
REDACTED
	var provider *service.GrokTokenProvider
	if mode != "nil_provider" {
		provider = service.NewGrokTokenProvider(repo, tokenCache)
		provider.SetRefreshAPI(service.NewOAuthRefreshAPI(repo, tokenCache), refresher)
REDACTED
	upstream := &grokCredentialHandlerUpstream{REDACTED
	switch mode {
	case "first_429":
		upstream.rateLimitIDs = map[int64]bool{801: trueREDACTED
	case "all_429":
		upstream.rateLimitIDs = map[int64]bool{801: true, 802: trueREDACTED
	case "mixed_429_500":
		upstream.rateLimitIDs = map[int64]bool{801: trueREDACTED
		upstream.failureStatus = map[int64]int{802: http.StatusInternalServerErrorREDACTED
	case "mixed_500_429":
		upstream.failureStatus = map[int64]int{801: http.StatusInternalServerErrorREDACTED
		upstream.rateLimitIDs = map[int64]bool{802: trueREDACTED
	case "oauth_429_apikey_500":
		upstream.rateLimitIDs = map[int64]bool{801: trueREDACTED
		upstream.failureStatus = map[int64]int{802: http.StatusInternalServerErrorREDACTED
REDACTED
	cfg := &config.Config{RunMode: config.RunModeSimpleREDACTED
	cfg.Gateway.MaxAccountSwitches = 3
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	gateway := service.NewOpenAIGatewayService(
		repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCache, upstream,
		&service.DeferredService{REDACTED, nil, provider, nil, nil, nil, nil, nil,
	)
	cache := &concurrencyCacheMock{
		acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil REDACTED,
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil REDACTED,
REDACTED
	h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(cache), billingCache, &service.APIKeyService{REDACTED, nil, nil, nil, nil, cfg)
	apiKey := &service.APIKey{
		ID: 902, GroupID: &groupID,
		User:  &service.User{ID: 903, Status: service.StatusActiveREDACTED,
		Group: &service.Group{ID: groupID, Platform: service.PlatformGrok, Status: service.StatusActive, AllowImageGeneration: trueREDACTED,
REDACTED
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.User.ID, Concurrency: 1REDACTED)
		c.Next()
REDACTED)
	router.POST("/openai/v1/responses", h.Responses)
	router.GET("/openai/v1/responses", h.ResponsesWebSocket)
	router.POST("/openai/v1/messages", h.Messages)
	router.POST("/openai/v1/chat/completions", h.ChatCompletions)
	router.POST("/openai/v1/videos/generations", h.GrokVideoGeneration)
	router.GET("/openai/v1/videos/:request_id", h.GrokVideoStatus)
	handlerRefresherStarted.Store(router, refresher.started)
	cleanup := func() {
		handlerRefresherStarted.Delete(router)
		billingCache.Stop()
REDACTED
	return h, repo, upstream, router, cleanup
REDACTED
