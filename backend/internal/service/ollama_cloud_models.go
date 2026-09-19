package service

// DefaultOllamaCloudModelIDs 是 Ollama Cloud 的默认模型目录：实测
// GET https://ollama.com/v1/models（2026-09-12，HTTP 200，20 个模型，
// owned_by 全部为 ollama，含两个带 tag 的快照名）。供分组默认模型列表、
// composite 候选与 /v1/models 末尾兜底使用；条目均可被计费价目命中
// （见 ollama_cloud_pricing_test.go 的逐名钉测）。
//
// 前端同源静态表：frontend/src/composables/useModelWhitelist.ts 的
// getModelsByPlatform 'ollama_cloud' case 必须与这里保持同一集合，
// 两侧各有测试钉住（ollama_cloud_models_test.go / useModelWhitelist.spec.ts），
// 改动任一侧须同步另一侧。
func DefaultOllamaCloudModelIDs() []string {
	return []string{
		"nemotron-3-super",
		"glm-5.3",
		"gpt-oss:120b",
		"glm-5.3-flash",
		"kimi-k2.6",
		"kimi-k3",
		"deepseek-v4.1-flash",
		"minimax-m2.7",
		"mistral-large-3:675b",
		"glm-5.1",
		"glm-5.2",
		"gpt-oss:20b",
		"qwen3.5:397b",
		"kimi-k2.7-code",
		"nemotron-3-nano:30b",
		"minimax-m3",
		"gemma4:31b",
		"nemotron-3-ultra",
		"deepseek-v4-flash:0731",
		"deepseek-v4-pro:0813",
	}
}
