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

type withdrawQuotaCall struct {
	userID int64
	amount float64
}

type withdrawQuotaRepoStub struct {
	service.AffiliateRepository
	calls  []withdrawQuotaCall
	result *service.AffiliateWithdrawResult
	err    error
}

func (s *withdrawQuotaRepoStub) WithdrawQuota(_ context.Context, userID int64, amount float64) (*service.AffiliateWithdrawResult, error) {
	s.calls = append(s.calls, withdrawQuotaCall{userID: userID, amount: amount})
	return s.result, s.err
}

type withdrawQuotaResponse struct {
	Code   int                              `json:"code"`
	Reason string                           `json:"reason"`
	Data   *service.AffiliateWithdrawResult `json:"data"`
}

func performWithdrawQuota(t *testing.T, repo *withdrawQuotaRepoStub, userID, body string) (int, withdrawQuotaResponse) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAffiliateHandler(service.NewAffiliateService(repo, nil, nil, nil), nil)
	router.POST("/api/v1/admin/affiliates/users/:user_id/withdraw", handler.WithdrawQuota)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/affiliates/users/"+userID+"/withdraw", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var resp withdrawQuotaResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return rec.Code, resp
}

// TestAffiliateHandlerWithdrawQuota_Success 验证登记线下提现把路径中的用户与
// 舍入后的金额交给仓储，并返回登记结果。
func TestAffiliateHandlerWithdrawQuota_Success(t *testing.T) {
	repo := &withdrawQuotaRepoStub{result: &service.AffiliateWithdrawResult{LedgerID: 9, UserID: 42, Amount: 12.34567891}}
	status, resp := performWithdrawQuota(t, repo, "42", `{"amount":12.3456789149}`)

	require.Equal(t, http.StatusOK, status)
	require.Equal(t, 0, resp.Code)
	require.NotNil(t, resp.Data)
	require.Equal(t, int64(9), resp.Data.LedgerID)
	require.Equal(t, []withdrawQuotaCall{{userID: 42, amount: 12.34567891}}, repo.calls)
}

// TestAffiliateHandlerWithdrawQuota_RejectsBadRequests 验证非法用户 ID、非法 JSON
// 与非法金额在进入仓储前以 400 拒绝；金额错误带 AFFILIATE_WITHDRAW_AMOUNT_INVALID。
func TestAffiliateHandlerWithdrawQuota_RejectsBadRequests(t *testing.T) {
	cases := []struct {
		name       string
		userID     string
		body       string
		wantReason string
	}{
		{name: "non-numeric user id", userID: "abc", body: `{"amount":1}`},
		{name: "non-positive user id", userID: "0", body: `{"amount":1}`},
		{name: "malformed json", userID: "42", body: `{"amount":`},
		{name: "zero amount", userID: "42", body: `{"amount":0}`, wantReason: "AFFILIATE_WITHDRAW_AMOUNT_INVALID"},
		{name: "missing amount", userID: "42", body: `{}`, wantReason: "AFFILIATE_WITHDRAW_AMOUNT_INVALID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &withdrawQuotaRepoStub{}
			status, resp := performWithdrawQuota(t, repo, tc.userID, tc.body)

			require.Equal(t, http.StatusBadRequest, status)
			require.Equal(t, tc.wantReason, resp.Reason)
			require.Empty(t, repo.calls)
		})
	}
}

// TestAffiliateHandlerWithdrawQuota_InsufficientQuota 验证额度不足以 400 和
// AFFILIATE_QUOTA_INSUFFICIENT 返回，前端据此提示。
func TestAffiliateHandlerWithdrawQuota_InsufficientQuota(t *testing.T) {
	repo := &withdrawQuotaRepoStub{err: service.ErrAffiliateQuotaInsufficient}
	status, resp := performWithdrawQuota(t, repo, "42", `{"amount":5}`)

	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "AFFILIATE_QUOTA_INSUFFICIENT", resp.Reason)
	require.Len(t, repo.calls, 1)
}
