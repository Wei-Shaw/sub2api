package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
	"github.com/Wei-Shaw/sub2api/internal/pkg/leonardo"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
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
	taskRepo               AsyncMediaTaskRepository
	userRepo               UserRepository
	groupRepo              GroupRepository
	billing                *BillingService
	resolver               *ModelPricingResolver
	cos                    *COSImageTransferService
	deferred               *DeferredService
	billingContextResolver *BillingContextResolver
	balanceCache           interface {
		InvalidateUserBalance(ctx context.Context, userID int64) error
	}

	pollInterval time.Duration
	failTimeout  time.Duration
}

func (s *AsyncMediaService) SetBalanceCache(cache interface {
	InvalidateUserBalance(ctx context.Context, userID int64) error
}) {
	if s != nil {
		s.balanceCache = cache
	}
}

func (s *AsyncMediaService) SetBillingContextResolver(resolver *BillingContextResolver) {
	if s != nil {
		s.billingContextResolver = resolver
	}
}

// NewAsyncMediaService 创建异步媒体执行内核。
//
// groupRepo 用于 estimateCost 在渠道无定价时回查分组二维价格矩阵；可为 nil，
// 此时仅依赖 resolver 解析的渠道定价（与历史行为一致）。
func NewAsyncMediaService(
	taskRepo AsyncMediaTaskRepository,
	userRepo UserRepository,
	groupRepo GroupRepository,
	billing *BillingService,
	resolver *ModelPricingResolver,
	cos *COSImageTransferService,
) *AsyncMediaService {
	return &AsyncMediaService{
		taskRepo:     taskRepo,
		userRepo:     userRepo,
		groupRepo:    groupRepo,
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

	BillingType       int8    // 0=balance / 1=subscription
	RateMultiplier    float64 // 下游图片计费倍率
	RateMultiplierSet bool    // true 时保留显式 0 倍率

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
	if strings.TrimSpace(in.InternalRequestID) == "" {
		// The Leonardo proxy requires an idempotency key for every submit. Keep
		// direct service callers safe as well as the HTTP facade, which normally
		// supplies the client request ID.
		in.InternalRequestID = uuid.NewString()
	}
	if !in.RateMultiplierSet && in.RateMultiplier == 0 {
		in.RateMultiplier = 1
	}

	upstreamModel := s.resolveUpstreamModel(in.Account, in.RequestedModel, in.Input.IsEdit)
	rawSize := in.Input.Size
	sizeTier := NormalizeImageBillingTierOrDefault(rawSize)
	quality := fal.MapQualityToFal(in.Input.Quality)
	numImages := in.Input.N
	if numImages <= 0 {
		numImages = 1
	}

	// 预估并预扣费用（按 num_images 的满额预扣）。
	_, heldCost, err := s.estimateCost(ctx, upstreamModel, in.GroupID, rawSize, sizeTier, quality, numImages, in.RateMultiplier)
	if err != nil {
		return nil, fmt.Errorf("async media: estimate cost: %w", err)
	}
	billingContext := &BillingContext{ConsumerUserID: in.UserID, PayerUserID: in.UserID, BalanceSource: BalanceSourceSelf}
	if s.billingContextResolver != nil {
		billingContext, err = s.billingContextResolver.ResolveForAmount(ctx, in.UserID, heldCost)
		if err != nil {
			return nil, fmt.Errorf("async media: resolve payer: %w", err)
		}
	}
	if err := s.charge(ctx, in.BillingType, billingContext, heldCost); err != nil {
		return nil, fmt.Errorf("async media: pre-charge: %w", err)
	}

	failDeadline := time.Now().Add(s.failTimeout)
	task := &AsyncMediaTask{
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
		// ImageSize 存客户端原始 size（如 "1024x1024"），结算阶段据此命中分组二维矩阵；
		// 已归一化的计费档位（"2K" 等）单独存入 SizeTier 字段。
		ImageSize:        amStrPtr(rawSize),
		Quality:          amStrPtr(quality),
		NumImages:        numImages,
		Status:           AsyncMediaStatusPending,
		HeldCost:         heldCost,
		RateMultiplier:   in.RateMultiplier,
		SizeTier:         amStrPtr(sizeTier),
		FailDeadlineAt:   &failDeadline,
		ClientIP:         amStrPtr(in.ClientIP),
		UserAgent:        amStrPtr(in.UserAgent),
		InboundEndpoint:  amStrPtr(in.InboundEndpoint),
		UpstreamEndpoint: amStrPtr(asyncImageUpstreamEndpoint(in.Account, upstreamModel)),
	}
	if err := s.taskRepo.Create(ctx, task); err != nil {
		// 落库失败：回滚预扣费，避免漏退。
		s.refund(ctx, in.BillingType, billingContext, heldCost)
		return nil, fmt.Errorf("async media: create task: %w", err)
	}

	requestID, statusURL, responseURL, err := s.submitUpstream(ctx, in, upstreamModel)
	if err != nil {
		s.markFailedAndRefund(ctx, task, in.BillingType, "submit: "+err.Error())
		return task, fmt.Errorf("async media: submit: %w", err)
	}
	if err := s.taskRepo.UpdateUpstreamRef(ctx, task.ID, requestID, statusURL, responseURL); err != nil {
		logger.L().Warn("async_media.update_upstream_ref_failed",
			zap.Int64("task_id", task.ID), zap.Error(err))
	}
	task.UpstreamRequestID = amStrPtr(requestID)
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
	if account != nil && account.Platform == PlatformFal && task.UpstreamRequestID != nil {
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
	if task.IsTerminal() {
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
	if account != nil && account.Platform == PlatformLeonardo {
		return s.pollLeonardoOnce(ctx, task, account, billingType)
	}
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

	imageURLs, imageOutputSizes := extractFalImageResult(result)
	if len(imageURLs) == 0 {
		s.markFailedAndRefund(ctx, task, billingType, "upstream returned no images")
		return task, true, nil
	}

	s.markSucceeded(ctx, task, account.BillingRateMultiplier(), billingType, imageURLs, imageOutputSizes)
	return task, true, nil
}

// markSucceeded 成功结算：转存 COS、结算 finalCost（退差）、置 succeeded、终态写 usage_log。
func (s *AsyncMediaService) markSucceeded(ctx context.Context, task *AsyncMediaTask, accountRateMultiplier float64, billingType int8, imageURLs, imageOutputSizes []string) {
	// COS 转存成功时复用已下载图片解析尺寸；未转存或 fal 未返回宽高时，再从原图文件头补测。
	var cosURLs []string
	imageOutputSizes = mergeImageOutputSizes(imageOutputSizes, nil, len(imageURLs))
	if s.cos != nil {
		if s.cos.IsEnabled(ctx) {
			transferred, transferredSizes, ok := s.cos.TransferImagesWithSizes(ctx, imageURLs)
			imageOutputSizes = mergeImageOutputSizes(imageOutputSizes, transferredSizes, len(imageURLs))
			if ok {
				cosURLs = transferred
			}
		}
		if hasMissingImageOutputSize(imageOutputSizes) {
			detectedSizes := s.cos.DetectImageSizes(ctx, imageURLs)
			imageOutputSizes = mergeImageOutputSizes(imageOutputSizes, detectedSizes, len(imageURLs))
		}
	}
	logger.L().Info("async_media.image_output_sizes_resolved",
		zap.Int64("task_id", task.ID),
		zap.String("request_id", task.InternalRequestID),
		zap.Strings("image_output_sizes", imageOutputSizes))

	// 结算 finalCost：按实际出图数量重算。
	upstreamModel := amDerefStr(task.UpstreamModel)
	rawSize := amDerefStr(task.ImageSize)
	sizeTier := amDerefStr(task.SizeTier)
	quality := amDerefStr(task.Quality)
	finalTotalCost, finalCost, err := s.estimateCost(ctx, upstreamModel, task.GroupID, rawSize, sizeTier, quality, len(imageURLs), task.RateMultiplier)
	if err != nil {
		// 结算失败时按预扣额结算，避免误退。
		finalCost = task.HeldCost
		finalTotalCost = asyncMediaBaseCost(task.HeldCost, task.RateMultiplier)
	}
	if finalCost > task.HeldCost {
		finalCost = task.HeldCost // 预扣为上限，不超额扣费
		finalTotalCost = asyncMediaBaseCost(finalCost, task.RateMultiplier)
	}

	updated, err := s.taskRepo.MarkSucceeded(ctx, task.ID, imageURLs, cosURLs, finalCost)
	if err != nil {
		logger.L().Error("async_media.mark_succeeded_failed", zap.Int64("task_id", task.ID), zap.Error(err))
		return
	}
	if !updated {
		// 可能已被另一条轮询路径成功终结。回读任务并刷新当前对象，避免外层把旧的 running task 当失败处理。
		current, getErr := s.taskRepo.GetByID(ctx, task.ID)
		if getErr != nil {
			logger.L().Warn("async_media.mark_succeeded_reload_failed", zap.Int64("task_id", task.ID), zap.Error(getErr))
			return
		}
		if current == nil || current.Status != AsyncMediaStatusSucceeded {
			status := ""
			if current != nil {
				status = current.Status
			}
			logger.L().Info("async_media.mark_succeeded_skipped_terminal", zap.Int64("task_id", task.ID), zap.String("current_status", status))
			return
		}
		*task = *current
		repairTotalCost := finalTotalCost
		repairCost := task.FinalCost
		if repairCost <= 0 {
			repairCost = finalCost
			if task.RateMultiplier > 0 {
				repairTotalCost = asyncMediaBaseCost(repairCost, task.RateMultiplier)
			}
		}
		repairImages := task.ImageURLs
		if len(repairImages) == 0 {
			repairImages = imageURLs
		}
		repairCos := task.CosURLs
		if len(repairCos) == 0 {
			repairCos = cosURLs
		}
		logger.L().Info("async_media.mark_succeeded_already_terminal", zap.Int64("task_id", task.ID), zap.String("request_id", task.InternalRequestID))
		s.writeTerminalUsageLog(ctx, task, billingType, repairTotalCost, repairCost, amFloat64Ptr(accountRateMultiplier), BillingStatusCharged, repairImages, repairCos, imageOutputSizes)
		return
	}
	task.Status = AsyncMediaStatusSucceeded
	task.ImageURLs = imageURLs
	task.CosURLs = cosURLs
	task.FinalCost = finalCost

	// 退还预扣与结算的差额。
	if refundDelta := task.HeldCost - finalCost; refundDelta > 0 {
		s.refund(ctx, billingType, asyncMediaBillingContext(task), refundDelta)
	}

	s.writeTerminalUsageLog(ctx, task, billingType, finalTotalCost, finalCost, amFloat64Ptr(accountRateMultiplier), BillingStatusCharged, imageURLs, cosURLs, imageOutputSizes)
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
		s.refund(ctx, billingType, asyncMediaBillingContext(task), task.HeldCost)
	}
	s.writeTerminalUsageLog(ctx, task, billingType, 0, 0, nil, BillingStatusRefunded, nil, nil, nil)
}

// writeTerminalUsageLog 终态追加写一条 usage_log。
func (s *AsyncMediaService) writeTerminalUsageLog(
	ctx context.Context,
	task *AsyncMediaTask,
	billingType int8,
	totalCost float64,
	actualCost float64,
	accountRateMultiplier *float64,
	billingStatus string,
	imageURLs, cosURLs, imageOutputSizes []string,
) {
	outputMetadata := ResolveImageBillingSize("", imageOutputSizes)
	imageCount := task.NumImages
	if billingStatus == BillingStatusCharged && len(imageURLs) > 0 {
		imageCount = len(imageURLs)
	}
	in := &TerminalUsageLogInput{
		UserID:                task.UserID,
		APIKeyID:              task.APIKeyID,
		AccountID:             amDerefInt64(task.AccountID),
		RequestID:             task.InternalRequestID,
		OrganizationID:        task.OrganizationID,
		PayerUserID:           task.PayerUserID,
		BalanceSource:         task.BalanceSource,
		AuthzGeneration:       task.AuthzGeneration,
		Model:                 amDerefStr(task.UpstreamModel),
		RequestedModel:        task.RequestedModel,
		UpstreamModel:         amDerefStr(task.UpstreamModel),
		GroupID:               task.GroupID,
		ChannelID:             task.ChannelID,
		TotalCost:             totalCost,
		ActualCost:            actualCost,
		RateMultiplier:        task.RateMultiplier,
		AccountRateMultiplier: accountRateMultiplier,
		BillingType:           billingType,
		RequestType:           int16(RequestTypeSync),
		ImageCount:            imageCount,
		ImageSize:             asyncMediaUsageLogImageSize(task),
		ImageInputSize:        amDerefStr(task.ImageSize),
		ImageQuality:          amDerefStr(task.Quality),
		ImageOutputSize:       outputMetadata.OutputSize,
		ImageSizeSource:       asyncMediaUsageLogImageSizeSource(task),
		ImageSizeBreakdown:    outputMetadata.Breakdown,
		BillingTier:           asyncMediaUsageLogImageSize(task),
		TaskID:                task.ID,
		ImageURLs:             imageURLs,
		CosURLs:               cosURLs,
		BillingStatus:         billingStatus,

		ClientIP:         amDerefStr(task.ClientIP),
		UserAgent:        amDerefStr(task.UserAgent),
		InboundEndpoint:  amDerefStr(task.InboundEndpoint),
		UpstreamEndpoint: amDerefStr(task.UpstreamEndpoint),
		DurationMs:       asyncMediaDurationMs(task),
	}
	inserted, err := s.taskRepo.InsertTerminalUsageLog(ctx, in)
	if err != nil {
		logger.L().Warn("async_media.terminal_usage_log_failed",
			zap.Int64("task_id", task.ID),
			zap.String("request_id", task.InternalRequestID),
			zap.String("billing_status", billingStatus),
			zap.Float64("total_cost", totalCost),
			zap.Float64("actual_cost", actualCost),
			zap.String("image_input_size", in.ImageInputSize),
			zap.String("image_output_size", in.ImageOutputSize),
			zap.Error(err))
		return
	}
	if inserted {
		logger.L().Info("async_media.terminal_usage_log_written",
			zap.Int64("task_id", task.ID),
			zap.String("request_id", task.InternalRequestID),
			zap.String("billing_status", billingStatus),
			zap.Float64("total_cost", totalCost),
			zap.Float64("actual_cost", actualCost),
			zap.String("image_input_size", in.ImageInputSize),
			zap.String("image_output_size", in.ImageOutputSize),
			zap.Int("image_count", len(imageURLs)))
	} else {
		logger.L().Warn("async_media.terminal_usage_log_conflict_or_skipped",
			zap.Int64("task_id", task.ID),
			zap.String("request_id", task.InternalRequestID),
			zap.String("billing_status", billingStatus),
			zap.Float64("total_cost", totalCost),
			zap.Float64("actual_cost", actualCost),
			zap.String("image_input_size", in.ImageInputSize),
			zap.String("image_output_size", in.ImageOutputSize),
			zap.Int("image_count", len(imageURLs)))
	}
	if billingStatus == BillingStatusCharged && s.billingContextResolver != nil {
		billing := &BillingContext{ConsumerUserID: task.UserID, OrganizationID: task.OrganizationID, PayerUserID: asyncMediaPayerID(task), BalanceSource: amDerefStr(task.BalanceSource)}
		if err := s.billingContextResolver.RecordSpendLimitAlert(ctx, billing); err != nil {
			logger.L().Warn("async_media.spend_limit_alert_failed", zap.Int64("task_id", task.ID), zap.Error(err))
		}
	}
}

func asyncMediaUsageLogImageSize(task *AsyncMediaTask) string {
	if task == nil {
		return ImageBillingSize2K
	}
	if tier, ok := ClassifyImageBillingTier(amDerefStr(task.SizeTier)); ok {
		return tier
	}
	return NormalizeImageBillingTierOrDefault(amDerefStr(task.ImageSize))
}

func asyncMediaUsageLogImageSizeSource(task *AsyncMediaTask) string {
	if task != nil {
		if _, ok := ClassifyImageBillingTier(amDerefStr(task.ImageSize)); ok {
			return ImageSizeSourceInput
		}
	}
	return ImageSizeSourceDefault
}

func asyncMediaPayerID(task *AsyncMediaTask) int64 {
	if task != nil && task.PayerUserID != nil && *task.PayerUserID > 0 {
		return *task.PayerUserID
	}
	if task != nil {
		return task.UserID
	}
	return 0
}

func asyncMediaBillingContext(task *AsyncMediaTask) *BillingContext {
	if task == nil {
		return nil
	}
	return &BillingContext{
		ConsumerUserID:  task.UserID,
		OrganizationID:  task.OrganizationID,
		PayerUserID:     asyncMediaPayerID(task),
		BalanceSource:   amDerefStr(task.BalanceSource),
		AuthzGeneration: amDerefInt64(task.AuthzGeneration),
	}
}

func amInt64Ptr(value int64) *int64       { return &value }
func amFloat64Ptr(value float64) *float64 { return &value }

func asyncMediaBaseCost(actualCost, rateMultiplier float64) float64 {
	if rateMultiplier > 0 {
		return actualCost / rateMultiplier
	}
	return actualCost
}

// estimateCost 通过统一计费入口估算 (size × quality × count) 的实际费用。
//
// 计费优先级（与 OpenAI 网关 calculateOpenAIImageCost 保持一致）：
//  1. 渠道定价（resolved.Source == PricingSourceChannel）且模式为 PerRequest/Image
//     → 走 CalculateCostUnified（按 size_tier 命中渠道分层价或默认按次价）。
//  2. 否则回退分组二维价格矩阵（image_pricing_matrix），按原始分辨率 + quality
//     命中后调用 CalculateImageCostWithQuality；旧三档（Price1K/2K/4K）作为兼容兜底。
//
// 强校验：两条路径均未拿到正向费用时返回 ErrAsyncMediaPricingMissing，
// 调用方据此拒绝提交任务，避免账户被「0 费用刷图」。
func (s *AsyncMediaService) estimateCost(
	ctx context.Context,
	model string, groupID *int64,
	rawSize, sizeTier, quality string, count int, rateMultiplier float64,
) (float64, float64, error) {
	if s.billing == nil {
		return 0, 0, fmt.Errorf("%w: billing service not initialized", ErrAsyncMediaPricingMissing)
	}
	if s.resolver == nil {
		return 0, 0, fmt.Errorf("%w: pricing resolver not initialized", ErrAsyncMediaPricingMissing)
	}
	if count <= 0 {
		count = 1
	}

	resolved := s.resolver.Resolve(ctx, PricingInput{
		Model:   model,
		GroupID: groupID,
	})

	// 路径 1：渠道定价且模式为 PerRequest / Image，走统一计费。
	if resolved != nil && resolved.Source == PricingSourceChannel &&
		(resolved.Mode == BillingModePerRequest || resolved.Mode == BillingModeImage) {
		if len(resolved.RequestTiers) > 0 || resolved.DefaultPerRequestPrice > 0 {
			breakdown, err := s.billing.CalculateCostUnified(CostInput{
				Ctx:            ctx,
				Model:          model,
				GroupID:        groupID,
				RequestCount:   count,
				SizeTier:       imageBillingSizeOrTier(rawSize),
				Quality:        quality,
				RateMultiplier: rateMultiplier,
				Resolver:       s.resolver,
				Resolved:       resolved,
			})
			if err != nil && !errors.Is(err, ErrModelPricingUnavailable) {
				return 0, 0, err
			}
			if err == nil && breakdown != nil && (rateMultiplier <= 0 || breakdown.ActualCost > 0) {
				return breakdown.TotalCost, breakdown.ActualCost, nil
			}
			// 渠道定价计算未拿到正向费用：继续走分组兜底（沉默回退，与 openai 路径一致）。
		}
	}

	// 路径 2：分组二维价格矩阵兜底。
	groupCfg, err := s.buildGroupImagePriceConfig(ctx, groupID, rawSize, quality)
	if err != nil {
		return 0, 0, err
	}
	if groupCfg == nil {
		return 0, 0, fmt.Errorf("%w: model=%s group=%v no channel pricing and group not loadable",
			ErrAsyncMediaPricingMissing, model, groupID)
	}
	breakdown, err := s.billing.CalculateImageCostWithQualityValidated(model, imageBillingSizeOrTier(rawSize), quality, count, groupCfg, rateMultiplier)
	if err != nil {
		return 0, 0, err
	}
	if breakdown == nil {
		return 0, 0, fmt.Errorf("%w: model=%s empty group breakdown", ErrAsyncMediaPricingMissing, model)
	}
	if rateMultiplier > 0 && breakdown.ActualCost <= 0 {
		return 0, 0, fmt.Errorf("%w: model=%s size=%s quality=%s group fallback zero cost",
			ErrAsyncMediaPricingMissing, model, sizeTier, quality)
	}
	return breakdown.TotalCost, breakdown.ActualCost, nil
}

// buildGroupImagePriceConfig 拉取分组并构造带原始分辨率/quality 的 ImagePriceConfig，
// 用于命中分组的二维价格矩阵 (tier_key × quality_key)。
//
// 当 groupRepo 未注入或 groupID 为空、加载失败时返回 (nil, error)；分组本身缺失
// image_pricing_matrix 等字段不视为错误，仍返回 cfg 由上层 CalculateImageCost 兜底。
func (s *AsyncMediaService) buildGroupImagePriceConfig(ctx context.Context, groupID *int64, rawSize, quality string) (*ImagePriceConfig, error) {
	if s.groupRepo == nil || groupID == nil || *groupID == 0 {
		return nil, nil
	}
	group, err := s.groupRepo.GetByID(ctx, *groupID)
	if err != nil {
		return nil, fmt.Errorf("%w: load group %d: %v", ErrAsyncMediaPricingMissing, *groupID, err)
	}
	if group == nil {
		return nil, nil
	}
	rawW, rawH, _ := parseImageBillingDimensions(rawSize)
	return group.BuildImagePriceConfig(rawW, rawH, quality), nil
}

// charge 预扣费用（仅 BillingTypeBalance 走余额账本）。
func (s *AsyncMediaService) charge(ctx context.Context, billingType int8, billing *BillingContext, amount float64) error {
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
	if err := s.userRepo.DeductBalance(ctx, billing.PayerUserID, amount); err != nil {
		return err
	}
	if s.balanceCache != nil {
		if err := s.balanceCache.InvalidateUserBalance(ctx, billing.PayerUserID); err != nil {
			logger.L().Warn("async_media.balance_cache_invalidate_failed", zap.Int64("payer_user_id", billing.PayerUserID), zap.Error(err))
		}
	}
	return nil
}

// refund 退还费用（仅 BillingTypeBalance 走余额账本）。
func (s *AsyncMediaService) refund(ctx context.Context, billingType int8, billing *BillingContext, amount float64) {
	if amount <= 0 || billingType != BillingTypeBalance || s.userRepo == nil {
		return
	}
	if billing == nil {
		return
	}
	if billing.UsesCompanyBalance() {
		if s.billingContextResolver == nil {
			logger.L().Error("async_media.refund_failed", zap.Error(errors.New("organization balance resolver is unavailable")))
			return
		}
		if _, err := s.billingContextResolver.CreditOrganizationBalance(ctx, billing, amount); err != nil {
			logger.L().Error("async_media.refund_failed", zap.Int64("organization_id", *billing.OrganizationID), zap.Float64("amount", amount), zap.Error(err))
		}
		return
	}
	if err := s.userRepo.UpdateBalance(ctx, billing.PayerUserID, amount); err != nil {
		logger.L().Error("async_media.refund_failed",
			zap.Int64("user_id", billing.PayerUserID), zap.Float64("amount", amount), zap.Error(err))
		return
	}
	if s.balanceCache != nil {
		if err := s.balanceCache.InvalidateUserBalance(ctx, billing.PayerUserID); err != nil {
			logger.L().Warn("async_media.balance_cache_invalidate_failed", zap.Int64("payer_user_id", billing.PayerUserID), zap.Error(err))
		}
	}
}

// resolveUpstreamModel 解析客户端模型到 fal 上游 slug。
// 账号/渠道自定义映射优先，缺失时按是否为编辑请求选择内置默认 slug。
func (s *AsyncMediaService) resolveUpstreamModel(account *Account, requestedModel string, isEdit bool) string {
	if account != nil && account.Platform == PlatformLeonardo {
		if mapped := strings.TrimSpace(account.GetModelMapping()[requestedModel]); mapped != "" {
			return mapped
		}
		return "gpt-image-2"
	}
	return resolveFalUpstreamModel(account, requestedModel, isEdit)
}

func (s *AsyncMediaService) submitUpstream(ctx context.Context, in *AsyncMediaSubmitInput, upstreamModel string) (string, string, string, error) {
	if in.Account.Platform == PlatformLeonardo {
		client, err := s.newLeonardoClient(in.Account)
		if err != nil {
			return "", "", "", err
		}
		request := leonardo.BuildSubmitRequest(upstreamModel, in.Input, in.Account.LeonardoEstimatedCreditCost())
		idempotencyKey := "sub2api-" + strings.TrimSpace(in.InternalRequestID)
		task, err := client.Submit(ctx, request, idempotencyKey)
		if err != nil {
			return "", "", "", err
		}
		taskURL := client.BuildTaskURL(task.TaskUUID)
		return task.TaskUUID, taskURL, taskURL, nil
	}
	client, err := s.newClient(in.Account)
	if err != nil {
		return "", "", "", err
	}
	request := fal.BuildRequest(in.Input)
	response, err := client.Submit(ctx, upstreamModel, request)
	if err != nil {
		return "", "", "", err
	}
	statusURL := response.StatusURL
	if statusURL == "" {
		statusURL = client.BuildStatusURL(upstreamModel, response.RequestID)
	}
	responseURL := response.ResponseURL
	if responseURL == "" {
		responseURL = client.BuildResponseURL(upstreamModel, response.RequestID)
	}
	return response.RequestID, statusURL, responseURL, nil
}

func (s *AsyncMediaService) pollLeonardoOnce(ctx context.Context, task *AsyncMediaTask, account *Account, billingType int8) (*AsyncMediaTask, bool, error) {
	client, err := s.newLeonardoClient(account)
	if err != nil {
		return task, false, fmt.Errorf("async media poll: build leonardo client: %w", err)
	}
	requestID := amDerefStr(task.UpstreamRequestID)
	upstreamTask, err := client.GetTask(ctx, requestID)
	if err != nil {
		var apiErr *leonardo.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			s.markFailedAndRefund(ctx, task, billingType, fmt.Sprintf("status %d: %s", apiErr.StatusCode, apiErr.Body))
			return task, true, nil
		}
		return task, false, nil
	}
	if upstreamTask.IsFailed() {
		s.markFailedAndRefund(ctx, task, billingType, upstreamTask.FailureMessage())
		return task, true, nil
	}
	if !upstreamTask.IsCompleted() {
		return task, false, nil
	}
	imageURLs, imageOutputSizes := extractLeonardoImageResult(upstreamTask)
	if len(imageURLs) == 0 {
		s.markFailedAndRefund(ctx, task, billingType, "upstream returned no images")
		return task, true, nil
	}
	s.markSucceeded(ctx, task, account.BillingRateMultiplier(), billingType, imageURLs, imageOutputSizes)
	return task, true, nil
}

// resolveFalUpstreamModel 解析客户端模型到 fal 上游 slug（账号/渠道自定义映射优先，
// 缺失时按是否为编辑请求选择内置默认 slug）。抽成包级函数供异步执行与选号阶段共用，
// 确保「选号时的定价预判」与「提交时的实际计费」使用同一上游模型。
func resolveFalUpstreamModel(account *Account, requestedModel string, isEdit bool) string {
	var mapping map[string]string
	if account != nil {
		mapping = account.GetModelMapping()
	}
	mapped := strings.TrimSpace(mapping[requestedModel])
	if !isEdit {
		if mapped != "" {
			return mapped
		}
		return domain.FalSlugTextToImage
	}

	// 直接映射本身已经是 edit endpoint 时原样使用。
	if _, api := falEndpointModelAPI(mapped); strings.EqualFold(api, FalAPIEdit) {
		return mapped
	}

	// /images/edits 可能仍以基础模型名请求；从同一账号映射中寻找同模型的 edit endpoint。
	targetModel, _ := falEndpointModelAPI(mapped)
	if targetModel == "" {
		targetModel, _ = falEndpointModelAPI(requestedModel)
	}
	bestKey := ""
	editEndpoint := ""
	for key, endpoint := range mapping {
		model, api := falEndpointModelAPI(endpoint)
		if !strings.EqualFold(api, FalAPIEdit) || !strings.EqualFold(model, targetModel) {
			continue
		}
		if bestKey == "" || key < bestKey {
			bestKey = key
			editEndpoint = strings.TrimSpace(endpoint)
		}
	}
	if editEndpoint != "" {
		return editEndpoint
	}

	// 自定义映射只有基础 endpoint 时，保持其 organization/model 并切换到 edit API。
	if mapped != "" {
		if _, api := falEndpointModelAPI(mapped); api == "" {
			return strings.TrimRight(mapped, "/") + "/" + FalAPIEdit
		}
	}
	return domain.FalSlugImageEdit
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

func (s *AsyncMediaService) newLeonardoClient(account *Account) (*leonardo.Client, error) {
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	return leonardo.NewClient(leonardo.Config{
		APIKey:   account.LeonardoAPIKey(),
		BaseURL:  account.LeonardoBaseURL(),
		ProxyURL: proxyURL,
	})
}

// ----- helpers -----

func extractFalImageResult(resp *fal.Response) ([]string, []string) {
	if resp == nil {
		return nil, nil
	}
	urls := make([]string, 0, len(resp.Images))
	sizes := make([]string, 0, len(resp.Images))
	for _, img := range resp.Images {
		if u := strings.TrimSpace(img.URL); u != "" {
			urls = append(urls, u)
			size := ""
			if img.Width > 0 && img.Height > 0 {
				size = fmt.Sprintf("%dx%d", img.Width, img.Height)
			}
			sizes = append(sizes, size)
		}
	}
	return urls, sizes
}

func extractLeonardoImageResult(task *leonardo.Task) ([]string, []string) {
	if task == nil {
		return nil, nil
	}
	urls := make([]string, 0, len(task.Output.Media))
	sizes := make([]string, 0, len(task.Output.Media))
	for _, media := range task.Output.Media {
		if url := strings.TrimSpace(media.URL); url != "" {
			urls = append(urls, url)
			size := ""
			if media.Width > 0 && media.Height > 0 {
				size = fmt.Sprintf("%dx%d", media.Width, media.Height)
			}
			sizes = append(sizes, size)
		}
	}
	return urls, sizes
}

func mergeImageOutputSizes(current, fallback []string, count int) []string {
	if count < 0 {
		count = 0
	}
	out := make([]string, count)
	copy(out, current)
	for i := 0; i < count && i < len(fallback); i++ {
		if strings.TrimSpace(out[i]) == "" {
			out[i] = strings.TrimSpace(fallback[i])
		}
	}
	return out
}

func hasMissingImageOutputSize(sizes []string) bool {
	for _, size := range sizes {
		if strings.TrimSpace(size) == "" {
			return true
		}
	}
	return false
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

func asyncImageUpstreamEndpoint(account *Account, upstreamModel string) string {
	if account != nil && account.Platform == PlatformLeonardo {
		return strings.TrimRight(account.LeonardoBaseURL(), "/") + "/v1/tasks"
	}
	return falUpstreamEndpoint(upstreamModel)
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
