package middleware

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Logger 请求日志中间件
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 开始时间
		startTime := time.Now()

		// 请求路径
		path := c.Request.URL.Path

		// 处理请求
		c.Next()

		// 跳过健康检查等高频探针路径的日志
		if path == "/health" || path == "/setup/status" {
			return
	REDACTED

		endTime := time.Now()
		latency := endTime.Sub(startTime)

		method := c.Request.Method
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		protocol := c.Request.Proto
		accountID, hasAccountID := c.Request.Context().Value(ctxkey.AccountID).(int64)
		platform, _ := c.Request.Context().Value(ctxkey.Platform).(string)
		model, _ := c.Request.Context().Value(ctxkey.Model).(string)

		fields := []zap.Field{
			zap.String("component", "http.access"),
			zap.Int("status_code", statusCode),
			zap.Int64("latency_ms", latency.Milliseconds()),
			zap.String("client_ip", clientIP),
			zap.String("protocol", protocol),
			zap.String("method", method),
			zap.String("path", path),
	REDACTED
		if hasAccountID && accountID > 0 {
			fields = append(fields, zap.Int64("account_id", accountID))
	REDACTED
		if platform != "" {
			fields = append(fields, zap.String("platform", platform))
	REDACTED
		if model != "" {
			fields = append(fields, zap.String("model", model))
	REDACTED

		l := logger.FromContext(c.Request.Context()).With(fields...)
		l.Info("http request completed", zap.Time("completed_at", endTime))
		// 当全局日志级别高于 info（如 warn/error）时，access info 不会进入 zap core，
		// 这里补写一次 sink，保证 ops 系统日志仍可索引关键访问轨迹。
		if !logger.L().Core().Enabled(logger.LevelInfo) {
			sinkFields := map[string]any{
				"component":    "http.access",
				"status_code":  statusCode,
				"latency_ms":   latency.Milliseconds(),
				"client_ip":    clientIP,
				"protocol":     protocol,
				"method":       method,
				"path":         path,
				"completed_at": endTime,
		REDACTED
			if requestID, ok := c.Request.Context().Value(ctxkey.RequestID).(string); ok && requestID != "" {
				sinkFields["request_id"] = requestID
		REDACTED
			if clientRequestID, ok := c.Request.Context().Value(ctxkey.ClientRequestID).(string); ok && clientRequestID != "" {
				sinkFields["client_request_id"] = clientRequestID
		REDACTED
			if hasAccountID && accountID > 0 {
				sinkFields["account_id"] = accountID
		REDACTED
			if platform != "" {
				sinkFields["platform"] = platform
		REDACTED
			if model != "" {
				sinkFields["model"] = model
		REDACTED
			logger.WriteSinkEvent("info", "http.access", "http request completed", sinkFields)
	REDACTED

		if len(c.Errors) > 0 {
			l.Warn("http request contains gin errors", zap.String("errors", c.Errors.String()))
	REDACTED
REDACTED
REDACTED
