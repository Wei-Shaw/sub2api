package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// 渠道监控「直连上游」探针的记账。
//
// 背景：探针默认打在本地网关（endpoint 是本站域名、Key 是本站 Key）时，
// 账由网关自己记；一旦把 endpoint/Key 换成上游直连（例如 A6），探针就绕过了网关，
// 于是「后台使用记录」里看不到这次消费，成本/费用/首字/总耗时全都无从谈起。
//
// 这个文件补上那一环，且只补「绕过网关」的那部分：
//   - 探针的 Key 能在本站 api_keys 里查到 → 走的是本地网关 → 跳过，不重复记；
//   - 查不到（上游 Key）→ 记一条 use_log 行。
//
// 归属：group 取监控绑定的分组；account 由「绑定分组 → 可调度账号」解析
// （与真实请求按分组选账号是同一套关系）；user/key 复用内部探针的
// 管理员 + disabled 专用 Key（见 internal_usage_attribution.go）。
const (
	// ChannelMonitorUsageInboundEndpoint 是这类记账行在使用记录里的 inbound_endpoint 标记。
	ChannelMonitorUsageInboundEndpoint = "internal://channel-monitor"

	// channelMonitorUsageKeyName 是渠道监控记账专用 API Key 名。
	//
	// 起得短是因为它只在「API 密钥」列表里露脸，长了会挤占列宽；但**名字是被
	// 运行时精确匹配的**（见 resolveInternalUsageAttribution），所以改名必须
	// 同时在下面登记旧名，否则升级那一刻会被判定为「不存在」而重复建一个。
	channelMonitorUsageKeyName = "渠道监控·系统"

	// channelMonitorUsageKeyLegacyName 是本 Key 历史上的长名。
	//
	// 仅供查找兜底：库里既有的那条 Key 仍叫这个名字，且它挂着全部历史记账行
	// （usage_logs.api_key_id 指向它），不能弃用重来。运维改名后本常量可保留，
	// 留着对「尚未完成改名的实例」也是安全的。
	channelMonitorUsageKeyLegacyName = "渠道监控记账专用Key（系统自动创建，请勿删除）"

	channelMonitorUsageLogComponent = "service.channel_monitor_usage"
)

// ChannelMonitorUsageRecorder 为直连上游的渠道监控探针补记使用记录。
// 依赖缺席时所有方法都安全地变成空操作，探针本身照常跑。
type ChannelMonitorUsageRecorder struct {
	usageLogRepo UsageLogRepository
	userRepo     UserRepository
	apiKeyRepo   APIKeyRepository
	accountRepo  AccountRepository
}

// NewChannelMonitorUsageRecorder 构造记账器。构造阶段不做 IO。
func NewChannelMonitorUsageRecorder(
	usageLogRepo UsageLogRepository,
	userRepo UserRepository,
	apiKeyRepo APIKeyRepository,
	accountRepo AccountRepository,
) *ChannelMonitorUsageRecorder {
	return &ChannelMonitorUsageRecorder{
		usageLogRepo: usageLogRepo,
		userRepo:     userRepo,
		apiKeyRepo:   apiKeyRepo,
		accountRepo:  accountRepo,
	}
}

// monitorProbeUsage 描述一次「直连上游」探针的记账上下文。
// 探针发出前解析好，跑完直接用来写行。
type monitorProbeUsage struct {
	accountID int64
	// requestIDHeader 来自归属账号的 extra.upstream_request_id_header，
	// 是抓上游请求 ID（A6 反查成本的输入）的头名。
	requestIDHeader string
}

// prepareProbeUsage 在探针发出前判定这次是否要记账，并解析归属账号与请求头名。
// 返回 nil 表示这次不记账（本地探针 / 没绑分组 / 没解析到账号）。
func (r *ChannelMonitorUsageRecorder) prepareProbeUsage(ctx context.Context, m *ChannelMonitor) *monitorProbeUsage {
	if r == nil || r.usageLogRepo == nil || r.accountRepo == nil || m == nil {
		return nil
	}
	if m.GroupID == nil || *m.GroupID <= 0 {
		// 没绑分组就没有「分组 → 账号」这条路，也就无从落 account_id（NOT NULL 外键）。
		return nil
	}
	key := strings.TrimSpace(m.APIKey)
	if key == "" {
		return nil
	}
	if r.isLocalAPIKey(ctx, key) {
		// 探针走的是本地网关，网关照常记账，这里必须跳过，否则一笔消费记两遍。
		return nil
	}
	accounts, err := r.accountRepo.ListSchedulableByGroupID(ctx, *m.GroupID)
	if err != nil || len(accounts) == 0 {
		logger.LegacyPrintf(channelMonitorUsageLogComponent,
			"channel_monitor_usage_account_unresolved: monitor_id=%d group_id=%d err=%v",
			m.ID, *m.GroupID, err)
		return nil
	}
	account := accounts[0]
	return &monitorProbeUsage{
		accountID:       account.ID,
		requestIDHeader: UpstreamRequestIDHeaderName(&account),
	}
}

func (r *ChannelMonitorUsageRecorder) isLocalAPIKey(ctx context.Context, key string) bool {
	if r.apiKeyRepo == nil {
		return false
	}
	local, err := r.apiKeyRepo.GetByKey(ctx, key)
	return err == nil && local != nil && local.ID > 0
}

// record 写使用记录。
//
// 只记 2xx 的探针：非 2xx 与网络失败基本没有产生上游消费，全部记进来只会让
// 使用记录被监控的失败重试刷屏。收到 2xx 但拿不到上游请求 ID 时照样写一行
// （成本列留空），让「跑了但成本取不到」是看得见的。
func (r *ChannelMonitorUsageRecorder) record(ctx context.Context, m *ChannelMonitor, results []*CheckResult, probe *monitorProbeUsage) {
	if r == nil || r.usageLogRepo == nil || m == nil || probe == nil || probe.accountID <= 0 || len(results) == 0 {
		return
	}
	usageCtx, cancel := detachedBillingContext(ctx)
	defer cancel()

	attr, err := resolveInternalUsageAttribution(usageCtx, r.userRepo, r.apiKeyRepo,
		channelMonitorUsageKeyName, channelMonitorUsageKeyLegacyName)
	if err != nil {
		logger.LegacyPrintf(channelMonitorUsageLogComponent,
			"channel_monitor_usage_attribution_unavailable: monitor_id=%d err=%v", m.ID, err)
		return
	}

	wrote := 0
	for index, res := range results {
		if res == nil || res.StatusCode < 200 || res.StatusCode >= 300 {
			continue
		}
		model := strings.TrimSpace(res.Model)
		if model == "" {
			continue
		}
		inboundEndpoint := ChannelMonitorUsageInboundEndpoint
		requestType := RequestTypeSync
		stream := false
		if res.Stream {
			requestType = RequestTypeStream
			stream = true
		}
		var durationMs *int
		if res.LatencyMs != nil {
			value := *res.LatencyMs
			durationMs = &value
		}
		createdAt := res.CheckedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}

		usageLog := &UsageLog{
			UserID:          attr.UserID,
			APIKeyID:        attr.APIKeyID,
			AccountID:       probe.accountID,
			GroupID:         m.GroupID,
			RequestID:       channelMonitorUsageRequestID(m.ID, index),
			Model:           model,
			RequestedModel:  model,
			InboundEndpoint: &inboundEndpoint,
			// 纯成本口径：没有向任何人收费；真实成本由 A6 按上游请求 ID 反查回填。
			TotalCost:           0,
			ActualCost:          0,
			RateMultiplier:      1,
			BillingType:         BillingTypeBalance,
			RequestType:         requestType,
			Stream:              stream,
			DurationMs:          durationMs,
			FirstTokenMs:        res.FirstTokenMs,
			InputTokens:         res.Usage.Input,
			OutputTokens:        res.Usage.Output,
			CacheCreationTokens: res.Usage.CacheCreation,
			CacheReadTokens:     res.Usage.CacheRead,
			CreatedAt:           createdAt,
		}
		if requestID := strings.TrimSpace(res.UpstreamRequestID); requestID != "" {
			usageLog.UpstreamRequestID = &requestID
		}
		// 记录真实打到的上游端点：使用记录页的「上游」列读的就是它。
		// 探针按模板直连原生协议端点，所以这里填的是真实协议（chat/completions/
		// responses/messages），不是推断值；留空会让该列显示成「-」，
		// 管理员无法判断这次探针到底验的是哪条协议链路。
		if endpoint := strings.TrimSpace(res.UpstreamEndpoint); endpoint != "" {
			usageLog.UpstreamEndpoint = &endpoint
		}
		writeUsageLogBestEffort(usageCtx, r.usageLogRepo, usageLog, channelMonitorUsageLogComponent)
		wrote++
	}
	if wrote > 0 {
		logger.LegacyPrintf(channelMonitorUsageLogComponent,
			"channel_monitor_usage_recorded: monitor_id=%d rows=%d", m.ID, wrote)
	}
}

// channelMonitorUsageRequestID 生成记账行的 request_id。
// usage_logs 在 (request_id, api_key_id) 上有唯一索引，同一轮多个模型的探针
// 共用同一个记账 Key，序号不进 request_id 的话第二行起会被 ON CONFLICT 吞掉。
func channelMonitorUsageRequestID(monitorID int64, index int) string {
	return fmt.Sprintf("cm-%d-%d-%d", monitorID, time.Now().UnixMilli(), index)
}
