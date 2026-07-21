package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetUpstreamBillingRateHistory returns persisted change events for one account.
// It never starts an upstream request.
func (h *AccountHandler) GetUpstreamBillingRateHistory(c *gin.Context) {
	if h.upstreamBillingProbe == nil {
		response.ErrorFrom(c, service.ErrUpstreamBillingProbeUnavailable)
		return
	}
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	days, ok := positiveQueryInt(c, "days", 90)
	if !ok {
		response.ErrorFrom(c, service.ErrUpstreamBillingRateHistoryRangeInvalid)
		return
	}
	limit, ok := positiveQueryInt(c, "limit", service.UpstreamBillingRateHistoryMaxEvents)
	if !ok {
		response.ErrorFrom(c, service.ErrUpstreamBillingRateHistoryRangeInvalid)
		return
	}

	history, err := h.upstreamBillingProbe.GetAccountRateHistory(c.Request.Context(), accountID, days, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-cache")
	etag := buildUpstreamBillingRateHistoryETag(history)
	if etag != "" {
		c.Header("ETag", etag)
		c.Header("Vary", "If-None-Match")
		if ifNoneMatchMatched(c.GetHeader("If-None-Match"), etag) {
			c.Status(http.StatusNotModified)
			return
		}
	}
	response.Success(c, history)
}

func positiveQueryInt(c *gin.Context, key string, fallback int) (int, bool) {
	raw := c.Query(key)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	return value, err == nil && value > 0
}

func buildUpstreamBillingRateHistoryETag(history *service.UpstreamBillingRateHistory) string {
	raw, err := json.Marshal(history)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "\"" + hex.EncodeToString(sum[:]) + "\""
}
