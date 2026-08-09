package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
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

func TestWeb3DepositHandlerRuntimeReturnsPerAssetEntries(t *testing.T) {
	runtime, err := web3deposit.NewScannerRuntime(nil, nil, web3deposit.ScannerRuntimeOptions{Enabled: false})
	require.NoError(t, err)
	handler := &Web3DepositHandler{
		deposits: &adminDepositReaderStub{},
		runtimes: scannerRuntimeRegistryStub{
			keys:    []web3deposit.RuntimeKey{{NetworkKey: "network", AssetKey: "usdt0"}},
			runtime: runtime,
		},
	}
	router := gin.New()
	router.GET("/runtime", handler.Runtime)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/runtime", nil))
	require.Equal(t, http.StatusOK, response.Code)

	var envelope struct {
		Data struct {
			Runtimes []struct {
				NetworkKey string `json:"network_key"`
				AssetKey   string `json:"asset_key"`
				State      string `json:"state"`
			} `json:"runtimes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.Equal(t, "network", envelope.Data.Runtimes[0].NetworkKey)
	require.Equal(t, "usdt0", envelope.Data.Runtimes[0].AssetKey)
	require.Equal(t, "disabled", envelope.Data.Runtimes[0].State)
}

func TestWeb3DepositHandlerRescanTargetsNetworkAndAsset(t *testing.T) {
	rescanner := &adminRescannerStub{}
	handler := &Web3DepositHandler{rescanner: rescanner}
	router := gin.New()
	router.POST("/rescan", handler.Rescan)

	response := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"network_key":"network","asset_key":"usdt0","from_block":"100","to_block":"120"}`)
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/rescan", body))
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "network", rescanner.networkKey)
	require.Equal(t, "usdt0", rescanner.assetKey)
	require.Equal(t, uint64(100), rescanner.fromBlock)
	require.Equal(t, uint64(120), rescanner.toBlock)
}

func TestWeb3DepositHandlerWritesAuditExtras(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &web3DepositAuditCaptureRepository{}
	auditService := service.NewAuditLogService(repository, nil)
	auditService.Start()

	deposit := web3deposit.Deposit{ID: 7, Status: web3deposit.DepositStatusManualReview}
	operator := &adminDepositOperatorStub{}
	rescanner := &adminRescannerStub{}
	handler := &Web3DepositHandler{deposits: &adminDepositReaderStub{deposit: deposit}, operator: operator, rescanner: rescanner}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 77})
		c.Set(string(servermiddleware.ContextKeyUserRole), "admin")
		c.Next()
	})
	router.Use(gin.HandlerFunc(servermiddleware.NewAuditLogMiddleware(auditService)))
	router.POST("/api/v1/admin/web3-deposits/:id/ignore", handler.Ignore)
	router.POST("/api/v1/admin/web3-deposits/rescan", handler.Rescan)

	ignoreResponse := httptest.NewRecorder()
	router.ServeHTTP(ignoreResponse, httptest.NewRequest(http.MethodPost, "/api/v1/admin/web3-deposits/7/ignore", bytes.NewBufferString(`{"reason":"large manual review"}`)))
	require.Equal(t, http.StatusOK, ignoreResponse.Code)

	rescanResponse := httptest.NewRecorder()
	router.ServeHTTP(rescanResponse, httptest.NewRequest(http.MethodPost, "/api/v1/admin/web3-deposits/rescan", bytes.NewBufferString(`{"network_key":"network","asset_key":"usdt0","from_block":"100","to_block":"120"}`)))
	require.Equal(t, http.StatusOK, rescanResponse.Code)
	auditService.Stop()

	logs := repository.snapshot()
	require.Len(t, logs, 2)
	byAction := make(map[string]*service.AuditLog, len(logs))
	for _, log := range logs {
		byAction[log.Action] = log
	}
	ignore := byAction["admin.web3_deposits.ignore"]
	require.NotNil(t, ignore)
	require.EqualValues(t, 7, ignore.Extra["deposit_id"])
	require.Equal(t, "manual_review", ignore.Extra["old_status"])
	require.Equal(t, "ignored", ignore.Extra["new_status"])
	require.Equal(t, "large manual review", ignore.Extra["reason"])

	rescan := byAction["admin.web3_deposits.rescan"]
	require.NotNil(t, rescan)
	require.Equal(t, "network", rescan.Extra["network_key"])
	require.Equal(t, "usdt0", rescan.Extra["asset_key"])
	require.EqualValues(t, 100, rescan.Extra["from_block"])
	require.EqualValues(t, 120, rescan.Extra["to_block"])
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

type scannerRuntimeRegistryStub struct {
	keys    []web3deposit.RuntimeKey
	runtime *web3deposit.ScannerRuntime
}

func (s scannerRuntimeRegistryStub) Keys() []web3deposit.RuntimeKey { return s.keys }

func (s scannerRuntimeRegistryStub) Runtime(string, string) (*web3deposit.ScannerRuntime, bool) {
	return s.runtime, s.runtime != nil
}

type adminRescannerStub struct {
	networkKey string
	assetKey   string
	fromBlock  uint64
	toBlock    uint64
}

type adminDepositOperatorStub struct{}

func (s *adminDepositOperatorStub) ApproveReviewedDeposit(context.Context, int64) error { return nil }
func (s *adminDepositOperatorStub) IgnoreReviewedDeposit(context.Context, int64, string) error {
	return nil
}
func (s *adminDepositOperatorStub) RetryFailedDeposit(context.Context, int64) error { return nil }

type web3DepositAuditCaptureRepository struct {
	logs []*service.AuditLog
}

func (r *web3DepositAuditCaptureRepository) BatchInsert(_ context.Context, logs []*service.AuditLog) (int64, error) {
	r.logs = append(r.logs, logs...)
	return int64(len(logs)), nil
}
func (r *web3DepositAuditCaptureRepository) Insert(_ context.Context, log *service.AuditLog) error {
	r.logs = append(r.logs, log)
	return nil
}
func (r *web3DepositAuditCaptureRepository) List(context.Context, *service.AuditLogFilter) (*service.AuditLogList, error) {
	return &service.AuditLogList{}, nil
}
func (r *web3DepositAuditCaptureRepository) GetByID(context.Context, int64) (*service.AuditLog, error) {
	return nil, service.ErrAuditLogNotFound
}
func (r *web3DepositAuditCaptureRepository) Count(context.Context) (int64, error) { return 0, nil }
func (r *web3DepositAuditCaptureRepository) TruncateAll(context.Context) error    { return nil }
func (r *web3DepositAuditCaptureRepository) DeleteBefore(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}
func (r *web3DepositAuditCaptureRepository) snapshot() []*service.AuditLog {
	return append([]*service.AuditLog(nil), r.logs...)
}

func (s *adminRescannerStub) Rescan(_ context.Context, networkKey, assetKey string, fromBlock, toBlock uint64) (web3deposit.BoundedRescanResult, error) {
	s.networkKey = networkKey
	s.assetKey = assetKey
	s.fromBlock = fromBlock
	s.toBlock = toBlock
	return web3deposit.BoundedRescanResult{FromBlock: fromBlock, ToBlock: toBlock}, nil
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
