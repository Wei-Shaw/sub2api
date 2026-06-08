package admin

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ExportData exports proxy-only data for migration.
func (h *ProxyHandler) ExportData(c *gin.Context) {
	ctx := c.Request.Context()

	selectedIDs, err := parseProxyIDs(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED

	var proxies []service.Proxy
	if len(selectedIDs) > 0 {
		proxies, err = h.getProxiesByIDs(ctx, selectedIDs)
		if err != nil {
			response.ErrorFrom(c, err)
			return
	REDACTED
REDACTED else {
		protocol := c.Query("protocol")
		status := c.Query("status")
		search := strings.TrimSpace(c.Query("search"))
		sortBy := c.DefaultQuery("sort_by", "id")
		sortOrder := c.DefaultQuery("sort_order", "desc")
		if len(search) > 100 {
			search = search[:100]
	REDACTED

		proxies, err = h.listProxiesFiltered(ctx, protocol, status, search, sortBy, sortOrder)
		if err != nil {
			response.ErrorFrom(c, err)
			return
	REDACTED
REDACTED

	// 构建 id→name 映射，用于导出备用代理 name
	proxyNameByID := make(map[int64]string, len(proxies))
	for i := range proxies {
		proxyNameByID[proxies[i].ID] = proxies[i].Name
REDACTED

	dataProxies := make([]DataProxy, 0, len(proxies))
	for i := range proxies {
		p := proxies[i]
		key := buildProxyKey(p.Protocol, p.Host, p.Port, p.Username, p.Password)

		var expiresAt *int64
		if p.ExpiresAt != nil {
			v := p.ExpiresAt.Unix()
			expiresAt = &v
	REDACTED
		var backupProxyName string
		if p.BackupProxyID != nil {
			backupProxyName = proxyNameByID[*p.BackupProxyID]
	REDACTED
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
	REDACTED)
REDACTED

	payload := DataPayload{
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Proxies:    dataProxies,
		Accounts:   []DataAccount{REDACTED,
REDACTED

	response.Success(c, payload)
REDACTED

// ImportData imports proxy-only data for migration.
func (h *ProxyHandler) ImportData(c *gin.Context) {
	type ProxyImportRequest struct {
		Data DataPayload `json:"data"`
REDACTED

	var req ProxyImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	if err := validateDataHeader(req.Data); err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED

	ctx := c.Request.Context()
	result := DataImportResult{REDACTED

	existingProxies, err := h.listProxiesFiltered(ctx, "", "", "", "id", "desc")
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	proxyByKey := make(map[string]service.Proxy, len(existingProxies))
	// proxyNameToID 用于 backup_proxy_name 反查：DB 已有 + 本批次新建均会写入
	proxyNameToID := make(map[string]int64, len(existingProxies))
	for i := range existingProxies {
		p := existingProxies[i]
		key := buildProxyKey(p.Protocol, p.Host, p.Port, p.Username, p.Password)
		proxyByKey[key] = p
		if p.Name != "" {
			proxyNameToID[p.Name] = p.ID
	REDACTED
REDACTED

	latencyProbeIDs := make([]int64, 0, len(req.Data.Proxies))
	for i := range req.Data.Proxies {
		item := req.Data.Proxies[i]
		key := item.ProxyKey
		if key == "" {
			key = buildProxyKey(item.Protocol, item.Host, item.Port, item.Username, item.Password)
	REDACTED

		if err := validateDataProxy(item); err != nil {
			result.ProxyFailed++
			result.Errors = append(result.Errors, DataImportError{
				Kind:     "proxy",
				Name:     item.Name,
				ProxyKey: key,
				Message:  err.Error(),
		REDACTED)
			continue
	REDACTED

		normalizedStatus := normalizeProxyStatus(item.Status)
		if existing, ok := proxyByKey[key]; ok {
			result.ProxyReused++
			if normalizedStatus != "" && normalizedStatus != existing.Status {
				// 已存在代理同步 status 时，同时保留/覆盖导入 item 的完整字段，
				// 避免 UpdateProxy 零值覆盖有效期/fallback 配置。
				var existingExpiresAt *time.Time
				if item.ExpiresAt != nil {
					t := time.Unix(*item.ExpiresAt, 0).UTC()
					existingExpiresAt = &t
			REDACTED
				existingFallbackMode := item.FallbackMode
				if existingFallbackMode == "" {
					existingFallbackMode = service.FallbackModeNone
			REDACTED
				var existingBackupProxyID *int64
				if item.BackupProxyName != "" {
					if bid, ok := proxyNameToID[item.BackupProxyName]; ok {
						existingBackupProxyID = &bid
				REDACTED
			REDACTED
				updateInput := &service.UpdateProxyInput{
					Status:         normalizedStatus,
					ExpiresAt:      existingExpiresAt,
					FallbackMode:   existingFallbackMode,
					BackupProxyID:  existingBackupProxyID,
					ExpiryWarnDays: item.ExpiryWarnDays,
					// 保留已存在代理的网络配置字段
					Name:     existing.Name,
					Protocol: existing.Protocol,
					Host:     existing.Host,
					Port:     existing.Port,
					Username: existing.Username,
					Password: existing.Password,
			REDACTED
				if _, err := h.adminService.UpdateProxy(ctx, existing.ID, updateInput); err != nil {
					result.Errors = append(result.Errors, DataImportError{
						Kind:     "proxy",
						Name:     item.Name,
						ProxyKey: key,
						Message:  "update status failed: " + err.Error(),
				REDACTED)
			REDACTED
		REDACTED
			latencyProbeIDs = append(latencyProbeIDs, existing.ID)
			continue
	REDACTED

		// 解析 expires_at（unix 秒 → *time.Time）
		var expiresAt *time.Time
		if item.ExpiresAt != nil {
			t := time.Unix(*item.ExpiresAt, 0).UTC()
			expiresAt = &t
	REDACTED

		// 解析 backup_proxy_name → backup_proxy_id
		fallbackMode := item.FallbackMode
		var backupProxyID *int64
		if item.BackupProxyName != "" {
			if bid, ok := proxyNameToID[item.BackupProxyName]; ok {
				backupProxyID = &bid
		REDACTED else {
				// 查不到备用代理：降级 fallback_mode=none，记录 warning
				fallbackMode = service.FallbackModeNone
				result.Errors = append(result.Errors, DataImportError{
					Kind:     "proxy",
					Name:     item.Name,
					ProxyKey: key,
					Message:  fmt.Sprintf("backup_proxy_name %q not found, fallback_mode downgraded to none", item.BackupProxyName),
			REDACTED)
		REDACTED
	REDACTED

		created, err := h.adminService.CreateProxy(ctx, &service.CreateProxyInput{
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
	REDACTED)
		if err != nil {
			result.ProxyFailed++
			result.Errors = append(result.Errors, DataImportError{
				Kind:     "proxy",
				Name:     item.Name,
				ProxyKey: key,
				Message:  err.Error(),
		REDACTED)
			continue
	REDACTED
		result.ProxyCreated++
		proxyByKey[key] = *created
		// 把新建代理的 name 也加入反查表，供后续批内代理引用
		if created.Name != "" {
			proxyNameToID[created.Name] = created.ID
	REDACTED

		if normalizedStatus != "" && normalizedStatus != created.Status {
			// 新建后同步 status 时，传入完整字段，避免零值覆盖刚创建的有效期/fallback 配置。
			if _, err := h.adminService.UpdateProxy(ctx, created.ID, &service.UpdateProxyInput{
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
		REDACTED); err != nil {
				result.Errors = append(result.Errors, DataImportError{
					Kind:     "proxy",
					Name:     item.Name,
					ProxyKey: key,
					Message:  "update status failed: " + err.Error(),
			REDACTED)
		REDACTED
	REDACTED
		// CreateProxy already triggers a latency probe, avoid double probing here.
REDACTED

	if len(latencyProbeIDs) > 0 {
		ids := append([]int64(nil), latencyProbeIDs...)
		go func() {
			for _, id := range ids {
				_, _ = h.adminService.TestProxy(context.Background(), id)
		REDACTED
	REDACTED()
REDACTED

	response.Success(c, result)
REDACTED

func (h *ProxyHandler) getProxiesByIDs(ctx context.Context, ids []int64) ([]service.Proxy, error) {
	if len(ids) == 0 {
		return []service.Proxy{REDACTED, nil
REDACTED
	return h.adminService.GetProxiesByIDs(ctx, ids)
REDACTED

func parseProxyIDs(c *gin.Context) ([]int64, error) {
	values := c.QueryArray("ids")
	if len(values) == 0 {
		raw := strings.TrimSpace(c.Query("ids"))
		if raw != "" {
			values = []string{rawREDACTED
	REDACTED
REDACTED
	if len(values) == 0 {
		return nil, nil
REDACTED

	ids := make([]int64, 0, len(values))
	for _, item := range values {
		for _, part := range strings.Split(item, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
		REDACTED
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil || id <= 0 {
				return nil, fmt.Errorf("invalid proxy id: %s", part)
		REDACTED
			ids = append(ids, id)
	REDACTED
REDACTED
	return ids, nil
REDACTED

func (h *ProxyHandler) listProxiesFiltered(ctx context.Context, protocol, status, search, sortBy, sortOrder string) ([]service.Proxy, error) {
	page := 1
	pageSize := dataPageCap
	var out []service.Proxy
	sortBy = strings.TrimSpace(sortBy)
	useAccountCountSort := strings.EqualFold(sortBy, "account_count")
	for {
		if useAccountCountSort {
			items, total, err := h.adminService.ListProxiesWithAccountCount(ctx, page, pageSize, protocol, status, search, sortBy, sortOrder)
			if err != nil {
				return nil, err
		REDACTED
			for i := range items {
				out = append(out, items[i].Proxy)
		REDACTED
			if len(out) >= int(total) || len(items) == 0 {
				break
		REDACTED
	REDACTED else {
			items, total, err := h.adminService.ListProxies(ctx, page, pageSize, protocol, status, search, sortBy, sortOrder)
			if err != nil {
				return nil, err
		REDACTED
			out = append(out, items...)
			if len(out) >= int(total) || len(items) == 0 {
				break
		REDACTED
	REDACTED
		page++
REDACTED
	return out, nil
REDACTED
