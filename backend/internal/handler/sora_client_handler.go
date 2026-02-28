package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	// 上游模型缓存 TTL
	modelCacheTTL       = 1 * time.Hour   // 上游获取成功
	modelCacheFailedTTL = 2 * time.Minute // 上游获取失败（降级到本地）
)

// SoraClientHandler 处理 Sora 客户端 API 请求。
type SoraClientHandler struct {
	genService         *service.SoraGenerationService
	quotaService       *service.SoraQuotaService
	s3Storage          *service.SoraS3Storage
	soraGatewayService *service.SoraGatewayService
	gatewayService     *service.GatewayService
	mediaStorage       *service.SoraMediaStorage
	apiKeyService      *service.APIKeyService

	// 上游模型缓存
	modelCacheMu       sync.RWMutex
	cachedFamilies     []service.SoraModelFamily
	modelCacheTime     time.Time
	modelCacheUpstream bool // 是否来自上游（决定 TTL）
REDACTED

// NewSoraClientHandler 创建 Sora 客户端 Handler。
func NewSoraClientHandler(
	genService *service.SoraGenerationService,
	quotaService *service.SoraQuotaService,
	s3Storage *service.SoraS3Storage,
	soraGatewayService *service.SoraGatewayService,
	gatewayService *service.GatewayService,
	mediaStorage *service.SoraMediaStorage,
	apiKeyService *service.APIKeyService,
) *SoraClientHandler {
	return &SoraClientHandler{
		genService:         genService,
		quotaService:       quotaService,
		s3Storage:          s3Storage,
		soraGatewayService: soraGatewayService,
		gatewayService:     gatewayService,
		mediaStorage:       mediaStorage,
		apiKeyService:      apiKeyService,
REDACTED
REDACTED

// GenerateRequest 生成请求。
type GenerateRequest struct {
	Model      string `json:"model" binding:"required"`
	Prompt     string `json:"prompt" binding:"required"`
	MediaType  string `json:"media_type"`            // video / image，默认 video
	VideoCount int    `json:"video_count,omitempty"` // 视频数量（1-3）
	ImageInput string `json:"image_input,omitempty"` // 参考图（base64 或 URL）
	APIKeyID   *int64 `json:"api_key_id,omitempty"`  // 前端传递的 API Key ID
REDACTED

// Generate 异步生成 — 创建 pending 记录后立即返回。
// POST /api/v1/sora/generate
func (h *SoraClientHandler) Generate(c *gin.Context) {
	userID := getUserIDFromContext(c)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, "未登录")
		return
REDACTED

	var req GenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
REDACTED

	if req.MediaType == "" {
		req.MediaType = "video"
REDACTED
	req.VideoCount = normalizeVideoCount(req.MediaType, req.VideoCount)

	// 并发数检查（最多 3 个）
	activeCount, err := h.genService.CountActiveByUser(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if activeCount >= 3 {
		response.Error(c, http.StatusTooManyRequests, "同时进行中的任务不能超过 3 个")
		return
REDACTED

	// 配额检查（粗略检查，实际文件大小在上传后才知道）
	if h.quotaService != nil {
		if err := h.quotaService.CheckQuota(c.Request.Context(), userID, 0); err != nil {
			var quotaErr *service.QuotaExceededError
			if errors.As(err, &quotaErr) {
				response.Error(c, http.StatusTooManyRequests, "存储配额已满，请删除不需要的作品释放空间")
				return
		REDACTED
			response.Error(c, http.StatusForbidden, err.Error())
			return
	REDACTED
REDACTED

	// 获取 API Key ID 和 Group ID
	var apiKeyID *int64
	var groupID *int64

	if req.APIKeyID != nil && h.apiKeyService != nil {
		// 前端传递了 api_key_id，需要校验
		apiKey, err := h.apiKeyService.GetByID(c.Request.Context(), *req.APIKeyID)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "API Key 不存在")
			return
	REDACTED
		if apiKey.UserID != userID {
			response.Error(c, http.StatusForbidden, "API Key 不属于当前用户")
			return
	REDACTED
		if apiKey.Status != service.StatusAPIKeyActive {
			response.Error(c, http.StatusForbidden, "API Key 不可用")
			return
	REDACTED
		apiKeyID = &apiKey.ID
		groupID = apiKey.GroupID
REDACTED else if id, ok := c.Get("api_key_id"); ok {
		// 兼容 API Key 认证路径（/sora/v1/ 网关路由）
		if v, ok := id.(int64); ok {
			apiKeyID = &v
	REDACTED
REDACTED

	gen, err := h.genService.CreatePending(c.Request.Context(), userID, apiKeyID, req.Model, req.Prompt, req.MediaType)
	if err != nil {
		if errors.Is(err, service.ErrSoraGenerationConcurrencyLimit) {
			response.Error(c, http.StatusTooManyRequests, "同时进行中的任务不能超过 3 个")
			return
	REDACTED
		response.ErrorFrom(c, err)
		return
REDACTED

	// 启动后台异步生成 goroutine
	go h.processGeneration(gen.ID, userID, groupID, req.Model, req.Prompt, req.MediaType, req.ImageInput, req.VideoCount)

	response.Success(c, gin.H{
		"generation_id": gen.ID,
		"status":        gen.Status,
REDACTED)
REDACTED

// processGeneration 后台异步执行 Sora 生成任务。
// 流程：选择账号 → Forward → 提取媒体 URL → 三层降级存储（S3 → 本地 → 上游）→ 更新记录。
func (h *SoraClientHandler) processGeneration(genID int64, userID int64, groupID *int64, model, prompt, mediaType, imageInput string, videoCount int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// 标记为生成中
	if err := h.genService.MarkGenerating(ctx, genID, ""); err != nil {
		if errors.Is(err, service.ErrSoraGenerationStateConflict) {
			logger.LegacyPrintf("handler.sora_client", "[SoraClient] 任务状态已变化，跳过生成 id=%d", genID)
			return
	REDACTED
		logger.LegacyPrintf("handler.sora_client", "[SoraClient] 标记生成中失败 id=%d err=%v", genID, err)
		return
REDACTED

	logger.LegacyPrintf(
		"handler.sora_client",
		"[SoraClient] 开始生成 id=%d user=%d group=%d model=%s media_type=%s video_count=%d has_image=%v prompt_len=%d",
		genID,
		userID,
		groupIDForLog(groupID),
		model,
		mediaType,
		videoCount,
		strings.TrimSpace(imageInput) != "",
		len(strings.TrimSpace(prompt)),
	)

	// 有 groupID 时由分组决定平台，无 groupID 时用 ForcePlatform 兜底
	if groupID == nil {
		ctx = context.WithValue(ctx, ctxkey.ForcePlatform, service.PlatformSora)
REDACTED

	if h.gatewayService == nil {
		_ = h.genService.MarkFailed(ctx, genID, "内部错误: gatewayService 未初始化")
		return
REDACTED

	// 选择 Sora 账号
	account, err := h.gatewayService.SelectAccountForModel(ctx, groupID, "", model)
	if err != nil {
		logger.LegacyPrintf(
			"handler.sora_client",
			"[SoraClient] 选择账号失败 id=%d user=%d group=%d model=%s err=%v",
			genID,
			userID,
			groupIDForLog(groupID),
			model,
			err,
		)
		_ = h.genService.MarkFailed(ctx, genID, "选择账号失败: "+err.Error())
		return
REDACTED
	logger.LegacyPrintf(
		"handler.sora_client",
		"[SoraClient] 选中账号 id=%d user=%d group=%d model=%s account_id=%d account_name=%s platform=%s type=%s",
		genID,
		userID,
		groupIDForLog(groupID),
		model,
		account.ID,
		account.Name,
		account.Platform,
		account.Type,
	)

	// 构建 chat completions 请求体（非流式）
	body := buildAsyncRequestBody(model, prompt, imageInput, normalizeVideoCount(mediaType, videoCount))

	if h.soraGatewayService == nil {
		_ = h.genService.MarkFailed(ctx, genID, "内部错误: soraGatewayService 未初始化")
		return
REDACTED

	// 创建 mock gin 上下文用于 Forward（捕获响应以提取媒体 URL）
	recorder := httptest.NewRecorder()
	mockGinCtx, _ := gin.CreateTestContext(recorder)
	mockGinCtx.Request, _ = http.NewRequest("POST", "/", nil)

	// 调用 Forward（非流式）
	result, err := h.soraGatewayService.Forward(ctx, mockGinCtx, account, body, false)
	if err != nil {
		logger.LegacyPrintf(
			"handler.sora_client",
			"[SoraClient] Forward失败 id=%d account_id=%d model=%s status=%d body=%s err=%v",
			genID,
			account.ID,
			model,
			recorder.Code,
			trimForLog(recorder.Body.String(), 400),
			err,
		)
		// 检查是否已取消
		gen, _ := h.genService.GetByID(ctx, genID, userID)
		if gen != nil && gen.Status == service.SoraGenStatusCancelled {
			return
	REDACTED
		_ = h.genService.MarkFailed(ctx, genID, "生成失败: "+err.Error())
		return
REDACTED

	// 提取媒体 URL（优先从 ForwardResult，其次从响应体解析）
	mediaURL, mediaURLs := extractMediaURLsFromResult(result, recorder)
	if mediaURL == "" {
		logger.LegacyPrintf(
			"handler.sora_client",
			"[SoraClient] 未提取到媒体URL id=%d account_id=%d model=%s status=%d body=%s",
			genID,
			account.ID,
			model,
			recorder.Code,
			trimForLog(recorder.Body.String(), 400),
		)
		_ = h.genService.MarkFailed(ctx, genID, "未获取到媒体 URL")
		return
REDACTED

	// 检查任务是否已被取消
	gen, _ := h.genService.GetByID(ctx, genID, userID)
	if gen != nil && gen.Status == service.SoraGenStatusCancelled {
		logger.LegacyPrintf("handler.sora_client", "[SoraClient] 任务已取消，跳过存储 id=%d", genID)
		return
REDACTED

	// 三层降级存储：S3 → 本地 → 上游临时 URL
	storedURL, storedURLs, storageType, s3Keys, fileSize := h.storeMediaWithDegradation(ctx, userID, mediaType, mediaURL, mediaURLs)

	usageAdded := false
	if (storageType == service.SoraStorageTypeS3 || storageType == service.SoraStorageTypeLocal) && fileSize > 0 && h.quotaService != nil {
		if err := h.quotaService.AddUsage(ctx, userID, fileSize); err != nil {
			h.cleanupStoredMedia(ctx, storageType, s3Keys, storedURLs)
			var quotaErr *service.QuotaExceededError
			if errors.As(err, &quotaErr) {
				_ = h.genService.MarkFailed(ctx, genID, "存储配额已满，请删除不需要的作品释放空间")
				return
		REDACTED
			_ = h.genService.MarkFailed(ctx, genID, "存储配额更新失败: "+err.Error())
			return
	REDACTED
		usageAdded = true
REDACTED

	// 存储完成后再做一次取消检查，防止取消被 completed 覆盖。
	gen, _ = h.genService.GetByID(ctx, genID, userID)
	if gen != nil && gen.Status == service.SoraGenStatusCancelled {
		logger.LegacyPrintf("handler.sora_client", "[SoraClient] 存储后检测到任务已取消，回滚存储 id=%d", genID)
		h.cleanupStoredMedia(ctx, storageType, s3Keys, storedURLs)
		if usageAdded && h.quotaService != nil {
			_ = h.quotaService.ReleaseUsage(ctx, userID, fileSize)
	REDACTED
		return
REDACTED

	// 标记完成
	if err := h.genService.MarkCompleted(ctx, genID, storedURL, storedURLs, storageType, s3Keys, fileSize); err != nil {
		if errors.Is(err, service.ErrSoraGenerationStateConflict) {
			h.cleanupStoredMedia(ctx, storageType, s3Keys, storedURLs)
			if usageAdded && h.quotaService != nil {
				_ = h.quotaService.ReleaseUsage(ctx, userID, fileSize)
		REDACTED
			return
	REDACTED
		logger.LegacyPrintf("handler.sora_client", "[SoraClient] 标记完成失败 id=%d err=%v", genID, err)
		return
REDACTED

	logger.LegacyPrintf("handler.sora_client", "[SoraClient] 生成完成 id=%d storage=%s size=%d", genID, storageType, fileSize)
REDACTED

// storeMediaWithDegradation 实现三层降级存储链：S3 → 本地 → 上游。
func (h *SoraClientHandler) storeMediaWithDegradation(
	ctx context.Context, userID int64, mediaType string,
	mediaURL string, mediaURLs []string,
) (storedURL string, storedURLs []string, storageType string, s3Keys []string, fileSize int64) {
	urls := mediaURLs
	if len(urls) == 0 {
		urls = []string{mediaURLREDACTED
REDACTED

	// 第一层：尝试 S3
	if h.s3Storage != nil && h.s3Storage.Enabled(ctx) {
		keys := make([]string, 0, len(urls))
		var totalSize int64
		allOK := true
		for _, u := range urls {
			key, size, err := h.s3Storage.UploadFromURL(ctx, userID, u)
			if err != nil {
				logger.LegacyPrintf("handler.sora_client", "[SoraClient] S3 上传失败 err=%v", err)
				allOK = false
				// 清理已上传的文件
				if len(keys) > 0 {
					_ = h.s3Storage.DeleteObjects(ctx, keys)
			REDACTED
				break
		REDACTED
			keys = append(keys, key)
			totalSize += size
	REDACTED
		if allOK && len(keys) > 0 {
			accessURLs := make([]string, 0, len(keys))
			for _, key := range keys {
				accessURL, err := h.s3Storage.GetAccessURL(ctx, key)
				if err != nil {
					logger.LegacyPrintf("handler.sora_client", "[SoraClient] 生成 S3 访问 URL 失败 err=%v", err)
					_ = h.s3Storage.DeleteObjects(ctx, keys)
					allOK = false
					break
			REDACTED
				accessURLs = append(accessURLs, accessURL)
		REDACTED
			if allOK && len(accessURLs) > 0 {
				return accessURLs[0], accessURLs, service.SoraStorageTypeS3, keys, totalSize
		REDACTED
	REDACTED
REDACTED

	// 第二层：尝试本地存储
	if h.mediaStorage != nil && h.mediaStorage.Enabled() {
		storedPaths, err := h.mediaStorage.StoreFromURLs(ctx, mediaType, urls)
		if err == nil && len(storedPaths) > 0 {
			firstPath := storedPaths[0]
			totalSize, sizeErr := h.mediaStorage.TotalSizeByRelativePaths(storedPaths)
			if sizeErr != nil {
				logger.LegacyPrintf("handler.sora_client", "[SoraClient] 统计本地文件大小失败 err=%v", sizeErr)
		REDACTED
			return firstPath, storedPaths, service.SoraStorageTypeLocal, nil, totalSize
	REDACTED
		logger.LegacyPrintf("handler.sora_client", "[SoraClient] 本地存储失败 err=%v", err)
REDACTED

	// 第三层：保留上游临时 URL
	return urls[0], urls, service.SoraStorageTypeUpstream, nil, 0
REDACTED

// buildAsyncRequestBody 构建 Sora 异步生成的 chat completions 请求体。
func buildAsyncRequestBody(model, prompt, imageInput string, videoCount int) []byte {
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": promptREDACTED,
	REDACTED,
		"stream": false,
REDACTED
	if imageInput != "" {
		body["image_input"] = imageInput
REDACTED
	if videoCount > 1 {
		body["video_count"] = videoCount
REDACTED
	b, _ := json.Marshal(body)
	return b
REDACTED

func normalizeVideoCount(mediaType string, videoCount int) int {
	if mediaType != "video" {
		return 1
REDACTED
	if videoCount <= 0 {
		return 1
REDACTED
	if videoCount > 3 {
		return 3
REDACTED
	return videoCount
REDACTED

// extractMediaURLsFromResult 从 Forward 结果和响应体中提取媒体 URL。
// OAuth 路径：ForwardResult.MediaURL 已填充。
// APIKey 路径：需从响应体解析 media_url / media_urls 字段。
func extractMediaURLsFromResult(result *service.ForwardResult, recorder *httptest.ResponseRecorder) (string, []string) {
	// 优先从 ForwardResult 获取（OAuth 路径）
	if result != nil && result.MediaURL != "" {
		// 尝试从响应体获取完整 URL 列表
		if urls := parseMediaURLsFromBody(recorder.Body.Bytes()); len(urls) > 0 {
			return urls[0], urls
	REDACTED
		return result.MediaURL, []string{result.MediaURLREDACTED
REDACTED

	// 从响应体解析（APIKey 路径）
	if urls := parseMediaURLsFromBody(recorder.Body.Bytes()); len(urls) > 0 {
		return urls[0], urls
REDACTED

	return "", nil
REDACTED

// parseMediaURLsFromBody 从 JSON 响应体中解析 media_url / media_urls 字段。
func parseMediaURLsFromBody(body []byte) []string {
	if len(body) == 0 {
		return nil
REDACTED
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
REDACTED

	// 优先 media_urls（多图数组）
	if rawURLs, ok := resp["media_urls"]; ok {
		if arr, ok := rawURLs.([]any); ok && len(arr) > 0 {
			urls := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok && s != "" {
					urls = append(urls, s)
			REDACTED
		REDACTED
			if len(urls) > 0 {
				return urls
		REDACTED
	REDACTED
REDACTED

	// 回退到 media_url（单个 URL）
	if url, ok := resp["media_url"].(string); ok && url != "" {
		return []string{urlREDACTED
REDACTED

	return nil
REDACTED

// ListGenerations 查询生成记录列表。
// GET /api/v1/sora/generations
func (h *SoraClientHandler) ListGenerations(c *gin.Context) {
	userID := getUserIDFromContext(c)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, "未登录")
		return
REDACTED

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	params := service.SoraGenerationListParams{
		UserID:      userID,
		Status:      c.Query("status"),
		StorageType: c.Query("storage_type"),
		MediaType:   c.Query("media_type"),
		Page:        page,
		PageSize:    pageSize,
REDACTED

	gens, total, err := h.genService.List(c.Request.Context(), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	// 为 S3 记录动态生成预签名 URL
	for _, gen := range gens {
		_ = h.genService.ResolveMediaURLs(c.Request.Context(), gen)
REDACTED

	response.Success(c, gin.H{
		"data":  gens,
		"total": total,
		"page":  page,
REDACTED)
REDACTED

// GetGeneration 查询生成记录详情。
// GET /api/v1/sora/generations/:id
func (h *SoraClientHandler) GetGeneration(c *gin.Context) {
	userID := getUserIDFromContext(c)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, "未登录")
		return
REDACTED

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的 ID")
		return
REDACTED

	gen, err := h.genService.GetByID(c.Request.Context(), id, userID)
	if err != nil {
		response.Error(c, http.StatusNotFound, err.Error())
		return
REDACTED

	_ = h.genService.ResolveMediaURLs(c.Request.Context(), gen)
	response.Success(c, gen)
REDACTED

// DeleteGeneration 删除生成记录。
// DELETE /api/v1/sora/generations/:id
func (h *SoraClientHandler) DeleteGeneration(c *gin.Context) {
	userID := getUserIDFromContext(c)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, "未登录")
		return
REDACTED

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的 ID")
		return
REDACTED

	gen, err := h.genService.GetByID(c.Request.Context(), id, userID)
	if err != nil {
		response.Error(c, http.StatusNotFound, err.Error())
		return
REDACTED

	// 先尝试清理本地文件，再删除记录（清理失败不阻塞删除）。
	if gen.StorageType == service.SoraStorageTypeLocal && h.mediaStorage != nil {
		paths := gen.MediaURLs
		if len(paths) == 0 && gen.MediaURL != "" {
			paths = []string{gen.MediaURLREDACTED
	REDACTED
		if err := h.mediaStorage.DeleteByRelativePaths(paths); err != nil {
			logger.LegacyPrintf("handler.sora_client", "[SoraClient] 删除本地文件失败 id=%d err=%v", id, err)
	REDACTED
REDACTED

	if err := h.genService.Delete(c.Request.Context(), id, userID); err != nil {
		response.Error(c, http.StatusNotFound, err.Error())
		return
REDACTED

	response.Success(c, gin.H{"message": "已删除"REDACTED)
REDACTED

// GetQuota 查询用户存储配额。
// GET /api/v1/sora/quota
func (h *SoraClientHandler) GetQuota(c *gin.Context) {
	userID := getUserIDFromContext(c)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, "未登录")
		return
REDACTED

	if h.quotaService == nil {
		response.Success(c, service.QuotaInfo{QuotaSource: "unlimited", Source: "unlimited"REDACTED)
		return
REDACTED

	quota, err := h.quotaService.GetQuota(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, quota)
REDACTED

// CancelGeneration 取消生成任务。
// POST /api/v1/sora/generations/:id/cancel
func (h *SoraClientHandler) CancelGeneration(c *gin.Context) {
	userID := getUserIDFromContext(c)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, "未登录")
		return
REDACTED

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的 ID")
		return
REDACTED

	// 权限校验
	gen, err := h.genService.GetByID(c.Request.Context(), id, userID)
	if err != nil {
		response.Error(c, http.StatusNotFound, err.Error())
		return
REDACTED
	_ = gen

	if err := h.genService.MarkCancelled(c.Request.Context(), id); err != nil {
		if errors.Is(err, service.ErrSoraGenerationNotActive) {
			response.Error(c, http.StatusConflict, "任务已结束，无法取消")
			return
	REDACTED
		response.Error(c, http.StatusBadRequest, err.Error())
		return
REDACTED

	response.Success(c, gin.H{"message": "已取消"REDACTED)
REDACTED

// SaveToStorage 手动保存 upstream 记录到 S3。
// POST /api/v1/sora/generations/:id/save
func (h *SoraClientHandler) SaveToStorage(c *gin.Context) {
	userID := getUserIDFromContext(c)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, "未登录")
		return
REDACTED

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的 ID")
		return
REDACTED

	gen, err := h.genService.GetByID(c.Request.Context(), id, userID)
	if err != nil {
		response.Error(c, http.StatusNotFound, err.Error())
		return
REDACTED

	if gen.StorageType != service.SoraStorageTypeUpstream {
		response.Error(c, http.StatusBadRequest, "仅 upstream 类型的记录可手动保存")
		return
REDACTED
	if gen.MediaURL == "" {
		response.Error(c, http.StatusBadRequest, "媒体 URL 为空，可能已过期")
		return
REDACTED

	if h.s3Storage == nil || !h.s3Storage.Enabled(c.Request.Context()) {
		response.Error(c, http.StatusServiceUnavailable, "云存储未配置，请联系管理员")
		return
REDACTED

	sourceURLs := gen.MediaURLs
	if len(sourceURLs) == 0 && gen.MediaURL != "" {
		sourceURLs = []string{gen.MediaURLREDACTED
REDACTED
	if len(sourceURLs) == 0 {
		response.Error(c, http.StatusBadRequest, "媒体 URL 为空，可能已过期")
		return
REDACTED

	uploadedKeys := make([]string, 0, len(sourceURLs))
	accessURLs := make([]string, 0, len(sourceURLs))
	var totalSize int64

	for _, sourceURL := range sourceURLs {
		objectKey, fileSize, uploadErr := h.s3Storage.UploadFromURL(c.Request.Context(), userID, sourceURL)
		if uploadErr != nil {
			if len(uploadedKeys) > 0 {
				_ = h.s3Storage.DeleteObjects(c.Request.Context(), uploadedKeys)
		REDACTED
			var upstreamErr *service.UpstreamDownloadError
			if errors.As(uploadErr, &upstreamErr) && (upstreamErr.StatusCode == http.StatusForbidden || upstreamErr.StatusCode == http.StatusNotFound) {
				response.Error(c, http.StatusGone, "媒体链接已过期，无法保存")
				return
		REDACTED
			response.Error(c, http.StatusInternalServerError, "上传到 S3 失败: "+uploadErr.Error())
			return
	REDACTED
		accessURL, err := h.s3Storage.GetAccessURL(c.Request.Context(), objectKey)
		if err != nil {
			uploadedKeys = append(uploadedKeys, objectKey)
			_ = h.s3Storage.DeleteObjects(c.Request.Context(), uploadedKeys)
			response.Error(c, http.StatusInternalServerError, "生成 S3 访问链接失败: "+err.Error())
			return
	REDACTED
		uploadedKeys = append(uploadedKeys, objectKey)
		accessURLs = append(accessURLs, accessURL)
		totalSize += fileSize
REDACTED

	usageAdded := false
	if totalSize > 0 && h.quotaService != nil {
		if err := h.quotaService.AddUsage(c.Request.Context(), userID, totalSize); err != nil {
			_ = h.s3Storage.DeleteObjects(c.Request.Context(), uploadedKeys)
			var quotaErr *service.QuotaExceededError
			if errors.As(err, &quotaErr) {
				response.Error(c, http.StatusTooManyRequests, "存储配额已满，请删除不需要的作品释放空间")
				return
		REDACTED
			response.Error(c, http.StatusInternalServerError, "配额更新失败: "+err.Error())
			return
	REDACTED
		usageAdded = true
REDACTED

	if err := h.genService.UpdateStorageForCompleted(
		c.Request.Context(),
		id,
		accessURLs[0],
		accessURLs,
		service.SoraStorageTypeS3,
		uploadedKeys,
		totalSize,
	); err != nil {
		_ = h.s3Storage.DeleteObjects(c.Request.Context(), uploadedKeys)
		if usageAdded && h.quotaService != nil {
			_ = h.quotaService.ReleaseUsage(c.Request.Context(), userID, totalSize)
	REDACTED
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{
		"message":     "已保存到 S3",
		"object_key":  uploadedKeys[0],
		"object_keys": uploadedKeys,
REDACTED)
REDACTED

// GetStorageStatus 返回存储状态。
// GET /api/v1/sora/storage-status
func (h *SoraClientHandler) GetStorageStatus(c *gin.Context) {
	s3Enabled := h.s3Storage != nil && h.s3Storage.Enabled(c.Request.Context())
	s3Healthy := false
	if s3Enabled {
		s3Healthy = h.s3Storage.IsHealthy(c.Request.Context())
REDACTED
	localEnabled := h.mediaStorage != nil && h.mediaStorage.Enabled()
	response.Success(c, gin.H{
		"s3_enabled":    s3Enabled,
		"s3_healthy":    s3Healthy,
		"local_enabled": localEnabled,
REDACTED)
REDACTED

func (h *SoraClientHandler) cleanupStoredMedia(ctx context.Context, storageType string, s3Keys []string, localPaths []string) {
	switch storageType {
	case service.SoraStorageTypeS3:
		if h.s3Storage != nil && len(s3Keys) > 0 {
			if err := h.s3Storage.DeleteObjects(ctx, s3Keys); err != nil {
				logger.LegacyPrintf("handler.sora_client", "[SoraClient] 清理 S3 文件失败 keys=%v err=%v", s3Keys, err)
		REDACTED
	REDACTED
	case service.SoraStorageTypeLocal:
		if h.mediaStorage != nil && len(localPaths) > 0 {
			if err := h.mediaStorage.DeleteByRelativePaths(localPaths); err != nil {
				logger.LegacyPrintf("handler.sora_client", "[SoraClient] 清理本地文件失败 paths=%v err=%v", localPaths, err)
		REDACTED
	REDACTED
REDACTED
REDACTED

// getUserIDFromContext 从 gin 上下文中提取用户 ID。
func getUserIDFromContext(c *gin.Context) int64 {
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok && subject.UserID > 0 {
		return subject.UserID
REDACTED

	if id, ok := c.Get("user_id"); ok {
		switch v := id.(type) {
		case int64:
			return v
		case float64:
			return int64(v)
		case string:
			n, _ := strconv.ParseInt(v, 10, 64)
			return n
	REDACTED
REDACTED
	// 尝试从 JWT claims 获取
	if id, ok := c.Get("userID"); ok {
		if v, ok := id.(int64); ok {
			return v
	REDACTED
REDACTED
	return 0
REDACTED

func groupIDForLog(groupID *int64) int64 {
	if groupID == nil {
		return 0
REDACTED
	return *groupID
REDACTED

func trimForLog(raw string, maxLen int) string {
	trimmed := strings.TrimSpace(raw)
	if maxLen <= 0 || len(trimmed) <= maxLen {
		return trimmed
REDACTED
	return trimmed[:maxLen] + "...(truncated)"
REDACTED

// GetModels 获取可用 Sora 模型家族列表。
// 优先从上游 Sora API 同步模型列表，失败时降级到本地配置。
// GET /api/v1/sora/models
func (h *SoraClientHandler) GetModels(c *gin.Context) {
	families := h.getModelFamilies(c.Request.Context())
	response.Success(c, families)
REDACTED

// getModelFamilies 获取模型家族列表（带缓存）。
func (h *SoraClientHandler) getModelFamilies(ctx context.Context) []service.SoraModelFamily {
	// 读锁检查缓存
	h.modelCacheMu.RLock()
	ttl := modelCacheTTL
	if !h.modelCacheUpstream {
		ttl = modelCacheFailedTTL
REDACTED
	if h.cachedFamilies != nil && time.Since(h.modelCacheTime) < ttl {
		families := h.cachedFamilies
		h.modelCacheMu.RUnlock()
		return families
REDACTED
	h.modelCacheMu.RUnlock()

	// 写锁更新缓存
	h.modelCacheMu.Lock()
	defer h.modelCacheMu.Unlock()

	// double-check
	ttl = modelCacheTTL
	if !h.modelCacheUpstream {
		ttl = modelCacheFailedTTL
REDACTED
	if h.cachedFamilies != nil && time.Since(h.modelCacheTime) < ttl {
		return h.cachedFamilies
REDACTED

	// 尝试从上游获取
	families, err := h.fetchUpstreamModels(ctx)
	if err != nil {
		logger.LegacyPrintf("handler.sora_client", "[SoraClient] 上游模型获取失败，使用本地配置: %v", err)
		families = service.BuildSoraModelFamilies()
		h.cachedFamilies = families
		h.modelCacheTime = time.Now()
		h.modelCacheUpstream = false
		return families
REDACTED

	logger.LegacyPrintf("handler.sora_client", "[SoraClient] 从上游同步到 %d 个模型家族", len(families))
	h.cachedFamilies = families
	h.modelCacheTime = time.Now()
	h.modelCacheUpstream = true
	return families
REDACTED

// fetchUpstreamModels 从上游 Sora API 获取模型列表。
func (h *SoraClientHandler) fetchUpstreamModels(ctx context.Context) ([]service.SoraModelFamily, error) {
	if h.gatewayService == nil {
		return nil, fmt.Errorf("gatewayService 未初始化")
REDACTED

	// 设置 ForcePlatform 用于 Sora 账号选择
	ctx = context.WithValue(ctx, ctxkey.ForcePlatform, service.PlatformSora)

	// 选择一个 Sora 账号
	account, err := h.gatewayService.SelectAccountForModel(ctx, nil, "", "sora2-landscape-10s")
	if err != nil {
		return nil, fmt.Errorf("选择 Sora 账号失败: %w", err)
REDACTED

	// 仅支持 API Key 类型账号
	if account.Type != service.AccountTypeAPIKey {
		return nil, fmt.Errorf("当前账号类型 %s 不支持模型同步", account.Type)
REDACTED

	apiKey := account.GetCredential("api_key")
	if apiKey == "" {
		return nil, fmt.Errorf("账号缺少 api_key")
REDACTED

	baseURL := account.GetBaseURL()
	if baseURL == "" {
		return nil, fmt.Errorf("账号缺少 base_url")
REDACTED

	// 构建上游模型列表请求
	modelsURL := strings.TrimRight(baseURL, "/") + "/sora/v1/models"

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
REDACTED
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 10 * time.SecondREDACTED
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求上游失败: %w", err)
REDACTED
	defer func() {
		_ = resp.Body.Close()
REDACTED()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("上游返回状态码 %d", resp.StatusCode)
REDACTED

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
REDACTED

	// 解析 OpenAI 格式的模型列表
	var modelsResp struct {
		Data []struct {
			ID string `json:"id"`
	REDACTED `json:"data"`
REDACTED
	if err := json.Unmarshal(body, &modelsResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
REDACTED

	if len(modelsResp.Data) == 0 {
		return nil, fmt.Errorf("上游返回空模型列表")
REDACTED

	// 提取模型 ID
	modelIDs := make([]string, 0, len(modelsResp.Data))
	for _, m := range modelsResp.Data {
		modelIDs = append(modelIDs, m.ID)
REDACTED

	// 转换为模型家族
	families := service.BuildSoraModelFamiliesFromIDs(modelIDs)
	if len(families) == 0 {
		return nil, fmt.Errorf("未能从上游模型列表中识别出有效的模型家族")
REDACTED

	return families, nil
REDACTED
