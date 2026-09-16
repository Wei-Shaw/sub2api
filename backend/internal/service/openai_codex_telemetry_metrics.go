package service

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
)

type codexMetricDescriptor struct {
	name       string
	kind       string
	unit       string
	attributes string
}

// codexMetricPoint is the aggregated state of one (metric, attribute set) in an export period.
//
// The real Codex client uses the OpenTelemetry SDK PeriodicReader with delta temporality:
// the SDK keeps count/sum/min/max/bucketCounts per (instrument, attribute set) instead of
// collapsing many observations into a scalar. This copy of that model keeps different
// model/session observations as separate points so histogram shape is not lost.
type codexMetricPoint struct {
	descriptor codexMetricDescriptor
	attributes map[string]string
	// sum (counter): accumulated value in this period.
	sumValue float64
	// histogram: per-observation aggregates in this period.
	count        uint64
	sum          float64
	min          float64
	max          float64
	bucketCounts []uint64
	// gauge: last observation in this period.
	lastValue float64
	observed  bool
}

type codexMetricState struct {
	profile           codexTelemetryProfile
	started           time.Time
	lastSeen          time.Time
	points            map[string]*codexMetricPoint
	externalAgentSent bool
}

var codexMetricDescriptors = []codexMetricDescriptor{
	{"codex.process.start", "sum", "", "originator"},
	{"codex.sqlite.init.count", "sum", "", "db,error,originator,phase,status"},
	{"codex.sqlite.init.duration_ms", "histogram", "ms", "db,error,originator,phase,status"},
	{"codex.app_server.codex_home.size_bytes", "histogram", "", "compression_enabled,directory"},
	{"codex.remote_models.fetch_update.duration_ms", "histogram", "ms", ""},
	{"codex.plugins.loaded_cache.event", "sum", "", "event"},
	{"codex.remote_models.load_cache.duration_ms", "histogram", "ms", ""},
	{"codex.plugins.loaded_cache.wait.duration_ms", "histogram", "ms", ""},
	{"codex.plugins.loaded_cache.load.duration_ms", "histogram", "ms", ""},
	{"codex.plugins.loaded_cache.request", "sum", "", "outcome"},
	{"codex.mcp.protocol_discovery", "sum", "", "mode,outcome,server_kind"},
	{"codex.mcp.protocol_discovery.duration_ms", "histogram", "ms", "mode,outcome,server_kind"},
	{"codex.mcp.tools.fetch_uncached.duration_ms", "histogram", "ms", "trigger"},
	{"codex.mcp.tools.list.duration_ms", "histogram", "ms", "cache"},
	{"codex.apps.installed.duration_ms", "histogram", "ms", "force_refresh,outcome,path,refresh,reload,retained_previous_snapshot"},
	{"codex.apps.installed.response_bytes", "histogram", "", "path"},
	{"codex.apps.installed.connector_count", "histogram", "", "path"},
	{"codex.apps.installed.tool_count", "histogram", "", "path"},
	{"codex.apps.snapshot.age_ms", "histogram", "ms", "observation,path"},
	{"codex.apps.read.duration_ms", "histogram", "ms", "include_tools"},
	{"codex.sqlite.logs.write.count", "sum", "", "error,originator,status"},
	{"codex.sqlite.logs.write.duration_ms", "histogram", "ms", "error,originator,status"},
	{"codex.sqlite.logs.write.bytes", "histogram", "", "error,originator,status"},
	{"codex.sqlite.logs.write.entries", "histogram", "", "error,originator,status"},
	{"codex.sqlite.logs.write.max_entry_bytes", "histogram", "", "error,originator,status"},
	{"codex.mcp.tools.cache_write.duration_ms", "histogram", "ms", "status"},
	{"codex.mcp.tools.cache_publish.duration_ms", "histogram", "ms", "result,source"},
	{"codex.apps.refresh.duration_ms", "histogram", "ms", "path,trigger"},
	{"codex.feature.state", "sum", "", "app.version,auth_mode,feature,model,originator,service_name,session_source,value"},
	{"codex.thread.started", "sum", "", "app.version,auth_mode,is_git,model,originator,service_name,session_source"},
	{"codex.shell_snapshot.duration_ms", "histogram", "ms", "app.version,auth_mode,model,originator,session_source,success,version"},
	{"codex.shell_snapshot", "sum", "", "app.version,auth_mode,failure_reason,model,originator,session_source,success,version"},
	{"codex.startup.phase.duration_ms", "histogram", "ms", "app.version,auth_mode,model,originator,phase,service_name,session_source,status"},
	{"codex.websocket.request", "sum", "", "app.version,auth_mode,model,originator,service_name,session_source,success"},
	{"codex.websocket.request.duration_ms", "histogram", "ms", "app.version,auth_mode,model,originator,service_name,session_source,success"},
	{"codex.rollout_compression.materialize", "sum", "", "outcome"},
	{"codex.websocket.event", "sum", "", "app.version,auth_mode,kind,model,originator,service_name,session_source,success"},
	{"codex.websocket.event.duration_ms", "histogram", "ms", "app.version,auth_mode,kind,model,originator,service_name,session_source,success"},
	{"codex.startup_prewarm.duration_ms", "histogram", "ms", "app.version,auth_mode,model,originator,service_name,session_source,status"},
	{"codex.startup_prewarm.age_at_first_turn_ms", "histogram", "ms", "app.version,auth_mode,model,originator,service_name,session_source,status"},
	{"codex.thread.skills.enabled_total", "histogram", "", "app.version,auth_mode,catalog_surface,model,originator,service_name,session_source"},
	{"codex.thread.skills.kept_total", "histogram", "", "app.version,auth_mode,catalog_surface,model,originator,service_name,session_source"},
	{"codex.thread.skills.truncated", "histogram", "", "app.version,auth_mode,catalog_surface,model,originator,service_name,session_source"},
	{"codex.thread.skills.description_truncated_chars", "histogram", "", "app.version,auth_mode,catalog_surface,model,originator,service_name,session_source"},
	{"codex.skills.shadow_selection", "sum", "", "candidate_set_truncated,method,query_script,query_truncated,status"},
	{"codex.skills.shadow_selection.duration_ms", "histogram", "ms", "candidate_set_truncated,method,query_script,query_truncated,status"},
	{"codex.skills.shadow_selection.catalog_entries", "histogram", "", "candidate_set_truncated,method,query_script,query_truncated,status"},
	{"codex.skills.shadow_selection.selected_entries", "histogram", "", "candidate_set_truncated,method,query_script,query_truncated,status"},
	{"codex.skills.shadow_selection.query_terms", "histogram", "", "candidate_set_truncated,method,query_script,query_truncated,status"},
	{"codex.skills.shadow_selection.reduction_bps", "histogram", "", "candidate_set_truncated,method,query_script,query_truncated,status"},
	{"codex.turn.ttft.duration_ms", "histogram", "ms", "app.version,auth_mode,model,originator,service_name,session_source"},
	{"codex.turn.ttfm.duration_ms", "histogram", "ms", "app.version,auth_mode,model,originator,service_name,session_source"},
	{"codex.responses_api_overhead.duration_ms", "histogram", "ms", "app.version,auth_mode,model,originator,service_name,session_source"},
	{"codex.responses_api_inference_time.duration_ms", "histogram", "ms", "app.version,auth_mode,model,originator,service_name,session_source"},
	{"codex.responses_api_engine_iapi_tbt.duration_ms", "histogram", "ms", "app.version,auth_mode,model,originator,service_name,session_source"},
	{"codex.turn.e2e_duration_ms", "histogram", "ms", "app.version,auth_mode,model,originator,service_name,session_source"},
	{"codex.turn.network_proxy", "sum", "", "active,app.version,auth_mode,model,originator,service_name,session_source,tmp_mem_enabled"},
	{"codex.turn.tool.call", "histogram", "", "app.version,auth_mode,model,originator,service_name,session_source,tmp_mem_enabled"},
	{"codex.turn.memory", "sum", "", "app.version,auth_mode,config_use_memories,feature_enabled,has_citations,model,originator,read_allowed,service_name,session_source"},
	{"codex.turn.unified_exec.running_processes", "sum", "", "app.version,auth_mode,model,originator,service_name,session_source"},
	{"codex.windows_mxc.available", "sum", "", "available"},
	{"codex.tool.unified_exec", "sum", "", "app.version,auth_mode,model,originator,session_source,tty"},
	{"codex.hooks.run", "sum", "", "app.version,auth_mode,execution_mode,handler_type,hook_name,model,originator,session_source,source,status"},
	{"codex.hooks.run.duration_ms", "histogram", "ms", "app.version,auth_mode,execution_mode,handler_type,hook_name,model,originator,session_source,source,status"},
	{"codex.external_agent_config.detect", "sum", "", "migration_type"},
	{"codex.rollout.size_bytes", "histogram", "", ""},
}

// codexMetricDescriptorIndex indexes descriptors by name for the record path.
var codexMetricDescriptorIndex = func() map[string]codexMetricDescriptor {
	index := make(map[string]codexMetricDescriptor, len(codexMetricDescriptors))
	for _, descriptor := range codexMetricDescriptors {
		index[descriptor.name] = descriptor
	}
	return index
}()

// codexDynamicMetricNames are turn-incremental metrics excluded from the startup batch.
// An explicit set replaces the `codexMetricDescriptors[:62]` positional contract so
// catalog inserts/reorders cannot silently change the startup/incremental split.
var codexDynamicMetricNames = map[string]struct{}{
	"codex.hooks.run":                    {},
	"codex.hooks.run.duration_ms":        {},
	"codex.external_agent_config.detect": {},
	"codex.rollout.size_bytes":           {},
}

// codexStartupMetric reports whether a metric belongs to the first-seen-account startup batch.
func codexStartupMetric(name string) bool {
	_, dynamic := codexDynamicMetricNames[name]
	return !dynamic
}

// codexStatsigDisabledMetrics are metrics the real Codex client never routes through
// Statsig (STATSIG_DISABLED_METRICS in codex-rs/otel/src/metrics/config.rs). The
// current catalog does not include these names; keep the guard so later additions
// are not sent by mistake.
var codexStatsigDisabledMetrics = map[string]struct{}{
	"codex.api_request":                                   {},
	"codex.api_request.duration_ms":                       {},
	"codex.conversation.turn.count":                       {},
	"exec_server_client_requests_total":                   {},
	"codex.responses_api_engine_iapi_ttft.duration_ms":    {},
	"codex.responses_api_engine_service_tbt.duration_ms":  {},
	"codex.responses_api_engine_service_ttft.duration_ms": {},
	"codex.tool.call":                                     {},
	"codex.tool.call.duration_ms":                         {},
	"codex.turn.cost_microusd":                            {},
	"codex.turn.token_usage":                              {},
}

// codexTelemetryStatsigAllowed reports whether a metric may be sent to Statsig.
func codexTelemetryStatsigAllowed(name string) bool {
	_, disabled := codexStatsigDisabledMetrics[name]
	return !disabled
}

// newCodexMetricPoint initializes an aggregate point from one observation.
func newCodexMetricPoint(descriptor codexMetricDescriptor, attributes map[string]string, value float64) *codexMetricPoint {
	bounds := codexBoundsFor(descriptor)
	point := &codexMetricPoint{
		descriptor: descriptor, attributes: attributes, min: value, max: value,
		bucketCounts: make([]uint64, len(bounds)+1), observed: true,
	}
	switch descriptor.kind {
	case "sum":
		point.sumValue = value
	case "histogram":
		point.count, point.sum = 1, value
		point.bucketCounts[codexBucketIndex(bounds, value)] = 1
	default:
		point.lastValue = value
	}
	return point
}

// recordCodexMetricPoint merges one observation into an aggregate point. Caller holds the manager lock.
func recordCodexMetricPoint(state *codexMetricState, profile codexTelemetryProfile, name string, value float64) {
	descriptor, ok := codexMetricDescriptorIndex[name]
	if !ok {
		return
	}
	attributes := codexMetricAttributeMap(profile, descriptor)
	key := name + "\x00" + codexMetricAttributeSignature(attributes)
	point := state.points[key]
	if point == nil {
		state.points[key] = newCodexMetricPoint(descriptor, attributes, value)
		return
	}
	switch descriptor.kind {
	case "sum":
		point.sumValue += value
	case "histogram":
		if value < point.min {
			point.min = value
		}
		if value > point.max {
			point.max = value
		}
		point.count++
		point.sum += value
		bounds := codexBoundsFor(descriptor)
		point.bucketCounts[codexBucketIndex(bounds, value)]++
	default:
		point.lastValue, point.observed = value, true
	}
}

// touchMetrics initializes per-account metric state and sends the startup batch.
func (m *codexTelemetryManager) touchMetrics(profile codexTelemetryProfile) {
	m.mu.Lock()
	state, created := m.ensureMetricStateLocked(profile, profile.started)
	started := state.started
	m.mu.Unlock()
	if created {
		m.enqueueStartupMetrics(profile, started)
	}
}

// ensureMetricStateLocked gets or creates per-account metric state while holding the lock.
// On create, lastSeen is the current observation time rather than profile.started: if a
// long turn is registered with its start time after TTL, the next flush would drop it.
func (m *codexTelemetryManager) ensureMetricStateLocked(profile codexTelemetryProfile, now time.Time) (*codexMetricState, bool) {
	accountID := profile.client.localID
	if state := m.metrics[accountID]; state != nil {
		state.profile, state.lastSeen = profile, now
		return state, false
	}
	state := &codexMetricState{profile: profile, started: now, lastSeen: now, points: make(map[string]*codexMetricPoint)}
	m.metrics[accountID] = state
	return state, true
}

// enqueueStartupMetrics sends the startup metric batch when an account is first seen.
func (m *codexTelemetryManager) enqueueStartupMetrics(profile codexTelemetryProfile, started time.Time) {
	points := make([]*codexMetricPoint, 0, 62)
	for _, descriptor := range codexMetricDescriptors {
		if !codexStartupMetric(descriptor.name) {
			continue
		}
		attributes := codexMetricAttributeMap(profile, descriptor)
		points = append(points, newCodexMetricPoint(descriptor, attributes, codexStartupMetricValue(profile, descriptor)))
	}
	m.enqueueMetrics(profile.client, buildCodexMetricsPayload(profile, started, points))
}

// recordTurnMetrics accumulates incremental metrics produced by one turn.
//
// Lookup and create happen under the same lock. Unlocking to call touchMetrics and then
// locking again could let the minute-level flush TTL-delete the just-created state and
// cause a nil dereference here.
func (m *codexTelemetryManager) recordTurnMetrics(profile codexTelemetryProfile, result codexTelemetryTerminal) {
	now := time.Now()
	m.mu.Lock()
	state, created := m.ensureMetricStateLocked(profile, now)
	started := state.started
	recordCodexMetricPoint(state, profile, "codex.turn.e2e_duration_ms", float64(codexNonNegativeMillis(now.Sub(profile.started))))
	recordCodexMetricPoint(state, profile, "codex.turn.ttft.duration_ms", float64(elapsedMillis(profile.started, result.firstEvent, now)))
	recordCodexMetricPoint(state, profile, "codex.turn.ttfm.duration_ms", float64(elapsedMillis(profile.started, result.firstToken, now)))
	// Four hook runs per turn: the counter adds 4, durations are four observations.
	recordCodexMetricPoint(state, profile, "codex.hooks.run", 4)
	for index := range 4 {
		duration := float64(50 + simulatedInt(profile.turnID+":hook:"+strconv.Itoa(index), 1500))
		recordCodexMetricPoint(state, profile, "codex.hooks.run.duration_ms", duration)
	}
	recordCodexMetricPoint(state, profile, "codex.turn.tool.call", float64(boolInt(profile.dynamicTool)+boolInt(profile.fileChange)))
	if profile.command {
		recordCodexMetricPoint(state, profile, "codex.tool.unified_exec", 1)
	}
	if profile.fileChange {
		recordCodexMetricPoint(state, profile, "codex.rollout.size_bytes", float64(1024+simulatedInt(profile.turnID+":rollout", 196608)))
	}
	if !state.externalAgentSent {
		recordCodexMetricPoint(state, profile, "codex.external_agent_config.detect", 1)
		state.externalAgentSent = true
	}
	m.mu.Unlock()
	if created {
		m.enqueueStartupMetrics(profile, started)
	}
}

// flushMetrics sends pending metrics and drops expired account/thread state.
func (m *codexTelemetryManager) flushMetrics(now time.Time) {
	type batch struct {
		client  codexTelemetryClient
		profile codexTelemetryProfile
		started time.Time
		points  []*codexMetricPoint
	}
	m.mu.Lock()
	batches := make([]batch, 0, len(m.metrics))
	for accountID, state := range m.metrics {
		if len(state.points) > 0 {
			points := make([]*codexMetricPoint, 0, len(state.points))
			for _, point := range state.points {
				points = append(points, point)
			}
			batches = append(batches, batch{state.profile.client, state.profile, state.started, points})
			state.points = make(map[string]*codexMetricPoint)
		}
		if now.Sub(state.lastSeen) > codexTelemetryStateTTL {
			delete(m.metrics, accountID)
		}
	}
	for key, seen := range m.threads {
		if now.Sub(seen) > codexTelemetryStateTTL {
			delete(m.threads, key)
		}
	}
	m.mu.Unlock()
	for _, item := range batches {
		m.enqueueMetrics(item.client, buildCodexMetricsPayload(item.profile, item.started, item.points))
	}
}

// enqueueMetrics places a non-empty OTLP payload on the async send queue.
func (m *codexTelemetryManager) enqueueMetrics(client codexTelemetryClient, body []byte) {
	if len(body) > 0 {
		m.enqueue(codexTelemetryJob{client: client, url: codexMetricsEndpoint, body: body, metrics: true})
	}
}

// buildCodexMetricsPayload encodes aggregate points as an OTLP JSON body.
func buildCodexMetricsPayload(profile codexTelemetryProfile, started time.Time, points []*codexMetricPoint) []byte {
	metrics := make([]any, 0, len(points))
	for _, point := range points {
		if !codexTelemetryStatsigAllowed(point.descriptor.name) {
			continue
		}
		metrics = append(metrics, codexOTLPMetricPoint(started, point))
	}
	resource := map[string]any{"attributes": codexResourceAttributes(profile), "droppedAttributesCount": 0, "entityRefs": []any{}}
	scope := map[string]any{"name": "codex", "version": "", "attributes": []any{}, "droppedAttributesCount": 0}
	scopeMetrics := map[string]any{"scope": scope, "metrics": metrics, "schemaUrl": ""}
	payload := map[string]any{"resourceMetrics": []any{map[string]any{"resource": resource, "scopeMetrics": []any{scopeMetrics}, "schemaUrl": ""}}}
	body, _ := json.Marshal(payload)
	return body
}

// codexOTLPMetricPoint converts an aggregate point into an OTLP sum, histogram, or gauge.
func codexOTLPMetricPoint(started time.Time, point *codexMetricPoint) map[string]any {
	nowNanos := strconv.FormatInt(time.Now().UnixNano(), 10)
	startNanos := strconv.FormatInt(started.UnixNano(), 10)
	dataPoint := map[string]any{
		"attributes": codexOTLPAttributes(point.attributes), "startTimeUnixNano": startNanos,
		"timeUnixNano": nowNanos, "exemplars": []any{}, "flags": 0,
	}
	metric := map[string]any{"name": point.descriptor.name, "description": "", "unit": point.descriptor.unit, "metadata": []any{}}
	switch point.descriptor.kind {
	case "sum":
		dataPoint["asInt"] = int64(point.sumValue)
		metric["sum"] = map[string]any{"dataPoints": []any{dataPoint}, "aggregationTemporality": 1, "isMonotonic": true}
	case "gauge":
		dataPoint["asInt"] = int64(point.lastValue)
		metric["gauge"] = map[string]any{"dataPoints": []any{dataPoint}}
	default:
		if point.descriptor.unit == "ms" {
			metric["description"] = "Duration in milliseconds."
		}
		dataPoint["count"], dataPoint["sum"], dataPoint["min"], dataPoint["max"] = point.count, point.sum, point.min, point.max
		dataPoint["explicitBounds"] = codexBoundsFor(point.descriptor)
		dataPoint["bucketCounts"] = point.bucketCounts
		metric["histogram"] = map[string]any{"dataPoints": []any{dataPoint}, "aggregationTemporality": 1}
	}
	return metric
}

// codexHistogramBounds are the millisecond histogram bounds, matching the client
// MILLISECOND_DURATION_BOUNDARIES.
var codexHistogramBounds = []float64{0, 5, 10, 25, 50, 75, 100, 250, 500, 750, 1000, 1250, 1500, 1750, 2000, 2250, 2500, 3000, 3500, 4000, 4500, 5000, 6000, 7000, 7500, 8000, 9000, 10000, 12000, 15000, 20000, 30000, 60000, 120000}

// codexSecondHistogramBounds matches the client SECOND_DURATION_BOUNDARIES.
var codexSecondHistogramBounds = []float64{0, 0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1.0, 2.5, 5.0, 7.5, 10.0, 12.0, 15.0, 20.0, 30.0, 60.0, 120.0}

// codexOtelDefaultHistogramBounds are the OpenTelemetry SDK default buckets for
// histograms that do not specify bounds.
var codexOtelDefaultHistogramBounds = []float64{0, 5, 10, 25, 50, 75, 100, 250, 500, 750, 1000, 2500, 5000, 7500, 10000}

// codexBoundsFor returns explicit bounds for a metric unit. The real client only
// configures duration metrics; other histograms use the SDK default buckets.
func codexBoundsFor(descriptor codexMetricDescriptor) []float64 {
	switch descriptor.unit {
	case "ms":
		return codexHistogramBounds
	case "s":
		return codexSecondHistogramBounds
	default:
		return codexOtelDefaultHistogramBounds
	}
}

// codexBucketIndex returns the bucket index for a value (last bucket is +Inf).
func codexBucketIndex(bounds []float64, value float64) int {
	for index, bound := range bounds {
		if value <= bound {
			return index
		}
	}
	return len(bounds)
}

// codexMetricAttributeMap returns the metric attribute set.
func codexMetricAttributeMap(profile codexTelemetryProfile, descriptor codexMetricDescriptor) map[string]string {
	values := make(map[string]string)
	if descriptor.attributes == "" {
		return values
	}
	for _, name := range strings.Split(descriptor.attributes, ",") {
		if value := codexMetricAttributeValue(profile, descriptor.name, name); value != "" {
			values[name] = value
		}
	}
	return values
}

// codexMetricAttributeSignature builds a stable attribute-set signature for merging observations.
func codexMetricAttributeSignature(attributes map[string]string) string {
	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(attributes[key])
		builder.WriteByte('\x1f')
	}
	return builder.String()
}

// codexResourceAttributes builds OTLP resource-level client attributes.
func codexResourceAttributes(profile codexTelemetryProfile) []any {
	_, _, osName, osVersion, _ := codexUserAgentParts(profile.client.userAgent, profile.client.version)
	return codexOTLPAttributes(map[string]string{
		"os": osName, "os_version": osVersion, "service.version": profile.client.version, "env": "dev",
		"telemetry.sdk.version": "0.31.0", "telemetry.sdk.language": "rust",
		"service.name": codexMetricResourceService(profile), "telemetry.sdk.name": "opentelemetry",
	})
}

// codexOTLPAttributes encodes OTLP string attributes sorted by key.
func codexOTLPAttributes(values map[string]string) []any {
	attributes := make([]any, 0, len(values))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := values[key]
		attributes = append(attributes, map[string]any{"key": key, "value": map[string]any{"stringValue": value}})
	}
	return attributes
}

// codexMetricAttributeValue returns the simulated value for a metric attribute.
func codexMetricAttributeValue(profile codexTelemetryProfile, metric, name string) string {
	if name == "originator" && (metric == "codex.process.start" || strings.HasPrefix(metric, "codex.sqlite.")) {
		return codexMetricResourceService(profile)
	}
	values := map[string]string{
		"app.version": profile.client.version, "auth_mode": "Chatgpt", "model": profile.model,
		"originator": codexMetricOriginator(profile), "service_name": codexMetricProductService(profile),
		"session_source": codexMetricSessionSource(profile), "status": "success", "success": "true",
		"error": "none", "outcome": "success", "active": "false", "available": "true",
		"is_git": "false", "tty": "false", "cache": "miss", "compression_enabled": "false",
		"execution_mode": "sync", "handler_type": "mcp_tool", "hook_name": "Stop", "source": "plugin",
		"migration_type": "config", "tmp_mem_enabled": "false", "value": "true",
		"candidate_set_truncated": "false", "catalog_surface": "thread_context", "config_use_memories": "true",
		"db": "state", "directory": "codex_home", "event": "clear", "feature": "hooks",
		"feature_enabled": "false", "failure_reason": "write_failed", "force_refresh": "false",
		"has_citations": "false", "include_tools": "false", "kind": "response.completed",
		"method": "weighted_lexical_v1", "mode": "legacy", "observation": "installed",
		"path": "new", "phase": "open_state", "query_script": "mixed", "query_truncated": "false",
		"read_allowed": "false", "refresh": "not_requested", "reload": "false", "result": "published",
		"retained_previous_snapshot": "false", "server_kind": "openai_codex_apps", "trigger": "initial", "version": "v1",
	}
	if value := values[name]; value != "" {
		return value
	}
	return "default"
}

// codexMetricResourceService returns the OTLP resource service for the client.
func codexMetricResourceService(profile codexTelemetryProfile) string {
	if strings.EqualFold(codexClientName(profile), "Codex Desktop") {
		return "codex-app-server"
	}
	return firstNonEmptyString(profile.client.originator, "codex_cli_rs")
}

// codexMetricOriginator sanitizes originator using Codex rules.
func codexMetricOriginator(profile codexTelemetryProfile) string {
	value := strings.Map(func(char rune) rune {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("._-/", char) {
			return char
		}
		return '_'
	}, profile.client.originator)
	value = strings.Trim(value, "_")
	if value == "" {
		return "unspecified"
	}
	if len(value) > 256 {
		return value[:256]
	}
	return value
}

// codexMetricProductService returns the product service name for Desktop or editor clients.
func codexMetricProductService(profile codexTelemetryProfile) string {
	name := strings.ToLower(codexClientName(profile))
	if name == "codex desktop" {
		return "codex_desktop"
	}
	if name == "codex_vscode" {
		return "codex_vscode"
	}
	return ""
}

// codexMetricSessionSource returns the session source used in metrics.
func codexMetricSessionSource(profile codexTelemetryProfile) string {
	if service := codexMetricProductService(profile); service != "" {
		return "vscode"
	}
	return "cli"
}

// codexStartupMetricValue generates a type-appropriate simulated startup observation.
func codexStartupMetricValue(profile codexTelemetryProfile, descriptor codexMetricDescriptor) float64 {
	if descriptor.name == "codex.turn.unified_exec.running_processes" || descriptor.name == "codex.turn.tool.call" {
		return 0
	}
	if descriptor.name == "codex.windows_mxc.available" && !strings.EqualFold(codexRuntime(profile)["runtime_os"].(string), "windows") {
		return 0
	}
	if descriptor.kind == "sum" {
		return 1
	}
	limit := 64
	if descriptor.unit == "ms" {
		limit = 4000
	} else if strings.Contains(descriptor.name, "bytes") {
		limit = 262144
	}
	return float64(simulatedInt(profile.turnID+":"+descriptor.name, limit))
}
