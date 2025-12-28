package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

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

// LoadCodeAssistResponse loadCodeAssist 响应
type LoadCodeAssistResponse struct {
	CloudAICompanionProject string `json:"cloudaicompanionProject"`
REDACTED

// Client Antigravity API 客户端
type Client struct {
	httpClient *http.Client
	proxyURL   string
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
		proxyURL:   proxyURL,
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
		return nil, fmt.Errorf("Token 交换请求失败: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
REDACTED

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Token 交换失败 (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
REDACTED

	var tokenResp TokenResponse
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return nil, fmt.Errorf("Token 解析失败: %w", err)
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
		return nil, fmt.Errorf("Token 刷新请求失败: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
REDACTED

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Token 刷新失败 (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
REDACTED

	var tokenResp TokenResponse
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return nil, fmt.Errorf("Token 解析失败: %w", err)
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

// LoadCodeAssist 获取 project_id
func (c *Client) LoadCodeAssist(ctx context.Context, accessToken string) (*LoadCodeAssistResponse, error) {
	reqBody := LoadCodeAssistRequest{REDACTED
	reqBody.Metadata.IDEType = "ANTIGRAVITY"

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
REDACTED

	url := BaseURL + "/v1internal:loadCodeAssist"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
REDACTED
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("loadCodeAssist 请求失败: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	respBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
REDACTED

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("loadCodeAssist 失败 (HTTP %d): %s", resp.StatusCode, string(respBodyBytes))
REDACTED

	var loadResp LoadCodeAssistResponse
	if err := json.Unmarshal(respBodyBytes, &loadResp); err != nil {
		return nil, fmt.Errorf("响应解析失败: %w", err)
REDACTED

	return &loadResp, nil
REDACTED
