package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
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
	}}, nil, nil)
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

type web3DepositAddressAllocatorStub struct {
	address    web3deposit.DepositAddress
	err        error
	calls      int
	userID     int64
	configured web3deposit.ConfiguredWallet
}

func (s *web3DepositAddressAllocatorStub) GetOrCreate(_ context.Context, userID int64, configured web3deposit.ConfiguredWallet) (web3deposit.DepositAddress, error) {
	s.calls++
	s.userID = userID
	s.configured = configured
	return s.address, s.err
}

func TestWeb3DepositHandlerGetOrCreateAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	allocator := &web3DepositAddressAllocatorStub{address: web3deposit.DepositAddress{Address: "0x1234567890abcdef1234567890ABCDEF12345678"}}
	handler := &Web3DepositHandler{
		cfg: &config.Config{Web3Deposit: config.Web3DepositConfig{
			Enabled:          true,
			UserEntryEnabled: true,
			Wallets: map[string]config.Web3DepositWalletConfig{
				"evm_wallet": {AccountXPub: "account_xpub", AccountPath: "m/44'/60'/0'"},
			},
			Networks: map[string]config.Web3DepositNetworkConfig{
				"z_network": {Enabled: true, WalletID: "evm_wallet"},
				"a_network": {Enabled: true, WalletID: "evm_wallet"},
			},
		}},
		addressAllocator: allocator,
	}
	router := authenticatedWeb3DepositTestRouter(handler)

	for range 2 {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/payment/web3/address", nil)
		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code)
		var envelope struct {
			Data web3DepositAddressResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
		require.Equal(t, "0x1234567890abcdef1234567890ABCDEF12345678", envelope.Data.Address)
		require.Equal(t, []string{"a_network", "z_network"}, envelope.Data.Networks)
	}

	require.Equal(t, 2, allocator.calls)
	require.Equal(t, int64(42), allocator.userID)
	require.Equal(t, "evm_wallet", allocator.configured.WalletID)
	require.Equal(t, "m/44'/60'/0'", allocator.configured.AccountPath)
	require.Equal(t, "account_xpub", allocator.configured.AccountXPub)
}

func TestWeb3DepositHandlerAddressDisabledFeatureDoesNotAllocate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	allocator := &web3DepositAddressAllocatorStub{}
	handler := &Web3DepositHandler{cfg: &config.Config{}, addressAllocator: allocator}
	router := authenticatedWeb3DepositTestRouter(handler)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/payment/web3/address", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "WEB3_DEPOSIT_DISABLED")
	require.Zero(t, allocator.calls)
}

func TestWeb3DepositHandlerUnhealthyNetworkDoesNotAllocate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	allocator := &web3DepositAddressAllocatorStub{}
	cfg := &config.Config{Web3Deposit: config.Web3DepositConfig{
		Enabled:          true,
		UserEntryEnabled: true,
		Wallets: map[string]config.Web3DepositWalletConfig{
			"evm_wallet": {AccountXPub: "account_xpub", AccountPath: "m/44'/60'/0'"},
		},
		Networks: map[string]config.Web3DepositNetworkConfig{
			"network": {Enabled: true, WalletID: "evm_wallet"},
		},
	}}
	handler := &Web3DepositHandler{
		cfg:              cfg,
		addressAllocator: allocator,
		networkRuntime:   web3deposit.NewConfluxNetworkRuntime(cfg),
	}
	router := authenticatedWeb3DepositTestRouter(handler)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/payment/web3/address", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "WEB3_DEPOSIT_UNAVAILABLE")
	require.Zero(t, allocator.calls)
}

func TestWeb3DepositHandlerMapsAddressAllocationConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	allocator := &web3DepositAddressAllocatorStub{err: web3deposit.ErrAddressAllocationConflict}
	handler := &Web3DepositHandler{
		cfg: &config.Config{Web3Deposit: config.Web3DepositConfig{
			Enabled:          true,
			UserEntryEnabled: true,
			Wallets: map[string]config.Web3DepositWalletConfig{
				"evm_wallet": {AccountXPub: "account_xpub", AccountPath: "m/44'/60'/0'"},
			},
			Networks: map[string]config.Web3DepositNetworkConfig{
				"network": {Enabled: true, WalletID: "evm_wallet"},
			},
		}},
		addressAllocator: allocator,
	}
	router := authenticatedWeb3DepositTestRouter(handler)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/payment/web3/address", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), "WEB3_DEPOSIT_ADDRESS_CONFLICT")
}

func authenticatedWeb3DepositTestRouter(handler *Web3DepositHandler) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
		c.Next()
	})
	router.POST("/api/v1/payment/web3/address", handler.GetOrCreateAddress)
	return router
}
