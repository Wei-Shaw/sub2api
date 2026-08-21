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

type blockingOpsResponseWriter struct {
	gin.ResponseWriter
	writeStarted chan struct{}
	writeRelease chan struct{}
}

func (w *blockingOpsResponseWriter) WriteString(s string) (int, error) {
	close(w.writeStarted)
	<-w.writeRelease
	return w.ResponseWriter.WriteString(s)
}

type deterministicOpsCaptureWriterStatePool struct {
	states []*opsCaptureWriterState
}

func (p *deterministicOpsCaptureWriterStatePool) Get() any {
	if len(p.states) == 0 {
		return &opsCaptureWriterState{limit: opsCaptureWriterLimit}
	}
	last := len(p.states) - 1
	state := p.states[last]
	p.states = p.states[:last]
	return state
}

func (p *deterministicOpsCaptureWriterStatePool) Put(value any) {
	if state, ok := value.(*opsCaptureWriterState); ok && state != nil {
		p.states = append(p.states, state)
	}
}

func TestOpsCaptureWriter_NilInnerWriter_NoPanic(t *testing.T) {
	w := &opsCaptureWriter{}

	assert.NotPanics(t, func() {
		assert.Equal(t, 0, w.Status())
	})
	assert.NotPanics(t, func() {
		assert.Equal(t, -1, w.Size())
	})
	assert.NotPanics(t, func() {
		assert.False(t, w.Written())
	})
	assert.NotPanics(t, func() {
		n, err := w.Write([]byte("test"))
		assert.Equal(t, 0, n)
		assert.NoError(t, err)
	})
	assert.NotPanics(t, func() {
		n, err := w.WriteString("test")
		assert.Equal(t, 0, n)
		assert.NoError(t, err)
	})
	assert.NotPanics(t, func() {
		h := w.Header()
		assert.NotNil(t, h)
	})
	assert.NotPanics(t, func() {
		w.WriteHeader(200)
	})
	assert.NotPanics(t, func() {
		w.WriteHeaderNow()
	})
	assert.NotPanics(t, func() {
		w.Flush()
	})
	assert.NotPanics(t, func() {
		conn, rw, err := w.Hijack()
		assert.Nil(t, conn)
		assert.Nil(t, rw)
		assert.Error(t, err)
	})
	assert.NotPanics(t, func() {
		ch := w.CloseNotify()
		assert.NotNil(t, ch)
	})
	assert.NotPanics(t, func() {
		p := w.Pusher()
		assert.Nil(t, p)
	})
}

func TestOpsCaptureWriter_CompactKeepaliveRestoresOriginalWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	outerStatus := -1
	router.Use(func(c *gin.Context) {
		c.Next()
		outerStatus = c.Writer.Status()
	})
	router.Use(OpsErrorLoggerMiddleware(nil))
	router.GET("/compact", func(c *gin.Context) {
		service.MarkOpenAICompactClientStream(c)
		stop := service.StartOpenAICompactSSEKeepalive(c, time.Hour)
		defer stop()
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/compact", nil)
	require.NotPanics(t, func() {
		router.ServeHTTP(recorder, request)
	})
	require.Equal(t, http.StatusOK, outerStatus)
	require.Equal(t, http.StatusOK, recorder.Code)
}

func TestOpsCaptureWriter_StaleLeaseCannotReachReacquiredState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pool := &deterministicOpsCaptureWriterStatePool{}

	firstRecorder := httptest.NewRecorder()
	firstContext, _ := gin.CreateTestContext(firstRecorder)
	stale := acquireOpsCaptureWriterFromPool(pool, firstContext.Writer)
	releaseOpsCaptureWriter(stale)

	secondRecorder := httptest.NewRecorder()
	secondContext, _ := gin.CreateTestContext(secondRecorder)
	current := acquireOpsCaptureWriterFromPool(pool, secondContext.Writer)
	defer releaseOpsCaptureWriter(current)
	require.NotSame(t, stale, current)
	require.Same(t, stale.state, current.state)

	current.WriteHeader(http.StatusInternalServerError)
	_, err := current.WriteString("current")
	require.NoError(t, err)
	require.Equal(t, []byte("current"), current.capturedBytes())

	n, err := stale.WriteString("stale")
	require.NoError(t, err)
	require.Zero(t, n)
	require.Nil(t, stale.capturedBytes())
	require.Equal(t, []byte("current"), current.capturedBytes())
	require.NotContains(t, secondRecorder.Body.String(), "stale")

	// Releasing the stale handle must not return an active state to the pool.
	releaseOpsCaptureWriter(stale)
	thirdRecorder := httptest.NewRecorder()
	thirdContext, _ := gin.CreateTestContext(thirdRecorder)
	other := acquireOpsCaptureWriterFromPool(pool, thirdContext.Writer)
	defer releaseOpsCaptureWriter(other)
	require.NotSame(t, current.state, other.state)
}

func TestOpsCaptureWriter_ReleaseWaitsForDelegatedWriteWithoutHoldingStateMutex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pool := &deterministicOpsCaptureWriterStatePool{}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	inner := &blockingOpsResponseWriter{
		ResponseWriter: ctx.Writer,
		writeStarted:   make(chan struct{}),
		writeRelease:   make(chan struct{}),
	}
	w := acquireOpsCaptureWriterFromPool(pool, inner)

	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		_, _ = w.WriteString("body")
	}()
	<-inner.writeStarted

	if !w.state.mu.TryLock() {
		t.Fatal("state mutex remained held across the delegated network write")
	}
	w.state.mu.Unlock()

	releaseDone := make(chan struct{})
	go func() {
		releaseOpsCaptureWriter(w)
		close(releaseDone)
	}()
	select {
	case <-releaseDone:
		t.Fatal("release returned while a delegated write was still active")
	case <-time.After(20 * time.Millisecond):
	}
	require.Empty(t, pool.states)

	close(inner.writeRelease)
	<-writeDone
	select {
	case <-releaseDone:
	case <-time.After(time.Second):
		t.Fatal("release did not finish after the delegated write returned")
	}
	require.Len(t, pool.states, 1)
}
