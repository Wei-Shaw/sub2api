package kiro

import "fmt"

// defaultRegion 是账号未指定 region 时的默认值。
const defaultRegion = "us-east-1"

// Endpoint 是一个可用的上游转发目标。
type Endpoint struct {
	// URL 是完整请求地址。
	URL string
	// Origin 填进 userInputMessage.origin。
	Origin string
	// AmzTarget 为空表示不发送 x-amz-target 头。
	AmzTarget string
	// Name 用于日志与监控。
	Name string
}

// EndpointsFor 返回按优先级排序的端点列表。
//
// OAuth 账号有三个可回退端点（429 时逐个尝试）；API Key 账号只有 CLI runtime
// 一条路径，且不使用 profileArn —— 调用方需据此清空 Options.ProfileArn。
func EndpointsFor(isAPIKey bool, region string) []Endpoint {
	if region == "" {
		region = defaultRegion
	}

	if isAPIKey {
		return []Endpoint{{
			URL:       fmt.Sprintf("https://runtime.%s.kiro.dev/", region),
			Origin:    "KIRO_CLI",
			AmzTarget: "AmazonCodeWhispererStreamingService.GenerateAssistantResponse",
			Name:      "Kiro CLI",
		}}
	}

	qHost := fmt.Sprintf("https://q.%s.amazonaws.com", region)
	cwHost := fmt.Sprintf("https://codewhisperer.%s.amazonaws.com", region)

	return []Endpoint{
		{
			URL:    qHost + "/generateAssistantResponse",
			Origin: "AI_EDITOR",
			Name:   "Kiro IDE",
		},
		{
			URL:       cwHost + "/generateAssistantResponse",
			Origin:    "AI_EDITOR",
			AmzTarget: "AmazonCodeWhispererStreamingService.GenerateAssistantResponse",
			Name:      "CodeWhisperer",
		},
		{
			URL:       qHost + "/generateAssistantResponse",
			Origin:    "AI_EDITOR",
			AmzTarget: "AmazonQDeveloperStreamingService.SendMessage",
			Name:      "AmazonQ",
		},
	}
}
