package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWeb3DepositHandlerUsesAdminJSONContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	detectedAt := time.Date(2026, time.August, 8, 10, 0, 0, 0, time.UTC)
	creditedAmount := "12.34000000"
	deposit := web3deposit.Deposit{
		ID:               7,
		UserID:           42,
		DepositAddressID: 99,
		ChainID:          1030,
		TokenContract:    "0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff",
		TxHash:           "0x1111111111111111111111111111111111111111111111111111111111111111",
		LogIndex:         3,
		BlockNumber:      100,
		BlockHash:        "0x2222222222222222222222222222222222222222222222222222222222222222",
		FromAddress:      "0x3333333333333333333333333333333333333333",
		ToAddress:        "0x4444444444444444444444444444444444444444",
		RawAmount:        "12340000",
		TokenDecimals:    6,
		TokenAmount:      "12.340000",
		CreditedAmount:   &creditedAmount,
		Status:           web3deposit.DepositStatusCredited,
		RetryCount:       2,
		DetectedAt:       detectedAt,
		CreatedAt:        detectedAt,
		UpdatedAt:        detectedAt,
	}
	reader := &adminDepositReaderStub{deposit: deposit}
	handler := NewWeb3DepositHandler(reader, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/deposits", handler.List)
	router.GET("/deposits/:id", handler.Get)

	t.Run("list", func(t *testing.T) {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/deposits", nil))
		require.Equal(t, http.StatusOK, response.Code)

		var envelope struct {
			Data struct {
				Items []map[string]any `json:"items"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
		require.Len(t, envelope.Data.Items, 1)
		assertAdminWeb3DepositJSON(t, envelope.Data.Items[0])
	})

	t.Run("detail", func(t *testing.T) {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/deposits/7", nil))
		require.Equal(t, http.StatusOK, response.Code)

		var envelope struct {
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
		assertAdminWeb3DepositJSON(t, envelope.Data)
	})
}

func assertAdminWeb3DepositJSON(t *testing.T, item map[string]any) {
	t.Helper()
	require.Equal(t, float64(7), item["id"])
	require.Equal(t, float64(42), item["user_id"])
	require.Equal(t, "12.340000", item["token_amount"])
	require.Equal(t, "12.34000000", item["credited_amount"])
	for _, field := range []string{"ID", "UserID", "DepositAddressID", "deposit_address_id", "RawAmount", "raw_amount", "TokenDecimals", "token_decimals", "RetryCount", "retry_count", "NextRetryAt", "next_retry_at"} {
		require.NotContains(t, item, field)
	}
}

type adminDepositReaderStub struct {
	deposit web3deposit.Deposit
}

func (s *adminDepositReaderStub) ListAdminDeposits(context.Context, web3deposit.AdminDepositFilter) ([]web3deposit.Deposit, int64, error) {
	return []web3deposit.Deposit{s.deposit}, 1, nil
}

func (s *adminDepositReaderStub) GetAdminDeposit(context.Context, int64) (web3deposit.Deposit, error) {
	return s.deposit, nil
}

func (s *adminDepositReaderStub) CountAdminDepositsByStatus(context.Context) (map[web3deposit.DepositStatus]int64, error) {
	return nil, nil
}
