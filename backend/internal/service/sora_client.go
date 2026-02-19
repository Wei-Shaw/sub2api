package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	openaioauth "github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"golang.org/x/crypto/sha3"
)

const (
	soraChatGPTBaseURL   = "https://chatgpt.com"
	soraSentinelFlow     = "sora_2_create_task"
	soraDefaultUserAgent = "Sora/1.2026.007 (Android 15; 24122RKC7C; build 2600700)"
)

var (
	soraSessionAuthURL = "https://sora.chatgpt.com/api/auth/session"
	soraOAuthTokenURL  = "https://auth.openai.com/oauth/token"
)

const (
	soraPowMaxIteration = 500000
)

var soraPowCores = []int{8, 16, 24, 32REDACTED

var soraPowScripts = []string{
	"https://cdn.oaistatic.com/_next/static/cXh69klOLzS0Gy2joLDRS/_ssgManifest.js?dpl=453ebaec0d44c2decab71692e1bfe39be35a24b3",
REDACTED

var soraPowDPL = []string{
	"prod-f501fe933b3edf57aea882da888e1a544df99840",
REDACTED

var soraPowNavigatorKeys = []string{
	"registerProtocolHandler−function registerProtocolHandler() { [native code] REDACTED",
	"storage−[object StorageManager]",
	"locks−[object LockManager]",
	"appCodeName−Mozilla",
	"permissions−[object Permissions]",
	"webdriver−false",
	"vendor−Google Inc.",
	"mediaDevices−[object MediaDevices]",
	"cookieEnabled−true",
	"product−Gecko",
	"productSub−20030107",
	"hardwareConcurrency−32",
	"onLine−true",
REDACTED

var soraPowDocumentKeys = []string{
	"_reactListeningo743lnnpvdg",
	"location",
REDACTED

var soraPowWindowKeys = []string{
	"0", "window", "self", "document", "name", "location",
	"navigator", "screen", "innerWidth", "innerHeight",
	"localStorage", "sessionStorage", "crypto", "performance",
	"fetch", "setTimeout", "setInterval", "console",
REDACTED

var soraDesktopUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 11.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
REDACTED

var soraRand = rand.New(rand.NewSource(time.Now().UnixNano()))
var soraRandMu sync.Mutex
var soraPerfStart = time.Now()

// SoraClient 定义直连 Sora 的任务操作接口。
type SoraClient interface {
	Enabled() bool
	UploadImage(ctx context.Context, account *Account, data []byte, filename string) (string, error)
	CreateImageTask(ctx context.Context, account *Account, req SoraImageRequest) (string, error)
	CreateVideoTask(ctx context.Context, account *Account, req SoraVideoRequest) (string, error)
	EnhancePrompt(ctx context.Context, account *Account, prompt, expansionLevel string, durationS int) (string, error)
	GetImageTask(ctx context.Context, account *Account, taskID string) (*SoraImageTaskStatus, error)
	GetVideoTask(ctx context.Context, account *Account, taskID string) (*SoraVideoTaskStatus, error)
REDACTED

// SoraImageRequest 图片生成请求参数
type SoraImageRequest struct {
	Prompt  string
	Width   int
	Height  int
	MediaID string
REDACTED

// SoraVideoRequest 视频生成请求参数
type SoraVideoRequest struct {
	Prompt        string
	Orientation   string
	Frames        int
	Model         string
	Size          string
	MediaID       string
	RemixTargetID string
REDACTED

// SoraImageTaskStatus 图片任务状态
type SoraImageTaskStatus struct {
	ID          string
	Status      string
	ProgressPct float64
	URLs        []string
	ErrorMsg    string
REDACTED

// SoraVideoTaskStatus 视频任务状态
type SoraVideoTaskStatus struct {
	ID          string
	Status      string
	ProgressPct int
	URLs        []string
	ErrorMsg    string
REDACTED

// SoraUpstreamError 上游错误
type SoraUpstreamError struct {
	StatusCode int
	Message    string
	Headers    http.Header
	Body       []byte
REDACTED

func (e *SoraUpstreamError) Error() string {
	if e == nil {
		return "sora upstream error"
REDACTED
	if e.Message != "" {
		return fmt.Sprintf("sora upstream error: %d %s", e.StatusCode, e.Message)
REDACTED
	return fmt.Sprintf("sora upstream error: %d", e.StatusCode)
REDACTED

// SoraDirectClient 直连 Sora 实现
type SoraDirectClient struct {
	cfg             *config.Config
	httpUpstream    HTTPUpstream
	tokenProvider   *OpenAITokenProvider
	accountRepo     AccountRepository
	soraAccountRepo SoraAccountRepository
	baseURL         string
REDACTED

// NewSoraDirectClient 创建 Sora 直连客户端
func NewSoraDirectClient(cfg *config.Config, httpUpstream HTTPUpstream, tokenProvider *OpenAITokenProvider) *SoraDirectClient {
	baseURL := ""
	if cfg != nil {
		rawBaseURL := strings.TrimRight(strings.TrimSpace(cfg.Sora.Client.BaseURL), "/")
		baseURL = normalizeSoraBaseURL(rawBaseURL)
		if rawBaseURL != "" && baseURL != rawBaseURL {
			log.Printf("[SoraClient] normalized base_url from %s to %s", sanitizeSoraLogURL(rawBaseURL), sanitizeSoraLogURL(baseURL))
	REDACTED
REDACTED
	return &SoraDirectClient{
		cfg:           cfg,
		httpUpstream:  httpUpstream,
		tokenProvider: tokenProvider,
		baseURL:       baseURL,
REDACTED
REDACTED

func (c *SoraDirectClient) SetAccountRepositories(accountRepo AccountRepository, soraAccountRepo SoraAccountRepository) {
	if c == nil {
		return
REDACTED
	c.accountRepo = accountRepo
	c.soraAccountRepo = soraAccountRepo
REDACTED

// Enabled 判断是否启用 Sora 直连
func (c *SoraDirectClient) Enabled() bool {
	if c == nil {
		return false
REDACTED
	if strings.TrimSpace(c.baseURL) != "" {
		return true
REDACTED
	if c.cfg == nil {
		return false
REDACTED
	return strings.TrimSpace(normalizeSoraBaseURL(c.cfg.Sora.Client.BaseURL)) != ""
REDACTED

// PreflightCheck 在创建任务前执行账号能力预检。
// 当前仅对视频模型执行 /nf/check 预检，用于提前识别额度耗尽或能力缺失。
func (c *SoraDirectClient) PreflightCheck(ctx context.Context, account *Account, requestedModel string, modelCfg SoraModelConfig) error {
	if modelCfg.Type != "video" {
		return nil
REDACTED
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return err
REDACTED
	headers := c.buildBaseHeaders(token, c.defaultUserAgent())
	headers.Set("Accept", "application/json")
	body, _, err := c.doRequest(ctx, account, http.MethodGet, c.buildURL("/nf/check"), headers, nil, false)
	if err != nil {
		var upstreamErr *SoraUpstreamError
		if errors.As(err, &upstreamErr) && upstreamErr.StatusCode == http.StatusNotFound {
			return &SoraUpstreamError{
				StatusCode: http.StatusForbidden,
				Message:    "当前账号未开通 Sora2 能力或无可用配额",
				Headers:    upstreamErr.Headers,
				Body:       upstreamErr.Body,
		REDACTED
	REDACTED
		return err
REDACTED

	rateLimitReached := gjson.GetBytes(body, "rate_limit_and_credit_balance.rate_limit_reached").Bool()
	remaining := gjson.GetBytes(body, "rate_limit_and_credit_balance.estimated_num_videos_remaining")
	if rateLimitReached || (remaining.Exists() && remaining.Int() <= 0) {
		msg := "当前账号 Sora2 可用配额不足"
		if requestedModel != "" {
			msg = fmt.Sprintf("当前账号 %s 可用配额不足", requestedModel)
	REDACTED
		return &SoraUpstreamError{
			StatusCode: http.StatusTooManyRequests,
			Message:    msg,
			Headers:    http.Header{REDACTED,
	REDACTED
REDACTED
	return nil
REDACTED

func (c *SoraDirectClient) UploadImage(ctx context.Context, account *Account, data []byte, filename string) (string, error) {
	if len(data) == 0 {
		return "", errors.New("empty image data")
REDACTED
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return "", err
REDACTED
	if filename == "" {
		filename = "image.png"
REDACTED
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	contentType := mime.TypeByExtension(path.Ext(filename))
	if contentType == "" {
		contentType = "application/octet-stream"
REDACTED
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	partHeader.Set("Content-Type", contentType)
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return "", err
REDACTED
	if _, err := part.Write(data); err != nil {
		return "", err
REDACTED
	if err := writer.WriteField("file_name", filename); err != nil {
		return "", err
REDACTED
	if err := writer.Close(); err != nil {
		return "", err
REDACTED

	headers := c.buildBaseHeaders(token, c.defaultUserAgent())
	headers.Set("Content-Type", writer.FormDataContentType())

	respBody, _, err := c.doRequest(ctx, account, http.MethodPost, c.buildURL("/uploads"), headers, &body, false)
	if err != nil {
		return "", err
REDACTED
	id := strings.TrimSpace(gjson.GetBytes(respBody, "id").String())
	if id == "" {
		return "", errors.New("upload response missing id")
REDACTED
	return id, nil
REDACTED

func (c *SoraDirectClient) CreateImageTask(ctx context.Context, account *Account, req SoraImageRequest) (string, error) {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return "", err
REDACTED
	operation := "simple_compose"
	inpaintItems := []map[string]any{REDACTED
	if strings.TrimSpace(req.MediaID) != "" {
		operation = "remix"
		inpaintItems = append(inpaintItems, map[string]any{
			"type":            "image",
			"frame_index":     0,
			"upload_media_id": req.MediaID,
	REDACTED)
REDACTED
	payload := map[string]any{
		"type":          "image_gen",
		"operation":     operation,
		"prompt":        req.Prompt,
		"width":         req.Width,
		"height":        req.Height,
		"n_variants":    1,
		"n_frames":      1,
		"inpaint_items": inpaintItems,
REDACTED
	headers := c.buildBaseHeaders(token, c.defaultUserAgent())
	headers.Set("Content-Type", "application/json")
	headers.Set("Origin", "https://sora.chatgpt.com")
	headers.Set("Referer", "https://sora.chatgpt.com/")

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
REDACTED
	sentinel, err := c.generateSentinelToken(ctx, account, token)
	if err != nil {
		return "", err
REDACTED
	headers.Set("openai-sentinel-token", sentinel)

	respBody, _, err := c.doRequest(ctx, account, http.MethodPost, c.buildURL("/video_gen"), headers, bytes.NewReader(body), true)
	if err != nil {
		return "", err
REDACTED
	taskID := strings.TrimSpace(gjson.GetBytes(respBody, "id").String())
	if taskID == "" {
		return "", errors.New("image task response missing id")
REDACTED
	return taskID, nil
REDACTED

func (c *SoraDirectClient) CreateVideoTask(ctx context.Context, account *Account, req SoraVideoRequest) (string, error) {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return "", err
REDACTED
	orientation := req.Orientation
	if orientation == "" {
		orientation = "landscape"
REDACTED
	nFrames := req.Frames
	if nFrames <= 0 {
		nFrames = 450
REDACTED
	model := req.Model
	if model == "" {
		model = "sy_8"
REDACTED
	size := req.Size
	if size == "" {
		size = "small"
REDACTED

	inpaintItems := []map[string]any{REDACTED
	if strings.TrimSpace(req.MediaID) != "" {
		inpaintItems = append(inpaintItems, map[string]any{
			"kind":      "upload",
			"upload_id": req.MediaID,
	REDACTED)
REDACTED
	payload := map[string]any{
		"kind":          "video",
		"prompt":        req.Prompt,
		"orientation":   orientation,
		"size":          size,
		"n_frames":      nFrames,
		"model":         model,
		"inpaint_items": inpaintItems,
REDACTED
	if strings.TrimSpace(req.RemixTargetID) != "" {
		payload["remix_target_id"] = req.RemixTargetID
		payload["cameo_ids"] = []string{REDACTED
		payload["cameo_replacements"] = map[string]any{REDACTED
REDACTED

	headers := c.buildBaseHeaders(token, c.defaultUserAgent())
	headers.Set("Content-Type", "application/json")
	headers.Set("Origin", "https://sora.chatgpt.com")
	headers.Set("Referer", "https://sora.chatgpt.com/")
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
REDACTED
	sentinel, err := c.generateSentinelToken(ctx, account, token)
	if err != nil {
		return "", err
REDACTED
	headers.Set("openai-sentinel-token", sentinel)

	respBody, _, err := c.doRequest(ctx, account, http.MethodPost, c.buildURL("/nf/create"), headers, bytes.NewReader(body), true)
	if err != nil {
		return "", err
REDACTED
	taskID := strings.TrimSpace(gjson.GetBytes(respBody, "id").String())
	if taskID == "" {
		return "", errors.New("video task response missing id")
REDACTED
	return taskID, nil
REDACTED

func (c *SoraDirectClient) EnhancePrompt(ctx context.Context, account *Account, prompt, expansionLevel string, durationS int) (string, error) {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return "", err
REDACTED
	if strings.TrimSpace(expansionLevel) == "" {
		expansionLevel = "medium"
REDACTED
	if durationS <= 0 {
		durationS = 10
REDACTED

	payload := map[string]any{
		"prompt":          prompt,
		"expansion_level": expansionLevel,
		"duration_s":      durationS,
REDACTED
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
REDACTED

	headers := c.buildBaseHeaders(token, c.defaultUserAgent())
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	headers.Set("Origin", "https://sora.chatgpt.com")
	headers.Set("Referer", "https://sora.chatgpt.com/")

	respBody, _, err := c.doRequest(ctx, account, http.MethodPost, c.buildURL("/editor/enhance_prompt"), headers, bytes.NewReader(body), false)
	if err != nil {
		return "", err
REDACTED
	enhancedPrompt := strings.TrimSpace(gjson.GetBytes(respBody, "enhanced_prompt").String())
	if enhancedPrompt == "" {
		return "", errors.New("enhance_prompt response missing enhanced_prompt")
REDACTED
	return enhancedPrompt, nil
REDACTED

func (c *SoraDirectClient) GetImageTask(ctx context.Context, account *Account, taskID string) (*SoraImageTaskStatus, error) {
	status, found, err := c.fetchRecentImageTask(ctx, account, taskID, c.recentTaskLimit())
	if err != nil {
		return nil, err
REDACTED
	if found {
		return status, nil
REDACTED
	maxLimit := c.recentTaskLimitMax()
	if maxLimit > 0 && maxLimit != c.recentTaskLimit() {
		status, found, err = c.fetchRecentImageTask(ctx, account, taskID, maxLimit)
		if err != nil {
			return nil, err
	REDACTED
		if found {
			return status, nil
	REDACTED
REDACTED
	return &SoraImageTaskStatus{ID: taskID, Status: "processing"REDACTED, nil
REDACTED

func (c *SoraDirectClient) fetchRecentImageTask(ctx context.Context, account *Account, taskID string, limit int) (*SoraImageTaskStatus, bool, error) {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return nil, false, err
REDACTED
	headers := c.buildBaseHeaders(token, c.defaultUserAgent())
	if limit <= 0 {
		limit = 20
REDACTED
	endpoint := fmt.Sprintf("/v2/recent_tasks?limit=%d", limit)
	respBody, _, err := c.doRequest(ctx, account, http.MethodGet, c.buildURL(endpoint), headers, nil, false)
	if err != nil {
		return nil, false, err
REDACTED
	var found *SoraImageTaskStatus
	gjson.GetBytes(respBody, "task_responses").ForEach(func(_, item gjson.Result) bool {
		if item.Get("id").String() != taskID {
			return true // continue
	REDACTED
		status := strings.TrimSpace(item.Get("status").String())
		progress := item.Get("progress_pct").Float()
		var urls []string
		item.Get("generations").ForEach(func(_, gen gjson.Result) bool {
			if u := strings.TrimSpace(gen.Get("url").String()); u != "" {
				urls = append(urls, u)
		REDACTED
			return true
	REDACTED)
		found = &SoraImageTaskStatus{
			ID:          taskID,
			Status:      status,
			ProgressPct: progress,
			URLs:        urls,
	REDACTED
		return false // break
REDACTED)
	if found != nil {
		return found, true, nil
REDACTED
	return &SoraImageTaskStatus{ID: taskID, Status: "processing"REDACTED, false, nil
REDACTED

func (c *SoraDirectClient) recentTaskLimit() int {
	if c == nil || c.cfg == nil {
		return 20
REDACTED
	if c.cfg.Sora.Client.RecentTaskLimit > 0 {
		return c.cfg.Sora.Client.RecentTaskLimit
REDACTED
	return 20
REDACTED

func (c *SoraDirectClient) recentTaskLimitMax() int {
	if c == nil || c.cfg == nil {
		return 0
REDACTED
	if c.cfg.Sora.Client.RecentTaskLimitMax > 0 {
		return c.cfg.Sora.Client.RecentTaskLimitMax
REDACTED
	return 0
REDACTED

func (c *SoraDirectClient) GetVideoTask(ctx context.Context, account *Account, taskID string) (*SoraVideoTaskStatus, error) {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED
	headers := c.buildBaseHeaders(token, c.defaultUserAgent())

	respBody, _, err := c.doRequest(ctx, account, http.MethodGet, c.buildURL("/nf/pending/v2"), headers, nil, false)
	if err != nil {
		return nil, err
REDACTED
	// 搜索 pending 列表（JSON 数组）
	pendingResult := gjson.ParseBytes(respBody)
	if pendingResult.IsArray() {
		var pendingFound *SoraVideoTaskStatus
		pendingResult.ForEach(func(_, task gjson.Result) bool {
			if task.Get("id").String() != taskID {
				return true
		REDACTED
			progress := 0
			if v := task.Get("progress_pct"); v.Exists() {
				progress = int(v.Float() * 100)
		REDACTED
			status := strings.TrimSpace(task.Get("status").String())
			pendingFound = &SoraVideoTaskStatus{
				ID:          taskID,
				Status:      status,
				ProgressPct: progress,
		REDACTED
			return false
	REDACTED)
		if pendingFound != nil {
			return pendingFound, nil
	REDACTED
REDACTED

	respBody, _, err = c.doRequest(ctx, account, http.MethodGet, c.buildURL("/project_y/profile/drafts?limit=15"), headers, nil, false)
	if err != nil {
		return nil, err
REDACTED
	var draftFound *SoraVideoTaskStatus
	gjson.GetBytes(respBody, "items").ForEach(func(_, draft gjson.Result) bool {
		if draft.Get("task_id").String() != taskID {
			return true
	REDACTED
		kind := strings.TrimSpace(draft.Get("kind").String())
		reason := strings.TrimSpace(draft.Get("reason_str").String())
		if reason == "" {
			reason = strings.TrimSpace(draft.Get("markdown_reason_str").String())
	REDACTED
		urlStr := strings.TrimSpace(draft.Get("downloadable_url").String())
		if urlStr == "" {
			urlStr = strings.TrimSpace(draft.Get("url").String())
	REDACTED

		if kind == "sora_content_violation" || reason != "" || urlStr == "" {
			msg := reason
			if msg == "" {
				msg = "Content violates guardrails"
		REDACTED
			draftFound = &SoraVideoTaskStatus{
				ID:       taskID,
				Status:   "failed",
				ErrorMsg: msg,
		REDACTED
	REDACTED else {
			draftFound = &SoraVideoTaskStatus{
				ID:     taskID,
				Status: "completed",
				URLs:   []string{urlStrREDACTED,
		REDACTED
	REDACTED
		return false
REDACTED)
	if draftFound != nil {
		return draftFound, nil
REDACTED

	return &SoraVideoTaskStatus{ID: taskID, Status: "processing"REDACTED, nil
REDACTED

func (c *SoraDirectClient) buildURL(endpoint string) string {
	base := strings.TrimRight(strings.TrimSpace(c.baseURL), "/")
	if base == "" && c != nil && c.cfg != nil {
		base = normalizeSoraBaseURL(c.cfg.Sora.Client.BaseURL)
		c.baseURL = base
REDACTED
	if base == "" {
		return endpoint
REDACTED
	if strings.HasPrefix(endpoint, "/") {
		return base + endpoint
REDACTED
	return base + "/" + endpoint
REDACTED

func (c *SoraDirectClient) defaultUserAgent() string {
	if c == nil || c.cfg == nil {
		return soraDefaultUserAgent
REDACTED
	ua := strings.TrimSpace(c.cfg.Sora.Client.UserAgent)
	if ua == "" {
		return soraDefaultUserAgent
REDACTED
	return ua
REDACTED

func (c *SoraDirectClient) getAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
REDACTED

	allowProvider := c.allowOpenAITokenProvider(account)
	var providerErr error
	if allowProvider && c.tokenProvider != nil {
		token, err := c.tokenProvider.GetAccessToken(ctx, account)
		if err == nil && strings.TrimSpace(token) != "" {
			c.logTokenSource(account, "openai_token_provider")
			return token, nil
	REDACTED
		providerErr = err
		if err != nil && c.debugEnabled() {
			c.debugLogf(
				"token_provider_failed account_id=%d platform=%s err=%s",
				account.ID,
				account.Platform,
				logredact.RedactText(err.Error()),
			)
	REDACTED
REDACTED
	token := strings.TrimSpace(account.GetCredential("access_token"))
	if token != "" {
		expiresAt := account.GetCredentialAsTime("expires_at")
		if expiresAt != nil && time.Until(*expiresAt) <= 2*time.Minute {
			refreshed, refreshErr := c.recoverAccessToken(ctx, account, "access_token_expiring")
			if refreshErr == nil && strings.TrimSpace(refreshed) != "" {
				c.logTokenSource(account, "refresh_token_recovered")
				return refreshed, nil
		REDACTED
			if refreshErr != nil && c.debugEnabled() {
				c.debugLogf("token_refresh_before_use_failed account_id=%d err=%s", account.ID, logredact.RedactText(refreshErr.Error()))
		REDACTED
	REDACTED
		c.logTokenSource(account, "account_credentials")
		return token, nil
REDACTED

	recovered, recoverErr := c.recoverAccessToken(ctx, account, "access_token_missing")
	if recoverErr == nil && strings.TrimSpace(recovered) != "" {
		c.logTokenSource(account, "session_or_refresh_recovered")
		return recovered, nil
REDACTED
	if recoverErr != nil && c.debugEnabled() {
		c.debugLogf("token_recover_failed account_id=%d platform=%s err=%s", account.ID, account.Platform, logredact.RedactText(recoverErr.Error()))
REDACTED
	if providerErr != nil {
		return "", providerErr
REDACTED
	if c.tokenProvider != nil && !allowProvider {
		c.logTokenSource(account, "account_credentials(provider_disabled)")
REDACTED
	return "", errors.New("access_token not found")
REDACTED

func (c *SoraDirectClient) recoverAccessToken(ctx context.Context, account *Account, reason string) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
REDACTED

	if sessionToken := strings.TrimSpace(account.GetCredential("session_token")); sessionToken != "" {
		accessToken, expiresAt, err := c.exchangeSessionToken(ctx, account, sessionToken)
		if err == nil && strings.TrimSpace(accessToken) != "" {
			c.applyRecoveredToken(ctx, account, accessToken, "", expiresAt, sessionToken)
			c.logTokenRecover(account, "session_token", reason, true, nil)
			return accessToken, nil
	REDACTED
		c.logTokenRecover(account, "session_token", reason, false, err)
REDACTED

	refreshToken := strings.TrimSpace(account.GetCredential("refresh_token"))
	if refreshToken == "" {
		return "", errors.New("session_token/refresh_token not found")
REDACTED
	accessToken, newRefreshToken, expiresAt, err := c.exchangeRefreshToken(ctx, account, refreshToken)
	if err != nil {
		c.logTokenRecover(account, "refresh_token", reason, false, err)
		return "", err
REDACTED
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("refreshed access_token is empty")
REDACTED
	c.applyRecoveredToken(ctx, account, accessToken, newRefreshToken, expiresAt, "")
	c.logTokenRecover(account, "refresh_token", reason, true, nil)
	return accessToken, nil
REDACTED

func (c *SoraDirectClient) exchangeSessionToken(ctx context.Context, account *Account, sessionToken string) (string, string, error) {
	headers := http.Header{REDACTED
	headers.Set("Cookie", "__Secure-next-auth.session-token="+sessionToken)
	headers.Set("Accept", "application/json")
	headers.Set("Origin", "https://sora.chatgpt.com")
	headers.Set("Referer", "https://sora.chatgpt.com/")
	headers.Set("User-Agent", c.defaultUserAgent())
	body, _, err := c.doRequest(ctx, account, http.MethodGet, soraSessionAuthURL, headers, nil, false)
	if err != nil {
		return "", "", err
REDACTED
	accessToken := strings.TrimSpace(gjson.GetBytes(body, "accessToken").String())
	if accessToken == "" {
		return "", "", errors.New("session exchange missing accessToken")
REDACTED
	expiresAt := strings.TrimSpace(gjson.GetBytes(body, "expires").String())
	return accessToken, expiresAt, nil
REDACTED

func (c *SoraDirectClient) exchangeRefreshToken(ctx context.Context, account *Account, refreshToken string) (string, string, string, error) {
	clientIDs := []string{
		strings.TrimSpace(account.GetCredential("client_id")),
		openaioauth.SoraClientID,
		openaioauth.ClientID,
REDACTED
	tried := make(map[string]struct{REDACTED, len(clientIDs))
	var lastErr error

	for _, clientID := range clientIDs {
		if clientID == "" {
			continue
	REDACTED
		if _, ok := tried[clientID]; ok {
			continue
	REDACTED
		tried[clientID] = struct{REDACTED{REDACTED

		payload := map[string]any{
			"client_id":     clientID,
			"grant_type":    "refresh_token",
			"refresh_token": refreshToken,
			"redirect_uri":  "com.openai.chat://auth0.openai.com/ios/com.openai.chat/callback",
	REDACTED
		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			return "", "", "", err
	REDACTED
		headers := http.Header{REDACTED
		headers.Set("Accept", "application/json")
		headers.Set("Content-Type", "application/json")
		headers.Set("User-Agent", c.defaultUserAgent())

		respBody, _, err := c.doRequest(ctx, account, http.MethodPost, soraOAuthTokenURL, headers, bytes.NewReader(bodyBytes), false)
		if err != nil {
			lastErr = err
			if c.debugEnabled() {
				c.debugLogf("refresh_token_exchange_failed account_id=%d client_id=%s err=%s", account.ID, clientID, logredact.RedactText(err.Error()))
		REDACTED
			continue
	REDACTED
		accessToken := strings.TrimSpace(gjson.GetBytes(respBody, "access_token").String())
		if accessToken == "" {
			lastErr = errors.New("oauth refresh response missing access_token")
			continue
	REDACTED
		newRefreshToken := strings.TrimSpace(gjson.GetBytes(respBody, "refresh_token").String())
		expiresIn := gjson.GetBytes(respBody, "expires_in").Int()
		expiresAt := ""
		if expiresIn > 0 {
			expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339)
	REDACTED
		return accessToken, newRefreshToken, expiresAt, nil
REDACTED

	if lastErr != nil {
		return "", "", "", lastErr
REDACTED
	return "", "", "", errors.New("no available client_id for refresh_token exchange")
REDACTED

func (c *SoraDirectClient) applyRecoveredToken(ctx context.Context, account *Account, accessToken, refreshToken, expiresAt, sessionToken string) {
	if account == nil {
		return
REDACTED
	if account.Credentials == nil {
		account.Credentials = make(map[string]any)
REDACTED
	if strings.TrimSpace(accessToken) != "" {
		account.Credentials["access_token"] = accessToken
REDACTED
	if strings.TrimSpace(refreshToken) != "" {
		account.Credentials["refresh_token"] = refreshToken
REDACTED
	if strings.TrimSpace(expiresAt) != "" {
		account.Credentials["expires_at"] = expiresAt
REDACTED
	if strings.TrimSpace(sessionToken) != "" {
		account.Credentials["session_token"] = sessionToken
REDACTED

	if c.accountRepo != nil {
		if err := c.accountRepo.Update(ctx, account); err != nil {
			if c.debugEnabled() {
				c.debugLogf("persist_recovered_token_failed account_id=%d err=%s", account.ID, logredact.RedactText(err.Error()))
		REDACTED
	REDACTED
REDACTED
	c.updateSoraAccountExtension(ctx, account, accessToken, refreshToken, sessionToken)
REDACTED

func (c *SoraDirectClient) updateSoraAccountExtension(ctx context.Context, account *Account, accessToken, refreshToken, sessionToken string) {
	if c == nil || c.soraAccountRepo == nil || account == nil || account.ID <= 0 {
		return
REDACTED
	updates := make(map[string]any)
	if strings.TrimSpace(accessToken) != "" && strings.TrimSpace(refreshToken) != "" {
		updates["access_token"] = accessToken
		updates["refresh_token"] = refreshToken
REDACTED
	if strings.TrimSpace(sessionToken) != "" {
		updates["session_token"] = sessionToken
REDACTED
	if len(updates) == 0 {
		return
REDACTED
	if err := c.soraAccountRepo.Upsert(ctx, account.ID, updates); err != nil && c.debugEnabled() {
		c.debugLogf("persist_sora_extension_failed account_id=%d err=%s", account.ID, logredact.RedactText(err.Error()))
REDACTED
REDACTED

func (c *SoraDirectClient) logTokenRecover(account *Account, source, reason string, success bool, err error) {
	if !c.debugEnabled() || account == nil {
		return
REDACTED
	if success {
		c.debugLogf("token_recover_success account_id=%d platform=%s source=%s reason=%s", account.ID, account.Platform, source, reason)
		return
REDACTED
	if err == nil {
		c.debugLogf("token_recover_failed account_id=%d platform=%s source=%s reason=%s", account.ID, account.Platform, source, reason)
		return
REDACTED
	c.debugLogf("token_recover_failed account_id=%d platform=%s source=%s reason=%s err=%s", account.ID, account.Platform, source, reason, logredact.RedactText(err.Error()))
REDACTED

func (c *SoraDirectClient) allowOpenAITokenProvider(account *Account) bool {
	if c == nil || c.tokenProvider == nil {
		return false
REDACTED
	if account != nil && account.Platform == PlatformSora {
		return c.cfg != nil && c.cfg.Sora.Client.UseOpenAITokenProvider
REDACTED
	return true
REDACTED

func (c *SoraDirectClient) logTokenSource(account *Account, source string) {
	if !c.debugEnabled() || account == nil {
		return
REDACTED
	c.debugLogf(
		"token_selected account_id=%d platform=%s account_type=%s source=%s",
		account.ID,
		account.Platform,
		account.Type,
		source,
	)
REDACTED

func (c *SoraDirectClient) buildBaseHeaders(token, userAgent string) http.Header {
	headers := http.Header{REDACTED
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
REDACTED
	if userAgent != "" {
		headers.Set("User-Agent", userAgent)
REDACTED
	if c != nil && c.cfg != nil {
		for key, value := range c.cfg.Sora.Client.Headers {
			if strings.EqualFold(key, "authorization") || strings.EqualFold(key, "openai-sentinel-token") {
				continue
		REDACTED
			headers.Set(key, value)
	REDACTED
REDACTED
	return headers
REDACTED

func (c *SoraDirectClient) doRequest(ctx context.Context, account *Account, method, urlStr string, headers http.Header, body io.Reader, allowRetry bool) ([]byte, http.Header, error) {
	if strings.TrimSpace(urlStr) == "" {
		return nil, nil, errors.New("empty upstream url")
REDACTED
	timeout := 0
	if c != nil && c.cfg != nil {
		timeout = c.cfg.Sora.Client.TimeoutSeconds
REDACTED
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
REDACTED
	maxRetries := 0
	if allowRetry && c != nil && c.cfg != nil {
		maxRetries = c.cfg.Sora.Client.MaxRetries
REDACTED
	if maxRetries < 0 {
		maxRetries = 0
REDACTED

	var bodyBytes []byte
	if body != nil {
		b, err := io.ReadAll(body)
		if err != nil {
			return nil, nil, err
	REDACTED
		bodyBytes = b
REDACTED

	attempts := maxRetries + 1
	authRecovered := false
	authRecoverExtraAttemptGranted := false
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if c.debugEnabled() {
			c.debugLogf(
				"request_start method=%s url=%s attempt=%d/%d timeout_s=%d body_bytes=%d proxy_bound=%t headers=%s",
				method,
				sanitizeSoraLogURL(urlStr),
				attempt,
				attempts,
				timeout,
				len(bodyBytes),
				account != nil && account.ProxyID != nil && account.Proxy != nil,
				formatSoraHeaders(headers),
			)
	REDACTED

		var reader io.Reader
		if bodyBytes != nil {
			reader = bytes.NewReader(bodyBytes)
	REDACTED
		req, err := http.NewRequestWithContext(ctx, method, urlStr, reader)
		if err != nil {
			return nil, nil, err
	REDACTED
		req.Header = headers.Clone()
		start := time.Now()

		proxyURL := ""
		if account != nil && account.ProxyID != nil && account.Proxy != nil {
			proxyURL = account.Proxy.URL()
	REDACTED
		resp, err := c.doHTTP(req, proxyURL, account)
		if err != nil {
			lastErr = err
			if c.debugEnabled() {
				c.debugLogf(
					"request_transport_error method=%s url=%s attempt=%d/%d err=%s",
					method,
					sanitizeSoraLogURL(urlStr),
					attempt,
					attempts,
					logredact.RedactText(err.Error()),
				)
		REDACTED
			if attempt < attempts && allowRetry {
				if c.debugEnabled() {
					c.debugLogf("request_retry_scheduled method=%s url=%s reason=transport_error next_attempt=%d/%d", method, sanitizeSoraLogURL(urlStr), attempt+1, attempts)
			REDACTED
				c.sleepRetry(attempt)
				continue
		REDACTED
			return nil, nil, err
	REDACTED

		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, resp.Header, readErr
	REDACTED

		if c.cfg != nil && c.cfg.Sora.Client.Debug {
			c.debugLogf(
				"response_received method=%s url=%s attempt=%d/%d status=%d cost=%s resp_bytes=%d resp_headers=%s",
				method,
				sanitizeSoraLogURL(urlStr),
				attempt,
				attempts,
				resp.StatusCode,
				time.Since(start),
				len(respBody),
				formatSoraHeaders(resp.Header),
			)
	REDACTED

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			if !authRecovered && shouldAttemptSoraTokenRecover(resp.StatusCode, urlStr) && account != nil {
				if recovered, recoverErr := c.recoverAccessToken(ctx, account, fmt.Sprintf("upstream_status_%d", resp.StatusCode)); recoverErr == nil && strings.TrimSpace(recovered) != "" {
					headers.Set("Authorization", "Bearer "+recovered)
					authRecovered = true
					if attempt == attempts && !authRecoverExtraAttemptGranted {
						attempts++
						authRecoverExtraAttemptGranted = true
				REDACTED
					if c.debugEnabled() {
						c.debugLogf("request_retry_with_recovered_token method=%s url=%s status=%d", method, sanitizeSoraLogURL(urlStr), resp.StatusCode)
				REDACTED
					continue
			REDACTED else if recoverErr != nil && c.debugEnabled() {
					c.debugLogf("request_recover_token_failed method=%s url=%s status=%d err=%s", method, sanitizeSoraLogURL(urlStr), resp.StatusCode, logredact.RedactText(recoverErr.Error()))
			REDACTED
		REDACTED
			if c.debugEnabled() {
				c.debugLogf(
					"response_non_success method=%s url=%s attempt=%d/%d status=%d body=%s",
					method,
					sanitizeSoraLogURL(urlStr),
					attempt,
					attempts,
					resp.StatusCode,
					summarizeSoraResponseBody(respBody, 512),
				)
		REDACTED
			upstreamErr := c.buildUpstreamError(resp.StatusCode, resp.Header, respBody, urlStr)
			lastErr = upstreamErr
			if allowRetry && attempt < attempts && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) {
				if c.debugEnabled() {
					c.debugLogf("request_retry_scheduled method=%s url=%s reason=status_%d next_attempt=%d/%d", method, sanitizeSoraLogURL(urlStr), resp.StatusCode, attempt+1, attempts)
			REDACTED
				c.sleepRetry(attempt)
				continue
		REDACTED
			return nil, resp.Header, upstreamErr
	REDACTED
		return respBody, resp.Header, nil
REDACTED
	if lastErr != nil {
		return nil, nil, lastErr
REDACTED
	return nil, nil, errors.New("upstream retries exhausted")
REDACTED

func shouldAttemptSoraTokenRecover(statusCode int, rawURL string) bool {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		parsed, err := url.Parse(strings.TrimSpace(rawURL))
		if err != nil {
			return false
	REDACTED
		host := strings.ToLower(parsed.Hostname())
		if host != "sora.chatgpt.com" && host != "chatgpt.com" {
			return false
	REDACTED
		// 避免在 ST->AT 转换接口上递归触发 token 恢复导致死循环。
		path := strings.ToLower(strings.TrimSpace(parsed.Path))
		if path == "/api/auth/session" {
			return false
	REDACTED
		return true
	default:
		return false
REDACTED
REDACTED

func (c *SoraDirectClient) doHTTP(req *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	enableTLS := c != nil && c.cfg != nil && c.cfg.Gateway.TLSFingerprint.Enabled && !c.cfg.Sora.Client.DisableTLSFingerprint
	if c.httpUpstream != nil {
		accountID := int64(0)
		accountConcurrency := 0
		if account != nil {
			accountID = account.ID
			accountConcurrency = account.Concurrency
	REDACTED
		return c.httpUpstream.DoWithTLS(req, proxyURL, accountID, accountConcurrency, enableTLS)
REDACTED
	return http.DefaultClient.Do(req)
REDACTED

func (c *SoraDirectClient) sleepRetry(attempt int) {
	backoff := time.Duration(attempt*attempt) * time.Second
	if backoff > 10*time.Second {
		backoff = 10 * time.Second
REDACTED
	time.Sleep(backoff)
REDACTED

func (c *SoraDirectClient) buildUpstreamError(status int, headers http.Header, body []byte, requestURL string) error {
	msg := strings.TrimSpace(extractUpstreamErrorMessage(body))
	msg = sanitizeUpstreamErrorMessage(msg)
	if status == http.StatusNotFound && strings.Contains(strings.ToLower(msg), "not found") {
		if hint := soraBaseURLNotFoundHint(requestURL); hint != "" {
			msg = strings.TrimSpace(msg + " " + hint)
	REDACTED
REDACTED
	if msg == "" {
		msg = truncateForLog(body, 256)
REDACTED
	return &SoraUpstreamError{
		StatusCode: status,
		Message:    msg,
		Headers:    headers,
		Body:       body,
REDACTED
REDACTED

func normalizeSoraBaseURL(raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return ""
REDACTED
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return trimmed
REDACTED
	host := strings.ToLower(parsed.Hostname())
	if host != "sora.chatgpt.com" && host != "chatgpt.com" {
		return trimmed
REDACTED
	pathVal := strings.TrimRight(strings.TrimSpace(parsed.Path), "/")
	switch pathVal {
	case "", "/":
		parsed.Path = "/backend"
	case "/backend-api":
		parsed.Path = "/backend"
REDACTED
	return strings.TrimRight(parsed.String(), "/")
REDACTED

func soraBaseURLNotFoundHint(requestURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(requestURL))
	if err != nil || parsed.Host == "" {
		return ""
REDACTED
	host := strings.ToLower(parsed.Hostname())
	if host != "sora.chatgpt.com" && host != "chatgpt.com" {
		return ""
REDACTED
	pathVal := strings.TrimSpace(parsed.Path)
	if strings.HasPrefix(pathVal, "/backend/") || pathVal == "/backend" {
		return ""
REDACTED
	return "(请检查 sora.client.base_url，建议配置为 https://sora.chatgpt.com/backend)"
REDACTED

func (c *SoraDirectClient) generateSentinelToken(ctx context.Context, account *Account, accessToken string) (string, error) {
	reqID := uuid.NewString()
	userAgent := soraRandChoice(soraDesktopUserAgents)
	powToken := soraGetPowToken(userAgent)
	payload := map[string]any{
		"p":    powToken,
		"flow": soraSentinelFlow,
		"id":   reqID,
REDACTED
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
REDACTED
	headers := http.Header{REDACTED
	headers.Set("Accept", "application/json, text/plain, */*")
	headers.Set("Content-Type", "application/json")
	headers.Set("Origin", "https://sora.chatgpt.com")
	headers.Set("Referer", "https://sora.chatgpt.com/")
	headers.Set("User-Agent", userAgent)
	if accessToken != "" {
		headers.Set("Authorization", "Bearer "+accessToken)
REDACTED

	urlStr := soraChatGPTBaseURL + "/backend-api/sentinel/req"
	respBody, _, err := c.doRequest(ctx, account, http.MethodPost, urlStr, headers, bytes.NewReader(body), true)
	if err != nil {
		return "", err
REDACTED
	var resp map[string]any
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", err
REDACTED

	sentinel := soraBuildSentinelToken(soraSentinelFlow, reqID, powToken, resp, userAgent)
	if sentinel == "" {
		return "", errors.New("failed to build sentinel token")
REDACTED
	return sentinel, nil
REDACTED

func soraRandChoice(items []string) string {
	if len(items) == 0 {
		return ""
REDACTED
	soraRandMu.Lock()
	idx := soraRand.Intn(len(items))
	soraRandMu.Unlock()
	return items[idx]
REDACTED

func soraGetPowToken(userAgent string) string {
	configList := soraBuildPowConfig(userAgent)
	seed := strconv.FormatFloat(soraRandFloat(), 'f', -1, 64)
	difficulty := "0fffff"
	solution, _ := soraSolvePow(seed, difficulty, configList)
	return "gAAAAAC" + solution
REDACTED

func soraRandFloat() float64 {
	soraRandMu.Lock()
	defer soraRandMu.Unlock()
	return soraRand.Float64()
REDACTED

func soraBuildPowConfig(userAgent string) []any {
	screen := soraRandChoice([]string{
		strconv.Itoa(1920 + 1080),
		strconv.Itoa(2560 + 1440),
		strconv.Itoa(1920 + 1200),
		strconv.Itoa(2560 + 1600),
REDACTED)
	screenVal, _ := strconv.Atoi(screen)
	perfMs := float64(time.Since(soraPerfStart).Milliseconds())
	wallMs := float64(time.Now().UnixNano()) / 1e6
	diff := wallMs - perfMs
	return []any{
		screenVal,
		soraPowParseTime(),
		4294705152,
		0,
		userAgent,
		soraRandChoice(soraPowScripts),
		soraRandChoice(soraPowDPL),
		"en-US",
		"en-US,es-US,en,es",
		0,
		soraRandChoice(soraPowNavigatorKeys),
		soraRandChoice(soraPowDocumentKeys),
		soraRandChoice(soraPowWindowKeys),
		perfMs,
		uuid.NewString(),
		"",
		soraRandChoiceInt(soraPowCores),
		diff,
REDACTED
REDACTED

func soraRandChoiceInt(items []int) int {
	if len(items) == 0 {
		return 0
REDACTED
	soraRandMu.Lock()
	idx := soraRand.Intn(len(items))
	soraRandMu.Unlock()
	return items[idx]
REDACTED

func soraPowParseTime() string {
	loc := time.FixedZone("EST", -5*3600)
	return time.Now().In(loc).Format("Mon Jan 02 2006 15:04:05 GMT-0700 (Eastern Standard Time)")
REDACTED

func soraSolvePow(seed, difficulty string, configList []any) (string, bool) {
	diffLen := len(difficulty) / 2
	target, err := hexDecodeString(difficulty)
	if err != nil {
		return "", false
REDACTED
	seedBytes := []byte(seed)

	part1 := mustMarshalJSON(configList[:3])
	part2 := mustMarshalJSON(configList[4:9])
	part3 := mustMarshalJSON(configList[10:])

	staticPart1 := append(part1[:len(part1)-1], ',')
	staticPart2 := append([]byte(","), append(part2[1:len(part2)-1], ',')...)
	staticPart3 := append([]byte(","), part3[1:]...)

	for i := 0; i < soraPowMaxIteration; i++ {
		dynamicI := []byte(strconv.Itoa(i))
		dynamicJ := []byte(strconv.Itoa(i >> 1))
		finalJSON := make([]byte, 0, len(staticPart1)+len(dynamicI)+len(staticPart2)+len(dynamicJ)+len(staticPart3))
		finalJSON = append(finalJSON, staticPart1...)
		finalJSON = append(finalJSON, dynamicI...)
		finalJSON = append(finalJSON, staticPart2...)
		finalJSON = append(finalJSON, dynamicJ...)
		finalJSON = append(finalJSON, staticPart3...)

		b64 := base64.StdEncoding.EncodeToString(finalJSON)
		hash := sha3.Sum512(append(seedBytes, []byte(b64)...))
		if bytes.Compare(hash[:diffLen], target[:diffLen]) <= 0 {
			return b64, true
	REDACTED
REDACTED

	errorToken := "wQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("\"%s\"", seed)))
	return errorToken, false
REDACTED

func soraBuildSentinelToken(flow, reqID, powToken string, resp map[string]any, userAgent string) string {
	finalPow := powToken
	proof, _ := resp["proofofwork"].(map[string]any)
	if required, _ := proof["required"].(bool); required {
		seed, _ := proof["seed"].(string)
		difficulty, _ := proof["difficulty"].(string)
		if seed != "" && difficulty != "" {
			configList := soraBuildPowConfig(userAgent)
			solution, _ := soraSolvePow(seed, difficulty, configList)
			finalPow = "gAAAAAB" + solution
	REDACTED
REDACTED
	if !strings.HasSuffix(finalPow, "~S") {
		finalPow += "~S"
REDACTED
	turnstile, _ := resp["turnstile"].(map[string]any)
	tokenPayload := map[string]any{
		"p":    finalPow,
		"t":    safeMapString(turnstile, "dx"),
		"c":    safeString(resp["token"]),
		"id":   reqID,
		"flow": flow,
REDACTED
	encoded, _ := json.Marshal(tokenPayload)
	return string(encoded)
REDACTED

func safeMapString(m map[string]any, key string) string {
	if m == nil {
		return ""
REDACTED
	if v, ok := m[key]; ok {
		return safeString(v)
REDACTED
	return ""
REDACTED

func safeString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		return fmt.Sprintf("%v", val)
REDACTED
REDACTED

func mustMarshalJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
REDACTED

func hexDecodeString(s string) ([]byte, error) {
	dst := make([]byte, len(s)/2)
	_, err := hex.Decode(dst, []byte(s))
	return dst, err
REDACTED

func sanitizeSoraLogURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
REDACTED
	q := parsed.Query()
	q.Del("sig")
	q.Del("expires")
	parsed.RawQuery = q.Encode()
	return parsed.String()
REDACTED

func (c *SoraDirectClient) debugEnabled() bool {
	return c != nil && c.cfg != nil && c.cfg.Sora.Client.Debug
REDACTED

func (c *SoraDirectClient) debugLogf(format string, args ...any) {
	if !c.debugEnabled() {
		return
REDACTED
	log.Printf("[SoraClient] "+format, args...)
REDACTED

func formatSoraHeaders(headers http.Header) string {
	if len(headers) == 0 {
		return "{REDACTED"
REDACTED
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
REDACTED
	sort.Strings(keys)
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		values := headers.Values(key)
		if len(values) == 0 {
			continue
	REDACTED
		val := strings.Join(values, ",")
		if isSensitiveHeader(key) {
			out[key] = "***"
			continue
	REDACTED
		out[key] = truncateForLog([]byte(logredact.RedactText(val)), 160)
REDACTED
	encoded, err := json.Marshal(out)
	if err != nil {
		return "{REDACTED"
REDACTED
	return string(encoded)
REDACTED

func isSensitiveHeader(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	switch k {
	case "authorization", "openai-sentinel-token", "cookie", "set-cookie", "x-api-key":
		return true
	default:
		return false
REDACTED
REDACTED

func summarizeSoraResponseBody(body []byte, maxLen int) string {
	if len(body) == 0 {
		return ""
REDACTED
	var text string
	if json.Valid(body) {
		text = logredact.RedactJSON(body)
REDACTED else {
		text = logredact.RedactText(string(body))
REDACTED
	text = strings.TrimSpace(text)
	if maxLen <= 0 || len(text) <= maxLen {
		return text
REDACTED
	return text[:maxLen] + "...(truncated)"
REDACTED
