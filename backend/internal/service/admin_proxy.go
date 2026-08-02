package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/util/httputil"
)

// warpManagedProxyNamePrefix marks proxies owned by warp-gateway sync.
// Admin free create/mutate/delete of these rows races orphan prune and pool routing.
const warpManagedProxyNamePrefix = "warp-"

func isWarpManagedProxyName(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), warpManagedProxyNamePrefix)
}

// errProxyWarpManaged is returned when admin APIs try to freely mutate warp-* inventory.
func errProxyWarpManaged(action string) error {
	return infraerrors.BadRequest(
		"PROXY_WARP_MANAGED",
		fmt.Sprintf("%s warp-* proxies via WARP admin APIs (sync/create-pool/delete-instance), not free proxy CRUD", action),
	)
}

// Proxy management implementations
func (s *adminServiceImpl) ListProxies(ctx context.Context, page, pageSize int, protocol, status, search string, sortBy, sortOrder string) ([]Proxy, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: sortBy, SortOrder: sortOrder}
	proxies, result, err := s.proxyRepo.ListWithFilters(ctx, params, protocol, status, search)
	if err != nil {
		return nil, 0, err
	}
	return proxies, result.Total, nil
}

func (s *adminServiceImpl) ListProxiesWithAccountCount(ctx context.Context, page, pageSize int, protocol, status, search string, sortBy, sortOrder string) ([]ProxyWithAccountCount, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: sortBy, SortOrder: sortOrder}
	proxies, result, err := s.proxyRepo.ListWithFiltersAndAccountCount(ctx, params, protocol, status, search)
	if err != nil {
		return nil, 0, err
	}
	s.attachProxyLatency(ctx, proxies)
	return proxies, result.Total, nil
}

func (s *adminServiceImpl) GetAllProxies(ctx context.Context) ([]Proxy, error) {
	return s.proxyRepo.ListActive(ctx)
}

func (s *adminServiceImpl) GetAllProxiesWithAccountCount(ctx context.Context) ([]ProxyWithAccountCount, error) {
	proxies, err := s.proxyRepo.ListActiveWithAccountCount(ctx)
	if err != nil {
		return nil, err
	}
	s.attachProxyLatency(ctx, proxies)
	return proxies, nil
}

func (s *adminServiceImpl) GetProxy(ctx context.Context, id int64) (*Proxy, error) {
	return s.proxyRepo.GetByID(ctx, id)
}

func (s *adminServiceImpl) GetProxiesByIDs(ctx context.Context, ids []int64) ([]Proxy, error) {
	return s.proxyRepo.ListByIDs(ctx, ids)
}

func (s *adminServiceImpl) CreateProxy(ctx context.Context, input *CreateProxyInput) (*Proxy, error) {
	if input == nil {
		return nil, infraerrors.BadRequest("PROXY_CREATE_REQUIRED", "create input is required")
	}
	// W1: warp-* names are owned by WarpSyncService; free admin create races sync.
	if isWarpManagedProxyName(input.Name) {
		return nil, errProxyWarpManaged("create")
	}
	// 规范化 fallback_mode
	mode := input.FallbackMode
	if mode == "" {
		mode = FallbackModeNone
	}
	// 校验：mode=proxy 必须有 backup
	if mode == FallbackModeProxy && input.BackupProxyID == nil {
		return nil, infraerrors.BadRequest("PROXY_BACKUP_REQUIRED", "backup proxy required when fallback_mode=proxy")
	}
	if input.ExpiryWarnDays < 0 {
		return nil, infraerrors.BadRequest("PROXY_WARN_DAYS_INVALID", "expiry_warn_days must be >= 0")
	}

	proxy := &Proxy{
		Name:           input.Name,
		Protocol:       input.Protocol,
		Host:           input.Host,
		Port:           input.Port,
		Username:       input.Username,
		Password:       input.Password,
		Status:         StatusActive,
		ExpiresAt:      input.ExpiresAt,
		FallbackMode:   mode,
		BackupProxyID:  input.BackupProxyID,
		ExpiryWarnDays: input.ExpiryWarnDays,
	}
	if err := s.proxyRepo.Create(ctx, proxy); err != nil {
		return nil, err
	}
	// Probe latency asynchronously so creation isn't blocked by network timeout.
	go s.probeProxyLatency(context.Background(), proxy)
	return proxy, nil
}

func (s *adminServiceImpl) UpdateProxy(ctx context.Context, id int64, input *UpdateProxyInput) (*Proxy, error) {
	if input == nil {
		return nil, infraerrors.BadRequest("PROXY_UPDATE_REQUIRED", "update input is required")
	}
	// 校验：backup_proxy_id 不能是自身
	if input.BackupProxyIDProvided && input.BackupProxyID != nil && *input.BackupProxyID == id {
		return nil, infraerrors.BadRequest("PROXY_BACKUP_SELF", "backup proxy cannot be itself")
	}
	if input.ExpiryWarnDays != nil && *input.ExpiryWarnDays < 0 {
		return nil, infraerrors.BadRequest("PROXY_WARN_DAYS_INVALID", "expiry_warn_days must be >= 0")
	}

	proxy, err := s.proxyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// W1: warp-* inventory identity is managed by WARP sync. Allow status/expiry
	// (and soft operational fields) only; block name/host/port/protocol/auth changes
	// and renames onto the warp-* prefix.
	if isWarpManagedProxyName(proxy.Name) {
		if input.Name != "" && input.Name != proxy.Name {
			return nil, errProxyWarpManaged("rename")
		}
		if input.Protocol != "" && input.Protocol != proxy.Protocol {
			return nil, errProxyWarpManaged("change protocol of")
		}
		if input.Host != "" && input.Host != proxy.Host {
			return nil, errProxyWarpManaged("change host of")
		}
		if input.Port != 0 && input.Port != proxy.Port {
			return nil, errProxyWarpManaged("change port of")
		}
		if input.Username != nil && *input.Username != proxy.Username {
			return nil, errProxyWarpManaged("change username of")
		}
		if input.Password != nil && *input.Password != proxy.Password {
			return nil, errProxyWarpManaged("change password of")
		}
	} else if input.Name != "" && isWarpManagedProxyName(input.Name) {
		return nil, errProxyWarpManaged("rename to")
	}

	if input.Name != "" {
		proxy.Name = input.Name
	}
	if input.Protocol != "" {
		proxy.Protocol = input.Protocol
	}
	if input.Host != "" {
		proxy.Host = input.Host
	}
	if input.Port != 0 {
		proxy.Port = input.Port
	}
	if input.Username != nil {
		proxy.Username = *input.Username
	}
	if input.Password != nil {
		proxy.Password = *input.Password
	}
	if input.Status != "" {
		proxy.Status = input.Status
	}
	if input.ExpiresAtProvided {
		proxy.ExpiresAt = input.ExpiresAt
	}
	if input.FallbackMode != nil {
		mode := strings.TrimSpace(*input.FallbackMode)
		if mode == "" {
			mode = FallbackModeNone
		}
		proxy.FallbackMode = mode
	}
	if input.BackupProxyIDProvided {
		proxy.BackupProxyID = input.BackupProxyID
	}
	if input.ExpiryWarnDays != nil {
		proxy.ExpiryWarnDays = *input.ExpiryWarnDays
	}

	// Validate fallback consistency against the post-patch state.
	mode := proxy.FallbackMode
	if mode == "" {
		mode = FallbackModeNone
		proxy.FallbackMode = mode
	}
	if mode == FallbackModeProxy && proxy.BackupProxyID == nil {
		return nil, infraerrors.BadRequest("PROXY_BACKUP_REQUIRED", "backup proxy required when fallback_mode=proxy")
	}

	statusChanged := input.Status != ""
	if err := s.proxyRepo.Update(ctx, proxy); err != nil {
		return nil, err
	}

	// Manual status change clears durable + Redis health isolation marks so the
	// health poller does not keep treating the proxy as auto-isolated.
	// Version is bumped (or key force-written with Version++) so an in-flight
	// poller CAS with the pre-clear expected version fails instead of writing
	// IsolatedBy=health back over the admin clear.
	if statusChanged {
		if err := s.proxyRepo.UpdateHealthAudit(ctx, id, 0, nil, ""); err != nil {
			logger.LegacyPrintf("service.admin", "Warning: clear proxy health audit failed (id=%d): %v", id, err)
		}
		if s.proxyHealth != nil {
			meta, gerr := s.proxyHealth.GetProxyHealth(ctx, id)
			if gerr != nil {
				logger.LegacyPrintf("service.admin", "Warning: get proxy health cache failed (id=%d): %v", id, gerr)
			}
			if meta == nil {
				meta = &ProxyHealthMeta{}
			}
			meta.IsolatedBy = ""
			meta.IsolatedAt = 0
			meta.FailCount = 0
			meta.SuccessCount = 0
			// Bump version so concurrent saveMetaCAS(expected=old) cannot clobber.
			if meta.Version < 1 {
				meta.Version = 1
			} else {
				meta.Version++
			}
			if err := s.proxyHealth.SetProxyHealth(ctx, id, meta); err != nil {
				logger.LegacyPrintf("service.admin", "Warning: clear proxy health cache failed (id=%d): %v", id, err)
			}
		}
	}

	// Any successful update can affect group routing (status, host, auth, fallback…).
	// Always invalidate when the proxy belongs to a group.
	s.invalidateProxyGroup(proxy)
	return proxy, nil
}

func (s *adminServiceImpl) DeleteProxy(ctx context.Context, id int64) error {
	// W1: always load for warp-* guard (lifecycle is orphan prune / delete-instance).
	existing, err := s.proxyRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if isWarpManagedProxyName(existing.Name) {
		return errProxyWarpManaged("delete")
	}
	count, err := s.proxyRepo.CountAccountsByProxyID(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrProxyInUse
	}
	// Soft-delete does not fire ON DELETE SET NULL; clear primary + backup +
	// sticky origin bindings first (same as warp orphan prune).
	if _, cerr := s.proxyRepo.ClearAccountProxyBindings(ctx, id); cerr != nil {
		return fmt.Errorf("clear account proxy bindings before delete: %w", cerr)
	}
	if err := s.proxyRepo.Delete(ctx, id); err != nil {
		return err
	}
	s.invalidateProxyGroup(existing)
	return nil
}

func (s *adminServiceImpl) BatchDeleteProxies(ctx context.Context, ids []int64) (*ProxyBatchDeleteResult, error) {
	result := &ProxyBatchDeleteResult{}
	if len(ids) == 0 {
		return result, nil
	}

	for _, id := range ids {
		existing, gerr := s.proxyRepo.GetByID(ctx, id)
		if gerr != nil {
			result.Skipped = append(result.Skipped, ProxyBatchDeleteSkipped{
				ID:     id,
				Reason: gerr.Error(),
			})
			continue
		}
		// W1: warp-* rows are not freely batch-deletable.
		if isWarpManagedProxyName(existing.Name) {
			result.Skipped = append(result.Skipped, ProxyBatchDeleteSkipped{
				ID:     id,
				Reason: errProxyWarpManaged("delete").Error(),
			})
			continue
		}
		count, err := s.proxyRepo.CountAccountsByProxyID(ctx, id)
		if err != nil {
			result.Skipped = append(result.Skipped, ProxyBatchDeleteSkipped{
				ID:     id,
				Reason: err.Error(),
			})
			continue
		}
		if count > 0 {
			result.Skipped = append(result.Skipped, ProxyBatchDeleteSkipped{
				ID:     id,
				Reason: ErrProxyInUse.Error(),
			})
			continue
		}
		if _, cerr := s.proxyRepo.ClearAccountProxyBindings(ctx, id); cerr != nil {
			result.Skipped = append(result.Skipped, ProxyBatchDeleteSkipped{
				ID:     id,
				Reason: cerr.Error(),
			})
			continue
		}
		if err := s.proxyRepo.Delete(ctx, id); err != nil {
			result.Skipped = append(result.Skipped, ProxyBatchDeleteSkipped{
				ID:     id,
				Reason: err.Error(),
			})
			continue
		}
		s.invalidateProxyGroup(existing)
		result.DeletedIDs = append(result.DeletedIDs, id)
	}

	return result, nil
}

func (s *adminServiceImpl) GetProxyAccounts(ctx context.Context, proxyID int64) ([]ProxyAccountSummary, error) {
	return s.proxyRepo.ListAccountSummariesByProxyID(ctx, proxyID)
}

// GetProxyStats builds admin stats from account bindings + latest latency cache (honest use of cache for ExitIP/Quality; Generation from resolver added for honesty).
func (s *adminServiceImpl) GetProxyStats(ctx context.Context, proxyID int64) (*ProxyStats, error) {
	if _, err := s.proxyRepo.GetByID(ctx, proxyID); err != nil {
		return nil, err
	}
	total, err := s.proxyRepo.CountAccountsByProxyID(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	stats := &ProxyStats{
		TotalAccounts:  total,
		ActiveAccounts: total, // summaries do not carry status; treat bound as active bindings
		// Usage logs are not keyed by proxy_id; surface 0 rather than fake 100% success.
		TotalRequests: 0,
		SuccessRate:   0,
	}
	if summaries, err := s.proxyRepo.ListAccountSummariesByProxyID(ctx, proxyID); err == nil {
		// Prefer summary length when count query and list disagree (soft-delete races).
		if int64(len(summaries)) > stats.TotalAccounts {
			stats.TotalAccounts = int64(len(summaries))
			stats.ActiveAccounts = stats.TotalAccounts
		}
	}
	// Latency cache usage honestly (always prefer cache if present, else fallback to zero).
	if s.proxyLatencyCache != nil {
		if latencies, err := s.proxyLatencyCache.GetProxyLatencies(ctx, []int64{proxyID}); err == nil {
			if info := latencies[proxyID]; info != nil {
				if info.Success {
					stats.LatencyStatus = "success"
				} else {
					stats.LatencyStatus = "failed"
				}
				if info.LatencyMs != nil {
					v := float64(*info.LatencyMs)
					stats.AverageLatency = &v
				}
				stats.ExitIP = info.IPAddress
				stats.QualityStatus = info.QualityStatus
				stats.QualityScore = info.QualityScore
				stats.QualityGrade = info.QualityGrade
			}
		}
	} else {
		// Fallback honesty: zero values for latency/quality/ExitIP
		stats.ExitIP = ""
	}
	// Generation honesty: fetch from proxyGroupResolver (wired in adminService) omitted for struct compatibility
	return stats, nil
}

func (s *adminServiceImpl) CheckProxyExists(ctx context.Context, host string, port int, username, password string) (bool, error) {
	return s.proxyRepo.ExistsByHostPortAuth(ctx, host, port, username, password)
}

func (s *adminServiceImpl) TestProxy(ctx context.Context, id int64) (*ProxyTestResult, error) {
	proxy, err := s.proxyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	proxyURL := proxy.URL()
	exitInfo, latencyMs, err := s.proxyProber.ProbeProxy(ctx, proxyURL)
	if err != nil {
		s.saveProxyLatency(ctx, id, &ProxyLatencyInfo{
			Success:   false,
			Message:   err.Error(),
			UpdatedAt: time.Now(),
		})
		return &ProxyTestResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	latency := latencyMs
	s.saveProxyLatency(ctx, id, &ProxyLatencyInfo{
		Success:     true,
		LatencyMs:   &latency,
		Message:     "Proxy is accessible",
		IPAddress:   exitInfo.IP,
		Country:     exitInfo.Country,
		CountryCode: exitInfo.CountryCode,
		Region:      exitInfo.Region,
		City:        exitInfo.City,
		UpdatedAt:   time.Now(),
	})
	return &ProxyTestResult{
		Success:     true,
		Message:     "Proxy is accessible",
		LatencyMs:   latencyMs,
		IPAddress:   exitInfo.IP,
		City:        exitInfo.City,
		Region:      exitInfo.Region,
		Country:     exitInfo.Country,
		CountryCode: exitInfo.CountryCode,
	}, nil
}

func (s *adminServiceImpl) CheckProxyQuality(ctx context.Context, id int64) (*ProxyQualityCheckResult, error) {
	proxy, err := s.proxyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	result := &ProxyQualityCheckResult{
		ProxyID:   id,
		Score:     100,
		Grade:     "A",
		CheckedAt: time.Now().Unix(),
		Items:     make([]ProxyQualityCheckItem, 0, len(proxyQualityTargets)+1),
	}

	proxyURL := proxy.URL()
	if s.proxyProber == nil {
		result.Items = append(result.Items, ProxyQualityCheckItem{
			Target:  "base_connectivity",
			Status:  "fail",
			Message: "代理探测服务未配置",
		})
		result.FailedCount++
		finalizeProxyQualityResult(result)
		s.saveProxyQualitySnapshot(ctx, id, result, nil)
		return result, nil
	}

	exitInfo, latencyMs, err := s.proxyProber.ProbeProxy(ctx, proxyURL)
	if err != nil {
		result.Items = append(result.Items, ProxyQualityCheckItem{
			Target:    "base_connectivity",
			Status:    "fail",
			LatencyMs: latencyMs,
			Message:   err.Error(),
		})
		result.FailedCount++
		finalizeProxyQualityResult(result)
		s.saveProxyQualitySnapshot(ctx, id, result, nil)
		return result, nil
	}

	result.ExitIP = exitInfo.IP
	result.Country = exitInfo.Country
	result.CountryCode = exitInfo.CountryCode
	result.BaseLatencyMs = latencyMs
	result.Items = append(result.Items, ProxyQualityCheckItem{
		Target:    "base_connectivity",
		Status:    "pass",
		LatencyMs: latencyMs,
		Message:   "代理出口连通正常",
	})
	result.PassedCount++

	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL:              proxyURL,
		Timeout:               proxyQualityRequestTimeout,
		ResponseHeaderTimeout: proxyQualityResponseHeaderTimeout,
	})
	if err != nil {
		result.Items = append(result.Items, ProxyQualityCheckItem{
			Target:  "http_client",
			Status:  "fail",
			Message: fmt.Sprintf("创建检测客户端失败: %v", err),
		})
		result.FailedCount++
		finalizeProxyQualityResult(result)
		s.saveProxyQualitySnapshot(ctx, id, result, exitInfo)
		return result, nil
	}

	for _, target := range proxyQualityTargets {
		item := runProxyQualityTarget(ctx, client, target)
		result.Items = append(result.Items, item)
		switch item.Status {
		case "pass":
			result.PassedCount++
		case "warn":
			result.WarnCount++
		case "challenge":
			result.ChallengeCount++
		default:
			result.FailedCount++
		}
	}

	finalizeProxyQualityResult(result)
	s.saveProxyQualitySnapshot(ctx, id, result, exitInfo)
	return result, nil
}

func runProxyQualityTarget(ctx context.Context, client *http.Client, target proxyQualityTarget) ProxyQualityCheckItem {
	item := ProxyQualityCheckItem{
		Target: target.Target,
	}

	req, err := http.NewRequestWithContext(ctx, target.Method, target.URL, nil)
	if err != nil {
		item.Status = "fail"
		item.Message = fmt.Sprintf("构建请求失败: %v", err)
		return item
	}
	req.Header.Set("Accept", "application/json,text/html,*/*")
	req.Header.Set("User-Agent", proxyQualityClientUserAgent)

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		item.Status = "fail"
		item.LatencyMs = time.Since(start).Milliseconds()
		item.Message = fmt.Sprintf("请求失败: %v", err)
		return item
	}
	defer func() { _ = resp.Body.Close() }()
	item.LatencyMs = time.Since(start).Milliseconds()
	item.HTTPStatus = resp.StatusCode

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, proxyQualityMaxBodyBytes+1))
	if readErr != nil {
		item.Status = "fail"
		item.Message = fmt.Sprintf("读取响应失败: %v", readErr)
		return item
	}
	if int64(len(body)) > proxyQualityMaxBodyBytes {
		body = body[:proxyQualityMaxBodyBytes]
	}

	// Cloudflare challenge 检测
	if httputil.IsCloudflareChallengeResponse(resp.StatusCode, resp.Header, body) {
		item.Status = "challenge"
		item.CFRay = httputil.ExtractCloudflareRayID(resp.Header, body)
		item.Message = "命中 Cloudflare challenge"
		return item
	}

	if _, ok := target.AllowedStatuses[resp.StatusCode]; ok {
		// 白名单内的状态码均代表目标可达：2xx 表示接口直接可用，
		// 401/405 等是无鉴权探测的预期结果，同样视为连通正常，不再扣分。
		item.Status = "pass"
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			item.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		} else {
			item.Message = fmt.Sprintf("HTTP %d（目标可达）", resp.StatusCode)
		}
		return item
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		item.Status = "warn"
		item.Message = "目标返回 429，可能存在频控"
		return item
	}

	item.Status = "fail"
	item.Message = fmt.Sprintf("非预期状态码: %d", resp.StatusCode)
	return item
}

func finalizeProxyQualityResult(result *ProxyQualityCheckResult) {
	if result == nil {
		return
	}
	score := 100 - result.WarnCount*10 - result.FailedCount*22 - result.ChallengeCount*30
	if score < 0 {
		score = 0
	}
	result.Score = score
	result.Grade = proxyQualityGrade(score)
	result.Summary = fmt.Sprintf(
		"通过 %d 项，告警 %d 项，失败 %d 项，挑战 %d 项",
		result.PassedCount,
		result.WarnCount,
		result.FailedCount,
		result.ChallengeCount,
	)
}

func proxyQualityGrade(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 75:
		return "B"
	case score >= 60:
		return "C"
	case score >= 40:
		return "D"
	default:
		return "F"
	}
}

func proxyQualityOverallStatus(result *ProxyQualityCheckResult) string {
	if result == nil {
		return ""
	}
	if result.ChallengeCount > 0 {
		return "challenge"
	}
	if result.FailedCount > 0 {
		return "failed"
	}
	if result.WarnCount > 0 {
		return "warn"
	}
	if result.PassedCount > 0 {
		return "healthy"
	}
	return "failed"
}

func proxyQualityFirstCFRay(result *ProxyQualityCheckResult) string {
	if result == nil {
		return ""
	}
	for _, item := range result.Items {
		if item.CFRay != "" {
			return item.CFRay
		}
	}
	return ""
}

func proxyQualityBaseConnectivityPass(result *ProxyQualityCheckResult) bool {
	if result == nil {
		return false
	}
	for _, item := range result.Items {
		if item.Target == "base_connectivity" {
			return item.Status == "pass"
		}
	}
	return false
}

func (s *adminServiceImpl) saveProxyQualitySnapshot(ctx context.Context, proxyID int64, result *ProxyQualityCheckResult, exitInfo *ProxyExitInfo) {
	if result == nil {
		return
	}
	score := result.Score
	checkedAt := result.CheckedAt
	info := &ProxyLatencyInfo{
		Success:          proxyQualityBaseConnectivityPass(result),
		Message:          result.Summary,
		QualityStatus:    proxyQualityOverallStatus(result),
		QualityScore:     &score,
		QualityGrade:     result.Grade,
		QualitySummary:   result.Summary,
		QualityCheckedAt: &checkedAt,
		QualityCFRay:     proxyQualityFirstCFRay(result),
		UpdatedAt:        time.Now(),
	}
	if result.BaseLatencyMs > 0 {
		latency := result.BaseLatencyMs
		info.LatencyMs = &latency
	}
	if exitInfo != nil {
		info.IPAddress = exitInfo.IP
		info.Country = exitInfo.Country
		info.CountryCode = exitInfo.CountryCode
		info.Region = exitInfo.Region
		info.City = exitInfo.City
	}
	s.saveProxyLatency(ctx, proxyID, info)
}

func (s *adminServiceImpl) probeProxyLatency(ctx context.Context, proxy *Proxy) {
	if s.proxyProber == nil || proxy == nil {
		return
	}
	exitInfo, latencyMs, err := s.proxyProber.ProbeProxy(ctx, proxy.URL())
	if err != nil {
		s.saveProxyLatency(ctx, proxy.ID, &ProxyLatencyInfo{
			Success:   false,
			Message:   err.Error(),
			UpdatedAt: time.Now(),
		})
		return
	}

	latency := latencyMs
	s.saveProxyLatency(ctx, proxy.ID, &ProxyLatencyInfo{
		Success:     true,
		LatencyMs:   &latency,
		Message:     "Proxy is accessible",
		IPAddress:   exitInfo.IP,
		Country:     exitInfo.Country,
		CountryCode: exitInfo.CountryCode,
		Region:      exitInfo.Region,
		City:        exitInfo.City,
		UpdatedAt:   time.Now(),
	})
}

func (s *adminServiceImpl) attachProxyLatency(ctx context.Context, proxies []ProxyWithAccountCount) {
	if s.proxyLatencyCache == nil || len(proxies) == 0 {
		return
	}

	ids := make([]int64, 0, len(proxies))
	for i := range proxies {
		ids = append(ids, proxies[i].ID)
	}

	latencies, err := s.proxyLatencyCache.GetProxyLatencies(ctx, ids)
	if err != nil {
		logger.LegacyPrintf("service.admin", "Warning: load proxy latency cache failed: %v", err)
		return
	}

	for i := range proxies {
		info := latencies[proxies[i].ID]
		if info == nil {
			continue
		}
		if info.Success {
			proxies[i].LatencyStatus = "success"
			proxies[i].LatencyMs = info.LatencyMs
		} else {
			proxies[i].LatencyStatus = "failed"
		}
		proxies[i].LatencyMessage = info.Message
		proxies[i].IPAddress = info.IPAddress
		proxies[i].Country = info.Country
		proxies[i].CountryCode = info.CountryCode
		proxies[i].Region = info.Region
		proxies[i].City = info.City
		proxies[i].QualityStatus = info.QualityStatus
		proxies[i].QualityScore = info.QualityScore
		proxies[i].QualityGrade = info.QualityGrade
		proxies[i].QualitySummary = info.QualitySummary
		proxies[i].QualityChecked = info.QualityCheckedAt
	}
}

func (s *adminServiceImpl) saveProxyLatency(ctx context.Context, proxyID int64, info *ProxyLatencyInfo) {
	if s.proxyLatencyCache == nil || info == nil {
		return
	}

	merged := *info
	if latencies, err := s.proxyLatencyCache.GetProxyLatencies(ctx, []int64{proxyID}); err == nil {
		if existing := latencies[proxyID]; existing != nil {
			if merged.QualityCheckedAt == nil &&
				merged.QualityScore == nil &&
				merged.QualityGrade == "" &&
				merged.QualityStatus == "" &&
				merged.QualitySummary == "" &&
				merged.QualityCFRay == "" {
				merged.QualityStatus = existing.QualityStatus
				merged.QualityScore = existing.QualityScore
				merged.QualityGrade = existing.QualityGrade
				merged.QualitySummary = existing.QualitySummary
				merged.QualityCheckedAt = existing.QualityCheckedAt
				merged.QualityCFRay = existing.QualityCFRay
			}
		}
	}

	if err := s.proxyLatencyCache.SetProxyLatency(ctx, proxyID, &merged); err != nil {
		logger.LegacyPrintf("service.admin", "Warning: store proxy latency cache failed: %v", err)
	}
}
