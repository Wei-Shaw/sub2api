// Package antigravity provides a client for the Antigravity API.
package antigravity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
)

// ForbiddenError 表示上游返回 403 Forbidden
type ForbiddenError struct {
	StatusCode int
	Body       string
REDACTED

func (e *ForbiddenError) Error() string {
	return fmt.Sprintf("fetchAvailableModels 失败 (HTTP %d): %s", e.StatusCode, e.Body)
REDACTED

// NewAPIRequestWithURL 使用指定的 base URL 创建 Antigravity API 请求（v1internal 端点）
func NewAPIRequestWithURL(ctx context.Context, baseURL, action, accessToken string, body []byte) (*http.Request, error) {
	// 构建 URL，流式请求添加 ?alt=sse 参数
	apiURL := fmt.Sprintf("%s/v1internal:%s", baseURL, action)
	isStream := action == "streamGenerateContent"
	if isStream {
		apiURL += "?alt=sse"
REDACTED

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED

	// 基础 Headers（与 Antigravity-Manager 保持一致，只设置这 3 个）
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", GetUserAgentForContext(ctx))

	return req, nil
REDACTED

// NewAPIRequest 使用默认 URL 创建 Antigravity API 请求（v1internal 端点）
// 向后兼容：仅使用默认 BaseURL
func NewAPIRequest(ctx context.Context, action, accessToken string, body []byte) (*http.Request, error) {
	return NewAPIRequestWithURL(ctx, BaseURL, action, accessToken, body)
REDACTED

// TokenResponse Google OAuth token 响应
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
REDACTED

// UserInfo Google 用户信息
type UserInfo struct {
	Email      string `json:"email"`
	Name       string `json:"name,omitempty"`
	GivenName  string `json:"given_name,omitempty"`
	FamilyName string `json:"family_name,omitempty"`
	Picture    string `json:"picture,omitempty"`
REDACTED

// LoadCodeAssistRequest loadCodeAssist 请求
type LoadCodeAssistRequest struct {
	Metadata struct {
		IDEType    string `json:"ideType"`
		IDEVersion string `json:"ideVersion"`
		IDEName    string `json:"ideName"`
REDACTED `json:"metadata"`
REDACTED

// TierInfo 账户类型信息
type TierInfo struct {
	ID          string `json:"id"`          // free-tier, g1-pro-tier, g1-ultra-tier
	Name        string `json:"name"`        // 显示名称
	Description string `json:"description"` // 描述
REDACTED

// UnmarshalJSON supports both legacy string tiers and object tiers.
func (t *TierInfo) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return nil
REDACTED
	if data[0] == '"' {
		var id string
		if err := json.Unmarshal(data, &id); err != nil {
			return err
	REDACTED
		t.ID = id
		return nil
REDACTED
	type alias TierInfo
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
REDACTED
	*t = TierInfo(decoded)
	return nil
REDACTED

// IneligibleTier 不符合条件的层级信息
type IneligibleTier struct {
	Tier *TierInfo `json:"tier,omitempty"`
	// ReasonCode 不符合条件的原因代码，如 INELIGIBLE_ACCOUNT
	ReasonCode    string `json:"reasonCode,omitempty"`
	ReasonMessage string `json:"reasonMessage,omitempty"`
REDACTED

// LoadCodeAssistResponse loadCodeAssist 响应
type LoadCodeAssistResponse struct {
	CloudAICompanionProject string            `json:"cloudaicompanionProject"`
	CurrentTier             *TierInfo         `json:"currentTier,omitempty"`
	PaidTier                *PaidTierInfo     `json:"paidTier,omitempty"`
	IneligibleTiers         []*IneligibleTier `json:"ineligibleTiers,omitempty"`
REDACTED

// PaidTierInfo 付费等级信息，包含 AI Credits 余额。
type PaidTierInfo struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	AvailableCredits []AvailableCredit `json:"availableCredits,omitempty"`
REDACTED

// UnmarshalJSON 兼容 paidTier 既可能是字符串也可能是对象的情况。
func (p *PaidTierInfo) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return nil
REDACTED
	if data[0] == '"' {
		var id string
		if err := json.Unmarshal(data, &id); err != nil {
			return err
	REDACTED
		p.ID = id
		return nil
REDACTED
	type alias PaidTierInfo
	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
REDACTED
	*p = PaidTierInfo(raw)
	return nil
REDACTED

// AvailableCredit 表示一条 AI Credits 余额记录。
type AvailableCredit struct {
	CreditType                  string `json:"creditType,omitempty"`
	CreditAmount                string `json:"creditAmount,omitempty"`
	MinimumCreditAmountForUsage string `json:"minimumCreditAmountForUsage,omitempty"`
REDACTED

// GetAmount 将 creditAmount 解析为浮点数。
func (c *AvailableCredit) GetAmount() float64 {
	if c.CreditAmount == "" {
		return 0
REDACTED
	var value float64
	_, _ = fmt.Sscanf(c.CreditAmount, "%f", &value)
	return value
REDACTED

// GetMinimumAmount 将 minimumCreditAmountForUsage 解析为浮点数。
func (c *AvailableCredit) GetMinimumAmount() float64 {
	if c.MinimumCreditAmountForUsage == "" {
		return 0
REDACTED
	var value float64
	_, _ = fmt.Sscanf(c.MinimumCreditAmountForUsage, "%f", &value)
	return value
REDACTED

// OnboardUserRequest onboardUser 请求
type OnboardUserRequest struct {
	TierID   string `json:"tierId"`
	Metadata struct {
		IDEType    string `json:"ideType"`
		Platform   string `json:"platform,omitempty"`
		PluginType string `json:"pluginType,omitempty"`
REDACTED `json:"metadata"`
REDACTED

// OnboardUserResponse onboardUser 响应
type OnboardUserResponse struct {
	Name     string         `json:"name,omitempty"`
	Done     bool           `json:"done"`
	Response map[string]any `json:"response,omitempty"`
REDACTED

// GetTier 获取账户类型
// 优先返回 paidTier（付费订阅级别），否则返回 currentTier
func (r *LoadCodeAssistResponse) GetTier() string {
	if r.PaidTier != nil && r.PaidTier.ID != "" {
		return r.PaidTier.ID
REDACTED
	if r.CurrentTier != nil {
		return r.CurrentTier.ID
REDACTED
	return ""
REDACTED

// GetAvailableCredits 返回 paid tier 中的 AI Credits 余额列表。
func (r *LoadCodeAssistResponse) GetAvailableCredits() []AvailableCredit {
	if r.PaidTier == nil {
		return nil
REDACTED
	return r.PaidTier.AvailableCredits
REDACTED

// TierIDToPlanType 将 tier ID 映射为用户可见的套餐名。
func TierIDToPlanType(tierID string) string {
	switch strings.ToLower(strings.TrimSpace(tierID)) {
	case "free-tier":
		return "Free"
	case "g1-pro-tier":
		return "Pro"
	case "g1-ultra-tier":
		return "Ultra"
	default:
		if tierID == "" {
			return "Free"
	REDACTED
		return tierID
REDACTED
REDACTED

// Client Antigravity API 客户端
type Client struct {
	httpClient *http.Client
REDACTED

const (
	// proxyDialTimeout 代理 TCP 连接超时（含代理握手），代理不通时快速失败
	proxyDialTimeout = 5 * time.Second
	// proxyTLSHandshakeTimeout 代理 TLS 握手超时
	proxyTLSHandshakeTimeout = 5 * time.Second
	// clientTimeout 整体请求超时（含连接、发送、等待响应、读取 body）
	clientTimeout = 10 * time.Second
	// fetchAvailableModelsBodyLimit limits model-list responses to avoid unbounded memory use.
	fetchAvailableModelsBodyLimit int64 = 8 << 20
)

func NewClient(proxyURL string) (*Client, error) {
	client := &http.Client{
		Timeout: clientTimeout,
REDACTED

	_, parsed, err := proxyurl.Parse(proxyURL)
	if err != nil {
		return nil, err
REDACTED
	if parsed != nil {
		transport := &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: proxyDialTimeout,
		REDACTED).DialContext,
			TLSHandshakeTimeout: proxyTLSHandshakeTimeout,
	REDACTED
		if err := proxyutil.ConfigureTransportProxy(transport, parsed); err != nil {
			return nil, fmt.Errorf("configure proxy: %w", err)
	REDACTED
		client.Transport = transport
REDACTED
	return &Client{
		httpClient: client,
REDACTED, nil
REDACTED

// IsConnectionError 判断是否为连接错误（网络超时、DNS 失败、连接拒绝）
func IsConnectionError(err error) bool {
	if err == nil {
		return false
REDACTED

	// 检查超时错误
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
REDACTED

	// 检查连接错误（DNS 失败、连接拒绝）
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
REDACTED

	// 检查 URL 错误
	var urlErr *url.Error
	return errors.As(err, &urlErr)
REDACTED

// shouldFallbackToNextURL 判断是否应切换到下一个 URL
// 与 Antigravity-Manager 保持一致：连接错误、429、408、404、5xx 触发 URL 降级
func shouldFallbackToNextURL(err error, statusCode int) bool {
	if IsConnectionError(err) {
		return true
REDACTED
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusRequestTimeout ||
		statusCode == http.StatusNotFound ||
		statusCode >= 500
REDACTED

// ExchangeCode 用 authorization code 交换 token
func (c *Client) ExchangeCode(ctx context.Context, code, codeVerifier string) (*TokenResponse, error) {
	clientSecret, err := getClientSecret()
	if err != nil {
		return nil, err
REDACTED

	params := url.Values{REDACTED
	params.Set("client_id", ClientID)
	params.Set("client_secret", clientSecret)
	params.Set("code", code)
	params.Set("redirect_uri", RedirectURI)
	params.Set("grant_type", "authorization_code")
	params.Set("code_verifier", codeVerifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
REDACTED
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := servertiming.Do(c.httpClient, req)
	if err != nil {
		return nil, fmt.Errorf("token 交换请求失败: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
REDACTED

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token 交换失败 (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
REDACTED

	var tokenResp TokenResponse
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return nil, fmt.Errorf("token 解析失败: %w", err)
REDACTED

	return &tokenResp, nil
REDACTED

// RefreshToken 刷新 access_token
func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	clientSecret, err := getClientSecret()
	if err != nil {
		return nil, err
REDACTED

	params := url.Values{REDACTED
	params.Set("client_id", ClientID)
	params.Set("client_secret", clientSecret)
	params.Set("refresh_token", refreshToken)
	params.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
REDACTED
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := servertiming.Do(c.httpClient, req)
	if err != nil {
		return nil, fmt.Errorf("token 刷新请求失败: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
REDACTED

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token 刷新失败 (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
REDACTED

	var tokenResp TokenResponse
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return nil, fmt.Errorf("token 解析失败: %w", err)
REDACTED

	return &tokenResp, nil
REDACTED

// GetUserInfo 获取用户信息
func (c *Client) GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, UserInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
REDACTED
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := servertiming.Do(c.httpClient, req)
	if err != nil {
		return nil, fmt.Errorf("用户信息请求失败: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
REDACTED

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取用户信息失败 (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
REDACTED

	var userInfo UserInfo
	if err := json.Unmarshal(bodyBytes, &userInfo); err != nil {
		return nil, fmt.Errorf("用户信息解析失败: %w", err)
REDACTED

	return &userInfo, nil
REDACTED

// LoadCodeAssist 获取账户信息，返回解析后的结构体和原始 JSON
// 支持 URL fallback：sandbox → daily → prod
func (c *Client) LoadCodeAssist(ctx context.Context, accessToken string) (*LoadCodeAssistResponse, map[string]any, error) {
	reqBody := LoadCodeAssistRequest{REDACTED
	reqBody.Metadata.IDEType = "ANTIGRAVITY"
	reqBody.Metadata.IDEVersion = GetUserAgentVersionForContext(ctx)
	reqBody.Metadata.IDEName = "antigravity"

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("序列化请求失败: %w", err)
REDACTED

	// 固定顺序：prod -> daily
	availableURLs := BaseURLs

	var lastErr error
	for urlIdx, baseURL := range availableURLs {
		apiURL := baseURL + "/v1internal:loadCodeAssist"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(bodyBytes)))
		if err != nil {
			lastErr = fmt.Errorf("创建请求失败: %w", err)
			continue
	REDACTED
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", GetUserAgentForContext(ctx))

		resp, err := servertiming.Do(c.httpClient, req)
		if err != nil {
			lastErr = fmt.Errorf("loadCodeAssist 请求失败: %w", err)
			if shouldFallbackToNextURL(err, 0) && urlIdx < len(availableURLs)-1 {
				log.Printf("[antigravity] loadCodeAssist URL fallback: %s -> %s", baseURL, availableURLs[urlIdx+1])
				continue
		REDACTED
			return nil, nil, lastErr
	REDACTED

		respBodyBytes, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close() // 立即关闭，避免循环内 defer 导致的资源泄漏
		if err != nil {
			return nil, nil, fmt.Errorf("读取响应失败: %w", err)
	REDACTED

		// 检查是否需要 URL 降级
		if shouldFallbackToNextURL(nil, resp.StatusCode) && urlIdx < len(availableURLs)-1 {
			log.Printf("[antigravity] loadCodeAssist URL fallback (HTTP %d): %s -> %s", resp.StatusCode, baseURL, availableURLs[urlIdx+1])
			continue
	REDACTED

		if resp.StatusCode != http.StatusOK {
			return nil, nil, fmt.Errorf("loadCodeAssist 失败 (HTTP %d): %s", resp.StatusCode, string(respBodyBytes))
	REDACTED

		var loadResp LoadCodeAssistResponse
		if err := json.Unmarshal(respBodyBytes, &loadResp); err != nil {
			return nil, nil, fmt.Errorf("响应解析失败: %w", err)
	REDACTED

		// 解析原始 JSON 为 map
		var rawResp map[string]any
		_ = json.Unmarshal(respBodyBytes, &rawResp)

		// 标记成功的 URL，下次优先使用
		DefaultURLAvailability.MarkSuccess(baseURL)
		return &loadResp, rawResp, nil
REDACTED

	return nil, nil, lastErr
REDACTED

// OnboardUser 触发账号 onboarding，并返回 project_id
// 说明：
// 1) 部分账号 loadCodeAssist 不会立即返回 cloudaicompanionProject；
// 2) 这时需要调用 onboardUser 完成初始化，之后才能拿到 project_id。
func (c *Client) OnboardUser(ctx context.Context, accessToken, tierID string) (string, error) {
	tierID = strings.TrimSpace(tierID)
	if tierID == "" {
		return "", fmt.Errorf("tier_id 为空")
REDACTED

	reqBody := OnboardUserRequest{TierID: tierIDREDACTED
	reqBody.Metadata.IDEType = "ANTIGRAVITY"
	reqBody.Metadata.Platform = "PLATFORM_UNSPECIFIED"
	reqBody.Metadata.PluginType = "GEMINI"

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
REDACTED

	availableURLs := BaseURLs
	var lastErr error

	for urlIdx, baseURL := range availableURLs {
		apiURL := baseURL + "/v1internal:onboardUser"

		for attempt := 1; attempt <= 5; attempt++ {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
			if err != nil {
				lastErr = fmt.Errorf("创建请求失败: %w", err)
				break
		REDACTED
			req.Header.Set("Authorization", "Bearer "+accessToken)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", GetUserAgentForContext(ctx))

			resp, err := servertiming.Do(c.httpClient, req)
			if err != nil {
				lastErr = fmt.Errorf("onboardUser 请求失败: %w", err)
				if shouldFallbackToNextURL(err, 0) && urlIdx < len(availableURLs)-1 {
					log.Printf("[antigravity] onboardUser URL fallback: %s -> %s", baseURL, availableURLs[urlIdx+1])
					break
			REDACTED
				return "", lastErr
		REDACTED

			respBodyBytes, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				return "", fmt.Errorf("读取响应失败: %w", err)
		REDACTED

			if shouldFallbackToNextURL(nil, resp.StatusCode) && urlIdx < len(availableURLs)-1 {
				log.Printf("[antigravity] onboardUser URL fallback (HTTP %d): %s -> %s", resp.StatusCode, baseURL, availableURLs[urlIdx+1])
				break
		REDACTED

			if resp.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("onboardUser 失败 (HTTP %d): %s", resp.StatusCode, string(respBodyBytes))
				return "", lastErr
		REDACTED

			var onboardResp OnboardUserResponse
			if err := json.Unmarshal(respBodyBytes, &onboardResp); err != nil {
				lastErr = fmt.Errorf("onboardUser 响应解析失败: %w", err)
				return "", lastErr
		REDACTED

			if onboardResp.Done {
				if projectID := extractProjectIDFromOnboardResponse(onboardResp.Response); projectID != "" {
					DefaultURLAvailability.MarkSuccess(baseURL)
					return projectID, nil
			REDACTED
				lastErr = fmt.Errorf("onboardUser 完成但未返回 project_id")
				return "", lastErr
		REDACTED

			// done=false 时等待后重试（与 CLIProxyAPI 行为一致）
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return "", ctx.Err()
		REDACTED
	REDACTED
REDACTED

	if lastErr != nil {
		return "", lastErr
REDACTED
	return "", fmt.Errorf("onboardUser 未返回 project_id")
REDACTED

func extractProjectIDFromOnboardResponse(resp map[string]any) string {
	if len(resp) == 0 {
		return ""
REDACTED

	if v, ok := resp["cloudaicompanionProject"]; ok {
		switch project := v.(type) {
		case string:
			return strings.TrimSpace(project)
		case map[string]any:
			if id, ok := project["id"].(string); ok {
				return strings.TrimSpace(id)
		REDACTED
	REDACTED
REDACTED

	return ""
REDACTED

// ModelQuotaInfo 模型配额信息
type ModelQuotaInfo struct {
	RemainingFraction float64 `json:"remainingFraction"`
	ResetTime         string  `json:"resetTime,omitempty"`
REDACTED

// ModelInfo 模型信息
type ModelInfo struct {
	QuotaInfo          *ModelQuotaInfo `json:"quotaInfo,omitempty"`
	DisplayName        string          `json:"displayName,omitempty"`
	SupportsImages     *bool           `json:"supportsImages,omitempty"`
	SupportsThinking   *bool           `json:"supportsThinking,omitempty"`
	ThinkingBudget     *int            `json:"thinkingBudget,omitempty"`
	Recommended        *bool           `json:"recommended,omitempty"`
	MaxTokens          *int            `json:"maxTokens,omitempty"`
	MaxOutputTokens    *int            `json:"maxOutputTokens,omitempty"`
	SupportedMimeTypes map[string]bool `json:"supportedMimeTypes,omitempty"`
REDACTED

// DeprecatedModelInfo 废弃模型转发信息
type DeprecatedModelInfo struct {
	NewModelID string `json:"newModelId"`
REDACTED

// FetchAvailableModelsRequest fetchAvailableModels 请求
type FetchAvailableModelsRequest struct {
	Project string `json:"project"`
REDACTED

// FetchAvailableModelsResponse fetchAvailableModels 响应
type FetchAvailableModelsResponse struct {
	Models             map[string]ModelInfo           `json:"models"`
	DeprecatedModelIDs map[string]DeprecatedModelInfo `json:"deprecatedModelIds,omitempty"`
REDACTED

// FetchAvailableModels 获取可用模型和配额信息，返回解析后的结构体和原始 JSON
// 支持 URL fallback：sandbox → daily → prod
func (c *Client) FetchAvailableModels(ctx context.Context, accessToken, projectID string) (*FetchAvailableModelsResponse, map[string]any, error) {
	if c == nil || c.httpClient == nil {
		return nil, nil, errors.New("antigravity client is not configured")
REDACTED

	reqBody := FetchAvailableModelsRequest{Project: projectIDREDACTED
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("序列化请求失败: %w", err)
REDACTED

	// 固定顺序：prod -> daily
	availableURLs := BaseURLs

	fetchClient := c.fetchAvailableModelsHTTPClient()
	var lastErr error
	for urlIdx, baseURL := range availableURLs {
		apiURL := baseURL + "/v1internal:fetchAvailableModels"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(bodyBytes)))
		if err != nil {
			lastErr = fmt.Errorf("创建请求失败: %w", err)
			continue
	REDACTED
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", GetUserAgentForContext(ctx))

		resp, err := servertiming.Do(fetchClient, req)
		if err != nil {
			lastErr = fmt.Errorf("fetchAvailableModels 请求失败: %w", err)
			if shouldFallbackToNextURL(err, 0) && urlIdx < len(availableURLs)-1 {
				log.Printf("[antigravity] fetchAvailableModels URL fallback: %s -> %s", baseURL, availableURLs[urlIdx+1])
				continue
		REDACTED
			return nil, nil, lastErr
	REDACTED

		respBodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, fetchAvailableModelsBodyLimit+1))
		_ = resp.Body.Close() // 立即关闭，避免循环内 defer 导致的资源泄漏
		if err != nil {
			return nil, nil, fmt.Errorf("读取响应失败: %w", err)
	REDACTED
		if int64(len(respBodyBytes)) > fetchAvailableModelsBodyLimit {
			return nil, nil, fmt.Errorf("响应超过 %d 字节", fetchAvailableModelsBodyLimit)
	REDACTED

		// 检查是否需要 URL 降级
		if shouldFallbackToNextURL(nil, resp.StatusCode) && urlIdx < len(availableURLs)-1 {
			log.Printf("[antigravity] fetchAvailableModels URL fallback (HTTP %d): %s -> %s", resp.StatusCode, baseURL, availableURLs[urlIdx+1])
			continue
	REDACTED

		if resp.StatusCode == http.StatusForbidden {
			return nil, nil, &ForbiddenError{
				StatusCode: resp.StatusCode,
				Body:       string(respBodyBytes),
		REDACTED
	REDACTED

		if resp.StatusCode != http.StatusOK {
			return nil, nil, fmt.Errorf("fetchAvailableModels 失败 (HTTP %d): %s", resp.StatusCode, string(respBodyBytes))
	REDACTED

		var modelsResp FetchAvailableModelsResponse
		if err := json.Unmarshal(respBodyBytes, &modelsResp); err != nil {
			return nil, nil, fmt.Errorf("响应解析失败: %w", err)
	REDACTED

		// 解析原始 JSON 为 map
		var rawResp map[string]any
		_ = json.Unmarshal(respBodyBytes, &rawResp)

		// 标记成功的 URL，下次优先使用
		DefaultURLAvailability.MarkSuccess(baseURL)
		return &modelsResp, rawResp, nil
REDACTED

	return nil, nil, lastErr
REDACTED

func (c *Client) fetchAvailableModelsHTTPClient() *http.Client {
	fetchClient := *c.httpClient
	fetchClient.CheckRedirect = checkFetchAvailableModelsRedirect
	return &fetchClient
REDACTED

func checkFetchAvailableModelsRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
REDACTED
	if req == nil || req.URL == nil {
		return errors.New("redirect url is nil")
REDACTED
	if !isAllowedFetchAvailableModelsRedirectHost(req.URL.Hostname()) {
		return fmt.Errorf("redirect to unsupported host: %s", req.URL.Hostname())
REDACTED
	return nil
REDACTED

func isAllowedFetchAvailableModelsRedirectHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
REDACTED
	for _, baseURL := range BaseURLs {
		parsed, err := url.Parse(baseURL)
		if err != nil {
			continue
	REDACTED
		if strings.EqualFold(host, parsed.Hostname()) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// ── Privacy API ──────────────────────────────────────────────────────

// privacyBaseURL 隐私设置 API 仅使用 daily 端点（与 Antigravity 客户端行为一致）
const privacyBaseURL = antigravityDailyBaseURL

// SetUserSettingsRequest setUserSettings 请求体
type SetUserSettingsRequest struct {
	UserSettings map[string]any `json:"user_settings"`
REDACTED

// FetchUserInfoRequest fetchUserInfo 请求体
type FetchUserInfoRequest struct {
	Project string `json:"project"`
REDACTED

// FetchUserInfoResponse fetchUserInfo 响应体
type FetchUserInfoResponse struct {
	UserSettings map[string]any `json:"userSettings,omitempty"`
	RegionCode   string         `json:"regionCode,omitempty"`
REDACTED

// IsPrivate 判断隐私是否已设置：userSettings 为空或不含 telemetryEnabled 表示已设置
func (r *FetchUserInfoResponse) IsPrivate() bool {
	if r == nil || r.UserSettings == nil {
		return true
REDACTED
	_, hasTelemetry := r.UserSettings["telemetryEnabled"]
	return !hasTelemetry
REDACTED

// SetUserSettingsResponse setUserSettings 响应体
type SetUserSettingsResponse struct {
	UserSettings map[string]any `json:"userSettings,omitempty"`
REDACTED

// IsSuccess 判断 setUserSettings 是否成功：返回 {"userSettings":{REDACTEDREDACTED 且无 telemetryEnabled
func (r *SetUserSettingsResponse) IsSuccess() bool {
	if r == nil {
		return false
REDACTED
	// userSettings 为 nil 或空 map 均视为成功
	if len(r.UserSettings) == 0 {
		return true
REDACTED
	// 如果包含 telemetryEnabled 字段，说明未成功清除
	_, hasTelemetry := r.UserSettings["telemetryEnabled"]
	return !hasTelemetry
REDACTED

// SetUserSettings 调用 setUserSettings API 设置用户隐私，返回解析后的响应
func (c *Client) SetUserSettings(ctx context.Context, accessToken string) (*SetUserSettingsResponse, error) {
	// 发送空 user_settings 以清除隐私设置
	payload := SetUserSettingsRequest{UserSettings: map[string]any{REDACTEDREDACTED
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
REDACTED

	apiURL := privacyBaseURL + "/v1internal:setUserSettings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
REDACTED
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", GetUserAgentForContext(ctx))
	req.Header.Set("X-Goog-Api-Client", "gl-node/22.21.1")
	req.Host = "daily-cloudcode-pa.googleapis.com"

	resp, err := servertiming.Do(c.httpClient, req)
	if err != nil {
		return nil, fmt.Errorf("setUserSettings 请求失败: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
REDACTED

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("setUserSettings 失败 (HTTP %d): %s", resp.StatusCode, string(respBody))
REDACTED

	var result SetUserSettingsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("响应解析失败: %w", err)
REDACTED

	return &result, nil
REDACTED

// FetchUserInfo 调用 fetchUserInfo API 获取用户隐私设置状态
func (c *Client) FetchUserInfo(ctx context.Context, accessToken, projectID string) (*FetchUserInfoResponse, error) {
	reqBody := FetchUserInfoRequest{Project: projectIDREDACTED
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
REDACTED

	apiURL := privacyBaseURL + "/v1internal:fetchUserInfo"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
REDACTED
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", GetUserAgentForContext(ctx))
	req.Header.Set("X-Goog-Api-Client", "gl-node/22.21.1")
	req.Host = "daily-cloudcode-pa.googleapis.com"

	resp, err := servertiming.Do(c.httpClient, req)
	if err != nil {
		return nil, fmt.Errorf("fetchUserInfo 请求失败: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
REDACTED

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetchUserInfo 失败 (HTTP %d): %s", resp.StatusCode, string(respBody))
REDACTED

	var result FetchUserInfoResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("响应解析失败: %w", err)
REDACTED

	return &result, nil
REDACTED
