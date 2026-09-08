package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/stretchr/testify/require"
)

type web3BalanceReaderStub struct {
	balances []web3deposit.UserBalance
	userID   int64
	err      error
}

func (s *web3BalanceReaderStub) ListUserBalances(_ context.Context, userID int64) ([]web3deposit.UserBalance, error) {
	s.userID = userID
	return s.balances, s.err
}

type web3TransferServiceStub struct {
	request web3deposit.TransferToMainBalanceRequest
	result  web3deposit.TransferToMainBalanceResult
	err     error
}

func (s *web3TransferServiceStub) TransferToMainBalance(_ context.Context, request web3deposit.TransferToMainBalanceRequest) (web3deposit.TransferToMainBalanceResult, error) {
	s.request = request
	return s.result, s.err
}

func TestWeb3DepositHandlerListsAuthenticatedUserBalances(t *testing.T) {
	reader := &web3BalanceReaderStub{balances: []web3deposit.UserBalance{{
		AssetKey: web3deposit.AssetKeyUSDT, AvailableAmount: "12.34000000",
		TotalDeposited: "15.00000000", TotalTransferred: "2.66000000",
		UpdatedAt: time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC),
	}}}
	handler := &Web3DepositHandler{balances: reader}
	router := authenticatedWeb3DepositTestRouter(handler)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/payment/web3/balances", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(42), reader.userID)
	var envelope struct {
		Data []web3UserBalanceResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data, 1)
	require.Equal(t, "12.34000000", envelope.Data[0].AvailableAmount)
}

func TestWeb3DepositHandlerTransfersBalanceWithUserScopedIdempotencyKey(t *testing.T) {
	transfers := &web3TransferServiceStub{result: web3deposit.TransferToMainBalanceResult{Transfer: web3deposit.BalanceTransfer{
		ID: 7, UserID: 42, Amount: "5.00000000", Web3BalanceBefore: "12.34000000",
		Web3BalanceAfter: "7.34000000", UserBalanceBefore: "1.00000000", UserBalanceAfter: "6.00000000",
	}}}
	handler := &Web3DepositHandler{transfers: transfers}
	router := authenticatedWeb3DepositTestRouter(handler)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/payment/web3/transfers", bytes.NewBufferString(`{"asset_key":"usdt","amount":"5"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "request-123")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(42), transfers.request.UserID)
	require.Equal(t, "usdt", transfers.request.AssetKey)
	require.Equal(t, "5", transfers.request.Amount)
	require.Equal(t, "web3-transfer:42:request-123", transfers.request.IdempotencyKey)
	require.Contains(t, recorder.Body.String(), "7.34000000")
}

func TestWeb3DepositHandlerRequiresTransferIdempotencyKey(t *testing.T) {
	handler := &Web3DepositHandler{transfers: &web3TransferServiceStub{}}
	router := authenticatedWeb3DepositTestRouter(handler)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/payment/web3/transfers", bytes.NewBufferString(`{"asset_key":"usdt","amount":"5"}`))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "WEB3_BALANCE_TRANSFER_IDEMPOTENCY_KEY_INVALID")
}
