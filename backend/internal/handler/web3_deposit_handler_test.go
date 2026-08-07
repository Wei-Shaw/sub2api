package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWeb3DepositHandlerGetConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewWeb3DepositHandler(&config.Config{Web3Deposit: config.Web3DepositConfig{
		Enabled:          true,
		UserEntryEnabled: true,
		Networks: map[string]config.Web3DepositNetworkConfig{
			"conflux_espace_mainnet": {
				Enabled:     true,
				DisplayName: "Conflux eSpace Mainnet",
				ChainID:     1030,
				Assets: map[string]config.Web3DepositAssetConfig{
					"usdt0": {ContractAddress: "0xaf37", Decimals: 6, MinimumDeposit: "1.000000", AutoCreditLimit: "10000.000000"},
				},
			},
		},
	}})
	router := gin.New()
	router.GET("/api/v1/payment/web3/config", handler.GetConfig)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/payment/web3/config", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var envelope response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Zero(t, envelope.Code)

	var payload struct {
		Enabled  bool `json:"enabled"`
		Networks []struct {
			ChainID string `json:"chain_id"`
			Assets  []struct {
				MinimumDeposit       string `json:"minimum_deposit"`
				AutomaticCreditLimit string `json:"automatic_credit_limit"`
			} `json:"assets"`
		} `json:"networks"`
	}
	data, err := json.Marshal(envelope.Data)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &payload))
	require.True(t, payload.Enabled)
	require.Equal(t, "1030", payload.Networks[0].ChainID)
	require.Equal(t, "1.000000", payload.Networks[0].Assets[0].MinimumDeposit)
	require.Equal(t, "10000.000000", payload.Networks[0].Assets[0].AutomaticCreditLimit)
}
