package service

import (
	"crypto/sha256"
	"encoding/binary"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

type codexAnalyticsEvent struct {
	EventType   string         `json:"event_type"`
	EventParams map[string]any `json:"event_params"`
	client      codexTelemetryClient
}

type codexThreadSpec struct {
	threadID       string
	sessionID      string
	parentThreadID any
	source         string
	subagentSource any
	model          string
	ephemeral      bool
}

type codexTurnSpec struct {
	threadID string
	turnID   string
	model    string
	effort   string
}

type codexToolSpec struct {
	terminal []byte
	itemID   string
	duration int64
	status   string
}

// newCodexAnalyticsEvent builds an analytics event with the send identity attached.
func newCodexAnalyticsEvent(profile codexTelemetryProfile, eventType string, params map[string]any) codexAnalyticsEvent {
	return codexAnalyticsEvent{EventType: eventType, EventParams: params, client: profile.client}
}

// buildCodexTelemetryProfile builds this turn's telemetry context from the
// outbound request. Session/thread/turn prefer staged fingerprint IDs, then
// rewritten outbound headers / client_metadata. Local affinity keys and row
// IDs are never used as session_id.
func buildCodexTelemetryProfile(client codexTelemetryClient, input codexTelemetryRequest) codexTelemetryProfile {
	metadata := parseCodexTelemetryTurnMetadata(input.body, input.headers)
	convergedSession, convergedThread, convergedTurn := "", "", ""
	if input.ids != nil {
		convergedSession = strings.TrimSpace(input.ids.sessionID)
		convergedThread = strings.TrimSpace(input.ids.threadID)
		convergedTurn = strings.TrimSpace(input.ids.turnID)
	}
	sessionID := firstNonEmptyString(
		convergedSession,
		headerOrEmpty(input.headers, "session_id"),
		headerOrEmpty(input.headers, "session-id"),
		gjson.GetBytes(input.body, "client_metadata.session_id").String(),
		metadata.Get("session_id").String(),
		input.sessionID,
	)
	threadID := firstNonEmptyString(
		convergedThread,
		headerOrEmpty(input.headers, "thread-id"),
		headerOrEmpty(input.headers, "thread_id"),
		gjson.GetBytes(input.body, "client_metadata.thread_id").String(),
		metadata.Get("thread_id").String(),
		sessionID,
	)
	if sessionID == "" {
		sessionID = newCodexTelemetryUUID()
	}
	if threadID == "" {
		threadID = sessionID
	}
	turnID := firstNonEmptyString(convergedTurn, metadata.Get("turn_id").String(), newCodexTelemetryUUID())
	effort := firstNonEmptyString(gjson.GetBytes(input.body, "reasoning.effort").String(), "medium")
	return codexTelemetryProfile{
		client: client, sessionID: sessionID, threadID: threadID, turnID: turnID,
		rootTurnID: firstNonEmptyString(metadata.Get("root_turn_id").String(), turnID),
		model:      firstNonEmptyString(gjson.GetBytes(input.body, "model").String(), "gpt-6-astra"),
		effort:     effort, serviceTier: firstNonEmptyString(gjson.GetBytes(input.body, "service_tier").String(), "default"),
		started: time.Now(), turnMetadata: metadata,
	}
}

func headerOrEmpty(headers http.Header, name string) string {
	if headers == nil {
		return ""
	}
	return strings.TrimSpace(headers.Get(name))
}

// parseCodexTelemetryTurnMetadata reads turn metadata from the body or headers.
func parseCodexTelemetryTurnMetadata(body []byte, headers map[string][]string) gjson.Result {
	raw := gjson.GetBytes(body, "client_metadata.x-codex-turn-metadata")
	if raw.Type == gjson.String && gjson.Valid(raw.String()) {
		return gjson.Parse(raw.String())
	}
	if value := strings.TrimSpace(firstHeaderValue(headers, codexTurnMetadataHeader)); gjson.Valid(value) {
		return gjson.Parse(value)
	}
	return gjson.Result{}
}

// firstHeaderValue returns the first header value, matching name case-insensitively.
func firstHeaderValue(headers map[string][]string, name string) string {
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

// codexInitializationEvents builds user, guardian, and title-thread init events.
func codexInitializationEvents(profile codexTelemetryProfile) []codexAnalyticsEvent {
	events := make([]codexAnalyticsEvent, 0, 4)
	if profile.firstThread {
		events = append(events, codexThreadInitialized(profile, codexThreadSpec{
			threadID: profile.threadID, sessionID: profile.sessionID, source: "user", model: profile.model,
		}))
	}
	guardianID := newCodexTelemetryUUID()
	events = append(events, codexThreadInitialized(profile, codexThreadSpec{
		threadID: guardianID, sessionID: profile.sessionID, parentThreadID: profile.threadID,
		source: "guardian_review", subagentSource: "guardian", model: "codex-auto-review",
	}))
	if !profile.firstThread {
		return events
	}
	titleThreadID := newCodexTelemetryUUID()
	events = append(events, codexThreadInitialized(profile, codexThreadSpec{
		threadID: titleThreadID, sessionID: titleThreadID, source: "thread_title", model: "gpt-5.6-luna", ephemeral: true,
	}))
	events = append(events, codexTitleTurnEvent(profile, titleThreadID))
	return events
}

// codexThreadInitialized builds a thread-initialized event from the given spec.
func codexThreadInitialized(profile codexTelemetryProfile, spec codexThreadSpec) codexAnalyticsEvent {
	appServer := codexAppServerClient(profile)
	if spec.source == "guardian_review" {
		appServer["rpc_transport"], appServer["experimental_api_enabled"] = "in_process", nil
	}
	params := map[string]any{
		"app_server_client": appServer, "created_at": profile.started.Unix(),
		"ephemeral": spec.ephemeral, "forked_from_thread_id": nil, "initialization_mode": "new",
		"model": spec.model, "parent_thread_id": spec.parentThreadID, "runtime": codexRuntime(profile),
		"session_id": spec.sessionID, "subagent_source": spec.subagentSource, "thread_id": spec.threadID,
		"thread_source": spec.source,
	}
	return newCodexAnalyticsEvent(profile, "codex_thread_initialized", params)
}

// codexTitleTurnEvent simulates the first-session title-generation turn.
func codexTitleTurnEvent(profile codexTelemetryProfile, threadID string) codexAnalyticsEvent {
	duration := int64(800 + simulatedInt(profile.turnID+":title", 1800))
	started := profile.started.Unix()
	turnID := newCodexTelemetryUUID()
	titleProfile := profile
	titleProfile.sessionID, titleProfile.threadID, titleProfile.rootTurnID = threadID, threadID, turnID
	titleProfile.dynamicTool, titleProfile.command, titleProfile.fileChange = false, false, false
	params := codexTurnEventBase(titleProfile, codexTurnSpec{threadID, turnID, "gpt-5.6-luna", "low"})
	params["approval_policy"], params["approvals_reviewer"] = "never", "user"
	params["before_first_sampling_ms"], params["sampling_ms"] = duration/2, duration/2
	params["completed_at"], params["duration_ms"] = started+duration/1000, duration
	params["ephemeral"], params["is_first_turn"] = true, true
	params["sandbox_policy"], params["status"] = "read_only", "completed"
	params["thread_source"], params["turn_trigger"] = "thread_title", "thread_title"
	params["reasoning_summary"] = nil
	params["workspace_kind"] = nil
	return newCodexAnalyticsEvent(profile, "codex_turn_event", params)
}

// codexTerminalEvents builds tool, hook, and main-turn completion events.
func codexTerminalEvents(profile codexTelemetryProfile, result codexTelemetryTerminal) []codexAnalyticsEvent {
	events := make([]codexAnalyticsEvent, 0, 9)
	if profile.command {
		events = append(events, codexCommandEvent(profile, result.body))
	}
	if profile.dynamicTool {
		events = append(events, codexDynamicToolEvent(profile, result.body))
	}
	if profile.fileChange {
		events = append(events, codexFileChangeEvent(profile, result.body), codexAcceptedLinesEvent(profile))
	}
	for range 4 {
		events = append(events, codexHookEvent(profile, result.status))
	}
	events = append(events, codexMainTurnEvent(profile, result))
	return events
}

// codexMainTurnEvent builds the main turn event from the real response status and usage.
func codexMainTurnEvent(profile codexTelemetryProfile, result codexTelemetryTerminal) codexAnalyticsEvent {
	now := time.Now()
	params := codexTurnEventBase(profile, codexTurnSpec{profile.threadID, profile.turnID, profile.model, profile.effort})
	response := codexTerminalResponse(result.body)
	params["approval_policy"] = firstNonEmptyString(profile.turnMetadata.Get("approval_policy").String(), "on-request")
	params["approvals_reviewer"], params["completed_at"] = "auto_review", now.Unix()
	params["duration_ms"] = codexNonNegativeMillis(now.Sub(profile.started))
	params["before_first_sampling_ms"] = elapsedMillis(profile.started, result.firstEvent, now)
	params["sampling_ms"] = elapsedMillis(result.firstEvent, now, now)
	params["after_last_sampling_ms"], params["between_sampling_overhead_ms"] = 0, 0
	params["status"], params["service_tier"] = result.status, firstNonEmptyString(response.Get("service_tier").String(), profile.serviceTier)
	params["sandbox_policy"] = firstNonEmptyString(profile.turnMetadata.Get("sandbox").String(), "workspace_write")
	params["explicit_client_interrupt_requested_at_ms"] = nil
	if result.status == "interrupted" {
		params["explicit_client_interrupt_requested_at_ms"] = now.UnixMilli()
	}
	setCodexTurnUsage(params, response)
	params["before_first_sampling_ms"] = elapsedMillis(profile.started, result.firstEvent, now)
	if !result.firstToken.IsZero() {
		params["sampling_ms"] = codexNonNegativeMillis(now.Sub(result.firstToken))
	}
	return newCodexAnalyticsEvent(profile, "codex_turn_event", params)
}

// codexTurnEventBase fills fields shared by Codex turn events.
func codexTurnEventBase(profile codexTelemetryProfile, spec codexTurnSpec) map[string]any {
	dynamicCount, commandCount, fileCount := boolInt(profile.dynamicTool), boolInt(profile.command), boolInt(profile.fileChange)
	return map[string]any{
		"app_server_client": codexAppServerClient(profile), "cache_write_input_tokens": 0,
		"cached_input_tokens": 0, "codex_error_http_status_code": nil, "codex_error_kind": nil,
		"codex_turn_source": nil, "collaboration_mode": "default", "compaction_ms": 0,
		"dynamic_tool_call_count": dynamicCount, "ephemeral": false, "file_change_count": fileCount,
		"guardian_v2_enabled": true, "image_generation_count": 0, "image_preparations": []any{},
		"initialization_mode": "new", "input_tokens": 0, "is_first_turn": profile.firstThread,
		"mcp_tool_call_count": 0, "model": spec.model, "model_provider": "openai", "num_input_images": 0,
		"output_tokens": 0, "parent_thread_id": nil, "personality": "pragmatic",
		"reasoning_effort": spec.effort, "reasoning_output_tokens": 0, "reasoning_summary": "detailed",
		"root_turn_id": profile.rootTurnID, "runtime": codexRuntime(profile), "sampling_request_count": 1,
		"sampling_retry_count": 0, "sandbox_network_access": false, "service_tier": profile.serviceTier,
		"session_id": profile.sessionID, "shell_command_count": commandCount, "started_at": profile.started.Unix(),
		"steer_count": 0, "subagent_source": nil, "subagent_tool_call_count": 0, "submission_type": nil,
		"thread_id": spec.threadID, "thread_source": "user", "tool_blocking_ms": 0,
		"total_tokens": 0, "total_tool_call_count": dynamicCount + commandCount + fileCount, "turn_error": nil,
		"turn_id": spec.turnID, "turn_trigger": "composer", "web_search_count": 0, "workspace_kind": "projectless",
	}
}

// setCodexTurnUsage copies Responses usage into turn event params.
func setCodexTurnUsage(params map[string]any, response gjson.Result) {
	usage := response.Get("usage")
	input, output := usage.Get("input_tokens").Int(), usage.Get("output_tokens").Int()
	total := usage.Get("total_tokens").Int()
	if total == 0 {
		total = input + output
	}
	params["input_tokens"], params["output_tokens"], params["total_tokens"] = input, output, total
	params["cached_input_tokens"] = usage.Get("input_tokens_details.cached_tokens").Int()
	params["reasoning_output_tokens"] = usage.Get("output_tokens_details.reasoning_tokens").Int()
}

// codexHookEvent simulates one Stop or Interrupt hook run.
func codexHookEvent(profile codexTelemetryProfile, status string) codexAnalyticsEvent {
	hookName := "Stop"
	if status != "completed" {
		hookName = "Interrupt"
	}
	params := map[string]any{
		"execution_mode": "sync", "handler_type": "mcp_tool", "hook_name": hookName,
		"hook_source": "plugin", "model_slug": profile.model, "product_client_id": codexClientName(profile),
		"status": "completed", "thread_id": profile.threadID, "turn_id": profile.turnID,
	}
	return newCodexAnalyticsEvent(profile, "codex_hook_run", params)
}

// codexDynamicToolEvent simulates one dynamic tool-call event.
func codexDynamicToolEvent(profile codexTelemetryProfile, terminal []byte) codexAnalyticsEvent {
	itemID, duration := newCodexTelemetryUUID(), int64(200+simulatedInt(profile.turnID+":dynamic", 1800))
	status := "completed"
	if profile.command && simulatedInt(profile.turnID+":command-status", 10) == 0 {
		status = "failed"
	}
	params := codexToolEventBase(profile, codexToolSpec{terminal, itemID, duration, status})
	params["dynamic_tool_name"], params["tool_name"] = "exec", "exec"
	params["success"] = status == "completed"
	for _, key := range []string{"output_audio_item_count", "output_content_item_count", "output_image_item_count", "output_text_item_count"} {
		params[key] = nil
	}
	return newCodexAnalyticsEvent(profile, "codex_dynamic_tool_call_event", params)
}

// codexCommandEvent simulates one unified-exec command event.
func codexCommandEvent(profile codexTelemetryProfile, terminal []byte) codexAnalyticsEvent {
	duration := int64(100 + simulatedInt(profile.turnID+":command-duration", 1200))
	failed := simulatedInt(profile.turnID+":command-status", 10) == 0
	status, exitCode, failure := "completed", 0, any(nil)
	if failed {
		status, exitCode, failure = "failed", 1, "tool_error"
	}
	params := codexToolEventBase(profile, codexToolSpec{terminal, newCodexTelemetryUUID(), duration, status})
	params["cell_id"], params["command_execution_source"] = "1", "unifiedExecStartup"
	params["exit_code"], params["failure_kind"], params["tool_name"] = exitCode, failure, "unified_exec"
	params["plugin_id"] = nil
	params["execution_duration_ms"], params["script_path"] = duration, nil
	setCodexCommandCounts(params, simulatedInt(profile.turnID+":command-kind", 4))
	return newCodexAnalyticsEvent(profile, "codex_command_execution_event", params)
}

// setCodexCommandCounts sets simulated command action-type counts.
func setCodexCommandCounts(params map[string]any, kind int) {
	params["command_total_action_count"] = 1
	keys := []string{"command_read_action_count", "command_list_files_action_count", "command_search_action_count", "command_unknown_action_count"}
	for index, key := range keys {
		params[key] = boolInt(index == kind)
	}
}

// codexFileChangeEvent simulates one file-change event.
func codexFileChangeEvent(profile codexTelemetryProfile, terminal []byte) codexAnalyticsEvent {
	total := 1 + simulatedInt(profile.turnID+":file-total", 3)
	kind := simulatedInt(profile.turnID+":file-kind", 4)
	params := codexToolEventBase(profile, codexToolSpec{terminal, newCodexTelemetryUUID(), int64(500 + simulatedInt(profile.turnID+":file-duration", 4000)), "completed"})
	keys := []string{"file_add_count", "file_update_count", "file_delete_count", "file_move_count"}
	for index, key := range keys {
		params[key] = total * boolInt(index == kind)
	}
	params["file_change_count"], params["tool_name"] = total, "apply_patch"
	return newCodexAnalyticsEvent(profile, "codex_file_change_event", params)
}

// codexAcceptedLinesEvent simulates an accepted-line-fingerprint event.
func codexAcceptedLinesEvent(profile codexTelemetryProfile) codexAnalyticsEvent {
	params := map[string]any{
		"accepted_added_lines":   1 + simulatedInt(profile.turnID+":added", 120),
		"accepted_deleted_lines": simulatedInt(profile.turnID+":deleted", 24),
		"completed_at":           time.Now().Unix(), "event_type": "codex.accepted_line_fingerprints",
		"line_fingerprints": []any{}, "model_slug": profile.model, "product_surface": "codex",
		"repo_hash": nil, "thread_id": profile.threadID, "turn_id": profile.turnID,
	}
	return newCodexAnalyticsEvent(profile, "codex_accepted_line_fingerprints", params)
}

// codexToolEventBase fills timing and identity fields shared by tool events.
func codexToolEventBase(profile codexTelemetryProfile, spec codexToolSpec) map[string]any {
	completed := time.Now()
	params := map[string]any{
		"app_server_client": codexAppServerClient(profile), "cell_id": spec.itemID, "completed_at_ms": completed.UnixMilli(),
		"duration_ms": spec.duration, "execution_duration_ms": spec.duration, "failure_kind": nil,
		"final_approval_outcome": "unknown", "guardian_review_count": 0, "item_id": spec.itemID,
		"originating_response_id": codexResponseID(spec.terminal), "parent_call_id": nil, "parent_thread_id": nil,
		"requested_additional_permissions": false, "requested_network_access": false,
		"review_count": 0, "root_turn_id": profile.rootTurnID, "runtime": codexRuntime(profile),
		"session_id": profile.sessionID, "started_at_ms": completed.Add(-time.Duration(spec.duration) * time.Millisecond).UnixMilli(),
		"subagent_source": nil, "subsequent_response_id": nil, "terminal_status": spec.status,
		"thread_id": profile.threadID, "thread_source": "user", "turn_id": profile.turnID, "user_review_count": 0,
	}
	return params
}

// codexTerminalResponse extracts the response object from a terminal event or non-stream body.
func codexTerminalResponse(terminal []byte) gjson.Result {
	root := gjson.ParseBytes(terminal)
	if response := root.Get("response"); response.Exists() {
		return response
	}
	return root
}

// codexResponseID returns the real response id, or a matching placeholder when missing.
func codexResponseID(terminal []byte) string {
	if value := codexTerminalResponse(terminal).Get("id").String(); value != "" {
		return value
	}
	return "resp_" + strings.ReplaceAll(newCodexTelemetryUUID(), "-", "")
}

// simulatedInt derives a stable bounded mock value from a turn seed.
func simulatedInt(seed string, limit int) int {
	if limit <= 1 {
		return 0
	}
	sum := sha256.Sum256([]byte(seed))
	return int(binary.BigEndian.Uint64(sum[:8]) % uint64(limit))
}

// boolInt converts a bool into an event count field.
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// elapsedMillis returns a non-negative millisecond span, using fallback when end is zero.
func elapsedMillis(start, end, fallback time.Time) int64 {
	if end.IsZero() {
		end = fallback
	}
	return codexNonNegativeMillis(end.Sub(start))
}

func codexNonNegativeMillis(d time.Duration) int64 {
	ms := d.Milliseconds()
	if ms < 0 {
		return 0
	}
	return ms
}
