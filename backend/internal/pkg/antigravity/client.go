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
)

// ForbiddenError 表示上游返回 403 Forbidden
type ForbiddenError struct {
	StatusCode int
	Body       string
}

func (e *ForbiddenError) Error() string {
	return fmt.Sprintf("fetchAvailableModels 失败 (HTTP %d): %s", e.StatusCode, e.Body)
}

// NewAPIRequestWithURL 使用指定的 base URL 创建 Antigravity API 请求（v1internal 端点）
func NewAPIRequestWithURL(ctx context.Context, baseURL, action, accessToken string, body []byte) (*http.Request, error) {
	// 构建 URL，流式请求添加 ?alt=sse 参数
	apiURL := fmt.Sprintf("%s/v1internal:%s", baseURL, action)
	isStream := action == "streamGenerateContent"
	if isStream {
		apiURL += "?alt=sse"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// 基础 Headers（与 Antigravity-Manager 保持一致，只设置这 3 个）
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", GetUserAgent())

	return req, nil
}

// NewAPIRequest 使用默认 URL 创建 Antigravity API 请求（v1internal 端点）
// 向后兼容：仅使用默认 BaseURL
func NewAPIRequest(ctx context.Context, action, accessToken string, body []byte) (*http.Request, error) {
	return NewAPIRequestWithURL(ctx, BaseURL, action, accessToken, body)
}

// TokenResponse Google OAuth token 响应
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// UserInfo Google 用户信息
type UserInfo struct {
	Email      string `json:"email"`
	Name       string `json:"name,omitempty"`
	GivenName  string `json:"given_name,omitempty"`
	FamilyName string `json:"family_name,omitempty"`
	Picture    string `json:"picture,omitempty"`
}

// LoadCodeAssistRequest loadCodeAssist 请求
type LoadCodeAssistRequest struct {
	Metadata struct {
		IDEType string `json:"ideType"`
	} `json:"metadata"`
}

// TierInfo 账户类型信息
type TierInfo struct {
	ID          string `json:"id"`          // free-tier, g1-pro-tier, g1-ultra-tier
	Name        string `json:"name"`        // 显示名称
	Description string `json:"description"` // 描述
}

// UnmarshalJSON supports both legacy string tiers and object tiers.
func (t *TierInfo) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '"' {
		var id string
		if err := json.Unmarshal(data, &id); err != nil {
			return err
		}
		t.ID = id
		return nil
	}
	type alias TierInfo
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*t = TierInfo(decoded)
	return nil
}

// IneligibleTier 不符合条件的层级信息
type IneligibleTier struct {
	Tier *TierInfo `json:"tier,omitempty"`
	// ReasonCode 不符合条件的原因代码，如 INELIGIBLE_ACCOUNT
	ReasonCode    string `json:"reasonCode,omitempty"`
	ReasonMessage string `json:"reasonMessage,omitempty"`
}

// LoadCodeAssistResponse loadCodeAssist 响应
type LoadCodeAssistResponse struct {
	CloudAICompanionProject string            `json:"cloudaicompanionProject"`
	CurrentTier             *TierInfo         `json:"currentTier,omitempty"`
	PaidTier                *PaidTierInfo     `json:"paidTier,omitempty"`
	IneligibleTiers         []*IneligibleTier `json:"ineligibleTiers,omitempty"`
}

// PaidTierInfo 付费等级信息，包含 AI Credits 余额。
type PaidTierInfo struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	AvailableCredits []AvailableCredit `json:"availableCredits,omitempty"`
}

// UnmarshalJSON 兼容 paidTier 既可能是字符串也可能是对象的情况。
func (p *PaidTierInfo) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '"' {
		var id string
		if err := json.Unmarshal(data, &id); err != nil {
			return err
		}
		p.ID = id
		return nil
	}
	type alias PaidTierInfo
	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*p = PaidTierInfo(raw)
	return nil
}

// AvailableCredit 表示一条 AI Credits 余额记录。
type AvailableCredit struct {
	CreditType                  string `json:"creditType,omitempty"`
	CreditAmount                string `json:"creditAmount,omitempty"`
	MinimumCreditAmountForUsage string `json:"minimumCreditAmountForUsage,omitempty"`
}

// GetAmount 将 creditAmount 解析为浮点数。
func (c *AvailableCredit) GetAmount() float64 {
	if c.CreditAmount == "" {
		return 0
	}
	var value float64
	_, _ = fmt.Sscanf(c.CreditAmount, "%f", &value)
	return value
}

// GetMinimumAmount 将 minimumCreditAmountForUsage 解析为浮点数。
func (c *AvailableCredit) GetMinimumAmount() float64 {
	if c.MinimumCreditAmountForUsage == "" {
		return 0
	}
	var value float64
	_, _ = fmt.Sscanf(c.MinimumCreditAmountForUsage, "%f", &value)
	return value
}

// OnboardUserRequest onboardUser 请求
type OnboardUserRequest struct {
	TierID   string `json:"tierId"`
	Metadata struct {
		IDEType    string `json:"ideType"`
		Platform   string `json:"platform,omitempty"`
		PluginType string `json:"pluginType,omitempty"`
	} `json:"metadata"`
}

// OnboardUserResponse onboardUser 响应
type OnboardUserResponse struct {
	Name     string         `json:"name,omitempty"`
	Done     bool           `json:"done"`
	Response map[string]any `json:"response,omitempty"`
}

// GetTier 获取账户类型
// 优先返回 paidTier（付费订阅级别），否则返回 currentTier
func (r *LoadCodeAssistResponse) GetTier() string {
	if r.PaidTier != nil && r.PaidTier.ID != "" {
		return r.PaidTier.ID
	}
	if r.CurrentTier != nil {
		return r.CurrentTier.ID
	}
	return ""
}

// GetAvailableCredits 返回 paid tier 中的 AI Credits 余额列表。
func (r *LoadCodeAssistResponse) GetAvailableCredits() []AvailableCredit {
	if r.PaidTier == nil {
		return nil
	}
	return r.PaidTier.AvailableCredits
}

// Client Antigravity API 客户端
type Client struct {
	httpClient *http.Client
}

func NewClient(proxyURL string) (*Client, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	_, parsed, err := proxyurl.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	if parsed != nil {
		transport := &http.Transport{}
		if err := proxyutil.ConfigureTransportProxy(transport, parsed); err != nil {
			return nil, fmt.Errorf("configure proxy: %w", err)
		}
		client.Transport = transport
	}

	return &Client{
		httpClient: client,
	}, nil
}

// isConnectionError 判断是否为连接错误（网络超时、DNS 失败、连接拒绝）
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	// 检查超时错误
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// 检查连接错误（DNS 失败、连接拒绝）
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	// 检查 URL 错误
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}

// shouldFallbackToNextURL 判断是否应切换到下一个 URL
// 与 Antigravity-Manager 保持一致：连接错误、429、408、404、5xx 触发 URL 降级
func shouldFallbackToNextURL(err error, statusCode int) bool {
	if isConnectionError(err) {
		return true
	}
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusRequestTimeout ||
		statusCode == http.StatusNotFound ||
		statusCode >= 500
}

// ExchangeCode 用 authorization code 交换 token
func (c *Client) ExchangeCode(ctx context.Context, code, codeVerifier string) (*TokenResponse, error) {
	clientSecret, err := getClientSecret()
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Set("client_id", ClientID)
	params.Set("client_secret", clientSecret)
	params.Set("code", code)
	params.Set("redirect_uri", RedirectURI)
	params.Set("grant_type", "authorization_code")
	params.Set("code_verifier", codeVerifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token 交换请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token 交换失败 (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return nil, fmt.Errorf("token 解析失败: %w", err)
	}

	return &tokenResp, nil
}

// RefreshToken 刷新 access_token
func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	clientSecret, err := getClientSecret()
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Set("client_id", ClientID)
	params.Set("client_secret", clientSecret)
	params.Set("refresh_token", refreshToken)
	params.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token 刷新请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token 刷新失败 (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return nil, fmt.Errorf("token 解析失败: %w", err)
	}

	return &tokenResp, nil
}

// GetUserInfo 获取用户信息
func (c *Client) GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, UserInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("用户信息请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取用户信息失败 (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var userInfo UserInfo
	if err := json.Unmarshal(bodyBytes, &userInfo); err != nil {
		return nil, fmt.Errorf("用户信息解析失败: %w", err)
	}

	return &userInfo, nil
}

// LoadCodeAssist 获取账户信息，返回解析后的结构体和原始 JSON
// 支持 URL fallback：sandbox → daily → prod
func (c *Client) LoadCodeAssist(ctx context.Context, accessToken string) (*LoadCodeAssistResponse, map[string]any, error) {
	reqBody := LoadCodeAssistRequest{}
	reqBody.Metadata.IDEType = "ANTIGRAVITY"

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	// 固定顺序：prod -> daily
	availableURLs := BaseURLs

	var lastErr error
	for urlIdx, baseURL := range availableURLs {
		apiURL := baseURL + "/v1internal:loadCodeAssist"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(bodyBytes)))
		if err != nil {
			lastErr = fmt.Errorf("创建请求失败: %w", err)
			continue
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", GetUserAgent())

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("loadCodeAssist 请求失败: %w", err)
			if shouldFallbackToNextURL(err, 0) && urlIdx < len(availableURLs)-1 {
				log.Printf("[antigravity] loadCodeAssist URL fallback: %s -> %s", baseURL, availableURLs[urlIdx+1])
				continue
			}
			return nil, nil, lastErr
		}

		respBodyBytes, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close() // 立即关闭，避免循环内 defer 导致的资源泄漏
		if err != nil {
			return nil, nil, fmt.Errorf("读取响应失败: %w", err)
		}

		// 检查是否需要 URL 降级
		if shouldFallbackToNextURL(nil, resp.StatusCode) && urlIdx < len(availableURLs)-1 {
			log.Printf("[antigravity] loadCodeAssist URL fallback (HTTP %d): %s -> %s", resp.StatusCode, baseURL, availableURLs[urlIdx+1])
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, nil, fmt.Errorf("loadCodeAssist 失败 (HTTP %d): %s", resp.StatusCode, string(respBodyBytes))
		}

		var loadResp LoadCodeAssistResponse
		if err := json.Unmarshal(respBodyBytes, &loadResp); err != nil {
			return nil, nil, fmt.Errorf("响应解析失败: %w", err)
		}

		// 解析原始 JSON 为 map
		var rawResp map[string]any
		_ = json.Unmarshal(respBodyBytes, &rawResp)

		// 标记成功的 URL，下次优先使用
		DefaultURLAvailability.MarkSuccess(baseURL)
		return &loadResp, rawResp, nil
	}

	return nil, nil, lastErr
}

// OnboardUser 触发账号 onboarding，并返回 project_id
// 说明：
// 1) 部分账号 loadCodeAssist 不会立即返回 cloudaicompanionProject；
// 2) 这时需要调用 onboardUser 完成初始化，之后才能拿到 project_id。
func (c *Client) OnboardUser(ctx context.Context, accessToken, tierID string) (string, error) {
	tierID = strings.TrimSpace(tierID)
	if tierID == "" {
		return "", fmt.Errorf("tier_id 为空")
	}

	reqBody := OnboardUserRequest{TierID: tierID}
	reqBody.Metadata.IDEType = "ANTIGRAVITY"
	reqBody.Metadata.Platform = "PLATFORM_UNSPECIFIED"
	reqBody.Metadata.PluginType = "GEMINI"

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	availableURLs := BaseURLs
	var lastErr error

	for urlIdx, baseURL := range availableURLs {
		apiURL := baseURL + "/v1internal:onboardUser"

		for attempt := 1; attempt <= 5; attempt++ {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
			if err != nil {
				lastErr = fmt.Errorf("创建请求失败: %w", err)
				break
			}
			req.Header.Set("Authorization", "Bearer "+accessToken)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", GetUserAgent())

			resp, err := c.httpClient.Do(req)
			if err != nil {
				lastErr = fmt.Errorf("onboardUser 请求失败: %w", err)
				if shouldFallbackToNextURL(err, 0) && urlIdx < len(availableURLs)-1 {
					log.Printf("[antigravity] onboardUser URL fallback: %s -> %s", baseURL, availableURLs[urlIdx+1])
					break
				}
				return "", lastErr
			}

			respBodyBytes, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				return "", fmt.Errorf("读取响应失败: %w", err)
			}

			if shouldFallbackToNextURL(nil, resp.StatusCode) && urlIdx < len(availableURLs)-1 {
				log.Printf("[antigravity] onboardUser URL fallback (HTTP %d): %s -> %s", resp.StatusCode, baseURL, availableURLs[urlIdx+1])
				break
			}

			if resp.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("onboardUser 失败 (HTTP %d): %s", resp.StatusCode, string(respBodyBytes))
				return "", lastErr
			}

			var onboardResp OnboardUserResponse
			if err := json.Unmarshal(respBodyBytes, &onboardResp); err != nil {
				lastErr = fmt.Errorf("onboardUser 响应解析失败: %w", err)
				return "", lastErr
			}

			if onboardResp.Done {
				if projectID := extractProjectIDFromOnboardResponse(onboardResp.Response); projectID != "" {
					DefaultURLAvailability.MarkSuccess(baseURL)
					return projectID, nil
				}
				lastErr = fmt.Errorf("onboardUser 完成但未返回 project_id")
				return "", lastErr
			}

			// done=false 时等待后重试（与 CLIProxyAPI 行为一致）
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("onboardUser 未返回 project_id")
}

func extractProjectIDFromOnboardResponse(resp map[string]any) string {
	if len(resp) == 0 {
		return ""
	}

	if v, ok := resp["cloudaicompanionProject"]; ok {
		switch project := v.(type) {
		case string:
			return strings.TrimSpace(project)
		case map[string]any:
			if id, ok := project["id"].(string); ok {
				return strings.TrimSpace(id)
			}
		}
	}

	return ""
}

// ModelQuotaInfo 模型配额信息
type ModelQuotaInfo struct {
	RemainingFraction float64 `json:"remainingFraction"`
	ResetTime         string  `json:"resetTime,omitempty"`
}

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
}

// DeprecatedModelInfo 废弃模型转发信息
type DeprecatedModelInfo struct {
	NewModelID string `json:"newModelId"`
}

// FetchAvailableModelsRequest fetchAvailableModels 请求
type FetchAvailableModelsRequest struct {
	Project string `json:"project"`
}

// FetchAvailableModelsResponse fetchAvailableModels 响应
type FetchAvailableModelsResponse struct {
	Models             map[string]ModelInfo           `json:"models"`
	DeprecatedModelIDs map[string]DeprecatedModelInfo `json:"deprecatedModelIds,omitempty"`
}

// FetchAvailableModels 获取可用模型和配额信息，返回解析后的结构体和原始 JSON
// 支持 URL fallback：sandbox → daily → prod
func (c *Client) FetchAvailableModels(ctx context.Context, accessToken, projectID string) (*FetchAvailableModelsResponse, map[string]any, error) {
	reqBody := FetchAvailableModelsRequest{Project: projectID}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	// 固定顺序：prod -> daily
	availableURLs := BaseURLs

	var lastErr error
	for urlIdx, baseURL := range availableURLs {
		apiURL := baseURL + "/v1internal:fetchAvailableModels"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(bodyBytes)))
		if err != nil {
			lastErr = fmt.Errorf("创建请求失败: %w", err)
			continue
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", GetUserAgent())

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("fetchAvailableModels 请求失败: %w", err)
			if shouldFallbackToNextURL(err, 0) && urlIdx < len(availableURLs)-1 {
				log.Printf("[antigravity] fetchAvailableModels URL fallback: %s -> %s", baseURL, availableURLs[urlIdx+1])
				continue
			}
			return nil, nil, lastErr
		}

		respBodyBytes, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close() // 立即关闭，避免循环内 defer 导致的资源泄漏
		if err != nil {
			return nil, nil, fmt.Errorf("读取响应失败: %w", err)
		}

		// 检查是否需要 URL 降级
		if shouldFallbackToNextURL(nil, resp.StatusCode) && urlIdx < len(availableURLs)-1 {
			log.Printf("[antigravity] fetchAvailableModels URL fallback (HTTP %d): %s -> %s", resp.StatusCode, baseURL, availableURLs[urlIdx+1])
			continue
		}

		if resp.StatusCode == http.StatusForbidden {
			return nil, nil, &ForbiddenError{
				StatusCode: resp.StatusCode,
				Body:       string(respBodyBytes),
			}
		}

		if resp.StatusCode != http.StatusOK {
			return nil, nil, fmt.Errorf("fetchAvailableModels 失败 (HTTP %d): %s", resp.StatusCode, string(respBodyBytes))
		}

		var modelsResp FetchAvailableModelsResponse
		if err := json.Unmarshal(respBodyBytes, &modelsResp); err != nil {
			return nil, nil, fmt.Errorf("响应解析失败: %w", err)
		}

		// 解析原始 JSON 为 map
		var rawResp map[string]any
		_ = json.Unmarshal(respBodyBytes, &rawResp)

		// 标记成功的 URL，下次优先使用
		DefaultURLAvailability.MarkSuccess(baseURL)
		return &modelsResp, rawResp, nil
	}

	return nil, nil, lastErr
}
