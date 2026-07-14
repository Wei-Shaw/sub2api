package service

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	OpenAIAgentRuntimeIDCredentialKey  = "agent_runtime_id"
	OpenAIAgentPrivateKeyCredentialKey = "agent_private_key"
	OpenAIAgentTaskIDCredentialKey     = "task_id"
)

// OpenAIRequestAuthResult 是一次上游请求使用的认证头及实际凭据账号。
type OpenAIRequestAuthResult struct {
	CredentialAccount *Account
	Headers           http.Header
	Mode              string
}

func (r *OpenAIRequestAuthResult) Apply(headers http.Header) {
	if r == nil || headers == nil {
		return
	}
	headers.Del("Authorization")
	headers.Del("X-Api-Key")
	headers.Del("X-Goog-Api-Key")
	headers.Del("ChatGPT-Account-ID")
	headers.Del("X-OpenAI-Fedramp")
	for key, values := range r.Headers {
		for _, value := range values {
			headers.Add(key, value)
		}
	}
}

// OpenAIRequestAuthProvider 统一生成 OpenAI Platform 与 ChatGPT Codex 请求认证头。
type OpenAIRequestAuthProvider struct {
	accountRepo   AccountRepository
	tokenProvider *OpenAITokenProvider
	now           func() time.Time
}

func NewOpenAIRequestAuthProvider(accountRepo AccountRepository, tokenProvider *OpenAITokenProvider) *OpenAIRequestAuthProvider {
	return &OpenAIRequestAuthProvider{
		accountRepo:   accountRepo,
		tokenProvider: tokenProvider,
		now:           time.Now,
	}
}

// Build 为一次新 HTTP 请求或 WebSocket 握手生成认证头。
func (p *OpenAIRequestAuthProvider) Build(ctx context.Context, account *Account) (*OpenAIRequestAuthResult, error) {
	if account == nil {
		return nil, errors.New("account is nil")
	}
	credentialAccount := account
	if account.IsShadow() {
		if p == nil || p.accountRepo == nil {
			return nil, errors.New("account repository is required for shadow account authentication")
		}
		resolved, err := resolveCredentialAccount(ctx, p.accountRepo, account)
		if err != nil {
			return nil, err
		}
		credentialAccount = resolved
	}

	headers := make(http.Header, 3)
	switch credentialAccount.Type {
	case AccountTypeAPIKey:
		apiKey := strings.TrimSpace(credentialAccount.GetOpenAIApiKey())
		if apiKey == "" {
			return nil, errors.New("api_key not found in credentials")
		}
		headers.Set("Authorization", "Bearer "+apiKey)
		return &OpenAIRequestAuthResult{CredentialAccount: credentialAccount, Headers: headers, Mode: "apikey"}, nil
	case AccountTypeOAuth:
		if credentialAccount.IsOpenAIAgentIdentity() {
			now := time.Now()
			if p != nil && p.now != nil {
				now = p.now()
			}
			authorization, err := buildOpenAIAgentIdentityAuthorization(credentialAccount.Credentials, now)
			if err != nil {
				return nil, err
			}
			headers.Set("Authorization", authorization)
			setOpenAIRequestAccountHeaders(headers, credentialAccount)
			return &OpenAIRequestAuthResult{CredentialAccount: credentialAccount, Headers: headers, Mode: OpenAIAuthModeAgentIdentity}, nil
		}

		var accessToken string
		var err error
		if p != nil && p.tokenProvider != nil {
			accessToken, err = p.tokenProvider.GetAccessToken(ctx, credentialAccount)
			if err != nil {
				return nil, err
			}
		} else {
			accessToken = credentialAccount.GetOpenAIAccessToken()
		}
		if strings.TrimSpace(accessToken) == "" {
			return nil, errors.New("access_token not found in credentials")
		}
		headers.Set("Authorization", "Bearer "+accessToken)
		setOpenAIRequestAccountHeaders(headers, credentialAccount)
		return &OpenAIRequestAuthResult{CredentialAccount: credentialAccount, Headers: headers, Mode: "oauth"}, nil
	default:
		return nil, fmt.Errorf("unsupported account type: %s", credentialAccount.Type)
	}
}

func setOpenAIRequestAccountHeaders(headers http.Header, account *Account) {
	if headers == nil || account == nil {
		return
	}
	accountID := strings.TrimSpace(account.GetCredential("chatgpt_account_id"))
	if accountID == "" {
		accountID = strings.TrimSpace(account.GetCredential("organization_id"))
	}
	if accountID != "" {
		headers.Set("ChatGPT-Account-ID", accountID)
	}
	if account.IsChatGPTAccountFedRAMP() {
		headers.Set("X-OpenAI-Fedramp", "true")
	}
}

// ValidateOpenAIAgentIdentityCredentials 校验导入凭据并保证私钥是 Ed25519 PKCS#8。
func ValidateOpenAIAgentIdentityCredentials(credentials map[string]any) error {
	if !isOpenAIAgentIdentityAuthMode(credentialString(credentials, openAIAuthModeCredentialKey)) {
		return errors.New("auth_mode must be agentIdentity")
	}
	for _, key := range []string{
		OpenAIAgentRuntimeIDCredentialKey,
		OpenAIAgentPrivateKeyCredentialKey,
		OpenAIAgentTaskIDCredentialKey,
		"chatgpt_account_id",
		"chatgpt_user_id",
	} {
		if credentialString(credentials, key) == "" {
			return fmt.Errorf("%s is required", key)
		}
	}
	_, err := parseOpenAIAgentIdentityPrivateKey(credentialString(credentials, OpenAIAgentPrivateKeyCredentialKey))
	return err
}

func buildOpenAIAgentIdentityAuthorization(credentials map[string]any, now time.Time) (string, error) {
	if err := ValidateOpenAIAgentIdentityCredentials(credentials); err != nil {
		return "", fmt.Errorf("invalid OpenAI agent identity credentials: %w", err)
	}
	privateKey, err := parseOpenAIAgentIdentityPrivateKey(credentialString(credentials, OpenAIAgentPrivateKeyCredentialKey))
	if err != nil {
		return "", err
	}
	runtimeID := credentialString(credentials, OpenAIAgentRuntimeIDCredentialKey)
	taskID := credentialString(credentials, OpenAIAgentTaskIDCredentialKey)
	timestamp := now.UTC().Truncate(time.Second).Format(time.RFC3339)
	signature := ed25519.Sign(privateKey, []byte(runtimeID+":"+taskID+":"+timestamp))
	envelope := struct {
		AgentRuntimeID string `json:"agent_runtime_id"`
		TaskID         string `json:"task_id"`
		Timestamp      string `json:"timestamp"`
		Signature      string `json:"signature"`
	}{
		AgentRuntimeID: runtimeID,
		TaskID:         taskID,
		Timestamp:      timestamp,
		Signature:      base64.StdEncoding.EncodeToString(signature),
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("marshal agent identity assertion: %w", err)
	}
	return "AgentAssertion " + base64.RawURLEncoding.EncodeToString(payload), nil
}

func parseOpenAIAgentIdentityPrivateKey(encoded string) (ed25519.PrivateKey, error) {
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, errors.New("agent_private_key is not valid base64")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, errors.New("agent_private_key is not valid PKCS#8")
	}
	privateKey, ok := parsed.(ed25519.PrivateKey)
	if !ok || len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("agent_private_key must be an Ed25519 PKCS#8 private key")
	}
	return privateKey, nil
}

func credentialString(credentials map[string]any, key string) string {
	if credentials == nil {
		return ""
	}
	value, _ := credentials[key].(string)
	return strings.TrimSpace(value)
}
