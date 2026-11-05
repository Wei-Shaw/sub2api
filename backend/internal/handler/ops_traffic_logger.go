package handler

import (
	"context"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const opsTrafficRegisteredKey = "ops_traffic_registered"

var opsTrafficLastEnqueueFailureLog atomic.Int64

// registerOpsTrafficEvent runs synchronously after c.Next, before any error-log
// filter. Only scalar copies escape the gin lifetime; no callback captures c,
// an API key, its group pointer, headers, or a request body.
func registerOpsTrafficEvent(c *gin.Context, ops *service.OpsService) {
	if c == nil || c.Request == nil || ops == nil || c.GetBool(opsTrafficRegisteredKey) || !ops.IsMonitoringEnabled(c.Request.Context()) {
		return
	}
	c.Set(opsTrafficRegisteredKey, true)
	ctx := c.Request.Context()
	collector := traffic.FromContext(ctx)
	apiKey := getOpsAPIKey(c)
	event := &service.OpsTrafficEvent{EventID: uuid.NewString(), Platform: resolveOpsPlatform(ctx, apiKey, "")}
	if requestID, ok := ctx.Value(ctxkey.RequestID).(string); ok {
		event.RequestID = requestID
	}
	if apiKey != nil && apiKey.GroupID != nil {
		groupID := *apiKey.GroupID
		event.GroupID = &groupID
	}
	if account, ok := c.Get(opsAccountIDKey); ok {
		if accountID, ok := account.(int64); ok && accountID > 0 {
			event.AccountID = &accountID
		}
	}
	// Do not infer an upstream platform from a URL before routing succeeded.
	event.Platform = strings.ToLower(strings.TrimSpace(event.Platform))
	traffic.AfterFinalize(ctx, func(snapshot *traffic.Usage) {
		// A WebSocket/hijacked request or a legacy route without a collector
		// has no HTTP measurement. Do not fabricate a zero-byte event.
		if snapshot == nil {
			return
		}
		// Earlier callbacks may block on usage persistence. Use the collector's
		// freeze timestamp, not this callback's wall time, for hourly buckets.
		event.CreatedAt = collector.FinalizedAt()
		if event.CreatedAt.IsZero() {
			event.CreatedAt = time.Now().UTC()
		}
		event.Traffic = snapshot
		// Cancellation of the client must not erase the failed HTTP request.
		if err := ops.RecordTrafficEvent(context.Background(), event); err != nil {
			// Saturation may reject many requests at once. Keep every failed
			// counter, but do not turn a full telemetry queue into a log flood.
			now := time.Now().Unix()
			last := opsTrafficLastEnqueueFailureLog.Load()
			if now-last >= 30 && opsTrafficLastEnqueueFailureLog.CompareAndSwap(last, now) {
				log.Printf("[OpsTrafficLedger] request events not queued; coverage is incomplete (see traffic.ingestion)")
			}
		}
	})
}
