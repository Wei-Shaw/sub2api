package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/keyprotection"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKeyProtectionNeverCapturesRestoredClientErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Set(keyprotection.ProtectedGinKey, true)
		w := acquireOpsCaptureWriter(c.Writer)
		w.setContext(c)
		secret := "ghp_" + strings.Repeat("Z", 36)
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, err := w.WriteString("event: error\ndata: {\"error\":{\"message\":\"" + secret + "\"}}\n\n")
			require.NoError(t, err)
		} else {
			w.WriteHeader(400)
			_, err := w.WriteString(`{"error":{"message":"` + secret + `"}}`)
			require.NoError(t, err)
		}
		w.finalizeCapture()
		require.Empty(t, w.capturedBytes())
		_, captured := w.capturedTerminalError()
		require.False(t, captured)
		require.Contains(t, rec.Body.String(), secret)
		releaseOpsCaptureWriter(w)
	}
}
