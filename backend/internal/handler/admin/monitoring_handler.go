package admin

import (
	"log"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type MonitoringHandler struct {
	monitoringService *service.MonitoringService
}

func NewMonitoringHandler(monitoringService *service.MonitoringService) *MonitoringHandler {
	return &MonitoringHandler{monitoringService: monitoringService}
}

func (h *MonitoringHandler) GetOverview(c *gin.Context) {
	overview, err := h.monitoringService.GetOverview(c.Request.Context())
	if err != nil {
		log.Printf("[Monitoring] GetOverview failed: %v", err)
		response.InternalError(c, "Failed to get monitoring data")
		return
	}
	response.Success(c, overview)
}

func (h *MonitoringHandler) GetSummary(c *gin.Context) {
	data, err := h.monitoringService.GetSummary(c.Request.Context())
	if err != nil {
		log.Printf("[Monitoring] GetSummary failed: %v", err)
		response.InternalError(c, "Failed to get summary data")
		return
	}
	response.Success(c, data)
}

func (h *MonitoringHandler) GetGroupModels(c *gin.Context) {
	data, err := h.monitoringService.GetGroupModels(c.Request.Context())
	if err != nil {
		log.Printf("[Monitoring] GetGroupModels failed: %v", err)
		response.InternalError(c, "Failed to get group model data")
		return
	}
	response.Success(c, data)
}

func (h *MonitoringHandler) GetModelLatency(c *gin.Context) {
	data, err := h.monitoringService.GetModelLatency(c.Request.Context())
	if err != nil {
		log.Printf("[Monitoring] GetModelLatency failed: %v", err)
		response.InternalError(c, "Failed to get model latency data")
		return
	}
	response.Success(c, data)
}
