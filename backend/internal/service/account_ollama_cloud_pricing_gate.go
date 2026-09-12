package service

import (
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// OllamaCloudAllowedModelsExtraKey 是 ollama_cloud 账号在 model_mapping 为空时
// 声明可出站模型清单的 extra 键。值为字符串数组（如 ["gpt-oss:120b","qwen3.5:397b"]）。
//
// 背景：account.IsModelSupported 对空 model_mapping 一律放行所有模型名，因此
// 只扫 mapping 目标值的校验对空 mapping 账号零覆盖；ollama_cloud 的计费又是
// 白名单语义（未列价的模型名显式拒绝计费），所以保存期必须拿到一个有限的
// 可出站模型名集合来断言「全部可被 GetModelPricing 解析」，否则只能全量拒绝。
const OllamaCloudAllowedModelsExtraKey = "allowed_models"

// accountModelPricingLookup 是保存期无价门禁所需的最小定价查询面，
// *BillingService 以同一查价链（LiteLLM → fallback，含 :tag 两级 fallback）实现。
type accountModelPricingLookup interface {
	GetModelPricing(model string) (*ModelPricing, error)
}

// validateOllamaCloudAccountModelPricingGate 是 B2-③ 的保存期无价门禁：
// ollama_cloud 账号保存时断言「该账号可出站的模型名集合均可被 GetModelPricing 解析」。
//
// 可出站集合的取法与 account.IsModelSupported 的放行语义对齐：
//   - 非空 model_mapping → 扫全部目标值（映射后真正发往上游的名字）；
//   - 空 model_mapping → 必须同时提供 OllamaCloudAllowedModelsExtraKey 显式清单，
//     清单全量纳入断言；空 mapping 且无清单一律拒绝（此时账号对全部模型名放行，
//     门禁对它零覆盖，是静默零计费的敞口）。
//
// 只对 ollama_cloud 生效：其它平台的保存路径行为不变（多数平台定价缺失走
// 请求期零成本兜底，属于既有的全平台 fail-open 设计，见
// openai_gateway_usage.go 的 pricing_missing_record_zero_cost 分支）。
// lookup 为 nil 时跳过（生产 wire 必然注入 *BillingService；仅直接构造
// struct 的单元测试/内部调用会出现）。
func validateOllamaCloudAccountModelPricingGate(lookup accountModelPricingLookup, account *Account) error {
	if lookup == nil || account == nil || !account.IsOllamaCloud() {
		return nil
	}

	models := ollamaCloudOutboundModelNames(account)
	if len(models) == 0 {
		return infraerrors.BadRequest("MODEL_PRICING_MISSING",
			fmt.Sprintf("ollama_cloud 账号 model_mapping 为空时必须在 extra.%s 提供显式的可出站模型清单，否则无法保证全部模型可计费",
				OllamaCloudAllowedModelsExtraKey))
	}

	unpriced := make([]string, 0)
	for _, model := range models {
		if _, err := lookup.GetModelPricing(model); err != nil {
			unpriced = append(unpriced, model)
		}
	}
	if len(unpriced) > 0 {
		return infraerrors.BadRequest("MODEL_PRICING_MISSING",
			fmt.Sprintf("以下 ollama_cloud 模型缺少定价，无法保证计费（请先补价或调整 model_mapping/%s）: %s",
				OllamaCloudAllowedModelsExtraKey, strings.Join(unpriced, ", ")))
	}
	return nil
}

// ollamaCloudOutboundModelNames 收集 ollama_cloud 账号的可出站模型名集合
// （去重、去空）。空 mapping 且无有效清单时返回 nil。
//
// 清单值兼容两种形态：JSON 反序列化出的 []any（元素为 string），以及程序化
// 构造的 []string。非字符串元素跳过（由清单「缺失即拒绝」的语义兜底不严的
// 情况——含非字符串元素的清单视同未提供，见下方 return nil）。
func ollamaCloudOutboundModelNames(account *Account) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	appendModel := func(model string) {
		trimmed := strings.TrimSpace(model)
		if trimmed == "" {
			return
		}
		if _, dup := seen[trimmed]; dup {
			return
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}

	if mapping := account.GetModelMapping(); len(mapping) > 0 {
		for _, target := range mapping {
			appendModel(target)
		}
		return out
	}

	switch list := account.Extra[OllamaCloudAllowedModelsExtraKey].(type) {
	case []any:
		for _, item := range list {
			model, ok := item.(string)
			if !ok {
				return nil
			}
			appendModel(model)
		}
	case []string:
		for _, model := range list {
			appendModel(model)
		}
	default:
		return nil
	}
	return out
}
