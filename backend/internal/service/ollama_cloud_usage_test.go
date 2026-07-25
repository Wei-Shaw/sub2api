package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type ollamaUsageTestEncryptor struct{}

func (ollamaUsageTestEncryptor) Encrypt(value string) (string, error) { return "cipher:" + value, nil }
func (ollamaUsageTestEncryptor) Decrypt(value string) (string, error) {
	if !strings.HasPrefix(value, "cipher:") {
		return "", errors.New("authentication failed")
	}
	return strings.TrimPrefix(value, "cipher:"), nil
}

type ollamaUsageTestRepo struct {
	*upstreamBillingProbeAccountRepo
	due                 []Account
	beforeSnapshot      func()
	disableAutoAttempts atomic.Int64
	disableAutoCalls    atomic.Int64
	groupResolveCalls   atomic.Int64
}

func (r *ollamaUsageTestRepo) ListOllamaCloudUsageGroupAccounts(_ context.Context, anchors []*Account) ([]Account, error) {
	r.groupResolveCalls.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	wanted := make(map[string]struct{}, len(anchors))
	for _, anchor := range anchors {
		if fingerprint, ok := ollamaCloudUsageGroupFingerprint(anchor); ok {
			wanted[fingerprint] = struct{}{}
		}
	}
	result := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		fingerprint, ok := ollamaCloudUsageGroupFingerprint(account)
		if _, match := wanted[fingerprint]; !ok || !match {
			continue
		}
		result = append(result, cloneOllamaUsageTestAccount(*account))
	}
	return result, nil
}

// cloneOllamaUsageTestAccount 深拷贝共享 map，模拟真实仓储每次查询返回全新行：
// 组写在 r.mu 下改成员 map，浅拷贝会让 RunDue 过滤循环无锁读到同一 map 而竞争。
func cloneOllamaUsageTestAccount(account Account) Account {
	account.Credentials = mergeMap(nil, account.Credentials)
	account.Extra = mergeMap(nil, account.Extra)
	return account
}

func (r *ollamaUsageTestRepo) SaveOllamaCloudUsageSession(_ context.Context, expected *Account, ciphertext string, autoRefresh bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	members, err := r.ollamaGroupMembersLocked(expected)
	if err != nil {
		return err
	}
	for _, account := range members {
		account.Extra[OllamaCloudUsageSessionExtraKey] = ciphertext
		account.Extra[OllamaCloudUsageAutoRefreshExtraKey] = autoRefresh
		delete(account.Extra, OllamaCloudUsageSnapshotExtraKey)
	}
	return nil
}

func (r *ollamaUsageTestRepo) DeleteOllamaCloudUsageSession(_ context.Context, expected *Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	members, err := r.ollamaGroupMembersLocked(expected)
	if err != nil {
		return err
	}
	for _, account := range members {
		delete(account.Extra, OllamaCloudUsageSessionExtraKey)
		delete(account.Extra, OllamaCloudUsageAutoRefreshExtraKey)
		delete(account.Extra, OllamaCloudUsageSnapshotExtraKey)
	}
	return nil
}

func (r *ollamaUsageTestRepo) SetOllamaCloudUsageAutoRefresh(_ context.Context, expected *Account, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	members, err := r.ollamaGroupMembersLocked(expected)
	if err != nil || !r.ollamaExpectedSessionExistsLocked(members, expected) {
		return ErrOllamaCloudUsageIdentityChanged
	}
	for _, account := range members {
		applyOllamaUsageTestManagedExtra(account, expected)
		account.Extra[OllamaCloudUsageAutoRefreshExtraKey] = enabled
	}
	return nil
}

func (r *ollamaUsageTestRepo) UpdateOllamaCloudUsageSnapshot(_ context.Context, expected *Account, snapshot *OllamaCloudUsageSnapshot) error {
	if r.beforeSnapshot != nil {
		r.beforeSnapshot()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	members, err := r.ollamaGroupMembersLocked(expected)
	if err != nil || !r.ollamaExpectedSessionExistsLocked(members, expected) {
		return ErrOllamaCloudUsageIdentityChanged
	}
	for _, account := range members {
		applyOllamaUsageTestManagedExtra(account, expected)
		account.Extra[OllamaCloudUsageSnapshotExtraKey] = snapshot
	}
	return nil
}

func (r *ollamaUsageTestRepo) DisableOllamaCloudUsageAutoRefresh(_ context.Context, expected *Account) error {
	r.disableAutoAttempts.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	members, err := r.ollamaGroupMembersLocked(expected)
	if err != nil || !r.ollamaExpectedSessionExistsLocked(members, expected) {
		return ErrOllamaCloudUsageIdentityChanged
	}
	for _, account := range members {
		applyOllamaUsageTestManagedExtra(account, expected)
		account.Extra[OllamaCloudUsageAutoRefreshExtraKey] = false
		delete(account.Extra, OllamaCloudUsageSnapshotExtraKey)
	}
	r.disableAutoCalls.Add(1)
	return nil
}

func (r *ollamaUsageTestRepo) ollamaGroupMembersLocked(expected *Account) ([]*Account, error) {
	anchor := r.accounts[expected.ID]
	if !sameOllamaUsageTestIdentity(anchor, expected) {
		return nil, ErrOllamaCloudUsageIdentityChanged
	}
	fingerprint, ok := ollamaCloudUsageGroupFingerprint(expected)
	if !ok {
		return nil, ErrOllamaCloudUsageAccountInvalid
	}
	members := make([]*Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		candidate, valid := ollamaCloudUsageGroupFingerprint(account)
		if valid && candidate == fingerprint {
			if account.Extra == nil {
				account.Extra = make(map[string]any)
			}
			members = append(members, account)
		}
	}
	return members, nil
}

func (r *ollamaUsageTestRepo) ollamaExpectedSessionExistsLocked(members []*Account, expected *Account) bool {
	for _, member := range members {
		if member.Extra[OllamaCloudUsageSessionExtraKey] == expected.Extra[OllamaCloudUsageSessionExtraKey] {
			return true
		}
	}
	return false
}

func applyOllamaUsageTestManagedExtra(account, source *Account) {
	for _, key := range []string{OllamaCloudUsageSessionExtraKey, OllamaCloudUsageAutoRefreshExtraKey, OllamaCloudUsageSnapshotExtraKey} {
		delete(account.Extra, key)
		if value, ok := source.Extra[key]; ok {
			account.Extra[key] = value
		}
	}
}

func (r *ollamaUsageTestRepo) ListDueOllamaCloudUsageAccounts(_ context.Context, _ time.Time, limit int) ([]Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.due) > 0 {
		out := make([]Account, 0, min(limit, len(r.due)))
		for _, account := range r.due[:min(limit, len(r.due))] {
			out = append(out, cloneOllamaUsageTestAccount(account))
		}
		return out, nil
	}
	out := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		out = append(out, cloneOllamaUsageTestAccount(*account))
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

type ollamaRefreshPreflightIdentityChangeRepo struct {
	*ollamaUsageTestRepo
	getCalls atomic.Int64
}

func (r *ollamaRefreshPreflightIdentityChangeRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	if r.getCalls.Add(1) == 2 {
		r.mu.Lock()
		r.accounts[id].Credentials["api_key"] = "rotated-before-refresh"
		r.mu.Unlock()
	}
	return r.upstreamBillingProbeAccountRepo.GetByID(ctx, id)
}

type ollamaManagedExtraUpdateRepo struct {
	AccountRepository
	account *Account
	updated *Account
}

func (r *ollamaManagedExtraUpdateRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	return r.account, nil
}

func (r *ollamaManagedExtraUpdateRepo) Update(_ context.Context, account *Account) error {
	r.updated = account
	return nil
}

func sameOllamaUsageTestIdentity(left, right *Account) bool {
	return left != nil && right != nil && left.Platform == right.Platform && left.Type == right.Type &&
		reflect.DeepEqual(left.Credentials, right.Credentials) && reflect.DeepEqual(left.ProxyID, right.ProxyID)
}

type ollamaUsageHTTPStub struct {
	status         int
	body           []byte
	header         http.Header
	calls          atomic.Int64
	active         atomic.Int64
	maxActive      atomic.Int64
	beforeResponse func(*http.Request)
	lastRequest    *http.Request
	lastProxyURL   string
	mu             sync.Mutex
}

func (s *ollamaUsageHTTPStub) Do(req *http.Request, proxyURL string, _ int64, _ int) (*http.Response, error) {
	s.calls.Add(1)
	active := s.active.Add(1)
	defer s.active.Add(-1)
	for {
		peak := s.maxActive.Load()
		if active <= peak || s.maxActive.CompareAndSwap(peak, active) {
			break
		}
	}
	s.mu.Lock()
	s.lastRequest = req
	s.lastProxyURL = proxyURL
	s.mu.Unlock()
	if s.beforeResponse != nil {
		s.beforeResponse(req)
	}
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	header := s.header
	if header == nil {
		header = http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(string(s.body))), Request: req}, nil
}

func (s *ollamaUsageHTTPStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, concurrency)
}

func ollamaUsageAccount(id int64) *Account {
	return &Account{
		ID: id, Name: fmt.Sprintf("ollama-%d", id), Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://ollama.com", "api_key": fmt.Sprintf("key-%d", id)},
		Extra:       map[string]any{}, Status: StatusActive, Schedulable: true, Concurrency: 1,
	}
}

func newOllamaUsageTestService(t *testing.T, repo *ollamaUsageTestRepo, upstream HTTPUpstream, settingsRepo SettingRepository, fixedKey bool) *OllamaCloudUsageService {
	t.Helper()
	svc := NewOllamaCloudUsageService(repo, upstream, NewSettingService(settingsRepo, nil), ollamaUsageTestEncryptor{}, fixedKey)
	t.Cleanup(svc.Stop)
	return svc
}

func ollamaUsageFixture(t *testing.T) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/ollama_settings_usage.html")
	require.NoError(t, err)
	return body
}

func TestOllamaCloudUsageSettingsDefaultOffAndValidation(t *testing.T) {
	repo := &upstreamBillingProbeSettingRepo{}
	settingsService := NewSettingService(repo, nil)
	settings, err := settingsService.GetOllamaCloudUsageSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.Enabled)
	require.Equal(t, 60, settings.IntervalMinutes)

	err = settingsService.SetOllamaCloudUsageSettings(context.Background(), &OllamaCloudUsageSettings{Enabled: true, IntervalMinutes: 14})
	require.Error(t, err)
	err = settingsService.SetOllamaCloudUsageSettings(context.Background(), &OllamaCloudUsageSettings{Enabled: true, IntervalMinutes: 90})
	require.NoError(t, err)
	settings, err = settingsService.GetOllamaCloudUsageSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 90, settings.IntervalMinutes)
}

func TestIsOllamaCloudUsageAccountStrictOfficialHost(t *testing.T) {
	tests := []struct {
		baseURL  string
		platform string
		want     bool
	}{
		{"https://ollama.com", PlatformOpenAI, true},
		{"HTTPS://OLLAMA.COM", PlatformAnthropic, true},
		{"https://www.OLLAMA.com:443/v1", PlatformOpenAI, true},
		{"https://ollama.com:443", PlatformOpenAI, true},
		{"https://ollama.com/", PlatformAnthropic, false},
		{"https://ollama.com/v1/", PlatformOpenAI, false},
		{"http://ollama.com", PlatformOpenAI, false},
		{"https://ollama.com.evil.test", PlatformOpenAI, false},
		{"https://ollama.com:444", PlatformOpenAI, false},
		{"https://user@ollama.com", PlatformOpenAI, false},
		{"https://ollama.com/v2", PlatformOpenAI, false},
		{"https://ollama.com?next=https://evil.test", PlatformOpenAI, false},
		{"https://ollama.com#usage", PlatformOpenAI, false},
	}
	for _, test := range tests {
		t.Run(test.baseURL+test.platform, func(t *testing.T) {
			account := ollamaUsageAccount(1)
			account.Platform = test.platform
			account.Credentials["base_url"] = test.baseURL
			require.Equal(t, test.want, IsOllamaCloudUsageAccount(account))
		})
	}
}

func TestNormalizeOllamaCloudUsageCookieAllowlist(t *testing.T) {
	normalized, err := normalizeOllamaCloudUsageCookie(" tracking=discard ; wos-session=secret ; __Secure-authjs.session-token.0=part-a ; device=discard ")
	require.NoError(t, err)
	require.Equal(t, "wos-session=secret; __Secure-authjs.session-token.0=part-a", normalized)

	normalized, err = normalizeOllamaCloudUsageCookie(" \t\r\nwos-session=secret; tracking=discard\r\n\t ")
	require.NoError(t, err)
	require.Equal(t, "wos-session=secret", normalized)

	_, err = normalizeOllamaCloudUsageCookie("wos-session=secret\r\nHost: evil.test")
	require.ErrorContains(t, err, "invalid header")

	for _, allowed := range []string{
		"wos-session", "__Secure-session", "session", "ollama_session", "__Host-ollama_session",
		"next-auth.session-token", "next-auth.session-token.0", "__Secure-next-auth.session-token.12",
		"authjs.session-token", "__Secure-authjs.session-token.1",
	} {
		normalized, err := normalizeOllamaCloudUsageCookie(allowed + "=value")
		require.NoError(t, err, allowed)
		require.Equal(t, allowed+"=value", normalized)
	}

	for _, invalid := range []string{
		"", "Domain=ollama.com; wos-session=x", "wos-session=x; Path=/",
		"wos-session=x; wos-session=y", "Secure", "tracking=only", "__session=arbitrary",
		"authjs.session-token.bad=not-a-shard", "Authjs.session-token=wrong-case",
	} {
		_, err := normalizeOllamaCloudUsageCookie(invalid)
		require.Error(t, err, invalid)
	}
	_, err = normalizeOllamaCloudUsageCookie("wos-session=" + strings.Repeat("x", ollamaCloudUsageMaxSessionBytes))
	require.ErrorContains(t, err, "too large")
}

func TestParseOllamaCloudUsageHTMLFixture(t *testing.T) {
	data, err := parseOllamaCloudUsageHTML(ollamaUsageFixture(t))
	require.NoError(t, err)
	require.Equal(t, "max", data.Plan)
	require.NotNil(t, data.FiveHour)
	require.Equal(t, 5.6, data.FiveHour.UsedPercent)
	require.NotNil(t, data.FiveHour.ResetAt)
	require.Equal(t, time.Date(2026, time.July, 23, 3, 0, 0, 0, time.UTC), *data.FiveHour.ResetAt)
	require.NotNil(t, data.SevenDay)
	require.Equal(t, 14.2, data.SevenDay.UsedPercent)
	require.NotNil(t, data.SevenDay.ResetAt)
	require.Equal(t, time.Date(2026, time.July, 29, 0, 0, 0, 0, time.UTC), *data.SevenDay.ResetAt)
	require.Equal(t, "$0", data.Balance)
	require.Equal(t, []OllamaCloudUsageModel{
		{Model: "gpt-oss:120b-cloud", Window: OllamaCloudUsageModelWindowFiveHour, Requests: 2},
		{Model: "qwen3-coder:480b-cloud", Window: OllamaCloudUsageModelWindowFiveHour, Requests: 3},
		{Model: "gpt-oss:120b-cloud", Window: OllamaCloudUsageModelWindowSevenDay, Requests: 12},
		{Model: "qwen3-coder:480b-cloud", Window: OllamaCloudUsageModelWindowSevenDay, Requests: 13},
	}, data.Models)

	_, err = parseOllamaCloudUsageHTML([]byte(`<html><body><main>Sign in to Ollama</main></body></html>`))
	require.ErrorIs(t, err, errOllamaCloudUsageUnauthorizedHTML)
	_, err = parseOllamaCloudUsageHTML([]byte(`<html><body><p>5 hour usage 42% used</p><form>Sign in to Ollama</form></body></html>`))
	require.ErrorIs(t, err, errOllamaCloudUsageUnauthorizedHTML)
	_, err = parseOllamaCloudUsageHTML([]byte(`<html><body><main>unrelated settings</main></body></html>`))
	require.Error(t, err)
}

func TestParseOllamaCloudUsageHTMLMissingOptionalFieldsAndCSSWidthFallback(t *testing.T) {
	data, err := parseOllamaCloudUsageHTML([]byte(`
		<section>
			<p>5 hour usage</p>
			<div data-usage-track>
				<div data-usage-segment style="width: 23.5%"><span data-model="model-a" data-requests="1,234"></span></div>
				<div data-usage-segment data-model="model-a" data-requests="9,999" style="width: 0%"></div>
			</div>
		</section>`))
	require.NoError(t, err)
	require.Equal(t, 23.5, data.FiveHour.UsedPercent)
	require.Nil(t, data.FiveHour.ResetAt)
	require.Empty(t, data.Plan)
	require.Nil(t, data.SevenDay)
	require.Empty(t, data.Balance)
	require.Equal(t, []OllamaCloudUsageModel{{
		Model: "model-a", Window: OllamaCloudUsageModelWindowFiveHour, Requests: 1234,
	}}, data.Models)
}

func TestParseOllamaCloudUsageHTMLResetElementVariants(t *testing.T) {
	const want = "2026-07-23T03:00:00Z"
	for name, element := range map[string]string{
		"time datetime":  `<time datetime="` + want + `">2 hours.</time>`,
		"custom element": `<local-time data-time="` + want + `">2 hours.</local-time>`,
		"class token":    `<span class="text-xs local-time tabular-nums" data-time="` + want + `">2 hours.</span>`,
	} {
		t.Run(name, func(t *testing.T) {
			data, err := parseOllamaCloudUsageHTML([]byte(
				`<div><div><span>Session usage</span><span>1% used</span></div><div>Resets in ` + element + `</div></div>`,
			))
			require.NoError(t, err)
			require.NotNil(t, data.FiveHour)
			require.NotNil(t, data.FiveHour.ResetAt)
			require.Equal(t, want, data.FiveHour.ResetAt.Format(time.RFC3339))
		})
	}
}

func TestParseOllamaCloudUsageHTMLPlanAndBalanceFallbacks(t *testing.T) {
	data, err := parseOllamaCloudUsageHTML([]byte(`
		<section>
			<h2><span>Cloud usage</span><span>max</span></h2>
			<div><span>Plan</span><span>Pro</span></div>
			<p>Credits currently available: USD $9.50</p>
		</section>`))
	require.NoError(t, err)
	require.Equal(t, "max", data.Plan)
	require.Equal(t, "USD$9.50", data.Balance)

	data, err = parseOllamaCloudUsageHTML([]byte(`<div><span>Subscription</span><span>Pro</span></div>`))
	require.NoError(t, err)
	require.Equal(t, "Pro", data.Plan)
}

func TestOllamaCloudUsageManagedExtraCannotBeImported(t *testing.T) {
	remoteExtra := map[string]any{
		OllamaCloudUsageSessionExtraKey:     "remote-ciphertext",
		OllamaCloudUsageAutoRefreshExtraKey: true,
		OllamaCloudUsageSnapshotExtraKey:    map[string]any{"status": "forged"},
	}
	created, err := buildAccountForCreate(&CreateAccountInput{
		Name: "ollama", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://ollama.com", "api_key": "key"},
		Concurrency: 1,
	}, mergeMap(nil, remoteExtra))
	require.NoError(t, err)
	require.NotContains(t, created.Extra, OllamaCloudUsageSessionExtraKey)
	require.NotContains(t, created.Extra, OllamaCloudUsageAutoRefreshExtraKey)
	require.NotContains(t, created.Extra, OllamaCloudUsageSnapshotExtraKey)

	existing := ollamaUsageAccount(6)
	existing.Extra = map[string]any{
		OllamaCloudUsageSessionExtraKey:     "local-ciphertext",
		OllamaCloudUsageAutoRefreshExtraKey: false,
		OllamaCloudUsageSnapshotExtraKey:    map[string]any{"status": OllamaCloudUsageStatusOK},
	}
	targetExtra := mergeMap(existing.Extra, remoteExtra)
	reconcileCRSUpstreamBillingProbeExtra(existing, existing.Platform, existing.Type, mergeMap(existing.Credentials, nil), targetExtra)
	require.Equal(t, "local-ciphertext", targetExtra[OllamaCloudUsageSessionExtraKey])
	require.Equal(t, false, targetExtra[OllamaCloudUsageAutoRefreshExtraKey])
	require.Equal(t, map[string]any{"status": OllamaCloudUsageStatusOK}, targetExtra[OllamaCloudUsageSnapshotExtraKey])

	changedCredentials := mergeMap(existing.Credentials, map[string]any{"api_key": "rotated"})
	targetExtra = mergeMap(existing.Extra, remoteExtra)
	reconcileCRSUpstreamBillingProbeExtra(existing, existing.Platform, existing.Type, changedCredentials, targetExtra)
	require.NotContains(t, targetExtra, OllamaCloudUsageSessionExtraKey)
	require.NotContains(t, targetExtra, OllamaCloudUsageAutoRefreshExtraKey)
	require.NotContains(t, targetExtra, OllamaCloudUsageSnapshotExtraKey)
}

func TestAccountServiceUpdateStripsOllamaManagedExtra(t *testing.T) {
	account := ollamaUsageAccount(61)
	account.Extra = map[string]any{
		OllamaCloudUsageSessionExtraKey:     "local-ciphertext",
		OllamaCloudUsageAutoRefreshExtraKey: true,
		OllamaCloudUsageSnapshotExtraKey:    map[string]any{"status": OllamaCloudUsageStatusOK},
	}
	repo := &ollamaManagedExtraUpdateRepo{account: account}
	svc := NewAccountService(repo, nil)
	requestedExtra := map[string]any{
		"note":                              "preserved",
		OllamaCloudUsageSessionExtraKey:     "forged-ciphertext",
		OllamaCloudUsageAutoRefreshExtraKey: nil,
		OllamaCloudUsageSnapshotExtraKey:    nil,
	}

	_, err := svc.Update(context.Background(), account.ID, UpdateAccountRequest{Extra: &requestedExtra})
	require.NoError(t, err)
	require.Equal(t, "preserved", repo.updated.Extra["note"])
	require.NotContains(t, repo.updated.Extra, OllamaCloudUsageSessionExtraKey)
	require.NotContains(t, repo.updated.Extra, OllamaCloudUsageAutoRefreshExtraKey)
	require.NotContains(t, repo.updated.Extra, OllamaCloudUsageSnapshotExtraKey)
	// The request map is not mutated while managed fields are stripped.
	require.Contains(t, requestedExtra, OllamaCloudUsageSessionExtraKey)
}

func TestOllamaCloudUsageSessionEncryptionFailClosedAndWriteOnlyState(t *testing.T) {
	account := ollamaUsageAccount(7)
	repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{7: account}}}
	settings := &upstreamBillingProbeSettingRepo{}

	ephemeral := newOllamaUsageTestService(t, repo, &ollamaUsageHTTPStub{}, settings, false)
	_, err := ephemeral.SaveSession(context.Background(), 7, "wos-session=plaintext-secret")
	require.ErrorIs(t, err, ErrOllamaCloudUsageEncryptionKey)
	require.NotContains(t, account.Extra, OllamaCloudUsageSessionExtraKey)

	svc := newOllamaUsageTestService(t, repo, &ollamaUsageHTTPStub{}, settings, true)
	_, err = svc.SaveSession(context.Background(), 7, "tracking=arbitrary-only")
	require.Error(t, err)
	require.NotContains(t, account.Extra, OllamaCloudUsageSessionExtraKey)

	state, err := svc.SaveSession(context.Background(), 7, "tracking=must-not-persist; wos-session=plaintext-secret")
	require.NoError(t, err)
	require.True(t, state.Configured)
	stored, ok := account.Extra[OllamaCloudUsageSessionExtraKey].(string)
	require.True(t, ok)
	require.Equal(t, "cipher:wos-session=plaintext-secret", stored)
	require.NotContains(t, stored, "tracking")
	raw, err := json.Marshal(state)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "plaintext-secret")
	require.NotContains(t, string(raw), "cipher:")

	account.Extra[OllamaCloudUsageSessionExtraKey] = "plaintext-secret"
	_, err = svc.Refresh(context.Background(), 7)
	require.ErrorContains(t, err, "cannot be decrypted")
}

func TestOllamaCloudUsageGroupSharesAcrossPlatformsURLVariantsAndDynamicSiblings(t *testing.T) {
	source := ollamaUsageAccount(71)
	source.Credentials["api_key"] = "shared-key"
	source.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=shared"
	source.Extra[OllamaCloudUsageAutoRefreshExtraKey] = true
	source.Extra[OllamaCloudUsageSnapshotExtraKey] = &OllamaCloudUsageSnapshot{
		Status: OllamaCloudUsageStatusOK,
		Data:   &OllamaCloudUsageData{Plan: "pro"},
	}
	source.UpdatedAt = time.Now().Add(-time.Minute)
	sibling := ollamaUsageAccount(72)
	sibling.Platform = PlatformAnthropic
	sibling.Credentials = map[string]any{"base_url": "HTTPS://WWW.OLLAMA.COM:443/v1", "api_key": "shared-key"}
	sibling.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=shared"
	sibling.Extra[OllamaCloudUsageAutoRefreshExtraKey] = true
	sibling.UpdatedAt = time.Now()
	different := ollamaUsageAccount(73)
	different.Credentials["api_key"] = "different-key"
	repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		source.ID: source, sibling.ID: sibling, different.ID: different,
	}}}
	svc := newOllamaUsageTestService(t, repo, &ollamaUsageHTTPStub{}, &upstreamBillingProbeSettingRepo{}, true)

	state, err := svc.GetState(context.Background(), sibling.ID)
	require.NoError(t, err)
	require.True(t, state.Configured)
	require.True(t, state.AutoRefreshEnabled)
	require.Equal(t, "pro", state.Snapshot.Data.Plan)

	differentState, err := svc.GetState(context.Background(), different.ID)
	require.NoError(t, err)
	require.False(t, differentState.Configured)

	newSibling := ollamaUsageAccount(74)
	newSibling.Platform = PlatformAnthropic
	newSibling.Credentials = map[string]any{"base_url": "https://ollama.com:443", "api_key": "shared-key"}
	repo.mu.Lock()
	repo.accounts[newSibling.ID] = newSibling
	repo.mu.Unlock()
	newState, err := svc.GetState(context.Background(), newSibling.ID)
	require.NoError(t, err)
	require.True(t, newState.Configured)
	require.Equal(t, state.Snapshot, newState.Snapshot)

	before := repo.groupResolveCalls.Load()
	require.NoError(t, svc.ResolveAccounts(context.Background(), []*Account{source, sibling, different, newSibling}))
	require.Equal(t, before+1, repo.groupResolveCalls.Load(), "one list batch must issue one group lookup")
}

func TestOllamaCloudUsageSaveAutoRefreshAndDeleteAreGroupScoped(t *testing.T) {
	first := ollamaUsageAccount(81)
	first.Credentials["api_key"] = "shared-key"
	second := ollamaUsageAccount(82)
	second.Platform = PlatformAnthropic
	second.Credentials = map[string]any{"base_url": "https://www.ollama.com/v1", "api_key": "shared-key"}
	different := ollamaUsageAccount(83)
	different.Credentials["api_key"] = "different-key"
	repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		first.ID: first, second.ID: second, different.ID: different,
	}}}
	svc := newOllamaUsageTestService(t, repo, &ollamaUsageHTTPStub{}, &upstreamBillingProbeSettingRepo{}, true)

	state, err := svc.SaveSession(context.Background(), second.ID, "wos-session=shared-browser")
	require.NoError(t, err)
	require.True(t, state.Configured)
	require.Equal(t, "cipher:wos-session=shared-browser", first.Extra[OllamaCloudUsageSessionExtraKey])
	require.Equal(t, first.Extra[OllamaCloudUsageSessionExtraKey], second.Extra[OllamaCloudUsageSessionExtraKey])
	require.NotContains(t, different.Extra, OllamaCloudUsageSessionExtraKey)

	state, err = svc.SetAutoRefresh(context.Background(), first.ID, true)
	require.NoError(t, err)
	require.True(t, state.AutoRefreshEnabled)
	require.Equal(t, true, first.Extra[OllamaCloudUsageAutoRefreshExtraKey])
	require.Equal(t, true, second.Extra[OllamaCloudUsageAutoRefreshExtraKey])

	state, err = svc.DeleteSession(context.Background(), second.ID)
	require.NoError(t, err)
	require.False(t, state.Configured)
	for _, member := range []*Account{first, second} {
		require.NotContains(t, member.Extra, OllamaCloudUsageSessionExtraKey)
		require.NotContains(t, member.Extra, OllamaCloudUsageAutoRefreshExtraKey)
		require.NotContains(t, member.Extra, OllamaCloudUsageSnapshotExtraKey)
	}
}

func TestOllamaCloudUsageRefreshSingleflightAndRunnerDeduplicateSharedGroup(t *testing.T) {
	first := ollamaUsageAccount(91)
	first.Credentials["api_key"] = "shared-key"
	first.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=shared"
	first.Extra[OllamaCloudUsageAutoRefreshExtraKey] = true
	second := ollamaUsageAccount(92)
	second.Platform = PlatformAnthropic
	second.Credentials = map[string]any{"base_url": "https://www.ollama.com:443/v1", "api_key": "shared-key"}
	second.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=shared"
	second.Extra[OllamaCloudUsageAutoRefreshExtraKey] = true
	repo := &ollamaUsageTestRepo{
		upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{first.ID: first, second.ID: second}},
		due:                             []Account{*first, *second},
	}
	settingsRepo := &upstreamBillingProbeSettingRepo{values: map[string]string{
		SettingKeyOllamaCloudUsageSettings: `{"enabled":true,"interval_minutes":60}`,
	}}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	upstream := &ollamaUsageHTTPStub{body: ollamaUsageFixture(t), beforeResponse: func(*http.Request) {
		once.Do(func() { close(started) })
		<-release
	}}
	svc := newOllamaUsageTestService(t, repo, upstream, settingsRepo, true)

	errs := make(chan error, 2)
	go func() { _, err := svc.Refresh(context.Background(), first.ID); errs <- err }()
	<-started
	go func() { _, err := svc.Refresh(context.Background(), second.ID); errs <- err }()
	close(release)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	require.Equal(t, int64(1), upstream.calls.Load())
	require.NotNil(t, decodeOllamaCloudUsageSnapshot(first.Extra))
	require.Equal(t, decodeOllamaCloudUsageSnapshot(first.Extra), decodeOllamaCloudUsageSnapshot(second.Extra))

	delete(first.Extra, OllamaCloudUsageSnapshotExtraKey)
	delete(second.Extra, OllamaCloudUsageSnapshotExtraKey)
	upstream.beforeResponse = nil
	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, int64(2), upstream.calls.Load(), "RunDue must issue one request for the shared group")
}

func TestOllamaCloudUsageRefreshRejectsGroupChangeBeforeUpstreamRequest(t *testing.T) {
	account := ollamaUsageAccount(94)
	account.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=secret"
	base := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}}
	repo := &ollamaRefreshPreflightIdentityChangeRepo{ollamaUsageTestRepo: base}
	upstream := &ollamaUsageHTTPStub{body: ollamaUsageFixture(t)}
	svc := NewOllamaCloudUsageService(repo, upstream, NewSettingService(&upstreamBillingProbeSettingRepo{}, nil), ollamaUsageTestEncryptor{}, true)
	t.Cleanup(svc.Stop)

	_, err := svc.Refresh(context.Background(), account.ID)

	require.ErrorIs(t, err, ErrOllamaCloudUsageIdentityChanged)
	require.Zero(t, upstream.calls.Load())
	require.NotContains(t, account.Extra, OllamaCloudUsageSnapshotExtraKey)
}

func TestOllamaCloudUsageRefreshUsesFixedURLCookieAndNoRedirects(t *testing.T) {
	account := ollamaUsageAccount(8)
	account.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=browser-secret; tracking=must-not-send"
	repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{8: account}}}
	upstream := &ollamaUsageHTTPStub{body: ollamaUsageFixture(t)}
	svc := newOllamaUsageTestService(t, repo, upstream, &upstreamBillingProbeSettingRepo{}, true)
	fixedNow := time.Date(2026, time.July, 22, 15, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	state, err := svc.Refresh(context.Background(), 8)
	require.NoError(t, err)
	require.Equal(t, OllamaCloudUsageStatusOK, state.Snapshot.Status)
	require.Equal(t, "https://ollama.com/settings", upstream.lastRequest.URL.String())
	require.Equal(t, "ollama.com", upstream.lastRequest.Host)
	require.Equal(t, "wos-session=browser-secret", upstream.lastRequest.Header.Get("Cookie"))
	require.NotContains(t, upstream.lastRequest.Header.Get("Cookie"), "tracking")
	require.Empty(t, upstream.lastRequest.Header.Get("Authorization"))
	require.True(t, HTTPUpstreamRedirectsDisabled(upstream.lastRequest.Context()))
}

func TestOllamaCloudUsageManualRefreshUsesShortIndependentInterval(t *testing.T) {
	account := ollamaUsageAccount(12)
	account.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=initial"
	repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{12: account}}}
	upstream := &ollamaUsageHTTPStub{body: ollamaUsageFixture(t)}
	svc := newOllamaUsageTestService(t, repo, upstream, &upstreamBillingProbeSettingRepo{}, true)
	fixedNow := time.Date(2026, time.July, 22, 15, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	_, err := svc.Refresh(context.Background(), 12)
	require.NoError(t, err)
	_, err = svc.Refresh(context.Background(), 12)
	require.ErrorIs(t, err, ErrOllamaCloudUsageRefreshRateLimited)
	require.Equal(t, int64(1), upstream.calls.Load())

	// Saving a repaired session clears the prior snapshot, so the global 60-minute
	// next_refresh_at does not block immediate administrator verification.
	_, err = svc.SaveSession(context.Background(), 12, "wos-session=repaired")
	require.NoError(t, err)
	_, err = svc.Refresh(context.Background(), 12)
	require.NoError(t, err)
	require.Equal(t, int64(2), upstream.calls.Load())
}

func TestOllamaCloudUsageRefreshUsesHydratedProxyIdentity(t *testing.T) {
	account := ollamaUsageAccount(13)
	account.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=secret"
	proxyID := int64(4)
	account.ProxyID = &proxyID
	account.Proxy = &Proxy{
		ID: proxyID, Protocol: "http", Host: "127.0.0.1", Port: 3128,
		Username: "proxy-user", Password: "proxy-pass", Status: StatusActive,
	}
	repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{13: account}}}
	upstream := &ollamaUsageHTTPStub{body: ollamaUsageFixture(t)}
	svc := newOllamaUsageTestService(t, repo, upstream, &upstreamBillingProbeSettingRepo{}, true)

	_, err := svc.Refresh(context.Background(), 13)
	require.NoError(t, err)
	require.Equal(t, account.Proxy.URL(), upstream.lastProxyURL)
}

func TestOllamaCloudUsageRedirectAndBodyLimitArePersistedSafely(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   []byte
		reason string
	}{
		{"redirect", http.StatusFound, nil, "redirect_blocked"},
		{"body limit", http.StatusOK, make([]byte, ollamaCloudUsageMaxBodyBytes+1), "response_too_large"},
	} {
		t.Run(test.name, func(t *testing.T) {
			account := ollamaUsageAccount(9)
			account.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=secret"
			repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{9: account}}}
			svc := newOllamaUsageTestService(t, repo, &ollamaUsageHTTPStub{status: test.status, body: test.body}, &upstreamBillingProbeSettingRepo{}, true)
			state, err := svc.Refresh(context.Background(), 9)
			require.NoError(t, err)
			require.Equal(t, OllamaCloudUsageStatusFailed, state.Snapshot.Status)
			require.Equal(t, test.reason, state.Snapshot.LastError)
		})
	}
}

func TestOllamaCloudUsageRefreshRejectsIdentityChange(t *testing.T) {
	account := ollamaUsageAccount(10)
	account.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=secret"
	repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{10: account}}}
	repo.beforeSnapshot = func() { account.Credentials["api_key"] = "rotated" }
	svc := newOllamaUsageTestService(t, repo, &ollamaUsageHTTPStub{body: ollamaUsageFixture(t)}, &upstreamBillingProbeSettingRepo{}, true)
	_, err := svc.Refresh(context.Background(), 10)
	require.ErrorIs(t, err, ErrOllamaCloudUsageIdentityChanged)
	require.NotContains(t, account.Extra, OllamaCloudUsageSnapshotExtraKey)
}

func TestOllamaCloudUsageRunnerHonorsLeaderLockAndBackoff(t *testing.T) {
	account := ollamaUsageAccount(11)
	account.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=secret"
	account.Extra[OllamaCloudUsageAutoRefreshExtraKey] = true
	repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{11: account}}}
	upstream := &ollamaUsageHTTPStub{body: ollamaUsageFixture(t)}
	settingsRepo := &upstreamBillingProbeSettingRepo{values: map[string]string{
		SettingKeyOllamaCloudUsageSettings: `{"enabled":true,"interval_minutes":60}`,
	}}
	cache := &fakeLeaderLockCache{}
	_, acquired := tryAcquireSingletonLeaderLock(context.Background(), cache, nil, ollamaCloudUsageLeaderLockKey, "peer", time.Minute)
	require.True(t, acquired)
	svc := newOllamaUsageTestService(t, repo, upstream, settingsRepo, true)
	svc.lockCache = cache
	require.NoError(t, svc.RunDue(context.Background()))
	require.Zero(t, upstream.calls.Load())
	require.NoError(t, cache.ReleaseLeaderLock(context.Background(), ollamaCloudUsageLeaderLockKey, "peer"))
	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, int64(1), upstream.calls.Load())

	firstFailure := nextOllamaCloudUsageDelay(60, 1, 0)
	thirdFailure := nextOllamaCloudUsageDelay(60, 3, 0)
	require.Greater(t, thirdFailure, firstFailure)
	require.GreaterOrEqual(t, nextOllamaCloudUsageDelay(60, 1, 3*time.Hour), 3*time.Hour)
	require.LessOrEqual(t, nextOllamaCloudUsageDelay(60, 20, 0), ollamaCloudUsageMaxDelay+5*time.Minute)
}

func TestOllamaCloudUsageRunnerDisablesAutoRefreshAfterUnpersistableIdentityError(t *testing.T) {
	account := ollamaUsageAccount(14)
	account.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=secret"
	account.Extra[OllamaCloudUsageAutoRefreshExtraKey] = true
	missingProxyID := int64(99)
	account.ProxyID = &missingProxyID
	account.Proxy = nil
	repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{14: account}}}
	settingsRepo := &upstreamBillingProbeSettingRepo{values: map[string]string{
		SettingKeyOllamaCloudUsageSettings: `{"enabled":true,"interval_minutes":60}`,
	}}
	upstream := &ollamaUsageHTTPStub{body: ollamaUsageFixture(t)}
	svc := newOllamaUsageTestService(t, repo, upstream, settingsRepo, true)

	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, int64(1), repo.disableAutoCalls.Load())
	require.Equal(t, false, account.Extra[OllamaCloudUsageAutoRefreshExtraKey])
	require.Zero(t, upstream.calls.Load())

	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, int64(1), repo.disableAutoCalls.Load())
	require.Zero(t, upstream.calls.Load())
}

func TestOllamaCloudUsageRunnerIdentityChangePreservesOldGroupAndDoesNotLoop(t *testing.T) {
	anchor := ollamaUsageAccount(15)
	anchor.Credentials["api_key"] = "shared-before-rotation"
	anchor.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=secret"
	anchor.Extra[OllamaCloudUsageAutoRefreshExtraKey] = true
	sibling := ollamaUsageAccount(16)
	sibling.Platform = PlatformAnthropic
	sibling.Credentials = map[string]any{"api_key": "shared-before-rotation", "base_url": "https://www.ollama.com:443/v1"}
	sibling.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=secret"
	sibling.Extra[OllamaCloudUsageAutoRefreshExtraKey] = true
	dueAnchor := *anchor
	dueAnchor.Credentials = mergeMap(nil, anchor.Credentials)
	dueAnchor.Extra = mergeMap(nil, anchor.Extra)
	repo := &ollamaUsageTestRepo{
		upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
			anchor.ID: anchor, sibling.ID: sibling,
		}},
		due: []Account{dueAnchor},
	}
	var rotateOnce sync.Once
	repo.beforeSnapshot = func() {
		rotateOnce.Do(func() {
			repo.mu.Lock()
			defer repo.mu.Unlock()
			anchor.Credentials["api_key"] = "rotated-account-key"
			delete(anchor.Extra, OllamaCloudUsageSessionExtraKey)
			delete(anchor.Extra, OllamaCloudUsageAutoRefreshExtraKey)
			delete(anchor.Extra, OllamaCloudUsageSnapshotExtraKey)
		})
	}
	settingsRepo := &upstreamBillingProbeSettingRepo{values: map[string]string{
		SettingKeyOllamaCloudUsageSettings: `{"enabled":true,"interval_minutes":60}`,
	}}
	upstream := &ollamaUsageHTTPStub{body: ollamaUsageFixture(t)}
	svc := newOllamaUsageTestService(t, repo, upstream, settingsRepo, true)

	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, int64(1), repo.disableAutoAttempts.Load())
	require.Zero(t, repo.disableAutoCalls.Load(), "the stale anchor CAS must not disable the old sibling group")
	require.Equal(t, true, sibling.Extra[OllamaCloudUsageAutoRefreshExtraKey])
	require.NotContains(t, anchor.Extra, OllamaCloudUsageAutoRefreshExtraKey)

	repo.due = []Account{*anchor, *sibling}
	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, int64(1), repo.disableAutoAttempts.Load(), "the changed account must not be retried")
	require.Equal(t, true, sibling.Extra[OllamaCloudUsageAutoRefreshExtraKey])
	require.NotNil(t, decodeOllamaCloudUsageSnapshot(sibling.Extra), "the still-valid sibling must refresh normally")
	require.Equal(t, int64(2), upstream.calls.Load())
}

func TestOllamaCloudUsageSingleflightConcurrencyAndRunnerSwitches(t *testing.T) {
	accounts := make(map[int64]*Account)
	for id := int64(1); id <= 7; id++ {
		account := ollamaUsageAccount(id)
		account.Extra[OllamaCloudUsageSessionExtraKey] = "cipher:wos-session=secret"
		account.Extra[OllamaCloudUsageAutoRefreshExtraKey] = true
		accounts[id] = account
	}
	repo := &ollamaUsageTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: accounts}}
	unblock := make(chan struct{})
	entered := make(chan struct{}, 10)
	upstream := &ollamaUsageHTTPStub{body: ollamaUsageFixture(t), beforeResponse: func(*http.Request) {
		entered <- struct{}{}
		<-unblock
	}}
	settingsRepo := &upstreamBillingProbeSettingRepo{values: map[string]string{}}
	svc := newOllamaUsageTestService(t, repo, upstream, settingsRepo, true)

	// Global automatic refresh is fail-safe off by default.
	require.NoError(t, svc.RunDue(context.Background()))
	require.Zero(t, upstream.calls.Load())

	settingsRepo.values[SettingKeyOllamaCloudUsageSettings] = `{"enabled":true,"interval_minutes":60}`
	var singleflight sync.WaitGroup
	singleflight.Add(2)
	for range 2 {
		go func() {
			defer singleflight.Done()
			_, _ = svc.Refresh(context.Background(), 1)
		}()
	}
	<-entered
	close(unblock)
	singleflight.Wait()
	require.Equal(t, int64(1), upstream.calls.Load())

	// Clear snapshots so all accounts are due, then verify the shared four-slot bound.
	for _, account := range accounts {
		delete(account.Extra, OllamaCloudUsageSnapshotExtraKey)
	}
	unblock2 := make(chan struct{})
	upstream.beforeResponse = func(*http.Request) { <-unblock2 }
	done := make(chan struct{})
	go func() {
		_ = svc.RunDue(context.Background())
		close(done)
	}()
	require.Eventually(t, func() bool { return upstream.active.Load() == ollamaCloudUsageConcurrency }, time.Second, 10*time.Millisecond)
	close(unblock2)
	<-done
	require.LessOrEqual(t, upstream.maxActive.Load(), int64(ollamaCloudUsageConcurrency))
	require.Equal(t, int64(8), upstream.calls.Load())
}
