package service

import (
	"context"
	"maps"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
)

// writeUsageLogBestEffort defers only persistence until the HTTP entity-body
// counters are final. Billing and the usage worker's reservation/drop policy
// retain their existing timing. In particular, waiting for Finalize here would
// deadlock the no-worker/overflow synchronous fallback inside the handler.
func writeUsageLogBestEffort(ctx context.Context, repo UsageLogRepository, usageLog *UsageLog, logKey string) {
	if repo == nil || usageLog == nil {
		return
	}
	collector := traffic.FromContext(ctx)
	if collector == nil {
		// Direct service/background calls retain their old path. WS records
		// without an HTTP collector cannot carry HTTP entity-body traffic.
		if usageLog.OpenAIWSMode || usageLog.RequestType == RequestTypeWSV2 || usageLog.RequestType == RequestTypeLive {
			if usageLog.Traffic != nil {
				copy := *usageLog
				copy.Traffic = nil
				usageLog = &copy
			}
		}
		writeUsageLogBestEffortNow(ctx, repo, usageLog, logKey)
		return
	}

	// No mutable result/input fields or gin.Context escape into the callback.
	// The collector supplies a separate deep snapshot for every callback, only
	// after response Close (including compressed trailers) has completed.
	frozen := snapshotUsageLogForTraffic(usageLog)
	persist := func(measured *traffic.Usage) {
		// Before this change a repository panic belonged to the usage task's
		// recovery boundary. Preserve isolation when we instead run on Finalize,
		// so one broken writer cannot suppress another completion callback.
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.LegacyPrintf(logKey, "Create usage log after traffic finalize panicked: %v", recovered)
			}
		}()
		if measured != nil && (frozen.OpenAIWSMode || frozen.RequestType == RequestTypeWSV2) {
			// OpenAIWSMode also describes HTTP ingress forwarded over upstream
			// WS. Keep the real downstream HTTP body, not upstream WS messages.
			measured.Upstream = nil
		}
		frozen.Traffic = measured
		writeUsageLogBestEffortNow(ctx, repo, frozen, logKey)
	}
	if collector.Finalized() {
		// Usually the billing worker starts after the handler completed. Keep
		// persistence on that worker rather than enqueueing a second task.
		traffic.AfterFinalize(ctx, persist)
		return
	}
	pool := UsageRecordWorkerPoolFromContext(ctx)
	traffic.AfterFinalize(ctx, func(measured *traffic.Usage) {
		// This callback can run on the HTTP finalizer: never put healthy-pool
		// database latency back on that goroutine. A race with Finalize between
		// the check and registration only adds one queue hop; it cannot repeat
		// the write. Full/stopped/nil pools keep mandatory synchronous fallback.
		pool.SubmitMandatory(func(context.Context) { persist(measured) })
	})
}

// snapshotUsageLogForTraffic freezes persisted optional scalar fields and the
// image breakdown, which otherwise alias ForwardResult/APIKey/input data.
// Relation objects are read-only and are not consumed by usage-log inserts.
func snapshotUsageLogForTraffic(source *UsageLog) *UsageLog {
	copy := *source
	copy.UpstreamModel = copyUsageLogValue(source.UpstreamModel)
	copy.UpstreamResponseModel = copyUsageLogValue(source.UpstreamResponseModel)
	copy.UpstreamModelMismatch = copyUsageLogValue(source.UpstreamModelMismatch)
	copy.ChannelID = copyUsageLogValue(source.ChannelID)
	copy.ModelMappingChain = copyUsageLogValue(source.ModelMappingChain)
	copy.BillingTier = copyUsageLogValue(source.BillingTier)
	copy.BillingMode = copyUsageLogValue(source.BillingMode)
	copy.ServiceTier = copyUsageLogValue(source.ServiceTier)
	copy.ReasoningEffort = copyUsageLogValue(source.ReasoningEffort)
	copy.RequestedReasoningEffort = copyUsageLogValue(source.RequestedReasoningEffort)
	copy.InboundEndpoint = copyUsageLogValue(source.InboundEndpoint)
	copy.UpstreamEndpoint = copyUsageLogValue(source.UpstreamEndpoint)
	copy.GroupID = copyUsageLogValue(source.GroupID)
	copy.SubscriptionID = copyUsageLogValue(source.SubscriptionID)
	copy.AccountRateMultiplier = copyUsageLogValue(source.AccountRateMultiplier)
	copy.AccountStatsCost = copyUsageLogValue(source.AccountStatsCost)
	copy.DurationMs = copyUsageLogValue(source.DurationMs)
	copy.FirstTokenMs = copyUsageLogValue(source.FirstTokenMs)
	copy.UserAgent = copyUsageLogValue(source.UserAgent)
	copy.IPAddress = copyUsageLogValue(source.IPAddress)
	copy.SessionID = copyUsageLogValue(source.SessionID)
	copy.UpstreamRequestID = copyUsageLogValue(source.UpstreamRequestID)
	copy.ImageSize = copyUsageLogValue(source.ImageSize)
	copy.ImageInputSize = copyUsageLogValue(source.ImageInputSize)
	copy.ImageOutputSize = copyUsageLogValue(source.ImageOutputSize)
	copy.ImageSizeSource = copyUsageLogValue(source.ImageSizeSource)
	copy.ImageSizeBreakdown = maps.Clone(source.ImageSizeBreakdown)
	copy.MediaType = copyUsageLogValue(source.MediaType)
	copy.VideoResolution = copyUsageLogValue(source.VideoResolution)
	copy.VideoDurationSeconds = copyUsageLogValue(source.VideoDurationSeconds)
	copy.Traffic = nil // only the final collector snapshot is authoritative
	return &copy
}

func copyUsageLogValue[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
