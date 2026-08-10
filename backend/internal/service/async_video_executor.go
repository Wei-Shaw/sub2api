package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apiz"
	"github.com/Wei-Shaw/sub2api/internal/pkg/atlascloud"
	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	defaultAsyncVideoPollInterval = 2 * time.Second
	defaultAsyncVideoFailTimeout  = 30 * time.Minute
	// defaultAutoDurationSeconds 在客户端传 duration="auto" 或缺失时用作
	// 预扣（冻结）秒数：即前端"预估费用"里 duration="auto" 的估算口径必须
	// 与它保持一致，否则会出现"UI 提示预估 X，但后端预扣 Y"的错位。
	// 完成时会按上游返回的实际时长重算 finalCost 并追扣/退还差额。
	defaultAutoDurationSeconds = 10
)

// AsyncVideoService 视频异步任务执行内核。
//
// 相较于 AsyncMediaService（图片）的差异：
//   - 请求 body 原样透传（RequestPayload），不做协议转换
//   - 结果 payload 原样透传（ResultPayload），返回给客户端时保留 fal 原始字段
//   - 计费维度：resolution × duration_seconds × price_per_second
//
// 定价来源：走通用的渠道定价体系（ModelPricingResolver），BillingMode=video 时，
// Intervals[].TierLabel 存分辨率（如 720p/480p），PerRequestPrice 语义为
// 每秒单价（USD/s）；PerRequestPrice 兜底为默认每秒单价（分辨率未命中时使用）。
type AsyncVideoService struct {
	taskRepo               AsyncVideoTaskRepository
	userRepo               UserRepository
	billing                *BillingService
	pricingResolver        *ModelPricingResolver
	deferred               *DeferredService
	billingContextResolver *BillingContextResolver
	balanceCache           interface {
		InvalidateUserBalance(ctx context.Context, userID int64) error
	}
	// costCenter：成本中心写入器。与 Gateway/OpenAI 网关的羊毛出在羊身上另行记账（income+upstream）
	// 一致，让视频任务的实际支出也能转进 cost_center_events。nil 时静默跳过（向后兼容禁用场景）。
	costCenter CostCenterWriter

	// cosService：视频产物 COS 转存器。nil 或未启用时全程 no-op，直接返回上游原始 URL。
	cosService *COSImageTransferService

	pollInterval time.Duration
	failTimeout  time.Duration
}

// NewAsyncVideoService 创建视频执行内核。
func NewAsyncVideoService(
	taskRepo AsyncVideoTaskRepository,
	userRepo UserRepository,
	billing *BillingService,
) *AsyncVideoService {
	return &AsyncVideoService{
		taskRepo:     taskRepo,
		userRepo:     userRepo,
		billing:      billing,
		pollInterval: defaultAsyncVideoPollInterval,
		failTimeout:  defaultAsyncVideoFailTimeout,
	}
}

// SetBalanceCache 注入余额缓存失效器。
func (s *AsyncVideoService) SetBalanceCache(cache interface {
	InvalidateUserBalance(ctx context.Context, userID int64) error
}) {
	if s != nil {
		s.balanceCache = cache
	}
}

// SetBillingContextResolver 注入付款上下文解析器（组织/个人余额）。
func (s *AsyncVideoService) SetBillingContextResolver(resolver *BillingContextResolver) {
	if s != nil {
		s.billingContextResolver = resolver
	}
}

// SetPricingResolver 注入渠道视频定价解析器。
//
// 未注入时 SubmitAsync 将以 402 拒绝所有视频提交（Q-A：A 方案）。
func (s *AsyncVideoService) SetPricingResolver(r *ModelPricingResolver) {
	if s != nil {
		s.pricingResolver = r
	}
}

// SetCostCenterWriter 注入成本中心写入器。nil 不会引发 panic，仅则会跳过写入。
func (s *AsyncVideoService) SetCostCenterWriter(w CostCenterWriter) {
	if s != nil {
		s.costCenter = w
	}
}

// SetCOSTransferService 注入视频 COS 转存器。nil 时保持 no-op，不会引发 panic。
// 任务成功结算时会尝试把上游视频 URL 转存到 COS并替换进结果 payload/video_urls；
// 转存失败项保留上游原始 URL 兕底。
func (s *AsyncVideoService) SetCOSTransferService(c *COSImageTransferService) {
	if s != nil {
		s.cosService = c
	}
}

// SetDeferredService 注入延迟批量更新服务，用于记录账号 last_used_at。
func (s *AsyncVideoService) SetDeferredService(d *DeferredService) {
	s.deferred = d
}

// SetPollInterval 配置轮询间隔。
func (s *AsyncVideoService) SetPollInterval(d time.Duration) {
	if d > 0 {
		s.pollInterval = d
	}
}

// SetFailTimeout 配置任务强制判失时间。
func (s *AsyncVideoService) SetFailTimeout(d time.Duration) {
	if d > 0 {
		s.failTimeout = d
	}
}

// FailTimeout 返回当前失败兜底时间。
func (s *AsyncVideoService) FailTimeout() time.Duration { return s.failTimeout }

// AsyncVideoSubmitInput 提交视频任务的入参。
type AsyncVideoSubmitInput struct {
	Account *Account
	User    *User

	APIKeyID  int64
	UserID    int64
	AccountID int64
	GroupID   *int64
	ChannelID *int64

	Facade            string
	InternalRequestID string
	RequestedModel    string // 客户端请求的 fal slug（模型别名映射后）
	UpstreamModel     string // 实际转发到 fal 的 slug（通常与 RequestedModel 相同）

	// 请求 payload 原样透传给 fal 上游（客户端提交的完整 body）。
	RequestPayload map[string]any

	// 计费维度（从 RequestPayload 提取）。
	Resolution      string
	DurationSeconds int
	AspectRatio     string

	BillingType    int8
	RateMultiplier float64

	ClientIP        string
	UserAgent       string
	InboundEndpoint string
}

// ErrAsyncVideoPending 表示伪同步等待超时（任务未终结，reconciler 兜底）。
var ErrAsyncVideoPending = errors.New("async video task still pending")

// SubmitAsync 提交视频任务：定价校验 → 预扣费 → 落库 pending → submit → running。
func (s *AsyncVideoService) SubmitAsync(ctx context.Context, in *AsyncVideoSubmitInput) (*AsyncVideoTask, error) {
	if in == nil {
		return nil, errors.New("nil async video submit input")
	}
	if in.Account == nil {
		return nil, errors.New("async video: account is required")
	}
	if in.RateMultiplier == 0 {
		in.RateMultiplier = 1
	}
	upstreamModel := strings.TrimSpace(in.UpstreamModel)
	if upstreamModel == "" {
		upstreamModel = in.RequestedModel
	}

	resolution := normalizeVideoResolution(in.Resolution)
	duration := in.DurationSeconds
	// duration=0 表示客户端传了 duration="auto" 或缺失（部分视频模型允许），
	// 此时用兜底秒数预扣，成功后按上游 result 里的实际时长重算差额。
	billableDuration := duration
	autoDuration := false
	if billableDuration <= 0 {
		billableDuration = defaultAutoDurationSeconds
		autoDuration = true
	}

	// 通过渠道定价解析每秒单价：
	//   - 按 (GroupID, RequestedModel) 走 ModelPricingResolver.Resolve；
	//   - Mode 必须为 BillingModeVideo；
	//   - 优先按 resolution 匹配 Intervals[].TierLabel 取 PerRequestPrice（USD/s）；
	//   - 未命中档位则回退到 DefaultPerRequestPrice；
	//   - 二者都为 0 → 402 拒绝（Q-A: A 方案，防止 0 费漏计）。
	if s.pricingResolver == nil {
		return nil, fmt.Errorf("async video: pricing resolver unavailable")
	}
	var groupID *int64
	if in.GroupID != nil {
		gid := *in.GroupID
		groupID = &gid
	}
	resolved := s.pricingResolver.Resolve(ctx, PricingInput{
		Model:   in.RequestedModel,
		GroupID: groupID,
	})
	if resolved == nil || resolved.Mode != BillingModeVideo {
		return nil, fmt.Errorf("async video: no video pricing configured for model %q in current group", in.RequestedModel)
	}
	unitPrice := s.pricingResolver.GetRequestTierPrice(resolved, resolution)
	if unitPrice <= 0 {
		unitPrice = resolved.DefaultPerRequestPrice
	}
	if unitPrice <= 0 {
		return nil, fmt.Errorf("async video: no video pricing configured for model %q resolution %q", in.RequestedModel, resolution)
	}
	heldCost := unitPrice * float64(billableDuration) * in.RateMultiplier
	if heldCost <= 0 {
		return nil, fmt.Errorf("async video: computed cost must be > 0 (unit=%.6f duration=%d)", unitPrice, billableDuration)
	}
	billingContext := &BillingContext{ConsumerUserID: in.UserID, PayerUserID: in.UserID, BalanceSource: BalanceSourceSelf}
	if s.billingContextResolver != nil {
		var err error
		billingContext, err = s.billingContextResolver.ResolveForAmount(ctx, in.UserID, heldCost)
		if err != nil {
			return nil, fmt.Errorf("async video: resolve payer: %w", err)
		}
	}
	if err := s.charge(ctx, in.BillingType, billingContext, heldCost); err != nil {
		return nil, fmt.Errorf("async video: pre-charge: %w", err)
	}

	failDeadline := time.Now().Add(s.failTimeout)
	// task.DurationSeconds：非 auto 直接落客户端传入值；auto 先记 0，
	// 待 markSucceeded 时用上游返回的实际时长回填。
	storedDuration := duration
	if autoDuration {
		storedDuration = 0
	}
	task := &AsyncVideoTask{
		InternalRequestID: in.InternalRequestID,
		APIKeyID:          in.APIKeyID,
		UserID:            in.UserID,
		OrganizationID:    billingContext.OrganizationID,
		PayerUserID:       amInt64Ptr(billingContext.PayerUserID),
		BalanceSource:     amStrPtr(billingContext.BalanceSource),
		AuthzGeneration:   amInt64Ptr(billingContext.AuthzGeneration),
		AccountID:         amOptInt64(in.AccountID),
		GroupID:           in.GroupID,
		ChannelID:         in.ChannelID,
		Facade:            in.Facade,
		RequestedModel:    in.RequestedModel,
		UpstreamModel:     amStrPtr(upstreamModel),
		Resolution:        amStrPtr(resolution),
		DurationSeconds:   storedDuration,
		AspectRatio:       amStrPtr(in.AspectRatio),
		Status:            AsyncVideoStatusPending,
		HeldCost:          heldCost,
		RateMultiplier:    in.RateMultiplier,
		UnitPriceSnapshot: unitPrice,
		RequestPayload:    in.RequestPayload,
		FailDeadlineAt:    &failDeadline,
		ClientIP:          amStrPtr(in.ClientIP),
		UserAgent:         amStrPtr(in.UserAgent),
		InboundEndpoint:   amStrPtr(in.InboundEndpoint),
		UpstreamEndpoint:  amStrPtr(falUpstreamEndpoint(upstreamModel)),
	}
	if err := s.taskRepo.Create(ctx, task); err != nil {
		s.refund(ctx, in.BillingType, billingContext, heldCost)
		return nil, fmt.Errorf("async video: create task: %w", err)
	}

	client, err := s.newClient(in.Account)
	if err != nil {
		s.markFailedAndRefund(ctx, task, in.BillingType, "build fal client: "+err.Error())
		return task, fmt.Errorf("async video: build client: %w", err)
	}

	submitResp, err := client.SubmitRaw(ctx, upstreamModel, in.RequestPayload)
	if err != nil {
		// 任务表内部落库仍保留最原始的错误（含上游 body），便于管理员事后排障。
		s.markFailedAndRefund(ctx, task, in.BillingType, "submit: "+err.Error())
		// 对外返回时脱敏：
		//   - 若是 fal 上游的 4xx/5xx（如 403 "User is locked. Exhausted balance"），
		//     把上游品牌/余额细节隐藏，只返回统一"上游暂不可用"提示 + request_id；
		//     handler 会把这类错误映射为 502 Bad Gateway。
		//   - request_id 是 fal client 侧生成的追踪 id（fal-<hex>），可用于串联
		//     后端日志（fal_http_request_dump / fal_http_response_dump）。
		var apiErr *fal.APIError
		if errors.As(err, &apiErr) {
			return task, fmt.Errorf("async video: submit: upstream provider temporarily unavailable (request_id=%s)", apiErr.RequestID)
		}
		return task, fmt.Errorf("async video: submit: %w", err)
	}

	statusURL := submitResp.StatusURL
	if statusURL == "" {
		statusURL = client.BuildStatusURL(upstreamModel, submitResp.RequestID)
	}
	responseURL := submitResp.ResponseURL
	if responseURL == "" {
		responseURL = client.BuildResponseURL(upstreamModel, submitResp.RequestID)
	}
	if err := s.taskRepo.UpdateUpstreamRef(ctx, task.ID, submitResp.RequestID, statusURL, responseURL); err != nil {
		logger.L().Warn("async_video.update_upstream_ref_failed",
			zap.Int64("task_id", task.ID), zap.Error(err))
	}
	task.UpstreamRequestID = amStrPtr(submitResp.RequestID)
	task.StatusURL = amStrPtr(statusURL)
	task.ResponseURL = amStrPtr(responseURL)
	task.Status = AsyncVideoStatusRunning

	if s.deferred != nil {
		s.deferred.ScheduleLastUsedUpdate(in.Account.ID)
	}
	return task, nil
}

// WaitForTerminal 伪同步阻塞等待任务终态（当前实现保留，供未来 OpenAI 风格视频门面复用）。
func (s *AsyncVideoService) WaitForTerminal(ctx context.Context, task *AsyncVideoTask, account *Account, billingType int8) (*AsyncVideoTask, error) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		updated, done, err := s.pollOnce(ctx, task, account, billingType)
		if err != nil {
			return updated, err
		}
		if done {
			return updated, nil
		}
		select {
		case <-ctx.Done():
			return task, ErrAsyncVideoPending
		case <-ticker.C:
		}
	}
}

// GetTaskByInternalID 按内部请求 ID 查询任务。
func (s *AsyncVideoService) GetTaskByInternalID(ctx context.Context, internalRequestID string) (*AsyncVideoTask, error) {
	return s.taskRepo.GetByInternalRequestID(ctx, internalRequestID)
}

// GetTaskByUpstreamID 按上游 request_id 查询任务。
func (s *AsyncVideoService) GetTaskByUpstreamID(ctx context.Context, upstreamRequestID string) (*AsyncVideoTask, error) {
	return s.taskRepo.GetByUpstreamRequestID(ctx, upstreamRequestID)
}

// AdvanceTask 供原生门面 status/result 触发单轮推进。
func (s *AsyncVideoService) AdvanceTask(ctx context.Context, task *AsyncVideoTask, account *Account) (*AsyncVideoTask, bool, error) {
	if task == nil {
		return nil, false, errors.New("async video advance: nil task")
	}
	if task.IsTerminal() {
		return task, true, nil
	}
	if account == nil {
		return task, false, errors.New("async video advance: account is nil")
	}
	return s.pollOnce(ctx, task, account, BillingTypeBalance)
}

// CancelTask 取消一个在飞任务并退费。
func (s *AsyncVideoService) CancelTask(ctx context.Context, task *AsyncVideoTask, account *Account) error {
	if task == nil {
		return errors.New("async video cancel: nil task")
	}
	if task.IsTerminal() {
		return nil
	}
	if account != nil && task.UpstreamRequestID != nil {
		if client, err := s.newClient(account); err == nil {
			cancelURL := client.BuildCancelURL(amDerefStr(task.UpstreamModel), *task.UpstreamRequestID)
			if cancelErr := client.Cancel(ctx, cancelURL); cancelErr != nil {
				logger.L().Warn("async_video.cancel_upstream_failed",
					zap.Int64("task_id", task.ID), zap.Error(cancelErr))
			}
		}
	}
	s.markFailedAndRefund(ctx, task, BillingTypeBalance, "cancelled by client")
	return nil
}

// ReconcileTask reconciler 兜底推进单个未终结任务。
func (s *AsyncVideoService) ReconcileTask(ctx context.Context, task *AsyncVideoTask, account *Account) error {
	if task == nil {
		return nil
	}
	billingType := BillingTypeBalance
	if task.FailDeadlineAt != nil && time.Now().After(*task.FailDeadlineAt) {
		s.markFailedAndRefund(ctx, task, billingType, "fail deadline exceeded")
		return nil
	}
	if account == nil {
		return errors.New("async video reconcile: account is nil")
	}
	_, _, err := s.pollOnce(ctx, task, account, billingType)
	return err
}

// ListUnfinished 扫描未终结任务供 reconciler 使用。
func (s *AsyncVideoService) ListUnfinished(ctx context.Context, limit int) ([]*AsyncVideoTask, error) {
	return s.taskRepo.ListUnfinished(ctx, limit)
}

// ListByUserAndSlug 分页列出某用户在指定 slug 下的历史任务。
// slug 为空时列出该用户所有视频任务。
func (s *AsyncVideoService) ListByUserAndSlug(ctx context.Context, userID int64, slug string, offset, limit int) ([]*AsyncVideoTask, int64, error) {
	return s.taskRepo.ListByUserAndSlug(ctx, userID, slug, offset, limit)
}

// pollOnce 执行一轮状态查询并在终态时结算/退费。
func (s *AsyncVideoService) pollOnce(ctx context.Context, task *AsyncVideoTask, account *Account, billingType int8) (*AsyncVideoTask, bool, error) {
	client, err := s.newClient(account)
	if err != nil {
		return task, false, fmt.Errorf("async video poll: build client: %w", err)
	}
	statusURL := ""
	if task.StatusURL != nil {
		statusURL = *task.StatusURL
	}
	st, err := client.Status(ctx, statusURL)
	if err != nil {
		var apiErr *fal.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			s.markFailedAndRefund(ctx, task, billingType, fmt.Sprintf("status %d: %s", apiErr.StatusCode, apiErr.Body))
			return task, true, nil
		}
		return task, false, nil
	}

	if !st.IsTerminal() {
		return task, false, nil
	}

	responseURL := st.ResponseURL
	if responseURL == "" && task.ResponseURL != nil {
		responseURL = *task.ResponseURL
	}
	result, err := client.ResultRaw(ctx, responseURL)
	if err != nil {
		var apiErr *fal.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			s.markFailedAndRefund(ctx, task, billingType, fmt.Sprintf("result %d: %s", apiErr.StatusCode, apiErr.Body))
			return task, true, nil
		}
		return task, false, nil
	}

	videoURLs := fal.ExtractVideoURLs(result)
	// 视频类模型有可能返回无 url 结构（例如仅返回 base64），
	// 此时仍视为成功但 video_urls 为空，交由客户端读取原始 payload。
	// 但为了防止空数据把用户扣费，我们要求至少 result payload 非空。
	if len(result) == 0 {
		s.markFailedAndRefund(ctx, task, billingType, "upstream returned empty result")
		return task, true, nil
	}

	// 将上游视频 URL 转存到 COS，并替换 videoURLs / result payload 里的链接。
	// 转存失败项保留上游原始 URL 兕底。未启用时 no-op。
	videoURLs, result = s.transferVideosToCOS(ctx, task, videoURLs, result)

	s.markSucceeded(ctx, task, billingType, videoURLs, result)
	return task, true, nil
}

// markSucceeded 成功结算：写入结果，按上游实际时长重算 finalCost。
//
// 若上游 result 里返回了 duration（`video.duration` / 顶层 `duration` / `duration_seconds` / `num_seconds`），
// 且当前 task.DurationSeconds 与之不一致（典型场景是提交时传了 duration="auto"，task.DurationSeconds=0），
// 则以实际时长为准重算 finalCost = unitPrice × actualDuration × rate，并追扣/退还差额。
// 若 result 里没有 duration，则保留 heldCost 作为 finalCost（不追扣不退还）。
func (s *AsyncVideoService) markSucceeded(ctx context.Context, task *AsyncVideoTask, billingType int8, videoURLs []string, resultPayload map[string]any) {
	finalCost := task.HeldCost
	settleDuration := task.DurationSeconds

	actualDuration := ExtractActualDurationSeconds(resultPayload)
	if actualDuration > 0 && actualDuration != task.DurationSeconds {
		if task.UnitPriceSnapshot > 0 && task.RateMultiplier > 0 {
			recomputed := task.UnitPriceSnapshot * float64(actualDuration) * task.RateMultiplier
			if recomputed > 0 {
				finalCost = recomputed
			}
		}
		settleDuration = actualDuration
	}

	// 差额结算：正数 → 补扣；负数 → 退还。
	if delta := finalCost - task.HeldCost; delta != 0 {
		billing := asyncVideoBillingContext(task)
		if delta > 0 {
			if err := s.charge(ctx, billingType, billing, delta); err != nil {
				// 余额不足等错误：不阻塞任务落库，但记录警告，视频已生成不能退。
				logger.L().Warn("async_video.settle_extra_charge_failed",
					zap.Int64("task_id", task.ID),
					zap.Float64("delta", delta),
					zap.Int("actual_duration", actualDuration),
					zap.Error(err))
			}
		} else {
			s.refund(ctx, billingType, billing, -delta)
		}
	}

	updated, err := s.taskRepo.MarkSucceeded(ctx, task.ID, videoURLs, nil, resultPayload, finalCost, settleDuration)
	if err != nil {
		logger.L().Error("async_video.mark_succeeded_failed", zap.Int64("task_id", task.ID), zap.Error(err))
		return
	}
	if !updated {
		return
	}
	task.Status = AsyncVideoStatusSucceeded
	task.VideoURLs = videoURLs
	task.ResultPayload = resultPayload
	task.FinalCost = finalCost
	task.DurationSeconds = settleDuration
	s.writeTerminalUsageLog(ctx, task, billingType, finalCost, BillingStatusCharged, videoURLs, nil)
}

// transferVideosToCOS 尝试把上游视频 URL 转存到 COS，并把 videoURLs / result payload
// 里的 URL 替换成 COS URL。转存失败或未启用时保留上游原始 URL 兜底（不影响主流程）。
//
// 返回替换后的 videoURLs 与 result（就地修改也可安全使用；这里保持函数式风格返回新引用）。
// 注意：payload 是 map[string]any，替换是原地写回；返回的仍是同一 map 指针。
func (s *AsyncVideoService) transferVideosToCOS(ctx context.Context, task *AsyncVideoTask, videoURLs []string, result map[string]any) ([]string, map[string]any) {
	if s == nil || s.cosService == nil || len(videoURLs) == 0 {
		return videoURLs, result
	}
	if !s.cosService.IsEnabled(ctx) {
		return videoURLs, result
	}

	cosURLs, allOK := s.cosService.TransferVideos(ctx, videoURLs)
	mapping := make(map[string]string, len(videoURLs))
	successCount := 0
	newVideoURLs := make([]string, len(videoURLs))
	for i, orig := range videoURLs {
		if i < len(cosURLs) && strings.TrimSpace(cosURLs[i]) != "" {
			mapping[orig] = cosURLs[i]
			newVideoURLs[i] = cosURLs[i]
			successCount++
		} else {
			newVideoURLs[i] = orig // 兜底原 URL
		}
	}

	if len(mapping) > 0 {
		replaceURLsInPayload(result, mapping)
	}

	logger.L().Info("async_video.cos_transfer.completed",
		zap.Int64("task_id", task.ID),
		zap.Int("total", len(videoURLs)),
		zap.Int("success", successCount),
		zap.Bool("all_ok", allOK))

	return newVideoURLs, result
}

// replaceURLsInPayload 递归遍历 payload，把命中 mapping 的 URL 字符串替换为 COS URL。
// 兼容 fal 视频结果的常见结构：
//   - {video: {url: "..."}}
//   - {output_video: {url: "..."}}
//   - {videos: [{url: "..."}, ...]}
//   - {video_url: "..."}
//
// 未命中的字符串保持不变；数字/布尔/nil 也保持不变。
func replaceURLsInPayload(node any, mapping map[string]string) any {
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			v[k] = replaceURLsInPayload(child, mapping)
		}
		return v
	case []any:
		for i, item := range v {
			v[i] = replaceURLsInPayload(item, mapping)
		}
		return v
	case string:
		if replaced, ok := mapping[v]; ok {
			return replaced
		}
		return v
	default:
		return v
	}
}

// markFailedAndRefund 失败终态：退还全部预扣、置 refunded/expired、终态写 usage_log（refunded）。
func (s *AsyncVideoService) markFailedAndRefund(ctx context.Context, task *AsyncVideoTask, billingType int8, reason string) {
	status := AsyncVideoStatusRefunded
	if task.FailDeadlineAt != nil && time.Now().After(*task.FailDeadlineAt) {
		status = AsyncVideoStatusExpired
	}
	updated, err := s.taskRepo.MarkRefunded(ctx, task.ID, status, reason)
	if err != nil {
		logger.L().Error("async_video.mark_refunded_failed", zap.Int64("task_id", task.ID), zap.Error(err))
		return
	}
	if !updated {
		return
	}
	task.Status = status
	task.ErrorReason = amStrPtr(reason)
	if task.HeldCost > 0 {
		s.refund(ctx, billingType, asyncVideoBillingContext(task), task.HeldCost)
	}
	s.writeTerminalUsageLog(ctx, task, billingType, 0, BillingStatusRefunded, nil, nil)
}

// writeTerminalUsageLog 终态追加写 usage_log（视频路径）。
func (s *AsyncVideoService) writeTerminalUsageLog(
	ctx context.Context,
	task *AsyncVideoTask,
	billingType int8,
	cost float64,
	billingStatus string,
	videoURLs, cosURLs []string,
) {
	in := &VideoTerminalUsageLogInput{
		UserID:          task.UserID,
		APIKeyID:        task.APIKeyID,
		AccountID:       amDerefInt64(task.AccountID),
		RequestID:       task.InternalRequestID,
		OrganizationID:  task.OrganizationID,
		PayerUserID:     task.PayerUserID,
		BalanceSource:   task.BalanceSource,
		AuthzGeneration: task.AuthzGeneration,
		Model:           amDerefStr(task.UpstreamModel),
		RequestedModel:  task.RequestedModel,
		UpstreamModel:   amDerefStr(task.UpstreamModel),
		GroupID:         task.GroupID,
		ChannelID:       task.ChannelID,
		TotalCost:       cost,
		ActualCost:      cost,
		RateMultiplier:  task.RateMultiplier,
		BillingType:     billingType,
		RequestType:     int16(RequestTypeSync),
		Resolution:      amDerefStr(task.Resolution),
		DurationSeconds: task.DurationSeconds,
		AspectRatio:     amDerefStr(task.AspectRatio),
		UnitPrice:       task.UnitPriceSnapshot,
		TaskID:          task.ID,
		VideoURLs:       videoURLs,
		CosURLs:         cosURLs,
		BillingStatus:   billingStatus,

		ClientIP:         amDerefStr(task.ClientIP),
		UserAgent:        amDerefStr(task.UserAgent),
		InboundEndpoint:  amDerefStr(task.InboundEndpoint),
		UpstreamEndpoint: amDerefStr(task.UpstreamEndpoint),
		DurationMs:       asyncVideoDurationMs(task),
	}
	// 诊断日志：
	//   - 走到这里就说明 task 已进入终态并调用了本函数；
	//   - inserted=true 表示 usage_logs 首次写入；
	//   - inserted=false 通常是 ON CONFLICT(request_id, api_key_id) 命中（重复 poll/幂等）；
	//   - 若 err!=nil 则打 Error，附带 request_id/task_id/user_id/model 以便定位。
	inserted, err := s.taskRepo.InsertTerminalUsageLog(ctx, in)
	if err != nil {
		logger.L().Error("async_video.terminal_usage_log_failed",
			zap.Int64("task_id", task.ID),
			zap.Int64("user_id", task.UserID),
			zap.Int64("api_key_id", task.APIKeyID),
			zap.String("request_id", task.InternalRequestID),
			zap.String("requested_model", task.RequestedModel),
			zap.String("upstream_model", amDerefStr(task.UpstreamModel)),
			zap.String("billing_status", billingStatus),
			zap.Float64("cost", cost),
			zap.Error(err))
		return
	}
	logger.L().Info("async_video.terminal_usage_log_written",
		zap.Int64("task_id", task.ID),
		zap.Int64("user_id", task.UserID),
		zap.String("request_id", task.InternalRequestID),
		zap.String("requested_model", task.RequestedModel),
		zap.String("billing_status", billingStatus),
		zap.Bool("inserted", inserted),
		zap.Float64("cost", cost),
	)
	if !inserted {
		// ON CONFLICT DO NOTHING 命中：多半是重复 poll，属正常；但如果频繁出现且用户报"看不到"，
		// 可以据此排查 request_id/api_key_id 冲突是不是把不同任务错误合并了。
		logger.L().Warn("async_video.terminal_usage_log_conflict_or_skipped",
			zap.Int64("task_id", task.ID),
			zap.Int64("user_id", task.UserID),
			zap.Int64("api_key_id", task.APIKeyID),
			zap.String("request_id", task.InternalRequestID),
			zap.String("billing_status", billingStatus))
	}
	if billingStatus == BillingStatusCharged && s.billingContextResolver != nil {
		billing := &BillingContext{ConsumerUserID: task.UserID, OrganizationID: task.OrganizationID, PayerUserID: asyncVideoPayerID(task), BalanceSource: amDerefStr(task.BalanceSource)}
		if err := s.billingContextResolver.RecordSpendLimitAlert(ctx, billing); err != nil {
			logger.L().Warn("async_video.spend_limit_alert_failed", zap.Int64("task_id", task.ID), zap.Error(err))
		}
	}

	// 写入成本中心：与文本/图片网关一致——仅在真正成功计费（charged）且 usage_log 首次写入（inserted=true）
	// 的情况下才写，避免 ON CONFLICT 重复 poll 时重复计账。cost==0 同样跳过，避免无意义零额 event。
	if inserted && billingStatus == BillingStatusCharged && cost > 0 {
		s.writeCostCenterEvents(ctx, task, cost)
	}
}

// writeCostCenterEvents 为已成功结算的视频任务写入成本中心事件。
//
// 处理方式与 Gateway/OpenAI 网关的 writeCostCenterUsageEvents 完全对齐：构造一个最小
// 可用的 UsageLog 后直接复用那个函数，避免重复实现 source 推断 / 幂等 key / 渠道分类等逻辑，
// 保证后续两边保持一致行为。
func (s *AsyncVideoService) writeCostCenterEvents(ctx context.Context, task *AsyncVideoTask, cost float64) {
	if s == nil || s.costCenter == nil || task == nil {
		return
	}
	// task 本身没有记账时间；用 FinishedAt（若设置）否则 now()。
	occurred := time.Now().UTC()
	if task.FinishedAt != nil {
		occurred = task.FinishedAt.UTC()
	}
	log := &UsageLog{
		UserID:         task.UserID,
		APIKeyID:       task.APIKeyID,
		AccountID:      amDerefInt64(task.AccountID),
		RequestID:      task.InternalRequestID,
		OrganizationID: task.OrganizationID,
		PayerUserID:    task.PayerUserID,
		BalanceSource:  task.BalanceSource,
		Model:          amDerefStr(task.UpstreamModel),
		RequestedModel: task.RequestedModel,
		GroupID:        task.GroupID,
		ChannelID:      task.ChannelID,
		TotalCost:      cost,
		ActualCost:     cost,
		CreatedAt:      occurred,
		// 视频任务未记录账号上的 rate multiplier，AccountRateMultiplier=nil 时
		// writeCostCenterUsageEvents 会自动按 1.0 处理 upstream cost，语义一致。
	}
	writeCostCenterUsageEvents(ctx, s.costCenter, log, false)
}

// charge / refund 与 async_media 共享同款语义（付款上下文、组织余额、缓存失效）。
func (s *AsyncVideoService) charge(ctx context.Context, billingType int8, billing *BillingContext, amount float64) error {
	if amount <= 0 || billingType != BillingTypeBalance || s.userRepo == nil {
		return nil
	}
	if billing == nil {
		return ErrUserNotFound
	}
	if billing.UsesCompanyBalance() {
		if s.billingContextResolver == nil {
			return fmt.Errorf("organization balance resolver is unavailable")
		}
		_, err := s.billingContextResolver.DeductOrganizationBalance(ctx, billing, amount)
		return err
	}
	// 关键：这里必须用**原子**扣款（AdjustBalance），不能用 DeductBalance。
	// DeductBalance 在余额不足时会走 fallback 直接把余额扣成负数——
	// 恶意用户发起并发提交（例如一次点击生成 100 条），旧路径会全部通过，
	// 只有最终成为负数才停止；用户实际只有 1 次的钱，却触发 100 次上游
	// 调用，造成亏损。
	//
	// AdjustBalance 走的是 UPDATE ... WHERE balance + delta >= 0 的单条 SQL：
	//   - 并发多个 goroutine 只会有 floor(balance / cost) 条 UPDATE 命中，
	//     其他直接返回 ErrBalanceNegative，任务根本不会被创建。
	//   - 我们把 ErrBalanceNegative 归一为 ErrInsufficientBalance，上层
	//     handler 会把它映射成 402 让客户端能识别到"余额不足"。
	if _, err := s.userRepo.AdjustBalance(ctx, billing.PayerUserID, -amount); err != nil {
		if errors.Is(err, ErrBalanceNegative) {
			return ErrInsufficientBalance
		}
		return err
	}
	if s.balanceCache != nil {
		if err := s.balanceCache.InvalidateUserBalance(ctx, billing.PayerUserID); err != nil {
			logger.L().Warn("async_video.balance_cache_invalidate_failed", zap.Int64("payer_user_id", billing.PayerUserID), zap.Error(err))
		}
	}
	return nil
}

func (s *AsyncVideoService) refund(ctx context.Context, billingType int8, billing *BillingContext, amount float64) {
	if amount <= 0 || billingType != BillingTypeBalance || s.userRepo == nil {
		return
	}
	if billing == nil {
		return
	}
	if billing.UsesCompanyBalance() {
		if s.billingContextResolver == nil {
			logger.L().Error("async_video.refund_failed", zap.Error(errors.New("organization balance resolver is unavailable")))
			return
		}
		if _, err := s.billingContextResolver.CreditOrganizationBalance(ctx, billing, amount); err != nil {
			logger.L().Error("async_video.refund_failed", zap.Int64("organization_id", *billing.OrganizationID), zap.Float64("amount", amount), zap.Error(err))
		}
		return
	}
	if err := s.userRepo.UpdateBalance(ctx, billing.PayerUserID, amount); err != nil {
		logger.L().Error("async_video.refund_failed",
			zap.Int64("user_id", billing.PayerUserID), zap.Float64("amount", amount), zap.Error(err))
		return
	}
	if s.balanceCache != nil {
		if err := s.balanceCache.InvalidateUserBalance(ctx, billing.PayerUserID); err != nil {
			logger.L().Warn("async_video.balance_cache_invalidate_failed", zap.Int64("payer_user_id", billing.PayerUserID), zap.Error(err))
		}
	}
}

// videoUpstreamClient 抽象异步视频上游的提交/轮询/取消能力，
// 使执行内核（提交、轮询、结算、退费、COS 转存）与具体平台解耦。
//
// *fal.Client、*atlascloud.Client 与 *apiz.Client 均满足该接口。返回类型统一复用
// fal 包的 SubmitResponse/StatusResponse/APIError，因此 SubmitAsync /
// pollOnce 中的 errors.As(err, &fal.APIError) 分支对各平台一致适用。
type videoUpstreamClient interface {
	SubmitRaw(ctx context.Context, model string, body any) (*fal.SubmitResponse, error)
	Status(ctx context.Context, statusURL string) (*fal.StatusResponse, error)
	ResultRaw(ctx context.Context, responseURL string) (map[string]any, error)
	BuildStatusURL(model, requestID string) string
	BuildResponseURL(model, requestID string) string
	BuildCancelURL(model, requestID string) string
	Cancel(ctx context.Context, cancelURL string) error
}

// newClient 基于账号平台与凭证构建对应的上游客户端。
//
// fal 账号走 fal queue/sync 协议；atlascloud / apiz 账号走各自的
// generate*/prediction 异步协议（均适配为 videoUpstreamClient）。
func (s *AsyncVideoService) newClient(account *Account) (videoUpstreamClient, error) {
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	if account.Platform == PlatformAtlasCloud {
		return atlascloud.NewClient(atlascloud.Config{
			APIKey:   account.AtlasCloudAPIKey(),
			BaseURL:  account.AtlasCloudBaseURL(),
			ProxyURL: proxyURL,
		})
	}
	if account.Platform == PlatformApiz {
		return apiz.NewClient(apiz.Config{
			APIKey:   account.ApizAPIKey(),
			BaseURL:  account.ApizBaseURL(),
			ProxyURL: proxyURL,
		})
	}
	return fal.NewClient(fal.Config{
		APIKey:       account.FalAPIKey(),
		QueueBaseURL: account.FalQueueBaseURL(),
		SyncBaseURL:  account.FalSyncBaseURL(),
		ProxyURL:     proxyURL,
	})
}

// ----- helpers -----

func asyncVideoPayerID(task *AsyncVideoTask) int64 {
	if task != nil && task.PayerUserID != nil && *task.PayerUserID > 0 {
		return *task.PayerUserID
	}
	if task != nil {
		return task.UserID
	}
	return 0
}

func asyncVideoBillingContext(task *AsyncVideoTask) *BillingContext {
	if task == nil {
		return nil
	}
	return &BillingContext{
		ConsumerUserID:  task.UserID,
		OrganizationID:  task.OrganizationID,
		PayerUserID:     asyncVideoPayerID(task),
		BalanceSource:   amDerefStr(task.BalanceSource),
		AuthzGeneration: amDerefInt64(task.AuthzGeneration),
	}
}

func asyncVideoDurationMs(task *AsyncVideoTask) int64 {
	if task == nil || task.CreatedAt.IsZero() {
		return 0
	}
	end := time.Now()
	if task.FinishedAt != nil && !task.FinishedAt.IsZero() {
		end = *task.FinishedAt
	}
	d := end.Sub(task.CreatedAt).Milliseconds()
	if d < 0 {
		return 0
	}
	return d
}

// ExtractVideoBillingDims 从客户端请求 payload 中提取视频计费维度：
// resolution、duration_seconds、aspect_ratio。
//
// 兼容常见字段名：
//   - resolution: "resolution" | "video_resolution"
//   - duration:   "duration" | "duration_seconds" | "num_seconds"
//   - ratio:      "aspect_ratio"
//
// duration 允许为 0：某些模型（如 veo3 系列）支持 duration="auto"，
// 由上游返回实际时长后再按实际值结算差额。调用方需自行判断 0 时使用兜底预扣。
func ExtractVideoBillingDims(payload map[string]any) (resolution string, duration int, aspectRatio string) {
	if payload == nil {
		return "", 0, ""
	}
	resolution = firstStringField(payload, "resolution", "video_resolution")
	aspectRatio = firstStringField(payload, "aspect_ratio")
	duration = firstIntField(payload, "duration", "duration_seconds", "num_seconds")
	return
}

// ExtractActualDurationSeconds 从上游 result payload 中尽力抽取视频实际时长（秒）。
//
// 兼容常见结构：
//   - { "video": { "duration": 10, ... } }
//   - { "video": { "duration_seconds": 10 } }
//   - { "duration": 10 } / { "duration_seconds": 10 } / { "num_seconds": 10 }
//
// 未识别到时返回 0，调用方应保留 heldCost 作为 finalCost（不追扣不退款）。
func ExtractActualDurationSeconds(result map[string]any) int {
	if result == nil {
		return 0
	}
	if v, ok := result["video"].(map[string]any); ok {
		if d := firstIntField(v, "duration", "duration_seconds", "num_seconds"); d > 0 {
			return d
		}
	}
	if v, ok := result["output_video"].(map[string]any); ok {
		if d := firstIntField(v, "duration", "duration_seconds", "num_seconds"); d > 0 {
			return d
		}
	}
	return firstIntField(result, "duration", "duration_seconds", "num_seconds")
}

// normalizeVideoResolution 将请求中的 resolution 字段归一化为定价表使用的小写档位。
// 支持常见写法："720p"/"720P"/"1080"/"1080p"/"4k"/"4K" 等。
func normalizeVideoResolution(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return ""
	}
	if strings.HasSuffix(v, "p") || v == "4k" {
		return v
	}
	switch v {
	case "480", "720", "1080":
		return v + "p"
	}
	return v
}

func firstStringField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func firstIntField(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch typed := v.(type) {
			case float64:
				return int(typed)
			case int:
				return typed
			case int64:
				return int(typed)
			case json.Number:
				if n, err := typed.Int64(); err == nil {
					return int(n)
				}
			case string:
				var n int
				if _, err := fmt.Sscanf(typed, "%d", &n); err == nil && n > 0 {
					return n
				}
			}
		}
	}
	return 0
}
