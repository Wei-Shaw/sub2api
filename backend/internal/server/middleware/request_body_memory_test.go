package middleware

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestBodyMemoryRejectsOverloadAndReleasesAfterHandler(t *testing.T) {
	router := gin.New()
	held := make(chan struct{})
	release := make(chan struct{})
	router.POST("/v1/responses", RequestBodyLimit(2048, pkghttputil.NewBodyMemoryBudget(1024)), func(c *gin.Context) {
		_, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if RejectRequestBodyMemoryOverload(c, err) {
			return
		}
		if err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		if c.Query("hold") == "1" {
			close(held)
			<-release
		}
		c.Status(http.StatusNoContent)
	})
	request := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(strings.Repeat("x", 700))))
		return w
	}
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { firstDone <- request("/v1/responses?hold=1") }()
	releaseHeld := sync.OnceFunc(func() { close(release) })
	defer releaseHeld()
	select {
	case <-held:
	case <-time.After(5 * time.Second):
		t.Fatal("first request did not reach handler")
	}
	w := request("/v1/responses")
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Equal(t, "1", w.Header().Get("Retry-After"))
	releaseHeld()
	select {
	case first := <-firstDone:
		require.Equal(t, http.StatusNoContent, first.Code)
	case <-time.After(5 * time.Second):
		t.Fatal("held request did not finish")
	}
	require.Equal(t, http.StatusNoContent, request("/v1/responses").Code)
}

func TestRejectRequestBodyReadErrorCancellation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cancelled bool
		committed bool
		readErr   error
		handled   bool
	}{
		{name: "cancelled", cancelled: true, handled: true},
		{name: "cancelled_after_commit", cancelled: true, committed: true, handled: true},
		{name: "ordinary_read_error"},
		// An HTTP/1 read deadline also cancels the request context; the
		// timeout must reach AbortWithRequestBodyReadFailure (408).
		{name: "read_timeout_with_cancelled_context", cancelled: true, readErr: &net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
			if tc.committed {
				_, err := c.Writer.WriteString("already committed")
				require.NoError(t, err)
			}
			bodyBefore := recorder.Body.String()
			readErr := tc.readErr
			if readErr == nil {
				readErr = io.ErrUnexpectedEOF
			}
			require.Equal(t, tc.handled, RejectRequestBodyReadError(c, "test", readErr))
			require.Equal(t, tc.handled, c.IsAborted())
			status := http.StatusOK
			if tc.handled && !tc.committed {
				status = 499
			}
			require.Equal(t, status, c.Writer.Status())
			require.Equal(t, bodyBefore, recorder.Body.String())
		})
	}
}
