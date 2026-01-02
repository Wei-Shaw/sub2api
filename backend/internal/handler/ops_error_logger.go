package handler

import (
	"context"
	"strings"
	"sync"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	opsModelKey  = "ops_model"
	opsStreamKey = "ops_stream"
)

const (
	opsErrorLogWorkerCount = 10
	opsErrorLogQueueSize   = 256
	opsErrorLogTimeout     = 2 * time.Second
)

type opsErrorLogJob struct {
	ops   *service.OpsService
	entry *service.OpsErrorLog
REDACTED

var (
	opsErrorLogOnce  sync.Once
	opsErrorLogQueue chan opsErrorLogJob
)

func startOpsErrorLogWorkers() {
	opsErrorLogQueue = make(chan opsErrorLogJob, opsErrorLogQueueSize)
	for i := 0; i < opsErrorLogWorkerCount; i++ {
		go func() {
			for job := range opsErrorLogQueue {
				if job.ops == nil || job.entry == nil {
					continue
			REDACTED
				ctx, cancel := context.WithTimeout(context.Background(), opsErrorLogTimeout)
				_ = job.ops.RecordError(ctx, job.entry)
				cancel()
		REDACTED
	REDACTED()
REDACTED
REDACTED

func enqueueOpsErrorLog(ops *service.OpsService, entry *service.OpsErrorLog) {
	if ops == nil || entry == nil {
		return
REDACTED

	opsErrorLogOnce.Do(startOpsErrorLogWorkers)

	select {
	case opsErrorLogQueue <- opsErrorLogJob{ops: ops, entry: entryREDACTED:
	default:
		// Queue is full; drop to avoid blocking request handling.
REDACTED
REDACTED

func setOpsRequestContext(c *gin.Context, model string, stream bool) {
	c.Set(opsModelKey, model)
	c.Set(opsStreamKey, stream)
REDACTED

func recordOpsError(c *gin.Context, ops *service.OpsService, status int, errType, message, fallbackPlatform string) {
	if ops == nil || c == nil {
		return
REDACTED

	model, _ := c.Get(opsModelKey)
	stream, _ := c.Get(opsStreamKey)

	var modelName string
	if m, ok := model.(string); ok {
		modelName = m
REDACTED
	streaming, _ := stream.(bool)

	apiKey, _ := middleware2.GetAPIKeyFromContext(c)

	logEntry := &service.OpsErrorLog{
		Phase:      classifyOpsPhase(errType, message),
		Type:       errType,
		Severity:   classifyOpsSeverity(errType, status),
		StatusCode: status,
		Platform:   resolveOpsPlatform(apiKey, fallbackPlatform),
		Model:      modelName,
		RequestID:  c.Writer.Header().Get("x-request-id"),
		Message:    message,
		ClientIP:   c.ClientIP(),
		RequestPath: func() string {
			if c.Request != nil && c.Request.URL != nil {
				return c.Request.URL.Path
		REDACTED
			return ""
	REDACTED(),
		Stream: streaming,
REDACTED

	if apiKey != nil {
		logEntry.APIKeyID = &apiKey.ID
		if apiKey.User != nil {
			logEntry.UserID = &apiKey.User.ID
	REDACTED
		if apiKey.GroupID != nil {
			logEntry.GroupID = apiKey.GroupID
	REDACTED
REDACTED

	enqueueOpsErrorLog(ops, logEntry)
REDACTED

func resolveOpsPlatform(apiKey *service.APIKey, fallback string) string {
	if apiKey != nil && apiKey.Group != nil && apiKey.Group.Platform != "" {
		return apiKey.Group.Platform
REDACTED
	return fallback
REDACTED

func classifyOpsPhase(errType, message string) string {
	msg := strings.ToLower(message)
	switch errType {
	case "authentication_error":
		return "auth"
	case "billing_error", "subscription_error":
		return "billing"
	case "rate_limit_error":
		if strings.Contains(msg, "concurrency") || strings.Contains(msg, "pending") {
			return "concurrency"
	REDACTED
		return "upstream"
	case "invalid_request_error":
		return "response"
	case "upstream_error", "overloaded_error":
		return "upstream"
	case "api_error":
		if strings.Contains(msg, "no available accounts") {
			return "scheduling"
	REDACTED
		return "internal"
	default:
		return "internal"
REDACTED
REDACTED

func classifyOpsSeverity(errType string, status int) string {
	switch errType {
	case "invalid_request_error", "authentication_error", "billing_error", "subscription_error":
		return "P3"
REDACTED
	if status >= 500 {
		return "P1"
REDACTED
	if status == 429 {
		return "P1"
REDACTED
	if status >= 400 {
		return "P2"
REDACTED
	return "P3"
REDACTED
