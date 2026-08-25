package service

import "time"

// sharedAccountHealthStats is deliberately process-local and account-scoped.
// It gives non-OpenAI gateway paths the same lightweight health signal as the
// OpenAI scheduler without changing persistence or existing circuit state.
var sharedAccountHealthStats = newOpenAIAccountRuntimeStats()

func reportSharedAccountHealth(accountID int64, success bool, observedErr error) {
	reportSharedAccountHealthWithLatency(accountID, success, observedErr, 0)
}

func reportSharedAccountHealthWithLatency(accountID int64, success bool, observedErr error, latencyMs int64) {
	if accountID <= 0 {
		return
	}
	category := openAIAccountFailureCategory("")
	if observedErr != nil {
		category = classifyOpenAIAccountFailure(observedErr)
	}
	var firstTokenMs *int
	if latencyMs > 0 && latencyMs <= int64(^uint(0)>>1) {
		value := int(latencyMs)
		firstTokenMs = &value
	}
	sharedAccountHealthStats.reportWithCategory(accountID, success, firstTokenMs, category)
}

func reportSharedAccountProbe(accountID int64, success bool, latencyMs int64) {
	if accountID <= 0 {
		return
	}
	sharedAccountHealthStats.reportProbeWithLatency(accountID, success, latencyMs, time.Now())
}

func sharedAccountHealthSnapshot(accountID int64) (errorRate, latencyMs float64, hasSample bool) {
	now := time.Now()
	errorRate, latencyMs, _ = sharedAccountHealthStats.healthSnapshotAt(accountID, now)
	return errorRate, latencyMs, sharedAccountHealthStats.hasSampleAt(accountID, now)
}
