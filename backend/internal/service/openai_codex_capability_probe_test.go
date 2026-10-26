//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type codexCapabilityProbeAccountRepo struct {
	AccountRepository
	mu           sync.Mutex
	accounts     map[int64]*Account
	byPlatform   []Account
	extraUpdates []map[string]any
	updated      chan struct{}
}

func (r *codexCapabilityProbeAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if account, ok := r.accounts[id]; ok {
		return account, nil
	}
	return nil, ErrNoAvailableAccounts
}

func (r *codexCapabilityProbeAccountRepo) ListByPlatform(_ context.Context, _ string) ([]Account, error) {
	return append([]Account(nil), r.byPlatform...), nil
}

func (r *codexCapabilityProbeAccountRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.mu.Lock()
	r.extraUpdates = append(r.extraUpdates, updates)
	r.mu.Unlock()
	if r.updated != nil {
		r.updated <- struct{}{}
	}
	return nil
}

func (r *codexCapabilityProbeAccountRepo) updateCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.extraUpdates)
}

func newCodexCapabilityProbeServer(t *testing.T, body *atomic.Value) (*atomic.Int32, *atomic.Value) {
	t.Helper()
	calls := &atomic.Int32{}
	versions := &atomic.Value{}
	versions.Store("")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		versions.Store(r.URL.Query().Get("client_version"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body.Load().(string)))
	}))
	t.Cleanup(server.Close)
	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	t.Cleanup(func() { chatgptCodexModelsURL = original })
	return calls, versions
}

func newCodexCapabilityProbeAccount(id int64, accountType string) *Account {
	return &Account{
		ID:       id,
		Platform: PlatformOpenAI,
		Type:     accountType,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "acc-123",
		},
	}
}

func TestCodexCapabilityProbe_WritesOnChangeOnly(t *testing.T) {
	body := &atomic.Value{}
	body.Store(codexCapabilitiesBlueAccountManifest)
	calls, versions := newCodexCapabilityProbeServer(t, body)
	repo := &codexCapabilityProbeAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	probe := svc.CodexCapabilityProbe()
	account := newCodexCapabilityProbeAccount(1, AccountTypeOAuth)

	require.NoError(t, probe.Probe(context.Background(), account))
	require.EqualValues(t, 1, calls.Load())
	require.Equal(t, CodexCanonicalClientVersion(), versions.Load(), "探测必须使用规范客户端版本")
	require.Equal(t, 1, repo.updateCount())
	caps := account.CodexCapabilities()
	require.NotNil(t, caps, "内存副本随写入更新")
	require.True(t, caps.SupportsCyberProgram("daybreak_blue"))

	// 未变化不写
	require.NoError(t, probe.Probe(context.Background(), account))
	require.EqualValues(t, 2, calls.Load())
	require.Equal(t, 1, repo.updateCount())

	// 权益撤销：变化写入，记录移除 daybreak_blue
	body.Store(codexCapabilitiesStandardAccountManifest)
	require.NoError(t, probe.Probe(context.Background(), account))
	require.Equal(t, 2, repo.updateCount())
	require.False(t, account.CodexCapabilities().SupportsCyberProgram("daybreak_blue"))
	require.Equal(t, []string{"priority"}, account.CodexCapabilities().ServiceTiers)
}

func TestCodexCapabilityProbe_EmptyManifestDoesNotOverwrite(t *testing.T) {
	body := &atomic.Value{}
	body.Store(`{"models":[]}`)
	_, _ = newCodexCapabilityProbeServer(t, body)
	repo := &codexCapabilityProbeAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	existing, ok := DeriveCodexAccountCapabilities([]byte(codexCapabilitiesBlueAccountManifest))
	require.True(t, ok)
	account := newCodexCapabilityProbeAccount(1, AccountTypeOAuth)
	account.Extra = map[string]any{CodexCapabilitiesExtraKey: existing}

	err := svc.CodexCapabilityProbe().Probe(context.Background(), account)
	require.ErrorIs(t, err, errCodexCapabilityManifestEmpty)
	require.Zero(t, repo.updateCount())
	require.True(t, account.CodexCapabilities().SupportsCyberProgram("daybreak_blue"))
}

func TestCodexCapabilityProbe_UpstreamFailureKeepsRecord(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	t.Cleanup(func() { chatgptCodexModelsURL = original })

	repo := &codexCapabilityProbeAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := newCodexCapabilityProbeAccount(1, AccountTypeOAuth)
	require.Error(t, svc.CodexCapabilityProbe().Probe(context.Background(), account))
	require.Zero(t, repo.updateCount())
}

func TestCodexCapabilityProbe_SkipsNonOAuthLikeAccounts(t *testing.T) {
	body := &atomic.Value{}
	body.Store(codexCapabilitiesBlueAccountManifest)
	calls, _ := newCodexCapabilityProbeServer(t, body)
	svc := &OpenAIGatewayService{accountRepo: &codexCapabilityProbeAccountRepo{}}
	apiKey := &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk"}}
	require.NoError(t, svc.CodexCapabilityProbe().Probe(context.Background(), apiKey))
	require.NoError(t, svc.CodexCapabilityProbe().Probe(context.Background(), nil))
	require.Zero(t, calls.Load())
}

func TestCodexCapabilityProbe_SetupTokenUsesOAuthManifestPath(t *testing.T) {
	body := &atomic.Value{}
	body.Store(codexCapabilitiesStandardAccountManifest)
	calls, _ := newCodexCapabilityProbeServer(t, body)
	repo := &codexCapabilityProbeAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := newCodexCapabilityProbeAccount(2, AccountTypeSetupToken)

	require.NoError(t, svc.CodexCapabilityProbe().Probe(context.Background(), account))
	require.EqualValues(t, 1, calls.Load())
	require.Equal(t, 1, repo.updateCount())

	manifest, err := svc.FetchCodexModelsManifest(context.Background(), account, "", "")
	require.NoError(t, err)
	require.JSONEq(t, codexCapabilitiesStandardAccountManifest, string(manifest.Body))
}

// manifest 缓存在 60 秒内新鲜直接返回；探测必须绕过缓存直达上游，成功后回填缓存。
func TestCodexCapabilityProbe_BypassesFreshManifestCacheAndBackfillsIt(t *testing.T) {
	body := &atomic.Value{}
	body.Store(codexCapabilitiesBlueAccountManifest)
	calls, _ := newCodexCapabilityProbeServer(t, body)
	repo := &codexCapabilityProbeAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := newCodexCapabilityProbeAccount(1, AccountTypeOAuth)

	manifest, err := svc.FetchCodexModelsManifest(context.Background(), account, "", "")
	require.NoError(t, err)
	require.JSONEq(t, codexCapabilitiesBlueAccountManifest, string(manifest.Body))
	require.EqualValues(t, 1, calls.Load())
	require.Equal(t, 1, repo.updateCount(), "规范版本的真实上游拉取顺手回写一次")

	body.Store(codexCapabilitiesStandardAccountManifest)
	require.NoError(t, svc.CodexCapabilityProbe().Probe(context.Background(), account))
	require.EqualValues(t, 2, calls.Load(), "缓存新鲜时探测仍发出上游请求")
	require.False(t, account.CodexCapabilities().SupportsCyberProgram("daybreak_blue"))
	require.Equal(t, 2, repo.updateCount())

	manifest, err = svc.FetchCodexModelsManifest(context.Background(), account, "", "")
	require.NoError(t, err)
	require.JSONEq(t, codexCapabilitiesStandardAccountManifest, string(manifest.Body), "探测结果回填缓存条目")
	require.EqualValues(t, 2, calls.Load(), "回填后的缓存命中不再请求上游")
	require.Equal(t, 2, repo.updateCount(), "缓存命中不回写")
}

func TestFetchCodexModelsManifest_RecordsOnlyCanonicalOrNewerClientVersion(t *testing.T) {
	body := &atomic.Value{}
	body.Store(codexCapabilitiesBlueAccountManifest)
	calls, _ := newCodexCapabilityProbeServer(t, body)
	repo := &codexCapabilityProbeAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := newCodexCapabilityProbeAccount(1, AccountTypeOAuth)

	_, err := svc.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")
	require.NoError(t, err)
	require.EqualValues(t, 1, calls.Load())
	require.Zero(t, repo.updateCount(), "旧版本客户端拉到的目录是子集，不回写")

	_, err = svc.FetchCodexModelsManifest(context.Background(), account, "99.0.0", "")
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load())
	require.Equal(t, 1, repo.updateCount(), "更新版本的目录是超集，回写")
}

func TestCodexCapabilityProbe_RecordFromManifestVersionGate(t *testing.T) {
	repo := &codexCapabilityProbeAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	probe := svc.CodexCapabilityProbe()
	account := newCodexCapabilityProbeAccount(1, AccountTypeOAuth)

	probe.RecordFromManifest(context.Background(), account, "0.100.0", []byte(codexCapabilitiesBlueAccountManifest))
	require.Zero(t, repo.updateCount())
	probe.RecordFromManifest(context.Background(), account, "garbage", []byte(codexCapabilitiesBlueAccountManifest))
	require.Zero(t, repo.updateCount())
	probe.RecordFromManifest(context.Background(), account, CodexCanonicalClientVersion(), []byte(codexCapabilitiesBlueAccountManifest))
	require.Equal(t, 1, repo.updateCount())
	require.True(t, account.CodexCapabilities().SupportsCyberProgram("daybreak_blue"))
	probe.RecordFromManifest(context.Background(), account, CodexCanonicalClientVersion(), []byte(`{"models":[]}`))
	require.Equal(t, 1, repo.updateCount(), "空列表不覆盖")
	require.True(t, account.CodexCapabilities().SupportsCyberProgram("daybreak_blue"))
}

func TestCodexCapabilityProbe_ReprobeCooldown(t *testing.T) {
	body := &atomic.Value{}
	body.Store(codexCapabilitiesBlueAccountManifest)
	calls, _ := newCodexCapabilityProbeServer(t, body)
	account := newCodexCapabilityProbeAccount(7, AccountTypeOAuth)
	other := newCodexCapabilityProbeAccount(8, AccountTypeOAuth)
	repo := &codexCapabilityProbeAccountRepo{accounts: map[int64]*Account{7: account, 8: other}, updated: make(chan struct{}, 4)}
	svc := &OpenAIGatewayService{accountRepo: repo}
	probe := svc.CodexCapabilityProbe()

	require.True(t, probe.ReprobeAsync(7))
	select {
	case <-repo.updated:
	case <-time.After(5 * time.Second):
		t.Fatal("async probe did not persist capabilities")
	}
	require.EqualValues(t, 1, calls.Load())
	require.False(t, probe.ReprobeAsync(7), "冷却窗口内跳过")
	require.EqualValues(t, 1, calls.Load())
	require.False(t, probe.ReprobeAsync(0))

	// 常规探测不占用拒绝重探测的冷却：刚被创建 / 刷新 / 测试探测过的账号收到拒绝仍可立即重探测。
	require.NoError(t, probe.Probe(context.Background(), other))
	require.EqualValues(t, 2, calls.Load())
	require.True(t, probe.ReprobeAsync(8), "常规探测后拒绝重探测不受冷却限制")
	require.Eventually(t, func() bool { return calls.Load() == 3 }, 5*time.Second, 10*time.Millisecond)
	require.False(t, probe.ReprobeAsync(8), "重探测后进入冷却")
}

func TestCodexCapabilityProbe_BackfillMissingProbesOnlyUnrecordedActiveOAuthLike(t *testing.T) {
	body := &atomic.Value{}
	body.Store(codexCapabilitiesStandardAccountManifest)
	calls, _ := newCodexCapabilityProbeServer(t, body)
	recorded, ok := DeriveCodexAccountCapabilities([]byte(codexCapabilitiesBlueAccountManifest))
	require.True(t, ok)
	withRecord := newCodexCapabilityProbeAccount(1, AccountTypeOAuth)
	withRecord.Extra = map[string]any{CodexCapabilitiesExtraKey: recorded}
	missing := newCodexCapabilityProbeAccount(2, AccountTypeSetupToken)
	apiKey := &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Credentials: map[string]any{"api_key": "sk"}}
	inactive := newCodexCapabilityProbeAccount(4, AccountTypeOAuth)
	inactive.Status = StatusDisabled
	repo := &codexCapabilityProbeAccountRepo{byPlatform: []Account{*withRecord, *missing, *apiKey, *inactive}}
	svc := &OpenAIGatewayService{accountRepo: repo}

	probed := svc.CodexCapabilityProbe().BackfillMissing(context.Background())
	require.Equal(t, 1, probed)
	require.EqualValues(t, 1, calls.Load())
	require.Equal(t, 1, repo.updateCount())
}

func TestCodexCapabilityProbe_StartStopIdempotent(t *testing.T) {
	repo := &codexCapabilityProbeAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	probe := svc.CodexCapabilityProbe()
	probe.Start()
	probe.Start()
	probe.Stop()
	probe.Stop()
	require.Same(t, probe, svc.CodexCapabilityProbe())
}
