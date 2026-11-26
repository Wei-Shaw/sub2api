package service

// GLM Coding Plan 账号接入统一 usage 管线的回归测试：
// 快照 → UsageProgress 的构建、快照过期判定、探测 TTL 节流，以及
// getZhipuUsage 全链路（探测成功合并快照 / 探测失败降级快照 / 无快照透出错误）。

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

func TestBuildZhipuUsageProgressFromExtra(t *testing.T) {
	t.Parallel()

	resetAt := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	future := resetAt.Format(time.RFC3339)

	t.Run("empty extra returns nil", func(t *testing.T) {
		require.Nil(t, buildZhipuUsageProgressFromExtra(nil, "5h", time.Now()))
	})

	t.Run("5h window builds from snapshot keys", func(t *testing.T) {
		extra := map[string]any{
			"zhipu_5h_used_percent": float64(20),
			"zhipu_5h_reset_at":     future,
		}
		progress := buildZhipuUsageProgressFromExtra(extra, "5h", time.Now())
		require.NotNil(t, progress)
		require.InDelta(t, 20.0, progress.Utilization, 1e-9)
		require.NotNil(t, progress.ResetsAt)
		require.Equal(t, resetAt.Unix(), progress.ResetsAt.Unix())
		require.Positive(t, progress.RemainingSeconds)
	})

	t.Run("weekly window maps onto SevenDay via applyZhipuExtraToUsage", func(t *testing.T) {
		extra := map[string]any{
			"zhipu_5h_used_percent":     float64(20),
			"zhipu_5h_reset_at":         future,
			"zhipu_weekly_used_percent": float64(70),
			"zhipu_weekly_reset_at":     resetAt.Add(24 * time.Hour).Format(time.RFC3339),
		}
		usage := &UsageInfo{}
		applyZhipuExtraToUsage(usage, extra, time.Now())
		require.NotNil(t, usage.FiveHour)
		require.NotNil(t, usage.SevenDay)
		require.InDelta(t, 20.0, usage.FiveHour.Utilization, 1e-9)
		require.InDelta(t, 70.0, usage.SevenDay.Utilization, 1e-9)
	})

	t.Run("expired window resets utilization to zero", func(t *testing.T) {
		past := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
		extra := map[string]any{
			"zhipu_5h_used_percent": float64(80),
			"zhipu_5h_reset_at":     past,
		}
		progress := buildZhipuUsageProgressFromExtra(extra, "5h", time.Now())
		require.NotNil(t, progress)
		require.InDelta(t, 0.0, progress.Utilization, 1e-9)
	})

	t.Run("missing used percent key returns nil", func(t *testing.T) {
		extra := map[string]any{"zhipu_5h_reset_at": future}
		require.Nil(t, buildZhipuUsageProgressFromExtra(extra, "5h", time.Now()))
	})

	t.Run("unknown window returns nil", func(t *testing.T) {
		extra := map[string]any{"zhipu_5h_used_percent": float64(20)}
		require.Nil(t, buildZhipuUsageProgressFromExtra(extra, "monthly", time.Now()))
	})
}

func TestZhipuSnapshotNeedsRefresh(t *testing.T) {
	t.Parallel()

	now := time.Now()
	snapshot := func(age time.Duration) map[string]any {
		return map[string]any{
			cnExtraKey(PlatformZhipu, cnExtraSuffixUsageUpdated): now.Add(-age).Format(time.RFC3339),
		}
	}
	freshUsage := &UsageInfo{FiveHour: &UsageProgress{}, SevenDay: &UsageProgress{}}

	t.Run("nil account never refreshes", func(t *testing.T) {
		require.False(t, zhipuSnapshotNeedsRefresh(nil, freshUsage, now))
	})
	t.Run("missing windows triggers refresh", func(t *testing.T) {
		require.True(t, zhipuSnapshotNeedsRefresh(&Account{Extra: snapshot(0)}, &UsageInfo{}, now))
	})
	t.Run("nil extra triggers refresh", func(t *testing.T) {
		require.True(t, zhipuSnapshotNeedsRefresh(&Account{}, freshUsage, now))
	})
	t.Run("missing timestamp triggers refresh", func(t *testing.T) {
		require.True(t, zhipuSnapshotNeedsRefresh(&Account{Extra: map[string]any{}}, freshUsage, now))
	})
	t.Run("fresh snapshot skips refresh", func(t *testing.T) {
		require.False(t, zhipuSnapshotNeedsRefresh(&Account{Extra: snapshot(time.Minute)}, freshUsage, now))
	})
	t.Run("stale snapshot refreshes", func(t *testing.T) {
		require.True(t, zhipuSnapshotNeedsRefresh(&Account{Extra: snapshot(zhipuCodingSnapshotStaleAfter + time.Minute)}, freshUsage, now))
	})
}

func TestShouldProbeZhipuSnapshot_TTLThrottle(t *testing.T) {
	t.Parallel()

	svc := &AccountUsageService{cache: NewUsageCache()}
	now := time.Now()

	require.True(t, svc.shouldProbeZhipuSnapshot(1, now), "first probe passes")
	require.False(t, svc.shouldProbeZhipuSnapshot(1, now.Add(time.Minute)), "probe inside TTL is throttled")
	require.True(t, svc.shouldProbeZhipuSnapshot(1, now.Add(zhipuCodingProbeCacheTTL+time.Second)), "probe after TTL passes")
	require.True(t, svc.shouldProbeZhipuSnapshot(1, now.Add(time.Minute), true), "force bypasses TTL")
	require.False(t, svc.shouldProbeZhipuSnapshot(1, now.Add(time.Minute+time.Second)), "timestamp updated by force throttles again")
}

// zhipuQuotaTestRepo 记录 CN 探测的持久化更新。
type zhipuQuotaTestRepo struct {
	fakeCNProbeAccountRepo
	updates map[string]any
}

func (r *zhipuQuotaTestRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	r.updates = updates
	return nil
}

// zhipuQuotaTestUpstream 返回智谱 monitor 端点形态的配额响应（unit 3=5h / 6=weekly）。
type zhipuQuotaTestUpstream struct {
	recordingHTTPUpstream
	request *http.Request
}

func (u *zhipuQuotaTestUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	u.calls++
	u.request = req
	body := fmt.Sprintf(`{
		"success": true,
		"data": {
			"limits": [
				{"type":"TOKENS_LIMIT","unit":3,"percentage":20,"nextResetTime":%d},
				{"type":"TOKENS_LIMIT","unit":6,"percentage":70,"nextResetTime":%d}
			],
			"level": "pro"
		}
	}`, time.Now().Add(2*time.Hour).UnixMilli(), time.Now().Add(48*time.Hour).UnixMilli())
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func newZhipuUsageTestService(repo AccountRepository, upstream HTTPUpstream) *AccountUsageService {
	return &AccountUsageService{
		cache:                  NewUsageCache(),
		cnProviderQuotaService: NewCNProviderQuotaService(repo, nil, upstream, nil),
	}
}

func TestGetZhipuUsage_ProbesAndMergesSnapshot(t *testing.T) {
	t.Parallel()

	repo := &zhipuQuotaTestRepo{fakeCNProbeAccountRepo: fakeCNProbeAccountRepo{account: codingAccount(PlatformZhipu)}}
	upstream := &zhipuQuotaTestUpstream{}
	svc := newZhipuUsageTestService(repo, upstream)
	account := codingAccount(PlatformZhipu)

	usage, err := svc.getZhipuUsage(context.Background(), account, false)
	require.NoError(t, err)
	require.NotNil(t, usage)

	// 探测结果合并为 5h + weekly（SevenDay）窗口。
	require.NotNil(t, usage.FiveHour)
	require.InDelta(t, 20.0, usage.FiveHour.Utilization, 1e-9)
	require.NotNil(t, usage.SevenDay)
	require.InDelta(t, 70.0, usage.SevenDay.Utilization, 1e-9)
	require.Positive(t, usage.FiveHour.RemainingSeconds)
	require.Empty(t, usage.Error)

	// 内存 Extra 与 CN 探测落库的快照键一致。
	require.Contains(t, account.Extra, "zhipu_5h_used_percent")
	require.Contains(t, repo.updates, "zhipu_weekly_used_percent")

	// 探测端点与鉴权对齐 CN 链路（官方 monitor 端点、裸 Key）。
	require.Equal(t, 1, upstream.calls)
	require.Equal(t, http.MethodGet, upstream.request.Method)
	require.Equal(t, "https://open.bigmodel.cn/api/monitor/usage/quota/limit", upstream.request.URL.String())
	require.Equal(t, "sk-test", upstream.request.Header.Get("Authorization"))

	// 快照新鲜：再次查询不再出站（needsRefresh 已满足，且 TTL 节流兜底）。
	_, err = svc.getZhipuUsage(context.Background(), account, false)
	require.NoError(t, err)
	require.Equal(t, 1, upstream.calls, "fresh snapshot must not trigger another probe")
}

func TestGetZhipuUsage_DegradesToSnapshotWhenProbeFails(t *testing.T) {
	t.Parallel()

	stale := time.Now().Add(-zhipuCodingSnapshotStaleAfter - time.Hour).UTC().Format(time.RFC3339)
	account := codingAccount(PlatformZhipu)
	account.Extra = map[string]any{
		"zhipu_5h_used_percent":     float64(30),
		"zhipu_5h_reset_at":         time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		"zhipu_weekly_used_percent": float64(60),
		"zhipu_weekly_reset_at":     time.Now().Add(12 * time.Hour).UTC().Format(time.RFC3339),
		cnExtraKey(PlatformZhipu, cnExtraSuffixUsageUpdated): stale,
	}

	repo := &zhipuQuotaTestRepo{fakeCNProbeAccountRepo: fakeCNProbeAccountRepo{account: account}}
	upstream := &recordingHTTPUpstream{}
	svc := newZhipuUsageTestService(repo, upstream)

	usage, err := svc.getZhipuUsage(context.Background(), account, false)
	require.NoError(t, err)
	require.NotNil(t, usage.FiveHour)
	require.InDelta(t, 30.0, usage.FiveHour.Utilization, 1e-9)
	require.NotNil(t, usage.SevenDay)
	require.InDelta(t, 60.0, usage.SevenDay.Utilization, 1e-9)
	require.Empty(t, usage.Error, "probe failure must degrade to snapshot without surfacing an error")
	require.Equal(t, 1, upstream.calls)
}

func TestGetZhipuUsage_SurfacesErrorWhenNoSnapshotAndProbeFails(t *testing.T) {
	t.Parallel()

	repo := &zhipuQuotaTestRepo{fakeCNProbeAccountRepo: fakeCNProbeAccountRepo{account: codingAccount(PlatformZhipu)}}
	upstream := &recordingHTTPUpstream{}
	svc := newZhipuUsageTestService(repo, upstream)
	account := codingAccount(PlatformZhipu)

	usage, err := svc.getZhipuUsage(context.Background(), account, false)
	require.NoError(t, err)
	require.NotEmpty(t, usage.Error, "probe failure with no snapshot must surface the error for diagnosis")
	require.Nil(t, usage.FiveHour)
	require.Nil(t, usage.SevenDay)
}

func TestGetZhipuUsage_FreshSnapshotSkipsProbe(t *testing.T) {
	t.Parallel()

	account := codingAccount(PlatformZhipu)
	account.Extra = map[string]any{
		"zhipu_5h_used_percent":                                 float64(10),
		"zhipu_weekly_used_percent":                             float64(40),
		cnExtraKey(PlatformZhipu, cnExtraSuffixUsageUpdated):   time.Now().Format(time.RFC3339),
	}

	repo := &zhipuQuotaTestRepo{fakeCNProbeAccountRepo: fakeCNProbeAccountRepo{account: account}}
	upstream := &zhipuQuotaTestUpstream{}
	svc := newZhipuUsageTestService(repo, upstream)

	usage, err := svc.getZhipuUsage(context.Background(), account, false)
	require.NoError(t, err)
	require.Zero(t, upstream.calls, "fresh snapshot must not probe")
	require.NotNil(t, usage.FiveHour)
	require.InDelta(t, 10.0, usage.FiveHour.Utilization, 1e-9)
	require.NotNil(t, usage.SevenDay)
	require.InDelta(t, 40.0, usage.SevenDay.Utilization, 1e-9)
}

// zhipuWindowStatsLogRepo 覆写 GetAccountWindowStats 以断言窗口统计的挂载。
type zhipuWindowStatsLogRepo struct {
	usageBatchLogRepoStub
	started []time.Time
}

func (r *zhipuWindowStatsLogRepo) GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error) {
	r.started = append(r.started, startTime)
	return &usagestats.AccountStats{Requests: 3, Tokens: 1200, Cost: 0.5}, nil
}

func TestGetZhipuUsage_AttachesWindowStats(t *testing.T) {
	t.Parallel()

	resetAt := time.Now().Add(90 * time.Minute).UTC().Truncate(time.Second)
	account := codingAccount(PlatformZhipu)
	account.Extra = map[string]any{
		"zhipu_5h_used_percent":                     float64(25),
		"zhipu_5h_reset_at":                         resetAt.Format(time.RFC3339),
		"zhipu_weekly_used_percent":                 float64(50),
		"zhipu_weekly_reset_at":                     resetAt.Add(24 * time.Hour).Format(time.RFC3339),
		cnExtraKey(PlatformZhipu, cnExtraSuffixUsageUpdated): time.Now().Format(time.RFC3339),
	}

	repo := &zhipuQuotaTestRepo{fakeCNProbeAccountRepo: fakeCNProbeAccountRepo{account: account}}
	svc := newZhipuUsageTestService(repo, &zhipuQuotaTestUpstream{})
	logRepo := &zhipuWindowStatsLogRepo{}
	svc.usageLogRepo = logRepo

	usage, err := svc.getZhipuUsage(context.Background(), account, false)
	require.NoError(t, err)
	require.Len(t, logRepo.started, 2, "both 5h and weekly windows must aggregate local stats")

	require.NotNil(t, usage.FiveHour.WindowStats)
	require.Equal(t, int64(3), usage.FiveHour.WindowStats.Requests)
	require.NotNil(t, usage.SevenDay.WindowStats)
	require.Equal(t, int64(1200), usage.SevenDay.WindowStats.Tokens)

	// 窗口统计起点锚定在 reset_at - 窗口长度（与 codex 窗口统计一致）。
	require.WithinDuration(t, resetAt.Add(-5*time.Hour), logRepo.started[0], time.Second)
	require.WithinDuration(t, resetAt.Add(24*time.Hour).Add(-7*24*time.Hour), logRepo.started[1], time.Second)
}
