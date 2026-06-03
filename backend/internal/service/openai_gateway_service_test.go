package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/cespare/xxhash/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 编译期接口断言
var _ AccountRepository = (*stubOpenAIAccountRepo)(nil)
var _ GatewayCache = (*stubGatewayCache)(nil)

type stubOpenAIAccountRepo struct {
	AccountRepository
	accounts []Account
REDACTED

type snapshotUpdateAccountRepo struct {
	stubOpenAIAccountRepo
	updateExtraCalls chan map[string]any
REDACTED

func (r *snapshotUpdateAccountRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	if r.updateExtraCalls != nil {
		copied := make(map[string]any, len(updates))
		for k, v := range updates {
			copied[k] = v
	REDACTED
		r.updateExtraCalls <- copied
REDACTED
	return nil
REDACTED

func (r stubOpenAIAccountRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			return &r.accounts[i], nil
	REDACTED
REDACTED
	return nil, errors.New("account not found")
REDACTED

func (r stubOpenAIAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range r.accounts {
		if acc.Platform == platform {
			result = append(result, acc)
	REDACTED
REDACTED
	return result, nil
REDACTED

func (r stubOpenAIAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range r.accounts {
		if acc.Platform == platform {
			result = append(result, acc)
	REDACTED
REDACTED
	return result, nil
REDACTED

func (r stubOpenAIAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
REDACTED

type stubConcurrencyCache struct {
	ConcurrencyCache
	loadBatchErr    error
	loadMap         map[int64]*AccountLoadInfo
	acquireResults  map[int64]bool
	waitCounts      map[int64]int
	skipDefaultLoad bool
REDACTED

type cancelReadCloser struct{REDACTED

func (c cancelReadCloser) Read(p []byte) (int, error) { return 0, context.Canceled REDACTED
func (c cancelReadCloser) Close() error               { return nil REDACTED

type errReadCloser struct {
	err error
REDACTED

func (r errReadCloser) Read([]byte) (int, error) { return 0, r.err REDACTED
func (r errReadCloser) Close() error             { return nil REDACTED

type failingGinWriter struct {
	gin.ResponseWriter
	failAfter int
	writes    int
REDACTED

func (w *failingGinWriter) Write(p []byte) (int, error) {
	if w.writes >= w.failAfter {
		return 0, errors.New("write failed")
REDACTED
	w.writes++
	return w.ResponseWriter.Write(p)
REDACTED

func (c stubConcurrencyCache) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
	if c.acquireResults != nil {
		if result, ok := c.acquireResults[accountID]; ok {
			return result, nil
	REDACTED
REDACTED
	return true, nil
REDACTED

func (c stubConcurrencyCache) ReleaseAccountSlot(ctx context.Context, accountID int64, requestID string) error {
	return nil
REDACTED

func (c stubConcurrencyCache) GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	if c.loadBatchErr != nil {
		return nil, c.loadBatchErr
REDACTED
	out := make(map[int64]*AccountLoadInfo, len(accounts))
	if c.skipDefaultLoad && c.loadMap != nil {
		for _, acc := range accounts {
			if load, ok := c.loadMap[acc.ID]; ok {
				out[acc.ID] = load
		REDACTED
	REDACTED
		return out, nil
REDACTED
	for _, acc := range accounts {
		if c.loadMap != nil {
			if load, ok := c.loadMap[acc.ID]; ok {
				out[acc.ID] = load
				continue
		REDACTED
	REDACTED
		out[acc.ID] = &AccountLoadInfo{AccountID: acc.ID, LoadRate: 0REDACTED
REDACTED
	return out, nil
REDACTED

func TestOpenAIGatewayService_GenerateSessionHash_Priority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

	svc := &OpenAIGatewayService{REDACTED

	bodyWithKey := []byte(`{"prompt_cache_key":"ses_aaa"REDACTED`)

	// 1) session_id header wins
	c.Request.Header.Set("session_id", "sess-123")
	c.Request.Header.Set("conversation_id", "conv-456")
	h1 := svc.GenerateSessionHash(c, bodyWithKey)
	if h1 == "" {
		t.Fatalf("expected non-empty hash")
REDACTED

	// 2) conversation_id used when session_id absent
	c.Request.Header.Del("session_id")
	h2 := svc.GenerateSessionHash(c, bodyWithKey)
	if h2 == "" {
		t.Fatalf("expected non-empty hash")
REDACTED
	if h1 == h2 {
		t.Fatalf("expected different hashes for different keys")
REDACTED

	// 3) prompt_cache_key used when both headers absent
	c.Request.Header.Del("conversation_id")
	h3 := svc.GenerateSessionHash(c, bodyWithKey)
	if h3 == "" {
		t.Fatalf("expected non-empty hash")
REDACTED
	if h2 == h3 {
		t.Fatalf("expected different hashes for different keys")
REDACTED

	// 4) empty when no signals
	h4 := svc.GenerateSessionHash(c, []byte(`{REDACTED`))
	if h4 != "" {
		t.Fatalf("expected empty hash when no signals")
REDACTED
REDACTED

func TestOpenAIGatewayService_GenerateSessionHash_UsesXXHash64(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

	c.Request.Header.Set("session_id", "sess-fixed-value")
	svc := &OpenAIGatewayService{REDACTED

	got := svc.GenerateSessionHash(c, nil)
	want := fmt.Sprintf("%016x", xxhash.Sum64String("sess-fixed-value"))
	require.Equal(t, want, got)
REDACTED

func TestOpenAIGatewayService_GenerateSessionHash_AttachesLegacyHashToContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

	c.Request.Header.Set("session_id", "sess-legacy-check")
	svc := &OpenAIGatewayService{REDACTED

	sessionHash := svc.GenerateSessionHash(c, nil)
	require.NotEmpty(t, sessionHash)
	require.NotNil(t, c.Request)
	require.NotNil(t, c.Request.Context())
	require.NotEmpty(t, openAILegacySessionHashFromContext(c.Request.Context()))
REDACTED

func TestExtractOpenAIResponseIDFromJSONBytes(t *testing.T) {
	require.Equal(t, "resp_json", extractOpenAIResponseIDFromJSONBytes([]byte(`{"id":"resp_json"REDACTED`)))
	require.Equal(t, "resp_sse", extractOpenAIResponseIDFromJSONBytes([]byte(`{"type":"response.completed","response":{"id":"resp_sse"REDACTEDREDACTED`)))
	require.Empty(t, extractOpenAIResponseIDFromJSONBytes([]byte(`{"response":{REDACTEDREDACTED`)))
	require.Empty(t, extractOpenAIResponseIDFromJSONBytes([]byte(`not-json`)))
REDACTED

func TestOpenAIGatewayService_BindHTTPResponseAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	groupID := int64(4201)
	c.Set("api_key", &APIKey{ID: 501, GroupID: &groupIDREDACTED)

	svc := &OpenAIGatewayService{REDACTED
	account := &Account{ID: 37001, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	svc.bindHTTPResponseAccount(context.Background(), c, account, "resp_http_001")

	got, err := svc.getOpenAIWSStateStore().GetResponseAccount(context.Background(), groupID, "resp_http_001")
REDACTED
	require.Equal(t, account.ID, got)
REDACTED

func TestOpenAIGatewayService_GenerateExplicitSessionHash_SkipsContentFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{REDACTED
	body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat"REDACTED`)

	t.Run("stateless image body stays unstuck", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

		require.Empty(t, svc.GenerateExplicitSessionHash(c, body))
		require.Empty(t, openAILegacySessionHashFromContext(c.Request.Context()))
REDACTED)

	t.Run("prompt_cache_key is explicit", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

		got := svc.GenerateExplicitSessionHash(c, []byte(`{"model":"gpt-image-2","prompt_cache_key":"image-session"REDACTED`))
		require.Equal(t, fmt.Sprintf("%016x", xxhash.Sum64String("image-session")), got)
		require.NotEmpty(t, openAILegacySessionHashFromContext(c.Request.Context()))
REDACTED)

	t.Run("header overrides body", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
		c.Request.Header.Set("session_id", "header-session")

		got := svc.GenerateExplicitSessionHash(c, []byte(`{"prompt_cache_key":"body-session"REDACTED`))
		require.Equal(t, fmt.Sprintf("%016x", xxhash.Sum64String("header-session")), got)
REDACTED)
REDACTED

func TestOpenAIGatewayService_GenerateSessionHashWithFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

	svc := &OpenAIGatewayService{REDACTED
	seed := "openai_ws_ingress:9:100:200"

	got := svc.GenerateSessionHashWithFallback(c, []byte(`{REDACTED`), seed)
	want := fmt.Sprintf("%016x", xxhash.Sum64String(seed))
	require.Equal(t, want, got)
	require.NotEmpty(t, openAILegacySessionHashFromContext(c.Request.Context()))

	empty := svc.GenerateSessionHashWithFallback(c, []byte(`{REDACTED`), "   ")
	require.Equal(t, "", empty)
REDACTED

func TestOpenAIGatewayService_GenerateSessionHash_ContentFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)

	svc := &OpenAIGatewayService{REDACTED

	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"You are helpful."REDACTED,{"role":"user","content":"Hello"REDACTED]REDACTED`)

	hash := svc.GenerateSessionHash(c, body)
	require.NotEmpty(t, hash, "content-based fallback should produce a hash")

	hash2 := svc.GenerateSessionHash(c, body)
	require.Equal(t, hash, hash2, "same content should produce same hash")

	bodyExtended := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"You are helpful."REDACTED,{"role":"user","content":"Hello"REDACTED,{"role":"assistant","content":"Hi!"REDACTED,{"role":"user","content":"How are you?"REDACTED]REDACTED`)
	hashExtended := svc.GenerateSessionHash(c, bodyExtended)
	require.Equal(t, hash, hashExtended, "hash should be stable across later turns")

	bodyDifferent := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"Different question"REDACTED]REDACTED`)
	hashDifferent := svc.GenerateSessionHash(c, bodyDifferent)
	require.NotEqual(t, hash, hashDifferent, "different content should produce different hash")
REDACTED

func TestOpenAIGatewayService_GenerateSessionHash_ExplicitSignalWinsOverContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)

	svc := &OpenAIGatewayService{REDACTED
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"Hello"REDACTED]REDACTED`)

	contentHash := svc.GenerateSessionHash(c, body)
	require.NotEmpty(t, contentHash)

	c.Request.Header.Set("session_id", "explicit-session")
	explicitHash := svc.GenerateSessionHash(c, body)
	require.NotEmpty(t, explicitHash)
	require.NotEqual(t, contentHash, explicitHash, "explicit session_id should override content fallback")
REDACTED

func TestOpenAIGatewayService_GenerateSessionHash_EmptyBodyStillEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)

	svc := &OpenAIGatewayService{REDACTED
	require.Empty(t, svc.GenerateSessionHash(c, []byte(`{REDACTED`)))
	require.Empty(t, svc.GenerateSessionHash(c, nil))
REDACTED

func (c stubConcurrencyCache) GetAccountWaitingCount(ctx context.Context, accountID int64) (int, error) {
	if c.waitCounts != nil {
		if count, ok := c.waitCounts[accountID]; ok {
			return count, nil
	REDACTED
REDACTED
	return 0, nil
REDACTED

type stubGatewayCache struct {
	sessionBindings map[string]int64
	deletedSessions map[string]int
REDACTED

func (c *stubGatewayCache) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	if id, ok := c.sessionBindings[sessionHash]; ok {
		return id, nil
REDACTED
	return 0, errors.New("not found")
REDACTED

func (c *stubGatewayCache) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	if c.sessionBindings == nil {
		c.sessionBindings = make(map[string]int64)
REDACTED
	c.sessionBindings[sessionHash] = accountID
	return nil
REDACTED

func (c *stubGatewayCache) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	return nil
REDACTED

func (c *stubGatewayCache) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	if c.sessionBindings == nil {
		return nil
REDACTED
	if c.deletedSessions == nil {
		c.deletedSessions = make(map[string]int)
REDACTED
	c.deletedSessions[sessionHash]++
	delete(c.sessionBindings, sessionHash)
	return nil
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_FiltersUnschedulable(t *testing.T) {
	now := time.Now()
	resetAt := now.Add(10 * time.Minute)
	groupID := int64(1)

	rateLimited := Account{
		ID:               1,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      1,
		Priority:         0,
		RateLimitResetAt: &resetAt,
REDACTED
	available := Account{
		ID:          2,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    1,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{rateLimited, availableREDACTEDREDACTED,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{REDACTED),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-5.2", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil {
		t.Fatalf("expected selection with account")
REDACTED
	if selection.Account.ID != available.ID {
		t.Fatalf("expected account %d, got %d", available.ID, selection.Account.ID)
REDACTED
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_FiltersUnschedulableWhenNoConcurrencyService(t *testing.T) {
	now := time.Now()
	resetAt := now.Add(10 * time.Minute)
	groupID := int64(1)

	rateLimited := Account{
		ID:               1,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      1,
		Priority:         0,
		RateLimitResetAt: &resetAt,
REDACTED
	available := Account{
		ID:          2,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    1,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{rateLimited, availableREDACTEDREDACTED,
		// concurrencyService is nil, forcing the non-load-batch selection path.
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-5.2", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil {
		t.Fatalf("expected selection with account")
REDACTED
	if selection.Account.ID != available.ID {
		t.Fatalf("expected account %d, got %d", available.ID, selection.Account.ID)
REDACTED
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
REDACTED
REDACTED

func TestOpenAISelectAccountForModelWithExclusions_StickyUnschedulableClearsSession(t *testing.T) {
	sessionHash := "session-1"
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusDisabled, Schedulable: true, Concurrency: 1REDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
REDACTED

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
REDACTED
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2, got %+v", acc)
REDACTED
	if cache.deletedSessions["openai:"+sessionHash] != 1 {
		t.Fatalf("expected sticky session to be deleted")
REDACTED
	if cache.sessionBindings["openai:"+sessionHash] != 2 {
		t.Fatalf("expected sticky session to bind to account 2")
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_StickyUnschedulableClearsSession(t *testing.T) {
	sessionHash := "session-2"
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusDisabled, Schedulable: true, Concurrency: 1REDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{REDACTED),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2, got %+v", selection)
REDACTED
	if cache.deletedSessions["openai:"+sessionHash] != 1 {
		t.Fatalf("expected sticky session to be deleted")
REDACTED
	if cache.sessionBindings["openai:"+sessionHash] != 2 {
		t.Fatalf("expected sticky session to bind to account 2")
REDACTED
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
REDACTED
REDACTED

func TestOpenAISelectAccountForModelWithExclusions_NoModelSupport(t *testing.T) {
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
		REDACTED"model_mapping": map[string]any{"gpt-3.5-turbo": "gpt-3.5-turbo"REDACTEDREDACTED,
		REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
REDACTED

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "gpt-4", nil)
	if err == nil {
		t.Fatalf("expected error for unsupported model")
REDACTED
	if acc != nil {
		t.Fatalf("expected nil account for unsupported model")
REDACTED
	if !strings.Contains(err.Error(), "supporting model") {
		t.Fatalf("unexpected error: %v", err)
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_LoadBatchErrorFallback(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2REDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED
	concurrencyCache := stubConcurrencyCache{
		loadBatchErr: errors.New("load batch failed"),
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "fallback", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil {
		t.Fatalf("expected selection")
REDACTED
	if selection.Account.ID != 2 {
		t.Fatalf("expected account 2, got %d", selection.Account.ID)
REDACTED
	if cache.sessionBindings["openai:fallback"] != 2 {
		t.Fatalf("expected sticky session updated")
REDACTED
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_NoSlotFallbackWait(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED
	concurrencyCache := stubConcurrencyCache{
		acquireResults: map[int64]bool{1: falseREDACTED,
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 10REDACTED,
	REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.WaitPlan == nil {
		t.Fatalf("expected wait plan fallback")
REDACTED
	if selection.Account == nil || selection.Account.ID != 1 {
		t.Fatalf("expected account 1")
REDACTED
REDACTED

func TestOpenAISelectAccountForModelWithExclusions_SetsStickyBinding(t *testing.T) {
	sessionHash := "bind"
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
REDACTED

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
REDACTED
	if acc == nil || acc.ID != 1 {
		t.Fatalf("expected account 1")
REDACTED
	if cache.sessionBindings["openai:"+sessionHash] != 1 {
		t.Fatalf("expected sticky session binding")
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_StickyWaitPlan(t *testing.T) {
	sessionHash := "sticky-wait"
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1REDACTED,
REDACTED
	concurrencyCache := stubConcurrencyCache{
		acquireResults: map[int64]bool{1: falseREDACTED,
		waitCounts:     map[int64]int{1: 0REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.WaitPlan == nil {
		t.Fatalf("expected sticky wait plan")
REDACTED
	if selection.Account == nil || selection.Account.ID != 1 {
		t.Fatalf("expected account 1")
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_PrefersLowerLoad(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 80REDACTED,
			2: {AccountID: 2, LoadRate: 10REDACTED,
	REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "load", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2")
REDACTED
	if cache.sessionBindings["openai:load"] != 2 {
		t.Fatalf("expected sticky session updated")
REDACTED
REDACTED

func TestOpenAISelectAccountForModelWithExclusions_StickyExcludedFallback(t *testing.T) {
	sessionHash := "excluded"
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
REDACTED

	excluded := map[int64]struct{REDACTED{1: {REDACTEDREDACTED
	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, sessionHash, "gpt-4", excluded)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
REDACTED
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2")
REDACTED
REDACTED

func TestOpenAISelectAccountForModelWithExclusions_StickyNonOpenAI(t *testing.T) {
	sessionHash := "non-openai"
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
REDACTED

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
REDACTED
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2")
REDACTED
REDACTED

func TestOpenAISelectAccountForModelWithExclusions_NoAccounts(t *testing.T) {
	repo := stubOpenAIAccountRepo{accounts: []Account{REDACTEDREDACTED
	cache := &stubGatewayCache{REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
REDACTED

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "", nil)
	if err == nil {
		t.Fatalf("expected error for no accounts")
REDACTED
	if acc != nil {
		t.Fatalf("expected nil account")
REDACTED
	if !strings.Contains(err.Error(), "no available OpenAI accounts") {
		t.Fatalf("unexpected error: %v", err)
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_NoCandidates(t *testing.T) {
	groupID := int64(1)
	resetAt := time.Now().Add(1 * time.Hour)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, RateLimitResetAt: &resetAtREDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED
	concurrencyCache := stubConcurrencyCache{REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err == nil {
		t.Fatalf("expected error for no candidates")
REDACTED
	if selection != nil {
		t.Fatalf("expected nil selection")
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_AllFullWaitPlan(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 100REDACTED,
	REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.WaitPlan == nil {
		t.Fatalf("expected wait plan")
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_LoadBatchErrorNoAcquire(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED
	concurrencyCache := stubConcurrencyCache{
		loadBatchErr:   errors.New("load batch failed"),
		acquireResults: map[int64]bool{1: falseREDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.WaitPlan == nil {
		t.Fatalf("expected wait plan")
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_MissingLoadInfo(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 50REDACTED,
	REDACTED,
		skipDefaultLoad: true,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2")
REDACTED
REDACTED

func TestOpenAISelectAccountForModelWithExclusions_LeastRecentlyUsed(t *testing.T) {
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-1 * time.Hour)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Priority: 1, LastUsedAt: &newTimeREDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Priority: 1, LastUsedAt: &oldTimeREDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
REDACTED

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
REDACTED
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2")
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_PreferNeverUsed(t *testing.T) {
	groupID := int64(1)
	lastUsed := time.Now().Add(-1 * time.Hour)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, LastUsedAt: &lastUsedREDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 10REDACTED,
			2: {AccountID: 2, LoadRate: 10REDACTED,
	REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2")
REDACTED
REDACTED

func TestOpenAIStreamingTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 1,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{REDACTED,
REDACTED

	start := time.Now()
	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1REDACTED, start, "model", "model")
	_ = pw.Close()
	_ = pr.Close()

	if err == nil || !strings.Contains(err.Error(), "stream data interval timeout") {
		t.Fatalf("expected stream timeout error, got %v", err)
REDACTED
	if !strings.Contains(rec.Body.String(), "\"type\":\"error\"") || !strings.Contains(rec.Body.String(), "stream_timeout") {
		t.Fatalf("expected OpenAI-compatible error SSE event, got %q", rec.Body.String())
REDACTED
REDACTED

func TestOpenAIStreamingContextCanceledReturnsIncompleteErrorWithoutInjectingErrorEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       cancelReadCloser{REDACTED,
		Header:     http.Header{REDACTED,
REDACTED

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model")
	if err == nil || !strings.Contains(err.Error(), "stream usage incomplete") {
		t.Fatalf("expected incomplete stream error, got %v", err)
REDACTED
	if strings.Contains(rec.Body.String(), "event: error") || strings.Contains(rec.Body.String(), "stream_read_error") {
		t.Fatalf("expected no injected SSE error event, got %q", rec.Body.String())
REDACTED
REDACTED

func TestOpenAIStreamingReadErrorBeforeOutputReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       errReadCloser{err: io.ErrUnexpectedEOFREDACTED,
		Header:     http.Header{"X-Request-Id": []string{"rid-disconnect"REDACTEDREDACTED,
REDACTED

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"REDACTED, time.Now(), "model", "model")
REDACTED
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
REDACTED

func TestOpenAIStreamingResponseFailedBeforeOutputReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"REDACTEDREDACTED`,
			"",
			"event: response.in_progress",
			`data: {"type":"response.in_progress","response":{"id":"resp_1"REDACTEDREDACTED`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","error":{"message":"An error occurred while processing your request."REDACTEDREDACTED`,
			"",
	REDACTED, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-failed"REDACTEDREDACTED,
REDACTED

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"REDACTED, time.Now(), "model", "model")
REDACTED
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "An error occurred while processing your request")
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
REDACTED

func TestOpenAIStreamingResponseFailedBeforeOutputCapacityErrorReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"REDACTEDREDACTED`,
			"",
			"event: response.in_progress",
			`data: {"type":"response.in_progress","response":{"id":"resp_1"REDACTEDREDACTED`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","error":{"message":"Selected model is at capacity. Please try a different model.","type":"invalid_request_error"REDACTEDREDACTED`,
			"",
	REDACTED, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-capacity-failed"REDACTEDREDACTED,
REDACTED

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"REDACTED, time.Now(), "model", "model")
REDACTED
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "Selected model is at capacity")
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
REDACTED

func TestOpenAIStreamingPreambleOnlyMissingTerminalReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"REDACTEDREDACTED`,
			"",
			"event: response.in_progress",
			`data: {"type":"response.in_progress","response":{"id":"resp_1"REDACTEDREDACTED`,
			"",
	REDACTED, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-missing-terminal"REDACTEDREDACTED,
REDACTED

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"REDACTED, time.Now(), "model", "model")
REDACTED
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
REDACTED

func TestOpenAIStreamingPreambleKeepaliveUsesDownstreamIdle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   1,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{REDACTED,
REDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"REDACTEDREDACTED\n\n"))
		for i := 0; i < 6; i++ {
			time.Sleep(250 * time.Millisecond)
			_, _ = pw.Write([]byte("data: {\"type\":\"response.in_progress\",\"response\":{\"id\":\"resp_1\"REDACTEDREDACTED\n\n"))
	REDACTED
		_, _ = pw.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2REDACTEDREDACTEDREDACTED\n\n"))
REDACTED()

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"REDACTED, time.Now(), "model", "model")
	_ = pr.Close()
REDACTED
	require.NotNil(t, result)
	require.Contains(t, rec.Body.String(), ":\n\n")
	require.Contains(t, rec.Body.String(), "response.completed")
REDACTED

func TestOpenAIStreamingPolicyResponseFailedBeforeOutputPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"REDACTEDREDACTED`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","error":{"type":"safety_error","message":"This request has been flagged for potentially high-risk cyber activity."REDACTEDREDACTED`,
			"",
	REDACTED, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-policy-failed"REDACTEDREDACTED,
REDACTED

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"REDACTED, time.Now(), "model", "model")
REDACTED
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.True(t, c.Writer.Written())
	require.Contains(t, rec.Body.String(), "response.failed")
	require.Contains(t, rec.Body.String(), "high-risk cyber activity")
REDACTED

func TestOpenAIStreamingClientDisconnectDrainsUpstreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Writer = &failingGinWriter{ResponseWriter: c.Writer, failAfter: 0REDACTED

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{REDACTED,
REDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.in_progress\",\"response\":{REDACTEDREDACTED\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":3,\"output_tokens\":5,\"input_tokens_details\":{\"cached_tokens\":1REDACTEDREDACTEDREDACTEDREDACTED\n\n"))
REDACTED()

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model")
	_ = pr.Close()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
REDACTED
	if result == nil || result.usage == nil {
		t.Fatalf("expected usage result")
REDACTED
	if result.usage.InputTokens != 3 || result.usage.OutputTokens != 5 || result.usage.CacheReadInputTokens != 1 {
		t.Fatalf("unexpected usage: %+v", *result.usage)
REDACTED
	if strings.Contains(rec.Body.String(), "event: error") || strings.Contains(rec.Body.String(), "write_failed") {
		t.Fatalf("expected no injected SSE error event, got %q", rec.Body.String())
REDACTED
REDACTED

func TestOpenAIStreamingMissingTerminalEventReturnsIncompleteError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{REDACTED,
REDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"message\"REDACTED,\"output_index\":0REDACTED\n\n"))
REDACTED()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model")
	_ = pr.Close()
	if err == nil || !strings.Contains(err.Error(), "missing terminal event") {
		t.Fatalf("expected missing terminal event error, got %v", err)
REDACTED
REDACTED

func TestOpenAIStreamingPassthroughMissingTerminalEventReturnsIncompleteError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{REDACTED,
REDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"message\"REDACTED,\"output_index\":0REDACTED\n\n"))
REDACTED()

	_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1REDACTED, time.Now(), "", "")
	_ = pr.Close()
	if err == nil || !strings.Contains(err.Error(), "missing terminal event") {
		t.Fatalf("expected missing terminal event error, got %v", err)
REDACTED
REDACTED

func TestOpenAIStreamingPassthroughResponseFailedBeforeOutputReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"REDACTEDREDACTED`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","error":{"message":"upstream processing failed"REDACTEDREDACTED`,
			"",
	REDACTED, "\n"))),
		Header: http.Header{"X-Request-Id": []string{"rid-passthrough-failed"REDACTEDREDACTED,
REDACTED

	_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"REDACTED, time.Now(), "", "")
REDACTED
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "upstream processing failed")
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
REDACTED

func TestOpenAIStreamingPassthroughResponseDoneWithoutDoneMarkerStillSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{REDACTED,
REDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.done\",\"response\":{\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"input_tokens_details\":{\"cached_tokens\":1REDACTEDREDACTEDREDACTEDREDACTED\n\n"))
REDACTED()

	result, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1REDACTED, time.Now(), "", "")
	_ = pr.Close()
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 2, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)
	require.Equal(t, 1, result.usage.CacheReadInputTokens)
REDACTED

func TestOpenAIStreamingPassthroughResponseIncompleteWithoutDoneMarkerStillSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{REDACTED,
REDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.incomplete\",\"response\":{\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"input_tokens_details\":{\"cached_tokens\":1REDACTEDREDACTEDREDACTEDREDACTED\n\n"))
REDACTED()

	result, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1REDACTED, time.Now(), "", "")
	_ = pr.Close()
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 2, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)
	require.Equal(t, 1, result.usage.CacheReadInputTokens)
REDACTED

func TestOpenAIStreamingTooLong(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               64 * 1024,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{REDACTED,
REDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		// 写入超过 MaxLineSize 的单行数据，触发 ErrTooLong
		payload := "data: " + strings.Repeat("a", 128*1024) + "\n"
		_, _ = pw.Write([]byte(payload))
REDACTED()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 2REDACTED, time.Now(), "model", "model")
	_ = pr.Close()

	if !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("expected ErrTooLong, got %v", err)
REDACTED
	if !strings.Contains(rec.Body.String(), "\"type\":\"error\"") || !strings.Contains(rec.Body.String(), "response_too_large") {
		t.Fatalf("expected OpenAI-compatible error SSE event, got %q", rec.Body.String())
REDACTED
REDACTED

func TestOpenAINonStreamingContentTypePassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			ResponseHeaders: config.ResponseHeaderConfig{Enabled: falseREDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	body := []byte(`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0REDACTEDREDACTEDREDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/vnd.test+json"REDACTEDREDACTED,
REDACTED

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{REDACTED, "model", "model")
	if err != nil {
		t.Fatalf("handleNonStreamingResponse error: %v", err)
REDACTED

	if !strings.Contains(rec.Header().Get("Content-Type"), "application/vnd.test+json") {
		t.Fatalf("expected Content-Type passthrough, got %q", rec.Header().Get("Content-Type"))
REDACTED
REDACTED

func TestOpenAINonStreamingContentTypeDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			ResponseHeaders: config.ResponseHeaderConfig{Enabled: falseREDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	body := []byte(`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0REDACTEDREDACTEDREDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{REDACTED,
REDACTED

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{REDACTED, "model", "model")
	if err != nil {
		t.Fatalf("handleNonStreamingResponse error: %v", err)
REDACTED

	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected default Content-Type, got %q", rec.Header().Get("Content-Type"))
REDACTED
REDACTED

func TestOpenAIStreamingHeadersOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			ResponseHeaders: config.ResponseHeaderConfig{Enabled: falseREDACTED,
	REDACTED,
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header: http.Header{
			"Cache-Control": []string{"upstream"REDACTED,
			"X-Request-Id":  []string{"req-123"REDACTED,
			"Content-Type":  []string{"application/custom"REDACTED,
	REDACTED,
REDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{REDACTEDREDACTED\n\n"))
REDACTED()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model")
	_ = pr.Close()
	if err != nil {
		t.Fatalf("handleStreamingResponse error: %v", err)
REDACTED

	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("expected Cache-Control override, got %q", rec.Header().Get("Cache-Control"))
REDACTED
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected Content-Type override, got %q", rec.Header().Get("Content-Type"))
REDACTED
	if rec.Header().Get("X-Request-Id") != "req-123" {
		t.Fatalf("expected X-Request-Id passthrough, got %q", rec.Header().Get("X-Request-Id"))
REDACTED
REDACTED

func TestOpenAIStreamingReuseScannerBufferAndStillWorks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{REDACTED,
REDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"input_tokens_details\":{\"cached_tokens\":3REDACTEDREDACTEDREDACTEDREDACTED\n\n"))
REDACTED()

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model")
	_ = pr.Close()
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 1, result.usage.InputTokens)
	require.Equal(t, 2, result.usage.OutputTokens)
	require.Equal(t, 3, result.usage.CacheReadInputTokens)
REDACTED

func TestOpenAIInvalidBaseURLWhenAllowlistDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
REDACTED"base_url": "://invalid-url"REDACTED,
REDACTED

	_, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte("{REDACTED"), "token", false, "", false)
	if err == nil {
		t.Fatalf("expected error for invalid base_url when allowlist disabled")
REDACTED
REDACTED

func TestOpenAIValidateUpstreamBaseURLDisabledRequiresHTTPS(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	if _, err := svc.validateUpstreamBaseURL("http://not-https.example.com"); err == nil {
		t.Fatalf("expected http to be rejected when allow_insecure_http is false")
REDACTED
	normalized, err := svc.validateUpstreamBaseURL("https://example.com")
	if err != nil {
		t.Fatalf("expected https to be allowed when allowlist disabled, got %v", err)
REDACTED
	if normalized != "https://example.com" {
		t.Fatalf("expected raw url passthrough, got %q", normalized)
REDACTED
REDACTED

func TestOpenAIValidateUpstreamBaseURLDisabledAllowsHTTP(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowInsecureHTTP: true,
		REDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	normalized, err := svc.validateUpstreamBaseURL("http://not-https.example.com")
	if err != nil {
		t.Fatalf("expected http allowed when allow_insecure_http is true, got %v", err)
REDACTED
	if normalized != "http://not-https.example.com" {
		t.Fatalf("expected raw url passthrough, got %q", normalized)
REDACTED
REDACTED

func TestOpenAIValidateUpstreamBaseURLEnabledEnforcesAllowlist(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:       true,
				UpstreamHosts: []string{"example.com"REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	if _, err := svc.validateUpstreamBaseURL("https://example.com"); err != nil {
		t.Fatalf("expected allowlisted host to pass, got %v", err)
REDACTED
	if _, err := svc.validateUpstreamBaseURL("https://evil.com"); err == nil {
		t.Fatalf("expected non-allowlisted host to fail")
REDACTED
REDACTED

func TestOpenAIUpdateCodexUsageSnapshotFromHeaders(t *testing.T) {
	repo := &snapshotUpdateAccountRepo{updateExtraCalls: make(chan map[string]any, 1)REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	headers := http.Header{REDACTED
	headers.Set("x-codex-primary-used-percent", "12")
	headers.Set("x-codex-secondary-used-percent", "34")
	headers.Set("x-codex-primary-window-minutes", "300")
	headers.Set("x-codex-secondary-window-minutes", "10080")
	headers.Set("x-codex-primary-reset-after-seconds", "600")
	headers.Set("x-codex-secondary-reset-after-seconds", "86400")

	svc.UpdateCodexUsageSnapshotFromHeaders(context.Background(), 123, headers)

	select {
	case updates := <-repo.updateExtraCalls:
		require.Equal(t, 88.0, updates["codex_5h_used_percent"])
		require.Equal(t, 34.0, updates["codex_7d_used_percent"])
		require.Equal(t, 600, updates["codex_5h_reset_after_seconds"])
		require.Equal(t, 86400, updates["codex_7d_reset_after_seconds"])
	case <-time.After(2 * time.Second):
		t.Fatal("expected UpdateExtra to be called")
REDACTED
REDACTED

func TestOpenAIResponsesRequestPathSuffix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	tests := []struct {
		name string
		path string
		want string
REDACTED{
		{name: "exact v1 responses", path: "/v1/responses", want: ""REDACTED,
		{name: "compact v1 responses", path: "/v1/responses/compact", want: "/compact"REDACTED,
		{name: "compact alias responses", path: "/responses/compact/", want: "/compact"REDACTED,
		{name: "nested suffix", path: "/openai/v1/responses/compact/detail", want: "/compact/detail"REDACTED,
		{name: "unrelated path", path: "/v1/chat/completions", want: ""REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)
			require.Equal(t, tt.want, openAIResponsesRequestPathSuffix(c))
	REDACTED)
REDACTED
REDACTED

func TestNormalizeOpenAICompactRequestBodyPreservesCurrentCodexPayloadFields(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"message","role":"user","content":"compact me"REDACTED],"instructions":"compact-test","tools":[{"type":"function","name":"shell"REDACTED],"parallel_tool_calls":true,"reasoning":{"effort":"high"REDACTED,"text":{"verbosity":"low"REDACTED,"previous_response_id":"resp_123","store":true,"stream":true,"prompt_cache_key":"cache_123"REDACTED`)

	normalized, changed, err := normalizeOpenAICompactRequestBody(body)

REDACTED
	require.True(t, changed)
	require.Equal(t, "gpt-5.5", gjson.GetBytes(normalized, "model").String())
	require.True(t, gjson.GetBytes(normalized, "tools").Exists())
	require.True(t, gjson.GetBytes(normalized, "parallel_tool_calls").Bool())
	require.Equal(t, "high", gjson.GetBytes(normalized, "reasoning.effort").String())
	require.Equal(t, "low", gjson.GetBytes(normalized, "text.verbosity").String())
	require.Equal(t, "resp_123", gjson.GetBytes(normalized, "previous_response_id").String())
	require.False(t, gjson.GetBytes(normalized, "store").Exists())
	require.False(t, gjson.GetBytes(normalized, "stream").Exists())
	require.False(t, gjson.GetBytes(normalized, "prompt_cache_key").Exists())
REDACTED

func TestOpenAIBuildUpstreamRequestOpenAIPassthroughPreservesCompactPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5"REDACTED`)))

	svc := &OpenAIGatewayService{REDACTED
	account := &Account{Type: AccountTypeOAuthREDACTED

	req, err := svc.buildUpstreamRequestOpenAIPassthrough(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"REDACTED`), "token")
REDACTED
	require.Equal(t, chatgptCodexURL+"/compact", req.URL.String())
	require.Equal(t, "application/json", req.Header.Get("Accept"))
	require.Equal(t, codexCLIVersion, req.Header.Get("Version"))
	require.NotEmpty(t, req.Header.Get("Session_Id"))
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(req.Context()))
REDACTED

func TestOpenAIBuildUpstreamRequestCompactForcesJSONAcceptForOAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5"REDACTED`)))

	svc := &OpenAIGatewayService{REDACTED
	account := &Account{
		Type:        AccountTypeOAuth,
REDACTED"chatgpt_account_id": "chatgpt-acc"REDACTED,
REDACTED

	req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"REDACTED`), "token", false, "", true)
REDACTED
	require.Equal(t, chatgptCodexURL+"/compact", req.URL.String())
	require.Equal(t, "application/json", req.Header.Get("Accept"))
	require.Equal(t, codexCLIVersion, req.Header.Get("Version"))
	require.NotEmpty(t, req.Header.Get("Session_Id"))
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(req.Context()))
REDACTED

func TestOpenAIBuildUpstreamRequestOAuthMessagesBridgeUsesSessionOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","prompt_cache_key":"anthropic-metadata-session-1","input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"<sub2api-claude-code-todo-guard>"REDACTED]REDACTED,{"type":"message","role":"user","content":"hello"REDACTED]REDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("OpenAI-Beta", "responses=experimental")
	c.Request.Header.Set("originator", "codex_cli_rs")

	svc := &OpenAIGatewayService{REDACTED
	account := &Account{
		Type:        AccountTypeOAuth,
REDACTED"chatgpt_account_id": "chatgpt-acc"REDACTED,
REDACTED

	req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, body, "token", true, "anthropic-metadata-session-1", false)
REDACTED
	require.NotEmpty(t, req.Header.Get("Session_Id"))
	require.Empty(t, req.Header.Get("Conversation_Id"))
	require.Empty(t, req.Header.Get("OpenAI-Beta"))
	require.Empty(t, req.Header.Get("originator"))
REDACTED

func TestOpenAIBuildUpstreamRequestPreservesCompactPathForAPIKeyBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5"REDACTED`)))

	svc := &OpenAIGatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTED,
	REDACTED,
REDACTEDREDACTED
	account := &Account{
		Type:        AccountTypeAPIKey,
		Platform:    PlatformOpenAI,
REDACTED"base_url": "https://example.com/v1"REDACTED,
REDACTED

	req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"REDACTED`), "token", false, "", false)
REDACTED
	require.Equal(t, "https://example.com/v1/responses/compact", req.URL.String())
REDACTED

func TestOpenAIBuildUpstreamRequestOAuthOfficialClientOriginatorCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		userAgent      string
		originator     string
		wantOriginator string
REDACTED{
		{name: "desktop originator preserved", originator: "Codex Desktop", wantOriginator: "Codex Desktop"REDACTED,
		{name: "vscode originator preserved", originator: "codex_vscode", wantOriginator: "codex_vscode"REDACTED,
		{name: "official ua fallback to codex_cli_rs", userAgent: "Codex Desktop/1.2.3", wantOriginator: "codex_cli_rs"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5"REDACTED`)))
			if tt.userAgent != "" {
				c.Request.Header.Set("User-Agent", tt.userAgent)
		REDACTED
			if tt.originator != "" {
				c.Request.Header.Set("originator", tt.originator)
		REDACTED

			svc := &OpenAIGatewayService{REDACTED
			account := &Account{
				Type:        AccountTypeOAuth,
		REDACTED"chatgpt_account_id": "chatgpt-acc"REDACTED,
		REDACTED

			isCodexCLI := openai.IsCodexOfficialClientByHeaders(c.GetHeader("User-Agent"), c.GetHeader("originator"))
			req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"REDACTED`), "token", false, "", isCodexCLI)
		REDACTED
			require.Equal(t, tt.wantOriginator, req.Header.Get("originator"))
	REDACTED)
REDACTED
REDACTED

// ==================== P1-08 修复：model 替换性能优化测试 ====================

// ==================== P1-08 修复：model 替换性能优化测试 =============
func TestReplaceModelInSSELine(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED

	tests := []struct {
		name     string
		line     string
		from     string
		to       string
		expected string
REDACTED{
		{
			name:     "顶层 model 字段替换",
			line:     `data: {"id":"chatcmpl-123","model":"gpt-4o","choices":[]REDACTED`,
			from:     "gpt-4o",
			to:       "my-custom-model",
			expected: `data: {"id":"chatcmpl-123","model":"my-custom-model","choices":[]REDACTED`,
	REDACTED,
		{
			name:     "嵌套 response.model 替换",
			line:     `data: {"type":"response","response":{"id":"resp-1","model":"gpt-4o","output":[]REDACTEDREDACTED`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: {"type":"response","response":{"id":"resp-1","model":"my-model","output":[]REDACTEDREDACTED`,
	REDACTED,
		{
			name:     "model 不匹配时不替换",
			line:     `data: {"id":"chatcmpl-123","model":"gpt-3.5-turbo","choices":[]REDACTED`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: {"id":"chatcmpl-123","model":"gpt-3.5-turbo","choices":[]REDACTED`,
	REDACTED,
		{
			name:     "无 model 字段时不替换",
			line:     `data: {"id":"chatcmpl-123","choices":[]REDACTED`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: {"id":"chatcmpl-123","choices":[]REDACTED`,
	REDACTED,
		{
			name:     "空 data 行",
			line:     `data: `,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: `,
	REDACTED,
		{
			name:     "[DONE] 行",
			line:     `data: [DONE]`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: [DONE]`,
	REDACTED,
		{
			name:     "非 data: 前缀行",
			line:     `event: message`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `event: message`,
	REDACTED,
		{
			name:     "非法 JSON 不替换",
			line:     `data: {invalid jsonREDACTED`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: {invalid jsonREDACTED`,
	REDACTED,
		{
			name:     "无空格 data: 格式",
			line:     `data:{"id":"x","model":"gpt-4o"REDACTED`,
			from:     "gpt-4o",
			to:       "my-model",
			expected: `data: {"id":"x","model":"my-model"REDACTED`,
	REDACTED,
		{
			name:     "model 名含特殊字符",
			line:     `data: {"model":"org/model-v2.1-beta"REDACTED`,
			from:     "org/model-v2.1-beta",
			to:       "custom/alias",
			expected: `data: {"model":"custom/alias"REDACTED`,
	REDACTED,
		{
			name:     "空行",
			line:     "",
			from:     "gpt-4o",
			to:       "my-model",
			expected: "",
	REDACTED,
		{
			name:     "保持其他字段不变",
			line:     `data: {"id":"abc","object":"chat.completion.chunk","model":"gpt-4o","created":1234567890,"choices":[{"index":0,"delta":{"content":"hi"REDACTEDREDACTED]REDACTED`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `data: {"id":"abc","object":"chat.completion.chunk","model":"alias","created":1234567890,"choices":[{"index":0,"delta":{"content":"hi"REDACTEDREDACTED]REDACTED`,
	REDACTED,
		{
			name:     "顶层优先于嵌套：同时存在两个 model",
			line:     `data: {"model":"gpt-4o","response":{"model":"gpt-4o"REDACTEDREDACTED`,
			from:     "gpt-4o",
			to:       "replaced",
			expected: `data: {"model":"replaced","response":{"model":"gpt-4o"REDACTEDREDACTED`,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.replaceModelInSSELine(tt.line, tt.from, tt.to)
			require.Equal(t, tt.expected, got)
	REDACTED)
REDACTED
REDACTED

func TestReplaceModelInSSEBody(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED

	tests := []struct {
		name     string
		body     string
		from     string
		to       string
		expected string
REDACTED{
		{
			name:     "多行 SSE body 替换",
			body:     "data: {\"model\":\"gpt-4o\",\"choices\":[]REDACTED\n\ndata: {\"model\":\"gpt-4o\",\"choices\":[{\"delta\":{\"content\":\"hi\"REDACTEDREDACTED]REDACTED\n\ndata: [DONE]\n",
			from:     "gpt-4o",
			to:       "alias",
			expected: "data: {\"model\":\"alias\",\"choices\":[]REDACTED\n\ndata: {\"model\":\"alias\",\"choices\":[{\"delta\":{\"content\":\"hi\"REDACTEDREDACTED]REDACTED\n\ndata: [DONE]\n",
	REDACTED,
		{
			name:     "无需替换的 body",
			body:     "data: {\"model\":\"gpt-3.5-turbo\"REDACTED\n\ndata: [DONE]\n",
			from:     "gpt-4o",
			to:       "alias",
			expected: "data: {\"model\":\"gpt-3.5-turbo\"REDACTED\n\ndata: [DONE]\n",
	REDACTED,
		{
			name:     "混合 event 和 data 行",
			body:     "event: message\ndata: {\"model\":\"gpt-4o\"REDACTED\n\n",
			from:     "gpt-4o",
			to:       "alias",
			expected: "event: message\ndata: {\"model\":\"alias\"REDACTED\n\n",
	REDACTED,
		{
			name:     "空 body",
			body:     "",
			from:     "gpt-4o",
			to:       "alias",
			expected: "",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.replaceModelInSSEBody(tt.body, tt.from, tt.to)
			require.Equal(t, tt.expected, got)
	REDACTED)
REDACTED
REDACTED

func TestReplaceModelInResponseBody(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED

	tests := []struct {
		name     string
		body     string
		from     string
		to       string
		expected string
REDACTED{
		{
			name:     "替换顶层 model",
			body:     `{"id":"chatcmpl-123","model":"gpt-4o","choices":[]REDACTED`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `{"id":"chatcmpl-123","model":"alias","choices":[]REDACTED`,
	REDACTED,
		{
			name:     "model 不匹配不替换",
			body:     `{"id":"chatcmpl-123","model":"gpt-3.5-turbo","choices":[]REDACTED`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `{"id":"chatcmpl-123","model":"gpt-3.5-turbo","choices":[]REDACTED`,
	REDACTED,
		{
			name:     "无 model 字段不替换",
			body:     `{"id":"chatcmpl-123","choices":[]REDACTED`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `{"id":"chatcmpl-123","choices":[]REDACTED`,
	REDACTED,
		{
			name:     "非法 JSON 返回原值",
			body:     `not json`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `not json`,
	REDACTED,
		{
			name:     "空 body 返回原值",
			body:     ``,
			from:     "gpt-4o",
			to:       "alias",
			expected: ``,
	REDACTED,
		{
			name:     "保持嵌套结构不变",
			body:     `{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":20REDACTED,"choices":[{"message":{"role":"assistant","content":"hello"REDACTEDREDACTED]REDACTED`,
			from:     "gpt-4o",
			to:       "alias",
			expected: `{"model":"alias","usage":{"prompt_tokens":10,"completion_tokens":20REDACTED,"choices":[{"message":{"role":"assistant","content":"hello"REDACTEDREDACTED]REDACTED`,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.replaceModelInResponseBody([]byte(tt.body), tt.from, tt.to)
			require.Equal(t, tt.expected, string(got))
	REDACTED)
REDACTED
REDACTED

func TestExtractOpenAISSEDataLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantData string
		wantOK   bool
REDACTED{
		{name: "标准格式", line: `data: {"type":"x"REDACTED`, wantData: `{"type":"x"REDACTED`, wantOK: trueREDACTED,
		{name: "无空格格式", line: `data:{"type":"x"REDACTED`, wantData: `{"type":"x"REDACTED`, wantOK: trueREDACTED,
		{name: "纯空数据", line: `data:   `, wantData: ``, wantOK: trueREDACTED,
		{name: "非 data 行", line: `event: message`, wantData: ``, wantOK: falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractOpenAISSEDataLine(tt.line)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.wantData, got)
	REDACTED)
REDACTED
REDACTED

func TestParseSSEUsage_SelectiveParsing(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	usage := &OpenAIUsage{InputTokens: 9, OutputTokens: 8, CacheReadInputTokens: 7REDACTED

	// 非 completed 事件，不应覆盖 usage
	svc.parseSSEUsage(`{"type":"response.in_progress","response":{"usage":{"input_tokens":1,"output_tokens":2REDACTEDREDACTEDREDACTED`, usage)
	require.Equal(t, 9, usage.InputTokens)
	require.Equal(t, 8, usage.OutputTokens)
	require.Equal(t, 7, usage.CacheReadInputTokens)

	// completed 事件，应提取 usage
	svc.parseSSEUsage(`{"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":5,"input_tokens_details":{"cached_tokens":2REDACTEDREDACTEDREDACTEDREDACTED`, usage)
	require.Equal(t, 3, usage.InputTokens)
	require.Equal(t, 5, usage.OutputTokens)
	require.Equal(t, 2, usage.CacheReadInputTokens)

	// done 事件同样可能携带最终 usage
	svc.parseSSEUsage(`{"type":"response.done","response":{"usage":{"input_tokens":13,"output_tokens":15,"input_tokens_details":{"cached_tokens":4REDACTEDREDACTEDREDACTEDREDACTED`, usage)
	require.Equal(t, 13, usage.InputTokens)
	require.Equal(t, 15, usage.OutputTokens)
	require.Equal(t, 4, usage.CacheReadInputTokens)

	// failed 事件在部分上游路径也会携带已消耗 usage，应与 WS/passthrough 保持一致
	svc.parseSSEUsage(`{"type":"response.failed","response":{"usage":{"input_tokens":17,"output_tokens":19,"input_tokens_details":{"cached_tokens":6REDACTEDREDACTEDREDACTEDREDACTED`, usage)
	require.Equal(t, 17, usage.InputTokens)
	require.Equal(t, 19, usage.OutputTokens)
	require.Equal(t, 6, usage.CacheReadInputTokens)

	svc.parseSSEUsage(`{"type":"response.completed","response":{"usage":{"prompt_tokens":21,"completion_tokens":8,"prompt_tokens_details":{"cached_tokens":6REDACTEDREDACTEDREDACTEDREDACTED`, usage)
	require.Equal(t, 21, usage.InputTokens)
	require.Equal(t, 8, usage.OutputTokens)
	require.Equal(t, 6, usage.CacheReadInputTokens)
REDACTED

func TestExtractOpenAIUsageFromJSONBytes_AcceptsResponseAndChatUsageShapes(t *testing.T) {
	usage, ok := extractOpenAIUsageFromJSONBytes([]byte(`{"id":"resp_1","usage":{"input_tokens":3,"output_tokens":5,"input_tokens_details":{"cached_tokens":2REDACTEDREDACTEDREDACTED`))
	require.True(t, ok)
	require.Equal(t, 3, usage.InputTokens)
	require.Equal(t, 5, usage.OutputTokens)
	require.Equal(t, 2, usage.CacheReadInputTokens)

	usage, ok = extractOpenAIUsageFromJSONBytes([]byte(`{"type":"response.completed","response":{"usage":{"prompt_tokens":13,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":4REDACTEDREDACTEDREDACTEDREDACTED`))
	require.True(t, ok)
	require.Equal(t, 13, usage.InputTokens)
	require.Equal(t, 7, usage.OutputTokens)
	require.Equal(t, 4, usage.CacheReadInputTokens)
REDACTED

func TestExtractCodexFinalResponse_SampleReplay(t *testing.T) {
	body := strings.Join([]string{
		`event: message`,
		`data: {"type":"response.in_progress","response":{"id":"resp_1"REDACTEDREDACTED`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","usage":{"input_tokens":11,"output_tokens":22,"input_tokens_details":{"cached_tokens":3REDACTEDREDACTEDREDACTEDREDACTED`,
		`data: [DONE]`,
REDACTED, "\n")

	finalResp, ok := extractCodexFinalResponse(body)
	require.True(t, ok)
	require.Contains(t, string(finalResp), `"id":"resp_1"`)
	require.Contains(t, string(finalResp), `"input_tokens":11`)
REDACTED

func TestHandleSSEToJSON_CompletedEventReturnsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{REDACTEDREDACTED
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
REDACTED
	body := []byte(strings.Join([]string{
		`data: {"type":"response.in_progress","response":{"id":"resp_2"REDACTEDREDACTED`,
		`data: {"type":"response.completed","response":{"id":"resp_2","model":"gpt-4o","usage":{"input_tokens":7,"output_tokens":9,"input_tokens_details":{"cached_tokens":1REDACTEDREDACTEDREDACTEDREDACTED`,
		`data: [DONE]`,
REDACTED, "\n"))

	usage, err := svc.handleSSEToJSON(resp, c, body, "gpt-4o", "gpt-4o")
REDACTED
	require.NotNil(t, usage)
	require.Equal(t, 7, usage.InputTokens)
	require.Equal(t, 9, usage.OutputTokens)
	require.Equal(t, 1, usage.CacheReadInputTokens)
	// Header 可能由上游 Content-Type 透传；关键是 body 已转换为最终 JSON 响应。
	require.NotContains(t, rec.Body.String(), "event:")
	require.Contains(t, rec.Body.String(), `"id":"resp_2"`)
	require.NotContains(t, rec.Body.String(), "data:")
REDACTED

func TestHandleNonStreamingResponse_APIKeyFallsBackToSSEBodyWhenContentTypeIsWrong(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{REDACTEDREDACTED
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.output_text.delta","delta":"hel"REDACTED`,
			`data: {"type":"response.output_text.delta","delta":"lo"REDACTED`,
			`data: {"type":"response.completed","response":{"id":"resp_api_key_sse","object":"response","model":"gpt-5.4","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5REDACTEDREDACTEDREDACTED`,
			`data: [DONE]`,
	REDACTED, "\n"))),
REDACTED
	account := &Account{ID: 1, Type: AccountTypeAPIKeyREDACTED

	result, err := svc.handleNonStreamingResponse(context.Background(), resp, c, account, "gpt-5.4", "gpt-5.4")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, 3, result.InputTokens)
	require.Equal(t, 2, result.OutputTokens)
	require.NotContains(t, rec.Body.String(), "data:")
	require.Equal(t, "resp_api_key_sse", gjson.Get(rec.Body.String(), "id").String())
	require.Equal(t, "hello", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
REDACTED

func TestHandleSSEToJSON_ReconstructsImageGenerationOutputItemDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{REDACTEDREDACTED
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
REDACTED
	body := []byte(strings.Join([]string{
		`data: {"type":"response.output_item.done","item":{"id":"ig_123","type":"image_generation_call","result":"aGVsbG8=","revised_prompt":"draw a cat","output_format":"png"REDACTEDREDACTED`,
		`data: {"type":"response.completed","response":{"id":"resp_img","model":"gpt-5.4","output":[],"usage":{"input_tokens":7,"output_tokens":9,"output_tokens_details":{"image_tokens":4REDACTEDREDACTEDREDACTEDREDACTED`,
		`data: [DONE]`,
REDACTED, "\n"))

	usage, err := svc.handleSSEToJSON(resp, c, body, "gpt-5.4", "gpt-5.4")
REDACTED
	require.NotNil(t, usage)
	require.Equal(t, 4, usage.ImageOutputTokens)
	require.NotContains(t, rec.Body.String(), "data:")
	require.Equal(t, "image_generation_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, "aGVsbG8=", gjson.Get(rec.Body.String(), "output.0.result").String())
	require.Equal(t, "draw a cat", gjson.Get(rec.Body.String(), "output.0.revised_prompt").String())
REDACTED

func TestHandleSSEToJSON_NoFinalResponseKeepsSSEBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{REDACTEDREDACTED
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
REDACTED
	body := []byte(strings.Join([]string{
		`data: {"type":"response.in_progress","response":{"id":"resp_3"REDACTEDREDACTED`,
		`data: [DONE]`,
REDACTED, "\n"))

	usage, err := svc.handleSSEToJSON(resp, c, body, "gpt-4o", "gpt-4o")
REDACTED
	require.NotNil(t, usage)
	require.Equal(t, 0, usage.InputTokens)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
	require.Contains(t, rec.Body.String(), `data: {"type":"response.in_progress"`)
REDACTED

func TestHandleSSEToJSON_ResponseFailedReturnsProtocolError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{REDACTEDREDACTED
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
REDACTED
	body := []byte(strings.Join([]string{
		`data: {"type":"response.failed","error":{"message":"upstream rejected request"REDACTEDREDACTED`,
		`data: [DONE]`,
REDACTED, "\n"))

	usage, err := svc.handleSSEToJSON(resp, c, body, "gpt-4o", "gpt-4o")
	require.Nil(t, usage)
REDACTED
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "upstream rejected request")
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
REDACTED

func TestOpenAICompatSSEFrameParserResetsEventTypeAtFrameBoundary(t *testing.T) {
	var parser openAICompatSSEFrameParser

	frame, ok := parser.AddLine("event: response.created")
	require.False(t, ok)
	require.Empty(t, frame)

	frame, ok = parser.AddLine(`data: {"response":{"id":"resp_1"REDACTEDREDACTED`)
	require.False(t, ok)
	require.Empty(t, frame)

	frame, ok = parser.AddLine("")
	require.True(t, ok)
	require.Equal(t, "response.created", frame.EventType)
	require.JSONEq(t, `{"response":{"id":"resp_1"REDACTEDREDACTED`, frame.Data)

	frame, ok = parser.AddLine(`data: {"delta":"ok"REDACTED`)
	require.False(t, ok)
	require.Empty(t, frame.EventType)

	frame, ok = parser.AddLine("")
	require.True(t, ok)
	require.Empty(t, frame.EventType)
	require.JSONEq(t, `{"delta":"ok"REDACTED`, frame.Data)
REDACTED
