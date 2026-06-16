package service

import (
	"context"
	"errors"
	"net/http/httptest"
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

// TestCyberSessionBlockKey verifies F5a key derivation: explicit session signals
// only (header session_id/conversation_id or body prompt_cache_key), apiKey
// isolated, and EMPTY when no explicit signal (no content-derived fallback —
// "不退化" decision).
func TestCyberSessionBlockKey(t *testing.T) {
	c1, b1 := newCyberBlockTestCtx(map[string]string{"session_id": "sess-abc"REDACTED, `{REDACTED`)
	k1 := CyberSessionBlockKey(101, c1, b1)
	require.NotEmpty(t, k1)

	// Same session, different apiKey → different key (isolation).
	c2, b2 := newCyberBlockTestCtx(map[string]string{"session_id": "sess-abc"REDACTED, `{REDACTED`)
	require.NotEqual(t, k1, CyberSessionBlockKey(202, c2, b2))

	// Same session + same apiKey → stable key.
	c3, b3 := newCyberBlockTestCtx(map[string]string{"session_id": "sess-abc"REDACTED, `{REDACTED`)
	require.Equal(t, k1, CyberSessionBlockKey(101, c3, b3))

	// prompt_cache_key in body counts as explicit.
	c4, b4 := newCyberBlockTestCtx(nil, `{"prompt_cache_key":"pck-1"REDACTED`)
	require.NotEmpty(t, CyberSessionBlockKey(101, c4, b4))

	// No explicit signal → empty key → caller must skip blocking entirely.
	c5, b5 := newCyberBlockTestCtx(nil, `{"input":"hello world"REDACTED`)
	require.Empty(t, CyberSessionBlockKey(101, c5, b5))

	// conversation_id header counts as explicit; key is stable and non-empty.
	c6, b6 := newCyberBlockTestCtx(map[string]string{"conversation_id": "conv-xyz"REDACTED, `{REDACTED`)
	k6 := CyberSessionBlockKey(101, c6, b6)
	require.NotEmpty(t, k6)
	c6b, b6b := newCyberBlockTestCtx(map[string]string{"conversation_id": "conv-xyz"REDACTED, `{REDACTED`)
	require.Equal(t, k6, CyberSessionBlockKey(101, c6b, b6b), "conversation_id key must be stable")
REDACTED

// --- fakes ---

type fakeCyberBlockStore struct {
	blocked map[string]bool
REDACTED

var _ CyberSessionBlockStore = (*fakeCyberBlockStore)(nil)

func (f *fakeCyberBlockStore) SetCyberSessionBlocked(_ context.Context, key string, _ time.Duration) error {
	if f.blocked == nil {
		f.blocked = map[string]bool{REDACTED
REDACTED
	f.blocked[key] = true
	return nil
REDACTED

func (f *fakeCyberBlockStore) IsCyberSessionBlocked(_ context.Context, key string) (bool, error) {
	return f.blocked[key], nil
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
func (c *comboCacheAndStore) SetCyberSessionBlocked(ctx context.Context, key string, ttl time.Duration) error {
	return c.store.SetCyberSessionBlocked(ctx, key, ttl)
REDACTED
func (c *comboCacheAndStore) IsCyberSessionBlocked(ctx context.Context, key string) (bool, error) {
	return c.store.IsCyberSessionBlocked(ctx, key)
REDACTED

// --- tests ---

// TestIsCyberSessionBlocked_EmptyKeyAndNilService covers the fail-open paths:
// empty key, nil service, store missing → always false / no panic.
func TestIsCyberSessionBlocked_EmptyKeyAndNilService(t *testing.T) {
	var nilSvc *OpenAIGatewayService
	require.False(t, nilSvc.IsCyberSessionBlocked(context.Background(), "k"))
	require.NotPanics(t, func() { nilSvc.MarkCyberSessionBlocked(context.Background(), "k") REDACTED)

	svc := &OpenAIGatewayService{REDACTED
	require.False(t, svc.IsCyberSessionBlocked(context.Background(), ""))
	require.False(t, svc.IsCyberSessionBlocked(context.Background(), "k"), "no store + no settings → fail-open false")
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

	// Before marking: not blocked.
	require.False(t, svc.IsCyberSessionBlocked(ctx, testKey))

	// Mark as blocked.
	svc.MarkCyberSessionBlocked(ctx, testKey)

	// After marking: blocked.
	require.True(t, svc.IsCyberSessionBlocked(ctx, testKey))

	// Different key: still not blocked.
	require.False(t, svc.IsCyberSessionBlocked(ctx, "other-key"))
REDACTED
