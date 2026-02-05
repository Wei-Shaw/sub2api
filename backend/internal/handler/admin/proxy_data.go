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
