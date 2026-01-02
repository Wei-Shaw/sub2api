// Package antigravity provides a client for interacting with Google's Antigravity API,
// handling OAuth authentication, token management, and account tier information retrieval.
package antigravity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NewAPIRequest 创建 Antigravity API 请求（v1internal 端点）
func NewAPIRequest(ctx context.Context, action, accessToken string, body []byte) (*http.Request, error) {
	apiURL := fmt.Sprintf("%s/v1internal:%s", BaseURL, action)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", UserAgent)
	return req, nil
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
		IDEType string `json:"ideType"`
REDACTED `json:"metadata"`
REDACTED

// TierInfo 账户类型信息
type TierInfo struct {
	ID          string `json:"id"`          // free-tier, g1-pro-tier, g1-ultra-tier
	Name        string `json:"name"`        // 显示名称
	Description string `json:"description"` // 描述
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
	PaidTier                *TierInfo         `json:"paidTier,omitempty"`
	IneligibleTiers         []*IneligibleTier `json:"ineligibleTiers,omitempty"`
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

// Client Antigravity API 客户端
type Client struct {
	httpClient *http.Client
REDACTED

func NewClient(proxyURL string) *Client {
	client := &http.Client{
		Timeout: 30 * time.Second,
REDACTED

	if strings.TrimSpace(proxyURL) != "" {
		if proxyURLParsed, err := url.Parse(proxyURL); err == nil {
			client.Transport = &http.Transport{
				Proxy: http.ProxyURL(proxyURLParsed),
		REDACTED
	REDACTED
REDACTED

	return &Client{
		httpClient: client,
REDACTED
REDACTED

// ExchangeCode 用 authorization code 交换 token
func (c *Client) ExchangeCode(ctx context.Context, code, codeVerifier string) (*TokenResponse, error) {
	params := url.Values{REDACTED
	params.Set("client_id", ClientID)
	params.Set("client_secret", ClientSecret)
	params.Set("code", code)
	params.Set("redirect_uri", RedirectURI)
	params.Set("grant_type", "authorization_code")
	params.Set("code_verifier", codeVerifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
REDACTED
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
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
	params := url.Values{REDACTED
	params.Set("client_id", ClientID)
	params.Set("client_secret", ClientSecret)
	params.Set("refresh_token", refreshToken)
	params.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
REDACTED
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
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

	resp, err := c.httpClient.Do(req)
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
func (c *Client) LoadCodeAssist(ctx context.Context, accessToken string) (*LoadCodeAssistResponse, map[string]any, error) {
	reqBody := LoadCodeAssistRequest{REDACTED
	reqBody.Metadata.IDEType = "ANTIGRAVITY"

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("序列化请求失败: %w", err)
REDACTED

	url := BaseURL + "/v1internal:loadCodeAssist"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, nil, fmt.Errorf("创建请求失败: %w", err)
REDACTED
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("loadCodeAssist 请求失败: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	respBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("读取响应失败: %w", err)
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

	return &loadResp, rawResp, nil
REDACTED

// ModelQuotaInfo 模型配额信息
type ModelQuotaInfo struct {
	RemainingFraction float64 `json:"remainingFraction"`
	ResetTime         string  `json:"resetTime,omitempty"`
REDACTED

// ModelInfo 模型信息
type ModelInfo struct {
	QuotaInfo *ModelQuotaInfo `json:"quotaInfo,omitempty"`
REDACTED

// FetchAvailableModelsRequest fetchAvailableModels 请求
type FetchAvailableModelsRequest struct {
	Project string `json:"project"`
REDACTED

// FetchAvailableModelsResponse fetchAvailableModels 响应
type FetchAvailableModelsResponse struct {
	Models map[string]ModelInfo `json:"models"`
REDACTED

// FetchAvailableModels 获取可用模型和配额信息，返回解析后的结构体和原始 JSON
func (c *Client) FetchAvailableModels(ctx context.Context, accessToken, projectID string) (*FetchAvailableModelsResponse, map[string]any, error) {
	reqBody := FetchAvailableModelsRequest{Project: projectIDREDACTED
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("序列化请求失败: %w", err)
REDACTED

	apiURL := BaseURL + "/v1internal:fetchAvailableModels"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, nil, fmt.Errorf("创建请求失败: %w", err)
REDACTED
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("fetchAvailableModels 请求失败: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	respBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("读取响应失败: %w", err)
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

	return &modelsResp, rawResp, nil
REDACTED
