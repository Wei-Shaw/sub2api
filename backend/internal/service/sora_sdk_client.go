package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DouDOU-start/go-sora2api/sora"
	"github.com/Wei-Shaw/sub2api/internal/config"
	openaioauth "github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/tidwall/gjson"
)

// SoraSDKClient 基于 go-sora2api SDK 的 Sora 客户端实现。
// 它实现了 SoraClient 接口，用 SDK 替代原有的自建 HTTP/PoW/TLS 指纹逻辑。
type SoraSDKClient struct {
	cfg             *config.Config
	httpUpstream    HTTPUpstream
	tokenProvider   *OpenAITokenProvider
	accountRepo     AccountRepository
	soraAccountRepo SoraAccountRepository

	// 每个 proxyURL 对应一个 SDK 客户端实例
	sdkClients sync.Map // key: proxyURL (string), value: *sora.Client
REDACTED

// NewSoraSDKClient 创建基于 SDK 的 Sora 客户端
func NewSoraSDKClient(cfg *config.Config, httpUpstream HTTPUpstream, tokenProvider *OpenAITokenProvider) *SoraSDKClient {
	return &SoraSDKClient{
		cfg:           cfg,
		httpUpstream:  httpUpstream,
		tokenProvider: tokenProvider,
REDACTED
REDACTED

// SetAccountRepositories 设置账号和 Sora 扩展仓库（用于 token 持久化）
func (c *SoraSDKClient) SetAccountRepositories(accountRepo AccountRepository, soraAccountRepo SoraAccountRepository) {
	if c == nil {
		return
REDACTED
	c.accountRepo = accountRepo
	c.soraAccountRepo = soraAccountRepo
REDACTED

// Enabled 判断是否启用 Sora
func (c *SoraSDKClient) Enabled() bool {
	if c == nil || c.cfg == nil {
		return false
REDACTED
	return strings.TrimSpace(c.cfg.Sora.Client.BaseURL) != ""
REDACTED

// PreflightCheck 在创建任务前执行账号能力预检。
// 当前仅对视频模型执行预检，用于提前识别额度耗尽或能力缺失。
func (c *SoraSDKClient) PreflightCheck(ctx context.Context, account *Account, requestedModel string, modelCfg SoraModelConfig) error {
	if modelCfg.Type != "video" {
		return nil
REDACTED
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return err
REDACTED
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return err
REDACTED
	balance, err := sdkClient.GetCreditBalance(ctx, token)
	if err != nil {
		return &SoraUpstreamError{
			StatusCode: http.StatusForbidden,
			Message:    "当前账号未开通 Sora2 能力或无可用配额",
	REDACTED
REDACTED
	if balance.RateLimitReached || balance.RemainingCount <= 0 {
		msg := "当前账号 Sora2 可用配额不足"
		if requestedModel != "" {
			msg = fmt.Sprintf("当前账号 %s 可用配额不足", requestedModel)
	REDACTED
		return &SoraUpstreamError{
			StatusCode: http.StatusTooManyRequests,
			Message:    msg,
	REDACTED
REDACTED
	return nil
REDACTED

func (c *SoraSDKClient) UploadImage(ctx context.Context, account *Account, data []byte, filename string) (string, error) {
	if len(data) == 0 {
		return "", errors.New("empty image data")
REDACTED
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return "", err
REDACTED
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return "", err
REDACTED
	if filename == "" {
		filename = "image.png"
REDACTED
	mediaID, err := sdkClient.UploadImage(ctx, token, data, filename)
	if err != nil {
		return "", c.wrapSDKError(err, account)
REDACTED
	return mediaID, nil
REDACTED

func (c *SoraSDKClient) CreateImageTask(ctx context.Context, account *Account, req SoraImageRequest) (string, error) {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return "", err
REDACTED
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return "", err
REDACTED
	sentinel, err := sdkClient.GenerateSentinelToken(ctx, token)
	if err != nil {
		return "", c.wrapSDKError(err, account)
REDACTED
	var taskID string
	if strings.TrimSpace(req.MediaID) != "" {
		taskID, err = sdkClient.CreateImageTaskWithImage(ctx, token, sentinel, req.Prompt, req.Width, req.Height, req.MediaID)
REDACTED else {
		taskID, err = sdkClient.CreateImageTask(ctx, token, sentinel, req.Prompt, req.Width, req.Height)
REDACTED
	if err != nil {
		return "", c.wrapSDKError(err, account)
REDACTED
	return taskID, nil
REDACTED

func (c *SoraSDKClient) CreateVideoTask(ctx context.Context, account *Account, req SoraVideoRequest) (string, error) {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return "", err
REDACTED
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return "", err
REDACTED
	sentinel, err := sdkClient.GenerateSentinelToken(ctx, token)
	if err != nil {
		return "", c.wrapSDKError(err, account)
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

	// Remix 模式
	if strings.TrimSpace(req.RemixTargetID) != "" {
		styleID := "" // SDK ExtractStyle 可从 prompt 中提取
		taskID, err := sdkClient.RemixVideo(ctx, token, sentinel, req.RemixTargetID, req.Prompt, orientation, nFrames, styleID)
		if err != nil {
			return "", c.wrapSDKError(err, account)
	REDACTED
		return taskID, nil
REDACTED

	// 普通视频（文生视频或图生视频）
	taskID, err := sdkClient.CreateVideoTaskWithOptions(ctx, token, sentinel, req.Prompt, orientation, nFrames, model, size, req.MediaID, "")
	if err != nil {
		return "", c.wrapSDKError(err, account)
REDACTED
	return taskID, nil
REDACTED

func (c *SoraSDKClient) CreateStoryboardTask(ctx context.Context, account *Account, req SoraStoryboardRequest) (string, error) {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return "", err
REDACTED
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return "", err
REDACTED
	sentinel, err := sdkClient.GenerateSentinelToken(ctx, token)
	if err != nil {
		return "", c.wrapSDKError(err, account)
REDACTED

	orientation := req.Orientation
	if orientation == "" {
		orientation = "landscape"
REDACTED
	nFrames := req.Frames
	if nFrames <= 0 {
		nFrames = 450
REDACTED

	taskID, err := sdkClient.CreateStoryboardTask(ctx, token, sentinel, req.Prompt, orientation, nFrames, req.MediaID, "")
	if err != nil {
		return "", c.wrapSDKError(err, account)
REDACTED
	return taskID, nil
REDACTED

func (c *SoraSDKClient) UploadCharacterVideo(ctx context.Context, account *Account, data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("empty video data")
REDACTED
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return "", err
REDACTED
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return "", err
REDACTED
	cameoID, err := sdkClient.UploadCharacterVideo(ctx, token, data)
	if err != nil {
		return "", c.wrapSDKError(err, account)
REDACTED
	return cameoID, nil
REDACTED

func (c *SoraSDKClient) GetCameoStatus(ctx context.Context, account *Account, cameoID string) (*SoraCameoStatus, error) {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return nil, err
REDACTED
	status, err := sdkClient.GetCameoStatus(ctx, token, cameoID)
	if err != nil {
		return nil, c.wrapSDKError(err, account)
REDACTED
	return &SoraCameoStatus{
		Status:          status.Status,
		DisplayNameHint: status.DisplayNameHint,
		UsernameHint:    status.UsernameHint,
		ProfileAssetURL: status.ProfileAssetURL,
REDACTED, nil
REDACTED

func (c *SoraSDKClient) DownloadCharacterImage(ctx context.Context, account *Account, imageURL string) ([]byte, error) {
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return nil, err
REDACTED
	data, err := sdkClient.DownloadCharacterImage(ctx, imageURL)
	if err != nil {
		return nil, c.wrapSDKError(err, account)
REDACTED
	return data, nil
REDACTED

func (c *SoraSDKClient) UploadCharacterImage(ctx context.Context, account *Account, data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("empty character image")
REDACTED
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return "", err
REDACTED
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return "", err
REDACTED
	assetPointer, err := sdkClient.UploadCharacterImage(ctx, token, data)
	if err != nil {
		return "", c.wrapSDKError(err, account)
REDACTED
	return assetPointer, nil
REDACTED

func (c *SoraSDKClient) FinalizeCharacter(ctx context.Context, account *Account, req SoraCharacterFinalizeRequest) (string, error) {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return "", err
REDACTED
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return "", err
REDACTED
	characterID, err := sdkClient.FinalizeCharacter(ctx, token, req.CameoID, req.Username, req.DisplayName, req.ProfileAssetPointer)
	if err != nil {
		return "", c.wrapSDKError(err, account)
REDACTED
	return characterID, nil
REDACTED

func (c *SoraSDKClient) SetCharacterPublic(ctx context.Context, account *Account, cameoID string) error {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return err
REDACTED
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return err
REDACTED
	if err := sdkClient.SetCharacterPublic(ctx, token, cameoID); err != nil {
		return c.wrapSDKError(err, account)
REDACTED
	return nil
REDACTED

func (c *SoraSDKClient) DeleteCharacter(ctx context.Context, account *Account, characterID string) error {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return err
REDACTED
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return err
REDACTED
	if err := sdkClient.DeleteCharacter(ctx, token, characterID); err != nil {
		return c.wrapSDKError(err, account)
REDACTED
	return nil
REDACTED

func (c *SoraSDKClient) PostVideoForWatermarkFree(ctx context.Context, account *Account, generationID string) (string, error) {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return "", err
REDACTED
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return "", err
REDACTED
	sentinel, err := sdkClient.GenerateSentinelToken(ctx, token)
	if err != nil {
		return "", c.wrapSDKError(err, account)
REDACTED
	postID, err := sdkClient.PublishVideo(ctx, token, sentinel, generationID)
	if err != nil {
		return "", c.wrapSDKError(err, account)
REDACTED
	return postID, nil
REDACTED

func (c *SoraSDKClient) DeletePost(ctx context.Context, account *Account, postID string) error {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return err
REDACTED
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return err
REDACTED
	if err := sdkClient.DeletePost(ctx, token, postID); err != nil {
		return c.wrapSDKError(err, account)
REDACTED
	return nil
REDACTED

// GetWatermarkFreeURLCustom 使用自定义第三方解析服务获取去水印链接。
// SDK 不涉及此功能，保留自建实现。
func (c *SoraSDKClient) GetWatermarkFreeURLCustom(ctx context.Context, account *Account, parseURL, parseToken, postID string) (string, error) {
	parseURL = strings.TrimRight(strings.TrimSpace(parseURL), "/")
	if parseURL == "" {
		return "", errors.New("custom parse url is required")
REDACTED
	if strings.TrimSpace(parseToken) == "" {
		return "", errors.New("custom parse token is required")
REDACTED
	shareURL := "https://sora.chatgpt.com/p/" + strings.TrimSpace(postID)
	payload := map[string]any{
		"url":   shareURL,
		"token": strings.TrimSpace(parseToken),
REDACTED
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
REDACTED
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, parseURL+"/get-sora-link", bytes.NewReader(body))
	if err != nil {
		return "", err
REDACTED
	req.Header.Set("Content-Type", "application/json")

	proxyURL := c.resolveProxyURL(account)
	accountID := int64(0)
	accountConcurrency := 0
	if account != nil {
		accountID = account.ID
		accountConcurrency = account.Concurrency
REDACTED
	var resp *http.Response
	if c.httpUpstream != nil {
		resp, err = c.httpUpstream.Do(req, proxyURL, accountID, accountConcurrency)
REDACTED else {
		resp, err = http.DefaultClient.Do(req)
REDACTED
	if err != nil {
		return "", err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
REDACTED
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("custom parse failed: %d %s", resp.StatusCode, truncateForLog(raw, 256))
REDACTED
	downloadLink := strings.TrimSpace(gjson.GetBytes(raw, "download_link").String())
	if downloadLink == "" {
		return "", errors.New("custom parse response missing download_link")
REDACTED
	return downloadLink, nil
REDACTED

func (c *SoraSDKClient) EnhancePrompt(ctx context.Context, account *Account, prompt, expansionLevel string, durationS int) (string, error) {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return "", err
REDACTED
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return "", err
REDACTED
	if strings.TrimSpace(expansionLevel) == "" {
		expansionLevel = "medium"
REDACTED
	if durationS <= 0 {
		durationS = 10
REDACTED
	enhanced, err := sdkClient.EnhancePrompt(ctx, token, prompt, expansionLevel, durationS)
	if err != nil {
		return "", c.wrapSDKError(err, account)
REDACTED
	return enhanced, nil
REDACTED

func (c *SoraSDKClient) GetImageTask(ctx context.Context, account *Account, taskID string) (*SoraImageTaskStatus, error) {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return nil, err
REDACTED
	result := sdkClient.QueryImageTaskOnce(ctx, token, taskID, time.Now().Add(-10*time.Second))
	if result.Err != nil {
		return &SoraImageTaskStatus{
			ID:       taskID,
			Status:   "failed",
			ErrorMsg: result.Err.Error(),
	REDACTED, nil
REDACTED
	if result.Done && result.ImageURL != "" {
		return &SoraImageTaskStatus{
			ID:     taskID,
			Status: "succeeded",
			URLs:   []string{result.ImageURLREDACTED,
	REDACTED, nil
REDACTED
	status := result.Progress.Status
	if status == "" {
		status = "processing"
REDACTED
	return &SoraImageTaskStatus{
		ID:          taskID,
		Status:      status,
		ProgressPct: float64(result.Progress.Percent) / 100.0,
REDACTED, nil
REDACTED

func (c *SoraSDKClient) GetVideoTask(ctx context.Context, account *Account, taskID string) (*SoraVideoTaskStatus, error) {
	token, err := c.getAccessToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED
	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return nil, err
REDACTED

	// 先查询 pending 列表
	result := sdkClient.QueryVideoTaskOnce(ctx, token, taskID, time.Now().Add(-10*time.Second), 0)
	if result.Err != nil {
		return &SoraVideoTaskStatus{
			ID:       taskID,
			Status:   "failed",
			ErrorMsg: result.Err.Error(),
	REDACTED, nil
REDACTED
	if !result.Done {
		return &SoraVideoTaskStatus{
			ID:          taskID,
			Status:      result.Progress.Status,
			ProgressPct: result.Progress.Percent,
	REDACTED, nil
REDACTED

	// 任务不在 pending 中，查询 drafts 获取下载链接
	downloadURL, err := sdkClient.GetDownloadURL(ctx, token, taskID)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "内容违规") || strings.Contains(errMsg, "Content violates") {
			return &SoraVideoTaskStatus{
				ID:       taskID,
				Status:   "failed",
				ErrorMsg: errMsg,
		REDACTED, nil
	REDACTED
		// 可能还在处理中
		return &SoraVideoTaskStatus{
			ID:     taskID,
			Status: "processing",
	REDACTED, nil
REDACTED
	return &SoraVideoTaskStatus{
		ID:     taskID,
		Status: "completed",
		URLs:   []string{downloadURLREDACTED,
REDACTED, nil
REDACTED

// --- 内部方法 ---

// getSDKClient 获取或创建指定代理的 SDK 客户端实例
func (c *SoraSDKClient) getSDKClient(account *Account) (*sora.Client, error) {
	proxyURL := c.resolveProxyURL(account)
	if v, ok := c.sdkClients.Load(proxyURL); ok {
		if cli, ok2 := v.(*sora.Client); ok2 {
			return cli, nil
	REDACTED
REDACTED
	client, err := sora.New(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("创建 Sora SDK 客户端失败: %w", err)
REDACTED
	actual, _ := c.sdkClients.LoadOrStore(proxyURL, client)
	if cli, ok := actual.(*sora.Client); ok {
		return cli, nil
REDACTED
	return client, nil
REDACTED

func (c *SoraSDKClient) resolveProxyURL(account *Account) string {
	if account == nil || account.ProxyID == nil || account.Proxy == nil {
		return ""
REDACTED
	return strings.TrimSpace(account.Proxy.URL())
REDACTED

// getAccessToken 获取账号的 access_token，支持多种 token 来源和自动刷新。
// 此方法保留了原 SoraDirectClient 的 token 管理逻辑。
func (c *SoraSDKClient) getAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
REDACTED

	// 优先尝试 OpenAI Token Provider
	allowProvider := c.allowOpenAITokenProvider(account)
	var providerErr error
	if allowProvider && c.tokenProvider != nil {
		token, err := c.tokenProvider.GetAccessToken(ctx, account)
		if err == nil && strings.TrimSpace(token) != "" {
			c.debugLogf("token_selected account_id=%d source=openai_token_provider", account.ID)
			return token, nil
	REDACTED
		providerErr = err
		if err != nil && c.debugEnabled() {
			c.debugLogf("token_provider_failed account_id=%d err=%s", account.ID, logredact.RedactText(err.Error()))
	REDACTED
REDACTED

	// 尝试直接使用 credentials 中的 access_token
	token := strings.TrimSpace(account.GetCredential("access_token"))
	if token != "" {
		expiresAt := account.GetCredentialAsTime("expires_at")
		if expiresAt != nil && time.Until(*expiresAt) <= 2*time.Minute {
			refreshed, refreshErr := c.recoverAccessToken(ctx, account, "access_token_expiring")
			if refreshErr == nil && strings.TrimSpace(refreshed) != "" {
				return refreshed, nil
		REDACTED
	REDACTED
		return token, nil
REDACTED

	// 尝试通过 session_token 或 refresh_token 恢复
	recovered, recoverErr := c.recoverAccessToken(ctx, account, "access_token_missing")
	if recoverErr == nil && strings.TrimSpace(recovered) != "" {
		return recovered, nil
REDACTED
	if providerErr != nil {
		return "", providerErr
REDACTED
	return "", errors.New("access_token not found")
REDACTED

// recoverAccessToken 通过 session_token 或 refresh_token 恢复 access_token
func (c *SoraSDKClient) recoverAccessToken(ctx context.Context, account *Account, reason string) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
REDACTED

	// 先尝试 session_token
	if sessionToken := strings.TrimSpace(account.GetCredential("session_token")); sessionToken != "" {
		accessToken, expiresAt, err := c.exchangeSessionToken(ctx, account, sessionToken)
		if err == nil && strings.TrimSpace(accessToken) != "" {
			c.applyRecoveredToken(ctx, account, accessToken, "", expiresAt, sessionToken)
			return accessToken, nil
	REDACTED
REDACTED

	// 再尝试 refresh_token
	refreshToken := strings.TrimSpace(account.GetCredential("refresh_token"))
	if refreshToken == "" {
		return "", errors.New("session_token/refresh_token not found")
REDACTED

	sdkClient, err := c.getSDKClient(account)
	if err != nil {
		return "", err
REDACTED

	// 尝试多个 client_id
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

		newAccess, newRefresh, refreshErr := sdkClient.RefreshAccessToken(ctx, refreshToken, clientID)
		if refreshErr != nil {
			lastErr = refreshErr
			continue
	REDACTED
		if strings.TrimSpace(newAccess) == "" {
			lastErr = errors.New("refreshed access_token is empty")
			continue
	REDACTED
		c.applyRecoveredToken(ctx, account, newAccess, newRefresh, "", "")
		return newAccess, nil
REDACTED

	if lastErr != nil {
		return "", lastErr
REDACTED
	return "", errors.New("no available client_id for refresh_token exchange")
REDACTED

// exchangeSessionToken 通过 session_token 换取 access_token
func (c *SoraSDKClient) exchangeSessionToken(ctx context.Context, account *Account, sessionToken string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://sora.chatgpt.com/api/auth/session", nil)
	if err != nil {
		return "", "", err
REDACTED
	req.Header.Set("Cookie", "__Secure-next-auth.session-token="+sessionToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://sora.chatgpt.com")
	req.Header.Set("Referer", "https://sora.chatgpt.com/")
	req.Header.Set("User-Agent", "Sora/1.2026.007 (Android 15; 24122RKC7C; build 2600700)")

	proxyURL := c.resolveProxyURL(account)
	accountID := int64(0)
	accountConcurrency := 0
	if account != nil {
		accountID = account.ID
		accountConcurrency = account.Concurrency
REDACTED

	var resp *http.Response
	if c.httpUpstream != nil {
		resp, err = c.httpUpstream.Do(req, proxyURL, accountID, accountConcurrency)
REDACTED else {
		resp, err = http.DefaultClient.Do(req)
REDACTED
	if err != nil {
		return "", "", err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", "", err
REDACTED
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("session exchange failed: %d", resp.StatusCode)
REDACTED

	accessToken := strings.TrimSpace(gjson.GetBytes(body, "accessToken").String())
	if accessToken == "" {
		return "", "", errors.New("session exchange missing accessToken")
REDACTED
	expiresAt := strings.TrimSpace(gjson.GetBytes(body, "expires").String())
	return accessToken, expiresAt, nil
REDACTED

// applyRecoveredToken 将恢复的 token 写入账号内存和数据库
func (c *SoraSDKClient) applyRecoveredToken(ctx context.Context, account *Account, accessToken, refreshToken, expiresAt, sessionToken string) {
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
		if err := c.accountRepo.Update(ctx, account); err != nil && c.debugEnabled() {
			c.debugLogf("persist_recovered_token_failed account_id=%d err=%s", account.ID, logredact.RedactText(err.Error()))
	REDACTED
REDACTED
	c.updateSoraAccountExtension(ctx, account, accessToken, refreshToken, sessionToken)
REDACTED

func (c *SoraSDKClient) updateSoraAccountExtension(ctx context.Context, account *Account, accessToken, refreshToken, sessionToken string) {
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

func (c *SoraSDKClient) allowOpenAITokenProvider(account *Account) bool {
	if c == nil || c.tokenProvider == nil {
		return false
REDACTED
	if account != nil && account.Platform == PlatformSora {
		return c.cfg != nil && c.cfg.Sora.Client.UseOpenAITokenProvider
REDACTED
	return true
REDACTED

// wrapSDKError 将 SDK 错误包装为 SoraUpstreamError
func (c *SoraSDKClient) wrapSDKError(err error, account *Account) error {
	if err == nil {
		return nil
REDACTED
	msg := err.Error()
	statusCode := http.StatusBadGateway
	if strings.Contains(msg, "HTTP 401") || strings.Contains(msg, "HTTP 403") {
		statusCode = http.StatusUnauthorized
REDACTED else if strings.Contains(msg, "HTTP 429") {
		statusCode = http.StatusTooManyRequests
REDACTED else if strings.Contains(msg, "HTTP 404") {
		statusCode = http.StatusNotFound
REDACTED
	return &SoraUpstreamError{
		StatusCode: statusCode,
		Message:    msg,
REDACTED
REDACTED

func (c *SoraSDKClient) debugEnabled() bool {
	return c != nil && c.cfg != nil && c.cfg.Sora.Client.Debug
REDACTED

func (c *SoraSDKClient) debugLogf(format string, args ...any) {
	if c.debugEnabled() {
		log.Printf("[SoraSDK] "+format, args...)
REDACTED
REDACTED
