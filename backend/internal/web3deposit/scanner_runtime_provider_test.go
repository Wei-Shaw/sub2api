package web3deposit

import (
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestProvideScannerRuntimeReturnsDisabledRuntimeWhenScannerIsOff(t *testing.T) {
	runtime := ProvideScannerRuntime(&config.Config{}, nil, nil, nil, nil, nil, nil, nil, nil)

	require.Equal(t, ScannerRuntimeStateDisabled, runtime.Status().State)
}

func TestProvideScannerRuntimeRejectsCreditingDuringObserverStage(t *testing.T) {
	cfg := &config.Config{Web3Deposit: config.Web3DepositConfig{
		Enabled:        true,
		ScannerEnabled: true,
		CreditEnabled:  true,
	}}

	runtime := ProvideScannerRuntime(cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	status := runtime.Status()
	require.Equal(t, ScannerRuntimeStateUnhealthy, status.State)
	require.True(t, strings.Contains(status.LastError, ErrScannerCreditMustBeDisabled.Error()))
}

func TestScannerLeaseTimingKeepsThreePollIntervals(t *testing.T) {
	leaseTTL, renewInterval := scannerLeaseTiming(10 * time.Second)
	require.Equal(t, DefaultScannerLeaseTTL, leaseTTL)
	require.Equal(t, DefaultScannerLeaseRenewInterval, renewInterval)

	leaseTTL, renewInterval = scannerLeaseTiming(30 * time.Second)
	require.Equal(t, 90*time.Second, leaseTTL)
	require.Equal(t, 30*time.Second, renewInterval)
}
