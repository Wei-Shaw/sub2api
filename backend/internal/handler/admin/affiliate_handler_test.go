package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAffiliateHandlerListRebateRecordsRejectsInvalidSourceType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := &AffiliateHandler{}
	router.GET("/api/v1/admin/affiliate/rebate-records", handler.ListRebateRecords)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/affiliate/rebate-records?source_type=invalid", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
}
