package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

// 异步媒体执行内核的默认时序参数。
const (
	defaultAsyncMediaPollInterval = 2 * time.Second
	// defaultAsyncMediaFailTimeout 是任务从创建到强制判失（退费兜底）的最长时间。
	// reconciler 在任务超过 fail_deadline_at 仍未出图时退费并置 expired。
	defaultAsyncMediaFailTimeout = 30 * time.Minute
)

// AsyncMediaService 异步媒体任务执行内核。
//
// 职责：
//   - 提交任务（构建 fal 请求 → 预扣费 → 落库 pending → submit → running）
//   - 轮询/取结果（running → succeeded，取 images）
//   - 失败判定（status 明确失败 或 到达 fail_deadline_at）与退费（幂等）
//   - 成功转存 COS 并在终态追加写 usage_log（charged/refunded）
//
// 计费采用「预扣 + 结算退差」模型：
//   - 提交时按 (size_tier × quality × num_images) 预扣 heldCost
//   - 成功时按实际出图数量结算 finalCost；若 finalCost < heldCost 退还差额
//   - 失败/超时退还全部 heldCost
//
// 余额账本仅对 BillingTypeBalance 生效；订阅计费的额度核算沿用既有 usage_log 记录路径。
type AsyncMediaService struct {
	taskRepo AsyncMediaTaskRepository
	userRepo UserRepository
	billing  *BillingService
	resolver *ModelPricingResolver
	cos      *COSImageTransferService
	deferred *DeferredService

	pollInterval time.Duration
	failTimeout  time.Duration
}

// NewAsyncMediaService 创建异步媒体执行内核。
func NewAsyncMediaService(
	taskRepo AsyncMediaTaskRepository,
	userRepo UserRepository,
	billing *BillingService,
	resolver *ModelPricingResolver,
	cos *COSImageTransferService,
) *AsyncMediaService {
	return &AsyncMediaService{
		taskRepo:     taskRepo,
		userRepo:     userRepo,
		billing:      billing,
		resolver:     resolver,
		cos:          cos,
		pollInterval: defaultAsyncMediaPollInterval,
		failTimeout:  defaultAsyncMediaFailTimeout,
	}
}

// SetPollInterval 配置轮询间隔（reconciler / 配置项）。
func (s *AsyncMediaService) SetPollInterval(d time.Duration) {
	if d > 0 {
		s.pollInterval = d
	}
}

// SetFailTimeout 配置任务强制判失（退费兜底）时间。
func (s *AsyncMediaService) SetFailTimeout(d time.Duration) {
	if d > 0 {
		s.failTimeout = d
	}
}

// SetDeferredService 注入延迟批量更新服务，用于在账号实际被使用时记录 last_used_at。
func (s *AsyncMediaService) SetDeferredService(d *DeferredService) {
	s.deferred = d
}

// FailTimeout 返回当前的失败兜底时间。
func (s *AsyncMediaService) FailTimeout() time.Duration { return s.failTimeout }

// AsyncMediaSubmitInput 提交异步媒体任务的入参。
type AsyncMediaSubmitInput struct {
	Account *Account
	User    *User

	APIKeyID  int64
	UserID    int64
	AccountID int64
	GroupID   *int64
	ChannelID *int64

	Facade            string // openai / fal
	InternalRequestID string // 网关侧生成的内部请求 ID（幂等键）
	RequestedModel    string // 客户端请求模型（映射前）

	Input fal.ImageGenInput // 协议无关的图片请求描述

	BillingType    int8    // 0=balance / 1=subscription
	RateMultiplier float64 // 计费倍率

	// 请求元信息（提交时持久化到任务表，供终态 usage_log 回填端点/IP/UA）。
	ClientIP        string // 客户端 IP
	UserAgent       string // 客户端 User-Agent
	InboundEndpoint string // 对外门面端点（客户端可见路径）
}

// SubmitAsync 提交一个异步媒体任务：预扣费 → 落库 → 提交上游 → 置 running。
//
// 任一前置步骤失败将回滚已扣余额并返回错误；任务一旦成功提交即进入 running，
// 后续由 WaitForTerminal（伪同步）或 reconciler（兜底）推进到终态。
func (s *AsyncMediaService) SubmitAsync(ctx context.Context, in *AsyncMediaSubmitInput) (*AsyncMediaTask, error) {
	if in == nil {
		return nil, errors.New("nil async media submit input")
	}
	if in.Account == nil {
		return nil, errors.New("async media: account is required")
	}
	if in.RateMultiplier == 0 {
		in.RateMultiplier = 1
	}

	upstreamModel := s.resolveUpstreamModel(in.Account, in.RequestedModel, in.Input.IsEdit)
	sizeTier := NormalizeImageBillingTierOrDefault(in.Input.Size)
	quality := fal.MapQualityToFal(in.Input.Quality)
	numImages := in.Input.N
	if numImages <= 0 {
		numImages = 1
	}

	// 预估并预扣费用（按 num_images 的满额预扣）。
	heldCost, err := s.estimateCost(ctx, upstreamModel, in.GroupID, sizeTier, quality, numImages, in.RateMultiplier)
	if err != nil {
		return nil, fmt.Errorf("async media: estimate cost: %w", err)
	}
	if err := s.charge(ctx, in.BillingType, in.UserID, heldCost); err != nil {
		return nil, fmt.Errorf("async media: pre-charge: %w", err)
	}

	failDeadline := time.Now().Add(s.failTimeout)
	task := &AsyncMediaTask{
		InternalRequestID: in.InternalRequestID,
		APIKeyID:          in.APIKeyID,
		UserID:            in.UserID,
		AccountID:         amOptInt64(in.AccountID),
		GroupID:           in.GroupID,
		ChannelID:         in.ChannelID,
		Facade:            in.Facade,
		RequestedModel:    in.RequestedModel,
		UpstreamModel:     amStrPtr(upstreamModel),
		ImageSize:         amStrPtr(sizeTier),
		Quality:           amStrPtr(quality),
		NumImages:         numImages,
		Status:            AsyncMediaStatusPending,
		HeldCost:          heldCost,
		RateMultiplier:    in.RateMultiplier,
		SizeTier:          amStrPtr(sizeTier),
		FailDeadlineAt:    &failDeadline,
		ClientIP:          amStrPtr(in.ClientIP),
		UserAgent:         amStrPtr(in.UserAgent),
		InboundEndpoint:   amStrPtr(in.InboundEndpoint),
		UpstreamEndpoint:  amStrPtr(falUpstreamEndpoint(upstreamModel)),
	}
	if err := s.taskRepo.Create(ctx, task); err != nil {
		// 落库失败：回滚预扣费，避免漏退。
		s.refund(ctx, in.BillingType, in.UserID, heldCost)
		return nil, fmt.Errorf("async media: create task: %w", err)
	}

	client, err := s.newClient(in.Account)
	if err != nil {
		s.markFailedAndRefund(ctx, task, in.BillingType, "build fal client: "+err.Error())
		return task, fmt.Errorf("async media: build client: %w", err)
	}

	req := fal.BuildRequest(in.Input)
	submitResp, err := client.Submit(ctx, upstreamModel, req)
	if err != nil {
		s.markFailedAndRefund(ctx, task, in.BillingType, "submit: "+err.Error())
		return task, fmt.Errorf("async media: submit: %w", err)
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
		logger.L().Warn("async_media.update_upstream_ref_failed",
			zap.Int64("task_id", task.ID), zap.Error(err))
	}
	task.UpstreamRequestID = amStrPtr(submitResp.RequestID)
	task.StatusURL = amStrPtr(statusURL)
	task.ResponseURL = amStrPtr(responseURL)
	task.Status = AsyncMediaStatusRunning

	// 账号已成功向上游提交任务，视为本次被使用：记录 last_used_at（延迟批量刷库）。
	if s.deferred != nil {
		s.deferred.ScheduleLastUsedUpdate(in.Account.ID)
	}
	return task, nil
}

// WaitForTerminal 伪同步阻塞等待任务终态，直到出图成功、明确失败或 ctx 超时。
//
// 关键约束：ctx 超时（伪同步等待超时）返回 ErrAsyncMediaPending，
// 但不退费、不终结任务——任务仍由 reconciler 兜底处理。
var ErrAsyncMediaPending = errors.New("async media task still pending")

// ErrAsyncMediaPricingMissing 表示上游模型未在渠道/分组中配置可用的定价。
//
// 触发条件：image / per-request 模式下，分层价（含 size_tier × quality）与
// 默认按次价均为 0，意味着该模型未配置任何有效定价。此时禁止提交任务，
// 防止账户被「免费刷图」。
var ErrAsyncMediaPricingMissing = errors.New("async media pricing not configured")

func (s *AsyncMediaService) WaitForTerminal(ctx context.Context, task *AsyncMediaTask, in *AsyncMediaSubmitInput) (*AsyncMediaTask, error) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		updated, done, err := s.pollOnce(ctx, task, in.Account, in.BillingType)
		if err != nil {
			return updated, err
		}
		if done {
			return updated, nil
		}
		select {
		case <-ctx.Done():
			// 伪同步超时：不退费、不终结，交由 reconciler 兜底。
			return task, ErrAsyncMediaPending
		case <-ticker.C:
		}
	}
}

// GetTaskByInternalID 按内部请求 ID 查询任务（不存在返回 nil,nil）。
func (s *AsyncMediaService) GetTaskByInternalID(ctx context.Context, internalRequestID string) (*AsyncMediaTask, error) {
	return s.taskRepo.GetByInternalRequestID(ctx, internalRequestID)
}

// GetTaskByUpstreamID 按上游 request_id 查询任务（不存在返回 nil,nil）。
func (s *AsyncMediaService) GetTaskByUpstreamID(ctx context.Context, upstreamRequestID string) (*AsyncMediaTask, error) {
	return s.taskRepo.GetByUpstreamRequestID(ctx, upstreamRequestID)
}

// AdvanceTask 推进单个任务一轮轮询，并在终态时结算/退费（供原生门面 status/result 触发）。
// 返回 (最新任务, 是否终态, error)。任务已终态时直接返回。
func (s *AsyncMediaService) AdvanceTask(ctx context.Context, task *AsyncMediaTask, account *Account) (*AsyncMediaTask, bool, error) {
	if task == nil {
		return nil, false, errors.New("async media advance: nil task")
	}
	if task.IsTerminal() {
		return task, true, nil
	}
	if account == nil {
		return task, false, errors.New("async media advance: account is nil")
	}
	return s.pollOnce(ctx, task, account, BillingTypeBalance)
}

// CancelTask 取消一个在飞任务并退费（幂等）。
func (s *AsyncMediaService) CancelTask(ctx context.Context, task *AsyncMediaTask, account *Account) error {
	if task == nil {
		return errors.New("async media cancel: nil task")
	}
	if task.IsTerminal() {
		return nil
	}
	if account != nil && task.UpstreamRequestID != nil {
		if client, err := s.newClient(account); err == nil {
			cancelURL := client.BuildCancelURL(amDerefStr(task.UpstreamModel), *task.UpstreamRequestID)
			if cancelErr := client.Cancel(ctx, cancelURL); cancelErr != nil {
				logger.L().Warn("async_media.cancel_upstream_failed",
					zap.Int64("task_id", task.ID), zap.Error(cancelErr))
			}
		}
	}
	s.markFailedAndRefund(ctx, task, BillingTypeBalance, "cancelled by client")
	return nil
}

// ReconcileTask 供 reconciler 调用：推进单个未终结任务到终态。
// 到达 fail_deadline_at 仍未出图则强制退费置 expired。
func (s *AsyncMediaService) ReconcileTask(ctx context.Context, task *AsyncMediaTask, account *Account) error {
	if task == nil {
		return nil
	}
	billingType := BillingTypeBalance // 兜底退费按余额账本（订阅额度由 usage_log 路径核算）

	if task.FailDeadlineAt != nil && time.Now().After(*task.FailDeadlineAt) {
		s.markFailedAndRefund(ctx, task, billingType, "fail deadline exceeded")
		return nil
	}
	if account == nil {
		return errors.New("async media reconcile: account is nil")
	}
	_, _, err := s.pollOnce(ctx, task, account, billingType)
	return err
}

// pollOnce 执行一轮状态查询并在终态时结算/退费。
// 返回 (最新任务, 是否终态, error)。
func (s *AsyncMediaService) pollOnce(ctx context.Context, task *AsyncMediaTask, account *Account, billingType int8) (*AsyncMediaTask, bool, error) {
	client, err := s.newClient(account)
	if err != nil {
		return task, false, fmt.Errorf("async media poll: build client: %w", err)
	}
	statusURL := ""
	if task.StatusURL != nil {
		statusURL = *task.StatusURL
	}
	st, err := client.Status(ctx, statusURL)
	if err != nil {
		var apiErr *fal.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			// 明确的客户端错误视为失败（任务不可恢复）。
			s.markFailedAndRefund(ctx, task, billingType, fmt.Sprintf("status %d: %s", apiErr.StatusCode, apiErr.Body))
			return task, true, nil
		}
		// 网络/5xx：暂不终结，等待下一轮或 reconciler。
		return task, false, nil
	}

	if !st.IsTerminal() {
		return task, false, nil
	}

	// 终态：取结果。
	responseURL := st.ResponseURL
	if responseURL == "" && task.ResponseURL != nil {
		responseURL = *task.ResponseURL
	}
	result, err := client.Result(ctx, responseURL)
	if err != nil {
		var apiErr *fal.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			s.markFailedAndRefund(ctx, task, billingType, fmt.Sprintf("result %d: %s", apiErr.StatusCode, apiErr.Body))
			return task, true, nil
		}
		return task, false, nil
	}

	imageURLs := extractFalImageURLs(result)
	if len(imageURLs) == 0 {
		s.markFailedAndRefund(ctx, task, billingType, "upstream returned no images")
		return task, true, nil
	}

	s.markSucceeded(ctx, task, billingType, imageURLs)
	return task, true, nil
}

// markSucceeded 成功结算：转存 COS、结算 finalCost（退差）、置 succeeded、终态写 usage_log。
func (s *AsyncMediaService) markSucceeded(ctx context.Context, task *AsyncMediaTask, billingType int8, imageURLs []string) {
	// COS 转存（失败回退 fal url，任务仍成功不退费）。
	var cosURLs []string
	if s.cos != nil && s.cos.IsEnabled(ctx) {
		if transferred, ok := s.cos.TransferImages(ctx, imageURLs); ok {
			cosURLs = transferred
		}
	}

	// 结算 finalCost：按实际出图数量重算。
	upstreamModel := amDerefStr(task.UpstreamModel)
	sizeTier := amDerefStr(task.SizeTier)
	quality := amDerefStr(task.Quality)
	finalCost, err := s.estimateCost(ctx, upstreamModel, task.GroupID, sizeTier, quality, len(imageURLs), task.RateMultiplier)
	if err != nil {
		// 结算失败时按预扣额结算，避免误退。
		finalCost = task.HeldCost
	}
	if finalCost > task.HeldCost {
		finalCost = task.HeldCost // 预扣为上限，不超额扣费
	}

	updated, err := s.taskRepo.MarkSucceeded(ctx, task.ID, imageURLs, cosURLs, finalCost)
	if err != nil {
		logger.L().Error("async_media.mark_succeeded_failed", zap.Int64("task_id", task.ID), zap.Error(err))
		return
	}
	if !updated {
		// 已被其它路径终结（幂等）。
		return
	}
	task.Status = AsyncMediaStatusSucceeded
	task.ImageURLs = imageURLs
	task.CosURLs = cosURLs
	task.FinalCost = finalCost

	// 退还预扣与结算的差额。
	if refundDelta := task.HeldCost - finalCost; refundDelta > 0 {
		s.refund(ctx, billingType, task.UserID, refundDelta)
	}

	s.writeTerminalUsageLog(ctx, task, billingType, finalCost, BillingStatusCharged, imageURLs, cosURLs)
}

// markFailedAndRefund 失败终态：退还全部预扣、置 refunded/expired、终态写 usage_log（refunded）。
func (s *AsyncMediaService) markFailedAndRefund(ctx context.Context, task *AsyncMediaTask, billingType int8, reason string) {
	status := AsyncMediaStatusRefunded
	if task.FailDeadlineAt != nil && time.Now().After(*task.FailDeadlineAt) {
		status = AsyncMediaStatusExpired
	}
	updated, err := s.taskRepo.MarkRefunded(ctx, task.ID, status, reason)
	if err != nil {
		logger.L().Error("async_media.mark_refunded_failed", zap.Int64("task_id", task.ID), zap.Error(err))
		return
	}
	if !updated {
		// 已被其它路径终结（幂等）：不重复退费。
		return
	}
	task.Status = status
	task.ErrorReason = amStrPtr(reason)
	if task.HeldCost > 0 {
		s.refund(ctx, billingType, task.UserID, task.HeldCost)
	}
	s.writeTerminalUsageLog(ctx, task, billingType, 0, BillingStatusRefunded, nil, nil)
}

// writeTerminalUsageLog 终态追加写一条 usage_log。
func (s *AsyncMediaService) writeTerminalUsageLog(
	ctx context.Context,
	task *AsyncMediaTask,
	billingType int8,
	cost float64,
	billingStatus string,
	imageURLs, cosURLs []string,
) {
	in := &TerminalUsageLogInput{
		UserID:         task.UserID,
		APIKeyID:       task.APIKeyID,
		AccountID:      amDerefInt64(task.AccountID),
		RequestID:      task.InternalRequestID,
		Model:          amDerefStr(task.UpstreamModel),
		RequestedModel: task.RequestedModel,
		UpstreamModel:  amDerefStr(task.UpstreamModel),
		GroupID:        task.GroupID,
		ChannelID:      task.ChannelID,
		TotalCost:      cost,
		ActualCost:     cost,
		RateMultiplier: task.RateMultiplier,
		BillingType:    billingType,
		RequestType:    int16(RequestTypeSync),
		ImageCount:     task.NumImages,
		ImageSize:      amDerefStr(task.ImageSize),
		BillingTier:    amDerefStr(task.SizeTier),
		TaskID:         task.ID,
		ImageURLs:      imageURLs,
		CosURLs:        cosURLs,
		BillingStatus:  billingStatus,

		ClientIP:         amDerefStr(task.ClientIP),
		UserAgent:        amDerefStr(task.UserAgent),
		InboundEndpoint:  amDerefStr(task.InboundEndpoint),
		UpstreamEndpoint: amDerefStr(task.UpstreamEndpoint),
		DurationMs:       asyncMediaDurationMs(task),
	}
	if _, err := s.taskRepo.InsertTerminalUsageLog(ctx, in); err != nil {
		logger.L().Warn("async_media.terminal_usage_log_failed",
			zap.Int64("task_id", task.ID), zap.String("billing_status", billingStatus), zap.Error(err))
	}
}

// estimateCost 通过统一计费入口估算 (size_tier × quality × count) 的实际费用。
//
// 强校验：image / per-request 模式下，若该模型在分组/渠道中未配置任何有效定价
// （RequestTiers 与 DefaultPerRequestPrice 均为 0），返回 ErrAsyncMediaPricingMissing，
// 调用方据此拒绝提交任务，避免账户被「0 费用刷图」。
func (s *AsyncMediaService) estimateCost(
	ctx context.Context,
	model string, groupID *int64,
	sizeTier, quality string, count int, rateMultiplier float64,
) (float64, error) {
	if s.billing == nil {
		return 0, fmt.Errorf("%w: billing service not initialized", ErrAsyncMediaPricingMissing)
	}
	if s.resolver == nil {
		return 0, fmt.Errorf("%w: pricing resolver not initialized", ErrAsyncMediaPricingMissing)
	}
	if count <= 0 {
		count = 1
	}

	// 先解析定价，校验是否存在有效配置（image/per-request 模式必须命中 tier 或默认价）。
	resolved := s.resolver.Resolve(ctx, PricingInput{
		Model:   model,
		GroupID: groupID,
	})
	if resolved == nil {
		return 0, fmt.Errorf("%w: model=%s", ErrAsyncMediaPricingMissing, model)
	}
	if resolved.Mode == BillingModePerRequest || resolved.Mode == BillingModeImage {
		if len(resolved.RequestTiers) == 0 && resolved.DefaultPerRequestPrice <= 0 {
			return 0, fmt.Errorf("%w: model=%s mode=%s", ErrAsyncMediaPricingMissing, model, resolved.Mode)
		}
	}

	breakdown, err := s.billing.CalculateCostUnified(CostInput{
		Ctx:            ctx,
		Model:          model,
		GroupID:        groupID,
		RequestCount:   count,
		SizeTier:       sizeTier,
		Quality:        quality,
		RateMultiplier: rateMultiplier,
		Resolver:       s.resolver,
		Resolved:       resolved,
	})
	if err != nil {
		// 统一计费层上抛的「定价不可用」视为配置缺失，转化为我们的 sentinel。
		if errors.Is(err, ErrModelPricingUnavailable) {
			return 0, fmt.Errorf("%w: model=%s: %v", ErrAsyncMediaPricingMissing, model, err)
		}
		return 0, err
	}
	if breakdown == nil {
		return 0, fmt.Errorf("%w: model=%s empty breakdown", ErrAsyncMediaPricingMissing, model)
	}
	// 二次防御：在合法计费倍率下结算价仍为 0，视为定价缺失。
	if rateMultiplier > 0 && breakdown.ActualCost <= 0 {
		return 0, fmt.Errorf("%w: model=%s size=%s quality=%s zero cost", ErrAsyncMediaPricingMissing, model, sizeTier, quality)
	}
	return breakdown.ActualCost, nil
}

// charge 预扣费用（仅 BillingTypeBalance 走余额账本）。
func (s *AsyncMediaService) charge(ctx context.Context, billingType int8, userID int64, amount float64) error {
	if amount <= 0 || billingType != BillingTypeBalance || s.userRepo == nil {
		return nil
	}
	return s.userRepo.DeductBalance(ctx, userID, amount)
}

// refund 退还费用（仅 BillingTypeBalance 走余额账本）。
func (s *AsyncMediaService) refund(ctx context.Context, billingType int8, userID int64, amount float64) {
	if amount <= 0 || billingType != BillingTypeBalance || s.userRepo == nil {
		return
	}
	if err := s.userRepo.UpdateBalance(ctx, userID, amount); err != nil {
		logger.L().Error("async_media.refund_failed",
			zap.Int64("user_id", userID), zap.Float64("amount", amount), zap.Error(err))
	}
}

// resolveUpstreamModel 解析客户端模型到 fal 上游 slug。
// 账号/渠道自定义映射优先，缺失时按是否为编辑请求选择内置默认 slug。
func (s *AsyncMediaService) resolveUpstreamModel(account *Account, requestedModel string, isEdit bool) string {
	if account != nil {
		if mapping := account.GetModelMapping(); mapping != nil {
			if v := strings.TrimSpace(mapping[requestedModel]); v != "" {
				return v
			}
		}
	}
	if isEdit {
		return domain.FalSlugImageEdit
	}
	return domain.FalSlugTextToImage
}

// newClient 基于账号凭证与代理构建 fal 客户端。
func (s *AsyncMediaService) newClient(account *Account) (*fal.Client, error) {
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	return fal.NewClient(fal.Config{
		APIKey:       account.FalAPIKey(),
		QueueBaseURL: account.FalQueueBaseURL(),
		SyncBaseURL:  account.FalSyncBaseURL(),
		ProxyURL:     proxyURL,
	})
}

// ----- helpers -----

func extractFalImageURLs(resp *fal.Response) []string {
	if resp == nil {
		return nil
	}
	out := make([]string, 0, len(resp.Images))
	for _, img := range resp.Images {
		if u := strings.TrimSpace(img.URL); u != "" {
			out = append(out, u)
		}
	}
	return out
}

func amStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}

// asyncMediaDurationMs 估算任务从提交到终结的耗时（毫秒）。
// 优先使用 finished_at - created_at；终态尚未回填 finished_at 时回退为「距创建时刻」。
func asyncMediaDurationMs(task *AsyncMediaTask) int64 {
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

// falUpstreamEndpoint 由上游 fal 模型 slug 推导对外展示的上游端点路径。
func falUpstreamEndpoint(upstreamModel string) string {
	slug := strings.TrimSpace(upstreamModel)
	if slug == "" {
		return ""
	}
	if !strings.HasPrefix(slug, "/") {
		slug = "/" + slug
	}
	return slug
}

func amDerefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func amOptInt64(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

func amDerefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
