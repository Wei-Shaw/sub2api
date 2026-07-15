package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/gin-gonic/gin"
)

const (
	snapshotCacheHeader = "X-Snapshot-Cache"
	usageCacheHeader    = "X-Usage-Stats-Cache"
)

type serverTimingResponseWriter struct {
	gin.ResponseWriter
	context *gin.Context
	once    sync.Once
REDACTED

func (w *serverTimingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
REDACTED

// ServerTiming collects timing for Admin and User web UI requests when enabled.
func ServerTiming(enabled bool) gin.HandlerFunc {
	if !enabled {
		return func(c *gin.Context) {
			c.Next()
	REDACTED
REDACTED
	return func(c *gin.Context) {
		if !shouldCollectServerTiming(c) || c.Request == nil {
			c.Next()
			return
	REDACTED

		collector := servertiming.New(time.Now())
		c.Request = c.Request.WithContext(servertiming.WithCollector(c.Request.Context(), collector))
		writer := &serverTimingResponseWriter{
			ResponseWriter: c.Writer,
			context:        c,
	REDACTED
		c.Writer = writer
		c.Next()
		writer.finalize()
REDACTED
REDACTED

func (w *serverTimingResponseWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
REDACTED

func (w *serverTimingResponseWriter) WriteHeaderNow() {
	w.finalize()
	w.ResponseWriter.WriteHeaderNow()
REDACTED

func (w *serverTimingResponseWriter) Write(data []byte) (int, error) {
	w.finalize()
	return w.ResponseWriter.Write(data)
REDACTED

func (w *serverTimingResponseWriter) WriteString(data string) (int, error) {
	w.finalize()
	return w.ResponseWriter.WriteString(data)
REDACTED

func (w *serverTimingResponseWriter) Flush() {
	w.finalize()
	w.ResponseWriter.Flush()
REDACTED

func (w *serverTimingResponseWriter) finalize() {
	if w == nil {
		return
REDACTED
	w.once.Do(func() {
		if value := ServerTimingHeaderValue(w.context); value != "" {
			w.ResponseWriter.Header().Set(servertiming.HeaderName, value)
	REDACTED
REDACTED)
REDACTED

// ServerTimingHeaderValue returns a timing value only for authorized UI scopes.
// Admins may receive timing for any collected Admin/User UI request. Non-admin
// authenticated users may receive timing only on allowlisted user-facing paths.
// X-User-UI-Request is a scope signal and is never used as authorization.
func ServerTimingHeaderValue(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
REDACTED
	role, ok := GetUserRoleFromContext(c)
	if !ok || role == "" {
		return ""
REDACTED
	if role != "admin" && !isUserTimingPath(c.Request.URL.Path) {
		return ""
REDACTED
	return servertiming.HeaderValue(c.Request.Context(), time.Now(), responseCacheStatus(c.Writer.Header()))
REDACTED

// ServerTimingResponseHeader builds the extra header map required by WebSocket upgrades.
func ServerTimingResponseHeader(c *gin.Context) http.Header {
	value := ServerTimingHeaderValue(c)
	if value == "" {
		return nil
REDACTED
	return http.Header{servertiming.HeaderName: []string{valueREDACTEDREDACTED
REDACTED

func shouldCollectServerTiming(c *gin.Context) bool {
	return isAdminUIRequest(c) || isUserUIRequest(c)
REDACTED

func isAdminUIRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
REDACTED
	if strings.TrimSpace(c.GetHeader(servertiming.AdminUIHeader)) == "1" {
		return true
REDACTED
	path := strings.TrimSpace(c.Request.URL.Path)
	return path == "/api/v1/admin" || strings.HasPrefix(path, "/api/v1/admin/")
REDACTED

func isUserUIRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
REDACTED
	if strings.TrimSpace(c.GetHeader(servertiming.UserUIHeader)) == "1" {
		return true
REDACTED
	return isUserTimingPath(c.Request.URL.Path)
REDACTED

// isUserTimingPath reports whether the path is a user-facing web API that may
// emit Server-Timing for authenticated callers (excluding public payment routes).
func isUserTimingPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
REDACTED
	const prefix = "/api/v1"
	if !strings.HasPrefix(path, prefix) {
		return false
REDACTED
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" {
		return false
REDACTED
	if !strings.HasPrefix(rest, "/") {
		rest = "/" + rest
REDACTED

	switch {
	case rest == "/auth/me",
		rest == "/auth/revoke-all-sessions",
		rest == "/auth/oauth/bind-token":
		return true
	case rest == "/user", strings.HasPrefix(rest, "/user/"):
		return true
	case rest == "/keys", strings.HasPrefix(rest, "/keys/"):
		return true
	case rest == "/groups/available", rest == "/groups/rates":
		return true
	case rest == "/channels/available":
		return true
	case rest == "/usage", strings.HasPrefix(rest, "/usage/"):
		return true
	case rest == "/announcements", strings.HasPrefix(rest, "/announcements/"):
		return true
	case rest == "/redeem", strings.HasPrefix(rest, "/redeem/"):
		return true
	case rest == "/subscriptions", strings.HasPrefix(rest, "/subscriptions/"):
		return true
	case rest == "/channel-monitors", strings.HasPrefix(rest, "/channel-monitors/"):
		return true
	case strings.HasPrefix(rest, "/payment/"):
		// Exclude public and webhook payment surfaces.
		if strings.HasPrefix(rest, "/payment/public") || strings.HasPrefix(rest, "/payment/webhook") {
			return false
	REDACTED
		return true
	default:
		return false
REDACTED
REDACTED

func responseCacheStatus(header http.Header) string {
	for _, name := range []string{snapshotCacheHeader, usageCacheHeaderREDACTED {
		switch strings.ToLower(strings.TrimSpace(header.Get(name))) {
		case "hit":
			return "hit"
		case "miss":
			return "miss"
		case "bypass":
			return "bypass"
	REDACTED
REDACTED
	return "bypass"
REDACTED
