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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
	"golang.org/x/crypto/sha3"
)

const (
	soraChatGPTBaseURL   = "https://chatgpt.com"
	soraSentinelFlow     = "sora_2_create_task"
	soraDefaultUserAgent = "Sora/1.2026.007 (Android 15; 24122RKC7C; build 2600700)"
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
	cfg           *config.Config
	httpUpstream  HTTPUpstream
	tokenProvider *OpenAITokenProvider
REDACTED

// NewSoraDirectClient 创建 Sora 直连客户端
func NewSoraDirectClient(cfg *config.Config, httpUpstream HTTPUpstream, tokenProvider *OpenAITokenProvider) *SoraDirectClient {
	return &SoraDirectClient{
		cfg:           cfg,
		httpUpstream:  httpUpstream,
		tokenProvider: tokenProvider,
REDACTED
REDACTED

// Enabled 判断是否启用 Sora 直连
func (c *SoraDirectClient) Enabled() bool {
	if c == nil || c.cfg == nil {
		return false
REDACTED
	return strings.TrimSpace(c.cfg.Sora.Client.BaseURL) != ""
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
	var payload map[string]any
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return "", fmt.Errorf("parse upload response: %w", err)
REDACTED
	id, _ := payload["id"].(string)
	if strings.TrimSpace(id) == "" {
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
	var resp map[string]any
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", err
REDACTED
	taskID, _ := resp["id"].(string)
	if strings.TrimSpace(taskID) == "" {
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
	var resp map[string]any
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", err
REDACTED
	taskID, _ := resp["id"].(string)
	if strings.TrimSpace(taskID) == "" {
		return "", errors.New("video task response missing id")
REDACTED
	return taskID, nil
REDACTED

func (c *SoraDirectClient) GetImageTask(ctx context.Context, account *Account, taskID string) (*SoraImageTaskStatus, error) {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED
	headers := c.buildBaseHeaders(token, c.defaultUserAgent())
	respBody, _, err := c.doRequest(ctx, account, http.MethodGet, c.buildURL("/v2/recent_tasks?limit=20"), headers, nil, false)
	if err != nil {
		return nil, err
REDACTED
	var resp map[string]any
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, err
REDACTED
	taskResponses, _ := resp["task_responses"].([]any)
	for _, item := range taskResponses {
		taskResp, ok := item.(map[string]any)
		if !ok {
			continue
	REDACTED
		if id, _ := taskResp["id"].(string); id == taskID {
			status := strings.TrimSpace(fmt.Sprintf("%v", taskResp["status"]))
			progress := 0.0
			if v, ok := taskResp["progress_pct"].(float64); ok {
				progress = v
		REDACTED
			urls := []string{REDACTED
			if generations, ok := taskResp["generations"].([]any); ok {
				for _, genItem := range generations {
					gen, ok := genItem.(map[string]any)
					if !ok {
						continue
				REDACTED
					if urlStr, ok := gen["url"].(string); ok && strings.TrimSpace(urlStr) != "" {
						urls = append(urls, urlStr)
				REDACTED
			REDACTED
		REDACTED
			return &SoraImageTaskStatus{
				ID:          taskID,
				Status:      status,
				ProgressPct: progress,
				URLs:        urls,
		REDACTED, nil
	REDACTED
REDACTED
	return &SoraImageTaskStatus{ID: taskID, Status: "processing"REDACTED, nil
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
	var pending any
	if err := json.Unmarshal(respBody, &pending); err == nil {
		if list, ok := pending.([]any); ok {
			for _, item := range list {
				task, ok := item.(map[string]any)
				if !ok {
					continue
			REDACTED
				if id, _ := task["id"].(string); id == taskID {
					progress := 0
					if v, ok := task["progress_pct"].(float64); ok {
						progress = int(v * 100)
				REDACTED
					status := strings.TrimSpace(fmt.Sprintf("%v", task["status"]))
					return &SoraVideoTaskStatus{
						ID:          taskID,
						Status:      status,
						ProgressPct: progress,
				REDACTED, nil
			REDACTED
		REDACTED
	REDACTED
REDACTED

	respBody, _, err = c.doRequest(ctx, account, http.MethodGet, c.buildURL("/project_y/profile/drafts?limit=15"), headers, nil, false)
	if err != nil {
		return nil, err
REDACTED
	var draftsResp map[string]any
	if err := json.Unmarshal(respBody, &draftsResp); err != nil {
		return nil, err
REDACTED
	items, _ := draftsResp["items"].([]any)
	for _, item := range items {
		draft, ok := item.(map[string]any)
		if !ok {
			continue
	REDACTED
		if id, _ := draft["task_id"].(string); id == taskID {
			kind := strings.TrimSpace(fmt.Sprintf("%v", draft["kind"]))
			reason := strings.TrimSpace(fmt.Sprintf("%v", draft["reason_str"]))
			if reason == "" {
				reason = strings.TrimSpace(fmt.Sprintf("%v", draft["markdown_reason_str"]))
		REDACTED
			urlStr := strings.TrimSpace(fmt.Sprintf("%v", draft["downloadable_url"]))
			if urlStr == "" {
				urlStr = strings.TrimSpace(fmt.Sprintf("%v", draft["url"]))
		REDACTED

			if kind == "sora_content_violation" || reason != "" || urlStr == "" {
				msg := reason
				if msg == "" {
					msg = "Content violates guardrails"
			REDACTED
				return &SoraVideoTaskStatus{
					ID:       taskID,
					Status:   "failed",
					ErrorMsg: msg,
			REDACTED, nil
		REDACTED
			return &SoraVideoTaskStatus{
				ID:     taskID,
				Status: "completed",
				URLs:   []string{urlStrREDACTED,
		REDACTED, nil
	REDACTED
REDACTED

	return &SoraVideoTaskStatus{ID: taskID, Status: "processing"REDACTED, nil
REDACTED

func (c *SoraDirectClient) buildURL(endpoint string) string {
	base := ""
	if c != nil && c.cfg != nil {
		base = strings.TrimRight(strings.TrimSpace(c.cfg.Sora.Client.BaseURL), "/")
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
	if c.tokenProvider != nil {
		return c.tokenProvider.GetAccessToken(ctx, account)
REDACTED
	token := strings.TrimSpace(account.GetCredential("access_token"))
	if token == "" {
		return "", errors.New("access_token not found")
REDACTED
	return token, nil
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
	for attempt := 1; attempt <= attempts; attempt++ {
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
			if attempt < attempts && allowRetry {
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
			log.Printf("[SoraClient] %s %s status=%d cost=%s", method, sanitizeSoraLogURL(urlStr), resp.StatusCode, time.Since(start))
	REDACTED

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			upstreamErr := c.buildUpstreamError(resp.StatusCode, resp.Header, respBody)
			if allowRetry && attempt < attempts && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) {
				c.sleepRetry(attempt)
				continue
		REDACTED
			return nil, resp.Header, upstreamErr
	REDACTED
		return respBody, resp.Header, nil
REDACTED
	return nil, nil, errors.New("upstream retries exhausted")
REDACTED

func (c *SoraDirectClient) doHTTP(req *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	enableTLS := false
	if c != nil && c.cfg != nil && c.cfg.Gateway.TLSFingerprint.Enabled && !c.cfg.Sora.Client.DisableTLSFingerprint {
		enableTLS = true
REDACTED
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

func (c *SoraDirectClient) buildUpstreamError(status int, headers http.Header, body []byte) error {
	msg := strings.TrimSpace(extractUpstreamErrorMessage(body))
	msg = sanitizeUpstreamErrorMessage(msg)
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
