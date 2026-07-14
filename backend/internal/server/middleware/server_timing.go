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

// ServerTiming collects timing only for requests made by the Admin web UI.
func ServerTiming(enabled bool) gin.HandlerFunc {
	if !enabled {
		return func(c *gin.Context) {
			c.Next()
	REDACTED
REDACTED
	return func(c *gin.Context) {
		if !isAdminUIRequest(c) || c.Request == nil {
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

// ServerTimingHeaderValue returns a timing value only for an authenticated admin.
func ServerTimingHeaderValue(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
REDACTED
	role, ok := GetUserRoleFromContext(c)
	if !ok || role != "admin" {
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
