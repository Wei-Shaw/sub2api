package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/stretchr/testify/require"
)

type web3DepositReaderStub struct {
	items     []web3deposit.Deposit
	total     int64
	detail    web3deposit.Deposit
	err       error
	userID    int64
	depositID int64
	page      int
	pageSize  int
	filter    web3deposit.UserDepositFilter
}

func (s *web3DepositReaderStub) ListUserDeposits(_ context.Context, userID int64, filter web3deposit.UserDepositFilter) ([]web3deposit.Deposit, int64, error) {
	s.userID, s.page, s.pageSize, s.filter = userID, filter.Page, filter.PageSize, filter
	return s.items, s.total, s.err
}

func TestWeb3DepositHandlerFiltersHistoryByConfiguredNetworkAsset(t *testing.T) {
	reader := &web3DepositReaderStub{}
	handler := &Web3DepositHandler{
		cfg: &config.Config{Web3Deposit: config.Web3DepositConfig{Networks: map[string]config.Web3DepositNetworkConfig{
			"conflux_testnet": {Enabled: true, ChainID: 71, Assets: map[string]config.Web3DepositAssetConfig{
				"test_usdt0": {ContractAddress: testWeb3TokenContract},
			}},
		}}},
		deposits: reader,
	}
	router := authenticatedWeb3DepositTestRouter(handler)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/payment/web3/deposits?network_key=conflux_testnet&asset_key=test_usdt0", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, uint64(71), reader.filter.ChainID)
	require.Equal(t, testWeb3TokenContract, reader.filter.TokenContract)
}

func (s *web3DepositReaderStub) GetUserDeposit(_ context.Context, userID, depositID int64) (web3deposit.Deposit, error) {
	s.userID, s.depositID = userID, depositID
	return s.detail, s.err
}

func TestWeb3DepositHandlerListsOnlyAuthenticatedUserDeposits(t *testing.T) {
	reader := &web3DepositReaderStub{
		total: 1,
		items: []web3deposit.Deposit{{
			ID: 7, ChainID: 1030, TokenContract: testWeb3TokenContract,
			TxHash: testWeb3TxHash, LogIndex: 2, BlockNumber: 99,
			TokenAmount: "12.340000", Status: web3deposit.DepositStatusCrediting,
			DetectedAt: time.Date(2026, 8, 8, 1, 2, 3, 0, time.UTC),
		}},
	}
	handler := &Web3DepositHandler{deposits: reader}
	router := authenticatedWeb3DepositTestRouter(handler)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/payment/web3/deposits?page=2&page_size=10", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(42), reader.userID)
	require.Equal(t, 2, reader.page)
	require.Equal(t, 10, reader.pageSize)
	var envelope struct {
		Data struct {
			Items []web3DepositUserResponse `json:"items"`
			Total int64                     `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, int64(1), envelope.Data.Total)
	require.Equal(t, "confirming", envelope.Data.Items[0].Status)
	require.Equal(t, "12.340000", envelope.Data.Items[0].TokenAmount)
}

func TestWeb3DepositHandlerDoesNotRevealOtherUsersDeposit(t *testing.T) {
	reader := &web3DepositReaderStub{err: web3deposit.ErrDepositNotFound}
	handler := &Web3DepositHandler{deposits: reader}
	router := authenticatedWeb3DepositTestRouter(handler)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/payment/web3/deposits/99", nil))

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Equal(t, int64(42), reader.userID)
	require.Equal(t, int64(99), reader.depositID)
	require.Contains(t, recorder.Body.String(), "WEB3_DEPOSIT_NOT_FOUND")
}

func TestWeb3DepositUserStatusHidesInternalStates(t *testing.T) {
	tests := map[web3deposit.DepositStatus]string{
		web3deposit.DepositStatusDetected:      "confirming",
		web3deposit.DepositStatusReadyToCredit: "confirming",
		web3deposit.DepositStatusCrediting:     "confirming",
		web3deposit.DepositStatusCredited:      "credited",
		web3deposit.DepositStatusBelowMinimum:  "below_minimum",
		web3deposit.DepositStatusManualReview:  "under_review",
		web3deposit.DepositStatusOrphaned:      "failed",
		web3deposit.DepositStatusFailed:        "failed",
		web3deposit.DepositStatusIgnored:       "failed",
	}
	for status, expected := range tests {
		require.Equal(t, expected, web3DepositUserStatus(status))
	}
}

const (
	testWeb3TxHash        = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testWeb3TokenContract = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)
