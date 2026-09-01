package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// openAILadderCatalogJSON 镜像真实同步目录的形态：长上下文用 above_272k 绝对价字段表达，
// 由解析层折算成阈值+倍率。静态 Go 兜底价不再携带阶梯，阶梯计费一律走目录数据。
const openAILadderCatalogJSON = `{
	"gpt-5.4": {"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 2.5e-06, "output_cost_per_token": 1.5e-05,
		"cache_read_input_token_cost": 2.5e-07, "cache_creation_input_token_cost": 2.5e-06,
		"input_cost_per_token_above_272k_tokens": 5e-06,
		"output_cost_per_token_above_272k_tokens": 2.25e-05,
		"cache_read_input_token_cost_above_272k_tokens": 5e-07},
	"gpt-5.5-pro": {"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 3e-05, "output_cost_per_token": 1.8e-04,
		"input_cost_per_token_above_272k_tokens": 6e-05,
		"output_cost_per_token_above_272k_tokens": 2.7e-04}
}`

// newStubPricingServiceFromJSON 用与生产一致的解析路径（含 above_XXXk 阶梯折算）
// 从原始目录 JSON 构造目录 stub。无 build tag：带 unit 标签与默认构建的测试文件都会用到。
func newStubPricingServiceFromJSON(t *testing.T, body string) *PricingService {
	t.Helper()
	s := &PricingService{}
	data, err := s.parsePricingData([]byte(body))
	require.NoError(t, err)
	s.pricingData = data
	return s
}
