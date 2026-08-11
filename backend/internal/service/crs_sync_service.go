package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

type CRSSyncService struct {
	accountRepo        AccountRepository
	proxyRepo          ProxyRepository
	oauthService       *OAuthService
	openaiOAuthService *OpenAIOAuthService
	geminiOAuthService *GeminiOAuthService
	cfg                *config.Config
}

func NewCRSSyncService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	oauthService *OAuthService,
	openaiOAuthService *OpenAIOAuthService,
	geminiOAuthService *GeminiOAuthService,
	cfg *config.Config,
) *CRSSyncService {
	return &CRSSyncService{
		accountRepo:        accountRepo,
		proxyRepo:          proxyRepo,
		oauthService:       oauthService,
		openaiOAuthService: openaiOAuthService,
		geminiOAuthService: geminiOAuthService,
		cfg:                cfg,
	}
}

// guardCRSShadowParentInvariant 守住「有 spark 影子的母账号」不变量(与 AdminService.UpdateAccount 一致):
// 影子读透母账号凭据,母账号必须**始终是 OpenAI OAuth**。CRS 同步按全局 crs_account_id 匹配既有账号
// (GetByCRSAccountID 已排除影子、但能命中母账号),各平台分支会重写 Platform/Type;若 CRS ID 跨 kind/平台
// 碰撞,非 OpenAI 分支会把母账号改成 Anthropic/Gemini 或 api_key→影子 resolveCredentialAccount 必崩(外审第9轮,
// 收紧第8轮仅查 Type 的版本:Claude OAuth 把 Type 保持 OAuth 但 Platform 改成 Anthropic 能绕过旧守卫)。
// 故任何会把母账号目标结果改离 OpenAI OAuth 的 CRS 更新,在其有影子时一律拒绝(须先删影子);返回非 nil
// 表示该账号更新应被跳过(调用方标记 failed)。
func guardCRSShadowParentInvariant(ctx context.Context, repo AccountRepository, existing *Account, newPlatform, newType string) error {
	if existing == nil {
		return nil
	}
	// 目标仍是合法影子父(OpenAI OAuth)→ 放行(常见:OpenAI OAuth 分支重新同步母账号),免去一次查询。
	if newPlatform == PlatformOpenAI && newType == AccountTypeOAuth {
		return nil
	}
	shadows, err := repo.ListShadowsByParent(ctx, existing.ID)
	if err != nil {
		return fmt.Errorf("check spark shadows for crs update: %w", err)
	}
	if len(shadows) > 0 {
		return fmt.Errorf("cannot change a spark-shadow parent account to %s/%s; it must stay OpenAI OAuth (delete the shadow first)", newPlatform, newType)
	}
	return nil
}

type SyncFromCRSInput struct {
	BaseURL            string
	Username           string
	Password           string
	SyncProxies        bool
	SelectedAccountIDs []string // if non-empty, only create new accounts with these CRS IDs
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
		GeminiOAuthAccounts     []crsGeminiOAuthAccount     `json:"geminiOAuthAccounts"`
		GeminiAPIKeyAccounts    []crsGeminiAPIKeyAccount    `json:"geminiApiKeyAccounts"`
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
	Extra       map[string]any `json:"extra"`
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

type crsGeminiOAuthAccount struct {
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

type crsGeminiAPIKeyAccount struct {
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
	Extra       map[string]any `json:"extra"`
}

// fetchCRSExport validates the connection parameters, authenticates with CRS,
// and returns the exported accounts. Shared by SyncFromCRS and PreviewFromCRS.
func (s *CRSSyncService) fetchCRSExport(ctx context.Context, baseURL, username, password string) (*crsExportResponse, error) {
	if s.cfg == nil {
		return nil, errors.New("config is not available")
	}
	normalizedURL := strings.TrimSpace(baseURL)
	if s.cfg.Security.URLAllowlist.Enabled {
		normalized, err := normalizeBaseURL(normalizedURL, s.cfg.Security.URLAllowlist.CRSHosts, s.cfg.Security.URLAllowlist.AllowPrivateHosts)
		if err != nil {
			return nil, err
		}
		normalizedURL = normalized
	} else {
		normalized, err := urlvalidator.ValidateURLFormat(normalizedURL, s.cfg.Security.URLAllowlist.AllowInsecureHTTP)
		if err != nil {
			return nil, fmt.Errorf("invalid base_url: %w", err)
		}
		normalizedURL = normalized
	}
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		return nil, errors.New("username and password are required")
	}

	client, err := httpclient.GetClient(httpclient.Options{
		Timeout:            20 * time.Second,
		ValidateResolvedIP: s.cfg.Security.URLAllowlist.Enabled,
		AllowPrivateHosts:  s.cfg.Security.URLAllowlist.AllowPrivateHosts,
	})
	if err != nil {
		return nil, fmt.Errorf("create http client failed: %w", err)
	}

	adminToken, err := crsLogin(ctx, client, normalizedURL, username, password)
	if err != nil {
		return nil, err
	}

	return crsExportAccounts(ctx, client, normalizedURL, adminToken)
}

func (s *CRSSyncService) SyncFromCRS(ctx context.Context, input SyncFromCRSInput) (*SyncFromCRSResult, error) {
	exported, err := s.fetchCRSExport(ctx, input.BaseURL, input.Username, input.Password)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)

	result := &SyncFromCRSResult{
		Items: make(
			[]SyncFromCRSItemResult,
			0,
			len(exported.Data.ClaudeAccounts)+len(exported.Data.ClaudeConsoleAccounts)+len(exported.Data.OpenAIOAuthAccounts)+len(exported.Data.OpenAIResponsesAccounts)+len(exported.Data.GeminiOAuthAccounts)+len(exported.Data.GeminiAPIKeyAccounts),
		),
	}

	selectedSet := buildSelectedSet(input.SelectedAccountIDs)

	var proxies []Proxy
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
		if targetType != AccountTypeOAuth && targetType != AccountTypeSetupToken {
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
		if existing != nil {
			extra = mergeMap(existing.Extra, extra)
			credentials = mergeMap(existing.Credentials, credentials)
		}
		reconcileCRSUpstreamBillingProbeExtra(existing, PlatformAnthropic, targetType, credentials, extra)

		if existing == nil {
			if !shouldCreateAccount(src.ID, selectedSet) {
				item.Action = "skipped"
				item.Error = "not selected"
				result.Skipped++
				result.Items = append(result.Items, item)
				continue
			}
			account := &Account{
				Name:        defaultName(src.Name, src.ID),
				Platform:    PlatformAnthropic,
				Type:        targetType,
				Credentials: credentials,
				Extra:       extra,
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
			if targetType == AccountTypeOAuth {
				if refreshedCreds := s.refreshOAuthToken(ctx, account); refreshedCreds != nil {
					_ = persistAccountCredentials(ctx, s.accountRepo, account, refreshedCreds)
				}
			}
			item.Action = "created"
			result.Created++
			result.Items = append(result.Items, item)
			continue
		}

		// 母账号守卫(外审第9轮):CRS ID 跨平台碰撞时,本(Anthropic OAuth)分支不得改坏有 spark 影子的 OpenAI 母账号。
		if gerr := guardCRSShadowParentInvariant(ctx, s.accountRepo, existing, PlatformAnthropic, targetType); gerr != nil {
			item.Action = "failed"
			item.Error = gerr.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		// Update existing
		existing.Extra = extra
		existing.Name = defaultName(src.Name, src.ID)
		existing.Platform = PlatformAnthropic
		existing.Type = targetType
		existing.Credentials = credentials
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
		if targetType == AccountTypeOAuth {
			if refreshedCreds := s.refreshOAuthToken(ctx, existing); refreshedCreds != nil {
				_ = persistAccountCredentials(ctx, s.accountRepo, existing, refreshedCreds)
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
		if existing != nil {
			extra = mergeMap(existing.Extra, extra)
			credentials = mergeMap(existing.Credentials, credentials)
		}
		reconcileCRSUpstreamBillingProbeExtra(existing, PlatformAnthropic, AccountTypeAPIKey, credentials, extra)

		if existing == nil {
			if !shouldCreateAccount(src.ID, selectedSet) {
				item.Action = "skipped"
				item.Error = "not selected"
				result.Skipped++
				result.Items = append(result.Items, item)
				continue
			}
			account := &Account{
				Name:        defaultName(src.Name, src.ID),
				Platform:    PlatformAnthropic,
				Type:        AccountTypeAPIKey,
				Credentials: credentials,
				Extra:       extra,
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

		// 母账号守卫(外审第9轮):CRS ID 跨平台碰撞时,本(Anthropic APIKey)分支不得改坏有 spark 影子的 OpenAI 母账号。
		if gerr := guardCRSShadowParentInvariant(ctx, s.accountRepo, existing, PlatformAnthropic, AccountTypeAPIKey); gerr != nil {
			item.Action = "failed"
			item.Error = gerr.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		existing.Extra = extra
		existing.Name = defaultName(src.Name, src.ID)
		existing.Platform = PlatformAnthropic
		existing.Type = AccountTypeAPIKey
		existing.Credentials = credentials
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
		var existingExtra map[string]any
		if existing != nil {
			existingExtra = existing.Extra
		}
		extra, err = mergeCRSOpenAILongContextBillingExtra(existingExtra, extra)
		if err != nil {
			item.Action = "failed"
			item.Error = err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		if existing != nil {
			credentials = mergeMap(existing.Credentials, credentials)
		}
		reconcileCRSUpstreamBillingProbeExtra(existing, PlatformOpenAI, AccountTypeOAuth, credentials, extra)

		if existing == nil {
			if !shouldCreateAccount(src.ID, selectedSet) {
				item.Action = "skipped"
				item.Error = "not selected"
				result.Skipped++
				result.Items = append(result.Items, item)
				continue
			}
			account := &Account{
				Name:        defaultName(src.Name, src.ID),
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Credentials: credentials,
				Extra:       extra,
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
				_ = persistAccountCredentials(ctx, s.accountRepo, account, refreshedCreds)
			}
			item.Action = "created"
			result.Created++
			result.Items = append(result.Items, item)
			continue
		}

		existing.Extra = extra
		existing.Name = defaultName(src.Name, src.ID)
		existing.Platform = PlatformOpenAI
		existing.Type = AccountTypeOAuth
		existing.Credentials = credentials
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
			_ = persistAccountCredentials(ctx, s.accountRepo, existing, refreshedCreds)
		}

		// 母账号 proxy 经 CRS 改动后同步到其 spark 影子,避免影子保留旧 proxy 出现出站漂移(外审第8轮)。
		// 影子 proxy 恒继承母账号(创建即继承、AdminService 编辑也传播)。best-effort:母账号本身已成功
		// 更新,影子传播失败仅记录告警,不回退该条目状态。
		if perr := propagateAccountProxyToShadows(ctx, s.accountRepo, existing.ID, existing.ProxyID); perr != nil {
			slog.Warn("crs_sync_propagate_proxy_to_shadows_failed", "account_id", existing.ID, "error", perr)
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

		extra := make(map[string]any, len(src.Extra)+3)
		for key, value := range src.Extra {
			extra[key] = value
		}
		extra["crs_account_id"] = src.ID
		extra["crs_kind"] = src.Kind
		extra["crs_synced_at"] = now

		existing, err := s.accountRepo.GetByCRSAccountID(ctx, src.ID)
		if err != nil {
			item.Action = "failed"
			item.Error = "db lookup failed: " + err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		var existingExtra map[string]any
		if existing != nil {
			existingExtra = existing.Extra
		}
		extra, err = mergeCRSOpenAILongContextBillingExtra(existingExtra, extra)
		if err != nil {
			item.Action = "failed"
			item.Error = err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		if existing != nil {
			credentials = mergeMap(existing.Credentials, credentials)
		}
		reconcileCRSUpstreamBillingProbeExtra(existing, PlatformOpenAI, AccountTypeAPIKey, credentials, extra)

		if existing == nil {
			if !shouldCreateAccount(src.ID, selectedSet) {
				item.Action = "skipped"
				item.Error = "not selected"
				result.Skipped++
				result.Items = append(result.Items, item)
				continue
			}
			account := &Account{
				Name:        defaultName(src.Name, src.ID),
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Credentials: credentials,
				Extra:       extra,
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

		// 母账号守卫(外审第8/9轮):CRS 不得把有 spark 影子的母账号改离 OpenAI OAuth(此处会翻成 api_key),
		// 否则影子读透母凭据失败、resolveCredentialAccount 必报错、spark 调度与用量刷新全崩。须先删影子再改。
		if gerr := guardCRSShadowParentInvariant(ctx, s.accountRepo, existing, PlatformOpenAI, AccountTypeAPIKey); gerr != nil {
			item.Action = "failed"
			item.Error = gerr.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		existing.Extra = extra
		existing.Name = defaultName(src.Name, src.ID)
		existing.Platform = PlatformOpenAI
		existing.Type = AccountTypeAPIKey
		existing.Credentials = credentials
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

	// Gemini OAuth -> sub2api gemini oauth
	for _, src := range exported.Data.GeminiOAuthAccounts {
		item := SyncFromCRSItemResult{
			CRSAccountID: src.ID,
			Kind:         src.Kind,
			Name:         src.Name,
		}

		refreshToken, _ := src.Credentials["refresh_token"].(string)
		if strings.TrimSpace(refreshToken) == "" {
			item.Action = "failed"
			item.Error = "missing refresh_token"
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
		if v, ok := credentials["token_type"].(string); !ok || strings.TrimSpace(v) == "" {
			credentials["token_type"] = "Bearer"
		}
		// Convert expires_at from RFC3339 to Unix seconds string (recommended to keep consistent with GetCredential())
		if expiresAtStr, ok := credentials["expires_at"].(string); ok && strings.TrimSpace(expiresAtStr) != "" {
			if t, err := time.Parse(time.RFC3339, expiresAtStr); err == nil {
				credentials["expires_at"] = strconv.FormatInt(t.Unix(), 10)
			}
		}

		extra := make(map[string]any)
		if src.Extra != nil {
			for k, v := range src.Extra {
				extra[k] = v
			}
		}
		extra["crs_account_id"] = src.ID
		extra["crs_kind"] = src.Kind
		extra["crs_synced_at"] = now

		existing, err := s.accountRepo.GetByCRSAccountID(ctx, src.ID)
		if err != nil {
			item.Action = "failed"
			item.Error = "db lookup failed: " + err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		if existing != nil {
			extra = mergeMap(existing.Extra, extra)
			credentials = mergeMap(existing.Credentials, credentials)
		}
		reconcileCRSUpstreamBillingProbeExtra(existing, PlatformGemini, AccountTypeOAuth, credentials, extra)

		if existing == nil {
			if !shouldCreateAccount(src.ID, selectedSet) {
				item.Action = "skipped"
				item.Error = "not selected"
				result.Skipped++
				result.Items = append(result.Items, item)
				continue
			}
			account := &Account{
				Name:        defaultName(src.Name, src.ID),
				Platform:    PlatformGemini,
				Type:        AccountTypeOAuth,
				Credentials: credentials,
				Extra:       extra,
				ProxyID:     proxyID,
				Concurrency: 3,
				Priority:    clampPriority(src.Priority),
				Status:      mapCRSStatus(src.IsActive, src.Status),
				Schedulable: src.Schedulable,
			}
			if err := s.accountRepo.Create(ctx, account); err != nil {
				item.Action = "failed"
				item.Error = "create failed: " + err.Error()
				result.Failed++
				result.Items = append(result.Items, item)
				continue
			}
			if refreshedCreds := s.refreshOAuthToken(ctx, account); refreshedCreds != nil {
				_ = persistAccountCredentials(ctx, s.accountRepo, account, refreshedCreds)
			}
			item.Action = "created"
			result.Created++
			result.Items = append(result.Items, item)
			continue
		}

		// 母账号守卫(外审第9轮):CRS ID 跨平台碰撞时,本(Gemini OAuth)分支不得改坏有 spark 影子的 OpenAI 母账号。
		if gerr := guardCRSShadowParentInvariant(ctx, s.accountRepo, existing, PlatformGemini, AccountTypeOAuth); gerr != nil {
			item.Action = "failed"
			item.Error = gerr.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		existing.Extra = extra
		existing.Name = defaultName(src.Name, src.ID)
		existing.Platform = PlatformGemini
		existing.Type = AccountTypeOAuth
		existing.Credentials = credentials
		if proxyID != nil {
			existing.ProxyID = proxyID
		}
		existing.Concurrency = 3
		existing.Priority = clampPriority(src.Priority)
		existing.Status = mapCRSStatus(src.IsActive, src.Status)
		existing.Schedulable = src.Schedulable

		if err := s.accountRepo.Update(ctx, existing); err != nil {
			item.Action = "failed"
			item.Error = "update failed: " + err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		if refreshedCreds := s.refreshOAuthToken(ctx, existing); refreshedCreds != nil {
			_ = persistAccountCredentials(ctx, s.accountRepo, existing, refreshedCreds)
		}

		item.Action = "updated"
		result.Updated++
		result.Items = append(result.Items, item)
	}

	// Gemini API Key -> sub2api gemini apikey
	for _, src := range exported.Data.GeminiAPIKeyAccounts {
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
		if baseURL, ok := credentials["base_url"].(string); !ok || strings.TrimSpace(baseURL) == "" {
			credentials["base_url"] = "https://generativelanguage.googleapis.com"
		}

		extra := make(map[string]any)
		if src.Extra != nil {
			for k, v := range src.Extra {
				extra[k] = v
			}
		}
		extra["crs_account_id"] = src.ID
		extra["crs_kind"] = src.Kind
		extra["crs_synced_at"] = now

		existing, err := s.accountRepo.GetByCRSAccountID(ctx, src.ID)
		if err != nil {
			item.Action = "failed"
			item.Error = "db lookup failed: " + err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		if existing != nil {
			extra = mergeMap(existing.Extra, extra)
			credentials = mergeMap(existing.Credentials, credentials)
		}
		reconcileCRSUpstreamBillingProbeExtra(existing, PlatformGemini, AccountTypeAPIKey, credentials, extra)

		if existing == nil {
			if !shouldCreateAccount(src.ID, selectedSet) {
				item.Action = "skipped"
				item.Error = "not selected"
				result.Skipped++
				result.Items = append(result.Items, item)
				continue
			}
			account := &Account{
				Name:        defaultName(src.Name, src.ID),
				Platform:    PlatformGemini,
				Type:        AccountTypeAPIKey,
				Credentials: credentials,
				Extra:       extra,
				ProxyID:     proxyID,
				Concurrency: 3,
				Priority:    clampPriority(src.Priority),
				Status:      mapCRSStatus(src.IsActive, src.Status),
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

		// 母账号守卫(外审第9轮):CRS ID 跨平台碰撞时,本(Gemini APIKey)分支不得改坏有 spark 影子的 OpenAI 母账号。
		if gerr := guardCRSShadowParentInvariant(ctx, s.accountRepo, existing, PlatformGemini, AccountTypeAPIKey); gerr != nil {
			item.Action = "failed"
			item.Error = gerr.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		existing.Extra = extra
		existing.Name = defaultName(src.Name, src.ID)
		existing.Platform = PlatformGemini
		existing.Type = AccountTypeAPIKey
		existing.Credentials = credentials
		if proxyID != nil {
			existing.ProxyID = proxyID
		}
		existing.Concurrency = 3
		existing.Priority = clampPriority(src.Priority)
		existing.Status = mapCRSStatus(src.IsActive, src.Status)
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

func mergeMap(existing map[string]any, updates map[string]any) map[string]any {
	out := make(map[string]any, len(existing)+len(updates))
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range updates {
		out[k] = v
	}
	return out
}

func reconcileCRSUpstreamBillingProbeExtra(
	existing *Account,
	targetPlatform, targetType string,
	targetCredentials map[string]any,
	extra map[string]any,
) {
	for _, key := range []string{
		UpstreamBillingProbeEnabledExtraKey,
		UpstreamBillingProbeExtraKey,
		OllamaCloudUsageSessionExtraKey,
		OllamaCloudUsageAutoRefreshExtraKey,
		OllamaCloudUsageSnapshotExtraKey,
	} {
		delete(extra, key)
	}
	if existing == nil {
		return
	}
	target := &Account{Platform: targetPlatform, Type: targetType, Credentials: targetCredentials}
	if targetPlatform == PlatformOpenAI && targetType == AccountTypeAPIKey {
		if enabled, ok := existing.Extra[UpstreamBillingProbeEnabledExtraKey]; ok {
			extra[UpstreamBillingProbeEnabledExtraKey] = enabled
		}
		if reflect.DeepEqual(upstreamBillingProbeIdentity(existing), upstreamBillingProbeIdentity(target)) {
			if snapshot, ok := existing.Extra[UpstreamBillingProbeExtraKey]; ok {
				extra[UpstreamBillingProbeExtraKey] = snapshot
			}
		}
	}
	if IsOllamaCloudUsageAccount(existing) && IsOllamaCloudUsageAccount(target) &&
		reflect.DeepEqual(ollamaCloudUsageIdentity(existing), ollamaCloudUsageIdentity(target)) {
		if session, ok := existing.Extra[OllamaCloudUsageSessionExtraKey]; ok {
			extra[OllamaCloudUsageSessionExtraKey] = session
		}
		if enabled, ok := existing.Extra[OllamaCloudUsageAutoRefreshExtraKey]; ok {
			extra[OllamaCloudUsageAutoRefreshExtraKey] = enabled
		}
		if snapshot, ok := existing.Extra[OllamaCloudUsageSnapshotExtraKey]; ok {
			extra[OllamaCloudUsageSnapshotExtraKey] = snapshot
		}
	}
}

func mergeCRSOpenAILongContextBillingExtra(existing, updates map[string]any) (map[string]any, error) {
	return normalizeOpenAILongContextBillingExtra(PlatformOpenAI, mergeMap(existing, updates))
}

func (s *CRSSyncService) mapOrCreateProxy(ctx context.Context, enabled bool, cached *[]Proxy, src *crsProxy, defaultName string) (*int64, error) {
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
	proxy := &Proxy{
		Name:     defaultProxyName(defaultName, protocol, host, port),
		Protocol: protocol,
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		Status:   StatusActive,
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

func normalizeBaseURL(raw string, allowlist []string, allowPrivate bool) (string, error) {
	// 当 allowlist 为空时，不强制要求白名单（只进行基本的 URL 和 SSRF 验证）
	requireAllowlist := len(allowlist) > 0
	normalized, err := urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     allowlist,
		RequireAllowlist: requireAllowlist,
		AllowPrivate:     allowPrivate,
	})
	if err != nil {
		return "", fmt.Errorf("invalid base_url: %w", err)
	}
	return normalized, nil
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
func (s *CRSSyncService) refreshOAuthToken(ctx context.Context, account *Account) map[string]any {
	if account.Type != AccountTypeOAuth {
		return nil
	}

	var newCredentials map[string]any
	var err error

	switch account.Platform {
	case PlatformAnthropic:
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
	case PlatformOpenAI:
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
			newCredentials = NormalizeOpenAIPersonalAccessTokenCredentials(account, tokenInfo, newCredentials)
		}
	case PlatformGemini:
		if s.geminiOAuthService == nil {
			return nil
		}
		tokenInfo, refreshErr := s.geminiOAuthService.RefreshAccountToken(ctx, account)
		if refreshErr != nil {
			err = refreshErr
		} else {
			newCredentials = s.geminiOAuthService.BuildAccountCredentials(tokenInfo)
			for k, v := range account.Credentials {
				if _, exists := newCredentials[k]; !exists {
					newCredentials[k] = v
				}
			}
		}
	default:
		return nil
	}

	if err != nil {
		// Log but don't fail the sync - token might still be valid or refreshable later
		return nil
	}

	return newCredentials
}

// buildSelectedSet converts a slice of selected CRS account IDs to a set for O(1) lookup.
// Returns nil if ids is nil (field not sent → backward compatible: create all).
// Returns an empty map if ids is non-nil but empty (user selected none → create none).
func buildSelectedSet(ids []string) map[string]struct{} {
	if ids == nil {
		return nil
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

// shouldCreateAccount checks if a new CRS account should be created based on user selection.
// Returns true if selectedSet is nil (backward compatible: create all) or if crsID is in the set.
func shouldCreateAccount(crsID string, selectedSet map[string]struct{}) bool {
	if selectedSet == nil {
		return true
	}
	_, ok := selectedSet[crsID]
	return ok
}

// PreviewFromCRSResult contains the preview of accounts from CRS before sync.
type PreviewFromCRSResult struct {
	NewAccounts      []CRSPreviewAccount `json:"new_accounts"`
	ExistingAccounts []CRSPreviewAccount `json:"existing_accounts"`
}

// CRSPreviewAccount represents a single account in the preview result.
type CRSPreviewAccount struct {
	CRSAccountID string `json:"crs_account_id"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Platform     string `json:"platform"`
	Type         string `json:"type"`
}

// PreviewFromCRS connects to CRS, fetches all accounts, and classifies them
// as new or existing by batch-querying local crs_account_id mappings.
func (s *CRSSyncService) PreviewFromCRS(ctx context.Context, input SyncFromCRSInput) (*PreviewFromCRSResult, error) {
	exported, err := s.fetchCRSExport(ctx, input.BaseURL, input.Username, input.Password)
	if err != nil {
		return nil, err
	}

	// Batch query all existing CRS account IDs
	existingCRSIDs, err := s.accountRepo.ListCRSAccountIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing CRS accounts: %w", err)
	}

	result := &PreviewFromCRSResult{
		NewAccounts:      make([]CRSPreviewAccount, 0),
		ExistingAccounts: make([]CRSPreviewAccount, 0),
	}

	classify := func(crsID, kind, name, platform, accountType string) {
		preview := CRSPreviewAccount{
			CRSAccountID: crsID,
			Kind:         kind,
			Name:         defaultName(name, crsID),
			Platform:     platform,
			Type:         accountType,
		}
		if _, exists := existingCRSIDs[crsID]; exists {
			result.ExistingAccounts = append(result.ExistingAccounts, preview)
		} else {
			result.NewAccounts = append(result.NewAccounts, preview)
		}
	}

	for _, src := range exported.Data.ClaudeAccounts {
		authType := strings.TrimSpace(src.AuthType)
		if authType == "" {
			authType = AccountTypeOAuth
		}
		classify(src.ID, src.Kind, src.Name, PlatformAnthropic, authType)
	}
	for _, src := range exported.Data.ClaudeConsoleAccounts {
		classify(src.ID, src.Kind, src.Name, PlatformAnthropic, AccountTypeAPIKey)
	}
	for _, src := range exported.Data.OpenAIOAuthAccounts {
		classify(src.ID, src.Kind, src.Name, PlatformOpenAI, AccountTypeOAuth)
	}
	for _, src := range exported.Data.OpenAIResponsesAccounts {
		classify(src.ID, src.Kind, src.Name, PlatformOpenAI, AccountTypeAPIKey)
	}
	for _, src := range exported.Data.GeminiOAuthAccounts {
		classify(src.ID, src.Kind, src.Name, PlatformGemini, AccountTypeOAuth)
	}
	for _, src := range exported.Data.GeminiAPIKeyAccounts {
		classify(src.ID, src.Kind, src.Name, PlatformGemini, AccountTypeAPIKey)
	}

	return result, nil
}
