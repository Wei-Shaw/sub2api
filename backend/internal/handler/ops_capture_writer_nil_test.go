package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpsCaptureWriter_NilInnerWriter_NoPanic(t *testing.T) {
	w := &opsCaptureWriter{REDACTED
	w.ResponseWriter = nil

	assert.NotPanics(t, func() {
		assert.Equal(t, 0, w.Status())
REDACTED)
	assert.NotPanics(t, func() {
		assert.Equal(t, -1, w.Size())
REDACTED)
	assert.NotPanics(t, func() {
		assert.False(t, w.Written())
REDACTED)
	assert.NotPanics(t, func() {
		n, err := w.Write([]byte("test"))
		assert.Equal(t, 0, n)
		assert.NoError(t, err)
REDACTED)
	assert.NotPanics(t, func() {
		n, err := w.WriteString("test")
		assert.Equal(t, 0, n)
		assert.NoError(t, err)
REDACTED)
	assert.NotPanics(t, func() {
		h := w.Header()
		assert.NotNil(t, h)
REDACTED)
	assert.NotPanics(t, func() {
		w.WriteHeader(200)
REDACTED)
	assert.NotPanics(t, func() {
		w.WriteHeaderNow()
REDACTED)
	assert.NotPanics(t, func() {
		w.Flush()
REDACTED)
	assert.NotPanics(t, func() {
		conn, rw, err := w.Hijack()
		assert.Nil(t, conn)
		assert.Nil(t, rw)
		assert.Error(t, err)
REDACTED)
	assert.NotPanics(t, func() {
		ch := w.CloseNotify()
		assert.NotNil(t, ch)
REDACTED)
	assert.NotPanics(t, func() {
		p := w.Pusher()
		assert.Nil(t, p)
REDACTED)
REDACTED

func TestOpsCaptureWriter_CompactKeepaliveRestoresOriginalWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	outerStatus := -1
	router.Use(func(c *gin.Context) {
		c.Next()
		outerStatus = c.Writer.Status()
REDACTED)
	router.Use(OpsErrorLoggerMiddleware(nil))
	router.GET("/compact", func(c *gin.Context) {
		service.MarkOpenAICompactClientStream(c)
		stop := service.StartOpenAICompactSSEKeepalive(c, time.Hour)
		defer stop()
		c.Status(http.StatusOK)
REDACTED)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/compact", nil)
	require.NotPanics(t, func() {
		router.ServeHTTP(recorder, request)
REDACTED)
	require.Equal(t, http.StatusOK, outerStatus)
	require.Equal(t, http.StatusOK, recorder.Code)
REDACTED
