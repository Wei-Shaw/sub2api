package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ticketproxy"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) GetTicketHarvestStatus(c *gin.Context) {
	status, err := h.settingService.GetOpenAICodexTicketRuntimeStatus(c.Request.Context())
	if err != nil {
		response.InternalError(c, "无法读取打票运行设置")
		return
	}
	response.Success(c, status)
}

func (h *SettingHandler) GetTicketProxyPool(c *gin.Context) {
	status, err := ticketproxy.Default.Status()
	if err != nil {
		response.InternalError(c, "无法读取打票代理池")
		return
	}
	response.Success(c, status)
}

func (h *SettingHandler) ImportTicketProxyPool(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	var req struct {
		Proxies []string `json:"proxies"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "导入内容无效或文件超过 2 MB")
		return
	}
	proxies, duplicates, err := ticketproxy.Parse(req.Proxies)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	result, err := ticketproxy.Default.Import(proxies, duplicates)
	if err != nil {
		response.BadRequest(c, "无法导入：代理池最多保留 20000 条，请检查容量或服务器存储状态")
		return
	}
	response.Success(c, result)
}

func (h *SettingHandler) GetTicketProxyProvider(c *gin.Context) {
	status, err := ticketproxy.OnesProxy.Status()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, status)
}

func (h *SettingHandler) SaveTicketProxyProvider(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	var req ticketproxy.ProviderConfig
	if c.ShouldBindJSON(&req) != nil {
		response.BadRequest(c, "API 配置格式无效")
		return
	}
	status, err := ticketproxy.OnesProxy.Save(req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, status)
}

func (h *SettingHandler) FetchTicketProxyProvider(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	req := struct {
		Count int `json:"count"`
	}{Count: 2000}
	if c.ShouldBindJSON(&req) != nil {
		response.BadRequest(c, "提取参数无效")
		return
	}
	result, err := ticketproxy.OnesProxy.Fetch(c.Request.Context(), req.Count)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, result)
}
