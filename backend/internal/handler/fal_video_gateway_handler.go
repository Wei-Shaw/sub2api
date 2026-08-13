package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// FalVideoGatewayHandler 处理视频异步平台的对外统一门面（原生 fal 协议，路径 /api/v1/model/*path）。
//
// 与 FalGatewayHandler 平行独立：
//   - 图片走 /fal/*path，走 AsyncMediaService，透传 fal.Response{Images}
//   - 视频走 /api/v1/model/*path，走 AsyncVideoService，透传上游 result 原始 JSON
//
// 路径形态：
//
//	POST /api/v1/model/{slug}                         -> submit
//	GET  /api/v1/model/{slug}/requests/{id}/status    -> status
//	GET  /api/v1/model/{slug}/requests/{id}           -> result（透传上游 result payload）
//	PUT  /api/v1/model/{slug}/requests/{id}/cancel    -> cancel
type FalVideoGatewayHandler struct {
	gatewayService *service.GatewayService
	accountService *service.AccountService
	videoService   *service.AsyncVideoService
}

// NewFalVideoGatewayHandler 创建视频门面。
func NewFalVideoGatewayHandler(
	gatewayService *service.GatewayService,
	accountService *service.AccountService,
	videoService *service.AsyncVideoService,
) *FalVideoGatewayHandler {
	return &FalVideoGatewayHandler{
		gatewayService: gatewayService,
		accountService: accountService,
		videoService:   videoService,
	}
}

func (h *FalVideoGatewayHandler) jsonError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// Native 是 /api/v1/model/*path 的统一入口，按 method + suffix 分发。
func (h *FalVideoGatewayHandler) Native(c *gin.Context) {
	path := strings.Trim(c.Param("path"), "/")
	method := c.Request.Method

	switch {
	case method == http.MethodGet && strings.HasSuffix(path, "/status"):
		h.nativeStatus(c, videoRequestIDFromPath(path))
	case method == http.MethodPut && strings.HasSuffix(path, "/cancel"):
		h.nativeCancel(c, videoRequestIDFromPath(path))
	case method == http.MethodGet && strings.Contains(path, "/requests/"):
		h.nativeResult(c, videoRequestIDFromPath(path))
	case method == http.MethodPost:
		h.nativeSubmit(c, path)
	default:
		h.jsonError(c, http.StatusNotFound, "not_found_error", "Unsupported video endpoint")
	}
}

func (h *FalVideoGatewayHandler) nativeSubmit(c *gin.Context, model string) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.jsonError(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.jsonError(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := logger.FromContext(c.Request.Context()).With(
		zap.String("component", "handler.fal_video.native_submit"),
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.String("model", model),
	)

	// 白名单：仅接收"两段及以上"的模型名（去掉可选 fal-ai/ 前缀后）。
	// 具体某个 model 是否可用，交给分组内 fal 账号的 model_mapping
	// 与"支持视频模型"开关决定；这里只做形态过滤，避免视频门面被用作
	// 任意 fal endpoint 的透传通道。
	if !domain.IsVideoModelName(model) {
		h.jsonError(c, http.StatusNotFound, "not_found_error", "Unsupported video model")
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil || len(body) == 0 {
		h.jsonError(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		h.jsonError(c, http.StatusBadRequest, "invalid_request_error", "Invalid JSON body")
		return
	}

	resolution, duration, aspectRatio := service.ExtractVideoBillingDims(payload)
	// duration 允许为 0：客户端传 duration="auto" 或缺失时按兜底秒数预扣，
	// 完成后按上游 result 里的实际时长重算 finalCost。
	if resolution == "" {
		h.jsonError(c, http.StatusBadRequest, "invalid_request_error", "Missing 'resolution' (e.g. 480p / 720p / 1080p)")
		return
	}

	// 选号：视频链路统一走 /api/v1/model 门面，在当前混合分组内按“该模型属于哪个平台”
	// 选出对应平台账号（fal / atlascloud / apiz），再转发到该账号。
	// slug 自带 api 段（如 .../text-to-video），api 传空串。
	account, err := h.gatewayService.SelectFalAccountInGroup(c.Request.Context(), apiKey.GroupID, "", model, nil, "")
	if err != nil || account == nil {
		reqLog.Warn("fal_video.no_available_account", zap.Error(err))
		h.jsonError(c, http.StatusServiceUnavailable, "api_error", "no available video account")
		return
	}

	billingType := service.BillingTypeBalance
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType() {
		billingType = service.BillingTypeSubscription
	}
	rateMultiplier := 1.0
	if account.RateMultiplier != nil && *account.RateMultiplier > 0 {
		rateMultiplier = *account.RateMultiplier
	}

	// 应用账号级 model_mapping：客户端传入的模型名（如 fal-ai/bytedance/...）
	// 会被映射到该账号对应的真实上游模型名（例如 apiz 账号会把 seedance 系列
	// 映射到 bytedance-seedance-1-0-pro-t2v 之类的上游 model 值）。
	// 未配置或未命中时，退回客户端原始模型名。
	upstreamModel := model
	if mappedModel, matched := account.ResolveMappedModel(model); matched {
		if mappedModel = strings.TrimSpace(mappedModel); mappedModel != "" {
			upstreamModel = mappedModel
		}
	}

	submitInput := &service.AsyncVideoSubmitInput{
		Account:           account,
		User:              apiKey.User,
		APIKeyID:          apiKey.ID,
		UserID:            subject.UserID,
		AccountID:         account.ID,
		GroupID:           apiKey.GroupID,
		Facade:            service.AsyncVideoFacadeFal,
		InternalRequestID: videoInternalRequestID(c),
		RequestedModel:    model,
		UpstreamModel:     upstreamModel,
		RequestPayload:    payload,
		Resolution:        resolution,
		DurationSeconds:   duration,
		AspectRatio:       aspectRatio,
		BillingType:       billingType,
		RateMultiplier:    rateMultiplier,
		ClientIP:          c.ClientIP(),
		UserAgent:         c.GetHeader("User-Agent"),
		InboundEndpoint:   videoInboundEndpoint(c),
	}

	task, err := h.videoService.SubmitAsync(c.Request.Context(), submitInput)
	if err != nil {
		reqLog.Warn("fal_video.submit_failed", zap.Error(err))
		// 余额不足 → 402 Payment Required；防止并发攻击“借用未预扣的余额”烧钱。
		// 前端可根据 402 特化提示（例如"余额不足，请充值后再试"），而不再吞成
		// 通用的 Failed to submit video task 提示。
		if errors.Is(err, service.ErrInsufficientBalance) {
			h.jsonError(c, http.StatusPaymentRequired, "insufficient_balance", "Insufficient balance to hold the estimated cost for this video task. Please recharge and try again.")
			return
		}
		h.jsonError(c, http.StatusBadGateway, "api_error", "Failed to submit video task: "+err.Error())
		return
	}

	reqID := ""
	if task.UpstreamRequestID != nil {
		reqID = *task.UpstreamRequestID
	}
	base := h.videoCallbackBase(c, model, reqID)
	c.JSON(http.StatusOK, fal.SubmitResponse{
		RequestID:   reqID,
		Status:      fal.StatusInQueue,
		StatusURL:   base + "/status",
		ResponseURL: base,
		CancelURL:   base + "/cancel",
	})
}

func (h *FalVideoGatewayHandler) nativeStatus(c *gin.Context, reqID string) {
	task, account := h.loadTaskAndAccount(c, reqID)
	if task == nil {
		return
	}
	if account != nil {
		_, _, _ = h.videoService.AdvanceTask(c.Request.Context(), task, account)
	}
	c.JSON(http.StatusOK, fal.StatusResponse{
		Status:      videoFalStatusFromTask(task),
		RequestID:   reqID,
		ResponseURL: h.videoCallbackBase(c, "", reqID),
	})
}

// nativeResult 返回终态时的 fal 上游 result 原始 payload；未终结返回 202 + 状态。
func (h *FalVideoGatewayHandler) nativeResult(c *gin.Context, reqID string) {
	task, account := h.loadTaskAndAccount(c, reqID)
	if task == nil {
		return
	}
	if account != nil && !task.IsTerminal() {
		if updated, _, _ := h.videoService.AdvanceTask(c.Request.Context(), task, account); updated != nil {
			task = updated
		}
	}
	if !task.IsTerminal() {
		c.JSON(http.StatusAccepted, fal.StatusResponse{Status: videoFalStatusFromTask(task), RequestID: reqID})
		return
	}
	if task.Status != service.AsyncVideoStatusSucceeded {
		reason := "video generation failed"
		if task.ErrorReason != nil && *task.ErrorReason != "" {
			reason = *task.ErrorReason
		}
		h.jsonError(c, http.StatusBadGateway, "api_error", reason)
		return
	}
	// 原样透传 fal result payload（例如 { video: {url, ...}, seed, ... }）。
	if task.ResultPayload == nil {
		task.ResultPayload = map[string]any{}
	}
	c.JSON(http.StatusOK, task.ResultPayload)
}

func (h *FalVideoGatewayHandler) nativeCancel(c *gin.Context, reqID string) {
	task, account := h.loadTaskAndAccount(c, reqID)
	if task == nil {
		return
	}
	if err := h.videoService.CancelTask(c.Request.Context(), task, account); err != nil {
		h.jsonError(c, http.StatusBadGateway, "api_error", "Failed to cancel task")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "CANCELLED", "request_id": reqID})
}

func (h *FalVideoGatewayHandler) loadTaskAndAccount(c *gin.Context, reqID string) (*service.AsyncVideoTask, *service.Account) {
	if strings.TrimSpace(reqID) == "" {
		h.jsonError(c, http.StatusBadRequest, "invalid_request_error", "Missing request id")
		return nil, nil
	}
	task, err := h.videoService.GetTaskByUpstreamID(c.Request.Context(), reqID)
	if err != nil || task == nil {
		h.jsonError(c, http.StatusNotFound, "not_found_error", "Request not found")
		return nil, nil
	}
	if apiKey, ok := middleware2.GetAPIKeyFromContext(c); ok && apiKey.ID != task.APIKeyID {
		h.jsonError(c, http.StatusNotFound, "not_found_error", "Request not found")
		return nil, nil
	}
	var account *service.Account
	if task.AccountID != nil {
		if acc, accErr := h.accountService.GetByID(c.Request.Context(), *task.AccountID); accErr == nil {
			account = acc
		}
	}
	return task, account
}

func (h *FalVideoGatewayHandler) videoCallbackBase(c *gin.Context, model, reqID string) string {
	scheme := "https"
	if c.Request.TLS == nil && c.GetHeader("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	model = strings.Trim(model, "/")
	if model == "" {
		return scheme + "://" + c.Request.Host + "/api/v1/model/requests/" + reqID
	}
	return scheme + "://" + c.Request.Host + "/api/v1/model/" + model + "/requests/" + reqID
}

// ----- helpers -----

func videoInternalRequestID(c *gin.Context) string {
	if v, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v := strings.TrimSpace(c.GetHeader("x-client-request-id")); v != "" {
		return v
	}
	return uuid.New().String()
}

func videoInboundEndpoint(c *gin.Context) string {
	if p := c.FullPath(); p != "" {
		return p
	}
	if c.Request != nil && c.Request.URL != nil {
		return c.Request.URL.Path
	}
	return ""
}

func videoRequestIDFromPath(path string) string {
	_, rest, ok := strings.Cut(path, "/requests/")
	if !ok {
		return ""
	}
	rest = strings.TrimSuffix(rest, "/status")
	rest = strings.TrimSuffix(rest, "/cancel")
	return strings.Trim(rest, "/")
}

func videoFalStatusFromTask(task *service.AsyncVideoTask) string {
	switch task.Status {
	case service.AsyncVideoStatusSucceeded:
		return fal.StatusCompleted
	case service.AsyncVideoStatusPending:
		return fal.StatusInQueue
	default:
		return fal.StatusInProgress
	}
}
