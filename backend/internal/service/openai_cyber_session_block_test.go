package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newCyberBlockTestCtx(headers map[string]string, body string) (*gin.Context, []byte) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("POST", "/openai/v1/responses", strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
REDACTED
	c.Request = req
	return c, []byte(body)
REDACTED

func TestCyberSessionExplicitBlockKey(t *testing.T) {
	c1, b1 := newCyberBlockTestCtx(map[string]string{"session_id": "sess-abc"REDACTED, `{REDACTED`)
	k1 := CyberSessionExplicitBlockKey(101, c1, b1)
	require.NotEmpty(t, k1)

	// Same session, different apiKey → different key (isolation).
	c2, b2 := newCyberBlockTestCtx(map[string]string{"session_id": "sess-abc"REDACTED, `{REDACTED`)
	require.NotEqual(t, k1, CyberSessionExplicitBlockKey(202, c2, b2))

	// Same session + same apiKey → stable key.
	c3, b3 := newCyberBlockTestCtx(map[string]string{"session_id": "sess-abc"REDACTED, `{REDACTED`)
	require.Equal(t, k1, CyberSessionExplicitBlockKey(101, c3, b3))

	// prompt_cache_key in body counts as explicit.
	c4, b4 := newCyberBlockTestCtx(nil, `{"prompt_cache_key":"pck-1"REDACTED`)
	require.NotEmpty(t, CyberSessionExplicitBlockKey(101, c4, b4))

	// No explicit signal → empty key → caller must skip blocking entirely.
	c5, b5 := newCyberBlockTestCtx(nil, `{"input":"hello world"REDACTED`)
	require.Empty(t, CyberSessionExplicitBlockKey(101, c5, b5))

	// conversation_id header counts as explicit; key is stable and non-empty.
	c6, b6 := newCyberBlockTestCtx(map[string]string{"conversation_id": "conv-xyz"REDACTED, `{REDACTED`)
	k6 := CyberSessionExplicitBlockKey(101, c6, b6)
	require.NotEmpty(t, k6)
	c6b, b6b := newCyberBlockTestCtx(map[string]string{"conversation_id": "conv-xyz"REDACTED, `{REDACTED`)
	require.Equal(t, k6, CyberSessionExplicitBlockKey(101, c6b, b6b), "conversation_id key must be stable")
REDACTED

func TestCyberTranscriptBlockKeysRequireModelGeneratedHistory(t *testing.T) {
	first := []byte(`{"instructions":"shared","input":[{"role":"user","content":"fixed environment"REDACTED,{"role":"user","content":"question one"REDACTED]REDACTED`)
	second := []byte(`{"instructions":"shared","input":[{"role":"user","content":"fixed environment"REDACTED,{"role":"user","content":"question two"REDACTED]REDACTED`)
	firstKeys := CyberSessionTranscriptBlockKeys(77, first)
	secondKeys := CyberSessionTranscriptBlockKeys(77, second)
	require.Len(t, firstKeys, 1)
	require.Len(t, secondKeys, 1)
	require.NotEqual(t, firstKeys[0], secondKeys[0])

	hit := []byte(`{"messages":[{"role":"user","content":"setup"REDACTED,{"role":"assistant","content":"ready"REDACTED,{"role":"user","content":"trigger"REDACTED]REDACTED`)
	continuation := []byte(`{"messages":[{"role":"user","content":"setup"REDACTED,{"role":"assistant","content":"ready"REDACTED,{"role":"user","content":"different trigger"REDACTED,{"role":"assistant","content":"blocked"REDACTED,{"role":"user","content":"continue"REDACTED]REDACTED`)
	hitKeys := CyberSessionTranscriptBlockKeys(77, hit)
	require.Len(t, hitKeys, 2)
	require.Contains(t, CyberSessionTranscriptLookupKeys(77, continuation), hitKeys[1])
REDACTED

func TestCyberTranscriptBlockKeysWebSocketResponseCreate(t *testing.T) {
	body := []byte(`{"type":"response.create","response":{"prompt_cache_key":"ws-session","input":[{"role":"user","content":"setup"REDACTED,{"role":"assistant","content":"ready"REDACTED,{"role":"user","content":"trigger"REDACTED]REDACTEDREDACTED`)
	c, _ := newCyberBlockTestCtx(nil, string(body))
	require.NotEmpty(t, CyberSessionExplicitBlockKey(88, c, body))
	require.Len(t, CyberSessionTranscriptBlockKeys(88, body), 2)
REDACTED

func TestCyberTranscriptLookupKeysAreBoundedAndKeepNewestOrder(t *testing.T) {
	messages := make([]map[string]string, maxOpenAICyberTranscriptLookupKeys+44)
	for i := range messages {
		messages[i] = map[string]string{"role": "user", "content": "message-" + strconv.Itoa(i)REDACTED
REDACTED
	body, err := json.Marshal(map[string]any{"messages": messagesREDACTED)
REDACTED

	keys := CyberSessionTranscriptLookupKeys(77, body)
	require.Len(t, keys, maxOpenAICyberTranscriptLookupKeys)

	firstRetainedBody, err := json.Marshal(map[string]any{"messages": messages[:45]REDACTED)
REDACTED
	firstRetainedPrefix := CyberSessionTranscriptLookupKeys(77, firstRetainedBody)
	require.Equal(t, firstRetainedPrefix[len(firstRetainedPrefix)-1], keys[0])

	fullKey := CyberSessionTranscriptBlockKeys(77, body)[0]
	require.Equal(t, fullKey, keys[len(keys)-1])
REDACTED

// --- fakes ---

type fakeCyberBlockStore struct {
	blocked   map[string]bool
	scopes    map[string]bool
	findCalls int
REDACTED

var _ CyberSessionBlockStore = (*fakeCyberBlockStore)(nil)

func (f *fakeCyberBlockStore) SetCyberSessionBlocked(_ context.Context, scopeKey string, keys []string, _ time.Duration) error {
	if f.blocked == nil {
		f.blocked = map[string]bool{REDACTED
REDACTED
	for _, key := range keys {
		f.blocked[key] = true
REDACTED
	if scopeKey != "" {
		if f.scopes == nil {
			f.scopes = map[string]bool{REDACTED
	REDACTED
		f.scopes[scopeKey] = true
REDACTED
	return nil
REDACTED

func (f *fakeCyberBlockStore) IsCyberSessionScopeActive(_ context.Context, scopeKey string) (bool, error) {
	return f.scopes[scopeKey], nil
REDACTED

func (f *fakeCyberBlockStore) FindCyberSessionBlocked(_ context.Context, keys []string) (string, error) {
	f.findCalls++
	for _, key := range keys {
		if f.blocked[key] {
			return key, nil
	REDACTED
REDACTED
	return "", nil
REDACTED

// fakeSettingRepo is a minimal SettingRepository stub for unit tests.
// Only GetValue is exercised by GetCyberSessionBlockRuntime; all other methods
// panic so accidental calls are caught immediately.
type fakeSettingRepo struct {
	vals map[string]string
REDACTED

func (r *fakeSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	v, ok := r.vals[key]
	if !ok {
		return "", ErrSettingNotFound
REDACTED
	return v, nil
REDACTED
func (r *fakeSettingRepo) Get(_ context.Context, _ string) (*Setting, error) {
	panic("fakeSettingRepo.Get not implemented")
REDACTED
func (r *fakeSettingRepo) Set(_ context.Context, _, _ string) error {
	panic("fakeSettingRepo.Set not implemented")
REDACTED
func (r *fakeSettingRepo) GetMultiple(_ context.Context, _ []string) (map[string]string, error) {
	panic("fakeSettingRepo.GetMultiple not implemented")
REDACTED
func (r *fakeSettingRepo) SetMultiple(_ context.Context, _ map[string]string) error {
	panic("fakeSettingRepo.SetMultiple not implemented")
REDACTED
func (r *fakeSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	panic("fakeSettingRepo.GetAll not implemented")
REDACTED
func (r *fakeSettingRepo) Delete(_ context.Context, _ string) error {
	panic("fakeSettingRepo.Delete not implemented")
REDACTED

var _ SettingRepository = (*fakeSettingRepo)(nil)

// comboCacheAndStore implements both GatewayCache (no-op stubs) and
// CyberSessionBlockStore (delegates to fakeCyberBlockStore) so it can be
// injected as s.cache and successfully type-asserted to CyberSessionBlockStore.
type comboCacheAndStore struct {
	store fakeCyberBlockStore
REDACTED

var _ GatewayCache = (*comboCacheAndStore)(nil)
var _ CyberSessionBlockStore = (*comboCacheAndStore)(nil)

func (c *comboCacheAndStore) GetSessionAccountID(_ context.Context, _ int64, _ string) (int64, error) {
	return 0, errors.New("stub")
REDACTED
func (c *comboCacheAndStore) SetSessionAccountID(_ context.Context, _ int64, _ string, _ int64, _ time.Duration) error {
	return nil
REDACTED
func (c *comboCacheAndStore) RefreshSessionTTL(_ context.Context, _ int64, _ string, _ time.Duration) error {
	return nil
REDACTED
func (c *comboCacheAndStore) DeleteSessionAccountID(_ context.Context, _ int64, _ string) error {
	return nil
REDACTED

func (c *comboCacheAndStore) SetGrokVideoPendingBilling(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
REDACTED
func (c *comboCacheAndStore) GetGrokVideoPendingBilling(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
REDACTED
func (c *comboCacheAndStore) ClaimGrokVideoBilled(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
REDACTED

func (c *comboCacheAndStore) ReleaseGrokVideoBilled(_ context.Context, _ string) error {
	return nil
REDACTED

func (c *comboCacheAndStore) SetReasoningContent(_ context.Context, _ string, _ string, _ time.Duration) error {
	return nil
REDACTED
func (c *comboCacheAndStore) GetReasoningContent(_ context.Context, _ string) (string, error) {
	return "", ErrReasoningContentNotFound
REDACTED

func (c *comboCacheAndStore) SetCyberSessionBlocked(ctx context.Context, scopeKey string, keys []string, ttl time.Duration) error {
	return c.store.SetCyberSessionBlocked(ctx, scopeKey, keys, ttl)
REDACTED
func (c *comboCacheAndStore) IsCyberSessionScopeActive(ctx context.Context, scopeKey string) (bool, error) {
	return c.store.IsCyberSessionScopeActive(ctx, scopeKey)
REDACTED
func (c *comboCacheAndStore) FindCyberSessionBlocked(ctx context.Context, keys []string) (string, error) {
	return c.store.FindCyberSessionBlocked(ctx, keys)
REDACTED

// --- tests ---

// TestIsCyberSessionBlocked_EmptyKeyAndNilService covers the fail-open paths:
// empty key, nil service, store missing → always false / no panic.
func TestFindCyberSessionBlocked_EmptyAndNilService(t *testing.T) {
	var nilSvc *OpenAIGatewayService
	require.Empty(t, nilSvc.FindCyberSessionBlockedForRequest(context.Background(), 1, nil, nil, "", ""))
	require.NotPanics(t, func() { nilSvc.MarkCyberSessionBlocked(context.Background(), "", []string{"k"REDACTED) REDACTED)

	svc := &OpenAIGatewayService{REDACTED
	require.Empty(t, svc.FindCyberSessionBlockedForRequest(context.Background(), 1, nil, nil, "", ""))
REDACTED

// TestCyberSessionBlock_RoundTrip exercises the type-assertion success path:
// mark a session blocked via a combo cache+store, then confirm IsCyberSessionBlocked
// returns true, and an unrelated key returns false.
func TestCyberSessionBlock_RoundTrip(t *testing.T) {
	// SettingService with only settingRepo set — GetCyberSessionBlockRuntime needs
	// nothing else (cfg/proxyRepo/etc. are not touched by this code path).
	settingSvc := &SettingService{
		settingRepo: &fakeSettingRepo{
			vals: map[string]string{
				SettingKeyCyberSessionBlockEnabled:    "true",
				SettingKeyCyberSessionBlockTTLSeconds: "60",
		REDACTED,
	REDACTED,
REDACTED

	combo := &comboCacheAndStore{REDACTED
	svc := &OpenAIGatewayService{
		cache:          combo,
		settingService: settingSvc,
REDACTED

	ctx := context.Background()
	const testKey = "deadbeef1234"

	c, body := newCyberBlockTestCtx(map[string]string{"session_id": "sess-roundtrip"REDACTED, `{REDACTED`)
	explicitKey := CyberSessionExplicitBlockKey(1, c, body)
	require.Empty(t, svc.FindCyberSessionBlockedForRequest(ctx, 1, c, body, "203.0.113.1", "client/1.0"))

	svc.MarkCyberSessionBlocked(ctx, "", []string{explicitKey, testKeyREDACTED)

	require.Equal(t, explicitKey, svc.FindCyberSessionBlockedForRequest(ctx, 1, c, body, "203.0.113.1", "client/1.0"))
REDACTED

func TestFindCyberSessionBlockedForRequestUsesScopeForTranscript(t *testing.T) {
	settingSvc := &SettingService{settingRepo: &fakeSettingRepo{vals: map[string]string{
		SettingKeyCyberSessionBlockEnabled:    "true",
		SettingKeyCyberSessionBlockTTLSeconds: "60",
REDACTEDREDACTEDREDACTED
	combo := &comboCacheAndStore{REDACTED
	svc := &OpenAIGatewayService{cache: combo, settingService: settingSvcREDACTED
	ctx := context.Background()

	hitBody := []byte(`{"messages":[{"role":"user","content":"setup"REDACTED,{"role":"assistant","content":"ready"REDACTED,{"role":"user","content":"trigger"REDACTED]REDACTED`)
	nextBody := []byte(`{"messages":[{"role":"user","content":"setup"REDACTED,{"role":"assistant","content":"ready"REDACTED,{"role":"user","content":"different trigger"REDACTED,{"role":"assistant","content":"blocked"REDACTED,{"role":"user","content":"continue"REDACTED]REDACTED`)
	nextCtx, _ := newCyberBlockTestCtx(nil, string(nextBody))
	const clientIP = "203.0.113.20"
	const userAgent = "Codex CLI 1.2.3"
	blockKey := CyberSessionTranscriptBlockKeys(9, hitBody)[1]

	// Without an active source scope, transcript candidates are never blocks.
	require.Empty(t, svc.FindCyberSessionBlockedForRequest(ctx, 9, nextCtx, nextBody, clientIP, userAgent))
	scopeKey := CyberSessionScopeKey(9, clientIP, userAgent)
	svc.MarkCyberSessionBlocked(ctx, scopeKey, []string{blockKeyREDACTED)
	require.Equal(t, blockKey, svc.FindCyberSessionBlockedForRequest(ctx, 9, nextCtx, nextBody, clientIP, "Codex CLI 1.2.4"))
REDACTED

func TestFindCyberSessionBlockedForRequestFailsClosedOnScopedTranscriptOverflow(t *testing.T) {
	settingSvc := &SettingService{settingRepo: &fakeSettingRepo{vals: map[string]string{
		SettingKeyCyberSessionBlockEnabled:    "true",
		SettingKeyCyberSessionBlockTTLSeconds: "60",
REDACTEDREDACTEDREDACTED
	combo := &comboCacheAndStore{REDACTED
	svc := &OpenAIGatewayService{cache: combo, settingService: settingSvcREDACTED
	ctx := context.Background()
	const apiKeyID = int64(9)
	const clientIP = "203.0.113.20"
	const userAgent = "Codex CLI 1.2.3"

	messages := make([]map[string]string, maxOpenAICyberTranscriptLookupKeys+1)
	for i := range messages {
		messages[i] = map[string]string{"role": "user", "content": "message-" + strconv.Itoa(i)REDACTED
REDACTED
	body, err := json.Marshal(map[string]any{"messages": messagesREDACTED)
REDACTED
	c, _ := newCyberBlockTestCtx(nil, string(body))
	require.Empty(t, svc.FindCyberSessionBlockedForRequest(ctx, apiKeyID, c, body, clientIP, userAgent),
		"overflow alone must not bypass the scope gate")
	combo.store.scopes = map[string]bool{CyberSessionScopeKey(apiKeyID, clientIP, userAgent): trueREDACTED

	require.Equal(t, cyberSessionTranscriptLookupOverflowBlockKey,
		svc.FindCyberSessionBlockedForRequest(ctx, apiKeyID, c, body, clientIP, userAgent))
	require.Zero(t, combo.store.findCalls, "overflow must not issue an unbounded Redis lookup")
REDACTED

func TestCyberSessionScopeKeyNormalizesUserAgentVersion(t *testing.T) {
	base := CyberSessionScopeKey(7, "203.0.113.10", "Codex CLI 1.2.3")
	require.NotEmpty(t, base)
	require.Equal(t, base, CyberSessionScopeKey(7, "203.0.113.10", "Codex CLI 1.2.4"))
	require.NotEqual(t, base, CyberSessionScopeKey(8, "203.0.113.10", "Codex CLI 1.2.3"))
	require.NotEqual(t, base, CyberSessionScopeKey(7, "203.0.113.11", "Codex CLI 1.2.3"))
REDACTED
