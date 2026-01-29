package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/sora"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

const (
	soraErrorDisableThreshold = 5
	maxImageDownloadSize      = 20 * 1024 * 1024  // 20MB
	maxVideoDownloadSize      = 200 * 1024 * 1024 // 200MB
)

var (
	ErrSoraAccountMissingToken = errors.New("sora account missing access token")
	ErrSoraAccountNotEligible  = errors.New("sora account not eligible")
)

// SoraGenerationRequest 表示 Sora 生成请求。
type SoraGenerationRequest struct {
	Model         string
	Prompt        string
	Image         string
	Video         string
	RemixTargetID string
	Stream        bool
	UserID        int64
REDACTED

// SoraGenerationResult 表示 Sora 生成结果。
type SoraGenerationResult struct {
	Content    string
	MediaType  string
	ResultURLs []string
	TaskID     string
REDACTED

// SoraGatewayService 处理 Sora 生成流程。
type SoraGatewayService struct {
	accountRepo     AccountRepository
	soraAccountRepo SoraAccountRepository
	usageRepo       SoraUsageStatRepository
	taskRepo        SoraTaskRepository
	cacheService    *SoraCacheService
	settingService  *SettingService
	concurrency     *ConcurrencyService
	cfg             *config.Config
	httpUpstream    HTTPUpstream
REDACTED

// NewSoraGatewayService 创建 SoraGatewayService。
func NewSoraGatewayService(
	accountRepo AccountRepository,
	soraAccountRepo SoraAccountRepository,
	usageRepo SoraUsageStatRepository,
	taskRepo SoraTaskRepository,
	cacheService *SoraCacheService,
	settingService *SettingService,
	concurrencyService *ConcurrencyService,
	cfg *config.Config,
	httpUpstream HTTPUpstream,
) *SoraGatewayService {
	return &SoraGatewayService{
		accountRepo:     accountRepo,
		soraAccountRepo: soraAccountRepo,
		usageRepo:       usageRepo,
		taskRepo:        taskRepo,
		cacheService:    cacheService,
		settingService:  settingService,
		concurrency:     concurrencyService,
		cfg:             cfg,
		httpUpstream:    httpUpstream,
REDACTED
REDACTED

// ListModels 返回 Sora 模型列表。
func (s *SoraGatewayService) ListModels() []sora.ModelListItem {
	return sora.ListModels()
REDACTED

// Generate 执行 Sora 生成流程。
func (s *SoraGatewayService) Generate(ctx context.Context, account *Account, req SoraGenerationRequest) (*SoraGenerationResult, error) {
	client, cfg := s.getClient(ctx)
	if client == nil {
		return nil, errors.New("sora client is not configured")
REDACTED
	modelCfg, ok := sora.ModelConfigs[req.Model]
	if !ok {
		return nil, fmt.Errorf("unsupported model: %s", req.Model)
REDACTED
	accessToken, soraAcc, err := s.getAccessToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED
	if soraAcc != nil && soraAcc.SoraCooldownUntil != nil && time.Now().Before(*soraAcc.SoraCooldownUntil) {
		return nil, ErrSoraAccountNotEligible
REDACTED
	if modelCfg.RequirePro && !isSoraProAccount(soraAcc) {
		return nil, ErrSoraAccountNotEligible
REDACTED
	if modelCfg.Type == "video" && soraAcc != nil {
		if !soraAcc.VideoEnabled || !soraAcc.SoraSupported || soraAcc.IsExpired {
			return nil, ErrSoraAccountNotEligible
	REDACTED
REDACTED
	if modelCfg.Type == "image" && soraAcc != nil {
		if !soraAcc.ImageEnabled || soraAcc.IsExpired {
			return nil, ErrSoraAccountNotEligible
	REDACTED
REDACTED

	opts := sora.RequestOptions{
		AccountID:          account.ID,
		AccountConcurrency: account.Concurrency,
		AccessToken:        accessToken,
REDACTED
	if account.Proxy != nil {
		opts.ProxyURL = account.Proxy.URL()
REDACTED

	releaseFunc, err := s.acquireSoraSlots(ctx, account, soraAcc, modelCfg.Type == "video")
	if err != nil {
		return nil, err
REDACTED
	if releaseFunc != nil {
		defer releaseFunc()
REDACTED

	if modelCfg.Type == "prompt_enhance" {
		content, err := client.EnhancePrompt(ctx, opts, req.Prompt, modelCfg.ExpansionLevel, modelCfg.DurationS)
		if err != nil {
			return nil, err
	REDACTED
		return &SoraGenerationResult{Content: content, MediaType: "text"REDACTED, nil
REDACTED

	var mediaID string
	if req.Image != "" {
		data, err := s.loadImageBytes(ctx, opts, req.Image)
		if err != nil {
			return nil, err
	REDACTED
		mediaID, err = client.UploadImage(ctx, opts, data, "image.png")
		if err != nil {
			return nil, err
	REDACTED
REDACTED
	if req.Video != "" && modelCfg.Type != "video" {
		return nil, errors.New("视频输入仅支持视频模型")
REDACTED
	if req.Video != "" && req.Image != "" {
		return nil, errors.New("不能同时传入 image 与 video")
REDACTED

	var cleanupCharacter func()
	if req.Video != "" && req.RemixTargetID == "" {
		username, characterID, err := s.createCharacter(ctx, client, opts, req.Video)
		if err != nil {
			return nil, err
	REDACTED
		if strings.TrimSpace(req.Prompt) == "" {
			return &SoraGenerationResult{
				Content:   fmt.Sprintf("角色创建成功，角色名@%s", username),
				MediaType: "text",
		REDACTED, nil
	REDACTED
		if username != "" {
			req.Prompt = fmt.Sprintf("@%s %s", username, strings.TrimSpace(req.Prompt))
	REDACTED
		if characterID != "" {
			cleanupCharacter = func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				_ = client.DeleteCharacter(ctx, opts, characterID)
		REDACTED
	REDACTED
REDACTED
	if cleanupCharacter != nil {
		defer cleanupCharacter()
REDACTED

	var taskID string
	if modelCfg.Type == "image" {
		taskID, err = client.GenerateImage(ctx, opts, req.Prompt, modelCfg.Width, modelCfg.Height, mediaID)
REDACTED else {
		orientation := modelCfg.Orientation
		if orientation == "" {
			orientation = "landscape"
	REDACTED
		modelName := modelCfg.Model
		if modelName == "" {
			modelName = "sy_8"
	REDACTED
		size := modelCfg.Size
		if size == "" {
			size = "small"
	REDACTED
		if req.RemixTargetID != "" {
			taskID, err = client.RemixVideo(ctx, opts, req.RemixTargetID, req.Prompt, orientation, modelCfg.NFrames, "")
	REDACTED else if sora.IsStoryboardPrompt(req.Prompt) {
			formatted := sora.FormatStoryboardPrompt(req.Prompt)
			taskID, err = client.GenerateStoryboard(ctx, opts, formatted, orientation, modelCfg.NFrames, mediaID, "")
	REDACTED else {
			taskID, err = client.GenerateVideo(ctx, opts, req.Prompt, orientation, modelCfg.NFrames, mediaID, "", modelName, size)
	REDACTED
REDACTED
	if err != nil {
		return nil, err
REDACTED

	if s.taskRepo != nil {
		_ = s.taskRepo.Create(ctx, &SoraTask{
			TaskID:    taskID,
			AccountID: account.ID,
			Model:     req.Model,
			Prompt:    req.Prompt,
			Status:    "processing",
			Progress:  0,
			CreatedAt: time.Now(),
	REDACTED)
REDACTED

	result, err := s.pollResult(ctx, client, cfg, opts, taskID, modelCfg.Type == "video", req)
	if err != nil {
		if s.taskRepo != nil {
			_ = s.taskRepo.UpdateStatus(ctx, taskID, "failed", 0, "", err.Error(), timePtr(time.Now()))
	REDACTED
		consecutive := 0
		if s.usageRepo != nil {
			consecutive, _ = s.usageRepo.RecordError(ctx, account.ID)
	REDACTED
		if consecutive >= soraErrorDisableThreshold {
			_ = s.accountRepo.SetError(ctx, account.ID, "Sora 连续错误次数过多，已自动禁用")
	REDACTED
		return nil, err
REDACTED

	if s.taskRepo != nil {
		payload, _ := json.Marshal(result.ResultURLs)
		_ = s.taskRepo.UpdateStatus(ctx, taskID, "completed", 100, string(payload), "", timePtr(time.Now()))
REDACTED
	if s.usageRepo != nil {
		_ = s.usageRepo.RecordSuccess(ctx, account.ID, modelCfg.Type == "video")
REDACTED
	return result, nil
REDACTED

func (s *SoraGatewayService) pollResult(ctx context.Context, client *sora.Client, cfg config.SoraConfig, opts sora.RequestOptions, taskID string, isVideo bool, req SoraGenerationRequest) (*SoraGenerationResult, error) {
	if taskID == "" {
		return nil, errors.New("missing task id")
REDACTED
	pollInterval := 2 * time.Second
	if cfg.PollInterval > 0 {
		pollInterval = time.Duration(cfg.PollInterval*1000) * time.Millisecond
REDACTED
	timeout := 300 * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
REDACTED
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
	REDACTED
		if isVideo {
			pending, err := client.GetPendingTasks(ctx, opts)
			if err == nil {
				for _, task := range pending {
					if stringFromMap(task, "id") == taskID {
						continue
				REDACTED
			REDACTED
		REDACTED
			drafts, err := client.GetVideoDrafts(ctx, opts)
			if err != nil {
				return nil, err
		REDACTED
			items, _ := drafts["items"].([]any)
			for _, item := range items {
				entry, ok := item.(map[string]any)
				if !ok {
					continue
			REDACTED
				if stringFromMap(entry, "task_id") != taskID {
					continue
			REDACTED
				url := firstNonEmpty(stringFromMap(entry, "downloadable_url"), stringFromMap(entry, "url"))
				reason := stringFromMap(entry, "reason_str")
				if url == "" {
					if reason == "" {
						reason = "视频生成失败"
				REDACTED
					return nil, errors.New(reason)
			REDACTED
				finalURL, err := s.handleWatermark(ctx, client, cfg, opts, url, entry, req, opts.AccountID, taskID)
				if err != nil {
					return nil, err
			REDACTED
				return &SoraGenerationResult{
					Content:    buildVideoMarkdown(finalURL),
					MediaType:  "video",
					ResultURLs: []string{finalURLREDACTED,
					TaskID:     taskID,
			REDACTED, nil
		REDACTED
	REDACTED else {
			resp, err := client.GetImageTasks(ctx, opts)
			if err != nil {
				return nil, err
		REDACTED
			tasks, _ := resp["task_responses"].([]any)
			for _, item := range tasks {
				entry, ok := item.(map[string]any)
				if !ok {
					continue
			REDACTED
				if stringFromMap(entry, "id") != taskID {
					continue
			REDACTED
				status := stringFromMap(entry, "status")
				switch status {
				case "succeeded":
					urls := extractImageURLs(entry)
					if len(urls) == 0 {
						return nil, errors.New("image urls empty")
				REDACTED
					content := buildImageMarkdown(urls)
					return &SoraGenerationResult{
						Content:    content,
						MediaType:  "image",
						ResultURLs: urls,
						TaskID:     taskID,
				REDACTED, nil
				case "failed":
					message := stringFromMap(entry, "error_message")
					if message == "" {
						message = "image generation failed"
				REDACTED
					return nil, errors.New(message)
			REDACTED
		REDACTED
	REDACTED

		time.Sleep(pollInterval)
REDACTED
	return nil, errors.New("generation timeout")
REDACTED

func (s *SoraGatewayService) handleWatermark(ctx context.Context, client *sora.Client, cfg config.SoraConfig, opts sora.RequestOptions, url string, entry map[string]any, req SoraGenerationRequest, accountID int64, taskID string) (string, error) {
	if !cfg.WatermarkFree.Enabled {
		return s.cacheVideo(ctx, url, req, accountID, taskID), nil
REDACTED
	generationID := stringFromMap(entry, "id")
	if generationID == "" {
		return s.cacheVideo(ctx, url, req, accountID, taskID), nil
REDACTED
	postID, err := client.PostVideoForWatermarkFree(ctx, opts, generationID)
	if err != nil {
		if cfg.WatermarkFree.FallbackOnFailure {
			return s.cacheVideo(ctx, url, req, accountID, taskID), nil
	REDACTED
		return "", err
REDACTED
	if postID == "" {
		if cfg.WatermarkFree.FallbackOnFailure {
			return s.cacheVideo(ctx, url, req, accountID, taskID), nil
	REDACTED
		return "", errors.New("watermark-free post id empty")
REDACTED
	var parsedURL string
	if cfg.WatermarkFree.ParseMethod == "custom" {
		if cfg.WatermarkFree.CustomParseURL == "" || cfg.WatermarkFree.CustomParseToken == "" {
			return "", errors.New("custom parse 未配置")
	REDACTED
		parsedURL, err = s.fetchCustomWatermarkURL(ctx, cfg.WatermarkFree.CustomParseURL, cfg.WatermarkFree.CustomParseToken, postID)
		if err != nil {
			if cfg.WatermarkFree.FallbackOnFailure {
				return s.cacheVideo(ctx, url, req, accountID, taskID), nil
		REDACTED
			return "", err
	REDACTED
REDACTED else {
		parsedURL = fmt.Sprintf("https://oscdn2.dyysy.com/MP4/%s.mp4", postID)
REDACTED
	cached := s.cacheVideo(ctx, parsedURL, req, accountID, taskID)
	_ = client.DeletePost(ctx, opts, postID)
	return cached, nil
REDACTED

func (s *SoraGatewayService) cacheVideo(ctx context.Context, url string, req SoraGenerationRequest, accountID int64, taskID string) string {
	if s.cacheService == nil {
		return url
REDACTED
	file, err := s.cacheService.CacheVideo(ctx, accountID, req.UserID, taskID, url)
	if err != nil || file == nil {
		return url
REDACTED
	return file.CacheURL
REDACTED

func (s *SoraGatewayService) getAccessToken(ctx context.Context, account *Account) (string, *SoraAccount, error) {
	if account == nil {
		return "", nil, errors.New("account is nil")
REDACTED
	var soraAcc *SoraAccount
	if s.soraAccountRepo != nil {
		soraAcc, _ = s.soraAccountRepo.GetByAccountID(ctx, account.ID)
REDACTED
	if soraAcc != nil && soraAcc.AccessToken != "" {
		return soraAcc.AccessToken, soraAcc, nil
REDACTED
	if account.Credentials != nil {
		if v, ok := account.Credentials["access_token"].(string); ok && v != "" {
			return v, soraAcc, nil
	REDACTED
		if v, ok := account.Credentials["token"].(string); ok && v != "" {
			return v, soraAcc, nil
	REDACTED
REDACTED
	return "", soraAcc, ErrSoraAccountMissingToken
REDACTED

func (s *SoraGatewayService) getClient(ctx context.Context) (*sora.Client, config.SoraConfig) {
	cfg := s.getSoraConfig(ctx)
	if s.httpUpstream == nil {
		return nil, cfg
REDACTED
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return nil, cfg
REDACTED
	timeout := time.Duration(cfg.Timeout) * time.Second
	if cfg.Timeout <= 0 {
		timeout = 120 * time.Second
REDACTED
	enableTLS := false
	if s.cfg != nil {
		enableTLS = s.cfg.Gateway.TLSFingerprint.Enabled
REDACTED
	return sora.NewClient(baseURL, timeout, s.httpUpstream, enableTLS), cfg
REDACTED

func decodeBase64(raw string) ([]byte, error) {
	data := raw
	if idx := strings.Index(raw, "base64,"); idx != -1 {
		data = raw[idx+7:]
REDACTED
	return base64.StdEncoding.DecodeString(data)
REDACTED

func extractImageURLs(entry map[string]any) []string {
	generations, _ := entry["generations"].([]any)
	urls := make([]string, 0, len(generations))
	for _, gen := range generations {
		m, ok := gen.(map[string]any)
		if !ok {
			continue
	REDACTED
		if url, ok := m["url"].(string); ok && url != "" {
			urls = append(urls, url)
	REDACTED
REDACTED
	return urls
REDACTED

func buildImageMarkdown(urls []string) string {
	parts := make([]string, 0, len(urls))
	for _, u := range urls {
		parts = append(parts, fmt.Sprintf("![Generated Image](%s)", u))
REDACTED
	return strings.Join(parts, "\n")
REDACTED

func buildVideoMarkdown(url string) string {
	return fmt.Sprintf("```html\n<video src='%s' controls></video>\n```", url)
REDACTED

func stringFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
REDACTED
	if v, ok := m[key].(string); ok {
		return v
REDACTED
	return ""
REDACTED

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
	REDACTED
REDACTED
	return ""
REDACTED

func isSoraProAccount(acc *SoraAccount) bool {
	if acc == nil {
		return false
REDACTED
	return strings.EqualFold(acc.PlanType, "chatgpt_pro")
REDACTED

func timePtr(t time.Time) *time.Time {
	return &t
REDACTED

// fetchCustomWatermarkURL 使用自定义解析服务获取无水印视频 URL
func (s *SoraGatewayService) fetchCustomWatermarkURL(ctx context.Context, parseURL, parseToken, postID string) (string, error) {
	// 使用项目的 URL 校验器验证 parseURL 格式，防止 SSRF 攻击
	if _, err := urlvalidator.ValidateHTTPSURL(parseURL, urlvalidator.ValidationOptions{REDACTED); err != nil {
		return "", fmt.Errorf("无效的解析服务地址: %w", err)
REDACTED

	payload := map[string]any{
		"url":   fmt.Sprintf("https://sora.chatgpt.com/p/%s", postID),
		"token": parseToken,
REDACTED
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
REDACTED
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(parseURL, "/")+"/get-sora-link", strings.NewReader(string(body)))
	if err != nil {
		return "", err
REDACTED
	req.Header.Set("Content-Type", "application/json")

	// 复用 httpUpstream，遵守代理和 TLS 配置
	enableTLS := false
	if s.cfg != nil {
		enableTLS = s.cfg.Gateway.TLSFingerprint.Enabled
REDACTED
	resp, err := s.httpUpstream.DoWithTLS(req, "", 0, 1, enableTLS)
	if err != nil {
		return "", err
REDACTED
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("custom parse failed: %d", resp.StatusCode)
REDACTED
	var parsed map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
REDACTED
	if errMsg, ok := parsed["error"].(string); ok && errMsg != "" {
		return "", errors.New(errMsg)
REDACTED
	if link, ok := parsed["download_link"].(string); ok {
		return link, nil
REDACTED
	return "", errors.New("custom parse response missing download_link")
REDACTED

const (
	soraSlotImageLock   int64 = 1
	soraSlotImageLimit  int64 = 2
	soraSlotVideoLimit  int64 = 3
	soraDefaultUsername       = "character"
)

func (s *SoraGatewayService) CallLogicMode(ctx context.Context) string {
	return strings.TrimSpace(s.getSoraConfig(ctx).CallLogicMode)
REDACTED

func (s *SoraGatewayService) getSoraConfig(ctx context.Context) config.SoraConfig {
	if s.settingService != nil {
		return s.settingService.GetSoraConfig(ctx)
REDACTED
	if s.cfg != nil {
		return s.cfg.Sora
REDACTED
	return config.SoraConfig{REDACTED
REDACTED

func (s *SoraGatewayService) acquireSoraSlots(ctx context.Context, account *Account, soraAcc *SoraAccount, isVideo bool) (func(), error) {
	if s.concurrency == nil || account == nil || soraAcc == nil {
		return nil, nil
REDACTED
	releases := make([]func(), 0, 2)
	appendRelease := func(release func()) {
		if release != nil {
			releases = append(releases, release)
	REDACTED
REDACTED
	// 错误时释放所有已获取的槽位
	releaseAll := func() {
		for _, r := range releases {
			r()
	REDACTED
REDACTED

	if isVideo {
		if soraAcc.VideoConcurrency > 0 {
			release, err := s.acquireSoraSlot(ctx, account.ID, soraAcc.VideoConcurrency, soraSlotVideoLimit)
			if err != nil {
				releaseAll()
				return nil, err
		REDACTED
			appendRelease(release)
	REDACTED
REDACTED else {
		release, err := s.acquireSoraSlot(ctx, account.ID, 1, soraSlotImageLock)
		if err != nil {
			releaseAll()
			return nil, err
	REDACTED
		appendRelease(release)
		if soraAcc.ImageConcurrency > 0 {
			release, err := s.acquireSoraSlot(ctx, account.ID, soraAcc.ImageConcurrency, soraSlotImageLimit)
			if err != nil {
				releaseAll() // 释放已获取的 soraSlotImageLock
				return nil, err
		REDACTED
			appendRelease(release)
	REDACTED
REDACTED

	if len(releases) == 0 {
		return nil, nil
REDACTED
	return func() {
		for _, release := range releases {
			release()
	REDACTED
REDACTED, nil
REDACTED

func (s *SoraGatewayService) acquireSoraSlot(ctx context.Context, accountID int64, maxConcurrency int, slotType int64) (func(), error) {
	if s.concurrency == nil || maxConcurrency <= 0 {
		return nil, nil
REDACTED
	derivedID := soraConcurrencyAccountID(accountID, slotType)
	result, err := s.concurrency.AcquireAccountSlot(ctx, derivedID, maxConcurrency)
	if err != nil {
		return nil, err
REDACTED
	if !result.Acquired {
		return nil, ErrSoraAccountNotEligible
REDACTED
	return result.ReleaseFunc, nil
REDACTED

func soraConcurrencyAccountID(accountID int64, slotType int64) int64 {
	if accountID < 0 {
		accountID = -accountID
REDACTED
	return -(accountID*10 + slotType)
REDACTED

func (s *SoraGatewayService) createCharacter(ctx context.Context, client *sora.Client, opts sora.RequestOptions, rawVideo string) (string, string, error) {
	videoBytes, err := s.loadVideoBytes(ctx, opts, rawVideo)
	if err != nil {
		return "", "", err
REDACTED
	cameoID, err := client.UploadCharacterVideo(ctx, opts, videoBytes)
	if err != nil {
		return "", "", err
REDACTED
	status, err := s.pollCameoStatus(ctx, client, opts, cameoID)
	if err != nil {
		return "", "", err
REDACTED
	username := processCharacterUsername(stringFromMap(status, "username_hint"))
	if username == "" {
		username = soraDefaultUsername
REDACTED
	displayName := stringFromMap(status, "display_name_hint")
	if displayName == "" {
		displayName = "Character"
REDACTED
	profileURL := stringFromMap(status, "profile_asset_url")
	if profileURL == "" {
		return "", "", errors.New("profile asset url missing")
REDACTED
	avatarData, err := client.DownloadCharacterImage(ctx, opts, profileURL)
	if err != nil {
		return "", "", err
REDACTED
	assetPointer, err := client.UploadCharacterImage(ctx, opts, avatarData)
	if err != nil {
		return "", "", err
REDACTED
	characterID, err := client.FinalizeCharacter(ctx, opts, cameoID, username, displayName, assetPointer)
	if err != nil {
		return "", "", err
REDACTED
	if err := client.SetCharacterPublic(ctx, opts, cameoID); err != nil {
		return "", "", err
REDACTED
	return username, characterID, nil
REDACTED

func (s *SoraGatewayService) pollCameoStatus(ctx context.Context, client *sora.Client, opts sora.RequestOptions, cameoID string) (map[string]any, error) {
	if cameoID == "" {
		return nil, errors.New("cameo id empty")
REDACTED
	timeout := 600 * time.Second
	pollInterval := 5 * time.Second
	deadline := time.Now().Add(timeout)
	consecutiveErrors := 0
	maxConsecutiveErrors := 3

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
	REDACTED
		time.Sleep(pollInterval)
		status, err := client.GetCameoStatus(ctx, opts, cameoID)
		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= maxConsecutiveErrors {
				return nil, err
		REDACTED
			continue
	REDACTED
		consecutiveErrors = 0
		statusValue := stringFromMap(status, "status")
		statusMessage := stringFromMap(status, "status_message")
		if statusValue == "failed" {
			if statusMessage == "" {
				statusMessage = "角色创建失败"
		REDACTED
			return nil, fmt.Errorf("角色创建失败: %s", statusMessage)
	REDACTED
		if strings.EqualFold(statusMessage, "Completed") || strings.EqualFold(statusValue, "finalized") {
			return status, nil
	REDACTED
REDACTED
	return nil, errors.New("角色创建超时")
REDACTED

func (s *SoraGatewayService) loadVideoBytes(ctx context.Context, opts sora.RequestOptions, rawVideo string) ([]byte, error) {
	trimmed := strings.TrimSpace(rawVideo)
	if trimmed == "" {
		return nil, errors.New("video data is empty")
REDACTED
	if looksLikeURL(trimmed) {
		if err := s.validateMediaURL(trimmed); err != nil {
			return nil, err
	REDACTED
		return s.downloadMedia(ctx, opts, trimmed, maxVideoDownloadSize)
REDACTED
	return decodeBase64(trimmed)
REDACTED

func (s *SoraGatewayService) loadImageBytes(ctx context.Context, opts sora.RequestOptions, rawImage string) ([]byte, error) {
	trimmed := strings.TrimSpace(rawImage)
	if trimmed == "" {
		return nil, errors.New("image data is empty")
REDACTED
	if looksLikeURL(trimmed) {
		if err := s.validateMediaURL(trimmed); err != nil {
			return nil, err
	REDACTED
		return s.downloadMedia(ctx, opts, trimmed, maxImageDownloadSize)
REDACTED
	return decodeBase64(trimmed)
REDACTED

func (s *SoraGatewayService) validateMediaURL(rawURL string) error {
	cfg := s.cfg
	if cfg == nil {
		return nil
REDACTED
	if cfg.Security.URLAllowlist.Enabled {
		_, err := urlvalidator.ValidateHTTPSURL(rawURL, urlvalidator.ValidationOptions{
			AllowedHosts:     cfg.Security.URLAllowlist.UpstreamHosts,
			RequireAllowlist: true,
			AllowPrivate:     cfg.Security.URLAllowlist.AllowPrivateHosts,
	REDACTED)
		if err != nil {
			return fmt.Errorf("媒体地址不合法: %w", err)
	REDACTED
		return nil
REDACTED
	if _, err := urlvalidator.ValidateURLFormat(rawURL, cfg.Security.URLAllowlist.AllowInsecureHTTP); err != nil {
		return fmt.Errorf("媒体地址不合法: %w", err)
REDACTED
	return nil
REDACTED

func (s *SoraGatewayService) downloadMedia(ctx context.Context, opts sora.RequestOptions, mediaURL string, maxSize int64) ([]byte, error) {
	if s.httpUpstream == nil {
		return nil, errors.New("upstream is nil")
REDACTED
	req, err := http.NewRequestWithContext(ctx, "GET", mediaURL, nil)
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	enableTLS := false
	if s.cfg != nil {
		enableTLS = s.cfg.Gateway.TLSFingerprint.Enabled
REDACTED
	resp, err := s.httpUpstream.DoWithTLS(req, opts.ProxyURL, opts.AccountID, opts.AccountConcurrency, enableTLS)
	if err != nil {
		return nil, err
REDACTED
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("下载失败: %d", resp.StatusCode)
REDACTED

	// 使用 LimitReader 限制最大读取大小，防止 DoS 攻击
	limitedReader := io.LimitReader(resp.Body, maxSize+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
REDACTED

	// 检查是否超过大小限制
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("媒体文件过大 (最大 %d 字节, 实际 %d 字节)", maxSize, len(data))
REDACTED

	return data, nil
REDACTED

func processCharacterUsername(usernameHint string) string {
	trimmed := strings.TrimSpace(usernameHint)
	if trimmed == "" {
		return ""
REDACTED
	base := trimmed
	if idx := strings.LastIndex(trimmed, "."); idx != -1 && idx+1 < len(trimmed) {
		base = trimmed[idx+1:]
REDACTED
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	return fmt.Sprintf("%s%d", base, rng.Intn(900)+100)
REDACTED

func looksLikeURL(value string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://")
REDACTED
