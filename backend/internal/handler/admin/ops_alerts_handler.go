package admin

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

var validOpsAlertMetricTypes = []string{
	"success_rate",
	"error_rate",
	"upstream_error_rate",
	"cpu_usage_percent",
	"memory_usage_percent",
	"concurrency_queue_depth",
	"group_available_accounts",
	"group_available_ratio",
	"group_rate_limit_ratio",
	"account_rate_limited_count",
	"account_error_count",
	"account_error_ratio",
	"account_temp_unscheduled_count",
	"overload_account_count",
	"proxy_expired_count",
	"proxy_expiring_soon_count",
REDACTED

var validOpsAlertMetricTypeSet = func() map[string]struct{REDACTED {
	set := make(map[string]struct{REDACTED, len(validOpsAlertMetricTypes))
	for _, v := range validOpsAlertMetricTypes {
		set[v] = struct{REDACTED{REDACTED
REDACTED
	return set
REDACTED()

var validOpsAlertOperators = []string{">", "<", ">=", "<=", "==", "!="REDACTED

var validOpsAlertOperatorSet = func() map[string]struct{REDACTED {
	set := make(map[string]struct{REDACTED, len(validOpsAlertOperators))
	for _, v := range validOpsAlertOperators {
		set[v] = struct{REDACTED{REDACTED
REDACTED
	return set
REDACTED()

var validOpsAlertSeverities = []string{"P0", "P1", "P2", "P3"REDACTED

var validOpsAlertSeveritySet = func() map[string]struct{REDACTED {
	set := make(map[string]struct{REDACTED, len(validOpsAlertSeverities))
	for _, v := range validOpsAlertSeverities {
		set[v] = struct{REDACTED{REDACTED
REDACTED
	return set
REDACTED()

type opsAlertRuleValidatedInput struct {
	Name       string
	MetricType string
	Operator   string
	Threshold  float64

	Severity string

	WindowMinutes    int
	SustainedMinutes int
	CooldownMinutes  int

	Enabled     bool
	NotifyEmail bool

	WindowProvided    bool
	SustainedProvided bool
	CooldownProvided  bool
	SeverityProvided  bool
	EnabledProvided   bool
	NotifyProvided    bool
REDACTED

func isPercentOrRateMetric(metricType string) bool {
	switch metricType {
	case "success_rate",
		"error_rate",
		"upstream_error_rate",
		"cpu_usage_percent",
		"memory_usage_percent",
		"group_available_ratio",
		"group_rate_limit_ratio",
		"account_error_ratio":
		return true
	default:
		return false
REDACTED
REDACTED

func validateOpsAlertRulePayload(raw map[string]json.RawMessage) (*opsAlertRuleValidatedInput, error) {
	if raw == nil {
		return nil, fmt.Errorf("invalid request body")
REDACTED

	requiredFields := []string{"name", "metric_type", "operator", "threshold"REDACTED
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			return nil, fmt.Errorf("%s is required", field)
	REDACTED
REDACTED

	var name string
	if err := json.Unmarshal(raw["name"], &name); err != nil || strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("name is required")
REDACTED
	name = strings.TrimSpace(name)

	var metricType string
	if err := json.Unmarshal(raw["metric_type"], &metricType); err != nil || strings.TrimSpace(metricType) == "" {
		return nil, fmt.Errorf("metric_type is required")
REDACTED
	metricType = strings.TrimSpace(metricType)
	if _, ok := validOpsAlertMetricTypeSet[metricType]; !ok {
		return nil, fmt.Errorf("metric_type must be one of: %s", strings.Join(validOpsAlertMetricTypes, ", "))
REDACTED

	var operator string
	if err := json.Unmarshal(raw["operator"], &operator); err != nil || strings.TrimSpace(operator) == "" {
		return nil, fmt.Errorf("operator is required")
REDACTED
	operator = strings.TrimSpace(operator)
	if _, ok := validOpsAlertOperatorSet[operator]; !ok {
		return nil, fmt.Errorf("operator must be one of: %s", strings.Join(validOpsAlertOperators, ", "))
REDACTED

	var threshold float64
	if err := json.Unmarshal(raw["threshold"], &threshold); err != nil {
		return nil, fmt.Errorf("threshold must be a number")
REDACTED
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) {
		return nil, fmt.Errorf("threshold must be a finite number")
REDACTED
	if isPercentOrRateMetric(metricType) {
		if threshold < 0 || threshold > 100 {
			return nil, fmt.Errorf("threshold must be between 0 and 100 for metric_type %s", metricType)
	REDACTED
REDACTED else if threshold < 0 {
		return nil, fmt.Errorf("threshold must be >= 0")
REDACTED

	validated := &opsAlertRuleValidatedInput{
		Name:       name,
		MetricType: metricType,
		Operator:   operator,
		Threshold:  threshold,
REDACTED

	if v, ok := raw["severity"]; ok {
		validated.SeverityProvided = true
		var sev string
		if err := json.Unmarshal(v, &sev); err != nil {
			return nil, fmt.Errorf("severity must be a string")
	REDACTED
		sev = strings.ToUpper(strings.TrimSpace(sev))
		if sev != "" {
			if _, ok := validOpsAlertSeveritySet[sev]; !ok {
				return nil, fmt.Errorf("severity must be one of: %s", strings.Join(validOpsAlertSeverities, ", "))
		REDACTED
			validated.Severity = sev
	REDACTED
REDACTED
	if validated.Severity == "" {
		validated.Severity = "P2"
REDACTED

	if v, ok := raw["enabled"]; ok {
		validated.EnabledProvided = true
		if err := json.Unmarshal(v, &validated.Enabled); err != nil {
			return nil, fmt.Errorf("enabled must be a boolean")
	REDACTED
REDACTED else {
		validated.Enabled = true
REDACTED

	if v, ok := raw["notify_email"]; ok {
		validated.NotifyProvided = true
		if err := json.Unmarshal(v, &validated.NotifyEmail); err != nil {
			return nil, fmt.Errorf("notify_email must be a boolean")
	REDACTED
REDACTED else {
		validated.NotifyEmail = true
REDACTED

	if v, ok := raw["window_minutes"]; ok {
		validated.WindowProvided = true
		if err := json.Unmarshal(v, &validated.WindowMinutes); err != nil {
			return nil, fmt.Errorf("window_minutes must be an integer")
	REDACTED
		switch validated.WindowMinutes {
		case 1, 5, 60:
		default:
			return nil, fmt.Errorf("window_minutes must be one of: 1, 5, 60")
	REDACTED
REDACTED else {
		validated.WindowMinutes = 1
REDACTED

	if v, ok := raw["sustained_minutes"]; ok {
		validated.SustainedProvided = true
		if err := json.Unmarshal(v, &validated.SustainedMinutes); err != nil {
			return nil, fmt.Errorf("sustained_minutes must be an integer")
	REDACTED
		if validated.SustainedMinutes < 1 || validated.SustainedMinutes > 1440 {
			return nil, fmt.Errorf("sustained_minutes must be between 1 and 1440")
	REDACTED
REDACTED else {
		validated.SustainedMinutes = 1
REDACTED

	if v, ok := raw["cooldown_minutes"]; ok {
		validated.CooldownProvided = true
		if err := json.Unmarshal(v, &validated.CooldownMinutes); err != nil {
			return nil, fmt.Errorf("cooldown_minutes must be an integer")
	REDACTED
		if validated.CooldownMinutes < 0 || validated.CooldownMinutes > 1440 {
			return nil, fmt.Errorf("cooldown_minutes must be between 0 and 1440")
	REDACTED
REDACTED else {
		validated.CooldownMinutes = 0
REDACTED

	return validated, nil
REDACTED

// ListAlertRules returns all ops alert rules.
// GET /api/v1/admin/ops/alert-rules
func (h *OpsHandler) ListAlertRules(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	rules, err := h.opsService.ListAlertRules(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, rules)
REDACTED

// CreateAlertRule creates an ops alert rule.
// POST /api/v1/admin/ops/alert-rules
func (h *OpsHandler) CreateAlertRule(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	var raw map[string]json.RawMessage
	if err := c.ShouldBindBodyWith(&raw, binding.JSON); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
REDACTED
	validated, err := validateOpsAlertRulePayload(raw)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED

	var rule service.OpsAlertRule
	if err := c.ShouldBindBodyWith(&rule, binding.JSON); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
REDACTED

	rule.Name = validated.Name
	rule.MetricType = validated.MetricType
	rule.Operator = validated.Operator
	rule.Threshold = validated.Threshold
	rule.WindowMinutes = validated.WindowMinutes
	rule.SustainedMinutes = validated.SustainedMinutes
	rule.CooldownMinutes = validated.CooldownMinutes
	rule.Severity = validated.Severity
	rule.Enabled = validated.Enabled
	rule.NotifyEmail = validated.NotifyEmail

	created, err := h.opsService.CreateAlertRule(c.Request.Context(), &rule)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, created)
REDACTED

// UpdateAlertRule updates an existing ops alert rule.
// PUT /api/v1/admin/ops/alert-rules/:id
func (h *OpsHandler) UpdateAlertRule(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid rule ID")
		return
REDACTED

	var raw map[string]json.RawMessage
	if err := c.ShouldBindBodyWith(&raw, binding.JSON); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
REDACTED
	validated, err := validateOpsAlertRulePayload(raw)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED

	var rule service.OpsAlertRule
	if err := c.ShouldBindBodyWith(&rule, binding.JSON); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
REDACTED

	rule.ID = id
	rule.Name = validated.Name
	rule.MetricType = validated.MetricType
	rule.Operator = validated.Operator
	rule.Threshold = validated.Threshold
	rule.WindowMinutes = validated.WindowMinutes
	rule.SustainedMinutes = validated.SustainedMinutes
	rule.CooldownMinutes = validated.CooldownMinutes
	rule.Severity = validated.Severity
	rule.Enabled = validated.Enabled
	rule.NotifyEmail = validated.NotifyEmail

	updated, err := h.opsService.UpdateAlertRule(c.Request.Context(), &rule)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, updated)
REDACTED

// DeleteAlertRule deletes an ops alert rule.
// DELETE /api/v1/admin/ops/alert-rules/:id
func (h *OpsHandler) DeleteAlertRule(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid rule ID")
		return
REDACTED

	if err := h.opsService.DeleteAlertRule(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"deleted": trueREDACTED)
REDACTED

// GetAlertEvent returns a single ops alert event.
// GET /api/v1/admin/ops/alert-events/:id
func (h *OpsHandler) GetAlertEvent(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid event ID")
		return
REDACTED

	ev, err := h.opsService.GetAlertEventByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, ev)
REDACTED

// UpdateAlertEventStatus updates an ops alert event status.
// PUT /api/v1/admin/ops/alert-events/:id/status
func (h *OpsHandler) UpdateAlertEventStatus(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid event ID")
		return
REDACTED

	var payload struct {
		Status string `json:"status"`
REDACTED
	if err := c.ShouldBindJSON(&payload); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
REDACTED
	payload.Status = strings.TrimSpace(payload.Status)
	if payload.Status == "" {
		response.BadRequest(c, "Invalid status")
		return
REDACTED
	if payload.Status != service.OpsAlertStatusResolved && payload.Status != service.OpsAlertStatusManualResolved {
		response.BadRequest(c, "Invalid status")
		return
REDACTED

	var resolvedAt *time.Time
	if payload.Status == service.OpsAlertStatusResolved || payload.Status == service.OpsAlertStatusManualResolved {
		now := time.Now().UTC()
		resolvedAt = &now
REDACTED
	if err := h.opsService.UpdateAlertEventStatus(c.Request.Context(), id, payload.Status, resolvedAt); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"updated": trueREDACTED)
REDACTED

// ListAlertEvents lists recent ops alert events.
// GET /api/v1/admin/ops/alert-events
// CreateAlertSilence creates a scoped silence for ops alerts.
// POST /api/v1/admin/ops/alert-silences
func (h *OpsHandler) CreateAlertSilence(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	var payload struct {
		RuleID   int64   `json:"rule_id"`
		Platform string  `json:"platform"`
		GroupID  *int64  `json:"group_id"`
		Region   *string `json:"region"`
		Until    string  `json:"until"`
		Reason   string  `json:"reason"`
REDACTED
	if err := c.ShouldBindJSON(&payload); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
REDACTED
	until, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.Until))
	if err != nil {
		response.BadRequest(c, "Invalid until")
		return
REDACTED

	createdBy := (*int64)(nil)
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok {
		uid := subject.UserID
		createdBy = &uid
REDACTED

	silence := &service.OpsAlertSilence{
		RuleID:    payload.RuleID,
		Platform:  strings.TrimSpace(payload.Platform),
		GroupID:   payload.GroupID,
		Region:    payload.Region,
		Until:     until,
		Reason:    strings.TrimSpace(payload.Reason),
		CreatedBy: createdBy,
REDACTED

	created, err := h.opsService.CreateAlertSilence(c.Request.Context(), silence)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, created)
REDACTED

func (h *OpsHandler) ListAlertEvents(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			response.BadRequest(c, "Invalid limit")
			return
	REDACTED
		limit = n
REDACTED

	filter := &service.OpsAlertEventFilter{
		Limit:    limit,
		Status:   strings.TrimSpace(c.Query("status")),
		Severity: strings.TrimSpace(c.Query("severity")),
REDACTED

	if v := strings.TrimSpace(c.Query("email_sent")); v != "" {
		vv := strings.ToLower(v)
		switch vv {
		case "true", "1":
			b := true
			filter.EmailSent = &b
		case "false", "0":
			b := false
			filter.EmailSent = &b
		default:
			response.BadRequest(c, "Invalid email_sent")
			return
	REDACTED
REDACTED

	// Cursor pagination: both params must be provided together.
	rawTS := strings.TrimSpace(c.Query("before_fired_at"))
	rawID := strings.TrimSpace(c.Query("before_id"))
	if (rawTS == "") != (rawID == "") {
		response.BadRequest(c, "before_fired_at and before_id must be provided together")
		return
REDACTED
	if rawTS != "" {
		ts, err := time.Parse(time.RFC3339Nano, rawTS)
		if err != nil {
			if t2, err2 := time.Parse(time.RFC3339, rawTS); err2 == nil {
				ts = t2
		REDACTED else {
				response.BadRequest(c, "Invalid before_fired_at")
				return
		REDACTED
	REDACTED
		filter.BeforeFiredAt = &ts
REDACTED
	if rawID != "" {
		id, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid before_id")
			return
	REDACTED
		filter.BeforeID = &id
REDACTED

	// Optional global filter support (platform/group/time range).
	if platform := strings.TrimSpace(c.Query("platform")); platform != "" {
		filter.Platform = platform
REDACTED
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
	REDACTED
		filter.GroupID = &id
REDACTED
	if startTime, endTime, err := parseOpsTimeRange(c, "24h"); err == nil {
		// Only apply when explicitly provided to avoid surprising default narrowing.
		if strings.TrimSpace(c.Query("start_time")) != "" || strings.TrimSpace(c.Query("end_time")) != "" || strings.TrimSpace(c.Query("time_range")) != "" {
			filter.StartTime = &startTime
			filter.EndTime = &endTime
	REDACTED
REDACTED else {
		response.BadRequest(c, err.Error())
		return
REDACTED

	events, err := h.opsService.ListAlertEvents(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, events)
REDACTED
