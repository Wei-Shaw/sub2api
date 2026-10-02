package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const testWithdrawIdempotencyKey = "affiliate-withdraw-1-5d0c7a2e"

type withdrawQuotaCall struct {
	userID      int64
	amount      float64
	operationID string
}

type withdrawQuotaRepoStub struct {
	service.AffiliateRepository
	calls  []withdrawQuotaCall
	result *service.AffiliateWithdrawResult
	err    error
}

func (s *withdrawQuotaRepoStub) WithdrawQuota(_ context.Context, userID int64, amount float64, operationID string) (*service.AffiliateWithdrawResult, error) {
	s.calls = append(s.calls, withdrawQuotaCall{userID: userID, amount: amount, operationID: operationID})
	return s.result, s.err
}

type withdrawQuotaResponse struct {
	Code   int             `json:"code"`
	Reason string          `json:"reason"`
	Data   json.RawMessage `json:"data"`
}

func performWithdrawQuota(t *testing.T, repo *withdrawQuotaRepoStub, userID, idempotencyKey, body string) (*httptest.ResponseRecorder, withdrawQuotaResponse) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAffiliateHandler(service.NewAffiliateService(repo, nil, nil, nil), nil)
	router.POST("/api/v1/admin/affiliates/users/:user_id/withdraw", handler.WithdrawQuota)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/affiliates/users/"+userID+"/withdraw", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var resp withdrawQuotaResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return rec, resp
}

// TestAffiliateHandlerWithdrawQuota_Success 验证登记线下提现把路径中的用户、
// 舍入后的金额与幂等键派生的 operation_id 交给仓储，并返回登记结果。
func TestAffiliateHandlerWithdrawQuota_Success(t *testing.T) {
	repo := &withdrawQuotaRepoStub{result: &service.AffiliateWithdrawResult{LedgerID: 9, UserID: 42, Amount: 12.34567891}}
	rec, resp := performWithdrawQuota(t, repo, "42", testWithdrawIdempotencyKey, `{"amount":12.3456789149}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 0, resp.Code)
	var data service.AffiliateWithdrawResult
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	require.Equal(t, int64(9), data.LedgerID)
	require.Empty(t, rec.Header().Get("X-Idempotency-Replayed"))

	require.Len(t, repo.calls, 1)
	require.Equal(t, int64(42), repo.calls[0].userID)
	require.Equal(t, 12.34567891, repo.calls[0].amount)
	require.Regexp(t, `^[0-9a-f]{64}$`, repo.calls[0].operationID)
}

// TestAffiliateHandlerWithdrawQuota_SameKeyMapsToSameOperation 验证重试时带同一个
// Idempotency-Key 的请求落到同一个 operation_id，换一个键则是另一笔登记。
func TestAffiliateHandlerWithdrawQuota_SameKeyMapsToSameOperation(t *testing.T) {
	repo := &withdrawQuotaRepoStub{result: &service.AffiliateWithdrawResult{LedgerID: 9, UserID: 42, Amount: 10}}
	for _, key := range []string{testWithdrawIdempotencyKey, testWithdrawIdempotencyKey, testWithdrawIdempotencyKey + "-next"} {
		rec, _ := performWithdrawQuota(t, repo, "42", key, `{"amount":10}`)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	require.Len(t, repo.calls, 3)
	require.Equal(t, repo.calls[0].operationID, repo.calls[1].operationID)
	require.NotEqual(t, repo.calls[0].operationID, repo.calls[2].operationID)
}

// TestAffiliateHandlerWithdrawQuota_ReplayReturnsFirstResult 验证命中已有登记时
// 原样返回首次登记的结果，并以 X-Idempotency-Replayed 标明是重放。
func TestAffiliateHandlerWithdrawQuota_ReplayReturnsFirstResult(t *testing.T) {
	repo := &withdrawQuotaRepoStub{result: &service.AffiliateWithdrawResult{
		LedgerID:            9,
		UserID:              42,
		Amount:              10,
		AvailableQuotaAfter: 90,
		HistoryQuotaAfter:   100,
		Replayed:            true,
	}}
	rec, resp := performWithdrawQuota(t, repo, "42", testWithdrawIdempotencyKey, `{"amount":10}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "true", rec.Header().Get("X-Idempotency-Replayed"))
	require.JSONEq(t, `{
		"ledger_id": 9,
		"user_id": 42,
		"amount": 10,
		"available_quota_after": 90,
		"frozen_quota_after": 0,
		"history_quota_after": 100
	}`, string(resp.Data))
}

// TestAffiliateHandlerWithdrawQuota_RejectsBadRequests 验证非法用户 ID、非法 JSON、
// 缺少或非法的幂等键与非法金额在进入仓储前以 400 拒绝。
func TestAffiliateHandlerWithdrawQuota_RejectsBadRequests(t *testing.T) {
	cases := []struct {
		name       string
		userID     string
		key        string
		body       string
		wantReason string
	}{
		{name: "non-numeric user id", userID: "abc", key: testWithdrawIdempotencyKey, body: `{"amount":1}`},
		{name: "non-positive user id", userID: "0", key: testWithdrawIdempotencyKey, body: `{"amount":1}`},
		{name: "malformed json", userID: "42", key: testWithdrawIdempotencyKey, body: `{"amount":`},
		{name: "missing idempotency key", userID: "42", body: `{"amount":1}`, wantReason: "IDEMPOTENCY_KEY_REQUIRED"},
		{name: "invalid idempotency key", userID: "42", key: "has space", body: `{"amount":1}`, wantReason: "IDEMPOTENCY_KEY_INVALID"},
		{name: "zero amount", userID: "42", key: testWithdrawIdempotencyKey, body: `{"amount":0}`, wantReason: "AFFILIATE_WITHDRAW_AMOUNT_INVALID"},
		{name: "missing amount", userID: "42", key: testWithdrawIdempotencyKey, body: `{}`, wantReason: "AFFILIATE_WITHDRAW_AMOUNT_INVALID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &withdrawQuotaRepoStub{}
			rec, resp := performWithdrawQuota(t, repo, tc.userID, tc.key, tc.body)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, tc.wantReason, resp.Reason)
			require.Empty(t, repo.calls)
		})
	}
}

// TestAffiliateHandlerWithdrawQuota_RepositoryRejections 验证额度不足以 400、
// 同一幂等键换了用户或金额以 409 返回，前端据错误码提示。
func TestAffiliateHandlerWithdrawQuota_RepositoryRejections(t *testing.T) {
	cases := []struct {
		err        error
		wantStatus int
		wantReason string
	}{
		{err: service.ErrAffiliateQuotaInsufficient, wantStatus: http.StatusBadRequest, wantReason: "AFFILIATE_QUOTA_INSUFFICIENT"},
		{err: service.ErrIdempotencyKeyConflict, wantStatus: http.StatusConflict, wantReason: "IDEMPOTENCY_KEY_CONFLICT"},
	}
	for _, tc := range cases {
		t.Run(tc.wantReason, func(t *testing.T) {
			repo := &withdrawQuotaRepoStub{err: tc.err}
			rec, resp := performWithdrawQuota(t, repo, "42", testWithdrawIdempotencyKey, `{"amount":5}`)

			require.Equal(t, tc.wantStatus, rec.Code)
			require.Equal(t, tc.wantReason, resp.Reason)
			require.Empty(t, rec.Header().Get("X-Idempotency-Replayed"))
			require.Len(t, repo.calls, 1)
		})
	}
}
