package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"path"
	"strconv"
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
	_ "golang.org/x/image/webp"
)

// defaultImagePseudoSyncTimeout 伪同步门面阻塞等待上限（超时返回错误但不退费/不终结）。
const defaultImagePseudoSyncTimeout = 300 * time.Second

// ImageGatewayHandler 处理 FAL/Leonardo 共用的 OpenAI 图片门面：
//   - OpenAI 伪同步门面（/v1/images/generations、/v1/images/edits）
//   - OpenAI/FAL/Leonardo 混合图片账号选号
type ImageGatewayHandler struct {
	gatewayService *service.GatewayService
	imagesService  *service.OpenAIGatewayService
	asyncMedia     *service.AsyncMediaService
	cosService     *service.COSImageTransferService
	cfg            *config.Config
}

// NewImageGatewayHandler 创建共享图片门面 handler。
func NewImageGatewayHandler(
	gatewayService *service.GatewayService,
	imagesService *service.OpenAIGatewayService,
	asyncMedia *service.AsyncMediaService,
	cosService *service.COSImageTransferService,
	cfg *config.Config,
) *ImageGatewayHandler {
	return &ImageGatewayHandler{
		gatewayService: gatewayService,
		imagesService:  imagesService,
		asyncMedia:     asyncMedia,
		cosService:     cosService,
		cfg:            cfg,
	}
}

func (h *ImageGatewayHandler) pseudoSyncTimeout() time.Duration {
	if h.cfg != nil && h.cfg.AsyncMedia.PseudoSyncTimeoutSeconds > 0 {
		return time.Duration(h.cfg.AsyncMedia.PseudoSyncTimeoutSeconds) * time.Second
	}
	return defaultImagePseudoSyncTimeout
}

func (h *ImageGatewayHandler) jsonError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// Images 实现 OpenAI 伪同步门面：提交异步图片任务 → 阻塞轮询 → 返回 OpenAI 格式响应。
// POST /v1/images/generations、POST /v1/images/edits（FAL/Leonardo 分组）
func (h *ImageGatewayHandler) Images(c *gin.Context) {
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
		zap.String("component", "handler.image_gateway.images"),
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
	)
	imageStatusRequestID := ""
	imageStatusCompleted := false
	imageStatusFailMessage := publicImageFailure
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

	account, err := h.selectImageAccount(c, apiKey, parsed.Model, imageAPIForOpenAIImages(parsed))
	if err != nil || account == nil {
		failImageStatus(publicImageAccountError)
		h.jsonError(c, http.StatusServiceUnavailable, "api_error", publicImageAccountError)
		return
	}
	if !service.GroupAllowsImageGeneration(apiKey.Group) {
		failImageStatus(service.ImageGenerationPermissionMessage())
		h.jsonError(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
	}
	input := buildImageInputFromOpenAI(parsed)
	if imageStatusRequestID != "" && h.imagesService != nil {
		h.imagesService.MarkResponsesImageStatusRunning(c.Request.Context(), imageStatusRequestID)
	}
	imageStatusCompleted = h.runPseudoSync(c, reqLog, apiKey, subject, service.AsyncMediaFacadeOpenAI, parsed.Model, account, input, func(task *service.AsyncMediaTask) {
		h.writeOpenAIImagesResponse(c, reqLog, parsed, task)
	})
}

// SelectMixedImageAccount 在当前 API Key 所属分组内构建 OpenAI 与图片平台混合候选池，
// 按“优先级 + 最久未用”统一选号。
//
// 供 OpenAI 图片门面统一调度使用：返回的账号可能属于 openai 或 fal 平台，调用方据
// account.Platform 分发到对应转发路径。fal 账号所需的 api 段由 parsed 推导。
//
// preferPlatform: ""/"openai" 维持现状；"fal" 反转为 fal 优先 + openai 兜底。
func (h *ImageGatewayHandler) SelectMixedImageAccount(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	imageCapability service.OpenAIImagesCapability,
	parsed *service.OpenAIImagesRequest,
	preferPlatform string,
) (*service.Account, error) {
	return h.gatewayService.SelectImageAccountMixed(ctx, groupID, sessionHash, requestedModel, excludedIDs, imageCapability, imageAPIForOpenAIImages(parsed), preferPlatform)
}

// ServeOpenAIImagesWithAccount 让已预选的图片账号服务 OpenAI 伪同步图片请求。
// 计费完全由 AsyncMediaService 承担（预扣/退费 + usage_log），调用方不应再重复记账。
//
// 账号由混合调度预先选出并 hydrate，本方法不再重新选号。
func (h *ImageGatewayHandler) ServeOpenAIImagesWithAccount(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	parsed *service.OpenAIImagesRequest,
	account *service.Account,
) bool {
	input := buildImageInputFromOpenAI(parsed)
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
func (h *ImageGatewayHandler) writeOpenAIImagesResponse(c *gin.Context, reqLog *zap.Logger, parsed *service.OpenAIImagesRequest, task *service.AsyncMediaTask) {
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
	reqLog.Info("image_gateway.write_response", zap.Int("image_count", len(urls)), zap.Strings("image_urls", urls))
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
		if task != nil && i < len(task.ImageMetadata) {
			metadata := task.ImageMetadata[i]
			item.ContentType = metadata.ContentType
			item.FileName = metadata.FileName
			item.FileSize = metadata.FileSize
			item.Width = metadata.Width
			item.Height = metadata.Height
		}
		if h.cosService != nil {
			reqLog.Info("image_gateway.try_b64_encode", zap.String("image_url", u), zap.Int("index", i))
			if b64, err := h.cosService.FetchAsBase64(c.Request.Context(), u); err == nil && b64 != "" {
				item.B64JSON = b64
				applyLocalImageMetadata(&item, b64, u, i)
				reqLog.Info("image_gateway.b64_encode_success", zap.String("image_url", u), zap.Int("b64_len", len(b64)))
			} else {
				reqLog.Warn("image_gateway.b64_encode_failed_fallback_url", zap.String("image_url", u), zap.Error(err))
				item.URL = u
			}
		} else {
			reqLog.Warn("image_gateway.cos_service_nil_fallback_url", zap.String("image_url", u))
			item.URL = u
		}
		if item.FileName == "" {
			item.FileName = imageFileName(u, i, item.ContentType)
		}
		resp.Data = append(resp.Data, item)
	}

	c.JSON(http.StatusOK, resp)
}

func applyLocalImageMetadata(item *fal.OpenAIImageData, b64, sourceURL string, index int) {
	if item == nil {
		return
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return
	}
	item.FileSize = int64(len(data))
	if item.ContentType == "" {
		item.ContentType = strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0])
	}
	if item.Width <= 0 || item.Height <= 0 {
		if config, _, decodeErr := image.DecodeConfig(bytes.NewReader(data)); decodeErr == nil {
			item.Width = config.Width
			item.Height = config.Height
		}
	}
	if item.FileName == "" {
		item.FileName = imageFileName(sourceURL, index, item.ContentType)
	}
}

func imageFileName(sourceURL string, index int, contentType string) string {
	if parsed, err := url.Parse(sourceURL); err == nil {
		if name := path.Base(strings.TrimSpace(parsed.Path)); name != "." && name != "/" && name != "" {
			return name
		}
	}
	ext := "png"
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg":
		ext = "jpg"
	case "image/webp":
		ext = "webp"
	case "image/gif":
		ext = "gif"
	}
	return "image-" + strconv.Itoa(index+1) + "." + ext
}

// imageAPIForOpenAIImages 把 OpenAI 图片请求映射为所需的异步图片 API 段：
// /v1/images/edits（图生图）→ edit，/v1/images/generations（文生图）→ 空串。
func imageAPIForOpenAIImages(parsed *service.OpenAIImagesRequest) string {
	if parsed != nil && parsed.IsEdits() {
		return service.FalAPIEdit
	}
	return ""
}

// selectImageAccount 在当前 API Key 所属分组内按目标图片平台选号。
// 组合分组的具体平台由 composite middleware 写入请求上下文；不能只看
// apiKey.Group.Platform，否则 composite -> Leonardo 会误走 FAL 账号池。
// api 表示本次请求所需的 fal api 段（edit=图生图 / 编辑，来自 /v1/images/edits 门面；
// 空串=文生图），用于在选号阶段校验账号是否配置了对应能力的 endpoint；
// /api/v1/model/* 门面的 slug 已自带 api 段，传空串即可。
func (h *ImageGatewayHandler) selectImageAccount(c *gin.Context, apiKey *service.APIKey, requestedModel string, api string) (*service.Account, error) {
	if effectiveAPIKeyPlatform(c, apiKey) == service.PlatformLeonardo {
		account, err := h.gatewayService.SelectImageAccountMixed(c.Request.Context(), apiKey.GroupID, "", requestedModel, nil, service.OpenAIImagesCapabilityBasic, api, service.PlatformLeonardo)
		if err != nil {
			return nil, err
		}
		if account == nil || account.Platform != service.PlatformLeonardo {
			return nil, errors.New("no available leonardo account")
		}
		return account, nil
	}
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
func (h *ImageGatewayHandler) runPseudoSync(
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
		reqLog.Warn("image_gateway.submit_failed", zap.Error(err))
		if errors.Is(err, service.ErrAsyncMediaPricingMissing) {
			// 模型未配置定价：拒绝提交，避免被「免费刷图」。
			h.jsonError(c, http.StatusServiceUnavailable, "pricing_unavailable",
				"Image model pricing is not configured for this group/channel; please contact the administrator")
			return false
		}
		// Provider/network details stay in server logs and must not be exposed to clients.
		h.jsonError(c, http.StatusBadGateway, "api_error", publicImageSubmitFailure)
		return false
	}

	waitCtx, cancel := context.WithTimeout(c.Request.Context(), h.pseudoSyncTimeout())
	defer cancel()

	finalTask, err := h.asyncMedia.WaitForTerminal(waitCtx, task, submitInput)
	if err != nil {
		if errors.Is(err, service.ErrAsyncMediaPending) {
			// 伪同步超时或客户端断开：任务不退费、不终结。这里接管 image status，
			// 用 detached context 继续轮询到 fal 终态，避免前端刷新把状态误写成 failed。
			reqLog.Info("image_gateway.pseudo_sync_timeout", zap.Int64("task_id", task.ID))
			h.continuePseudoSyncImageStatus(c.Request.Context(), task, submitInput, reqLog)
			c.JSON(http.StatusGatewayTimeout, gin.H{
				"error": gin.H{
					"type":       "timeout_error",
					"message":    "Image generation timed out; task is still processing",
					"request_id": derefStringPtr(task.UpstreamRequestID),
				},
			})
			return true
		}
		reqLog.Warn("image_gateway.wait_failed", zap.Int64("task_id", task.ID), zap.Error(err))
		h.jsonError(c, http.StatusBadGateway, "api_error", publicImageFailure)
		return false
	}

	if finalTask == nil || finalTask.Status != service.AsyncMediaStatusSucceeded {
		h.jsonError(c, http.StatusBadGateway, "api_error", publicImageFailure)
		return false
	}

	onSuccess(finalTask)
	return true
}

func (h *ImageGatewayHandler) continuePseudoSyncImageStatus(parentCtx context.Context, task *service.AsyncMediaTask, submitInput *service.AsyncMediaSubmitInput, reqLog *zap.Logger) {
	if h == nil || h.asyncMedia == nil || h.imagesService == nil || task == nil || submitInput == nil {
		return
	}
	requestID := service.ResponsesImageStatusRequestIDFromContext(parentCtx)
	if requestID == "" {
		return
	}
	deadline := time.Now().Add(h.asyncMedia.FailTimeout() + time.Minute)
	if task.FailDeadlineAt != nil && task.FailDeadlineAt.After(time.Now()) {
		deadline = task.FailDeadlineAt.Add(time.Minute)
	}
	go func() {
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()
		ctx = service.WithResponsesImageStatusRequestID(ctx, requestID)
		finalTask, err := h.asyncMedia.WaitForTerminal(ctx, task, submitInput)
		if err != nil {
			message := publicImageFailure
			if errors.Is(err, service.ErrAsyncMediaPending) {
				message = "Image generation timed out"
			}
			reqLog.Warn("image_gateway.background_wait_failed", zap.Int64("task_id", task.ID), zap.String("request_id", requestID), zap.Error(err))
			h.imagesService.FailResponsesImageStatus(ctx, requestID, message)
			return
		}
		if finalTask == nil || finalTask.Status != service.AsyncMediaStatusSucceeded {
			message := publicImageFailure
			h.imagesService.FailResponsesImageStatus(ctx, requestID, message)
			return
		}
		result := &service.OpenAIForwardResult{ImageOutputURLs: finalTask.ImageURLs, ImageOutputCosURLs: finalTask.CosURLs}
		h.imagesService.MarkResponsesImageStatusUpstreamDone(ctx, result)
		h.imagesService.SucceedResponsesImageStatus(ctx, result)
		reqLog.Info("image_gateway.background_wait_succeeded", zap.Int64("task_id", finalTask.ID), zap.String("request_id", requestID), zap.Strings("image_urls", finalTask.ResultURLs()))
	}()
}

// buildSubmitInput 组装异步任务提交入参（账号由调用方预选）。
func (h *ImageGatewayHandler) buildSubmitInput(
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

	rateMultiplier := h.imagesService.ResolveImageRateMultiplier(c.Request.Context(), subject.UserID, apiKey)

	return &service.AsyncMediaSubmitInput{
		Account:           account,
		User:              apiKey.User,
		APIKeyID:          apiKey.ID,
		UserID:            subject.UserID,
		AccountID:         account.ID,
		GroupID:           apiKey.GroupID,
		Facade:            facade,
		InternalRequestID: imageInternalRequestID(c),
		RequestedModel:    requestedModel,
		Input:             input,
		BillingType:       billingType,
		RateMultiplier:    rateMultiplier,
		RateMultiplierSet: true,
		ClientIP:          c.ClientIP(),
		UserAgent:         c.GetHeader("User-Agent"),
		InboundEndpoint:   imageInboundEndpoint(c),
	}
}

// imageInboundEndpoint 取客户端可见的对外端点路径，优先路由模板，回退实际请求路径。
func imageInboundEndpoint(c *gin.Context) string {
	if p := c.FullPath(); p != "" {
		return p
	}
	if c.Request != nil && c.Request.URL != nil {
		return c.Request.URL.Path
	}
	return ""
}

// ----- helpers -----

func buildImageInputFromOpenAI(parsed *service.OpenAIImagesRequest) fal.ImageGenInput {
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

// imageInternalRequestID 取客户端请求关联 ID 作为幂等键，缺失时生成 UUID。
func imageInternalRequestID(c *gin.Context) string {
	if v, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v := strings.TrimSpace(c.GetHeader("x-client-request-id")); v != "" {
		return v
	}
	return uuid.New().String()
}

func derefStringPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
