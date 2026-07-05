package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
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

// defaultFalPseudoSyncTimeout 伪同步门面阻塞等待上限（超时返回错误但不退费/不终结）。
const defaultFalPseudoSyncTimeout = 300 * time.Second

// FalGatewayHandler 处理 fal 异步图片平台的对外门面：
//   - OpenAI 伪同步门面（/v1/images/generations、/v1/images/edits，fal 分组）
//   - fal 原生异步门面（/fal/... 的 submit/status/result/cancel）
type FalGatewayHandler struct {
	gatewayService *service.GatewayService
	imagesService  *service.OpenAIGatewayService
	accountService *service.AccountService
	asyncMedia     *service.AsyncMediaService
	cosService     *service.COSImageTransferService
	cfg            *config.Config
}

// NewFalGatewayHandler 创建 fal 门面 handler。
func NewFalGatewayHandler(
	gatewayService *service.GatewayService,
	imagesService *service.OpenAIGatewayService,
	accountService *service.AccountService,
	asyncMedia *service.AsyncMediaService,
	cosService *service.COSImageTransferService,
	cfg *config.Config,
) *FalGatewayHandler {
	return &FalGatewayHandler{
		gatewayService: gatewayService,
		imagesService:  imagesService,
		accountService: accountService,
		asyncMedia:     asyncMedia,
		cosService:     cosService,
		cfg:            cfg,
	}
}

func (h *FalGatewayHandler) pseudoSyncTimeout() time.Duration {
	if h.cfg != nil && h.cfg.AsyncMedia.PseudoSyncTimeoutSeconds > 0 {
		return time.Duration(h.cfg.AsyncMedia.PseudoSyncTimeoutSeconds) * time.Second
	}
	return defaultFalPseudoSyncTimeout
}

func (h *FalGatewayHandler) jsonError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// Images 实现 OpenAI 伪同步门面：提交 fal → 阻塞轮询 → 返回 OpenAI 格式响应（cos_url 优先）。
// POST /v1/images/generations、POST /v1/images/edits（fal 分组）
func (h *FalGatewayHandler) Images(c *gin.Context) {
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
		zap.String("component", "handler.fal_gateway.images"),
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
	)
	imageStatusRequestID := ""
	imageStatusCompleted := false
	imageStatusFailMessage := "image generation failed"
	failImageStatus := func(message string) {
		if strings.TrimSpace(message) != "" {
			imageStatusFailMessage = strings.TrimSpace(message)
		}
	}
	imageStatusRequestID = clientProvidedImageStatusRequestID(c)
	if imageStatusRequestID != "" && h.imagesService != nil {
		ctx := service.WithResponsesImageStatusRequestID(c.Request.Context(), imageStatusRequestID)
		c.Request = c.Request.WithContext(ctx)
		h.imagesService.BeginResponsesImageStatus(ctx, imageStatusRequestID)
		defer func() {
			if !imageStatusCompleted {
				h.imagesService.FailResponsesImageStatus(context.Background(), imageStatusRequestID, imageStatusFailMessage)
			}
		}()
	}

	if !service.GroupAllowsImageGeneration(apiKey.Group) {
		failImageStatus(service.ImageGenerationPermissionMessage())
		h.jsonError(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil || len(body) == 0 {
		failImageStatus("Failed to read request body")
		h.jsonError(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	parsed, err := h.imagesService.ParseOpenAIImagesRequest(c, body)
	if err != nil {
		failImageStatus(err.Error())
		h.jsonError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	account, err := h.selectFalAccount(c, apiKey, parsed.Model, falAPIForOpenAIImages(parsed))
	if err != nil || account == nil {
		failImageStatus("no available fal account")
		h.jsonError(c, http.StatusServiceUnavailable, "api_error", "no available fal account")
		return
	}
	input := buildFalInputFromOpenAI(parsed)
	if imageStatusRequestID != "" && h.imagesService != nil {
		h.imagesService.MarkResponsesImageStatusRunning(c.Request.Context(), imageStatusRequestID)
	}
	imageStatusCompleted = h.runPseudoSync(c, reqLog, apiKey, subject, service.AsyncMediaFacadeOpenAI, parsed.Model, account, input, func(task *service.AsyncMediaTask) {
		h.writeOpenAIImagesResponse(c, reqLog, parsed, task)
	})
}

// SelectMixedImageAccount 在当前 API Key 所属分组内构建 openai + fal 混合候选池，
// 按“优先级 + 最久未用”统一选号（openai/fal 同池竞争，不做 fallback）。
//
// 供 OpenAI 图片门面统一调度使用：返回的账号可能属于 openai 或 fal 平台，调用方据
// account.Platform 分发到对应转发路径。fal 账号所需的 api 段由 parsed 推导。
//
// preferPlatform: ""/"openai" 维持现状；"fal" 反转为 fal 优先 + openai 兜底。
func (h *FalGatewayHandler) SelectMixedImageAccount(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	imageCapability service.OpenAIImagesCapability,
	parsed *service.OpenAIImagesRequest,
	preferPlatform string,
) (*service.Account, error) {
	return h.gatewayService.SelectImageAccountMixed(ctx, groupID, sessionHash, requestedModel, excludedIDs, imageCapability, falAPIForOpenAIImages(parsed), preferPlatform)
}

// ServeOpenAIImagesWithAccount 让已预选的 fal 账号服务 OpenAI 伪同步图片请求：
// 走 fal 伪同步流程（提交 → 阻塞轮询 → 转存 → 写 OpenAI 响应）。
// 计费完全由 AsyncMediaService 承担（预扣/退费 + usage_log），调用方不应再重复记账。
//
// 账号由混合调度预先选出并 hydrate，本方法不再重新选号。
func (h *FalGatewayHandler) ServeOpenAIImagesWithAccount(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	parsed *service.OpenAIImagesRequest,
	account *service.Account,
) bool {
	input := buildFalInputFromOpenAI(parsed)
	return h.runPseudoSync(c, reqLog, apiKey, subject, service.AsyncMediaFacadeOpenAI, parsed.Model, account, input, func(task *service.AsyncMediaTask) {
		h.writeOpenAIImagesResponse(c, reqLog, parsed, task)
	})
}

// writeOpenAIImagesResponse 将异步任务终态结果按 gpt-image 风格的 OpenAI Images API 格式写回：
//   - data[].b64_json：下载出图（COS 转存地址优先）并做 base64 编码，对齐 gpt-image 行为；
//     单张下载失败时回退为 data[].url，避免整体空响应。
//   - 补齐 model/size/quality/background/output_format envelope 字段（取自请求参数）。
//
// 注：fal 上游不提供 OpenAI 风格的 token usage，故不伪造 usage 字段（保持省略）。
func (h *FalGatewayHandler) writeOpenAIImagesResponse(c *gin.Context, reqLog *zap.Logger, parsed *service.OpenAIImagesRequest, task *service.AsyncMediaTask) {
	resp := &fal.OpenAIImagesResponse{
		Created: time.Now().Unix(),
		Data:    []fal.OpenAIImageData{},
	}
	revisedPrompt := ""
	if parsed != nil {
		resp.Model = parsed.Model
		resp.Size = parsed.Size
		resp.Quality = parsed.Quality
		resp.Background = parsed.Background
		resp.OutputFormat = parsed.OutputFormat
		revisedPrompt = parsed.Prompt
	}

	urls := task.ResultURLs()
	reqLog.Info("fal.images.write_response", zap.Int("image_count", len(urls)), zap.Strings("image_urls", urls))
	if h.imagesService != nil {
		result := &service.OpenAIForwardResult{
			ImageOutputURLs: urls,
		}
		if task != nil {
			result.ImageOutputURLs = task.ImageURLs
			result.ImageOutputCosURLs = task.CosURLs
		}
		h.imagesService.MarkResponsesImageStatusUpstreamDone(c.Request.Context(), result)
		h.imagesService.SucceedResponsesImageStatus(c.Request.Context(), result)
	}

	for i, u := range urls {
		item := fal.OpenAIImageData{RevisedPrompt: revisedPrompt}
		if h.cosService != nil {
			reqLog.Info("fal.images.try_b64_encode", zap.String("image_url", u), zap.Int("index", i))
			if b64, err := h.cosService.FetchAsBase64(c.Request.Context(), u); err == nil && b64 != "" {
				item.B64JSON = b64
				reqLog.Info("fal.images.b64_encode_success", zap.String("image_url", u), zap.Int("b64_len", len(b64)))
			} else {
				reqLog.Warn("fal.images.b64_encode_failed_fallback_url", zap.String("image_url", u), zap.Error(err))
				item.URL = u
			}
		} else {
			reqLog.Warn("fal.images.cos_service_nil_fallback_url", zap.String("image_url", u))
			item.URL = u
		}
		resp.Data = append(resp.Data, item)
	}

	c.JSON(http.StatusOK, resp)
}

// falAPIForOpenAIImages 把 OpenAI 图片请求映射为所需的 fal api 段：
// /v1/images/edits（图生图）→ edit，/v1/images/generations（文生图）→ 空串。
func falAPIForOpenAIImages(parsed *service.OpenAIImagesRequest) string {
	if parsed != nil && parsed.IsEdits() {
		return service.FalAPIEdit
	}
	return ""
}

// selectFalAccount 在当前 API Key 所属分组内强制按 fal 平台选号（兼容 fal 分组与 openai 分组）。
// api 表示本次请求所需的 fal api 段（edit=图生图 / 编辑，来自 /v1/images/edits 门面；
// 空串=文生图），用于在选号阶段校验账号是否配置了对应能力的 endpoint；
// 原生 /fal/* 门面的 slug 已自带 api 段，传空串即可。
func (h *FalGatewayHandler) selectFalAccount(c *gin.Context, apiKey *service.APIKey, requestedModel string, api string) (*service.Account, error) {
	account, err := h.gatewayService.SelectFalAccountInGroup(c.Request.Context(), apiKey.GroupID, "", requestedModel, nil, api)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, errors.New("no available fal account")
	}
	return account, nil
}

// runPseudoSync 是伪同步门面的共享主流程：提交 → 阻塞轮询 → 终态回调（账号由调用方预选）。
func (h *FalGatewayHandler) runPseudoSync(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	facade string,
	requestedModel string,
	account *service.Account,
	input fal.ImageGenInput,
	onSuccess func(task *service.AsyncMediaTask),
) bool {
	submitInput := h.buildSubmitInput(c, apiKey, subject, facade, requestedModel, account, input)

	task, err := h.asyncMedia.SubmitAsync(c.Request.Context(), submitInput)
	if err != nil {
		reqLog.Warn("fal.images.submit_failed", zap.Error(err))
		if errors.Is(err, service.ErrAsyncMediaPricingMissing) {
			// 模型未配置定价：拒绝提交，避免被「免费刷图」。
			h.jsonError(c, http.StatusServiceUnavailable, "pricing_unavailable",
				"Image model pricing is not configured for this group/channel; please contact the administrator")
			return false
		}
		h.jsonError(c, http.StatusBadGateway, "api_error", "Failed to submit image task: "+err.Error())
		return false
	}

	waitCtx, cancel := context.WithTimeout(c.Request.Context(), h.pseudoSyncTimeout())
	defer cancel()

	finalTask, err := h.asyncMedia.WaitForTerminal(waitCtx, task, submitInput)
	if err != nil {
		if errors.Is(err, service.ErrAsyncMediaPending) {
			// 伪同步超时：任务不退费、不终结，由 reconciler 兜底；返回错误并附带 request_id。
			reqLog.Info("fal.images.pseudo_sync_timeout", zap.Int64("task_id", task.ID))
			c.JSON(http.StatusGatewayTimeout, gin.H{
				"error": gin.H{
					"type":       "timeout_error",
					"message":    "Image generation timed out; task is still processing",
					"request_id": derefStringPtr(task.UpstreamRequestID),
				},
			})
			return false
		}
		reqLog.Warn("fal.images.wait_failed", zap.Int64("task_id", task.ID), zap.Error(err))
		h.jsonError(c, http.StatusBadGateway, "api_error", "Image generation failed")
		return false
	}

	if finalTask == nil || finalTask.Status != service.AsyncMediaStatusSucceeded {
		reason := "Image generation failed"
		if finalTask != nil && finalTask.ErrorReason != nil && *finalTask.ErrorReason != "" {
			reason = *finalTask.ErrorReason
		}
		h.jsonError(c, http.StatusBadGateway, "api_error", reason)
		return false
	}

	onSuccess(finalTask)
	return true
}

// buildSubmitInput 组装异步任务提交入参（账号由调用方预选）。
func (h *FalGatewayHandler) buildSubmitInput(
	c *gin.Context,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	facade string,
	requestedModel string,
	account *service.Account,
	input fal.ImageGenInput,
) *service.AsyncMediaSubmitInput {
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	billingType := service.BillingTypeBalance
	if subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType() {
		billingType = service.BillingTypeSubscription
	}

	rateMultiplier := 1.0
	if account.RateMultiplier != nil && *account.RateMultiplier > 0 {
		rateMultiplier = *account.RateMultiplier
	}

	return &service.AsyncMediaSubmitInput{
		Account:           account,
		User:              apiKey.User,
		APIKeyID:          apiKey.ID,
		UserID:            subject.UserID,
		AccountID:         account.ID,
		GroupID:           apiKey.GroupID,
		Facade:            facade,
		InternalRequestID: falInternalRequestID(c),
		RequestedModel:    requestedModel,
		Input:             input,
		BillingType:       billingType,
		RateMultiplier:    rateMultiplier,
		ClientIP:          c.ClientIP(),
		UserAgent:         c.GetHeader("User-Agent"),
		InboundEndpoint:   falInboundEndpoint(c),
	}
}

// falInboundEndpoint 取客户端可见的对外端点路径，优先路由模板，回退实际请求路径。
func falInboundEndpoint(c *gin.Context) string {
	if p := c.FullPath(); p != "" {
		return p
	}
	if c.Request != nil && c.Request.URL != nil {
		return c.Request.URL.Path
	}
	return ""
}

// Native 实现 fal 原生异步门面（catch-all 分发 submit/status/result/cancel）。
//
// 路由：/fal/*path
//   - POST /fal/{model}                          -> submit
//   - GET  /fal/{model}/requests/{id}/status     -> status
//   - GET  /fal/{model}/requests/{id}            -> result
//   - PUT  /fal/{model}/requests/{id}/cancel     -> cancel
func (h *FalGatewayHandler) Native(c *gin.Context) {
	path := strings.Trim(c.Param("path"), "/")
	method := c.Request.Method

	switch {
	case method == http.MethodGet && strings.HasSuffix(path, "/status"):
		h.nativeStatus(c, falRequestIDFromPath(path))
	case method == http.MethodPut && strings.HasSuffix(path, "/cancel"):
		h.nativeCancel(c, falRequestIDFromPath(path))
	case method == http.MethodGet && strings.Contains(path, "/requests/"):
		h.nativeResult(c, falRequestIDFromPath(path))
	case method == http.MethodPost:
		h.nativeSubmit(c, path)
	default:
		h.jsonError(c, http.StatusNotFound, "not_found_error", "Unsupported fal endpoint")
	}
}

func (h *FalGatewayHandler) nativeSubmit(c *gin.Context, model string) {
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
	if !service.GroupAllowsImageGeneration(apiKey.Group) {
		h.jsonError(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil || len(body) == 0 {
		h.jsonError(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	var falReq fal.Request
	if err := json.Unmarshal(body, &falReq); err != nil {
		h.jsonError(c, http.StatusBadRequest, "invalid_request_error", "Invalid fal request body")
		return
	}
	input := fal.FalRequestToInput(&falReq)

	// 原生 submit 的请求模型即路径上的 fal slug，slug 自带 api 段（如 .../edit），
	// 由 falAccountSupportsModel 直接解析，故此处 api 传空串。
	account, err := h.selectFalAccount(c, apiKey, model, "")
	if err != nil || account == nil {
		h.jsonError(c, http.StatusServiceUnavailable, "api_error", "no available fal account")
		return
	}
	submitInput := h.buildSubmitInput(c, apiKey, subject, service.AsyncMediaFacadeFal, model, account, input)

	task, err := h.asyncMedia.SubmitAsync(c.Request.Context(), submitInput)
	if err != nil {
		if errors.Is(err, service.ErrAsyncMediaPricingMissing) {
			h.jsonError(c, http.StatusServiceUnavailable, "pricing_unavailable",
				"Image model pricing is not configured for this group/channel; please contact the administrator")
			return
		}
		h.jsonError(c, http.StatusBadGateway, "api_error", "Failed to submit image task: "+err.Error())
		return
	}

	reqID := derefStringPtr(task.UpstreamRequestID)
	base := h.falCallbackBase(c, model, reqID)
	c.JSON(http.StatusOK, fal.SubmitResponse{
		RequestID:   reqID,
		Status:      fal.StatusInQueue,
		StatusURL:   base + "/status",
		ResponseURL: base,
		CancelURL:   base + "/cancel",
	})
}

func (h *FalGatewayHandler) nativeStatus(c *gin.Context, reqID string) {
	task, account := h.loadTaskAndAccount(c, reqID)
	if task == nil {
		return
	}
	if account != nil {
		_, _, _ = h.asyncMedia.AdvanceTask(c.Request.Context(), task, account)
	}
	c.JSON(http.StatusOK, fal.StatusResponse{
		Status:      falStatusFromTask(task),
		RequestID:   reqID,
		ResponseURL: h.falCallbackBase(c, "", reqID),
	})
}

func (h *FalGatewayHandler) nativeResult(c *gin.Context, reqID string) {
	task, account := h.loadTaskAndAccount(c, reqID)
	if task == nil {
		return
	}
	if account != nil && !task.IsTerminal() {
		if updated, _, _ := h.asyncMedia.AdvanceTask(c.Request.Context(), task, account); updated != nil {
			task = updated
		}
	}
	if !task.IsTerminal() {
		// 仍在处理中：fal 协议下结果未就绪返回 202。
		c.JSON(http.StatusAccepted, fal.StatusResponse{Status: falStatusFromTask(task), RequestID: reqID})
		return
	}
	if task.Status != service.AsyncMediaStatusSucceeded {
		reason := "image generation failed"
		if task.ErrorReason != nil && *task.ErrorReason != "" {
			reason = *task.ErrorReason
		}
		h.jsonError(c, http.StatusBadGateway, "api_error", reason)
		return
	}
	resp := &fal.Response{Images: []fal.Image{}}
	for _, u := range task.ResultURLs() {
		resp.Images = append(resp.Images, fal.Image{URL: u})
	}
	c.JSON(http.StatusOK, resp)
}

func (h *FalGatewayHandler) nativeCancel(c *gin.Context, reqID string) {
	task, account := h.loadTaskAndAccount(c, reqID)
	if task == nil {
		return
	}
	if err := h.asyncMedia.CancelTask(c.Request.Context(), task, account); err != nil {
		h.jsonError(c, http.StatusBadGateway, "api_error", "Failed to cancel task")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "CANCELLED", "request_id": reqID})
}

// loadTaskAndAccount 按上游 request_id 加载任务与其归属账号；任务不存在时已写 404 响应。
func (h *FalGatewayHandler) loadTaskAndAccount(c *gin.Context, reqID string) (*service.AsyncMediaTask, *service.Account) {
	if strings.TrimSpace(reqID) == "" {
		h.jsonError(c, http.StatusBadRequest, "invalid_request_error", "Missing request id")
		return nil, nil
	}
	task, err := h.asyncMedia.GetTaskByUpstreamID(c.Request.Context(), reqID)
	if err != nil || task == nil {
		h.jsonError(c, http.StatusNotFound, "not_found_error", "Request not found")
		return nil, nil
	}
	// 校验任务归属当前 API Key，避免越权查询其它用户的任务。
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

func (h *FalGatewayHandler) falCallbackBase(c *gin.Context, model, reqID string) string {
	scheme := "https"
	if c.Request.TLS == nil && c.GetHeader("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	model = strings.Trim(model, "/")
	if model == "" {
		return scheme + "://" + c.Request.Host + "/fal/requests/" + reqID
	}
	return scheme + "://" + c.Request.Host + "/fal/" + model + "/requests/" + reqID
}

// ----- helpers -----

func buildFalInputFromOpenAI(parsed *service.OpenAIImagesRequest) fal.ImageGenInput {
	input := fal.ImageGenInput{
		Prompt:       parsed.Prompt,
		Size:         parsed.Size,
		Quality:      parsed.Quality,
		N:            parsed.N,
		OutputFormat: parsed.OutputFormat,
		IsEdit:       parsed.IsEdits(),
	}
	if parsed.IsEdits() {
		input.ImageURLs = append(input.ImageURLs, parsed.InputImageURLs...)
		for _, up := range parsed.Uploads {
			if dataURL := up.ModerationDataURL(); dataURL != "" {
				input.ImageURLs = append(input.ImageURLs, dataURL)
			}
		}
		input.MaskURL = parsed.MaskImageURL
		if parsed.MaskUpload != nil {
			if dataURL := parsed.MaskUpload.ModerationDataURL(); dataURL != "" {
				input.MaskURL = dataURL
			}
		}
	}
	return input
}

// falInternalRequestID 取客户端请求关联 ID 作为幂等键，缺失时生成 UUID。
func falInternalRequestID(c *gin.Context) string {
	if v, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v := strings.TrimSpace(c.GetHeader("x-client-request-id")); v != "" {
		return v
	}
	return uuid.New().String()
}

// falRequestIDFromPath 从 ".../requests/{id}[/status|/cancel]" 中提取 request_id。
func falRequestIDFromPath(path string) string {
	_, rest, ok := strings.Cut(path, "/requests/")
	if !ok {
		return ""
	}
	rest = strings.TrimSuffix(rest, "/status")
	rest = strings.TrimSuffix(rest, "/cancel")
	return strings.Trim(rest, "/")
}

func falStatusFromTask(task *service.AsyncMediaTask) string {
	switch task.Status {
	case service.AsyncMediaStatusSucceeded:
		return fal.StatusCompleted
	case service.AsyncMediaStatusPending:
		return fal.StatusInQueue
	default:
		return fal.StatusInProgress
	}
}

func derefStringPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
