package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service/ports"
)

type CRSSyncService struct {
	accountRepo        ports.AccountRepository
	proxyRepo          ports.ProxyRepository
	oauthService       *OAuthService
	openaiOAuthService *OpenAIOAuthService
}

func NewCRSSyncService(
	accountRepo ports.AccountRepository,
	proxyRepo ports.ProxyRepository,
	oauthService *OAuthService,
	openaiOAuthService *OpenAIOAuthService,
) *CRSSyncService {
	return &CRSSyncService{
		accountRepo:        accountRepo,
		proxyRepo:          proxyRepo,
		oauthService:       oauthService,
		openaiOAuthService: openaiOAuthService,
	}
}

type SyncFromCRSInput struct {
	BaseURL     string
	Username    string
	Password    string
	SyncProxies bool
}

type SyncFromCRSItemResult struct {
	CRSAccountID string `json:"crs_account_id"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Action       string `json:"action"` // created/updated/failed/skipped
	Error        string `json:"error,omitempty"`
}

type SyncFromCRSResult struct {
	Created int                     `json:"created"`
	Updated int                     `json:"updated"`
	Skipped int                     `json:"skipped"`
	Failed  int                     `json:"failed"`
	Items   []SyncFromCRSItemResult `json:"items"`
}

type crsLoginResponse struct {
	Success  bool   `json:"success"`
	Token    string `json:"token"`
	Message  string `json:"message"`
	Error    string `json:"error"`
	Username string `json:"username"`
}

type crsExportResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Message string `json:"message"`
	Data    struct {
		ExportedAt              string                      `json:"exportedAt"`
		ClaudeAccounts          []crsClaudeAccount          `json:"claudeAccounts"`
		ClaudeConsoleAccounts   []crsConsoleAccount         `json:"claudeConsoleAccounts"`
		OpenAIOAuthAccounts     []crsOpenAIOAuthAccount     `json:"openaiOAuthAccounts"`
		OpenAIResponsesAccounts []crsOpenAIResponsesAccount `json:"openaiResponsesAccounts"`
	} `json:"data"`
}

type crsProxy struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type crsClaudeAccount struct {
	Kind        string         `json:"kind"`
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Platform    string         `json:"platform"`
	AuthType    string         `json:"authType"` // oauth/setup-token
	IsActive    bool           `json:"isActive"`
	Schedulable bool           `json:"schedulable"`
	Priority    int            `json:"priority"`
	Status      string         `json:"status"`
	Proxy       *crsProxy      `json:"proxy"`
	Credentials map[string]any `json:"credentials"`
	Extra       map[string]any `json:"extra"`
}

type crsConsoleAccount struct {
	Kind               string         `json:"kind"`
	ID                 string         `json:"id"`
	Name               string         `json:"name"`
	Description        string         `json:"description"`
	Platform           string         `json:"platform"`
	IsActive           bool           `json:"isActive"`
	Schedulable        bool           `json:"schedulable"`
	Priority           int            `json:"priority"`
	Status             string         `json:"status"`
	MaxConcurrentTasks int            `json:"maxConcurrentTasks"`
	Proxy              *crsProxy      `json:"proxy"`
	Credentials        map[string]any `json:"credentials"`
}

type crsOpenAIResponsesAccount struct {
	Kind        string         `json:"kind"`
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Platform    string         `json:"platform"`
	IsActive    bool           `json:"isActive"`
	Schedulable bool           `json:"schedulable"`
	Priority    int            `json:"priority"`
	Status      string         `json:"status"`
	Proxy       *crsProxy      `json:"proxy"`
	Credentials map[string]any `json:"credentials"`
}

type crsOpenAIOAuthAccount struct {
	Kind        string         `json:"kind"`
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Platform    string         `json:"platform"`
	AuthType    string         `json:"authType"` // oauth
	IsActive    bool           `json:"isActive"`
	Schedulable bool           `json:"schedulable"`
	Priority    int            `json:"priority"`
	Status      string         `json:"status"`
	Proxy       *crsProxy      `json:"proxy"`
	Credentials map[string]any `json:"credentials"`
	Extra       map[string]any `json:"extra"`
}

func (s *CRSSyncService) SyncFromCRS(ctx context.Context, input SyncFromCRSInput) (*SyncFromCRSResult, error) {
	baseURL, err := normalizeBaseURL(input.BaseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Username) == "" || strings.TrimSpace(input.Password) == "" {
		return nil, errors.New("username and password are required")
	}

	client := &http.Client{Timeout: 20 * time.Second}

	adminToken, err := crsLogin(ctx, client, baseURL, input.Username, input.Password)
	if err != nil {
		return nil, err
	}

	exported, err := crsExportAccounts(ctx, client, baseURL, adminToken)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)

	result := &SyncFromCRSResult{
		Items: make(
			[]SyncFromCRSItemResult,
			0,
			len(exported.Data.ClaudeAccounts)+len(exported.Data.ClaudeConsoleAccounts)+len(exported.Data.OpenAIOAuthAccounts)+len(exported.Data.OpenAIResponsesAccounts),
		),
	}

	var proxies []model.Proxy
	if input.SyncProxies {
		proxies, _ = s.proxyRepo.ListActive(ctx)
	}

	// Claude OAuth / Setup Token -> sub2api anthropic oauth/setup-token
	for _, src := range exported.Data.ClaudeAccounts {
		item := SyncFromCRSItemResult{
			CRSAccountID: src.ID,
			Kind:         src.Kind,
			Name:         src.Name,
		}

		targetType := strings.TrimSpace(src.AuthType)
		if targetType == "" {
			targetType = "oauth"
		}
		if targetType != model.AccountTypeOAuth && targetType != model.AccountTypeSetupToken {
			item.Action = "skipped"
			item.Error = "unsupported authType: " + targetType
			result.Skipped++
			result.Items = append(result.Items, item)
			continue
		}

		accessToken, _ := src.Credentials["access_token"].(string)
		if strings.TrimSpace(accessToken) == "" {
			item.Action = "failed"
			item.Error = "missing access_token"
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		proxyID, err := s.mapOrCreateProxy(ctx, input.SyncProxies, &proxies, src.Proxy, fmt.Sprintf("crs-%s", src.Name))
		if err != nil {
			item.Action = "failed"
			item.Error = "proxy sync failed: " + err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		credentials := sanitizeCredentialsMap(src.Credentials)
		// 🔧 Remove /v1 suffix from base_url for Claude accounts
		cleanBaseURL(credentials, "/v1")
		// 🔧 Convert expires_at from ISO string to Unix timestamp
		if expiresAtStr, ok := credentials["expires_at"].(string); ok && expiresAtStr != "" {
			if t, err := time.Parse(time.RFC3339, expiresAtStr); err == nil {
				credentials["expires_at"] = t.Unix()
			}
		}
		// 🔧 Add intercept_warmup_requests if not present (defaults to false)
		if _, exists := credentials["intercept_warmup_requests"]; !exists {
			credentials["intercept_warmup_requests"] = false
		}
		priority := clampPriority(src.Priority)
		concurrency := 3
		status := mapCRSStatus(src.IsActive, src.Status)

		// 🔧 Preserve all CRS extra fields and add sync metadata
		extra := make(map[string]any)
		if src.Extra != nil {
			for k, v := range src.Extra {
				extra[k] = v
			}
		}
		extra["crs_account_id"] = src.ID
		extra["crs_kind"] = src.Kind
		extra["crs_synced_at"] = now
		// Extract org_uuid and account_uuid from CRS credentials to extra
		if orgUUID, ok := src.Credentials["org_uuid"]; ok {
			extra["org_uuid"] = orgUUID
		}
		if accountUUID, ok := src.Credentials["account_uuid"]; ok {
			extra["account_uuid"] = accountUUID
		}

		existing, err := s.accountRepo.GetByCRSAccountID(ctx, src.ID)
		if err != nil {
			item.Action = "failed"
			item.Error = "db lookup failed: " + err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		if existing == nil {
			account := &model.Account{
				Name:        defaultName(src.Name, src.ID),
				Platform:    model.PlatformAnthropic,
				Type:        targetType,
				Credentials: model.JSONB(credentials),
				Extra:       model.JSONB(extra),
				ProxyID:     proxyID,
				Concurrency: concurrency,
				Priority:    priority,
				Status:      status,
				Schedulable: src.Schedulable,
			}
			if err := s.accountRepo.Create(ctx, account); err != nil {
				item.Action = "failed"
				item.Error = "create failed: " + err.Error()
				result.Failed++
				result.Items = append(result.Items, item)
				continue
			}
			// 🔄 Refresh OAuth token after creation
			if targetType == model.AccountTypeOAuth {
				if refreshedCreds := s.refreshOAuthToken(ctx, account); refreshedCreds != nil {
					account.Credentials = refreshedCreds
					_ = s.accountRepo.Update(ctx, account)
				}
			}
			item.Action = "created"
			result.Created++
			result.Items = append(result.Items, item)
			continue
		}

		// Update existing
		existing.Extra = mergeJSONB(existing.Extra, extra)
		existing.Name = defaultName(src.Name, src.ID)
		existing.Platform = model.PlatformAnthropic
		existing.Type = targetType
		existing.Credentials = mergeJSONB(existing.Credentials, credentials)
		if proxyID != nil {
			existing.ProxyID = proxyID
		}
		existing.Concurrency = concurrency
		existing.Priority = priority
		existing.Status = status
		existing.Schedulable = src.Schedulable

		if err := s.accountRepo.Update(ctx, existing); err != nil {
			item.Action = "failed"
			item.Error = "update failed: " + err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		// 🔄 Refresh OAuth token after update
		if targetType == model.AccountTypeOAuth {
			if refreshedCreds := s.refreshOAuthToken(ctx, existing); refreshedCreds != nil {
				existing.Credentials = refreshedCreds
				_ = s.accountRepo.Update(ctx, existing)
			}
		}

		item.Action = "updated"
		result.Updated++
		result.Items = append(result.Items, item)
	}

	// Claude Console API Key -> sub2api anthropic apikey
	for _, src := range exported.Data.ClaudeConsoleAccounts {
		item := SyncFromCRSItemResult{
			CRSAccountID: src.ID,
			Kind:         src.Kind,
			Name:         src.Name,
		}

		apiKey, _ := src.Credentials["api_key"].(string)
		if strings.TrimSpace(apiKey) == "" {
			item.Action = "failed"
			item.Error = "missing api_key"
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		proxyID, err := s.mapOrCreateProxy(ctx, input.SyncProxies, &proxies, src.Proxy, fmt.Sprintf("crs-%s", src.Name))
		if err != nil {
			item.Action = "failed"
			item.Error = "proxy sync failed: " + err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		credentials := sanitizeCredentialsMap(src.Credentials)
		priority := clampPriority(src.Priority)
		concurrency := 3
		if src.MaxConcurrentTasks > 0 {
			concurrency = src.MaxConcurrentTasks
		}
		status := mapCRSStatus(src.IsActive, src.Status)

		extra := map[string]any{
			"crs_account_id": src.ID,
			"crs_kind":       src.Kind,
			"crs_synced_at":  now,
		}

		existing, err := s.accountRepo.GetByCRSAccountID(ctx, src.ID)
		if err != nil {
			item.Action = "failed"
			item.Error = "db lookup failed: " + err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		if existing == nil {
			account := &model.Account{
				Name:        defaultName(src.Name, src.ID),
				Platform:    model.PlatformAnthropic,
				Type:        model.AccountTypeApiKey,
				Credentials: model.JSONB(credentials),
				Extra:       model.JSONB(extra),
				ProxyID:     proxyID,
				Concurrency: concurrency,
				Priority:    priority,
				Status:      status,
				Schedulable: src.Schedulable,
			}
			if err := s.accountRepo.Create(ctx, account); err != nil {
				item.Action = "failed"
				item.Error = "create failed: " + err.Error()
				result.Failed++
				result.Items = append(result.Items, item)
				continue
			}
			item.Action = "created"
			result.Created++
			result.Items = append(result.Items, item)
			continue
		}

		existing.Extra = mergeJSONB(existing.Extra, extra)
		existing.Name = defaultName(src.Name, src.ID)
		existing.Platform = model.PlatformAnthropic
		existing.Type = model.AccountTypeApiKey
		existing.Credentials = mergeJSONB(existing.Credentials, credentials)
		if proxyID != nil {
			existing.ProxyID = proxyID
		}
		existing.Concurrency = concurrency
		existing.Priority = priority
		existing.Status = status
		existing.Schedulable = src.Schedulable

		if err := s.accountRepo.Update(ctx, existing); err != nil {
			item.Action = "failed"
			item.Error = "update failed: " + err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		item.Action = "updated"
		result.Updated++
		result.Items = append(result.Items, item)
	}

	// OpenAI OAuth -> sub2api openai oauth
	for _, src := range exported.Data.OpenAIOAuthAccounts {
		item := SyncFromCRSItemResult{
			CRSAccountID: src.ID,
			Kind:         src.Kind,
			Name:         src.Name,
		}

		accessToken, _ := src.Credentials["access_token"].(string)
		if strings.TrimSpace(accessToken) == "" {
			item.Action = "failed"
			item.Error = "missing access_token"
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		proxyID, err := s.mapOrCreateProxy(
			ctx,
			input.SyncProxies,
			&proxies,
			src.Proxy,
			fmt.Sprintf("crs-%s", src.Name),
		)
		if err != nil {
			item.Action = "failed"
			item.Error = "proxy sync failed: " + err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		credentials := sanitizeCredentialsMap(src.Credentials)
		// Normalize token_type
		if v, ok := credentials["token_type"].(string); !ok || strings.TrimSpace(v) == "" {
			credentials["token_type"] = "Bearer"
		}
		// 🔧 Convert expires_at from ISO string to Unix timestamp
		if expiresAtStr, ok := credentials["expires_at"].(string); ok && expiresAtStr != "" {
			if t, err := time.Parse(time.RFC3339, expiresAtStr); err == nil {
				credentials["expires_at"] = t.Unix()
			}
		}
		priority := clampPriority(src.Priority)
		concurrency := 3
		status := mapCRSStatus(src.IsActive, src.Status)

		// 🔧 Preserve all CRS extra fields and add sync metadata
		extra := make(map[string]any)
		if src.Extra != nil {
			for k, v := range src.Extra {
				extra[k] = v
			}
		}
		extra["crs_account_id"] = src.ID
		extra["crs_kind"] = src.Kind
		extra["crs_synced_at"] = now
		// Extract email from CRS extra (crs_email -> email)
		if crsEmail, ok := src.Extra["crs_email"]; ok {
			extra["email"] = crsEmail
		}

		existing, err := s.accountRepo.GetByCRSAccountID(ctx, src.ID)
		if err != nil {
			item.Action = "failed"
			item.Error = "db lookup failed: " + err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		if existing == nil {
			account := &model.Account{
				Name:        defaultName(src.Name, src.ID),
				Platform:    model.PlatformOpenAI,
				Type:        model.AccountTypeOAuth,
				Credentials: model.JSONB(credentials),
				Extra:       model.JSONB(extra),
				ProxyID:     proxyID,
				Concurrency: concurrency,
				Priority:    priority,
				Status:      status,
				Schedulable: src.Schedulable,
			}
			if err := s.accountRepo.Create(ctx, account); err != nil {
				item.Action = "failed"
				item.Error = "create failed: " + err.Error()
				result.Failed++
				result.Items = append(result.Items, item)
				continue
			}
			// 🔄 Refresh OAuth token after creation
			if refreshedCreds := s.refreshOAuthToken(ctx, account); refreshedCreds != nil {
				account.Credentials = refreshedCreds
				_ = s.accountRepo.Update(ctx, account)
			}
			item.Action = "created"
			result.Created++
			result.Items = append(result.Items, item)
			continue
		}

		existing.Extra = mergeJSONB(existing.Extra, extra)
		existing.Name = defaultName(src.Name, src.ID)
		existing.Platform = model.PlatformOpenAI
		existing.Type = model.AccountTypeOAuth
		existing.Credentials = mergeJSONB(existing.Credentials, credentials)
		if proxyID != nil {
			existing.ProxyID = proxyID
		}
		existing.Concurrency = concurrency
		existing.Priority = priority
		existing.Status = status
		existing.Schedulable = src.Schedulable

		if err := s.accountRepo.Update(ctx, existing); err != nil {
			item.Action = "failed"
			item.Error = "update failed: " + err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		// 🔄 Refresh OAuth token after update
		if refreshedCreds := s.refreshOAuthToken(ctx, existing); refreshedCreds != nil {
			existing.Credentials = refreshedCreds
			_ = s.accountRepo.Update(ctx, existing)
		}

		item.Action = "updated"
		result.Updated++
		result.Items = append(result.Items, item)
	}

	// OpenAI Responses API Key -> sub2api openai apikey
	for _, src := range exported.Data.OpenAIResponsesAccounts {
		item := SyncFromCRSItemResult{
			CRSAccountID: src.ID,
			Kind:         src.Kind,
			Name:         src.Name,
		}

		apiKey, _ := src.Credentials["api_key"].(string)
		if strings.TrimSpace(apiKey) == "" {
			item.Action = "failed"
			item.Error = "missing api_key"
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		if baseURL, ok := src.Credentials["base_url"].(string); !ok || strings.TrimSpace(baseURL) == "" {
			src.Credentials["base_url"] = "https://api.openai.com"
		}
		// 🔧 Remove /v1 suffix from base_url for OpenAI accounts
		cleanBaseURL(src.Credentials, "/v1")

		proxyID, err := s.mapOrCreateProxy(
			ctx,
			input.SyncProxies,
			&proxies,
			src.Proxy,
			fmt.Sprintf("crs-%s", src.Name),
		)
		if err != nil {
			item.Action = "failed"
			item.Error = "proxy sync failed: " + err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		credentials := sanitizeCredentialsMap(src.Credentials)
		priority := clampPriority(src.Priority)
		concurrency := 3
		status := mapCRSStatus(src.IsActive, src.Status)

		extra := map[string]any{
			"crs_account_id": src.ID,
			"crs_kind":       src.Kind,
			"crs_synced_at":  now,
		}

		existing, err := s.accountRepo.GetByCRSAccountID(ctx, src.ID)
		if err != nil {
			item.Action = "failed"
			item.Error = "db lookup failed: " + err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		if existing == nil {
			account := &model.Account{
				Name:        defaultName(src.Name, src.ID),
				Platform:    model.PlatformOpenAI,
				Type:        model.AccountTypeApiKey,
				Credentials: model.JSONB(credentials),
				Extra:       model.JSONB(extra),
				ProxyID:     proxyID,
				Concurrency: concurrency,
				Priority:    priority,
				Status:      status,
				Schedulable: src.Schedulable,
			}
			if err := s.accountRepo.Create(ctx, account); err != nil {
				item.Action = "failed"
				item.Error = "create failed: " + err.Error()
				result.Failed++
				result.Items = append(result.Items, item)
				continue
			}
			item.Action = "created"
			result.Created++
			result.Items = append(result.Items, item)
			continue
		}

		existing.Extra = mergeJSONB(existing.Extra, extra)
		existing.Name = defaultName(src.Name, src.ID)
		existing.Platform = model.PlatformOpenAI
		existing.Type = model.AccountTypeApiKey
		existing.Credentials = mergeJSONB(existing.Credentials, credentials)
		if proxyID != nil {
			existing.ProxyID = proxyID
		}
		existing.Concurrency = concurrency
		existing.Priority = priority
		existing.Status = status
		existing.Schedulable = src.Schedulable

		if err := s.accountRepo.Update(ctx, existing); err != nil {
			item.Action = "failed"
			item.Error = "update failed: " + err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		item.Action = "updated"
		result.Updated++
		result.Items = append(result.Items, item)
	}

	return result, nil
}

// mergeJSONB merges two JSONB maps without removing keys that are absent in updates.
func mergeJSONB(existing model.JSONB, updates map[string]any) model.JSONB {
	out := make(model.JSONB)
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range updates {
		out[k] = v
	}
	return out
}

func (s *CRSSyncService) mapOrCreateProxy(ctx context.Context, enabled bool, cached *[]model.Proxy, src *crsProxy, defaultName string) (*int64, error) {
	if !enabled || src == nil {
		return nil, nil
	}
	protocol := strings.ToLower(strings.TrimSpace(src.Protocol))
	switch protocol {
	case "socks":
		protocol = "socks5"
	case "socks5h":
		protocol = "socks5"
	}
	host := strings.TrimSpace(src.Host)
	port := src.Port
	username := strings.TrimSpace(src.Username)
	password := strings.TrimSpace(src.Password)

	if protocol == "" || host == "" || port <= 0 {
		return nil, nil
	}
	if protocol != "http" && protocol != "https" && protocol != "socks5" {
		return nil, nil
	}

	// Find existing proxy (active only).
	for _, p := range *cached {
		if strings.EqualFold(p.Protocol, protocol) &&
			p.Host == host &&
			p.Port == port &&
			p.Username == username &&
			p.Password == password {
			id := p.ID
			return &id, nil
		}
	}

	// Create new proxy
	proxy := &model.Proxy{
		Name:     defaultProxyName(defaultName, protocol, host, port),
		Protocol: protocol,
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		Status:   model.StatusActive,
	}
	if err := s.proxyRepo.Create(ctx, proxy); err != nil {
		return nil, err
	}

	*cached = append(*cached, *proxy)
	id := proxy.ID
	return &id, nil
}

func defaultProxyName(base, protocol, host string, port int) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "crs"
	}
	return fmt.Sprintf("%s (%s://%s:%d)", base, protocol, host, port)
}

func defaultName(name, id string) string {
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return "CRS " + id
}

func clampPriority(priority int) int {
	if priority < 1 || priority > 100 {
		return 50
	}
	return priority
}

func sanitizeCredentialsMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		// Avoid nil values to keep JSONB cleaner
		if v != nil {
			out[k] = v
		}
	}
	return out
}

func mapCRSStatus(isActive bool, status string) string {
	if !isActive {
		return "inactive"
	}
	if strings.EqualFold(strings.TrimSpace(status), "error") {
		return "error"
	}
	return "active"
}

func normalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("base_url is required")
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid base_url: %s", trimmed)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return strings.TrimRight(u.String(), "/"), nil
}

// cleanBaseURL removes trailing suffix from base_url in credentials
// Used for both Claude and OpenAI accounts to remove /v1
func cleanBaseURL(credentials map[string]any, suffixToRemove string) {
	if baseURL, ok := credentials["base_url"].(string); ok && baseURL != "" {
		trimmed := strings.TrimSpace(baseURL)
		if strings.HasSuffix(trimmed, suffixToRemove) {
			credentials["base_url"] = strings.TrimSuffix(trimmed, suffixToRemove)
		}
	}
}

func crsLogin(ctx context.Context, client *http.Client, baseURL, username, password string) (string, error) {
	payload := map[string]any{
		"username": username,
		"password": password,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/web/auth/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("crs login failed: status=%d body=%s", resp.StatusCode, string(raw))
	}

	var parsed crsLoginResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("crs login parse failed: %w", err)
	}
	if !parsed.Success || strings.TrimSpace(parsed.Token) == "" {
		msg := parsed.Message
		if msg == "" {
			msg = parsed.Error
		}
		if msg == "" {
			msg = "unknown error"
		}
		return "", errors.New("crs login failed: " + msg)
	}
	return parsed.Token, nil
}

func crsExportAccounts(ctx context.Context, client *http.Client, baseURL, adminToken string) (*crsExportResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/admin/sync/export-accounts?include_secrets=true", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("crs export failed: status=%d body=%s", resp.StatusCode, string(raw))
	}

	var parsed crsExportResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("crs export parse failed: %w", err)
	}
	if !parsed.Success {
		msg := parsed.Message
		if msg == "" {
			msg = parsed.Error
		}
		if msg == "" {
			msg = "unknown error"
		}
		return nil, errors.New("crs export failed: " + msg)
	}
	return &parsed, nil
}

// refreshOAuthToken attempts to refresh OAuth token for a synced account
// Returns updated credentials or nil if refresh failed/not applicable
func (s *CRSSyncService) refreshOAuthToken(ctx context.Context, account *model.Account) model.JSONB {
	if account.Type != model.AccountTypeOAuth {
		return nil
	}

	var newCredentials map[string]any
	var err error

	if account.Platform == model.PlatformAnthropic {
		if s.oauthService == nil {
			return nil
		}
		tokenInfo, refreshErr := s.oauthService.RefreshAccountToken(ctx, account)
		if refreshErr != nil {
			err = refreshErr
		} else {
			// Preserve existing credentials
			newCredentials = make(map[string]any)
			for k, v := range account.Credentials {
				newCredentials[k] = v
			}
			// Update token fields
			newCredentials["access_token"] = tokenInfo.AccessToken
			newCredentials["token_type"] = tokenInfo.TokenType
			newCredentials["expires_in"] = tokenInfo.ExpiresIn
			newCredentials["expires_at"] = tokenInfo.ExpiresAt
			if tokenInfo.RefreshToken != "" {
				newCredentials["refresh_token"] = tokenInfo.RefreshToken
			}
			if tokenInfo.Scope != "" {
				newCredentials["scope"] = tokenInfo.Scope
			}
		}
	} else if account.Platform == model.PlatformOpenAI {
		if s.openaiOAuthService == nil {
			return nil
		}
		tokenInfo, refreshErr := s.openaiOAuthService.RefreshAccountToken(ctx, account)
		if refreshErr != nil {
			err = refreshErr
		} else {
			newCredentials = s.openaiOAuthService.BuildAccountCredentials(tokenInfo)
			// Preserve non-token settings from existing credentials
			for k, v := range account.Credentials {
				if _, exists := newCredentials[k]; !exists {
					newCredentials[k] = v
				}
			}
		}
	}

	if err != nil {
		// Log but don't fail the sync - token might still be valid or refreshable later
		return nil
	}

	return model.JSONB(newCredentials)
}
