package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"log/slog"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	dataType       = "sub2api-data"
	legacyDataType = "sub2api-bundle"
	dataVersion    = 1
	dataPageCap    = 1000

	dataImportDefaultSchedulable = false

	cockpitAccountTransferSchema = "cockpit-tools.account-transfer"
	cockpitDataTransferSchema    = "cockpit-tools.data-transfer"
)

type DataPayload struct {
	Type       string        `json:"type,omitempty"`
	Version    int           `json:"version,omitempty"`
	ExportedAt string        `json:"exported_at"`
	Proxies    []DataProxy   `json:"proxies"`
	Accounts   []DataAccount `json:"accounts"`
}

type dataImportRequestWire struct {
	Data                    json.RawMessage `json:"data"`
	SkipDefaultGroupBind    *bool           `json:"skip_default_group_bind"`
	BatchID                 *int64          `json:"batch_id"`
	GroupIDs                []int64         `json:"group_ids"`
	ProxyID                 *int64          `json:"proxy_id"`
	Concurrency             *int            `json:"concurrency"`
	Priority                *int            `json:"priority"`
	RateMultiplier          *float64        `json:"rate_multiplier"`
	LoadFactor              *int            `json:"load_factor"`
	ExpiresAt               *int64          `json:"expires_at"`
	AutoPauseOnExpired      *bool           `json:"auto_pause_on_expired"`
	Schedulable             *bool           `json:"schedulable"`
	CredentialExtras        map[string]any  `json:"credential_extras"`
	Extra                   map[string]any  `json:"extra"`
	AutoDetectModels        *bool           `json:"auto_detect_models"`
	ConfirmMixedChannelRisk *bool           `json:"confirm_mixed_channel_risk"`
}

type DataProxy struct {
	ProxyKey        string `json:"proxy_key"`
	Name            string `json:"name"`
	Protocol        string `json:"protocol"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Username        string `json:"username,omitempty"`
	Password        string `json:"password,omitempty"`
	Status          string `json:"status"`
	ExpiresAt       *int64 `json:"expires_at,omitempty"`        // unix 秒，与 DataAccount.ExpiresAt 风格一致
	FallbackMode    string `json:"fallback_mode,omitempty"`     // none/direct/proxy
	BackupProxyName string `json:"backup_proxy_name,omitempty"` // 备用代理 name（跨实例按 name 反查）
	ExpiryWarnDays  int    `json:"expiry_warn_days,omitempty"`
}

// DataAccount 是管理员显式备份导出使用的账号结构，故意不走 dto.Account 的脱敏路径，
// Credentials 原文返回。这是"管理员备份"这一显式行为的一部分；如未来需要导出脱敏版本，
// 应新增独立结构而非修改这里。
type DataAccount struct {
	Name               string         `json:"name"`
	Notes              *string        `json:"notes,omitempty"`
	Platform           string         `json:"platform"`
	Type               string         `json:"type"`
	Credentials        map[string]any `json:"credentials"`
	Extra              map[string]any `json:"extra,omitempty"`
	ProxyKey           *string        `json:"proxy_key,omitempty"`
	Concurrency        int            `json:"concurrency"`
	Priority           int            `json:"priority"`
	RateMultiplier     *float64       `json:"rate_multiplier,omitempty"`
	ExpiresAt          *int64         `json:"expires_at,omitempty"`
	AutoPauseOnExpired *bool          `json:"auto_pause_on_expired,omitempty"`
	LoadFactor         *int           `json:"load_factor,omitempty"`
}

type DataImportRequest struct {
	Data                    DataPayload       `json:"data"`
	SkipDefaultGroupBind    *bool             `json:"skip_default_group_bind"`
	BatchID                 *int64            `json:"batch_id"`
	GroupIDs                []int64           `json:"group_ids"`
	ProxyID                 *int64            `json:"proxy_id"`
	Concurrency             *int              `json:"concurrency"`
	Priority                *int              `json:"priority"`
	RateMultiplier          *float64          `json:"rate_multiplier"`
	LoadFactor              *int              `json:"load_factor"`
	ExpiresAt               *int64            `json:"expires_at"`
	AutoPauseOnExpired      *bool             `json:"auto_pause_on_expired"`
	Schedulable             *bool             `json:"schedulable"`
	CredentialExtras        map[string]any    `json:"credential_extras"`
	Extra                   map[string]any    `json:"extra"`
	AutoDetectModels        *bool             `json:"auto_detect_models"`
	ConfirmMixedChannelRisk *bool             `json:"confirm_mixed_channel_risk"`
	ConversionAccountFailed int               `json:"-"`
	ConversionErrors        []DataImportError `json:"-"`
}

type DataImportResult struct {
	ProxyCreated       int               `json:"proxy_created"`
	ProxyReused        int               `json:"proxy_reused"`
	ProxyFailed        int               `json:"proxy_failed"`
	AccountCreated     int               `json:"account_created"`
	AccountFailed      int               `json:"account_failed"`
	ModelSyncSucceeded int               `json:"model_sync_succeeded,omitempty"`
	ModelSyncFailed    int               `json:"model_sync_failed,omitempty"`
	Errors             []DataImportError `json:"errors,omitempty"`
}

type DataImportError struct {
	Kind     string `json:"kind"`
	Name     string `json:"name,omitempty"`
	ProxyKey string `json:"proxy_key,omitempty"`
	Message  string `json:"message"`
}

func buildProxyKey(protocol, host string, port int, username, password string) string {
	return fmt.Sprintf("%s|%s|%d|%s|%s", strings.TrimSpace(protocol), strings.TrimSpace(host), port, strings.TrimSpace(username), strings.TrimSpace(password))
}

func (h *AccountHandler) ExportData(c *gin.Context) {
	ctx := c.Request.Context()

	selectedIDs, err := parseAccountIDs(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	accounts, err := h.resolveExportAccounts(ctx, selectedIDs, c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	includeProxies, err := parseIncludeProxies(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	var proxies []service.Proxy
	if includeProxies {
		proxies, err = h.resolveExportProxies(ctx, accounts)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
	} else {
		proxies = []service.Proxy{}
	}

	// 构建 id→name 映射，用于导出备用代理 name
	proxyNameByID := make(map[int64]string, len(proxies))
	for i := range proxies {
		proxyNameByID[proxies[i].ID] = proxies[i].Name
	}

	proxyKeyByID := make(map[int64]string, len(proxies))
	dataProxies := make([]DataProxy, 0, len(proxies))
	for i := range proxies {
		p := proxies[i]
		key := buildProxyKey(p.Protocol, p.Host, p.Port, p.Username, p.Password)
		proxyKeyByID[p.ID] = key

		var expiresAt *int64
		if p.ExpiresAt != nil {
			v := p.ExpiresAt.Unix()
			expiresAt = &v
		}
		var backupProxyName string
		if p.BackupProxyID != nil {
			backupProxyName = proxyNameByID[*p.BackupProxyID]
		}
		dataProxies = append(dataProxies, DataProxy{
			ProxyKey:        key,
			Name:            p.Name,
			Protocol:        p.Protocol,
			Host:            p.Host,
			Port:            p.Port,
			Username:        p.Username,
			Password:        p.Password,
			Status:          p.Status,
			ExpiresAt:       expiresAt,
			FallbackMode:    p.FallbackMode,
			BackupProxyName: backupProxyName,
			ExpiryWarnDays:  p.ExpiryWarnDays,
		})
	}

	dataAccounts := make([]DataAccount, 0, len(accounts))
	for i := range accounts {
		acc := accounts[i]
		var proxyKey *string
		if acc.ProxyID != nil {
			if key, ok := proxyKeyByID[*acc.ProxyID]; ok {
				proxyKey = &key
			}
		}
		var expiresAt *int64
		if acc.ExpiresAt != nil {
			v := acc.ExpiresAt.Unix()
			expiresAt = &v
		}
		dataAccounts = append(dataAccounts, DataAccount{
			Name:               acc.Name,
			Notes:              acc.Notes,
			Platform:           acc.Platform,
			Type:               acc.Type,
			Credentials:        acc.Credentials,
			Extra:              acc.Extra,
			ProxyKey:           proxyKey,
			Concurrency:        acc.Concurrency,
			Priority:           acc.Priority,
			RateMultiplier:     acc.RateMultiplier,
			ExpiresAt:          expiresAt,
			AutoPauseOnExpired: &acc.AutoPauseOnExpired,
		})
	}

	payload := DataPayload{
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Proxies:    dataProxies,
		Accounts:   dataAccounts,
	}

	response.Success(c, payload)
}

func (h *AccountHandler) ImportData(c *gin.Context) {
	req, err := decodeDataImportRequest(c)
	if err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := validateDataHeader(req.Data); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := validateDataImportOptions(req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := executeAdminIdempotent(c, "admin.accounts.import_data", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.importData(ctx, req)
	})
	if err != nil {
		var mixedErr *service.MixedChannelError
		if errors.As(err, &mixedErr) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "mixed_channel_warning",
				"message": mixedErr.Error(),
			})
			return
		}
		if retryAfter := service.RetryAfterSecondsFromError(err); retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		response.ErrorFrom(c, err)
		return
	}
	if result != nil && result.Replayed {
		c.Header("X-Idempotency-Replayed", "true")
	}
	response.Success(c, result.Data)
}

func decodeDataImportRequest(c *gin.Context) (DataImportRequest, error) {
	var req DataImportRequest
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return req, err
	}

	var wire dataImportRequestWire
	if err := json.Unmarshal(body, &wire); err != nil {
		return req, err
	}
	if len(wire.Data) == 0 || string(wire.Data) == "null" {
		return req, errors.New("data is required")
	}

	payload, conversionErrors, err := normalizeDataImportPayload(wire.Data)
	if err != nil {
		return req, err
	}

	req = DataImportRequest{
		Data:                    payload,
		SkipDefaultGroupBind:    wire.SkipDefaultGroupBind,
		BatchID:                 wire.BatchID,
		GroupIDs:                wire.GroupIDs,
		ProxyID:                 wire.ProxyID,
		Concurrency:             wire.Concurrency,
		Priority:                wire.Priority,
		RateMultiplier:          wire.RateMultiplier,
		LoadFactor:              wire.LoadFactor,
		ExpiresAt:               wire.ExpiresAt,
		AutoPauseOnExpired:      wire.AutoPauseOnExpired,
		Schedulable:             wire.Schedulable,
		CredentialExtras:        wire.CredentialExtras,
		Extra:                   wire.Extra,
		AutoDetectModels:        wire.AutoDetectModels,
		ConfirmMixedChannelRisk: wire.ConfirmMixedChannelRisk,
		ConversionAccountFailed: len(conversionErrors),
		ConversionErrors:        conversionErrors,
	}
	return req, nil
}

func normalizeDataImportPayload(raw json.RawMessage) (DataPayload, []DataImportError, error) {
	var payload DataPayload
	var envelope struct {
		Schema string          `json:"schema"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return payload, nil, err
	}
	switch envelope.Schema {
	case cockpitAccountTransferSchema:
		return convertCockpitAccountTransferPayload(raw)
	case cockpitDataTransferSchema:
		if len(envelope.Data) > 0 && string(envelope.Data) != "null" {
			return convertCockpitAccountTransferPayload(envelope.Data)
		}
		var bundle struct {
			Accounts json.RawMessage `json:"accounts"`
		}
		if err := json.Unmarshal(raw, &bundle); err != nil {
			return payload, nil, err
		}
		if len(bundle.Accounts) == 0 || string(bundle.Accounts) == "null" {
			return payload, nil, errors.New("cockpit-tools data-transfer bundle does not include accounts")
		}
		return convertCockpitAccountTransferPayload(bundle.Accounts)
	default:
		if err := json.Unmarshal(raw, &payload); err != nil {
			return payload, nil, err
		}
		return payload, nil, nil
	}
}

func convertCockpitAccountTransferPayload(raw json.RawMessage) (DataPayload, []DataImportError, error) {
	var bundle struct {
		Schema     string                            `json:"schema"`
		Version    int                               `json:"version"`
		ExportedAt string                            `json:"exported_at"`
		Platforms  map[string]cockpitPlatformPayload `json:"platforms"`
	}
	var payload DataPayload
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return payload, nil, err
	}
	if bundle.Schema != cockpitAccountTransferSchema {
		return payload, nil, fmt.Errorf("unsupported cockpit-tools schema: %s", bundle.Schema)
	}
	if len(bundle.Platforms) == 0 {
		return payload, nil, errors.New("cockpit-tools account-transfer bundle has no platforms")
	}

	payload = DataPayload{
		Type:       dataType,
		Version:    dataVersion,
		ExportedAt: bundle.ExportedAt,
		Proxies:    []DataProxy{},
		Accounts:   []DataAccount{},
	}
	if payload.ExportedAt == "" {
		payload.ExportedAt = time.Now().UTC().Format(time.RFC3339)
	}

	var conversionErrors []DataImportError
	for platform, section := range bundle.Platforms {
		accounts, errs := convertCockpitPlatformAccounts(platform, section.ExportedData)
		payload.Accounts = append(payload.Accounts, accounts...)
		conversionErrors = append(conversionErrors, errs...)
	}
	return payload, conversionErrors, nil
}

type cockpitPlatformPayload struct {
	AccountCount int             `json:"account_count"`
	ExportedData json.RawMessage `json:"exported_data"`
	Data         json.RawMessage `json:"data"`
	Accounts     json.RawMessage `json:"accounts"`
}

func (p *cockpitPlatformPayload) UnmarshalJSON(data []byte) error {
	type alias cockpitPlatformPayload
	var wrapped alias
	if err := json.Unmarshal(data, &wrapped); err == nil && (wrapped.ExportedData != nil || wrapped.Data != nil || wrapped.Accounts != nil || wrapped.AccountCount > 0) {
		if len(wrapped.ExportedData) == 0 {
			if len(wrapped.Data) > 0 {
				wrapped.ExportedData = wrapped.Data
			} else if len(wrapped.Accounts) > 0 {
				wrapped.ExportedData = wrapped.Accounts
			}
		}
		*p = cockpitPlatformPayload(wrapped)
		return nil
	}
	p.ExportedData = append(p.ExportedData[:0], data...)
	return nil
}

func convertCockpitPlatformAccounts(platform string, raw json.RawMessage) ([]DataAccount, []DataImportError) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var values []map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		var single map[string]any
		if singleErr := json.Unmarshal(raw, &single); singleErr != nil {
			count := cockpitRawArrayLength(raw)
			if count == 0 {
				return nil, nil
			}
			if isKnownUnsupportedCockpitPlatform(platform) {
				return nil, []DataImportError{{
					Kind:    "account",
					Name:    platform,
					Message: fmt.Sprintf("cockpit-tools platform %q is an IDE/client session format and has no matching sub2api upstream platform", platform),
				}}
			}
			return nil, []DataImportError{{
				Kind:    "account",
				Name:    platform,
				Message: fmt.Sprintf("cockpit-tools %s export is not a supported account array", platform),
			}}
		}
		values = []map[string]any{single}
	}

	out := make([]DataAccount, 0, len(values))
	var errs []DataImportError
	for i, item := range values {
		account, err := convertCockpitAccount(platform, item, i+1)
		if err != nil {
			errs = append(errs, DataImportError{
				Kind:    "account",
				Name:    cockpitAccountName(platform, item, i+1),
				Message: err.Error(),
			})
			continue
		}
		out = append(out, account)
	}
	return out, errs
}

func cockpitRawArrayLength(raw json.RawMessage) int {
	var arr []any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return 0
	}
	return len(arr)
}

func isKnownUnsupportedCockpitPlatform(platform string) bool {
	switch normalizeCockpitPlatform(platform) {
	case "github-copilot", "windsurf", "kiro", "cursor", "zed", "codebuddy", "codebuddy_cn", "qoder", "trae", "workbuddy":
		return true
	default:
		return false
	}
}

func convertCockpitAccount(platform string, item map[string]any, index int) (DataAccount, error) {
	switch normalizeCockpitPlatform(platform) {
	case "antigravity", "antigravity_ide":
		return convertCockpitAntigravityAccount(item, index)
	case "codex":
		return convertCockpitCodexAccount(item, index)
	case "gemini":
		return convertCockpitGeminiAccount(item, index)
	case "github-copilot", "windsurf", "kiro", "cursor", "zed", "codebuddy", "codebuddy_cn", "qoder", "trae", "workbuddy":
		return DataAccount{}, fmt.Errorf("cockpit-tools platform %q is an IDE/client session format and has no matching sub2api upstream platform", platform)
	default:
		return DataAccount{}, fmt.Errorf("cockpit-tools platform %q is not supported by sub2api account import", platform)
	}
}

func convertCockpitAntigravityAccount(item map[string]any, index int) (DataAccount, error) {
	token := cockpitMapValue(item, "token")
	accessToken := cockpitString(token, "access_token")
	refreshToken := cockpitString(token, "refresh_token")
	if accessToken == "" {
		accessToken = cockpitString(item, "access_token")
	}
	if refreshToken == "" {
		refreshToken = cockpitString(item, "refresh_token")
	}
	if accessToken == "" && refreshToken == "" {
		return DataAccount{}, errors.New("cockpit-tools Antigravity account missing token.access_token/token.refresh_token")
	}

	credentials := map[string]any{}
	setImportString(credentials, "access_token", accessToken)
	setImportString(credentials, "refresh_token", refreshToken)
	setImportString(credentials, "token_type", firstImportString(cockpitString(token, "token_type"), cockpitString(item, "token_type")))
	setImportString(credentials, "email", firstImportString(cockpitString(item, "email"), cockpitString(token, "email")))
	setImportString(credentials, "project_id", firstImportString(cockpitString(token, "project_id"), cockpitString(item, "project_id")))
	setImportString(credentials, "plan_type", cockpitString(item, "plan_type"))
	setImportString(credentials, "expires_at", cockpitExpiresAtString(cockpitAny(token, "expiry_timestamp"), cockpitAny(token, "expires_at"), cockpitAny(item, "expires_at")))

	extra := cockpitImportExtra("cockpit-tools", "antigravity", item)
	return DataAccount{
		Name:        cockpitAccountName("antigravity", item, index),
		Notes:       cockpitNotes(item),
		Platform:    service.PlatformAntigravity,
		Type:        service.AccountTypeOAuth,
		Credentials: credentials,
		Extra:       extra,
		Concurrency: 3,
		Priority:    50,
	}, nil
}

func convertCockpitCodexAccount(item map[string]any, index int) (DataAccount, error) {
	authMode := strings.ToLower(firstImportString(cockpitString(item, "auth_mode"), cockpitString(item, "authMode")))
	if authMode == service.AccountTypeAPIKey || authMode == "api-key" || authMode == "apikey" || strings.TrimSpace(cockpitString(item, "openai_api_key")) != "" {
		apiKey := firstImportString(cockpitString(item, "openai_api_key"), cockpitString(item, "OPENAI_API_KEY"), cockpitString(item, "api_key"))
		if apiKey == "" {
			return DataAccount{}, errors.New("cockpit-tools Codex API key account missing openai_api_key")
		}
		credentials := map[string]any{
			"api_key":  apiKey,
			"base_url": firstImportString(cockpitString(item, "api_base_url"), "https://api.openai.com"),
		}
		setImportString(credentials, "provider_id", cockpitString(item, "api_provider_id"))
		setImportString(credentials, "provider_name", cockpitString(item, "api_provider_name"))
		return DataAccount{
			Name:        cockpitAccountName("codex", item, index),
			Notes:       cockpitNotes(item),
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Credentials: credentials,
			Extra:       cockpitImportExtra("cockpit-tools", "codex", item),
			Concurrency: 3,
			Priority:    50,
		}, nil
	}

	tokens := cockpitMapValue(item, "tokens")
	accessToken := firstImportString(cockpitString(tokens, "access_token"), cockpitString(item, "access_token"))
	if accessToken == "" {
		return DataAccount{}, errors.New("cockpit-tools Codex OAuth account missing tokens.access_token")
	}
	refreshToken := firstImportString(cockpitString(tokens, "refresh_token"), cockpitString(item, "refresh_token"))
	credentials := map[string]any{
		"access_token": accessToken,
	}
	setImportString(credentials, "refresh_token", refreshToken)
	setImportString(credentials, "id_token", firstImportString(cockpitString(tokens, "id_token"), cockpitString(item, "id_token")))
	setImportString(credentials, "email", cockpitString(item, "email"))
	setImportString(credentials, "chatgpt_account_id", firstImportString(cockpitString(item, "account_id"), cockpitString(item, "chatgpt_account_id")))
	setImportString(credentials, "chatgpt_user_id", firstImportString(cockpitString(item, "user_id"), cockpitString(item, "chatgpt_user_id")))
	setImportString(credentials, "organization_id", cockpitString(item, "organization_id"))
	setImportString(credentials, "plan_type", cockpitString(item, "plan_type"))
	if refreshToken != "" {
		setImportString(credentials, "client_id", firstImportString(
			cockpitString(tokens, "client_id"),
			cockpitString(tokens, "clientId"),
			cockpitString(tokens, "clientID"),
			cockpitString(item, "client_id"),
			cockpitString(item, "clientId"),
			cockpitString(item, "clientID"),
			openai.ClientID,
		))
	}

	extra := cockpitImportExtra("cockpit-tools", "codex", item)
	setImportString(extra, "account_name", cockpitString(item, "account_name"))
	setImportString(extra, "account_structure", cockpitString(item, "account_structure"))
	var expiresAt *int64
	var autoPauseOnExpired *bool
	if tokenExpiresAt, ok, err := cockpitCodexTokenExpiresAt(tokens, item, accessToken); err != nil {
		return DataAccount{}, err
	} else if ok {
		credentials["expires_at"] = tokenExpiresAt.Format(time.RFC3339)
		if refreshToken == "" {
			if tokenExpiresAt.Unix() <= time.Now().UTC().Unix()-codexImportClockSkewSeconds {
				return DataAccount{}, fmt.Errorf("cockpit-tools Codex OAuth access_token expired at %s", tokenExpiresAt.Format(time.RFC3339))
			}
			v := tokenExpiresAt.Unix()
			expiresAt = &v
			autoPause := true
			autoPauseOnExpired = &autoPause
		}
	} else if refreshToken == "" {
		return DataAccount{}, errors.New("cockpit-tools Codex OAuth account missing refresh_token and access token expiry")
	}
	return DataAccount{
		Name:               cockpitAccountName("codex", item, index),
		Notes:              cockpitNotes(item),
		Platform:           service.PlatformOpenAI,
		Type:               service.AccountTypeOAuth,
		Credentials:        credentials,
		Extra:              extra,
		Concurrency:        3,
		Priority:           50,
		ExpiresAt:          expiresAt,
		AutoPauseOnExpired: autoPauseOnExpired,
	}, nil
}

func convertCockpitGeminiAccount(item map[string]any, index int) (DataAccount, error) {
	authMode := strings.ToLower(firstImportString(
		cockpitString(item, "auth_mode"),
		cockpitString(item, "authMode"),
		cockpitString(item, "selected_auth_type"),
		cockpitString(item, "selectedAuthType"),
	))
	apiKey := firstImportString(
		cockpitString(item, "gemini_api_key"),
		cockpitString(item, "GEMINI_API_KEY"),
		cockpitString(item, "google_api_key"),
		cockpitString(item, "GOOGLE_API_KEY"),
		cockpitString(item, "api_key"),
		cockpitString(item, "API_KEY"),
	)
	if authMode == service.AccountTypeAPIKey || authMode == "api-key" || authMode == "apikey" || apiKey != "" {
		if apiKey == "" {
			return DataAccount{}, errors.New("cockpit-tools Gemini API key account missing gemini_api_key")
		}
		baseURL := firstImportString(
			cockpitString(item, "api_base_url"),
			cockpitString(item, "base_url"),
			cockpitString(item, "gemini_api_base_url"),
			cockpitString(item, "GOOGLE_API_BASE_URL"),
			"https://generativelanguage.googleapis.com",
		)
		credentials := map[string]any{
			"api_key":  apiKey,
			"base_url": baseURL,
			"tier_id":  service.GeminiTierAIStudioFree,
		}
		setImportString(credentials, "provider_id", cockpitString(item, "api_provider_id"))
		setImportString(credentials, "provider_name", cockpitString(item, "api_provider_name"))
		if !service.IsOfficialGeminiBaseURL(baseURL) {
			credentials["tier_id"] = service.GeminiUpstreamCompatibleRelay
			credentials["upstream_type"] = service.GeminiUpstreamCompatibleRelay
		}
		return DataAccount{
			Name:        cockpitAccountName("gemini", item, index),
			Notes:       cockpitNotes(item),
			Platform:    service.PlatformGemini,
			Type:        service.AccountTypeAPIKey,
			Credentials: credentials,
			Extra:       cockpitImportExtra("cockpit-tools", "gemini", item),
			Concurrency: 3,
			Priority:    50,
		}, nil
	}

	accessToken := cockpitString(item, "access_token")
	refreshToken := cockpitString(item, "refresh_token")
	if accessToken == "" && refreshToken == "" {
		return DataAccount{}, errors.New("cockpit-tools Gemini account missing access_token/refresh_token")
	}
	credentials := map[string]any{}
	setImportString(credentials, "access_token", accessToken)
	setImportString(credentials, "refresh_token", refreshToken)
	setImportString(credentials, "id_token", cockpitString(item, "id_token"))
	setImportString(credentials, "token_type", cockpitString(item, "token_type"))
	setImportString(credentials, "scope", cockpitString(item, "scope"))
	setImportString(credentials, "email", cockpitString(item, "email"))
	setImportString(credentials, "project_id", cockpitString(item, "project_id"))
	setImportString(credentials, "tier_id", firstImportString(cockpitString(item, "tier_id"), cockpitString(item, "plan_name")))
	setImportString(credentials, "oauth_type", cockpitGeminiOAuthType(item))
	setImportString(credentials, "expires_at", cockpitExpiresAtString(cockpitAny(item, "expiry_date"), cockpitAny(item, "expires_at")))

	return DataAccount{
		Name:        cockpitAccountName("gemini", item, index),
		Notes:       cockpitNotes(item),
		Platform:    service.PlatformGemini,
		Type:        service.AccountTypeOAuth,
		Credentials: credentials,
		Extra:       cockpitImportExtra("cockpit-tools", "gemini", item),
		Concurrency: 3,
		Priority:    50,
	}, nil
}

func normalizeCockpitPlatform(platform string) string {
	return strings.ToLower(strings.TrimSpace(platform))
}

func cockpitMapValue(item map[string]any, key string) map[string]any {
	if item == nil {
		return nil
	}
	if value, ok := item[key].(map[string]any); ok {
		return value
	}
	return nil
}

func cockpitAny(item map[string]any, key string) any {
	if item == nil {
		return nil
	}
	return item[key]
}

func cockpitString(item map[string]any, key string) string {
	if item == nil {
		return ""
	}
	return importStringValue(item[key])
}

func importStringValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return strings.TrimSpace(v.String())
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(v, 'f', -1, 64))
	case float32:
		return strings.TrimSpace(strconv.FormatFloat(float64(v), 'f', -1, 32))
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	default:
		return ""
	}
}

func firstImportString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func setImportString(target map[string]any, key, value string) {
	if target == nil {
		return
	}
	value = strings.TrimSpace(value)
	if value != "" {
		target[key] = value
	}
}

func cockpitNotes(item map[string]any) *string {
	note := firstImportString(cockpitString(item, "notes"), cockpitString(item, "note"), cockpitString(item, "account_note"))
	if note == "" {
		return nil
	}
	return &note
}

func cockpitAccountName(platform string, item map[string]any, index int) string {
	for _, value := range []string{
		cockpitString(item, "name"),
		cockpitString(item, "email"),
		cockpitString(item, "account_name"),
		cockpitString(item, "github_login"),
		cockpitString(item, "id"),
	} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return fmt.Sprintf("%s imported account %d", platform, index)
}

func cockpitImportExtra(source, platform string, item map[string]any) map[string]any {
	extra := map[string]any{
		"import_source":   source,
		"source_platform": platform,
	}
	setImportString(extra, "source_account_id", cockpitString(item, "id"))
	if tags, ok := item["tags"]; ok {
		extra["source_tags"] = tags
	}
	if disabled, ok := item["disabled"].(bool); ok && disabled {
		extra["source_disabled"] = true
		setImportString(extra, "source_disabled_reason", cockpitString(item, "disabled_reason"))
	}
	return extra
}

func cockpitExpiresAtString(values ...any) string {
	for _, value := range values {
		if parsed, ok := cockpitTimeValue(value); ok {
			return parsed.Format(time.RFC3339)
		}
		if text := importStringValue(value); text != "" {
			return text
		}
	}
	return ""
}

func cockpitFirstTimeValue(values ...any) (time.Time, bool) {
	for _, value := range values {
		if parsed, ok := cockpitTimeValue(value); ok {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func cockpitCodexTokenExpiresAt(tokens, item map[string]any, accessToken string) (time.Time, bool, error) {
	if parsed, ok := cockpitFirstTimeValue(
		cockpitAny(tokens, "expires_at"),
		cockpitAny(tokens, "expiresAt"),
		cockpitAny(tokens, "expiry_timestamp"),
		cockpitAny(tokens, "expiryTimestamp"),
		cockpitAny(tokens, "expiry_date"),
		cockpitAny(tokens, "expiryDate"),
		cockpitAny(item, "expires_at"),
		cockpitAny(item, "expiresAt"),
		cockpitAny(item, "expiry_timestamp"),
		cockpitAny(item, "expiryTimestamp"),
		cockpitAny(item, "expiry_date"),
		cockpitAny(item, "expiryDate"),
	); ok {
		return parsed.UTC(), true, nil
	}

	claims, err := decodeCodexJWTClaims(accessToken)
	if err != nil || claims.Exp <= 0 {
		return time.Time{}, false, nil
	}
	return time.Unix(claims.Exp, 0).UTC(), true, nil
}

func cockpitTimeValue(value any) (time.Time, bool) {
	switch v := value.(type) {
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return cockpitUnixTime(i), true
		}
	case float64:
		return cockpitUnixTime(int64(v)), true
	case int64:
		return cockpitUnixTime(v), true
	case int:
		return cockpitUnixTime(int64(v)), true
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return time.Time{}, false
		}
		if parsed, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return parsed.UTC(), true
		}
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return cockpitUnixTime(i), true
		}
	}
	return time.Time{}, false
}

func cockpitUnixTime(value int64) time.Time {
	if value > 1_000_000_000_000 {
		return time.UnixMilli(value).UTC()
	}
	return time.Unix(value, 0).UTC()
}

func cockpitGeminiOAuthType(item map[string]any) string {
	selected := strings.ToLower(firstImportString(cockpitString(item, "selected_auth_type"), cockpitString(item, "selectedAuthType")))
	if strings.Contains(selected, "studio") {
		return "ai_studio"
	}
	if strings.Contains(selected, "personal") || strings.Contains(selected, "google") || strings.Contains(selected, "one") {
		return "google_one"
	}
	if strings.TrimSpace(cockpitString(item, "project_id")) != "" {
		return "code_assist"
	}
	return "google_one"
}

func (h *AccountHandler) importData(ctx context.Context, req DataImportRequest) (DataImportResult, error) {
	skipDefaultGroupBind := true
	if req.SkipDefaultGroupBind != nil {
		skipDefaultGroupBind = *req.SkipDefaultGroupBind
	}
	skipMixedChannelCheck := req.ConfirmMixedChannelRisk != nil && *req.ConfirmMixedChannelRisk
	autoDetectModels := req.AutoDetectModels != nil && *req.AutoDetectModels

	dataPayload := req.Data
	result := DataImportResult{
		AccountFailed: req.ConversionAccountFailed,
		Errors:        append([]DataImportError(nil), req.ConversionErrors...),
	}

	if len(req.GroupIDs) > 0 && !skipMixedChannelCheck {
		if err := h.checkDataImportMixedChannelRisk(ctx, dataPayload.Accounts, req.GroupIDs); err != nil {
			return result, err
		}
	}

	existingProxies, err := h.listAllProxies(ctx)
	if err != nil {
		return result, err
	}

	proxyKeyToID := make(map[string]int64, len(existingProxies))
	// proxyNameToID 用于 backup_proxy_name 反查：DB 已有 + 本批次新建均会写入
	proxyNameToID := make(map[string]int64, len(existingProxies))
	for i := range existingProxies {
		p := existingProxies[i]
		key := buildProxyKey(p.Protocol, p.Host, p.Port, p.Username, p.Password)
		proxyKeyToID[key] = p.ID
		if p.Name != "" {
			proxyNameToID[p.Name] = p.ID
		}
	}

	for i := range dataPayload.Proxies {
		item := dataPayload.Proxies[i]
		key := item.ProxyKey
		if key == "" {
			key = buildProxyKey(item.Protocol, item.Host, item.Port, item.Username, item.Password)
		}
		if err := validateDataProxy(item); err != nil {
			result.ProxyFailed++
			result.Errors = append(result.Errors, DataImportError{
				Kind:     "proxy",
				Name:     item.Name,
				ProxyKey: key,
				Message:  err.Error(),
			})
			continue
		}
		normalizedStatus := normalizeProxyStatus(item.Status)
		if existingID, ok := proxyKeyToID[key]; ok {
			proxyKeyToID[key] = existingID
			result.ProxyReused++
			if normalizedStatus != "" {
				if proxy, getErr := h.adminService.GetProxy(ctx, existingID); getErr == nil && proxy != nil && proxy.Status != normalizedStatus {
					// 同步 status 时传入完整字段，避免零值覆盖已存在代理的有效期/fallback 配置。
					var existingExpiresAt *time.Time
					if item.ExpiresAt != nil {
						t := time.Unix(*item.ExpiresAt, 0).UTC()
						existingExpiresAt = &t
					}
					existingFallbackMode := item.FallbackMode
					if existingFallbackMode == "" {
						existingFallbackMode = service.FallbackModeNone
					}
					var existingBackupProxyID *int64
					if item.BackupProxyName != "" {
						if bid, ok := proxyNameToID[item.BackupProxyName]; ok {
							existingBackupProxyID = &bid
						}
					}
					_, _ = h.adminService.UpdateProxy(ctx, existingID, &service.UpdateProxyInput{
						Status:         normalizedStatus,
						ExpiresAt:      existingExpiresAt,
						FallbackMode:   existingFallbackMode,
						BackupProxyID:  existingBackupProxyID,
						ExpiryWarnDays: item.ExpiryWarnDays,
						Name:           proxy.Name,
						Protocol:       proxy.Protocol,
						Host:           proxy.Host,
						Port:           proxy.Port,
						Username:       proxy.Username,
						Password:       proxy.Password,
					})
				}
			}
			continue
		}

		// 解析 expires_at（unix 秒 → *time.Time）
		var expiresAt *time.Time
		if item.ExpiresAt != nil {
			t := time.Unix(*item.ExpiresAt, 0).UTC()
			expiresAt = &t
		}

		// 解析 backup_proxy_name → backup_proxy_id
		fallbackMode := item.FallbackMode
		var backupProxyID *int64
		if item.BackupProxyName != "" {
			if bid, ok := proxyNameToID[item.BackupProxyName]; ok {
				backupProxyID = &bid
			} else {
				// 查不到备用代理：降级 fallback_mode=none，记录 warning
				fallbackMode = service.FallbackModeNone
				result.Errors = append(result.Errors, DataImportError{
					Kind:     "proxy",
					Name:     item.Name,
					ProxyKey: key,
					Message:  fmt.Sprintf("backup_proxy_name %q not found, fallback_mode downgraded to none", item.BackupProxyName),
				})
			}
		}

		created, createErr := h.adminService.CreateProxy(ctx, &service.CreateProxyInput{
			Name:           defaultProxyName(item.Name),
			Protocol:       item.Protocol,
			Host:           item.Host,
			Port:           item.Port,
			Username:       item.Username,
			Password:       item.Password,
			ExpiresAt:      expiresAt,
			FallbackMode:   fallbackMode,
			BackupProxyID:  backupProxyID,
			ExpiryWarnDays: item.ExpiryWarnDays,
		})
		if createErr != nil {
			result.ProxyFailed++
			result.Errors = append(result.Errors, DataImportError{
				Kind:     "proxy",
				Name:     item.Name,
				ProxyKey: key,
				Message:  createErr.Error(),
			})
			continue
		}
		proxyKeyToID[key] = created.ID
		// 把新建代理的 name 也加入反查表，供后续批内代理引用
		if created.Name != "" {
			proxyNameToID[created.Name] = created.ID
		}
		result.ProxyCreated++

		if normalizedStatus != "" && normalizedStatus != created.Status {
			// 新建后同步 status 时，传入完整字段，避免零值覆盖刚创建的有效期/fallback 配置。
			_, _ = h.adminService.UpdateProxy(ctx, created.ID, &service.UpdateProxyInput{
				Status:         normalizedStatus,
				ExpiresAt:      expiresAt,
				FallbackMode:   fallbackMode,
				BackupProxyID:  backupProxyID,
				ExpiryWarnDays: item.ExpiryWarnDays,
				Name:           created.Name,
				Protocol:       created.Protocol,
				Host:           created.Host,
				Port:           created.Port,
				Username:       created.Username,
				Password:       created.Password,
			})
		}
	}

	// 收集需要异步设置隐私的 Antigravity OAuth 账号
	var privacyAccounts []*service.Account
	existingCredentialIndex, indexErr := h.buildExistingImportOAuthCredentialIndex(ctx)
	if indexErr != nil {
		slog.Warn("import_account_credential_index_failed", "error", indexErr)
		existingCredentialIndex = map[string]importOAuthCredentialRef{}
	}
	seenImportCredentials := make(map[string]string)

	for i := range dataPayload.Accounts {
		item := dataPayload.Accounts[i]
		if err := validateDataAccount(item); err != nil {
			result.AccountFailed++
			result.Errors = append(result.Errors, DataImportError{
				Kind:    "account",
				Name:    item.Name,
				Message: err.Error(),
			})
			continue
		}

		var proxyID *int64
		if req.ProxyID != nil {
			proxyID = normalizeImportProxyID(req.ProxyID)
		} else if item.ProxyKey != nil && *item.ProxyKey != "" {
			if id, ok := proxyKeyToID[*item.ProxyKey]; ok {
				proxyID = &id
			} else {
				result.AccountFailed++
				result.Errors = append(result.Errors, DataImportError{
					Kind:     "account",
					Name:     item.Name,
					ProxyKey: *item.ProxyKey,
					Message:  "proxy_key not found",
				})
				continue
			}
		}

		enrichCredentialsFromIDToken(&item)
		mergedCredentials := mergeImportMaps(item.Credentials, req.CredentialExtras)
		accountType := service.NormalizeGeminiAPIKeyAccountType(item.Platform, item.Type, mergedCredentials)
		credentials := service.NormalizeGeminiAPIKeyCredentials(
			item.Platform,
			accountType,
			mergedCredentials,
		)
		extra := mergeImportMaps(item.Extra, req.Extra)
		if warning := accessTokenOnlyOAuthImportWarning(item.Platform, accountType, credentials); warning != "" {
			result.Errors = append(result.Errors, DataImportError{
				Kind:    "account_warning",
				Name:    item.Name,
				Message: warning,
			})
		}
		if fingerprint, tokenKind := importOAuthCredentialFingerprint(item.Platform, accountType, credentials); fingerprint != "" {
			if existing, ok := existingCredentialIndex[fingerprint]; ok {
				result.AccountFailed++
				result.Errors = append(result.Errors, DataImportError{
					Kind: "account",
					Name: item.Name,
					Message: fmt.Sprintf(
						"duplicate OAuth %s already exists on account #%d (%s); skipped to avoid refresh-token rotation conflicts",
						tokenKind,
						existing.ID,
						existing.Name,
					),
				})
				continue
			}
			if previousName, ok := seenImportCredentials[fingerprint]; ok {
				result.AccountFailed++
				result.Errors = append(result.Errors, DataImportError{
					Kind: "account",
					Name: item.Name,
					Message: fmt.Sprintf(
						"duplicate OAuth %s in this import payload; already used by %q, skipped to avoid refresh-token rotation conflicts",
						tokenKind,
						previousName,
					),
				})
				continue
			}
			seenImportCredentials[fingerprint] = item.Name
		}

		accountInput := &service.CreateAccountInput{
			Name:                  item.Name,
			Notes:                 item.Notes,
			Platform:              item.Platform,
			Type:                  accountType,
			Credentials:           credentials,
			Extra:                 extra,
			ProxyID:               proxyID,
			Concurrency:           resolveImportInt(req.Concurrency, item.Concurrency),
			Priority:              resolveImportInt(req.Priority, item.Priority),
			RateMultiplier:        resolveImportFloat(req.RateMultiplier, item.RateMultiplier),
			GroupIDs:              append([]int64(nil), req.GroupIDs...),
			BatchID:               req.BatchID,
			ExpiresAt:             resolveImportInt64(req.ExpiresAt, item.ExpiresAt),
			AutoPauseOnExpired:    resolveImportBool(req.AutoPauseOnExpired, item.AutoPauseOnExpired),
			Schedulable:           resolveImportSchedulable(req.Schedulable),
			SkipDefaultGroupBind:  skipDefaultGroupBind,
			SkipMixedChannelCheck: skipMixedChannelCheck,
			LoadFactor:            resolveImportIntPtr(req.LoadFactor, item.LoadFactor),
		}

		created, err := h.adminService.CreateAccount(ctx, accountInput)
		if err != nil {
			result.AccountFailed++
			result.Errors = append(result.Errors, DataImportError{
				Kind:    "account",
				Name:    item.Name,
				Message: err.Error(),
			})
			continue
		}
		// 收集 Antigravity OAuth 账号，稍后异步设置隐私
		if created.Platform == service.PlatformAntigravity && created.Type == service.AccountTypeOAuth {
			privacyAccounts = append(privacyAccounts, created)
		}
		result.AccountCreated++

		if autoDetectModels {
			if err := h.syncImportedAccountModels(ctx, created); err != nil {
				result.ModelSyncFailed++
				result.Errors = append(result.Errors, DataImportError{
					Kind:    "model_sync",
					Name:    item.Name,
					Message: err.Error(),
				})
			} else {
				result.ModelSyncSucceeded++
			}
		}
	}

	// 异步设置 Antigravity 隐私，避免大量导入时阻塞请求
	if len(privacyAccounts) > 0 {
		adminSvc := h.adminService
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("import_antigravity_privacy_panic", "recover", r)
				}
			}()
			bgCtx := context.Background()
			for _, acc := range privacyAccounts {
				adminSvc.ForceAntigravityPrivacy(bgCtx, acc)
			}
			slog.Info("import_antigravity_privacy_done", "count", len(privacyAccounts))
		}()
	}

	return result, nil
}

func (h *AccountHandler) syncImportedAccountModels(ctx context.Context, account *service.Account) error {
	if h.accountTestService == nil {
		return errors.New("account model detection service is not configured")
	}
	if account == nil {
		return errors.New("account is required")
	}

	syncAccount := account
	if loaded, err := h.adminService.GetAccount(ctx, account.ID); err == nil && loaded != nil {
		if strings.TrimSpace(loaded.Platform) != "" {
			syncAccount = loaded
		}
	}

	models, err := h.accountTestService.FetchUpstreamSupportedModels(ctx, syncAccount)
	if err != nil {
		var syncErr *service.UpstreamModelSyncError
		if errors.As(err, &syncErr) {
			return errors.New(syncErr.SafeMessage())
		}
		return err
	}

	credentials := withModelMapping(syncAccount.Credentials, models)
	if _, err := h.adminService.UpdateAccount(ctx, syncAccount.ID, &service.UpdateAccountInput{
		Credentials: credentials,
	}); err != nil {
		return err
	}
	return nil
}

func withModelMapping(credentials map[string]any, models []string) map[string]any {
	out := make(map[string]any, len(credentials)+1)
	for key, value := range credentials {
		out[key] = value
	}

	modelMapping := make(map[string]any, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		modelMapping[model] = model
	}
	if len(modelMapping) == 0 {
		delete(out, "model_mapping")
		return out
	}
	out["model_mapping"] = modelMapping
	return out
}

func (h *AccountHandler) checkDataImportMixedChannelRisk(ctx context.Context, accounts []DataAccount, groupIDs []int64) error {
	seenPlatforms := make(map[string]struct{})
	for i := range accounts {
		platform := strings.TrimSpace(accounts[i].Platform)
		if platform == "" {
			continue
		}
		if _, ok := seenPlatforms[platform]; ok {
			continue
		}
		seenPlatforms[platform] = struct{}{}
		if err := h.adminService.CheckMixedChannelRisk(ctx, 0, platform, groupIDs); err != nil {
			return err
		}
	}
	return nil
}

func validateDataImportOptions(req DataImportRequest) error {
	if req.BatchID != nil && *req.BatchID <= 0 {
		return errors.New("batch_id is invalid")
	}
	if req.ProxyID != nil && *req.ProxyID < 0 {
		return errors.New("proxy_id is invalid")
	}
	if req.Concurrency != nil && *req.Concurrency < 0 {
		return errors.New("concurrency must be >= 0")
	}
	if req.Priority != nil && *req.Priority < 0 {
		return errors.New("priority must be >= 0")
	}
	if req.RateMultiplier != nil && *req.RateMultiplier < 0 {
		return errors.New("rate_multiplier must be >= 0")
	}
	if req.LoadFactor != nil && *req.LoadFactor > 10000 {
		return errors.New("load_factor must be <= 10000")
	}
	for _, groupID := range req.GroupIDs {
		if groupID <= 0 {
			return errors.New("group_ids contains invalid group id")
		}
	}
	return nil
}

func normalizeImportProxyID(proxyID *int64) *int64 {
	if proxyID == nil || *proxyID <= 0 {
		return nil
	}
	value := *proxyID
	return &value
}

func resolveImportInt(override *int, fallback int) int {
	if override != nil {
		return *override
	}
	return fallback
}

func resolveImportIntPtr(override *int, fallback *int) *int {
	if override != nil {
		value := *override
		return &value
	}
	return fallback
}

func resolveImportInt64(override *int64, fallback *int64) *int64 {
	if override != nil {
		value := *override
		return &value
	}
	return fallback
}

func resolveImportFloat(override *float64, fallback *float64) *float64 {
	if override != nil {
		value := *override
		return &value
	}
	return fallback
}

func resolveImportBool(override *bool, fallback *bool) *bool {
	if override != nil {
		value := *override
		return &value
	}
	return fallback
}

func resolveImportSchedulable(override *bool) *bool {
	if override != nil {
		value := *override
		return &value
	}
	value := dataImportDefaultSchedulable
	return &value
}

func mergeImportMaps(base map[string]any, override map[string]any) map[string]any {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := make(map[string]any, len(base)+len(override))
	for key, value := range base {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" || value == nil {
			continue
		}
		merged[trimmedKey] = value
	}
	for key, value := range override {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" || value == nil {
			continue
		}
		merged[trimmedKey] = value
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

type importOAuthCredentialRef struct {
	ID   int64
	Name string
}

func (h *AccountHandler) buildExistingImportOAuthCredentialIndex(ctx context.Context) (map[string]importOAuthCredentialRef, error) {
	accounts, err := h.listAccountsFiltered(ctx, "", "", "", "", 0, "", "id", "asc")
	if err != nil {
		return nil, err
	}

	index := make(map[string]importOAuthCredentialRef)
	for i := range accounts {
		fingerprint, _ := importOAuthCredentialFingerprint(accounts[i].Platform, accounts[i].Type, accounts[i].Credentials)
		if fingerprint == "" {
			continue
		}
		if _, exists := index[fingerprint]; exists {
			continue
		}
		index[fingerprint] = importOAuthCredentialRef{
			ID:   accounts[i].ID,
			Name: accounts[i].Name,
		}
	}
	return index, nil
}

func importOAuthCredentialFingerprint(platform, accountType string, credentials map[string]any) (string, string) {
	if strings.TrimSpace(accountType) != service.AccountTypeOAuth {
		return "", ""
	}
	refreshToken := importStringValue(credentials["refresh_token"])
	if refreshToken != "" {
		return importSecretFingerprint("oauth_refresh", platform, refreshToken), "refresh_token"
	}
	accessToken := importStringValue(credentials["access_token"])
	if accessToken != "" {
		return importSecretFingerprint("oauth_access", platform, accessToken), "access_token"
	}
	return "", ""
}

func importSecretFingerprint(kind, platform, secret string) string {
	normalizedPlatform := strings.ToLower(strings.TrimSpace(platform))
	sum := sha256.Sum256([]byte(kind + "\x00" + normalizedPlatform + "\x00" + secret))
	return kind + ":" + normalizedPlatform + ":" + hex.EncodeToString(sum[:])
}

func accessTokenOnlyOAuthImportWarning(platform, accountType string, credentials map[string]any) string {
	if strings.TrimSpace(accountType) != service.AccountTypeOAuth {
		return ""
	}
	if importStringValue(credentials["access_token"]) == "" || importStringValue(credentials["refresh_token"]) != "" {
		return ""
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		platform = "OAuth"
	}
	return fmt.Sprintf("%s OAuth account has access_token but no refresh_token; it cannot auto-refresh and may become 401 when the access token expires or is invalidated", platform)
}

func (h *AccountHandler) listAllProxies(ctx context.Context) ([]service.Proxy, error) {
	page := 1
	pageSize := dataPageCap
	var out []service.Proxy
	for {
		items, total, err := h.adminService.ListProxies(ctx, page, pageSize, "", "", "", "created_at", "desc")
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if len(out) >= int(total) || len(items) == 0 {
			break
		}
		page++
	}
	return out, nil
}

func (h *AccountHandler) listAccountsFiltered(ctx context.Context, platform, accountType, status, search string, groupID int64, privacyMode, sortBy, sortOrder string) ([]service.Account, error) {
	page := 1
	pageSize := dataPageCap
	var out []service.Account
	for {
		items, total, err := h.adminService.ListAccounts(ctx, page, pageSize, platform, accountType, status, search, groupID, privacyMode, sortBy, sortOrder, 0)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if len(out) >= int(total) || len(items) == 0 {
			break
		}
		page++
	}
	return out, nil
}

func (h *AccountHandler) resolveExportAccounts(ctx context.Context, ids []int64, c *gin.Context) ([]service.Account, error) {
	if len(ids) > 0 {
		accounts, err := h.adminService.GetAccountsByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		out := make([]service.Account, 0, len(accounts))
		for _, acc := range accounts {
			if acc == nil {
				continue
			}
			out = append(out, *acc)
		}
		return out, nil
	}

	platform := c.Query("platform")
	accountType := c.Query("type")
	status := c.Query("status")
	privacyMode := strings.TrimSpace(c.Query("privacy_mode"))
	search := strings.TrimSpace(c.Query("search"))
	sortBy := c.DefaultQuery("sort_by", "name")
	sortOrder := c.DefaultQuery("sort_order", "asc")
	if len(search) > 100 {
		search = search[:100]
	}

	groupID := int64(0)
	if groupIDStr := c.Query("group"); groupIDStr != "" {
		if groupIDStr == accountListGroupUngroupedQueryValue {
			groupID = service.AccountListGroupUngrouped
		} else {
			parsedGroupID, parseErr := strconv.ParseInt(groupIDStr, 10, 64)
			if parseErr != nil || parsedGroupID <= 0 {
				return nil, infraerrors.BadRequest("INVALID_GROUP_FILTER", "invalid group filter")
			}
			groupID = parsedGroupID
		}
	}

	return h.listAccountsFiltered(ctx, platform, accountType, status, search, groupID, privacyMode, sortBy, sortOrder)
}

func (h *AccountHandler) resolveExportProxies(ctx context.Context, accounts []service.Account) ([]service.Proxy, error) {
	if len(accounts) == 0 {
		return []service.Proxy{}, nil
	}

	seen := make(map[int64]struct{})
	ids := make([]int64, 0)
	for i := range accounts {
		if accounts[i].ProxyID == nil {
			continue
		}
		id := *accounts[i].ProxyID
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return []service.Proxy{}, nil
	}

	return h.adminService.GetProxiesByIDs(ctx, ids)
}

func parseAccountIDs(c *gin.Context) ([]int64, error) {
	values := c.QueryArray("ids")
	if len(values) == 0 {
		raw := strings.TrimSpace(c.Query("ids"))
		if raw != "" {
			values = []string{raw}
		}
	}
	if len(values) == 0 {
		return nil, nil
	}

	ids := make([]int64, 0, len(values))
	for _, item := range values {
		for _, part := range strings.Split(item, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil || id <= 0 {
				return nil, fmt.Errorf("invalid account id: %s", part)
			}
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func parseIncludeProxies(c *gin.Context) (bool, error) {
	raw := strings.TrimSpace(strings.ToLower(c.Query("include_proxies")))
	if raw == "" {
		return true, nil
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return true, fmt.Errorf("invalid include_proxies value: %s", raw)
	}
}

func validateDataHeader(payload DataPayload) error {
	if payload.Type != "" && payload.Type != dataType && payload.Type != legacyDataType {
		return fmt.Errorf("unsupported data type: %s", payload.Type)
	}
	if payload.Version != 0 && payload.Version != dataVersion {
		return fmt.Errorf("unsupported data version: %d", payload.Version)
	}
	if payload.Proxies == nil {
		return errors.New("proxies is required")
	}
	if payload.Accounts == nil {
		return errors.New("accounts is required")
	}
	return nil
}

func validateDataProxy(item DataProxy) error {
	if strings.TrimSpace(item.Protocol) == "" {
		return errors.New("proxy protocol is required")
	}
	if strings.TrimSpace(item.Host) == "" {
		return errors.New("proxy host is required")
	}
	if item.Port <= 0 || item.Port > 65535 {
		return errors.New("proxy port is invalid")
	}
	switch item.Protocol {
	case "http", "https", "socks5", "socks5h":
	default:
		return fmt.Errorf("proxy protocol is invalid: %s", item.Protocol)
	}
	if item.Status != "" {
		normalizedStatus := normalizeProxyStatus(item.Status)
		if normalizedStatus != service.StatusActive && normalizedStatus != "inactive" {
			return fmt.Errorf("proxy status is invalid: %s", item.Status)
		}
	}
	return nil
}

func validateDataAccount(item DataAccount) error {
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("account name is required")
	}
	if strings.TrimSpace(item.Platform) == "" {
		return errors.New("account platform is required")
	}
	if strings.TrimSpace(item.Type) == "" {
		return errors.New("account type is required")
	}
	if len(item.Credentials) == 0 {
		return errors.New("account credentials is required")
	}
	switch item.Type {
	case service.AccountTypeOAuth, service.AccountTypeSetupToken, service.AccountTypeAPIKey, service.AccountTypeUpstream:
	default:
		return fmt.Errorf("account type is invalid: %s", item.Type)
	}
	if item.RateMultiplier != nil && *item.RateMultiplier < 0 {
		return errors.New("rate_multiplier must be >= 0")
	}
	if item.Concurrency < 0 {
		return errors.New("concurrency must be >= 0")
	}
	if item.Priority < 0 {
		return errors.New("priority must be >= 0")
	}
	return nil
}

func defaultProxyName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "imported-proxy"
	}
	return name
}

// enrichCredentialsFromIDToken performs best-effort extraction of user info fields
// (email, plan_type, chatgpt_account_id, etc.) from id_token in credentials.
// Only applies to OpenAI OAuth accounts. Skips expired token errors silently.
// Existing credential values are never overwritten — only missing fields are filled.
func enrichCredentialsFromIDToken(item *DataAccount) {
	if item.Credentials == nil {
		return
	}
	// Only enrich OpenAI OAuth accounts
	platform := strings.ToLower(strings.TrimSpace(item.Platform))
	if platform != service.PlatformOpenAI {
		return
	}
	if strings.ToLower(strings.TrimSpace(item.Type)) != service.AccountTypeOAuth {
		return
	}

	idToken, _ := item.Credentials["id_token"].(string)
	if strings.TrimSpace(idToken) == "" {
		return
	}

	// DecodeIDToken skips expiry validation — safe for imported data
	claims, err := openai.DecodeIDToken(idToken)
	if err != nil {
		slog.Debug("import_enrich_id_token_decode_failed", "account", item.Name, "error", err)
		return
	}

	userInfo := claims.GetUserInfo()
	if userInfo == nil {
		return
	}

	// Fill missing fields only (never overwrite existing values)
	setIfMissing := func(key, value string) {
		if value == "" {
			return
		}
		if existing, _ := item.Credentials[key].(string); existing == "" {
			item.Credentials[key] = value
		}
	}

	setIfMissing("email", userInfo.Email)
	setIfMissing("plan_type", userInfo.PlanType)
	setIfMissing("chatgpt_account_id", userInfo.ChatGPTAccountID)
	setIfMissing("chatgpt_user_id", userInfo.ChatGPTUserID)
	setIfMissing("organization_id", userInfo.OrganizationID)
}

func normalizeProxyStatus(status string) string {
	normalized := strings.TrimSpace(strings.ToLower(status))
	switch normalized {
	case "":
		return ""
	case service.StatusActive:
		return service.StatusActive
	case "inactive", service.StatusDisabled:
		return "inactive"
	case "expired":
		// 导入 expired 代理按 inactive 处理，避免导入即触发到期改投逻辑
		return "inactive"
	default:
		return normalized
	}
}
