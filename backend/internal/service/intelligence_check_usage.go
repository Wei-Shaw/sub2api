package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

// 智力检测跑测要「留下的消费」全部落在这里。
//
// 背景：跑测走的是账号连通性测试那条内存探针，官方路径从设计上就不写使用记录
// （连通性测试、定时巡检、用量采样都一样）。于是它在上游真实花掉的钱既不出现在
// 「使用记录」里，也不进盈亏。这个文件补上那一环：
//
//   - 探针在发起跑测前装一个 capture，平台测试拿到上游响应时把响应头里的
//     上游请求 ID 记进去——这正是后台 A6 取数任务按 ID 反查真实扣费的输入；
//   - 跑测结束后按「一个上游请求一行」写 usage_logs，费用与成本刻意留 0，
//     由 A6 取数任务把上游真实扣费回填到「成本」列（纯成本口径：没有向任何人收费）。
//
// 归属（user_id / api_key_id）不是可有可无的：usage_logs 这两列是 NOT NULL
// 且带外键，插不了"无主"行。主人指定挂在管理员名下，因此这里取第一个可用管理员，
// 并挂一个自动创建的记账专用 Key（disabled，不参与认证）。
const (
	// intelligenceCheckUsageInboundEndpoint 是这类记账行在使用记录里的 inbound_endpoint 标记。
	// 便于在「使用记录」里一眼过滤出智力检测产生的消费。
	intelligenceCheckUsageInboundEndpoint = "internal://intelligence-check"

	// intelligenceCheckUsageKeyName 是自动创建的记账专用 API Key 名。
	//
	// 短名是为了在 Key 列表里不挤占列宽；但**名字被运行时精确匹配**，
	// 改名必须同时登记旧名（见 resolveInternalUsageAttribution），
	// 否则系统会把它当成新 Key 再建一个。删它会级联带走这些使用记录，
	// 所以维护时仍需谨慎。
	intelligenceCheckUsageKeyName = "智力检测·系统"

	// intelligenceCheckUsageKeyLegacyName 是本 Key 历史上的长名，仅供查找兜底：
	// 库里既有的那条 Key 仍叫这个名字，且它挂着全部历史记账行。
	intelligenceCheckUsageKeyLegacyName = "智力检测记账专用Key（系统自动创建，请勿删除）"

	intelligenceCheckUsageLogComponent = "service.intelligence_check_usage"

	// intelligenceCheckCaptureContextKey 是挂在本次跑测 gin.Context 上的捕获器键。
	intelligenceCheckCaptureContextKey = "sub2api_intelligence_check_upstream_capture"
)

// intelligenceCheckUpstreamCapture 收集一次跑测里**所有**上游调用的响应信息。
//
// 为什么要收集一组而不是一个：CN 自适应账号的一轮跑测会连打 Chat Completions、
// Anthropic、Responses 三个原生端点，每次都是真扣费。只记一个请求 ID 会漏掉其余账。
//
// responded 与 requestIDs 分开记：收到响应与"上游是否在响应头里给了请求 ID"是两回事。
// 装了捕获却拿不到 ID（账号没配 upstream_request_id_header）时，我们仍要留下一条
// 使用记录（成本待补），但绝不能凭空编一个 ID 去查账单。
type intelligenceCheckUpstreamCapture struct {
	mu         sync.Mutex
	responded  bool
	requestIDs []string
	// firstContentAt 是首个 content 事件到达的时刻（只记第一次）。
	// 智力检测是流式跑测，用它算「首字」；非流式跑测不采信（那只是响应结束时刻）。
	firstContentAt time.Time
	// usage 是上游响应里解析出的 token 用量（输入/输出/缓存），记账行填写。
	usage ProbeUsageTokens
}

// withIntelligenceCheckUpstreamCapture 把捕获器挂到本次跑测的 gin.Context 上。
// 只有智力检测跑测会调用；官方连通性测试没有捕获器，因此相关逻辑全部退化为空操作。
func withIntelligenceCheckUpstreamCapture(c *gin.Context, capture *intelligenceCheckUpstreamCapture) {
	if c == nil || capture == nil {
		return
	}
	c.Set(intelligenceCheckCaptureContextKey, capture)
}

func intelligenceCheckUpstreamCaptureFrom(c *gin.Context) *intelligenceCheckUpstreamCapture {
	if c == nil {
		return nil
	}
	value, ok := c.Get(intelligenceCheckCaptureContextKey)
	if !ok {
		return nil
	}
	capture, _ := value.(*intelligenceCheckUpstreamCapture)
	return capture
}

// captureIntelligenceCheckUpstreamRequestID 在平台测试刚拿到上游响应时调用。
//
// 这是唯一一处需要插进官方测试路径的钩子，因此刻意做成"没有捕获器就立即返回"：
// 官方连通性测试、定时巡检与用量采样的行为逐字不变。
func captureIntelligenceCheckUpstreamRequestID(c *gin.Context, account *Account, header http.Header) {
	captureIntelligenceCheckUpstreamRequestIDForAccounts(c, header, account)
}

// captureIntelligenceCheckUpstreamRequestIDForAccounts 是上面的多账号版本。
//
// 存在的理由：凭据影子账号会先解析出真实凭据账号再出站，而
// upstream_request_id_header 配在哪一层并不固定，两个都读一遍最省事——
// 读不到就都读不到，读到哪个都算数。
func captureIntelligenceCheckUpstreamRequestIDForAccounts(c *gin.Context, header http.Header, accounts ...*Account) {
	capture := intelligenceCheckUpstreamCaptureFrom(c)
	if capture == nil {
		return
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.responded = true
	for _, account := range accounts {
		ptr := usageUpstreamRequestIDPtr(account, header, false)
		if ptr == nil {
			continue
		}
		id := strings.TrimSpace(*ptr)
		if id == "" {
			continue
		}
		already := false
		for _, existing := range capture.requestIDs {
			if existing == id {
				already = true
				break
			}
		}
		if !already {
			capture.requestIDs = append(capture.requestIDs, id)
		}
	}
}

// snapshot 返回「是否收到过上游响应」与去重后的上游请求 ID 列表。
func (c *intelligenceCheckUpstreamCapture) snapshot() (bool, []string) {
	if c == nil {
		return false, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.responded, append([]string(nil), c.requestIDs...)
}

// markIntelligenceCheckFirstContent 由 sendEvent 在发出第一个 content 事件时调用。
// 只有智力检测跑测装了捕获器；官方连通性测试、定时巡检与用量采样都没装，
// 所以这个钩子对它们恒为空操作。
func markIntelligenceCheckFirstContent(c *gin.Context) {
	capture := intelligenceCheckUpstreamCaptureFrom(c)
	if capture == nil {
		return
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if capture.firstContentAt.IsZero() {
		capture.firstContentAt = time.Now()
	}
}

// firstContentTimestamp 返回首个 content 事件到达时刻；没等到内容时为零值。
func (c *intelligenceCheckUpstreamCapture) firstContentTimestamp() time.Time {
	if c == nil {
		return time.Time{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.firstContentAt
}

// markIntelligenceCheckUsage 由各平台解析器在解出一段上游 JSON 后调用。
// 与首字钩子同样只对装了捕获器的智力检测跑测生效，官方测试路径恒为空操作。
func markIntelligenceCheckUsage(c *gin.Context, obj map[string]any) {
	capture := intelligenceCheckUpstreamCaptureFrom(c)
	if capture == nil || len(obj) == 0 {
		return
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	captureProbeUsage(&capture.usage, obj)
}

// usageSnapshot 返回本次跑测累计到的 token 用量。
func (c *intelligenceCheckUpstreamCapture) usageSnapshot() ProbeUsageTokens {
	if c == nil {
		return ProbeUsageTokens{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.usage
}

// intelligenceCheckUsageRequestID 生成记账行的 request_id。
//
// 必须唯一：usage_logs 在 (request_id, api_key_id) 上有唯一索引，同一轮跑测的多个
// 上游请求共用同一个记账 Key，序号不进 request_id 的话第二行起会被 ON CONFLICT 吞掉。
func intelligenceCheckUsageRequestID(runID int64, index int) string {
	return fmt.Sprintf("ic-%d-%d-%d", runID, time.Now().UnixMilli(), index)
}

// resolveUsageAttribution 解析记账归属：管理员用户 + 智力检测记账专用 Key。
// 归属解析本身在 internal_usage_attribution.go，与渠道监控共用同一套。
func (s *IntelligenceCheckService) resolveUsageAttribution(ctx context.Context) (int64, int64, error) {
	attr, err := resolveInternalUsageAttribution(ctx, s.userRepo, s.apiKeyRepo,
		intelligenceCheckUsageKeyName, intelligenceCheckUsageKeyLegacyName)
	if err != nil {
		return 0, 0, err
	}
	return attr.UserID, attr.APIKeyID, nil
}

// recordRunUsage 为一次跑测写使用记录。
//
// 只在真的收到过上游响应时才写：没收到响应说明这次跑测根本没在别人家花钱，
// 记一行"0 成本"只会让使用记录变脏。收到响应但拿不到上游请求 ID 时照样写一行
// （成本列留空），这样"跑了但成本取不到"是看得见的，而不是悄悄消失。
func (s *IntelligenceCheckService) recordRunUsage(
	ctx context.Context,
	run *IntelligenceCheckRun,
	req IntelligenceCheckRequest,
	probe *IntelligenceCheckProbeResult,
) {
	if s == nil || s.usageLogRepo == nil || run == nil || probe == nil {
		return
	}
	if !probe.UpstreamResponded || run.AccountID <= 0 {
		return
	}
	model := strings.TrimSpace(run.ModelID)
	if model == "" {
		model = strings.TrimSpace(req.ModelID)
	}
	if model == "" {
		return
	}

	// 与计费落库同一套脱离请求生命周期的上下文：跑测超时/客户端断开都不该让
	// 已经产生的成本记录凭空消失。
	usageCtx, cancel := detachedBillingContext(ctx)
	defer cancel()

	userID, apiKeyID, err := s.resolveUsageAttribution(usageCtx)
	if err != nil {
		logger.LegacyPrintf(intelligenceCheckUsageLogComponent,
			"intelligence_check_usage_attribution_unavailable: run_id=%d account_id=%d err=%v",
			run.ID, run.AccountID, err)
		return
	}

	requestIDs := probe.UpstreamRequestIDs
	if len(requestIDs) == 0 {
		// 收到响应但账号没配 upstream_request_id_header：留一条无 ID 的记录，
		// 成本列会一直空着——它在页面上就是"这笔还没取到成本"。
		requestIDs = []string{""}
	}

	inboundEndpoint := intelligenceCheckUsageInboundEndpoint
	requestType := RequestTypeStream
	stream := true
	if req.DisableStream {
		requestType = RequestTypeSync
		stream = false
	}
	durationMs := int(probe.LatencyMs)
	createdAt := probe.StartedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	reasoningEffort := strings.TrimSpace(run.ReasoningEffort)
	upstreamModel := strings.TrimSpace(probe.UpstreamModel)

	for index, rawRequestID := range requestIDs {
		upstreamRequestID := strings.TrimSpace(rawRequestID)
		usageLog := &UsageLog{
			UserID:              userID,
			APIKeyID:            apiKeyID,
			AccountID:           run.AccountID,
			RequestID:           intelligenceCheckUsageRequestID(run.ID, index),
			Model:               model,
			RequestedModel:      model,
			InboundEndpoint:     &inboundEndpoint,
			FirstTokenMs:        probe.FirstTokenMs,
			InputTokens:         probe.Usage.Input,
			OutputTokens:        probe.Usage.Output,
			CacheCreationTokens: probe.Usage.CacheCreation,
			CacheReadTokens:     probe.Usage.CacheRead,
			// 费用与成本都留 0：没有向任何人收费，真实成本由 A6 反查回填到成本列。
			TotalCost:      0,
			ActualCost:     0,
			RateMultiplier: 1,
			BillingType:    BillingTypeBalance,
			RequestType:    requestType,
			Stream:         stream,
			DurationMs:     &durationMs,
			CreatedAt:      createdAt,
		}
		if upstreamRequestID != "" {
			usageLog.UpstreamRequestID = &upstreamRequestID
		}
		if upstreamModel != "" {
			usageLog.UpstreamModel = &upstreamModel
		}
		if reasoningEffort != "" {
			usageLog.ReasoningEffort = &reasoningEffort
		}
		writeUsageLogBestEffort(usageCtx, s.usageLogRepo, usageLog, intelligenceCheckUsageLogComponent)
	}
}
