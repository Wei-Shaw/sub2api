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

func TestProvideScannerRuntimeAllowsCreditingStageToReachDependencyValidation(t *testing.T) {
	cfg := &config.Config{Web3Deposit: config.Web3DepositConfig{
		Enabled:        true,
		ScannerEnabled: true,
		CreditEnabled:  true,
	}}

	runtime := ProvideScannerRuntime(cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	status := runtime.Status()
	require.Equal(t, ScannerRuntimeStateUnhealthy, status.State)
	require.True(t, strings.Contains(status.LastError, "network runtime is unavailable"))
}

func TestScannerRuntimeRegistryKeepsIndependentEntries(t *testing.T) {
	readyPool := func() *ConfluxRPCPool {
		return &ConfluxRPCPool{
			endpoints: []*confluxRPCEndpoint{{client: successfulConfluxRPCCaller("0x1")}},
			now:       time.Now,
		}
	}
	networks := &ConfluxNetworkRuntimeRegistry{enabled: true, runtimes: map[RuntimeKey]*ConfluxNetworkRuntime{
		{NetworkKey: "network_a", AssetKey: "asset_a"}: {enabled: true, ready: true, pool: readyPool()},
		{NetworkKey: "network_b", AssetKey: "asset_b"}: {enabled: true, ready: true, pool: readyPool()},
	}}
	registry := &ScannerRuntimeRegistry{enabled: true, networks: networks, runtimes: map[RuntimeKey]*ScannerRuntime{
		{NetworkKey: "network_a", AssetKey: "asset_a"}: {status: ScannerRuntimeStatus{State: ScannerRuntimeStateStandby}},
		{NetworkKey: "network_b", AssetKey: "asset_b"}: {status: ScannerRuntimeStatus{State: ScannerRuntimeStateUnhealthy}},
	}}

	require.True(t, registry.AssetReady("network_a", "asset_a"))
	require.False(t, registry.AssetReady("network_b", "asset_b"))
}

func TestScannerLeaseTimingKeepsThreePollIntervals(t *testing.T) {
	leaseTTL, renewInterval := scannerLeaseTiming(10 * time.Second)
	require.Equal(t, DefaultScannerLeaseTTL, leaseTTL)
	require.Equal(t, DefaultScannerLeaseRenewInterval, renewInterval)

	leaseTTL, renewInterval = scannerLeaseTiming(30 * time.Second)
	require.Equal(t, 90*time.Second, leaseTTL)
	require.Equal(t, 30*time.Second, renewInterval)
}
