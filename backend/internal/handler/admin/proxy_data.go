package admin

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ExportData exports proxy-only data for migration.
func (h *ProxyHandler) ExportData(c *gin.Context) {
	ctx := c.Request.Context()

	protocol := c.Query("protocol")
	status := c.Query("status")
	search := strings.TrimSpace(c.Query("search"))
	if len(search) > 100 {
		search = search[:100]
REDACTED

	proxies, err := h.listProxiesFiltered(ctx, protocol, status, search)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	dataProxies := make([]DataProxy, 0, len(proxies))
	for i := range proxies {
		p := proxies[i]
		key := buildProxyKey(p.Protocol, p.Host, p.Port, p.Username, p.Password)
		dataProxies = append(dataProxies, DataProxy{
			ProxyKey: key,
			Name:     p.Name,
			Protocol: p.Protocol,
			Host:     p.Host,
			Port:     p.Port,
			Username: p.Username,
			Password: p.Password,
			Status:   p.Status,
	REDACTED)
REDACTED

	payload := DataPayload{
		Type:       dataType,
		Version:    dataVersion,
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

	existingProxies, err := h.listProxiesFiltered(ctx, "", "", "")
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	proxyByKey := make(map[string]service.Proxy, len(existingProxies))
	for i := range existingProxies {
		p := existingProxies[i]
		key := buildProxyKey(p.Protocol, p.Host, p.Port, p.Username, p.Password)
		proxyByKey[key] = p
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

		if existing, ok := proxyByKey[key]; ok {
			result.ProxyReused++
			if item.Status != "" && item.Status != existing.Status {
				if _, err := h.adminService.UpdateProxy(ctx, existing.ID, &service.UpdateProxyInput{Status: item.StatusREDACTED); err != nil {
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

		created, err := h.adminService.CreateProxy(ctx, &service.CreateProxyInput{
			Name:     defaultProxyName(item.Name),
			Protocol: item.Protocol,
			Host:     item.Host,
			Port:     item.Port,
			Username: item.Username,
			Password: item.Password,
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

		if item.Status != "" && item.Status != created.Status {
			if _, err := h.adminService.UpdateProxy(ctx, created.ID, &service.UpdateProxyInput{Status: item.StatusREDACTED); err != nil {
				result.Errors = append(result.Errors, DataImportError{
					Kind:     "proxy",
					Name:     item.Name,
					ProxyKey: key,
					Message:  "update status failed: " + err.Error(),
			REDACTED)
		REDACTED
	REDACTED
		latencyProbeIDs = append(latencyProbeIDs, created.ID)
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

func (h *ProxyHandler) listProxiesFiltered(ctx context.Context, protocol, status, search string) ([]service.Proxy, error) {
	page := 1
	pageSize := dataPageCap
	var out []service.Proxy
	for {
		items, total, err := h.adminService.ListProxies(ctx, page, pageSize, protocol, status, search)
		if err != nil {
			return nil, err
	REDACTED
		out = append(out, items...)
		if len(out) >= int(total) || len(items) == 0 {
			break
	REDACTED
		page++
REDACTED
	return out, nil
REDACTED
