package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type handlerWindowUsageAccountRepo struct {
	service.AccountRepository
	account *service.Account
	err     error
}

func (r *handlerWindowUsageAccountRepo) GetByID(context.Context, int64) (*service.Account, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.account, nil
}

type handlerWindowUsageLogRepo struct {
	service.UsageLogRepository
}

func (r *handlerWindowUsageLogRepo) GetAccountWindowUsage(
	_ context.Context,
	_ int64,
	queries []service.AccountWindowUsageQuery,
) ([]service.AccountWindowUsageAggregate, error) {
	result := make([]service.AccountWindowUsageAggregate, 0, len(queries))
	for _, query := range queries {
		result = append(result, service.AccountWindowUsageAggregate{
			WindowKey: query.WindowKey, Period: query.Period,
			StartTime: query.StartTime, EndTime: query.EndTime,
			SuccessCalls: 4, FailureCalls: 1, TotalTokens: 500, AccountCost: 0.75,
		})
	}
	return result, nil
}

func newWindowUsageHandler(accountRepo service.AccountRepository) *AccountHandler {
	usageService := service.NewAccountUsageService(
		accountRepo,
		&handlerWindowUsageLogRepo{},
		nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	return &AccountHandler{accountUsageService: usageService}
}

func validWindowUsageBody(t *testing.T) []byte {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	body, err := json.Marshal(service.AccountWindowUsageRequest{Windows: []service.AccountWindowUsageTarget{{
		WindowKey: service.AccountWindowKeyFiveHour,
		Period:    service.AccountWindowPeriodCurrent,
		StartTime: now.Add(-2 * time.Hour).Format(time.RFC3339),
		EndTime:   now.Add(-time.Hour).Format(time.RFC3339),
	}}})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return body
}

func performWindowUsageRequest(handler *AccountHandler, id string, body []byte) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/admin/accounts/:id/window-usage", handler.GetWindowUsage)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+id+"/window-usage", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAccountHandlerGetWindowUsage(t *testing.T) {
	handler := newWindowUsageHandler(&handlerWindowUsageAccountRepo{account: &service.Account{ID: 7}})
	recorder := performWindowUsageRequest(handler, "7", validWindowUsageBody(t))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Code int                                `json:"code"`
		Data service.AccountWindowUsageResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Code != 0 || len(envelope.Data.Items) != 1 {
		t.Fatalf("response = %#v", envelope)
	}
	item := envelope.Data.Items[0]
	if item.TotalRequests != 5 || item.SuccessRate != nil || item.SuccessRateStatus != service.SuccessRateStatusMonitoringDisabled {
		t.Fatalf("item = %#v", item)
	}
}

func TestAccountHandlerGetWindowUsageRejectsInvalidRequests(t *testing.T) {
	handler := newWindowUsageHandler(&handlerWindowUsageAccountRepo{account: &service.Account{ID: 7}})
	tests := []struct {
		name string
		id   string
		body []byte
	}{
		{name: "invalid account id", id: "invalid", body: validWindowUsageBody(t)},
		{name: "invalid json", id: "7", body: []byte(`{"windows":`)},
		{name: "empty targets", id: "7", body: []byte(`{"windows":[]}`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := performWindowUsageRequest(handler, test.id, test.body)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestAccountHandlerGetWindowUsageReturnsNotFound(t *testing.T) {
	handler := newWindowUsageHandler(&handlerWindowUsageAccountRepo{err: service.ErrAccountNotFound})
	recorder := performWindowUsageRequest(handler, "99", validWindowUsageBody(t))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
