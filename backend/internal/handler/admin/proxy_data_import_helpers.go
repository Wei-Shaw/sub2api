package admin

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type pendingProxyFallbackImport struct {
	ProxyID     int64
	Item        DataProxy
	ProxyKey    string
	CurrentMode string
	CurrentID   *int64
}

func buildProxyLookup(proxies []service.Proxy) map[string]service.Proxy {
	proxyByKey := make(map[string]service.Proxy, len(proxies))
	for i := range proxies {
		p := proxies[i]
		key := buildProxyKeyWithBasePath(p.Protocol, p.Host, p.Port, p.Username, p.Password, p.BasePath)
		proxyByKey[key] = p
		if p.BasePath == "" {
			proxyByKey[buildProxyKey(p.Protocol, p.Host, p.Port, p.Username, p.Password)] = p
		}
	}
	return proxyByKey
}

func proxyKeyForDataProxy(item DataProxy) string {
	if item.ProxyKey != "" {
		return item.ProxyKey
	}
	return buildProxyKeyWithBasePath(item.Protocol, item.Host, item.Port, item.Username, item.Password, item.BasePath)
}

func proxyExpiryWarnInput(days *int) service.OptionalIntInput {
	if days == nil {
		return service.OptionalIntInput{}
	}
	return service.OptionalIntInput{Set: true, Value: *days}
}

func proxyExpiresAtInput(field DataNullableInt64) service.OptionalTimeInput {
	if !field.Set {
		return service.OptionalTimeInput{}
	}
	var expiresAt *time.Time
	if field.Value != nil && *field.Value > 0 {
		t := time.Unix(*field.Value, 0).UTC()
		expiresAt = &t
	}
	return service.OptionalTimeInput{Set: true, Value: expiresAt}
}

func proxyExpiresAtValue(field DataNullableInt64) *time.Time {
	if field.Value == nil || *field.Value <= 0 {
		return nil
	}
	t := time.Unix(*field.Value, 0).UTC()
	return &t
}

func dataProxyWarnDaysValue(item DataProxy) int {
	if item.ExpiryWarnDays == nil {
		return 0
	}
	return *item.ExpiryWarnDays
}

func buildImportProxyNameIndex(existing []service.Proxy, incoming []DataProxy) map[string]int64 {
	proxyNameToID := make(map[string]int64, len(existing)+len(incoming))
	for i := range existing {
		if existing[i].Name != "" {
			proxyNameToID[existing[i].Name] = existing[i].ID
		}
	}
	for i := range incoming {
		if incoming[i].Name != "" {
			if _, ok := proxyNameToID[incoming[i].Name]; ok {
				continue
			}
			proxyNameToID[incoming[i].Name] = 0
		}
	}
	return proxyNameToID
}

func registerImportedProxyName(proxyNameToID map[string]int64, item DataProxy, proxy *service.Proxy) {
	if proxy == nil {
		return
	}
	if proxy.Name != "" {
		proxyNameToID[proxy.Name] = proxy.ID
	}
	if item.Name != "" {
		proxyNameToID[item.Name] = proxy.ID
	}
}

func registerProxyKey(proxyByKey map[string]service.Proxy, key string, proxy *service.Proxy) {
	if proxy == nil {
		return
	}
	proxyByKey[key] = *proxy
	if proxy.BasePath == "" {
		proxyByKey[buildProxyKey(proxy.Protocol, proxy.Host, proxy.Port, proxy.Username, proxy.Password)] = *proxy
	}
}

func proxyImportNeedsUpdate(input *service.UpdateProxyInput) bool {
	return input.Status != "" ||
		input.ExpiresAt.Set ||
		input.FallbackMode.Set ||
		input.BackupProxyID.Set ||
		input.ExpiryWarnDays.Set
}

func updateProxyInputForImport(existing service.Proxy, item DataProxy, normalizedStatus string) *service.UpdateProxyInput {
	input := &service.UpdateProxyInput{
		ExpiresAt:      proxyExpiresAtInput(item.ExpiresAt),
		ExpiryWarnDays: proxyExpiryWarnInput(item.ExpiryWarnDays),
	}
	if normalizedStatus != "" && normalizedStatus != existing.Status {
		input.Status = normalizedStatus
	}
	return input
}

func createProxyInputForImport(item DataProxy) *service.CreateProxyInput {
	return &service.CreateProxyInput{
		Name:           defaultProxyName(item.Name),
		Protocol:       item.Protocol,
		Host:           item.Host,
		Port:           item.Port,
		Username:       item.Username,
		Password:       item.Password,
		BasePath:       item.BasePath,
		ExpiresAt:      proxyExpiresAtValue(item.ExpiresAt),
		FallbackMode:   service.FallbackModeNone,
		ExpiryWarnDays: proxyExpiryWarnInput(item.ExpiryWarnDays),
	}
}

func applyImportedProxyStatus(
	ctx context.Context,
	adminService service.AdminService,
	proxy *service.Proxy,
	normalizedStatus string,
	item DataProxy,
	key string,
	result *DataImportResult,
) {
	if proxy == nil || normalizedStatus == "" || normalizedStatus == proxy.Status {
		return
	}
	if _, err := adminService.UpdateProxy(ctx, proxy.ID, &service.UpdateProxyInput{Status: normalizedStatus}); err != nil {
		result.Errors = append(result.Errors, DataImportError{
			Kind:     "proxy",
			Name:     item.Name,
			ProxyKey: key,
			Message:  "update status failed: " + err.Error(),
		})
	}
}

func pendingFallbackForImportedProxy(proxy service.Proxy, item DataProxy, key string) pendingProxyFallbackImport {
	return pendingProxyFallbackImport{
		ProxyID:     proxy.ID,
		Item:        item,
		ProxyKey:    key,
		CurrentMode: proxy.FallbackMode,
		CurrentID:   proxy.BackupProxyID,
	}
}

func applyPendingProxyFallbacks(
	ctx context.Context,
	adminService service.AdminService,
	pending []pendingProxyFallbackImport,
	proxyNameToID map[string]int64,
	result *DataImportResult,
) {
	for _, item := range pending {
		mode := item.Item.FallbackMode
		if mode == "" {
			continue
		}

		var backupProxyID *int64
		if item.Item.BackupProxyName != "" {
			id, ok := proxyNameToID[item.Item.BackupProxyName]
			if !ok || id <= 0 {
				result.Errors = append(result.Errors, DataImportError{
					Kind:     "proxy",
					Name:     item.Item.Name,
					ProxyKey: item.ProxyKey,
					Message:  fmt.Sprintf("backup_proxy_name %q not found", item.Item.BackupProxyName),
				})
				continue
			}
			backupProxyID = &id
		}

		if mode == item.CurrentMode && sameInt64Ptr(backupProxyID, item.CurrentID) {
			continue
		}

		_, err := adminService.UpdateProxy(ctx, item.ProxyID, &service.UpdateProxyInput{
			FallbackMode:  service.OptionalStringInput{Set: true, Value: mode},
			BackupProxyID: service.OptionalInt64Input{Set: true, Value: backupProxyID},
		})
		if err != nil {
			result.Errors = append(result.Errors, DataImportError{
				Kind:     "proxy",
				Name:     item.Item.Name,
				ProxyKey: item.ProxyKey,
				Message:  "update fallback failed: " + err.Error(),
			})
		}
	}
}

func sameInt64Ptr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func loadFallbackProxyNames(ctx context.Context, adminService service.AdminService, proxies []service.Proxy) (map[int64]string, error) {
	proxyNameByID := make(map[int64]string, len(proxies))
	missingIDs := make(map[int64]struct{})
	for i := range proxies {
		proxyNameByID[proxies[i].ID] = proxies[i].Name
	}
	for i := range proxies {
		if proxies[i].BackupProxyID != nil {
			id := *proxies[i].BackupProxyID
			if _, ok := proxyNameByID[id]; !ok {
				missingIDs[id] = struct{}{}
			}
		}
	}
	if len(missingIDs) == 0 {
		return proxyNameByID, nil
	}

	ids := make([]int64, 0, len(missingIDs))
	for id := range missingIDs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	backupProxies, err := adminService.GetProxiesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range backupProxies {
		proxyNameByID[backupProxies[i].ID] = backupProxies[i].Name
	}
	return proxyNameByID, nil
}

func appendMissingFallbackProxies(ctx context.Context, adminService service.AdminService, proxies []service.Proxy) ([]service.Proxy, error) {
	seen := make(map[int64]struct{}, len(proxies))
	queue := make([]int64, 0)
	for i := range proxies {
		seen[proxies[i].ID] = struct{}{}
	}
	for i := range proxies {
		if proxies[i].BackupProxyID == nil {
			continue
		}
		id := *proxies[i].BackupProxyID
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		queue = append(queue, id)
	}
	if len(queue) == 0 {
		return proxies, nil
	}

	for len(queue) > 0 {
		ids := append([]int64(nil), queue...)
		queue = queue[:0]
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

		extra, err := adminService.GetProxiesByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		for i := range extra {
			proxy := extra[i]
			proxies = append(proxies, proxy)
			if proxy.BackupProxyID == nil {
				continue
			}
			id := *proxy.BackupProxyID
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			queue = append(queue, id)
		}
	}
	return proxies, nil
}
