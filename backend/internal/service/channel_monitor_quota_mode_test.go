//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

// --- repo / fetcher 装配 ---

// quotaModeRepoStub 记录 RunCheck 落库行为（历史行 + MarkChecked）。
type quotaModeRepoStub struct {
	ChannelMonitorRepository
	monitor   *ChannelMonitor
	history   []*ChannelMonitorHistoryRow
	markedIDs []int64
	updated   []*ChannelMonitor
}

func (r *quotaModeRepoStub) GetByID(_ context.Context, id int64) (*ChannelMonitor, error) {
	if r.monitor == nil || r.monitor.ID != id {
		return nil, ErrChannelMonitorNotFound
	}
	clone := *r.monitor
	return &clone, nil
}

func (r *quotaModeRepoStub) InsertHistoryBatch(_ context.Context, rows []*ChannelMonitorHistoryRow) error {
	r.history = append(r.history, rows...)
	return nil
}

func (r *quotaModeRepoStub) MarkChecked(_ context.Context, id int64, _ time.Time) error {
	r.markedIDs = append(r.markedIDs, id)
	return nil
}

func (r *quotaModeRepoStub) Update(_ context.Context, m *ChannelMonitor) error {
	clone := *m
	r.updated = append(r.updated, &clone)
	return nil
}

// newQuotaModeService 构造启用 V1 探活的 service（复用 retirement/duplicate 测试的 stub）。
func newQuotaModeService(repo *quotaModeRepoStub) *ChannelMonitorService {
	svc := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
	svc.SetRuntimeReader(channelMonitorRuntimeStub{rt: ChannelMonitorRuntime{
		Enabled: true,
		Mode:    ChannelMonitorModeV1,
	}})
	return svc
}

func newQuotaModeFetcher(accounts map[int64]*Account, usage *stubMonitorUsageSource) *ChannelMonitorQuotaFetcher {
	if accounts == nil {
		accounts = make(map[int64]*Account)
	}
	if usage == nil {
		usage = &stubMonitorUsageSource{}
	}
	return &ChannelMonitorQuotaFetcher{
		usage:    usage,
		accounts: &stubMonitorAccountSource{accounts: accounts},
		cache:    make(map[int64]monitorQuotaCacheEntry),
	}
}

// --- RunCheck 分派 ---

func TestRunCheck_QuotaModeProducesSingleQuotaResult(t *testing.T) {
	repo := &quotaModeRepoStub{monitor: &ChannelMonitor{
		ID:              1,
		Name:            "kimi-quota",
		Provider:        MonitorProviderKimi,
		APIMode:         MonitorAPIModeChatCompletions,
		PrimaryModel:    "quota",
		Enabled:         true,
		IntervalSeconds: 60,
		CheckMode:       MonitorCheckModeQuota,
		AccountID:       int64Ptr(9),
	}}
	svc := newQuotaModeService(repo)
	fetcher := newQuotaModeFetcher(map[int64]*Account{
		9: {ID: 9, Platform: domain.PlatformKimi, Credentials: map[string]any{"account_mode": AccountModeCoding}},
	}, nil)
	fetcher.cnQuota = &stubMonitorCNQuotaSource{result: &CNProviderQuotaProbeResult{
		Success:         true,
		CredentialValid: true,
		Tiers:           []CNQuotaTier{{Window: "5h", UsedPercent: 30}},
	}}
	svc.SetQuotaFetcher(fetcher)

	results, err := svc.RunCheck(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, results, 1)

	res := results[0]
	require.Equal(t, "quota", res.Model)
	require.Equal(t, MonitorStatusOperational, res.Status)
	require.Nil(t, res.LatencyMs)
	require.Nil(t, res.PingLatencyMs)
	require.NotNil(t, res.Quota)
	require.True(t, res.Quota.Success)
	require.Equal(t, "cn_quota", res.Quota.Source)

	// 历史行携带配额快照，并推进 last_checked_at。
	require.Len(t, repo.history, 1)
	require.Equal(t, "quota", repo.history[0].Model)
	require.NotNil(t, repo.history[0].Quota)
	require.Equal(t, []int64{1}, repo.markedIDs)
}

func TestRunCheck_QuotaModeUnlinkedAccountDegrades(t *testing.T) {
	repo := &quotaModeRepoStub{monitor: &ChannelMonitor{
		ID:              2,
		Provider:        MonitorProviderDeepseek,
		APIMode:         MonitorAPIModeChatCompletions,
		Endpoint:        "",
		PrimaryModel:    "quota",
		Enabled:         true,
		IntervalSeconds: 60,
		CheckMode:       MonitorCheckModeQuota,
		AccountID:       nil, // FK ON DELETE SET NULL 后的形态
	}}
	svc := newQuotaModeService(repo)
	svc.SetQuotaFetcher(newQuotaModeFetcher(nil, nil))

	results, err := svc.RunCheck(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, MonitorStatusDegraded, results[0].Status)
	require.Contains(t, results[0].Message, "linked account not found")
	require.False(t, results[0].Quota.Success)
}

func TestRunCheck_QuotaModeNilFetcherFailsClosed(t *testing.T) {
	repo := &quotaModeRepoStub{monitor: &ChannelMonitor{
		ID:              3,
		Provider:        MonitorProviderZhipu,
		APIMode:         MonitorAPIModeChatCompletions,
		PrimaryModel:    "quota",
		Enabled:         true,
		IntervalSeconds: 60,
		CheckMode:       MonitorCheckModeQuota,
		AccountID:       int64Ptr(5),
	}}
	svc := newQuotaModeService(repo) // 不注入 fetcher

	results, err := svc.RunCheck(context.Background(), 3)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, MonitorStatusError, results[0].Status)
	require.Contains(t, results[0].Message, "not configured")
}

func TestRunCheck_QuotaProbeAttachesSnapshotToPrimaryRowOnly(t *testing.T) {
	h := &openAICaptureHandler{}
	endpoint := setupFakeOpenAI(t, h)
	repo := &quotaModeRepoStub{monitor: &ChannelMonitor{
		ID:              4,
		Provider:        MonitorProviderOpenAI,
		APIMode:         MonitorAPIModeChatCompletions,
		Endpoint:        endpoint,
		APIKey:          "OLD:sk-openai",
		PrimaryModel:    "gpt-test",
		ExtraModels:     []string{"gpt-extra"},
		Enabled:         true,
		IntervalSeconds: 60,
		CheckMode:       MonitorCheckModeQuotaProbe,
		AccountID:       int64Ptr(12),
	}}
	svc := newQuotaModeService(repo)
	usage := &stubMonitorUsageSource{usage: &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 20},
	}}
	svc.SetQuotaFetcher(newQuotaModeFetcher(map[int64]*Account{
		12: {ID: 12, Platform: domain.PlatformOpenAI},
	}, usage))

	results, err := svc.RunCheck(context.Background(), 4)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// 探活状态为准，配额只挂主模型行。
	require.Equal(t, MonitorStatusOperational, results[0].Status)
	require.NotNil(t, results[0].Quota)
	require.True(t, results[0].Quota.Success)
	require.Equal(t, "usage", results[0].Quota.Source)
	require.Nil(t, results[1].Quota, "extra model rows must not carry quota")

	// 历史落库时同样只有主模型行带快照。
	require.Len(t, repo.history, 2)
	require.NotNil(t, repo.history[0].Quota)
	require.Equal(t, "gpt-test", repo.history[0].Model)
	require.Nil(t, repo.history[1].Quota)
}

func TestRunCheck_QuotaProbeQuotaFailureKeepsProbeStatus(t *testing.T) {
	h := &openAICaptureHandler{}
	endpoint := setupFakeOpenAI(t, h)
	repo := &quotaModeRepoStub{monitor: &ChannelMonitor{
		ID:              5,
		Provider:        MonitorProviderOpenAI,
		APIMode:         MonitorAPIModeChatCompletions,
		Endpoint:        endpoint,
		APIKey:          "OLD:sk-openai",
		PrimaryModel:    "gpt-test",
		Enabled:         true,
		IntervalSeconds: 60,
		CheckMode:       MonitorCheckModeQuotaProbe,
		AccountID:       nil, // 配额侧失效
	}}
	svc := newQuotaModeService(repo)
	svc.SetQuotaFetcher(newQuotaModeFetcher(nil, nil))

	results, err := svc.RunCheck(context.Background(), 5)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, MonitorStatusOperational, results[0].Status, "quota failure must not flip probe status")
	require.False(t, results[0].Quota.Success)
}

// --- attachQuotaSnapshot 细节 ---

func TestAttachQuotaSnapshot_NoteOnlyWhenProbeMessageEmpty(t *testing.T) {
	results := []*CheckResult{
		{Model: "primary", Status: MonitorStatusOperational, Message: "challenge passed"},
		{Model: "extra"},
	}
	failed := &domain.MonitorQuotaSnapshot{Success: false, Error: "boom"}

	attachQuotaSnapshot(results, failed)

	require.Equal(t, "challenge passed", results[0].Message, "existing probe message wins")
	require.Equal(t, failed, results[0].Quota)
	require.Nil(t, results[1].Quota)

	quiet := []*CheckResult{{Model: "primary", Status: MonitorStatusOperational}}
	attachQuotaSnapshot(quiet, failed)
	require.Contains(t, quiet[0].Message, "quota fetch failed: boom")

	attachQuotaSnapshot(nil, failed)  // 空结果不 panic
	attachQuotaSnapshot(results, nil) // 空快照不动结果
}

// --- 校验矩阵 ---

func TestValidateCreateParams_CheckModeMatrix(t *testing.T) {
	accountID := int64(9)

	cases := []struct {
		name    string
		params  ChannelMonitorCreateParams
		wantErr error
	}{
		{
			name: "probe requires endpoint",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderOpenAI, CheckMode: MonitorCheckModeProbe,
				APIKey: "sk", IntervalSeconds: 60, PrimaryModel: "gpt-5",
			},
			wantErr: ErrChannelMonitorInvalidEndpoint,
		},
		{
			name: "probe requires api key",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderOpenAI, CheckMode: MonitorCheckModeProbe,
				Endpoint: "https://api.openai.com", IntervalSeconds: 60, PrimaryModel: "gpt-5",
			},
			wantErr: ErrChannelMonitorMissingAPIKey,
		},
		{
			name: "quota drops endpoint and api key requirements",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderAntigravity, CheckMode: MonitorCheckModeQuota,
				IntervalSeconds: 60, AccountID: &accountID,
			},
			wantErr: nil, // primary_model 默认 "quota"
		},
		{
			name: "quota requires account",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeQuota,
				IntervalSeconds: 60, PrimaryModel: "quota",
			},
			wantErr: ErrChannelMonitorAccountRequired,
		},
		{
			name: "quota_probe requires endpoint and api key too",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeQuotaProbe,
				IntervalSeconds: 60, AccountID: &accountID, PrimaryModel: "kimi-k2",
			},
			wantErr: ErrChannelMonitorInvalidEndpoint,
		},
		{
			name: "antigravity probe unsupported",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderAntigravity, CheckMode: MonitorCheckModeProbe,
				Endpoint: "https://example.com", APIKey: "k",
				IntervalSeconds: 60, AccountID: &accountID, PrimaryModel: "gemini-3-pro",
			},
			wantErr: ErrChannelMonitorInvalidCheckMode,
		},
		{
			name: "antigravity quota_probe unsupported",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderAntigravity, CheckMode: MonitorCheckModeQuotaProbe,
				Endpoint: "https://example.com", APIKey: "k",
				IntervalSeconds: 60, AccountID: &accountID, PrimaryModel: "gemini-3-pro",
			},
			wantErr: ErrChannelMonitorInvalidCheckMode,
		},
		{
			name: "unknown mode rejected",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderOpenAI, CheckMode: "auto",
				Endpoint: "https://api.openai.com", APIKey: "sk",
				IntervalSeconds: 60, PrimaryModel: "gpt-5",
			},
			wantErr: ErrChannelMonitorInvalidCheckMode,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCreateParams(tc.params)
			if tc.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

func TestNormalizeMonitorPrimaryModel_QuotaDefault(t *testing.T) {
	require.Equal(t, "quota", normalizeMonitorPrimaryModel(MonitorProviderKimi, MonitorCheckModeQuota, ""))
	require.Equal(t, "quota", normalizeMonitorPrimaryModel(MonitorProviderAntigravity, MonitorCheckModeQuota, "  "))
	// 探活模式沿用原语义：grok 默认模型，其余必填（空串报错在 validateCreateParams）。
	require.Equal(t, MonitorDefaultGrokModel, normalizeMonitorPrimaryModel(MonitorProviderGrok, MonitorCheckModeProbe, ""))
	require.Equal(t, "kimi-k2", normalizeMonitorPrimaryModel(MonitorProviderKimi, MonitorCheckModeQuotaProbe, "kimi-k2"))
}

func TestProviderProbeCapabilityMatrix(t *testing.T) {
	require.False(t, providerSupportsProbe(MonitorProviderAntigravity))
	for _, p := range []string{
		MonitorProviderOpenAI, MonitorProviderAnthropic, MonitorProviderGemini,
		MonitorProviderGrok, MonitorProviderKimi, MonitorProviderZhipu, MonitorProviderDeepseek,
	} {
		require.True(t, providerSupportsProbe(p), p)
	}
	for _, p := range []string{
		MonitorProviderOpenAI, MonitorProviderAnthropic, MonitorProviderGemini,
		MonitorProviderGrok, MonitorProviderAntigravity,
		MonitorProviderKimi, MonitorProviderZhipu, MonitorProviderDeepseek,
	} {
		require.NoError(t, validateProvider(p), p)
	}
}

// --- 关联账号校验 ---

func TestValidateLinkedAccount_Matrix(t *testing.T) {
	svc := NewChannelMonitorService(nil, nil)
	fetcher := newQuotaModeFetcher(map[int64]*Account{
		1: {ID: 1, Platform: domain.PlatformKimi},
	}, nil)
	svc.SetQuotaFetcher(fetcher)

	require.NoError(t, svc.validateLinkedAccount(context.Background(), MonitorProviderKimi, nil))
	require.NoError(t, svc.validateLinkedAccount(context.Background(), MonitorProviderKimi, int64Ptr(0)))
	require.NoError(t, svc.validateLinkedAccount(context.Background(), MonitorProviderKimi, int64Ptr(1)))
	require.ErrorIs(t, svc.validateLinkedAccount(context.Background(), MonitorProviderZhipu, int64Ptr(1)), ErrChannelMonitorProviderIncompatible)
	require.ErrorIs(t, svc.validateLinkedAccount(context.Background(), MonitorProviderKimi, int64Ptr(404)), ErrChannelMonitorAccountRequired)

	noFetcher := NewChannelMonitorService(nil, nil)
	require.ErrorIs(t, noFetcher.validateLinkedAccount(context.Background(), MonitorProviderKimi, int64Ptr(1)), ErrChannelMonitorAccountRequired)
}

func TestRevalidateLinkedAccount_QuotaErrorsProbeUnbinds(t *testing.T) {
	fetcher := newQuotaModeFetcher(nil, nil) // 账号一律加载失败
	svc := NewChannelMonitorService(nil, nil)
	svc.SetQuotaFetcher(fetcher)

	quota := &ChannelMonitor{Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeQuota, AccountID: int64Ptr(9)}
	require.ErrorIs(t, svc.revalidateLinkedAccount(context.Background(), quota), ErrChannelMonitorAccountRequired)
	require.NotNil(t, quota.AccountID)

	probe := &ChannelMonitor{Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeProbe, AccountID: int64Ptr(9)}
	require.NoError(t, svc.revalidateLinkedAccount(context.Background(), probe))
	require.Nil(t, probe.AccountID, "probe mode should silently unbind stale account")

	quotaNoAccount := &ChannelMonitor{Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeQuota}
	require.ErrorIs(t, svc.revalidateLinkedAccount(context.Background(), quotaNoAccount), ErrChannelMonitorAccountRequired)
}

func TestRevalidateLinkedAccount_PlatformMismatch(t *testing.T) {
	svc := NewChannelMonitorService(nil, nil)
	svc.SetQuotaFetcher(newQuotaModeFetcher(map[int64]*Account{
		2: {ID: 2, Platform: domain.PlatformDeepseek},
	}, nil))

	quota := &ChannelMonitor{Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeQuota, AccountID: int64Ptr(2)}
	require.ErrorIs(t, svc.revalidateLinkedAccount(context.Background(), quota), ErrChannelMonitorProviderIncompatible)

	probe := &ChannelMonitor{Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeProbe, AccountID: int64Ptr(2)}
	require.NoError(t, svc.revalidateLinkedAccount(context.Background(), probe))
	require.Nil(t, probe.AccountID)
}

// --- quota → probe 切换的 key 管控（validateProbeAPIKey） ---

func TestValidateProbeAPIKey_QuotaToProbeRequiresFreshKey(t *testing.T) {
	svc := NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})

	quota := &ChannelMonitor{Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeQuota, APIKey: "NEW:"}
	require.NoError(t, svc.validateProbeAPIKey(quota, "")) // quota 模式不管 key

	quota.CheckMode = MonitorCheckModeProbe
	// 存量密文解出空明文（quota 监控存的加密空串）→ 必须重填 key。
	require.ErrorIs(t, svc.validateProbeAPIKey(quota, ""), ErrChannelMonitorMissingAPIKey)
	// 提供新明文 key → 放行。
	require.NoError(t, svc.validateProbeAPIKey(quota, "sk-fresh"))
	// 密文解出非空明文 → 放行。
	require.NoError(t, svc.validateProbeAPIKey(
		&ChannelMonitor{Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeProbe, APIKey: "OLD:sk-live"}, ""))
}

// --- Duplicate：quota 模式空明文重加密 ---

func TestDuplicateChannelMonitorQuotaModeReencryptsEmptyKey(t *testing.T) {
	accountID := int64(9)
	source := &ChannelMonitor{
		ID:              42,
		Name:            "kimi-quota",
		Provider:        MonitorProviderKimi,
		APIMode:         MonitorAPIModeChatCompletions,
		Endpoint:        "",
		APIKey:          "OLD:", // 解密为空串（quota 监控的加密空 key）
		PrimaryModel:    "quota",
		Enabled:         true,
		IntervalSeconds: 60,
		CheckMode:       MonitorCheckModeQuota,
		AccountID:       &accountID,
	}
	repo := &duplicateChannelMonitorRepoStub{source: source}
	service := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})

	dup, err := service.Duplicate(context.Background(), 42, 7, "admin:7", "op-1")
	require.NoError(t, err)
	require.Equal(t, MonitorCheckModeQuota, dup.CheckMode)
	require.NotNil(t, dup.AccountID)
	require.Equal(t, accountID, *dup.AccountID)
	require.Empty(t, dup.APIKey, "plaintext stays empty for quota monitors")
	require.Len(t, repo.created, 1)
	require.Equal(t, "NEW:", repo.created[0].APIKey, "empty key must be re-encrypted, not dropped")
}
