package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/gin-gonic/gin"
)

const accountStatsBatchBodyLimit = 64 * 1024

type accountStatsBatchRequest struct {
	Windows []usagestats.AccountStatsWindow `json:"windows"`
}

// GetBatchAccountWindowStats is a read-only POST for independently timed cycles.
func (h *DashboardHandler) GetBatchAccountWindowStats(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, accountStatsBatchBodyLimit)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var req accountStatsBatchRequest
	if err := decoder.Decode(&req); err != nil {
		writeAccountStatsBodyError(c, err)
		return
	}
	// Decode to EOF so trailing payloads cannot evade validation or the body cap.
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeAccountStatsBodyError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	stats, err := h.dashboardService.GetAccountStatsByWindows(ctx, req.Windows)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"stats": stats})
}

func writeAccountStatsBodyError(c *gin.Context, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		response.Error(c, http.StatusRequestEntityTooLarge, "Account statistics request exceeds 64 KiB")
		return
	}
	response.BadRequest(c, "Invalid account statistics request: use windows with account_id, start_at and end_at")
}
