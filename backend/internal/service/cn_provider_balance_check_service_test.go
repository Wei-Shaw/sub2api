package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 周期任务对 coding plan 账号的额度探测行为（runOnce 集成路径）：
//   - kimi coding 账号（含已被阈值停调的）→ 额度探测被调用；
//   - 智谱 coding 账号 → 额度探测被调用（智谱不进 kimi/deepseek 余额循环）；
//   - payg 账号不经过额度探测（走余额路径，本测试不放 payg 账号避免真实网络）；
//   - 非激活账号完全跳过。

// fakeCNQuotaProber 需要并发安全：runOnce 以 cnQuotaProbeConcurrency 并发调用 QueryUsage。
type fakeCNQuotaProber struct {
	mu     sync.Mutex
	probed []int64
}

func (f *fakeCNQuotaProber) QueryUsage(ctx context.Context, accountID int64) (*CNProviderQuotaProbeResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.probed = append(f.probed, accountID)
	return &CNProviderQuotaProbeResult{Success: true, Persisted: true}, nil
}

type fakeCNCheckRepo struct {
	AccountRepository
	byPlatform map[string][]Account
}

func (r *fakeCNCheckRepo) ListByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return r.byPlatform[platform], nil
}

func TestCNProviderBalanceCheckRunOnceProbesCodingPlanQuota(t *testing.T) {
	kimiActive := Account{ID: 1, Platform: PlatformKimi, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"account_mode": "coding"}}
	// 已被阈值停调的 coding 账号也要刷新快照（决定是否续停）。
	kimiPaused := Account{ID: 2, Platform: PlatformKimi, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: false,
		Credentials: map[string]any{"account_mode": "coding"}}
	// 非激活账号跳过。
	kimiInactive := Account{ID: 3, Platform: PlatformKimi, Type: AccountTypeAPIKey, Status: StatusDisabled,
		Credentials: map[string]any{"account_mode": "coding"}}
	zhipuCoding := Account{ID: 4, Platform: PlatformZhipu, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"account_mode": "coding"}}
	minimaxCoding := Account{ID: 5, Platform: PlatformMiniMax, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"account_mode": "coding"}}

	repo := &fakeCNCheckRepo{byPlatform: map[string][]Account{
		PlatformKimi:    {kimiActive, kimiPaused, kimiInactive},
		PlatformZhipu:   {zhipuCoding},
		PlatformMiniMax: {minimaxCoding},
	}}
	prober := &fakeCNQuotaProber{}
	svc := &CNProviderBalanceCheckService{
		accountRepo:  repo,
		quotaService: prober,
		cfg:          &config.Config{},
	}

	svc.runOnce()

	require.ElementsMatch(t, []int64{1, 2, 4, 5}, prober.probed)
}

// runOnceZhipuQuota 在 quotaService 缺失时安全跳过（Start 门控不启动的老部署路径）。
func TestCNProviderBalanceCheckRunOnceWithoutQuotaService(t *testing.T) {
	repo := &fakeCNCheckRepo{byPlatform: map[string][]Account{
		PlatformZhipu: {{ID: 4, Platform: PlatformZhipu, Type: AccountTypeAPIKey, Status: StatusActive,
			Credentials: map[string]any{"account_mode": "coding"}}},
	}}
	svc := &CNProviderBalanceCheckService{accountRepo: repo, cfg: &config.Config{}}
	require.NotPanics(t, func() { svc.runOnce() })
}

// recordingCNBalanceLoadRepo 记录余额探测服务的 GetByID 调用：payg 账号进入
// payg 检查队列必然触发 loadPayGAccount → GetByID，以此断言"未入队"这一内部
// 状态。GetByID 直接报错，保证不发生真实外呼。
type recordingCNBalanceLoadRepo struct {
	AccountRepository
	getByIDIDs []int64
}

func (r *recordingCNBalanceLoadRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	r.getByIDIDs = append(r.getByIDIDs, id)
	return nil, errors.New("not found")
}

// 挂在国产平台下、base_url 指向官方 ollama.com 的账号由 Ollama Cloud 用量窗口
// 负责：CN 探测端点由 base_url 衍生，ollama.com 会被出站 URL 白名单拒绝
// （CN_BALANCE_URL_REJECTED），周期任务必须整体跳过——不进额度目标，也不进
// payg 检查队列。
func TestCNProviderBalanceCheckRunOnceSkipsOllamaCloudUsageAccounts(t *testing.T) {
	// 对照组：普通 kimi coding 账号仍进额度探测。
	kimiCoding := Account{ID: 1, Platform: PlatformKimi, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"account_mode": "coding"}}
	// ollama.com 挂 kimi：coding 模式不进额度目标。
	ollamaKimiCoding := Account{ID: 2, Platform: PlatformKimi, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"account_mode": "coding", "base_url": "https://ollama.com"}}
	// ollama.com 挂 kimi：payg 模式不进 payg 检查队列。
	ollamaKimiPayg := Account{ID: 3, Platform: PlatformKimi, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"base_url": "https://ollama.com", "api_key": "sk-ollama"}}

	loadRepo := &recordingCNBalanceLoadRepo{}
	repo := &fakeCNCheckRepo{byPlatform: map[string][]Account{
		PlatformKimi: {kimiCoding, ollamaKimiCoding, ollamaKimiPayg},
	}}
	prober := &fakeCNQuotaProber{}
	svc := &CNProviderBalanceCheckService{
		accountRepo:    repo,
		balanceService: NewCNProviderBalanceService(loadRepo, nil, nil, &config.Config{}),
		quotaService:   prober,
		cfg:            &config.Config{},
	}

	svc.runOnce()

	require.Equal(t, []int64{1}, prober.probed)
	require.Empty(t, loadRepo.getByIDIDs, "ollama 账号不得进入 payg 检查队列")
}

// 双币种（deepseek CNY+USD）停调判定：任一币种达标即不停调，全部低于阈值才停；
// 无明细时退回主币种（兼容旧结果）。
func TestAllCNBalancesBelowThreshold(t *testing.T) {
	dualLow := &CNProviderBalanceResult{
		Balance:  1.0,
		Currency: "CNY",
		Balances: []CNProviderBalanceEntry{
			{Currency: "CNY", Balance: 1.0},
			{Currency: "USD", Balance: 0.5},
		},
	}
	require.True(t, allCNBalancesBelowThreshold(dualLow, 5.0))

	dualMixed := &CNProviderBalanceResult{
		Balance:  1.0,
		Currency: "CNY",
		Balances: []CNProviderBalanceEntry{
			{Currency: "CNY", Balance: 1.0},
			{Currency: "USD", Balance: 20.0},
		},
	}
	require.False(t, allCNBalancesBelowThreshold(dualMixed, 5.0))

	// 无明细：按主币种判定（旧行为）。
	singleLow := &CNProviderBalanceResult{Balance: 1.0, Currency: "CNY"}
	require.True(t, allCNBalancesBelowThreshold(singleLow, 5.0))
	singleOK := &CNProviderBalanceResult{Balance: 10.0, Currency: "CNY"}
	require.False(t, allCNBalancesBelowThreshold(singleOK, 5.0))
}

// OpenDesign 的用量快照由周期任务刷新：有 control key 的账号进探测队列，
// 没走过设备授权的账号跳过（探测必然 401，白跑还刷失败日志）。
func TestCNProviderBalanceCheckRunOnceProbesOpenDesign(t *testing.T) {
	authorized := Account{ID: 21, Platform: PlatformOpenDesign, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{
			"api_key":                     "sk-amr",
			OpenDesignUsageCredentialsKey: "vela_ctrl_x",
			OpenDesignUsageWorkspaceKey:   "ws-1",
		}}
	// 被阈值停调的账号也要保持快照新鲜，用于判定是否续停。
	paused := Account{ID: 22, Platform: PlatformOpenDesign, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: false,
		Credentials: map[string]any{
			OpenDesignUsageCredentialsKey: "vela_ctrl_x",
			OpenDesignUsageWorkspaceKey:   "ws-1",
		}}
	// 手工建的 API Key 账号没有 control key：不探测。
	noControlKey := Account{ID: 23, Platform: PlatformOpenDesign, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"api_key": "sk-amr"}}
	// 非激活账号跳过。
	inactive := Account{ID: 24, Platform: PlatformOpenDesign, Type: AccountTypeAPIKey, Status: StatusDisabled,
		Credentials: map[string]any{OpenDesignUsageCredentialsKey: "vela_ctrl_x"}}

	repo := &fakeCNCheckRepo{byPlatform: map[string][]Account{
		PlatformOpenDesign: {authorized, paused, noControlKey, inactive},
	}}
	prober := &fakeCNQuotaProber{}
	svc := &CNProviderBalanceCheckService{
		accountRepo:  repo,
		quotaService: prober,
		cfg:          &config.Config{},
	}

	svc.runOnce()

	require.ElementsMatch(t, []int64{21, 22}, prober.probed)
}
