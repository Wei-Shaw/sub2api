package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBatchUsageSourceValidation(t *testing.T) {
	for _, source := range []string{"", "passive", "active", "invalid"} {
		t.Run(source, func(t *testing.T) {
			router := gin.New()
			handler := &AccountHandler{}
			router.POST("/usage/batch", handler.GetBatchUsage)
			req := httptest.NewRequest(http.MethodPost, "/usage/batch", strings.NewReader(`{"account_ids":[],"source":"`+source+`"}`))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			want := http.StatusOK
			if source == "invalid" {
				want = http.StatusBadRequest
			}
			require.Equal(t, want, recorder.Code)
		})
	}
}
