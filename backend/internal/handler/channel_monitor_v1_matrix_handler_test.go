//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ---------- 假体 ----------

// channelMonitorV1MatrixSettingRepoStub 是路由 / handler 门禁测试用的最小 SettingRepository。
type channelMonitorV1MatrixSettingRepoStub struct {
	values map[string]string
}

func (s *channelMonitorV1MatrixSettingRepoStub) Get(context.Context, string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *channelMonitorV1MatrixSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

func (s *channelMonitorV1MatrixSettingRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *channelMonitorV1MatrixSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *channelMonitorV1MatrixSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *channelMonitorV1MatrixSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *channelMonitorV1MatrixSettingRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func newChannelMonitorV1MatrixSettings(enabled bool, mode string) *service.SettingService {
	enabledValue := "false"
	if enabled {
		enabledValue = "true"
	}
	return service.NewSettingService(&channelMonitorV1MatrixSettingRepoStub{
		values: map[string]string{
			service.SettingKeyChannelMonitorEnabled: enabledValue,
			service.SettingKeyChannelMonitorMode:    mode,
		},
	}, &config.Config{})
}

type channelMonitorV1MatrixHandlerMonitorsStub struct {
	monitors []*service.ChannelMonitor
}

func (s *channelMonitorV1MatrixHandlerMonitorsStub) ListEnabledMonitors(context.Context) ([]*service.ChannelMonitor, error) {
	return s.monitors, nil
}

// channelMonitorV1MatrixHandlerRepoStub 同时提供两路假数据：
// points = 柱子（最近 N 次探测，与档位无关）；counts = 成功率 / 健康分（窗口内计数）。
type channelMonitorV1MatrixHandlerRepoStub struct {
	points     map[int64][]service.ChannelMonitorV1HistoryPoint
	counts     map[int64]service.ChannelMonitorV1StatusCounts
	coverage   map[int64]service.ChannelMonitorV1CoverageBounds
	liveGroups service.ChannelMonitorV1LiveGroupIDs
}

func (s *channelMonitorV1MatrixHandlerRepoStub) LoadMonitorV1RecentPoints(
	_ context.Context,
	_ []int64,
	_ int,
) (map[int64][]service.ChannelMonitorV1HistoryPoint, error) {
	return s.points, nil
}

func (s *channelMonitorV1MatrixHandlerRepoStub) LoadMonitorV1WindowCounts(
	_ context.Context,
	_ []int64,
	_, _ time.Time,
) (map[int64]service.ChannelMonitorV1StatusCounts, error) {
	return s.counts, nil
}

// channelMonitorV1MatrixHandlerLatency 构造一次探测的延迟样本指针（LatencyMs 是可空字段）。
func channelMonitorV1MatrixHandlerLatency(ms int64) *int64 { return &ms }

func (s *channelMonitorV1MatrixHandlerRepoStub) LoadMonitorV1Coverage(
	_ context.Context,
	_ []int64,
) (map[int64]service.ChannelMonitorV1CoverageBounds, error) {
	return s.coverage, nil
}

func (s *channelMonitorV1MatrixHandlerRepoStub) LoadMonitorV1LiveGroupIDs(
	_ context.Context,
	_ []int64,
) (service.ChannelMonitorV1LiveGroupIDs, error) {
	return s.liveGroups, nil
}

// ---------- 响应解析 ----------

type channelMonitorV1MatrixEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		GroupBy  string `json:"group_by"`
		Coverage struct {
			RequestedStart time.Time `json:"requested_start"`
			RequestedEnd   time.Time `json:"requested_end"`
			BucketSeconds  int       `json:"bucket_seconds"`
			ComputedAt     time.Time `json:"computed_at"`
		} `json:"coverage"`
		Items []struct {
			Platform  string `json:"platform"`
			GroupID   *int64 `json:"group_id"`
			GroupName string `json:"group_name"`
			Model     string `json:"model"`
			Metrics   struct {
				RequestCount int64   `json:"request_count"`
				SuccessRate  float64 `json:"success_rate"`
			} `json:"metrics"`
			Health struct {
				Overall string   `json:"overall"`
				Score   *float64 `json:"score"`
			} `json:"health"`
			Buckets []struct {
				BucketStart time.Time `json:"bucket_start"`
				Health      struct {
					Overall string   `json:"overall"`
					Score   *float64 `json:"score"`
				} `json:"health"`
			} `json:"buckets"`
		} `json:"items"`
	} `json:"data"`
}

func runChannelMonitorV1MatrixHandler(
	t *testing.T,
	h *ChannelMonitorV1MatrixHandler,
	target string,
) (*httptest.ResponseRecorder, channelMonitorV1MatrixEnvelope) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	h.Matrix(c)

	var envelope channelMonitorV1MatrixEnvelope
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	return recorder, envelope
}

// ---------- 门禁 ----------

func TestChannelMonitorV1MatrixHandlerGatedReturnsEmptyItemsWith200(t *testing.T) {
	tests := []struct {
		name    string
		service *service.SettingService
	}{
		{"feature disabled", newChannelMonitorV1MatrixSettings(false, service.ChannelMonitorModeV1)},
		{"mode v2", newChannelMonitorV1MatrixSettings(true, service.ChannelMonitorModeV2)},
		{"feature disabled and mode v2", newChannelMonitorV1MatrixSettings(false, service.ChannelMonitorModeV2)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			monitors := &channelMonitorV1MatrixHandlerMonitorsStub{
				monitors: []*service.ChannelMonitor{{ID: 1, Name: "不该出现", Provider: "openai"}},
			}
			h := NewChannelMonitorV1MatrixHandler(
				service.NewChannelMonitorV1MatrixService(monitors, &channelMonitorV1MatrixHandlerRepoStub{}),
				test.service,
			)

			recorder, envelope := runChannelMonitorV1MatrixHandler(t, h, "/channel-monitors/matrix?range=24h-5m&group_by=platform_group")
			require.Equal(t, http.StatusOK, recorder.Code, "门禁不通过必须是 200 而不是 403")
			require.Equal(t, 0, envelope.Code)
			require.Equal(t, "success", envelope.Message)
			require.Empty(t, envelope.Data.Items)
			require.Contains(t, recorder.Body.String(), `"items":[]`, "空 items 必须序列化成 []")
			// coverage 仍按请求窗口填充，避免前端拿到零值时间戳算出无意义的横轴。
			require.Equal(t, "platform_group", envelope.Data.GroupBy)
			require.EqualValues(t, 300, envelope.Data.Coverage.BucketSeconds)
			require.False(t, envelope.Data.Coverage.RequestedStart.IsZero())
			require.True(t, envelope.Data.Coverage.RequestedEnd.After(envelope.Data.Coverage.RequestedStart))
		})
	}
}

func TestChannelMonitorV1MatrixHandlerRejectsInvalidRange(t *testing.T) {
	h := NewChannelMonitorV1MatrixHandler(nil, newChannelMonitorV1MatrixSettings(true, service.ChannelMonitorModeV1))
	for _, token := range []string{"", "90m", "24h", "7d", "30d", "30m", "9999d-1s"} {
		t.Run("range="+token, func(t *testing.T) {
			recorder, envelope := runChannelMonitorV1MatrixHandler(
				t, h, "/channel-monitors/matrix?range="+token+"&group_by=platform_group",
			)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Equal(t, http.StatusBadRequest, envelope.Code)
			// 与 V2 handler 对 ErrChannelMonitorV2InvalidRange 的处理完全同款（错误文案一致）。
			require.Contains(t, envelope.Message, "invalid channel monitor v2 range")
		})
	}
}

func TestChannelMonitorV1MatrixHandlerRejectsUnsupportedGroupBy(t *testing.T) {
	h := NewChannelMonitorV1MatrixHandler(nil, newChannelMonitorV1MatrixSettings(true, service.ChannelMonitorModeV1))

	// 非法值走官方 ParseChannelMonitorV2GroupBy 的报错。
	recorder, _ := runChannelMonitorV1MatrixHandler(t, h, "/channel-monitors/matrix?range=24h-5m&group_by=nope")
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid channel monitor v2 group_by")

	// 合法但 V1 无法如实表达的分组，明确 400（不静默换维度）。
	for _, groupBy := range []string{"platform", "platform_model", "platform_group_model"} {
		t.Run(groupBy, func(t *testing.T) {
			recorder, _ := runChannelMonitorV1MatrixHandler(
				t, h, "/channel-monitors/matrix?range=24h-5m&group_by="+groupBy,
			)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Contains(t, recorder.Body.String(), "unsupported group_by for channel monitor v1 matrix")
		})
	}

	// 缺省 group_by 等价于 platform_group。
	monitors := &channelMonitorV1MatrixHandlerMonitorsStub{}
	okHandler := NewChannelMonitorV1MatrixHandler(
		service.NewChannelMonitorV1MatrixService(monitors, &channelMonitorV1MatrixHandlerRepoStub{}),
		nil,
	)
	recorder, envelope := runChannelMonitorV1MatrixHandler(t, okHandler, "/channel-monitors/matrix?range=24h-5m")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "platform_group", envelope.Data.GroupBy)
}

// ---------- 正常路径（同时打印真实 JSON 供人工核对同形性） ----------

func TestChannelMonitorV1MatrixHandlerHappyPath(t *testing.T) {
	accountID := int64(42)
	// 故意同时带上 account_id 与自由文本 group_name 标签：它们**都不再参与**
	// 分组归属判定，只有显式绑定的 GroupID 说了算（旧的三级降级猜测已下线）。
	groupID := int64(7)
	monitors := &channelMonitorV1MatrixHandlerMonitorsStub{monitors: []*service.ChannelMonitor{
		{ID: 1, Name: "GPT 主力渠道", Provider: "openai", AccountID: &accountID, GroupName: "主力分组", GroupID: &groupID, SortOrder: 1},
		{ID: 2, Name: "Grok 备用渠道", Provider: "grok", SortOrder: 2},
	}}
	now := time.Now().UTC()
	// 4 次真实探测：3 次 operational + 1 次 degraded。行级口径（**degraded 算成功**）：
	// sample=4、success_rate=1.0、score=100；但档位仍然是 health=warning
	// —— Overall() 的口径没变（有 degraded 就是黄色柱子），变的只有成功率统计。
	// 每次探测各带一个 300ms 延迟样本。
	base := now.Truncate(time.Second)
	repo := &channelMonitorV1MatrixHandlerRepoStub{
		points: map[int64][]service.ChannelMonitorV1HistoryPoint{
			1: {
				{MonitorID: 1, CheckedAt: base.Add(-4 * time.Minute), Status: "operational", LatencyMs: channelMonitorV1MatrixHandlerLatency(300)},
				{MonitorID: 1, CheckedAt: base.Add(-3 * time.Minute), Status: "operational", LatencyMs: channelMonitorV1MatrixHandlerLatency(300)},
				{MonitorID: 1, CheckedAt: base.Add(-2 * time.Minute), Status: "operational", LatencyMs: channelMonitorV1MatrixHandlerLatency(300)},
				{MonitorID: 1, CheckedAt: base.Add(-time.Minute), Status: "degraded", LatencyMs: channelMonitorV1MatrixHandlerLatency(300)},
			},
		},
		// 窗口计数：口径与上面 4 根柱子一致（3 operational + 1 degraded）。
		// 行级的成功率 / 健康分**只认这一路**，不再从 buckets 汇总。
		counts: map[int64]service.ChannelMonitorV1StatusCounts{
			1: {TotalChecks: 4, Operational: 3, Degraded: 1},
		},
		coverage: map[int64]service.ChannelMonitorV1CoverageBounds{
			1: {MinCheckedAt: now.Add(-2 * time.Hour), MaxCheckedAt: now.Add(-30 * time.Second)},
		},
		// 只需声明「id=7 这个分组仍在架」；归属本身来自监控的 GroupID。
		liveGroups: service.ChannelMonitorV1LiveGroupIDs{7: {}},
	}
	h := NewChannelMonitorV1MatrixHandler(
		service.NewChannelMonitorV1MatrixService(monitors, repo),
		newChannelMonitorV1MatrixSettings(true, service.ChannelMonitorModeV1),
	)

	recorder, envelope := runChannelMonitorV1MatrixHandler(
		t, h, "/channel-monitors/matrix?range=30m-1m&group_by=platform_group",
	)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 0, envelope.Code)
	require.Equal(t, "platform_group", envelope.Data.GroupBy)
	require.Len(t, envelope.Data.Items, 2)

	first := envelope.Data.Items[0]
	require.Equal(t, "openai", first.Platform)
	require.NotNil(t, first.GroupID)
	require.EqualValues(t, 7, *first.GroupID)
	require.Equal(t, "GPT 主力渠道", first.GroupName)
	require.Empty(t, first.Model)
	require.Equal(t, "warning", first.Health.Overall, "有 degraded → 档位仍是 warning（Overall 口径未变）")
	require.NotNil(t, first.Health.Score)
	require.InDelta(t, 100, *first.Health.Score, 1e-9, "degraded 算成功：100 * (3+1)/4 = 100")
	// 用户端（admin=false）脱敏：绝对量为 0，比率保留。
	require.Zero(t, first.Metrics.RequestCount)
	require.InDelta(t, 1, first.Metrics.SuccessRate, 1e-9, "degraded 算成功 ⇒ success_rate=1.0")
	// 曲线 = 窗口内的探测明细：4 次探测 ⇒ 4 个点，bucket_start 就是这次探测的 checked_at，
	// 没有探测的时间段一个点都不补 —— 所以曲线上不会出现 unknown（灰色「样本不足」已消失）。
	require.Len(t, first.Buckets, 4, "一次探测一个点，不再按时间桶补空柱子")
	for i, bucket := range first.Buckets {
		require.Equal(t, base.Add(time.Duration(i-4)*time.Minute), bucket.BucketStart)
		require.NotEqual(t, "unknown", bucket.Health.Overall)
		require.NotNil(t, bucket.Health.Score)
	}
	require.Equal(t, "healthy", first.Buckets[0].Health.Overall)
	require.Equal(t, "warning", first.Buckets[3].Health.Overall)

	second := envelope.Data.Items[1]
	require.Nil(t, second.GroupID, "三级降级全落空时不带 group_id")
	require.Empty(t, second.Buckets, "窗口内零探测 ⇒ 空曲线，不补 unknown 灰柱")
	require.Equal(t, "unknown", second.Health.Overall)
	require.Nil(t, second.Health.Score, "无样本 health.score 必须是 null")
	require.NotContains(t, recorder.Body.String(), `"group_id":null`, "nil group_id 必须整个字段省略")

	// 真实响应样例（-v 时可读）：buckets 截断到前 2 个，避免日志刷屏；
	// 其余字段与线上返回逐字一致，供人工核对与 V2 matrix 的同形性。
	sample := compactChannelMonitorV1MatrixSample(t, recorder.Body.Bytes(), 2)
	t.Logf("GET /api/v1/channel-monitors/matrix?range=30m-1m&group_by=platform_group\n%s", sample)
}

// compactChannelMonitorV1MatrixSample 把真实响应里的每个 item 的 buckets 截断到前 limit 个后再格式化，
// 仅用于测试日志（不改动被测代码路径）。
func compactChannelMonitorV1MatrixSample(t *testing.T, raw []byte, limit int) string {
	t.Helper()
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(raw, &envelope))
	data, ok := envelope["data"].(map[string]any)
	require.True(t, ok)
	items, ok := data["items"].([]any)
	require.True(t, ok)
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		require.True(t, ok)
		buckets, ok := item["buckets"].([]any)
		require.True(t, ok)
		if len(buckets) > limit {
			item["buckets"] = buckets[:limit]
		}
	}
	out, err := json.MarshalIndent(envelope, "", "  ")
	require.NoError(t, err)
	return string(out)
}

// TestChannelMonitorV1MatrixHandlerNilServiceFailsLoudly 未接线时不要 panic，返回 500。
func TestChannelMonitorV1MatrixHandlerNilServiceFailsLoudly(t *testing.T) {
	h := NewChannelMonitorV1MatrixHandler(nil, nil)
	recorder, envelope := runChannelMonitorV1MatrixHandler(t, h, "/channel-monitors/matrix?range=24h-5m&group_by=platform_group")
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Equal(t, http.StatusInternalServerError, envelope.Code)
}
