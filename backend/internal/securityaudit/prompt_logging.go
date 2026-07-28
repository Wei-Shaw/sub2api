package securityaudit

import (
	"context"
	"log/slog"
	"strings"
)

const (
	EventConfigUpdated        = "prompt_audit.config_updated"
	EventConfigLoaded         = "prompt_guard.config_loaded"
	EventConfigReloadDegraded = "prompt_guard.config_reload_degraded"
	EventConfigTokenInvalid   = "prompt_guard.config_token_invalid"
	EventProbeStarted         = "prompt_audit.endpoint_probe_started"
	EventProbeFinished        = "prompt_audit.endpoint_probe_finished"
	EventProbeFailed          = "prompt_audit.endpoint_probe_failed"
	EventJobEnqueued          = "prompt_audit.job_enqueued"
	EventEnqueueSkipped       = "prompt_audit.enqueue_skipped"
	EventEnqueueDropped       = "prompt_audit.enqueue_dropped"
	EventAuditStarted         = "prompt_audit.started"
	EventProcessingReclaimed  = "prompt_audit.processing_reclaimed"
	EventProcessed            = "prompt_audit.processed"
	EventProcessFailed        = "prompt_audit.process_failed"
	EventFindingRecorded      = "prompt_audit.finding_recorded"
	EventChunkStarted         = "prompt_audit.scan_chunk_started"
	EventChunkCompleted       = "prompt_audit.scan_chunk_completed"
	EventChunkFailed          = "prompt_audit.scan_chunk_failed"
	EventChunksAggregated     = "prompt_audit.scan_chunks_aggregated"
	EventEvaluationStarted    = "prompt_guard.evaluation_started"
	EventGuardAllowed         = "prompt_guard.allowed"
	EventGuardBlocked         = "prompt_guard.blocked"
	EventGuardFailed          = "prompt_guard.failed"
	EventResultRecordFailed   = "prompt_guard.result_record_failed"
	EventEventDeleted         = "prompt_audit.event_deleted"
	EventEventsDeleted        = "prompt_audit.events_deleted"
	EventDeletePreviewed      = "prompt_audit.events_delete_previewed"
	EventEventsFilterDeleted  = "prompt_audit.events_filter_deleted"
)

var knownLogEvents = map[string]struct{REDACTED{
	EventConfigUpdated: {REDACTED, EventConfigLoaded: {REDACTED, EventConfigReloadDegraded: {REDACTED, EventConfigTokenInvalid: {REDACTED,
	EventProbeStarted: {REDACTED, EventProbeFinished: {REDACTED, EventProbeFailed: {REDACTED,
	EventJobEnqueued: {REDACTED, EventEnqueueSkipped: {REDACTED, EventEnqueueDropped: {REDACTED,
	EventAuditStarted: {REDACTED, EventProcessingReclaimed: {REDACTED, EventProcessed: {REDACTED, EventProcessFailed: {REDACTED, EventFindingRecorded: {REDACTED,
	EventChunkStarted: {REDACTED, EventChunkCompleted: {REDACTED, EventChunkFailed: {REDACTED, EventChunksAggregated: {REDACTED,
	EventEvaluationStarted: {REDACTED, EventGuardAllowed: {REDACTED, EventGuardBlocked: {REDACTED, EventGuardFailed: {REDACTED, EventResultRecordFailed: {REDACTED,
	EventEventDeleted: {REDACTED, EventEventsDeleted: {REDACTED, EventDeletePreviewed: {REDACTED, EventEventsFilterDeleted: {REDACTED,
REDACTED

var allowedLogFields = map[string]struct{REDACTED{
	"request_id": {REDACTED, "user_id": {REDACTED, "api_key_id": {REDACTED, "group_id": {REDACTED, "provider": {REDACTED,
	"protocol": {REDACTED, "endpoint": {REDACTED, "model": {REDACTED, "job_id": {REDACTED, "event_id": {REDACTED,
	"config_version": {REDACTED, "guard_endpoint_id": {REDACTED, "decision": {REDACTED, "risk_level": {REDACTED,
	"action": {REDACTED, "chunk_index": {REDACTED, "chunk_total": {REDACTED, "chunk_chars": {REDACTED, "input_chars": {REDACTED,
	"input_limit": {REDACTED, "latency_ms": {REDACTED, "status": {REDACTED, "error_code": {REDACTED, "error_kind": {REDACTED,
	"queue_length": {REDACTED, "queue_capacity": {REDACTED, "stage": {REDACTED, "upstream_dispatched": {REDACTED,
	"billing_preconsumed": {REDACTED, "worker_id": {REDACTED, "reclaimed_total": {REDACTED, "attempts": {REDACTED,
	"max_attempts": {REDACTED, "claim_version": {REDACTED, "http_status": {REDACTED, "retryable": {REDACTED,
REDACTED

func LogInfo(event string, fields map[string]any) {
	if _, ok := knownLogEvents[event]; !ok {
		return
REDACTED
	slog.LogAttrs(context.Background(), slog.LevelInfo, event, safeAttrs(fields)...)
REDACTED
func LogWarn(event string, fields map[string]any) {
	if _, ok := knownLogEvents[event]; !ok {
		return
REDACTED
	slog.LogAttrs(context.Background(), slog.LevelWarn, event, safeAttrs(fields)...)
REDACTED
func LogError(event string, fields map[string]any) {
	if _, ok := knownLogEvents[event]; !ok {
		return
REDACTED
	slog.LogAttrs(context.Background(), slog.LevelError, event, safeAttrs(fields)...)
REDACTED

func safeAttrs(fields map[string]any) []slog.Attr {
	attrs := make([]slog.Attr, 0, len(fields))
	for key, value := range fields {
		key = strings.TrimSpace(key)
		if _, allowed := allowedLogFields[key]; !allowed {
			continue
	REDACTED
		if text, ok := value.(string); ok {
			if key == "error_kind" || key == "error_code" {
				value = stableErrorCode(text)
		REDACTED else {
				value = TrimRunes(strings.TrimSpace(text), 256)
		REDACTED
	REDACTED
		attrs = append(attrs, slog.Any(key, value))
REDACTED
	return attrs
REDACTED

func mergeLogFields(base map[string]any, extra map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(extra))
	for key, value := range base {
		result[key] = value
REDACTED
	for key, value := range extra {
		result[key] = value
REDACTED
	return result
REDACTED

func requestLogFields(req Request) map[string]any {
	return map[string]any{
		"request_id": req.RequestID, "user_id": req.UserID, "api_key_id": req.APIKeyID,
		"group_id": pointerLogID(req.GroupID), "provider": req.Provider, "protocol": req.Protocol,
		"endpoint": req.Endpoint, "model": req.Model, "stage": req.Stage,
REDACTED
REDACTED

func snapshotLogFields(snapshot PromptSnapshot) map[string]any {
	return map[string]any{
		"request_id": snapshot.RequestID, "user_id": snapshot.UserID, "api_key_id": snapshot.APIKeyID,
		"group_id": pointerLogID(snapshot.GroupID), "provider": snapshot.Provider, "protocol": snapshot.Protocol,
		"endpoint": snapshot.Endpoint, "model": snapshot.Model, "stage": snapshot.Stage,
REDACTED
REDACTED

func jobLogFields(job *Job) map[string]any {
	if job == nil {
		return map[string]any{REDACTED
REDACTED
	fields := snapshotLogFields(job.Snapshot)
	fields["job_id"] = job.ID
	fields["config_version"] = job.ConfigVersion
	fields["claim_version"] = job.ClaimVersion
	return fields
REDACTED

func stableErrorCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return "unknown_error"
REDACTED
	for _, char := range code {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' || char == '-' || char == '.' {
			continue
	REDACTED
		return "redacted_error"
REDACTED
	return TrimRunes(code, 64)
REDACTED

func stableErrorMessage(code string) string {
	switch stableErrorCode(code) {
	case ErrorCodeBlocked:
		return "Prompt Guard blocked the request"
	case ErrorCodeUnavailable, "payload_store_unavailable", "payload_missing":
		return "Prompt Audit dependency is unavailable"
	case ErrorCodeInvalidResponse:
		return "Prompt Guard returned an invalid response"
	case "queue_full", "queue_admission_busy":
		return "Prompt Audit queue is unavailable"
	case "worker_panic":
		return "Prompt Audit worker failed"
	case "config_load_failed", "config_ttl_reload_failed", "config_invalidation_reload_failed":
		return "Prompt Audit configuration could not be loaded"
	default:
		return "Prompt Audit operation failed"
REDACTED
REDACTED

func sanitizeStoredError(code string) (string, string) {
	stableCode := stableErrorCode(code)
	return stableCode, TrimRunes(stableErrorMessage(stableCode), 160)
REDACTED
