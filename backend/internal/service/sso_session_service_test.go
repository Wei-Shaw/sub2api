package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ─── 测试基础设施 ────────────────────────────────────────────────────────────

// fixedNowSsoSession 给 service 注入一个可控时钟。
func fixedNowSsoSession(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// memSettingRepoForSso 提供一个最小的 SettingRepository 实现，仅供本测试文件使用。
type memSettingRepoForSso struct {
	mu     sync.Mutex
	values map[string]string
}

func newMemSettingRepoForSso() *memSettingRepoForSso {
	return &memSettingRepoForSso{values: map[string]string{}}
}

func (r *memSettingRepoForSso) Get(_ context.Context, key string) (*Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: v}, nil
}

func (r *memSettingRepoForSso) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return v, nil
}

func (r *memSettingRepoForSso) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}

func (r *memSettingRepoForSso) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.values[key]; !ok {
		return ErrSettingNotFound
	}
	delete(r.values, key)
	return nil
}

func (r *memSettingRepoForSso) GetAll(_ context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(r.values))
	for k, v := range r.values {
		out[k] = v
	}
	return out, nil
}

func (r *memSettingRepoForSso) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := r.values[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}

func (r *memSettingRepoForSso) SetMultiple(_ context.Context, settings map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range settings {
		r.values[k] = v
	}
	return nil
}

var _ SettingRepository = (*memSettingRepoForSso)(nil)

// ─── Issue ───────────────────────────────────────────────────────────────────

func TestSsoSession_Issue_PersistsRowAndSetsCookie(t *testing.T) {
	client := newOidcSigningTestClient(t)
	repo := newMemSettingRepoForSso()
	svc := NewSsoSessionService(client, repo)

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	svc.now = fixedNowSsoSession(now)

	// 必须先建一个 user 以满足 FK
	_, err := client.User.Create().
		SetEmail("alice@example.com").
		SetUsername("alice").
		SetPasswordHash("x").
		SetRole("user").
		Save(context.Background())
	require.NoError(t, err)
	userID := int64(1) // 第一行；ent 自增主键从 1 开始

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("User-Agent", "TestAgent/1.0")
	r.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")

	sid, err := svc.Issue(context.Background(), w, r, userID)
	require.NoError(t, err)
	require.NotEmpty(t, sid)
	require.GreaterOrEqual(t, len(sid), 40, "session id should be base64url of 32B")

	// cookie 属性
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	c := cookies[0]
	require.Equal(t, SsoCookieName, c.Name)
	require.Equal(t, sid, c.Value)
	require.Equal(t, "/", c.Path)
	require.True(t, c.HttpOnly)
	require.True(t, c.Secure)
	require.Equal(t, http.SameSiteLaxMode, c.SameSite)
	require.Equal(t, DefaultOidcProviderSSOCookieMaxAgeSeconds, c.MaxAge)

	// DB 行
	info, err := svc.ResolveSessionID(context.Background(), sid)
	require.NoError(t, err)
	require.Equal(t, userID, info.UserID)
	require.Equal(t, "TestAgent/1.0", info.UserAgent)
	require.Equal(t, "203.0.113.5", info.IPAddress)
	require.Equal(t, now, info.IssuedAt)
	require.Equal(t, now, info.LastSeenAt)
	require.Equal(t, now.Add(time.Duration(DefaultOidcProviderSSOCookieMaxAgeSeconds)*time.Second), info.ExpiresAt)
}

func TestSsoSession_Issue_HonorsCookieDomainAndMaxAgeFromSetting(t *testing.T) {
	client := newOidcSigningTestClient(t)
	repo := newMemSettingRepoForSso()
	require.NoError(t, repo.Set(context.Background(), SettingKeyOidcProviderSSOCookieDomain, ".sub2api.com"))
	require.NoError(t, repo.Set(context.Background(), SettingKeyOidcProviderSSOCookieMaxAgeSeconds, "1234"))

	svc := NewSsoSessionService(client, repo)

	_, err := client.User.Create().
		SetEmail("bob@example.com").
		SetUsername("bob").
		SetPasswordHash("x").
		SetRole("user").
		Save(context.Background())
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err = svc.Issue(context.Background(), w, r, 1)
	require.NoError(t, err)

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	// net/http 标准库会把 ".sub2api.com" 标准化为 "sub2api.com" (RFC 6265，前导点冗余)
	require.Equal(t, "sub2api.com", cookies[0].Domain)
	require.Equal(t, 1234, cookies[0].MaxAge)
}

func TestSsoSession_Issue_RejectsZeroUserID(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewSsoSessionService(client, newMemSettingRepoForSso())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := svc.Issue(context.Background(), w, r, 0)
	require.Error(t, err)
}

// ─── Resolve ─────────────────────────────────────────────────────────────────

func TestSsoSession_Resolve_NoCookieReturnsNotFound(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewSsoSessionService(client, newMemSettingRepoForSso())

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := svc.Resolve(context.Background(), r)
	require.True(t, errors.Is(err, ErrSsoSessionNotFound))
}

func TestSsoSession_Resolve_UnknownSessionReturnsNotFound(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewSsoSessionService(client, newMemSettingRepoForSso())

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: SsoCookieName, Value: "doesnotexist"})
	_, err := svc.Resolve(context.Background(), r)
	require.True(t, errors.Is(err, ErrSsoSessionNotFound))
}

func TestSsoSession_Resolve_RevokedReturnsRevoked(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewSsoSessionService(client, newMemSettingRepoForSso())

	_, err := client.User.Create().
		SetEmail("c@x.com").SetUsername("c").SetPasswordHash("x").SetRole("user").
		Save(context.Background())
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	sid, err := svc.Issue(context.Background(), w, r, 1)
	require.NoError(t, err)

	require.NoError(t, svc.Revoke(context.Background(), nil, sid))

	_, err = svc.ResolveSessionID(context.Background(), sid)
	require.True(t, errors.Is(err, ErrSsoSessionRevoked))
}

func TestSsoSession_Resolve_ExpiredReturnsExpired(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewSsoSessionService(client, newMemSettingRepoForSso())

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	svc.now = fixedNowSsoSession(now)

	_, err := client.User.Create().
		SetEmail("d@x.com").SetUsername("d").SetPasswordHash("x").SetRole("user").
		Save(context.Background())
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	sid, err := svc.Issue(context.Background(), w, r, 1)
	require.NoError(t, err)

	// 把时钟推过 max-age
	svc.now = fixedNowSsoSession(now.Add(time.Duration(DefaultOidcProviderSSOCookieMaxAgeSeconds+10) * time.Second))
	_, err = svc.ResolveSessionID(context.Background(), sid)
	require.True(t, errors.Is(err, ErrSsoSessionExpired))
}

// ─── Revoke ──────────────────────────────────────────────────────────────────

func TestSsoSession_Revoke_EmitsEmptyCookieAndMarksRow(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewSsoSessionService(client, newMemSettingRepoForSso())

	_, err := client.User.Create().
		SetEmail("e@x.com").SetUsername("e").SetPasswordHash("x").SetRole("user").
		Save(context.Background())
	require.NoError(t, err)

	wIssue := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	sid, err := svc.Issue(context.Background(), wIssue, r, 1)
	require.NoError(t, err)

	wRevoke := httptest.NewRecorder()
	require.NoError(t, svc.Revoke(context.Background(), wRevoke, sid))

	cookies := wRevoke.Result().Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, SsoCookieName, cookies[0].Name)
	require.Empty(t, cookies[0].Value)
	require.Equal(t, -1, cookies[0].MaxAge)

	// 二次 Revoke 幂等成功
	require.NoError(t, svc.Revoke(context.Background(), nil, sid))
}

func TestSsoSession_Revoke_UnknownSessionReturnsNotFound(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewSsoSessionService(client, newMemSettingRepoForSso())

	w := httptest.NewRecorder()
	err := svc.Revoke(context.Background(), w, "nope")
	require.True(t, errors.Is(err, ErrSsoSessionNotFound))
	// 即使 not found 也写出空 cookie
	require.Len(t, w.Result().Cookies(), 1)
}

// ─── RevokeAllForUser ────────────────────────────────────────────────────────

func TestSsoSession_RevokeAllForUser_RevokesActiveSessionsOnly(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewSsoSessionService(client, newMemSettingRepoForSso())

	_, err := client.User.Create().
		SetEmail("f@x.com").SetUsername("f").SetPasswordHash("x").SetRole("user").
		Save(context.Background())
	require.NoError(t, err)

	wA := httptest.NewRecorder()
	wB := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	sidA, err := svc.Issue(context.Background(), wA, r, 1)
	require.NoError(t, err)
	sidB, err := svc.Issue(context.Background(), wB, r, 1)
	require.NoError(t, err)

	// 提前手动吊销 A
	require.NoError(t, svc.Revoke(context.Background(), nil, sidA))

	n, err := svc.RevokeAllForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, 1, n, "only the still-active session B should be revoked")

	_, err = svc.ResolveSessionID(context.Background(), sidB)
	require.True(t, errors.Is(err, ErrSsoSessionRevoked))
}

// ─── TouchLastSeen 限流 ──────────────────────────────────────────────────────

func TestSsoSession_TouchLastSeen_RateLimited(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewSsoSessionService(client, newMemSettingRepoForSso())
	svc.touchInterval = 1 * time.Hour // 测试期间禁止真正二次写入

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	svc.now = fixedNowSsoSession(now)

	require.True(t, svc.TouchLastSeen("sid-1"), "first call should dispatch")
	require.False(t, svc.TouchLastSeen("sid-1"), "second call within window should be skipped")

	// 跨过窗口
	svc.now = fixedNowSsoSession(now.Add(2 * time.Hour))
	require.True(t, svc.TouchLastSeen("sid-1"), "after window expires should dispatch again")
}

func TestSsoSession_TouchLastSeenSync_UpdatesRow(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewSsoSessionService(client, newMemSettingRepoForSso())

	_, err := client.User.Create().
		SetEmail("g@x.com").SetUsername("g").SetPasswordHash("x").SetRole("user").
		Save(context.Background())
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	sid, err := svc.Issue(context.Background(), w, r, 1)
	require.NoError(t, err)

	t1 := time.Date(2026, 6, 1, 1, 0, 0, 0, time.UTC)
	svc.now = fixedNowSsoSession(t1)

	hit, err := svc.TouchLastSeenSync(context.Background(), sid)
	require.NoError(t, err)
	require.True(t, hit)

	info, err := svc.ResolveSessionID(context.Background(), sid)
	require.NoError(t, err)
	require.Equal(t, t1, info.LastSeenAt)
}

func TestSsoSession_TouchLastSeenSync_UnknownSessionReturnsFalse(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewSsoSessionService(client, newMemSettingRepoForSso())

	hit, err := svc.TouchLastSeenSync(context.Background(), "nope")
	require.NoError(t, err)
	require.False(t, hit)
}

// ─── extractUserAgentAndIP 边界 ──────────────────────────────────────────────

func TestExtractUserAgentAndIP_PrefersXFFFirstSegment(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("User-Agent", "ua/1")
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.1, 10.0.0.2")
	ua, ip := extractUserAgentAndIP(r)
	require.Equal(t, "ua/1", ua)
	require.Equal(t, "1.2.3.4", ip)
}

func TestExtractUserAgentAndIP_FallsBackToRemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.7:54321"
	_, ip := extractUserAgentAndIP(r)
	require.Equal(t, "203.0.113.7", ip)
}

func TestExtractUserAgentAndIP_NilRequestSafe(t *testing.T) {
	ua, ip := extractUserAgentAndIP(nil)
	require.Empty(t, ua)
	require.Empty(t, ip)
}

// 防止 imports 中 strings 包无引用警告 — 在某些重构中被去除时及时报错
func TestSsoSession_PackageStringsImported(t *testing.T) {
	require.True(t, strings.HasPrefix(SsoCookieName, "sub2api"))
}
