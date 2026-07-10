package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type adminReliabilitySnapshotSource struct{}

func (adminReliabilitySnapshotSource) ReliabilitySnapshot(context.Context, time.Time, int) (service.ReliabilityReconciliationSnapshot, error) {
	return service.ReliabilityReconciliationSnapshot{Rows: []service.ReliabilityReconciliationRow{{
		TaskID: 901, Status: service.VideoStatusSucceeded, SettlementStatus: service.VideoSettlementStatusPending, RemoteAssetAvailable: true,
	}}}, nil
}

func TestVideoReliabilityReconciliationDryRunReturnsOnlySafeProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewVideoHandlerWithReliability(nil, service.NewReliabilityReconciler(adminReliabilitySnapshotSource{}))
	router := gin.New()
	router.GET("/admin/video/reliability/reconciliation", handler.ReliabilityReconciliation)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/video/reliability/reconciliation?limit=25", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(envelope.Data.Items) != 1 {
		t.Fatalf("items = %#v", envelope.Data.Items)
	}
	item := envelope.Data.Items[0]
	if len(item) != 4 || item["code"] != service.ReliabilityCodeSuccessUnsettled || item["recommended_action"] != service.ReliabilityActionReviewRequired {
		t.Fatalf("unsafe or unexpected projection: %#v", item)
	}
}
