package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const videoTaskHoldPrefix = "video_task:"

func NewVideoTaskHoldID() string {
	return videoTaskHoldPrefix + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func videoTaskHoldCommand(requestID, taskID string, userID, apiKeyID int64, holdAmount, actualAmount float64, payloadHash string) *BatchImageBalanceHoldCommand {
	return &BatchImageBalanceHoldCommand{
		RequestID:          requestID,
		APIKeyID:           apiKeyID,
		UserID:             userID,
		BatchID:            taskID,
		HoldAmount:         holdAmount,
		ActualAmount:       actualAmount,
		RequestPayloadHash: payloadHash,
	}
}

func (s *OpenAIGatewayService) reserveVideoTaskBalance(ctx context.Context, userID, apiKeyID int64, taskID string, amount float64, payloadHash string) error {
	if s == nil || s.usageBillingRepo == nil {
		return errors.New("video task billing repository is unavailable")
	}
	if amount <= 0 {
		return nil
	}
	_, err := s.usageBillingRepo.ReserveBatchImageBalance(ctx, videoTaskHoldCommand(BatchImageHoldRequestID(taskID), taskID, userID, apiKeyID, amount, 0, payloadHash))
	if err == nil {
		if s.billingCacheService != nil {
			_ = s.billingCacheService.InvalidateUserBalance(ctx, userID)
		}
		slog.Info("video_billing.hold_reserved", "task_id", taskID, "user_id", userID, "api_key_id", apiKeyID, "amount", amount)
	} else {
		slog.Warn("video_billing.hold_reserve_failed", "task_id", taskID, "user_id", userID, "api_key_id", apiKeyID, "amount", amount, "error", err)
	}
	return err
}

func (s *OpenAIGatewayService) ReserveVideoTaskBalance(ctx context.Context, userID, apiKeyID int64, taskID string, amount float64, payloadHash string) error {
	return s.reserveVideoTaskBalance(ctx, userID, apiKeyID, taskID, amount, payloadHash)
}

func (s *OpenAIGatewayService) captureVideoTaskBalance(ctx context.Context, task *GrokVideoPendingBilling, userID, apiKeyID int64, actualAmount float64) error {
	if s == nil || s.usageBillingRepo == nil || task == nil || task.HoldAmount <= 0 {
		return nil
	}
	_, err := s.usageBillingRepo.CaptureBatchImageBalance(ctx,
		videoTaskHoldCommand(BatchImageCaptureRequestID(task.HoldID), task.HoldID, userID, apiKeyID, task.HoldAmount, actualAmount, StableVideoTaskBillingRequestID(task.Platform, task.RequestID)))
	if err == nil {
		if s.billingCacheService != nil {
			_ = s.billingCacheService.InvalidateUserBalance(ctx, userID)
		}
		slog.Info("video_billing.hold_captured", "task_id", task.RequestID, "platform", task.Platform, "user_id", userID, "api_key_id", apiKeyID, "hold_amount", task.HoldAmount, "actual_amount", actualAmount, "released_amount", QuantizeUsageBillingAmount(task.HoldAmount-actualAmount))
	} else {
		slog.Warn("video_billing.hold_capture_failed", "task_id", task.RequestID, "platform", task.Platform, "user_id", userID, "api_key_id", apiKeyID, "hold_amount", task.HoldAmount, "actual_amount", actualAmount, "error", err)
	}
	return err
}

func (s *OpenAIGatewayService) releaseVideoTaskBalance(ctx context.Context, task *GrokVideoPendingBilling, userID, apiKeyID int64) error {
	if s == nil || s.usageBillingRepo == nil || task == nil || task.HoldAmount <= 0 {
		return nil
	}
	_, err := s.usageBillingRepo.ReleaseBatchImageBalance(ctx,
		videoTaskHoldCommand(BatchImageReleaseRequestID(task.HoldID), task.HoldID, userID, apiKeyID, task.HoldAmount, 0, StableVideoTaskBillingRequestID(task.Platform, task.RequestID)))
	if err == nil {
		if s.billingCacheService != nil {
			_ = s.billingCacheService.InvalidateUserBalance(ctx, userID)
		}
		slog.Info("video_billing.hold_released", "task_id", task.RequestID, "platform", task.Platform, "user_id", userID, "api_key_id", apiKeyID, "amount", task.HoldAmount)
	} else {
		slog.Warn("video_billing.hold_release_failed", "task_id", task.RequestID, "platform", task.Platform, "user_id", userID, "api_key_id", apiKeyID, "amount", task.HoldAmount, "error", err)
	}
	return err
}

func (s *OpenAIGatewayService) ReleaseVideoTaskBalance(ctx context.Context, task *GrokVideoPendingBilling, userID, apiKeyID int64) error {
	return s.releaseVideoTaskBalance(ctx, task, userID, apiKeyID)
}

// CalculateVideoTaskCost uses the same pricing path as final usage billing,
// but with the deterministic create-time video units.
func (s *OpenAIGatewayService) CalculateVideoTaskCost(ctx context.Context, apiKey *APIKey, account *Account, model, resolution string, durationSeconds int) (float64, error) {
	return s.calculateVideoTaskCostAt(ctx, apiKey, account, model, resolution, durationSeconds, time.Now())
}

func (s *OpenAIGatewayService) calculateVideoTaskCostAt(ctx context.Context, apiKey *APIKey, account *Account, model, resolution string, durationSeconds int, pricingAt time.Time) (float64, error) {
	if s == nil || apiKey == nil || account == nil || apiKey.User == nil {
		return 0, errors.New("video task pricing context is incomplete")
	}
	result := &OpenAIForwardResult{Model: strings.TrimSpace(model), BillingModel: strings.TrimSpace(model), VideoCount: 1, VideoResolution: resolution, VideoDurationSeconds: durationSeconds}
	baseMultiplier := 1.0
	if s.cfg != nil {
		baseMultiplier = s.cfg.Default.RateMultiplier
	}
	if apiKey.GroupID != nil && apiKey.Group != nil {
		baseMultiplier = resolveAccountUserBillingMultiplier(account, s.resolveUserGroupRate(ctx, apiKey.User.ID, *apiKey.GroupID, apiKey.Group.RateMultiplier))
	}
	videoMultiplier := resolveVideoRateMultiplier(apiKey, baseMultiplier)
	candidates := usageBillingModelCandidates(model, model, "", "", model, model)
	candidates = s.filterCNProviderBillingModelCandidates(ctx, account, apiKey, candidates)
	cost, err := s.calculateOpenAIRecordUsageCost(ctx, result, apiKey, account, candidates, baseMultiplier, baseMultiplier, videoMultiplier, baseMultiplier, UsageTokens{}, "", openAILongContextBillingGate(account), pricingAt)
	if err != nil {
		return 0, fmt.Errorf("calculate video task cost: %w", err)
	}
	return QuantizeUsageBillingAmount(cost.ActualCost), nil
}

// SetVideoTaskAPIKeyService supplies the worker with an owner snapshot after
// the gateway service has been constructed (avoiding a constructor cycle).
func (s *OpenAIGatewayService) SetVideoTaskAPIKeyService(v *APIKeyService) {
	if s != nil {
		s.videoTaskAPIKeyService = v
	}
}

func (s *OpenAIGatewayService) StartVideoTaskBillingReconciler() {
	if s == nil {
		return
	}
	if _, ok := s.cache.(VideoTaskBillingCache); !ok || s.videoTaskAPIKeyService == nil {
		return
	}
	go s.runVideoTaskBillingReconciler()
}

func (s *OpenAIGatewayService) runVideoTaskBillingReconciler() {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		queue, ok := s.cache.(VideoTaskBillingCache)
		if !ok {
			cancel()
			return
		}
		key, payload, err := queue.ClaimDueVideoTask(ctx, time.Now())
		if err == nil && key != "" && len(payload) > 0 {
			s.reconcileVideoTask(ctx, key, payload)
		}
		cancel()
		time.Sleep(5 * time.Second)
	}
}

func (s *OpenAIGatewayService) reconcileVideoTask(ctx context.Context, key string, payload []byte) {
	var pending GrokVideoPendingBilling
	if err := json.Unmarshal(payload, &pending); err != nil {
		slog.Error("video_billing.invalid_task_payload", "queue_key", key, "error", err)
		return
	}
	slog.Debug("video_billing.reconcile_started", "task_id", pending.RequestID, "platform", pending.Platform, "user_id", pending.UserID, "api_key_id", pending.APIKeyID, "amount", pending.HoldAmount)
	queue, ok := s.cache.(VideoTaskBillingCache)
	if !ok {
		slog.Error("video_billing.cache_unavailable", "task_id", pending.RequestID)
		return
	}
	account, err := s.accountRepo.GetByID(ctx, pending.AccountID)
	if err != nil || account == nil {
		slog.Warn("video_billing.account_unavailable", "task_id", pending.RequestID, "account_id", pending.AccountID, "error", err)
		_ = s.requeueVideoTask(ctx, queue, key, payload)
		return
	}
	status, result, err := s.pollVideoTaskStatus(ctx, account, pending.Platform, pending.RequestID)
	if err != nil {
		slog.Warn("video_billing.poll_failed", "task_id", pending.RequestID, "platform", pending.Platform, "account_id", pending.AccountID, "error", err)
		_ = s.requeueVideoTask(ctx, queue, key, payload)
		return
	}
	if status == "pending" {
		slog.Debug("video_billing.poll_pending", "task_id", pending.RequestID, "platform", pending.Platform)
		_ = s.requeueVideoTask(ctx, queue, key, payload)
		return
	}
	if status != "succeeded" {
		slog.Info("video_billing.task_failed", "task_id", pending.RequestID, "platform", pending.Platform, "user_id", pending.UserID, "amount", pending.HoldAmount)
		if err := s.releaseVideoTaskBalance(ctx, &pending, pending.UserID, pending.APIKeyID); err != nil {
			_ = s.requeueVideoTask(ctx, queue, key, payload)
			return
		}
		_ = queue.DeleteVideoTaskBilling(ctx, key)
		return
	}
	apiKey, err := s.videoTaskAPIKeyService.GetByID(ctx, pending.APIKeyID)
	if err != nil || apiKey == nil {
		slog.Warn("video_billing.api_key_unavailable", "task_id", pending.RequestID, "api_key_id", pending.APIKeyID, "error", err)
		_ = s.requeueVideoTask(ctx, queue, key, payload)
		return
	}
	apiKey.User = &User{ID: pending.UserID}
	if err := mergeVideoTaskBillingResult(result, &pending); err != nil {
		slog.Error("video_billing.completion_metadata_missing", "task_id", pending.RequestID, "platform", pending.Platform, "user_id", pending.UserID, "error", err)
		_ = s.requeueVideoTask(ctx, queue, key, payload)
		return
	}
	applyVideoTaskTotalDuration(result, &pending, time.Now())
	result.VideoCount = 1
	result.RequestID = StableVideoTaskBillingRequestID(pending.Platform, pending.RequestID)
	pricingAt, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(pending.CreatedAt))
	if pricingAt.IsZero() {
		pricingAt = time.Now()
	}
	actualAmount, err := s.calculateVideoTaskCostAt(ctx, apiKey, account, result.BillingModel, result.VideoResolution, result.VideoDurationSeconds, pricingAt)
	if err != nil {
		slog.Warn("video_billing.actual_cost_failed", "task_id", pending.RequestID, "platform", pending.Platform, "user_id", pending.UserID, "error", err)
		_ = s.requeueVideoTask(ctx, queue, key, payload)
		return
	}
	if err := s.captureVideoTaskBalance(ctx, &pending, pending.UserID, pending.APIKeyID, actualAmount); err != nil {
		_ = s.requeueVideoTask(ctx, queue, key, payload)
		return
	}
	if err := s.RecordUsage(ctx, &OpenAIRecordUsageInput{Result: result, APIKey: apiKey, User: apiKey.User, Account: account, APIKeyService: s.videoTaskAPIKeyService, BalanceAlreadyReserved: true, SettledBalanceCost: &actualAmount, PricingAt: pricingAt, QuotaPlatform: pending.Platform}); err != nil {
		slog.Warn("video_billing.usage_record_failed", "task_id", pending.RequestID, "platform", pending.Platform, "user_id", pending.UserID, "hold_amount", pending.HoldAmount, "actual_amount", actualAmount, "error", err)
		_ = s.requeueVideoTask(ctx, queue, key, payload)
		return
	}
	slog.Info("video_billing.settled", "task_id", pending.RequestID, "platform", pending.Platform, "user_id", pending.UserID, "api_key_id", pending.APIKeyID, "hold_amount", pending.HoldAmount, "actual_amount", actualAmount)
	_ = queue.DeleteVideoTaskBilling(ctx, key)
}

func applyVideoTaskTotalDuration(result *OpenAIForwardResult, pending *GrokVideoPendingBilling, discoveredAt time.Time) {
	if result == nil || pending == nil {
		return
	}
	createdAtUnix := result.VideoCreatedAtUnix
	if createdAtUnix <= 0 {
		createdAtUnix = pending.UpstreamCreatedAtUnix
	}
	if createdAtUnix > 0 && result.VideoCompletedAtUnix >= createdAtUnix {
		result.Duration = time.Duration(result.VideoCompletedAtUnix-createdAtUnix) * time.Second
		return
	}
	if localDuration := GrokVideoE2EDuration(pending.CreatedAt, discoveredAt); localDuration > 0 {
		result.Duration = localDuration
	}
}

func mergeVideoTaskBillingResult(result *OpenAIForwardResult, pending *GrokVideoPendingBilling) error {
	if result == nil || pending == nil {
		return errors.New("video task completion result is missing")
	}
	if strings.TrimSpace(result.Model) == "" {
		result.Model = pending.Model
	}
	if strings.TrimSpace(result.BillingModel) == "" {
		result.BillingModel = firstNonEmpty(result.Model, pending.BillingModel, pending.Model)
	}
	if result.VideoDurationSeconds <= 0 {
		result.VideoDurationSeconds = pending.VideoDurationSeconds
	}
	if strings.TrimSpace(result.VideoResolution) == "" {
		result.VideoResolution = pending.VideoResolution
	}
	if result.VideoDurationSeconds <= 0 {
		return errors.New("video completion duration is unavailable")
	}
	if strings.TrimSpace(result.VideoResolution) == "" {
		return errors.New("video completion resolution is unavailable")
	}
	result.VideoDurationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(result.VideoDurationSeconds)
	result.VideoResolution = NormalizeVideoBillingResolutionOrDefault(result.VideoResolution)
	return nil
}

func (s *OpenAIGatewayService) requeueVideoTask(ctx context.Context, queue VideoTaskBillingCache, key string, payload []byte) error {
	err := queue.SetVideoTaskBilling(ctx, key, payload, time.Now().Add(15*time.Second), 30*24*time.Hour)
	if err != nil {
		slog.Error("video_billing.requeue_failed", "queue_key", key, "error", err)
	}
	return err
}

func (s *OpenAIGatewayService) pollVideoTaskStatus(ctx context.Context, account *Account, platform, requestID string) (string, *OpenAIForwardResult, error) {
	token, _, err := s.GetRequestCredential(ctx, nil, account)
	if err != nil {
		return "", nil, err
	}
	var url string
	if platform == VideoTaskPlatformOpenAI {
		url = buildOpenAIEndpointURL(account.GetOpenAIFormatBaseURL(), "/v1/videos/"+requestID)
	} else {
		url, err = buildGrokMediaURL(account, s.cfg, GrokMediaEndpointVideoStatus, requestID)
	}
	if err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	account.ApplyHeaderOverrides(req.Header)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return "", nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode >= 400 {
		return "", nil, fmt.Errorf("video status returned %d", resp.StatusCode)
	}
	if platform == VideoTaskPlatformOpenAI {
		result := parseOpenAIVideoResult(body, requestID, 0)
		if result.VideoCount > 0 {
			return "succeeded", result, nil
		}
		status := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "status").String()))
		if status == "failed" || status == "cancelled" {
			return "failed", result, nil
		}
		return "pending", result, nil
	}
	if IsGrokVideoStatusBillable(body) {
		result := ExtractGrokVideoBillingFromStatusBody(body, nil, requestID)
		return "succeeded", result, nil
	}
	status := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "status").String()))
	if status == "failed" || status == "expired" || status == "cancelled" {
		return "failed", nil, nil
	}
	return "pending", nil, nil
}
