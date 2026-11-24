package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

type agQuotaRecoveryClear struct {
	accountID int64
	key       string
}

type agQuotaRecoveryRepo struct {
	AccountRepository
	accounts []Account
	cleared  []agQuotaRecoveryClear
	clearErr error
}

func (r *agQuotaRecoveryRepo) ListByPlatform(_ context.Context, platform string) ([]Account, error) {
	var out []Account
	for _, account := range r.accounts {
		if account.Platform == platform {
			out = append(out, account)
		}
	}
	return out, nil
}

func (r *agQuotaRecoveryRepo) ClearModelRateLimit(_ context.Context, id int64, scope string) error {
	if r.clearErr != nil {
		return r.clearErr
	}
	r.cleared = append(r.cleared, agQuotaRecoveryClear{accountID: id, key: scope})
	return nil
}

type agQuotaRecoveryProxyRepo struct {
	ProxyRepository
	proxies map[int64]*Proxy
	err     error
}

func (r *agQuotaRecoveryProxyRepo) GetByID(_ context.Context, id int64) (*Proxy, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.proxies[id], nil
}

// agQuotaUpstream is a local stand-in for the three Antigravity endpoints a quota fetch reads.
type agQuotaUpstream struct {
	mu           sync.Mutex
	hits         int
	modelsStatus int
	models       string
	summary      string
	loadAssist   string
}

func (u *agQuotaUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	u.hits++
	u.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, ":fetchAvailableModels"):
		if u.modelsStatus != 0 {
			w.WriteHeader(u.modelsStatus)
			_, _ = w.Write([]byte(`{"error":{"message":"backend error"}}`))
			return
		}
		_, _ = w.Write([]byte(u.models))
	case strings.HasSuffix(r.URL.Path, ":retrieveUserQuotaSummary"):
		_, _ = w.Write([]byte(u.summary))
	case strings.HasSuffix(r.URL.Path, ":loadCodeAssist"):
		_, _ = w.Write([]byte(u.loadAssist))
	default:
		http.NotFound(w, r)
	}
}

func (u *agQuotaUpstream) hitCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.hits
}

func agQuotaSummaryJSON(gemini5h, geminiWeekly, thirdParty5h, thirdPartyWeekly float64) string {
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	return `{"groups":[{"displayName":"Gemini Models","buckets":[` +
		`{"bucketId":"gemini-5h","window":"5h","remainingFraction":` + f(gemini5h) + `},` +
		`{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":` + f(geminiWeekly) + `}]},` +
		`{"displayName":"Claude and GPT models","buckets":[` +
		`{"bucketId":"3p-5h","window":"5h","remainingFraction":` + f(thirdParty5h) + `},` +
		`{"bucketId":"3p-weekly","window":"weekly","remainingFraction":` + f(thirdPartyWeekly) + `}]}]}`
}

func startAgQuotaUpstream(t *testing.T, upstream *agQuotaUpstream) *httptest.Server {
	t.Helper()
	if upstream.loadAssist == "" {
		upstream.loadAssist = `{"paidTier":{"id":"g1-ultra-tier"}}`
	}
	server := httptest.NewServer(upstream)
	t.Cleanup(server.Close)

	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	t.Cleanup(func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
	})
	antigravity.BaseURLs = []string{server.URL}
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)
	t.Setenv(antigravityForwardBaseURLEnv, "")
	return server
}

func agLimitedAccount(id int64, now time.Time, keys ...string) Account {
	limits := map[string]any{}
	for _, key := range keys {
		limits[key] = map[string]any{
			"rate_limited_at":     now.Add(-7 * time.Hour).UTC().Format(time.RFC3339),
			"rate_limit_reset_at": now.Add(109 * time.Hour).UTC().Format(time.RFC3339),
		}
	}
	return Account{
		ID:          id,
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"access_token": "token", "project_id": "project"},
		Extra:       map[string]any{modelRateLimitsKey: limits},
	}
}

func newAgQuotaRecoveryService(repo *agQuotaRecoveryRepo, proxyRepo ProxyRepository) *AntigravityQuotaRecoveryService {
	cfg := &config.Config{}
	cfg.Gateway.ModelsListReadMaxBytes = 1 << 20
	return NewAntigravityQuotaRecoveryService(repo, proxyRepo, NewAntigravityQuotaFetcher(proxyRepo, cfg), antigravityQuotaRecoveryInterval)
}

// runAgQuotaRecoveryDue registers the limited accounts, then runs the tick at which every first check is due.
func runAgQuotaRecoveryDue(svc *AntigravityQuotaRecoveryService, now time.Time) {
	svc.runOnce(now)
	svc.runOnce(now.Add(antigravityQuotaRecoveryInterval))
}

func TestAntigravityQuotaGroupForRateLimitKey(t *testing.T) {
	cases := []struct {
		key   string
		group string
		ok    bool
	}{
		{"antigravity:gemini", "gemini", true},
		{"antigravity:gemini:argon", "gemini", true},
		{"gemini-4-neon-high", "gemini", true},
		{"gemini-3.8-flash-low", "gemini", true},
		{"models/gemini-4-carbon-medium", "gemini", true},
		{"claude-opus-5-5-high", "3p", true},
		{"gpt-oss-120b-medium", "3p", true},
		{"gemini-3-pro-image", "", false},
		{"AICredits", "", false},
		{"openai:image_generation", "", false},
		{"some-unknown-model", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		group, ok := antigravityQuotaGroupForRateLimitKey(tc.key)
		require.Equal(t, tc.ok, ok, tc.key)
		require.Equal(t, tc.group, group, tc.key)
	}
}

func agUsage(gemini5h, geminiWeekly float64, models map[string]int) *UsageInfo {
	info := &UsageInfo{
		AntigravityQuota: map[string]*AntigravityModelQuota{},
		AntigravityQuotaGroups: []*AntigravityQuotaGroup{
			{Key: "gemini", Windows: []*AntigravityQuotaWindow{
				{Window: "5h", Utilization: gemini5h},
				{Window: "weekly", Utilization: geminiWeekly},
			}},
		},
	}
	for model, utilization := range models {
		info.AntigravityQuota[model] = &AntigravityModelQuota{Utilization: utilization}
	}
	return info
}

func TestAntigravityQuotaShowsRecovered(t *testing.T) {
	const model = "gemini-4-neon-high"
	scope := antigravityGeminiModelRateLimitKey

	// The weekly window is still exhausted.
	require.False(t, antigravityQuotaShowsRecovered(agUsage(0, 100, nil), scope))
	require.False(t, antigravityQuotaShowsRecovered(agUsage(100, 3, nil), scope))
	// Both windows have room again.
	require.True(t, antigravityQuotaShowsRecovered(agUsage(0, 0, nil), scope))
	require.True(t, antigravityQuotaShowsRecovered(agUsage(99.5, 42, map[string]int{model: 0}), model))
	// The model itself has nothing left although the group shows room.
	require.False(t, antigravityQuotaShowsRecovered(agUsage(0, 0, map[string]int{model: 100}), model))
	// The exhausted model does not hold back the family scope or its siblings.
	require.True(t, antigravityQuotaShowsRecovered(agUsage(0, 0, map[string]int{model: 100}), scope))

	// Missing data never clears: no group for the key, a group without windows, no summary,
	// a forbidden or degraded result, an unknown key.
	require.False(t, antigravityQuotaShowsRecovered(agUsage(0, 0, nil), "claude-opus-5-5-high"))
	require.False(t, antigravityQuotaShowsRecovered(&UsageInfo{AntigravityQuotaGroups: []*AntigravityQuotaGroup{{Key: "gemini"}}}, scope))
	require.False(t, antigravityQuotaShowsRecovered(&UsageInfo{AntigravityQuota: map[string]*AntigravityModelQuota{model: {Utilization: 0}}}, model))
	require.False(t, antigravityQuotaShowsRecovered(nil, scope))
	forbidden := agUsage(0, 0, nil)
	forbidden.IsForbidden = true
	require.False(t, antigravityQuotaShowsRecovered(forbidden, scope))
	degraded := agUsage(0, 0, nil)
	degraded.ErrorCode = errorCodeNetworkError
	require.False(t, antigravityQuotaShowsRecovered(degraded, scope))
	require.False(t, antigravityQuotaShowsRecovered(agUsage(0, 0, nil), "some-unknown-model"))
}

func TestRecoverAntigravityModelRateLimitsFromQuota(t *testing.T) {
	now := time.Now()
	const model = "gemini-4-neon-high"

	t.Run("clears each recovered key and updates the account object", func(t *testing.T) {
		repo := &agQuotaRecoveryRepo{}
		account := agLimitedAccount(77, now, model, antigravityGeminiModelRateLimitKey, "some-unknown-model")
		account.RateLimitResetAt = &now
		shared, _ := account.Extra[modelRateLimitsKey].(map[string]any)

		cleared := recoverAntigravityModelRateLimitsFromQuota(context.Background(), repo, &account, agUsage(0, 0, map[string]int{model: 0}), now)

		require.Equal(t, []string{antigravityGeminiModelRateLimitKey, model}, cleared)
		require.Equal(t, []agQuotaRecoveryClear{{77, antigravityGeminiModelRateLimitKey}, {77, model}}, repo.cleared)
		require.False(t, account.isRateLimitActiveForKey(model))
		require.False(t, account.isRateLimitActiveForKey(antigravityGeminiModelRateLimitKey))
		require.True(t, account.isRateLimitActiveForKey("some-unknown-model"))
		require.Len(t, shared, 3, "the map shared with the scheduler snapshot is not mutated")
		require.NotNil(t, account.RateLimitResetAt, "the account-level limit is left alone")
	})

	t.Run("an expired limit is not cleared", func(t *testing.T) {
		repo := &agQuotaRecoveryRepo{}
		account := agLimitedAccount(77, now, model)
		require.Empty(t, recoverAntigravityModelRateLimitsFromQuota(context.Background(), repo, &account, agUsage(0, 0, nil), now.Add(200*time.Hour)))
		require.Empty(t, repo.cleared)
	})

	t.Run("a failed clear keeps the limit on the account object", func(t *testing.T) {
		repo := &agQuotaRecoveryRepo{clearErr: errors.New("db down")}
		account := agLimitedAccount(77, now, model)
		require.Empty(t, recoverAntigravityModelRateLimitsFromQuota(context.Background(), repo, &account, agUsage(0, 0, nil), now))
		require.True(t, account.isRateLimitActiveForKey(model))
	})

	t.Run("other platforms are ignored", func(t *testing.T) {
		repo := &agQuotaRecoveryRepo{}
		account := agLimitedAccount(77, now, model)
		account.Platform = PlatformGemini
		require.Empty(t, recoverAntigravityModelRateLimitsFromQuota(context.Background(), repo, &account, agUsage(0, 0, nil), now))
		require.Empty(t, repo.cleared)
	})
}

func TestAntigravityQuotaRecovery_SkipsAccountsWithoutLimits(t *testing.T) {
	upstream := &agQuotaUpstream{models: `{"models":{}}`, summary: agQuotaSummaryJSON(1, 1, 1, 1)}
	startAgQuotaUpstream(t, upstream)
	now := time.Now()

	unlimited := agLimitedAccount(1, now)
	expired := agLimitedAccount(2, now, "gemini-4-neon-high")
	expired.Extra[modelRateLimitsKey].(map[string]any)["gemini-4-neon-high"].(map[string]any)["rate_limit_reset_at"] = now.Add(-time.Minute).UTC().Format(time.RFC3339)
	undecidable := agLimitedAccount(3, now, "some-unknown-model")
	inactive := agLimitedAccount(4, now, "gemini-4-neon-high")
	inactive.Status = StatusError

	repo := &agQuotaRecoveryRepo{accounts: []Account{unlimited, expired, undecidable, inactive}}
	svc := newAgQuotaRecoveryService(repo, nil)
	for i := 0; i <= 25; i++ {
		svc.runOnce(now.Add(time.Duration(i) * time.Minute))
	}

	require.Zero(t, upstream.hitCount(), "no upstream call when nothing can be lifted")
	require.Empty(t, repo.cleared)
	require.Empty(t, svc.nextDue)
}

func TestAntigravityQuotaRecovery_ClearsResetGroupAndKeepsExhaustedGroup(t *testing.T) {
	// Gemini was reset early (both windows full again); the Claude/GPT weekly window is used up.
	upstream := &agQuotaUpstream{
		models: `{"models":{"gemini-4-neon-high":{"quotaInfo":{"remainingFraction":1}},` +
			`"claude-opus-5-5-high":{"quotaInfo":{"remainingFraction":0}}}}`,
		summary: agQuotaSummaryJSON(1, 1, 1, 0),
	}
	startAgQuotaUpstream(t, upstream)
	now := time.Now()

	account := agLimitedAccount(77, now, "gemini-4-neon-high", antigravityGeminiModelRateLimitKey, "claude-opus-5-5-high")
	account.Schedulable = false
	repo := &agQuotaRecoveryRepo{accounts: []Account{account}}
	svc := newAgQuotaRecoveryService(repo, nil)

	svc.runOnce(now)
	require.Zero(t, upstream.hitCount(), "the first check waits for the account's slot")
	svc.runOnce(now.Add(antigravityQuotaRecoveryInterval))

	require.Equal(t, 3, upstream.hitCount())
	require.Equal(t, []agQuotaRecoveryClear{
		{77, antigravityGeminiModelRateLimitKey},
		{77, "gemini-4-neon-high"},
	}, repo.cleared)

	// The next check comes one interval later, not on the following ticks.
	svc.runOnce(now.Add(antigravityQuotaRecoveryInterval + 5*time.Minute))
	require.Equal(t, 3, upstream.hitCount())
	svc.runOnce(now.Add(2 * antigravityQuotaRecoveryInterval))
	require.Equal(t, 6, upstream.hitCount())
}

func TestAntigravityQuotaRecovery_ModelWithoutQuotaKeepsItsLimit(t *testing.T) {
	upstream := &agQuotaUpstream{
		models:  `{"models":{"gemini-4-neon-high":{"quotaInfo":{"remainingFraction":0}}}}`,
		summary: agQuotaSummaryJSON(0.6, 0.9, 1, 1),
	}
	startAgQuotaUpstream(t, upstream)
	now := time.Now()

	repo := &agQuotaRecoveryRepo{accounts: []Account{agLimitedAccount(77, now, "gemini-4-neon-high")}}
	svc := newAgQuotaRecoveryService(repo, nil)
	runAgQuotaRecoveryDue(svc, now)

	require.Equal(t, 3, upstream.hitCount())
	require.Empty(t, repo.cleared)
}

func TestAntigravityQuotaRecovery_ExhaustedWeeklyKeepsLimit(t *testing.T) {
	upstream := &agQuotaUpstream{models: `{"models":{}}`, summary: agQuotaSummaryJSON(1, 0, 1, 1)}
	startAgQuotaUpstream(t, upstream)
	now := time.Now()

	repo := &agQuotaRecoveryRepo{accounts: []Account{agLimitedAccount(77, now, antigravityGeminiModelRateLimitKey)}}
	svc := newAgQuotaRecoveryService(repo, nil)
	runAgQuotaRecoveryDue(svc, now)

	require.Equal(t, 3, upstream.hitCount())
	require.Empty(t, repo.cleared)
}

func TestAntigravityQuotaRecovery_FetchErrorKeepsLimit(t *testing.T) {
	upstream := &agQuotaUpstream{modelsStatus: http.StatusInternalServerError, summary: agQuotaSummaryJSON(1, 1, 1, 1)}
	startAgQuotaUpstream(t, upstream)
	now := time.Now()

	repo := &agQuotaRecoveryRepo{accounts: []Account{agLimitedAccount(77, now, antigravityGeminiModelRateLimitKey)}}
	svc := newAgQuotaRecoveryService(repo, nil)
	runAgQuotaRecoveryDue(svc, now)

	require.Positive(t, upstream.hitCount())
	require.Empty(t, repo.cleared)
}

// The AICredits marker is written for causes a credit balance does not show, so it is left to
// expire even when credits are plentiful and every quota window has room.
func TestAntigravityQuotaRecovery_LeavesCreditsMarkerAlone(t *testing.T) {
	upstream := &agQuotaUpstream{
		models:     `{"models":{}}`,
		summary:    agQuotaSummaryJSON(1, 1, 1, 1),
		loadAssist: `{"paidTier":{"id":"g1-ultra-tier","availableCredits":[{"creditType":"GOOGLE_ONE_AI","creditAmount":"25","minimumCreditAmountForUsage":"5"}]}}`,
	}
	startAgQuotaUpstream(t, upstream)
	now := time.Now()

	repo := &agQuotaRecoveryRepo{accounts: []Account{agLimitedAccount(77, now, creditsExhaustedKey)}}
	svc := newAgQuotaRecoveryService(repo, nil)
	runAgQuotaRecoveryDue(svc, now)

	require.Zero(t, upstream.hitCount())
	require.Empty(t, repo.cleared)
}

// A limit that resets within minutes is a rate or capacity cooldown. It is kept although the
// quota shows room, and it alone does not make the account worth an upstream call.
func TestAntigravityQuotaRecovery_ShortCooldownIsKept(t *testing.T) {
	upstream := &agQuotaUpstream{models: `{"models":{}}`, summary: agQuotaSummaryJSON(1, 1, 1, 1)}
	startAgQuotaUpstream(t, upstream)
	now := time.Now()
	setReset := func(account *Account, key string, in time.Duration) {
		limits, _ := account.Extra[modelRateLimitsKey].(map[string]any)
		entry, _ := limits[key].(map[string]any)
		entry["rate_limit_reset_at"] = now.Add(in).UTC().Format(time.RFC3339)
	}

	shortOnly := agLimitedAccount(1, now, antigravityGeminiModelRateLimitKey)
	setReset(&shortOnly, antigravityGeminiModelRateLimitKey, 5*time.Minute)
	repo := &agQuotaRecoveryRepo{accounts: []Account{shortOnly}}
	svc := newAgQuotaRecoveryService(repo, nil)
	svc.runOnce(now)
	svc.runOnce(now.Add(2 * time.Minute))
	require.Zero(t, upstream.hitCount())
	require.Empty(t, repo.cleared)

	// Next to a long limit of the same group, only the long one is lifted.
	mixed := agLimitedAccount(2, now, antigravityGeminiModelRateLimitKey, "gemini-4-neon-high")
	setReset(&mixed, "gemini-4-neon-high", 5*time.Minute)
	cleared := recoverAntigravityModelRateLimitsFromQuota(context.Background(), repo, &mixed, agUsage(0, 0, nil), now)
	require.Equal(t, []string{antigravityGeminiModelRateLimitKey}, cleared)
	require.True(t, mixed.isRateLimitActiveForKey("gemini-4-neon-high"))

	// The boundary itself is kept; one second more is considered.
	edge := agLimitedAccount(3, now, antigravityGeminiModelRateLimitKey)
	resetAt := *edge.modelRateLimitResetAt(antigravityGeminiModelRateLimitKey)
	require.Empty(t, activeAntigravityModelRateLimits(&edge, resetAt.Add(-antigravityQuotaRecoveryMinRemaining)))
	require.Len(t, activeAntigravityModelRateLimits(&edge, resetAt.Add(-antigravityQuotaRecoveryMinRemaining-time.Second)), 1)
}

func TestAntigravityQuotaRecovery_BrokenBoundProxySkipsAccount(t *testing.T) {
	upstream := &agQuotaUpstream{models: `{"models":{}}`, summary: agQuotaSummaryJSON(1, 1, 1, 1)}
	startAgQuotaUpstream(t, upstream)
	now := time.Now()
	proxyID := int64(1)

	for name, proxyRepo := range map[string]ProxyRepository{
		"load error":    &agQuotaRecoveryProxyRepo{err: errors.New("db down")},
		"proxy missing": &agQuotaRecoveryProxyRepo{},
		"no repository": nil,
	} {
		t.Run(name, func(t *testing.T) {
			account := agLimitedAccount(77, now, antigravityGeminiModelRateLimitKey)
			account.ProxyID = &proxyID
			repo := &agQuotaRecoveryRepo{accounts: []Account{account}}
			svc := newAgQuotaRecoveryService(repo, proxyRepo)
			runAgQuotaRecoveryDue(svc, now)

			require.Zero(t, upstream.hitCount(), "never falls back to a direct connection")
			require.Empty(t, repo.cleared)
		})
	}
}

func TestAntigravityQuotaRecovery_UsesBoundProxy(t *testing.T) {
	// The direct endpoint must stay untouched; every request has to arrive at the proxy.
	direct := &agQuotaUpstream{models: `{"models":{}}`, summary: agQuotaSummaryJSON(0, 0, 0, 0)}
	startAgQuotaUpstream(t, direct)
	viaProxy := &agQuotaUpstream{
		models:     `{"models":{}}`,
		summary:    agQuotaSummaryJSON(1, 1, 1, 1),
		loadAssist: `{"paidTier":{"id":"g1-ultra-tier"}}`,
	}
	proxyServer := httptest.NewServer(viaProxy)
	t.Cleanup(proxyServer.Close)
	proxyURL, err := url.Parse(proxyServer.URL)
	require.NoError(t, err)
	host, portRaw, err := net.SplitHostPort(proxyURL.Host)
	require.NoError(t, err)
	port, err := strconv.Atoi(portRaw)
	require.NoError(t, err)

	now := time.Now()
	proxyID := int64(1)
	account := agLimitedAccount(77, now, antigravityGeminiModelRateLimitKey)
	account.ProxyID = &proxyID
	repo := &agQuotaRecoveryRepo{accounts: []Account{account}}
	proxyRepo := &agQuotaRecoveryProxyRepo{proxies: map[int64]*Proxy{proxyID: {ID: proxyID, Protocol: "http", Host: host, Port: port, Status: StatusActive}}}
	svc := newAgQuotaRecoveryService(repo, proxyRepo)
	runAgQuotaRecoveryDue(svc, now)

	require.Zero(t, direct.hitCount())
	require.Equal(t, 3, viaProxy.hitCount())
	require.Equal(t, []agQuotaRecoveryClear{{77, antigravityGeminiModelRateLimitKey}}, repo.cleared)
}

func TestAntigravityQuotaRecovery_StaggersFirstChecks(t *testing.T) {
	svc := NewAntigravityQuotaRecoveryService(nil, nil, nil, antigravityQuotaRecoveryInterval)
	seen := map[time.Duration]struct{}{}
	for id := int64(1); id <= 10; id++ {
		delay := svc.firstCheckDelay(id)
		require.Greater(t, delay, time.Duration(0))
		require.LessOrEqual(t, delay, antigravityQuotaRecoveryInterval)
		seen[delay] = struct{}{}
	}
	require.Len(t, seen, 10)
}

// The admin usage view applies the same decision to the result it has just fetched, so it never
// shows a recovered quota next to a limit that is still in force.
func TestGetAntigravityUsage_FreshFetchLiftsRecoveredLimits(t *testing.T) {
	upstream := &agQuotaUpstream{models: `{"models":{}}`, summary: agQuotaSummaryJSON(1, 1, 1, 0)}
	startAgQuotaUpstream(t, upstream)
	now := time.Now()

	repo := &agQuotaRecoveryRepo{}
	cfg := &config.Config{}
	cfg.Gateway.ModelsListReadMaxBytes = 1 << 20
	svc := &AccountUsageService{
		accountRepo:             repo,
		antigravityQuotaFetcher: NewAntigravityQuotaFetcher(nil, cfg),
		cache:                   NewUsageCache(),
	}
	account := agLimitedAccount(77, now, antigravityGeminiModelRateLimitKey, "claude-opus-5-5-high")

	usage, err := svc.getAntigravityUsage(context.Background(), &account, true)
	require.NoError(t, err)
	require.Len(t, usage.AntigravityQuotaGroups, 2)
	require.Equal(t, []agQuotaRecoveryClear{{77, antigravityGeminiModelRateLimitKey}}, repo.cleared)
	require.False(t, account.isRateLimitActiveForKey(antigravityGeminiModelRateLimitKey))
	require.True(t, account.isRateLimitActiveForKey("claude-opus-5-5-high"))

	// A cached answer does not act again.
	_, err = svc.getAntigravityUsage(context.Background(), &account, false)
	require.NoError(t, err)
	require.Equal(t, 3, upstream.hitCount())
	require.Len(t, repo.cleared, 1)
}
