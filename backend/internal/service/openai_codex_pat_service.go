package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

const openAICodexPATWhoamiURLDefault = "https://auth.openai.com/api/accounts/v1/user-auth-credential/whoami"

var openAICodexPATWhoamiURL = openAICodexPATWhoamiURLDefault

var openAIPersonalAccessTokenOAuthCredentialKeys = [...]string{
	"refresh_token",
	"id_token",
	"expires_at",
	"expires_in",
	"client_id",
REDACTED

type openAICodexPATWhoamiResponse struct {
	Email                   string `json:"email"`
	ChatGPTUserID           string `json:"chatgpt_user_id"`
	ChatGPTAccountID        string `json:"chatgpt_account_id"`
	ChatGPTPlanType         string `json:"chatgpt_plan_type"`
	ChatGPTAccountIsFedRAMP *bool  `json:"chatgpt_account_is_fedramp"`
REDACTED

// ValidateCodexPersonalAccessToken validates a Codex at-* token using the same
// first-class PAT endpoint used by the Codex client.
func (s *OpenAIOAuthService) ValidateCodexPersonalAccessToken(ctx context.Context, accessToken, proxyURL string) (*OpenAITokenInfo, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_CODEX_PAT_REQUIRED", "access token is required")
REDACTED
	if !strings.HasPrefix(accessToken, "at-") {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_CODEX_PAT_INVALID_PREFIX", "Codex personal access token must start with at-")
REDACTED

	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL:              proxyURL,
		Timeout:               20 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
REDACTED)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadRequest, "OPENAI_CODEX_PAT_PROXY_INVALID", "invalid proxy configuration: %v", err)
REDACTED

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openAICodexPATWhoamiURL, nil)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_CODEX_PAT_REQUEST_FAILED", "failed to build validation request: %v", err)
REDACTED
	req.Header.Set("authorization", "Bearer "+accessToken)
	req.Header.Set("accept", "application/json")
	req.Header.Set("originator", "codex_cli_rs")
	req.Header.Set("user-agent", codexCLIUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_PAT_VALIDATE_FAILED", "failed to validate Codex personal access token: %v", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_CODEX_PAT_INVALID", "Codex personal access token is invalid or expired")
REDACTED
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
	REDACTED
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_PAT_VALIDATE_FAILED", "Codex personal access token validation failed: %s", message)
REDACTED

	var whoami openAICodexPATWhoamiResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&whoami); err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_PAT_RESPONSE_INVALID", "invalid Codex personal access token validation response: %v", err)
REDACTED
	if err := validateOpenAICodexPATWhoami(whoami); err != nil {
		return nil, err
REDACTED

	return &OpenAITokenInfo{
		AccessToken:           accessToken,
		AuthMode:              OpenAIAuthModePersonalAccessToken,
		Email:                 strings.TrimSpace(whoami.Email),
		ChatGPTAccountID:      strings.TrimSpace(whoami.ChatGPTAccountID),
		ChatGPTUserID:         strings.TrimSpace(whoami.ChatGPTUserID),
		ChatGPTAccountFedRAMP: *whoami.ChatGPTAccountIsFedRAMP,
		PlanType:              strings.TrimSpace(whoami.ChatGPTPlanType),
REDACTED, nil
REDACTED

func validateOpenAICodexPATWhoami(whoami openAICodexPATWhoamiResponse) error {
	required := map[string]string{
		"email":              whoami.Email,
		"chatgpt_user_id":    whoami.ChatGPTUserID,
		"chatgpt_account_id": whoami.ChatGPTAccountID,
		"chatgpt_plan_type":  whoami.ChatGPTPlanType,
REDACTED
	for key, value := range required {
		if strings.TrimSpace(value) == "" {
			return infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_PAT_RESPONSE_INVALID", "Codex personal access token validation response is missing %s", key)
	REDACTED
REDACTED
	if whoami.ChatGPTAccountIsFedRAMP == nil {
		return infraerrors.New(http.StatusBadGateway, "OPENAI_CODEX_PAT_RESPONSE_INVALID", "Codex personal access token validation response is missing chatgpt_account_is_fedramp")
REDACTED
	return nil
REDACTED

// NormalizeOpenAIPersonalAccessTokenCredentials removes OAuth-only credential
// fields from Codex personal access token accounts while preserving local
// routing, mapping, quota, and metadata fields.
func NormalizeOpenAIPersonalAccessTokenCredentials(account *Account, tokenInfo *OpenAITokenInfo, credentials map[string]any) map[string]any {
	if credentials == nil || !isOpenAIPersonalAccessTokenCredentialSet(account, tokenInfo, credentials) {
		return credentials
REDACTED

	for _, key := range openAIPersonalAccessTokenOAuthCredentialKeys {
		delete(credentials, key)
REDACTED
	credentials[openAIAuthModeCredentialKey] = OpenAIAuthModePersonalAccessToken
	credentials[openAIAuthModeLegacyCredentialKey] = "personal_access_token"
	credentials["token_type"] = "Bearer"
	return credentials
REDACTED

func isOpenAIPersonalAccessTokenCredentialSet(account *Account, tokenInfo *OpenAITokenInfo, credentials map[string]any) bool {
	if tokenInfo != nil && isOpenAIPersonalAccessTokenAuthMode(tokenInfo.AuthMode) {
		return true
REDACTED
	if account != nil && account.IsOpenAIPersonalAccessToken() {
		return true
REDACTED
	return isOpenAIPersonalAccessTokenAuthMode(openAICredentialString(credentials[openAIAuthModeCredentialKey])) ||
		isOpenAIPersonalAccessTokenAuthMode(openAICredentialString(credentials[openAIAuthModeLegacyCredentialKey]))
REDACTED

func openAICredentialString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
REDACTED
REDACTED
