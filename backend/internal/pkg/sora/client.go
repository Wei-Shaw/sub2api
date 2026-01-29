package sora

import (
	"bytes"
	"context"
	"crypto/sha3"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	chatGPTBaseURL      = "https://chatgpt.com"
	sentinelFlow        = "sora_2_create_task"
	maxAPIResponseSize  = 1 * 1024 * 1024 // 1MB
)

var (
	defaultMobileUA  = "Sora/1.2026.007 (Android 15; Pixel 8 Pro; build 2600700)"
	defaultDesktopUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	sentinelCache    sync.Map // 包级缓存，存储 Sentinel Token，key 为 accountID
)

// sentinelCacheEntry 是 Sentinel Token 缓存条目
type sentinelCacheEntry struct {
	token     string
	expiresAt time.Time
REDACTED

// UpstreamClient defines the HTTP client interface for Sora requests.
type UpstreamClient interface {
	Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error)
	DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, enableTLSFingerprint bool) (*http.Response, error)
REDACTED

// Client is a minimal Sora API client.
type Client struct {
	baseURL              string
	timeout              time.Duration
	upstream             UpstreamClient
	enableTLSFingerprint bool
REDACTED

// RequestOptions configures per-request context.
type RequestOptions struct {
	AccountID          int64
	AccountConcurrency int
	ProxyURL           string
	AccessToken        string
REDACTED

// getCachedSentinel 从缓存中获取 Sentinel Token
func getCachedSentinel(accountID int64) (string, bool) {
	v, ok := sentinelCache.Load(accountID)
	if !ok {
		return "", false
REDACTED
	entry := v.(*sentinelCacheEntry)
	if time.Now().After(entry.expiresAt) {
		sentinelCache.Delete(accountID)
		return "", false
REDACTED
	return entry.token, true
REDACTED

// cacheSentinel 缓存 Sentinel Token
func cacheSentinel(accountID int64, token string) {
	sentinelCache.Store(accountID, &sentinelCacheEntry{
		token:     token,
		expiresAt: time.Now().Add(3 * time.Minute), // 3分钟有效期
REDACTED)
REDACTED

// NewClient creates a Sora client.
func NewClient(baseURL string, timeout time.Duration, upstream UpstreamClient, enableTLSFingerprint bool) *Client {
	return &Client{
		baseURL:              strings.TrimRight(baseURL, "/"),
		timeout:              timeout,
		upstream:             upstream,
		enableTLSFingerprint: enableTLSFingerprint,
REDACTED
REDACTED

// UploadImage uploads an image and returns media ID.
func (c *Client) UploadImage(ctx context.Context, opts RequestOptions, data []byte, filename string) (string, error) {
	if filename == "" {
		filename = "image.png"
REDACTED
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", filename)
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
	resp, err := c.doRequest(ctx, "POST", "/uploads", opts, &buf, writer.FormDataContentType(), false)
	if err != nil {
		return "", err
REDACTED
	return stringFromJSON(resp, "id"), nil
REDACTED

// GenerateImage creates an image generation task.
func (c *Client) GenerateImage(ctx context.Context, opts RequestOptions, prompt string, width, height int, mediaID string) (string, error) {
	operation := "simple_compose"
	var inpaint []map[string]any
	if mediaID != "" {
		operation = "remix"
		inpaint = []map[string]any{
			{
				"type":           "image",
				"frame_index":    0,
				"upload_media_id": mediaID,
		REDACTED,
	REDACTED
REDACTED
	payload := map[string]any{
		"type":        "image_gen",
		"operation":   operation,
		"prompt":      prompt,
		"width":       width,
		"height":      height,
		"n_variants":  1,
		"n_frames":    1,
		"inpaint_items": inpaint,
REDACTED
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
REDACTED
	resp, err := c.doRequest(ctx, "POST", "/video_gen", opts, bytes.NewReader(body), "application/json", true)
	if err != nil {
		return "", err
REDACTED
	return stringFromJSON(resp, "id"), nil
REDACTED

// GenerateVideo creates a video generation task.
func (c *Client) GenerateVideo(ctx context.Context, opts RequestOptions, prompt, orientation string, nFrames int, mediaID, styleID, model, size string) (string, error) {
	var inpaint []map[string]any
	if mediaID != "" {
		inpaint = []map[string]any{{"kind": "upload", "upload_id": mediaIDREDACTEDREDACTED
REDACTED
	payload := map[string]any{
		"kind":         "video",
		"prompt":       prompt,
		"orientation":  orientation,
		"size":         size,
		"n_frames":     nFrames,
		"model":        model,
		"inpaint_items": inpaint,
		"style_id":     styleID,
REDACTED
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
REDACTED
	resp, err := c.doRequest(ctx, "POST", "/nf/create", opts, bytes.NewReader(body), "application/json", true)
	if err != nil {
		return "", err
REDACTED
	return stringFromJSON(resp, "id"), nil
REDACTED

// GenerateStoryboard creates a storyboard video task.
func (c *Client) GenerateStoryboard(ctx context.Context, opts RequestOptions, prompt, orientation string, nFrames int, mediaID, styleID string) (string, error) {
	var inpaint []map[string]any
	if mediaID != "" {
		inpaint = []map[string]any{{"kind": "upload", "upload_id": mediaIDREDACTEDREDACTED
REDACTED
	payload := map[string]any{
		"kind":          "video",
		"prompt":        prompt,
		"title":         "Draft your video",
		"orientation":   orientation,
		"size":          "small",
		"n_frames":      nFrames,
		"storyboard_id": nil,
		"inpaint_items": inpaint,
		"remix_target_id": nil,
		"model":         "sy_8",
		"metadata":      nil,
		"style_id":      styleID,
		"cameo_ids":     nil,
		"cameo_replacements": nil,
REDACTED
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
REDACTED
	resp, err := c.doRequest(ctx, "POST", "/nf/create/storyboard", opts, bytes.NewReader(body), "application/json", true)
	if err != nil {
		return "", err
REDACTED
	return stringFromJSON(resp, "id"), nil
REDACTED

// RemixVideo creates a remix task.
func (c *Client) RemixVideo(ctx context.Context, opts RequestOptions, remixTargetID, prompt, orientation string, nFrames int, styleID string) (string, error) {
	payload := map[string]any{
		"kind":           "video",
		"prompt":         prompt,
		"inpaint_items":  []map[string]any{REDACTED,
		"remix_target_id": remixTargetID,
		"cameo_ids":      []string{REDACTED,
		"cameo_replacements": map[string]any{REDACTED,
		"model":          "sy_8",
		"orientation":    orientation,
		"n_frames":       nFrames,
		"style_id":       styleID,
REDACTED
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
REDACTED
	resp, err := c.doRequest(ctx, "POST", "/nf/create", opts, bytes.NewReader(body), "application/json", true)
	if err != nil {
		return "", err
REDACTED
	return stringFromJSON(resp, "id"), nil
REDACTED

// GetImageTasks returns recent image tasks.
func (c *Client) GetImageTasks(ctx context.Context, opts RequestOptions) (map[string]any, error) {
	return c.doRequest(ctx, "GET", "/v2/recent_tasks?limit=20", opts, nil, "", false)
REDACTED

// GetPendingTasks returns pending video tasks.
func (c *Client) GetPendingTasks(ctx context.Context, opts RequestOptions) ([]map[string]any, error) {
	resp, err := c.doRequestAny(ctx, "GET", "/nf/pending/v2", opts, nil, "", false)
	if err != nil {
		return nil, err
REDACTED
	switch v := resp.(type) {
	case []any:
		return convertList(v), nil
	case map[string]any:
		if list, ok := v["items"].([]any); ok {
			return convertList(list), nil
	REDACTED
		if arr, ok := v["data"].([]any); ok {
			return convertList(arr), nil
	REDACTED
		return convertListFromAny(v), nil
	default:
		return nil, nil
REDACTED
REDACTED

// GetVideoDrafts returns recent video drafts.
func (c *Client) GetVideoDrafts(ctx context.Context, opts RequestOptions) (map[string]any, error) {
	return c.doRequest(ctx, "GET", "/project_y/profile/drafts?limit=15", opts, nil, "", false)
REDACTED

// EnhancePrompt calls prompt enhancement API.
func (c *Client) EnhancePrompt(ctx context.Context, opts RequestOptions, prompt, expansionLevel string, durationS int) (string, error) {
	payload := map[string]any{
		"prompt":          prompt,
		"expansion_level": expansionLevel,
		"duration_s":      durationS,
REDACTED
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
REDACTED
	resp, err := c.doRequest(ctx, "POST", "/editor/enhance_prompt", opts, bytes.NewReader(body), "application/json", false)
	if err != nil {
		return "", err
REDACTED
	return stringFromJSON(resp, "enhanced_prompt"), nil
REDACTED

// PostVideoForWatermarkFree publishes a video for watermark-free parsing.
func (c *Client) PostVideoForWatermarkFree(ctx context.Context, opts RequestOptions, generationID string) (string, error) {
	payload := map[string]any{
		"attachments_to_create": []map[string]any{{
			"generation_id": generationID,
			"kind":          "sora",
REDACTED
		"post_text": "",
REDACTED
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
REDACTED
	resp, err := c.doRequest(ctx, "POST", "/project_y/post", opts, bytes.NewReader(body), "application/json", true)
	if err != nil {
		return "", err
REDACTED
	post, _ := resp["post"].(map[string]any)
	if post == nil {
		return "", nil
REDACTED
	return stringFromJSON(post, "id"), nil
REDACTED

// DeletePost deletes a Sora post.
func (c *Client) DeletePost(ctx context.Context, opts RequestOptions, postID string) error {
	if postID == "" {
		return nil
REDACTED
	_, err := c.doRequest(ctx, "DELETE", "/project_y/post/"+postID, opts, nil, "", false)
	return err
REDACTED

func (c *Client) doRequest(ctx context.Context, method, endpoint string, opts RequestOptions, body io.Reader, contentType string, addSentinel bool) (map[string]any, error) {
	resp, err := c.doRequestAny(ctx, method, endpoint, opts, body, contentType, addSentinel)
	if err != nil {
		return nil, err
REDACTED
	parsed, ok := resp.(map[string]any)
	if !ok {
		return nil, errors.New("unexpected response format")
REDACTED
	return parsed, nil
REDACTED

func (c *Client) doRequestAny(ctx context.Context, method, endpoint string, opts RequestOptions, body io.Reader, contentType string, addSentinel bool) (any, error) {
	if c.upstream == nil {
		return nil, errors.New("upstream is nil")
REDACTED
	url := c.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
REDACTED
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
REDACTED
	if opts.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+opts.AccessToken)
REDACTED
	req.Header.Set("User-Agent", defaultMobileUA)
	if addSentinel {
		sentinel, err := c.generateSentinelToken(ctx, opts)
		if err != nil {
			return nil, err
	REDACTED
		req.Header.Set("openai-sentinel-token", sentinel)
REDACTED
	resp, err := c.upstream.DoWithTLS(req, opts.ProxyURL, opts.AccountID, opts.AccountConcurrency, c.enableTLSFingerprint)
	if err != nil {
		return nil, err
REDACTED
	defer resp.Body.Close()

	// 使用 LimitReader 限制最大响应大小，防止 DoS 攻击
	limitedReader := io.LimitReader(resp.Body, maxAPIResponseSize+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
REDACTED

	// 检查是否超过大小限制
	if int64(len(data)) > maxAPIResponseSize {
		return nil, fmt.Errorf("API 响应过大 (最大 %d 字节)", maxAPIResponseSize)
REDACTED

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sora api error: %d %s", resp.StatusCode, strings.TrimSpace(string(data)))
REDACTED
	if len(data) == 0 {
		return map[string]any{REDACTED, nil
REDACTED
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
REDACTED
	return parsed, nil
REDACTED

func (c *Client) generateSentinelToken(ctx context.Context, opts RequestOptions) (string, error) {
	// 尝试从缓存获取
	if token, ok := getCachedSentinel(opts.AccountID); ok {
		return token, nil
REDACTED

	reqID := uuid.New().String()
	powToken, err := generatePowToken(defaultDesktopUA)
	if err != nil {
		return "", err
REDACTED
	payload := map[string]any{"p": powToken, "flow": sentinelFlow, "id": reqIDREDACTED
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
REDACTED
	url := chatGPTBaseURL + "/backend-api/sentinel/req"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
REDACTED
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://sora.chatgpt.com")
	req.Header.Set("Referer", "https://sora.chatgpt.com/")
	req.Header.Set("User-Agent", defaultDesktopUA)
	if opts.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+opts.AccessToken)
REDACTED
	resp, err := c.upstream.DoWithTLS(req, opts.ProxyURL, opts.AccountID, opts.AccountConcurrency, c.enableTLSFingerprint)
	if err != nil {
		return "", err
REDACTED
	defer resp.Body.Close()

	// 使用 LimitReader 限制最大响应大小，防止 DoS 攻击
	limitedReader := io.LimitReader(resp.Body, maxAPIResponseSize+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", err
REDACTED

	// 检查是否超过大小限制
	if int64(len(data)) > maxAPIResponseSize {
		return "", fmt.Errorf("API 响应过大 (最大 %d 字节)", maxAPIResponseSize)
REDACTED

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("sentinel request failed: %d %s", resp.StatusCode, strings.TrimSpace(string(data)))
REDACTED
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
REDACTED
	token := buildSentinelToken(reqID, powToken, parsed)

	// 缓存结果
	cacheSentinel(opts.AccountID, token)

	return token, nil
REDACTED

func buildSentinelToken(reqID, powToken string, resp map[string]any) string {
	finalPow := powToken
	pow, _ := resp["proofofwork"].(map[string]any)
	if pow != nil {
		required, _ := pow["required"].(bool)
		if required {
			seed, _ := pow["seed"].(string)
			difficulty, _ := pow["difficulty"].(string)
			if seed != "" && difficulty != "" {
				candidate, _ := solvePow(seed, difficulty, defaultDesktopUA)
				if candidate != "" {
					finalPow = "gAAAAAB" + candidate
			REDACTED
		REDACTED
	REDACTED
REDACTED
	if !strings.HasSuffix(finalPow, "~S") {
		finalPow += "~S"
REDACTED
	turnstile := ""
	if t, ok := resp["turnstile"].(map[string]any); ok {
		turnstile, _ = t["dx"].(string)
REDACTED
	token := ""
	if v, ok := resp["token"].(string); ok {
		token = v
REDACTED
	payload := map[string]any{
		"p":    finalPow,
		"t":    turnstile,
		"c":    token,
		"id":   reqID,
		"flow": sentinelFlow,
REDACTED
	data, _ := json.Marshal(payload)
	return string(data)
REDACTED

func generatePowToken(userAgent string) (string, error) {
	seed := fmt.Sprintf("%f", float64(time.Now().UnixNano())/1e9)
	candidate, _ := solvePow(seed, "0fffff", userAgent)
	if candidate == "" {
		return "", errors.New("pow generation failed")
REDACTED
	return "gAAAAAC" + candidate, nil
REDACTED

func solvePow(seed, difficulty, userAgent string) (string, bool) {
	config := powConfig(userAgent)
	seedBytes := []byte(seed)
	diffBytes, err := hexDecode(difficulty)
	if err != nil {
		return "", false
REDACTED
	configBytes, err := json.Marshal(config)
	if err != nil {
		return "", false
REDACTED
	prefix := configBytes[:len(configBytes)-1]
	for i := 0; i < 500000; i++ {
		payload := append(prefix, []byte(fmt.Sprintf(",%d,%d]", i, i>>1))...)
		b64 := base64.StdEncoding.EncodeToString(payload)
		h := sha3.Sum512(append(seedBytes, []byte(b64)...))
		if bytes.Compare(h[:len(diffBytes)], diffBytes) <= 0 {
			return b64, true
	REDACTED
REDACTED
	return "", false
REDACTED

func powConfig(userAgent string) []any {
	return []any{
		3000,
		formatPowTime(),
		4294705152,
		0,
		userAgent,
		"",
		nil,
		"en-US",
		"en-US,es-US,en,es",
		0,
		"webdriver-false",
		"location",
		"window",
		time.Now().UnixMilli(),
		uuid.New().String(),
		"",
		16,
		float64(time.Now().UnixMilli()),
REDACTED
REDACTED

func formatPowTime() string {
	loc := time.FixedZone("EST", -5*60*60)
	return time.Now().In(loc).Format("Mon Jan 02 2006 15:04:05") + " GMT-0500 (Eastern Standard Time)"
REDACTED

func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, errors.New("invalid hex length")
REDACTED
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		byteVal, err := hexPair(s[i*2 : i*2+2])
		if err != nil {
			return nil, err
	REDACTED
		out[i] = byteVal
REDACTED
	return out, nil
REDACTED

func hexPair(pair string) (byte, error) {
	var v byte
	for i := 0; i < 2; i++ {
		c := pair[i]
		var n byte
		switch {
		case c >= '0' && c <= '9':
			n = c - '0'
		case c >= 'a' && c <= 'f':
			n = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			n = c - 'A' + 10
		default:
			return 0, errors.New("invalid hex")
	REDACTED
		v = v<<4 | n
REDACTED
	return v, nil
REDACTED

func stringFromJSON(data map[string]any, key string) string {
	if data == nil {
		return ""
REDACTED
	if v, ok := data[key].(string); ok {
		return v
REDACTED
	return ""
REDACTED

func convertList(list []any) []map[string]any {
	results := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			results = append(results, m)
	REDACTED
REDACTED
	return results
REDACTED

func convertListFromAny(data map[string]any) []map[string]any {
	if data == nil {
		return nil
REDACTED
	items, ok := data["items"].([]any)
	if ok {
		return convertList(items)
REDACTED
	return nil
REDACTED
