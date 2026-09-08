//go:build unit

package admin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// modelDefaultPricingCatalogJSON 供模型默认价接口用例的目录条目：sonnet-4 不带 1h 缓存写入价。
const modelDefaultPricingCatalogJSON = `{
	"claude-sonnet-4": {"litellm_provider": "anthropic", "mode": "chat",
		"input_cost_per_token": 3e-06, "output_cost_per_token": 1.5e-05,
		"cache_creation_input_token_cost": 3.75e-06, "cache_read_input_token_cost": 3e-07}
}`

// newBillingServiceWithCatalog 通过 pricing.fallback_file 从给定目录 JSON 构造计费服务：
// 不访问网络，测试结束时停止定价服务的后台调度器。
func newBillingServiceWithCatalog(t *testing.T, cfg *config.Config, catalogJSON string) *service.BillingService {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	require.NoError(t, os.WriteFile(path, []byte(catalogJSON), 0o600))
	pricingCfg := &config.Config{}
	pricingCfg.Pricing.DataDir = dir
	pricingCfg.Pricing.FallbackFile = path
	pricingSvc := service.NewPricingService(pricingCfg, nil)
	require.NoError(t, pricingSvc.Initialize())
	t.Cleanup(pricingSvc.Stop)
	return service.NewBillingService(cfg, pricingSvc)
}
