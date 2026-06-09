package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"log/slog"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	dataType                        = "sub2api-data"
	legacyDataType                  = "sub2api-bundle"
	dataVersion                     = 1
	dataPageCap                     = 1000
	dataInspectLiveProbeConcurrency = 5
	dataInspectLiveProbeTimeout     = 15 * time.Second
)

type DataPayload struct {
	Type       string        `json:"type,omitempty"`
	Version    int           `json:"version,omitempty"`
	ExportedAt string        `json:"exported_at"`
	Proxies    []DataProxy   `json:"proxies"`
	Accounts   []DataAccount `json:"accounts"`
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
}

type DataImportRequest struct {
	Data                 DataPayload `json:"data"`
	SkipDefaultGroupBind *bool       `json:"skip_default_group_bind"`
	GroupIDs             []int64     `json:"group_ids"`
}

type DataImportResult struct {
	ProxyCreated   int               `json:"proxy_created"`
	ProxyReused    int               `json:"proxy_reused"`
	ProxyFailed    int               `json:"proxy_failed"`
	AccountCreated int               `json:"account_created"`
	AccountFailed  int               `json:"account_failed"`
	Errors         []DataImportError `json:"errors,omitempty"`
}

type DataImportError struct {
	Kind     string `json:"kind"`
	Name     string `json:"name,omitempty"`
	ProxyKey string `json:"proxy_key,omitempty"`
	Message  string `json:"message"`
}

type DataInspectRequest struct {
	Data           DataPayload `json:"data"`
	ValidProxyKeys []string    `json:"valid_proxy_keys,omitempty"`
}

type DataInspectResult struct {
	Total          int               `json:"total"`
	Healthy        int               `json:"healthy"`
	Unhealthy      int               `json:"unhealthy"`
	Results        []DataInspectItem `json:"results"`
	ValidProxyKeys []string          `json:"valid_proxy_keys,omitempty"`
}

type DataInspectItem struct {
	Index    int      `json:"index"`
	Name     string   `json:"name,omitempty"`
	Platform string   `json:"platform,omitempty"`
	Type     string   `json:"type,omitempty"`
	ProxyKey string   `json:"proxy_key,omitempty"`
	Healthy  bool     `json:"healthy"`
	Reasons  []string `json:"reasons,omitempty"`
}

type DataInspectStreamEvent struct {
	Type   string             `json:"type"`
	Item   *DataInspectItem   `json:"item,omitempty"`
	Result *DataInspectResult `json:"result,omitempty"`
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
	var req DataImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := validateDataHeader(req.Data); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	executeAdminIdempotentJSON(c, "admin.accounts.import_data", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.importData(ctx, req)
	})
}

func (h *AccountHandler) InspectData(c *gin.Context) {
	var req DataInspectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := validateDataHeader(req.Data); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	existingProxies, err := h.listAllProxies(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if h.accountTestService == nil {
		response.Success(c, inspectDataPayload(req.Data, existingProxies, req.ValidProxyKeys, time.Now()))
		return
	}

	response.Success(c, h.inspectDataPayloadWithLiveProbe(c.Request.Context(), req.Data, existingProxies, req.ValidProxyKeys, time.Now()))
}

func (h *AccountHandler) InspectDataStream(c *gin.Context) {
	var req DataInspectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := validateDataHeader(req.Data); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	existingProxies, err := h.listAllProxies(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, _ := c.Writer.(http.Flusher)
	var writeMu sync.Mutex
	writeEvent := func(event DataInspectStreamEvent) {
		payload, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			return
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", payload)
		if flusher != nil {
			flusher.Flush()
		}
	}

	onItem := func(item DataInspectItem) {
		itemCopy := item
		writeEvent(DataInspectStreamEvent{Type: "item", Item: &itemCopy})
	}

	var result DataInspectResult
	if h.accountTestService == nil {
		result = inspectDataPayloadWithItemCallback(req.Data, existingProxies, req.ValidProxyKeys, time.Now(), onItem)
	} else {
		result = h.inspectDataPayloadWithLiveProbeAndItemCallback(c.Request.Context(), req.Data, existingProxies, req.ValidProxyKeys, time.Now(), onItem)
	}
	writeEvent(DataInspectStreamEvent{Type: "done", Result: &result})
}

func (h *AccountHandler) importData(ctx context.Context, req DataImportRequest) (DataImportResult, error) {
	skipDefaultGroupBind := true
	if req.SkipDefaultGroupBind != nil {
		skipDefaultGroupBind = *req.SkipDefaultGroupBind
	}
	groupIDs := singleDataImportGroupIDs(req.GroupIDs)

	dataPayload := req.Data
	result := DataImportResult{}

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
		if item.ProxyKey != nil && *item.ProxyKey != "" {
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

		accountInput := &service.CreateAccountInput{
			Name:                 item.Name,
			Notes:                item.Notes,
			Platform:             item.Platform,
			Type:                 item.Type,
			Credentials:          item.Credentials,
			Extra:                item.Extra,
			ProxyID:              proxyID,
			Concurrency:          item.Concurrency,
			Priority:             item.Priority,
			RateMultiplier:       item.RateMultiplier,
			GroupIDs:             groupIDs,
			ExpiresAt:            item.ExpiresAt,
			AutoPauseOnExpired:   item.AutoPauseOnExpired,
			SkipDefaultGroupBind: skipDefaultGroupBind,
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

func singleDataImportGroupIDs(groupIDs []int64) []int64 {
	for _, groupID := range groupIDs {
		if groupID > 0 {
			return []int64{groupID}
		}
	}
	return nil
}

func inspectDataPayload(payload DataPayload, existingProxies []service.Proxy, validProxyKeysFromRequest []string, now time.Time) DataInspectResult {
	return buildDataInspectResult(context.Background(), payload, existingProxies, validProxyKeysFromRequest, now, nil, nil)
}

func inspectDataPayloadWithItemCallback(payload DataPayload, existingProxies []service.Proxy, validProxyKeysFromRequest []string, now time.Time, onItem func(DataInspectItem)) DataInspectResult {
	return buildDataInspectResult(context.Background(), payload, existingProxies, validProxyKeysFromRequest, now, nil, onItem)
}

func (h *AccountHandler) inspectDataPayloadWithLiveProbe(ctx context.Context, payload DataPayload, existingProxies []service.Proxy, validProxyKeysFromRequest []string, now time.Time) DataInspectResult {
	return h.inspectDataPayloadWithLiveProbeAndItemCallback(ctx, payload, existingProxies, validProxyKeysFromRequest, now, nil)
}

func (h *AccountHandler) inspectDataPayloadWithLiveProbeAndItemCallback(ctx context.Context, payload DataPayload, existingProxies []service.Proxy, validProxyKeysFromRequest []string, now time.Time, onItem func(DataInspectItem)) DataInspectResult {
	probe := func(ctx context.Context, item DataAccount, proxy *service.Proxy) []string {
		if h == nil || h.accountTestService == nil {
			return []string{"live probe failed: account test service is not configured"}
		}
		account := temporaryServiceAccount(item, proxy)
		probeCtx, cancel := context.WithTimeout(ctx, dataInspectLiveProbeTimeout)
		defer cancel()
		result := h.accountTestService.ProbeTemporaryAccount(probeCtx, account, "")
		if result.Healthy {
			return nil
		}
		message := strings.TrimSpace(result.Message)
		if message == "" {
			message = "unknown error"
		}
		return []string{"live probe failed: " + message}
	}
	return buildDataInspectResult(ctx, payload, existingProxies, validProxyKeysFromRequest, now, probe, onItem)
}

type dataAccountProbeFunc func(context.Context, DataAccount, *service.Proxy) []string

func buildDataInspectResult(ctx context.Context, payload DataPayload, existingProxies []service.Proxy, validProxyKeysFromRequest []string, now time.Time, probe dataAccountProbeFunc, onItem func(DataInspectItem)) DataInspectResult {
	validProxyKeys := make(map[string]struct{}, len(existingProxies)+len(payload.Proxies))
	proxiesByKey := make(map[string]service.Proxy, len(existingProxies)+len(payload.Proxies))
	for i := range existingProxies {
		p := existingProxies[i]
		key := buildProxyKey(p.Protocol, p.Host, p.Port, p.Username, p.Password)
		validProxyKeys[key] = struct{}{}
		proxiesByKey[key] = p
	}
	carriedProxyKeys := make(map[string]struct{}, len(validProxyKeysFromRequest)+len(payload.Proxies))
	for _, key := range validProxyKeysFromRequest {
		key = strings.TrimSpace(key)
		if key != "" {
			validProxyKeys[key] = struct{}{}
			carriedProxyKeys[key] = struct{}{}
		}
	}
	for i := range payload.Proxies {
		item := payload.Proxies[i]
		key := item.ProxyKey
		if key == "" {
			key = buildProxyKey(item.Protocol, item.Host, item.Port, item.Username, item.Password)
		}
		if err := validateDataProxy(item); err == nil {
			validProxyKeys[key] = struct{}{}
			carriedProxyKeys[key] = struct{}{}
			proxiesByKey[key] = dataProxyToServiceProxy(item)
		}
	}

	result := DataInspectResult{
		Total:   len(payload.Accounts),
		Results: make([]DataInspectItem, len(payload.Accounts)),
	}
	if len(carriedProxyKeys) > 0 {
		result.ValidProxyKeys = make([]string, 0, len(carriedProxyKeys))
		for key := range carriedProxyKeys {
			result.ValidProxyKeys = append(result.ValidProxyKeys, key)
		}
		sort.Strings(result.ValidProxyKeys)
	}

	probeIndexes := make([]int, 0, len(payload.Accounts))
	for i := range payload.Accounts {
		account := payload.Accounts[i]
		reasons := inspectDataAccount(account, validProxyKeys, now)
		item := DataInspectItem{
			Index:    i,
			Name:     account.Name,
			Platform: account.Platform,
			Type:     account.Type,
			Healthy:  len(reasons) == 0,
			Reasons:  reasons,
		}
		if account.ProxyKey != nil {
			item.ProxyKey = *account.ProxyKey
		}
		result.Results[i] = item
		if item.Healthy && probe != nil {
			probeIndexes = append(probeIndexes, i)
			continue
		}
		if onItem != nil {
			onItem(item)
		}
	}

	if probe != nil && len(probeIndexes) > 0 {
		runDataInspectLiveProbes(ctx, payload.Accounts, proxiesByKey, result.Results, probeIndexes, probe, onItem)
	}

	for i := range result.Results {
		if result.Results[i].Healthy {
			result.Healthy++
		} else {
			result.Unhealthy++
		}
	}
	return result
}

func runDataInspectLiveProbes(ctx context.Context, accounts []DataAccount, proxiesByKey map[string]service.Proxy, results []DataInspectItem, indexes []int, probe dataAccountProbeFunc, onItem func(DataInspectItem)) {
	workerCount := dataInspectLiveProbeConcurrency
	if workerCount > len(indexes) {
		workerCount = len(indexes)
	}
	if workerCount < 1 {
		workerCount = 1
	}

	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					results[index].Healthy = false
					results[index].Reasons = append(results[index].Reasons, ctx.Err().Error())
					if onItem != nil {
						onItem(results[index])
					}
					continue
				}
				var proxy *service.Proxy
				if accounts[index].ProxyKey != nil {
					key := strings.TrimSpace(*accounts[index].ProxyKey)
					if key != "" {
						if p, ok := proxiesByKey[key]; ok {
							proxy = &p
						} else {
							results[index].Healthy = false
							results[index].Reasons = append(results[index].Reasons, "proxy_key not available for live probe")
							if onItem != nil {
								onItem(results[index])
							}
							continue
						}
					}
				}
				if reasons := probe(ctx, accounts[index], proxy); len(reasons) > 0 {
					results[index].Healthy = false
					results[index].Reasons = append(results[index].Reasons, reasons...)
				}
				if onItem != nil {
					onItem(results[index])
				}
			}
		}()
	}
	for _, index := range indexes {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
}

func dataProxyToServiceProxy(item DataProxy) service.Proxy {
	status := normalizeProxyStatus(item.Status)
	if status == "" {
		status = service.StatusActive
	}
	var expiresAt *time.Time
	if item.ExpiresAt != nil {
		t := time.Unix(*item.ExpiresAt, 0).UTC()
		expiresAt = &t
	}
	fallbackMode := item.FallbackMode
	if fallbackMode == "" {
		fallbackMode = service.FallbackModeNone
	}
	return service.Proxy{
		Name:           defaultProxyName(item.Name),
		Protocol:       strings.TrimSpace(item.Protocol),
		Host:           strings.TrimSpace(item.Host),
		Port:           item.Port,
		Username:       strings.TrimSpace(item.Username),
		Password:       strings.TrimSpace(item.Password),
		Status:         status,
		ExpiresAt:      expiresAt,
		FallbackMode:   fallbackMode,
		ExpiryWarnDays: item.ExpiryWarnDays,
	}
}

func temporaryServiceAccount(item DataAccount, proxy *service.Proxy) *service.Account {
	concurrency := item.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	var expiresAt *time.Time
	if item.ExpiresAt != nil {
		t := time.Unix(*item.ExpiresAt, 0).UTC()
		expiresAt = &t
	}
	account := &service.Account{
		Name:               strings.TrimSpace(item.Name),
		Notes:              item.Notes,
		Platform:           strings.TrimSpace(item.Platform),
		Type:               strings.TrimSpace(item.Type),
		Credentials:        item.Credentials,
		Extra:              item.Extra,
		Concurrency:        concurrency,
		Priority:           item.Priority,
		RateMultiplier:     item.RateMultiplier,
		Status:             service.StatusActive,
		ExpiresAt:          expiresAt,
		AutoPauseOnExpired: false,
		Schedulable:        true,
	}
	if item.AutoPauseOnExpired != nil {
		account.AutoPauseOnExpired = *item.AutoPauseOnExpired
	}
	if proxy != nil {
		account.Proxy = proxy
		proxyID := proxy.ID
		if proxyID == 0 {
			proxyID = -1
		}
		account.ProxyID = &proxyID
	}
	return account
}

func inspectDataAccount(item DataAccount, validProxyKeys map[string]struct{}, now time.Time) []string {
	var reasons []string
	if strings.TrimSpace(item.Name) == "" {
		reasons = append(reasons, "account name is required")
	}
	platform := strings.TrimSpace(item.Platform)
	if platform == "" {
		reasons = append(reasons, "account platform is required")
	} else if !isSupportedImportPlatform(platform) {
		reasons = append(reasons, fmt.Sprintf("account platform is invalid: %s", platform))
	}
	accountType := strings.TrimSpace(item.Type)
	if accountType == "" {
		reasons = append(reasons, "account type is required")
	} else if !isSupportedImportAccountType(accountType) {
		reasons = append(reasons, fmt.Sprintf("account type is invalid: %s", accountType))
	}
	if len(item.Credentials) == 0 {
		reasons = append(reasons, "account credentials is required")
	}
	if item.RateMultiplier != nil && *item.RateMultiplier < 0 {
		reasons = append(reasons, "rate_multiplier must be >= 0")
	}
	if item.Concurrency < 0 {
		reasons = append(reasons, "concurrency must be >= 0")
	}
	if item.Priority < 0 {
		reasons = append(reasons, "priority must be >= 0")
	}
	if item.ExpiresAt != nil && *item.ExpiresAt > 0 && time.Unix(*item.ExpiresAt, 0).Before(now) {
		reasons = append(reasons, "account expired")
	}
	if item.ProxyKey != nil && strings.TrimSpace(*item.ProxyKey) != "" {
		if _, ok := validProxyKeys[*item.ProxyKey]; !ok {
			reasons = append(reasons, "proxy_key not found")
		}
	}

	reasons = append(reasons, inspectDataAccountCredentials(item)...)
	return reasons
}

func isSupportedImportPlatform(platform string) bool {
	switch platform {
	case service.PlatformAnthropic, service.PlatformOpenAI, service.PlatformGemini, service.PlatformAntigravity:
		return true
	default:
		return false
	}
}

func isSupportedImportAccountType(accountType string) bool {
	switch accountType {
	case service.AccountTypeOAuth, service.AccountTypeSetupToken, service.AccountTypeAPIKey, service.AccountTypeUpstream, service.AccountTypeBedrock, service.AccountTypeServiceAccount:
		return true
	default:
		return false
	}
}

func inspectDataAccountCredentials(item DataAccount) []string {
	switch item.Type {
	case service.AccountTypeAPIKey:
		if credentialString(item, "api_key") == "" {
			return []string{"api_key is required"}
		}
	case service.AccountTypeUpstream:
		var reasons []string
		if credentialString(item, "base_url") == "" {
			reasons = append(reasons, "base_url is required")
		}
		if credentialString(item, "api_key") == "" {
			reasons = append(reasons, "api_key is required")
		}
		return reasons
	case service.AccountTypeOAuth, service.AccountTypeSetupToken:
		if credentialString(item, "access_token") == "" && credentialString(item, "refresh_token") == "" {
			return []string{"access_token or refresh_token is required"}
		}
	case service.AccountTypeBedrock:
		if credentialString(item, "auth_mode") == "apikey" {
			if credentialString(item, "api_key") == "" {
				return []string{"api_key is required"}
			}
			return nil
		}
		var reasons []string
		if credentialString(item, "aws_access_key_id") == "" {
			reasons = append(reasons, "aws_access_key_id is required")
		}
		if credentialString(item, "aws_secret_access_key") == "" {
			reasons = append(reasons, "aws_secret_access_key is required")
		}
		return reasons
	case service.AccountTypeServiceAccount:
		if !credentialPresent(item, "service_account_json") && !credentialPresent(item, "service_account") {
			return []string{"service_account_json or service_account is required"}
		}
	}
	return nil
}

func credentialPresent(item DataAccount, key string) bool {
	if credentialString(item, key) != "" {
		return true
	}
	if item.Credentials == nil {
		return false
	}
	value, ok := item.Credentials[key]
	if !ok || value == nil {
		return false
	}
	switch v := value.(type) {
	case map[string]any:
		return len(v) > 0
	default:
		return false
	}
}

func credentialString(item DataAccount, key string) string {
	account := service.Account{Credentials: item.Credentials}
	return strings.TrimSpace(account.GetCredential(key))
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
		items, total, err := h.adminService.ListAccounts(ctx, page, pageSize, platform, accountType, status, search, groupID, privacyMode, sortBy, sortOrder)
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
	if !isSupportedImportAccountType(strings.TrimSpace(item.Type)) {
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
