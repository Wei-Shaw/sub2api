package service

import (
	"fmt"
	"sort"
	"strings"
)

// deepseekToolsEndpoints 列出 DeepSeek 独立工具接口支持的 endpoint 白名单。
// 与上游约定：search/fetch/ocr/asr/tts 挂在 /v1/tools/ 下，files 系列挂在
// /v1/files 下。
var deepseekToolsEndpoints = map[string]bool{
	"search":       true,
	"fetch":        true,
	"ocr":          true,
	"asr":          true,
	"tts":          true,
	"files_create": true,
	"files_list":   true,
	"files_get":    true,
	"files_delete": true,
}

// deepseekDefaultToolsBaseURL 返回 provider profile 里 DeepSeek 的默认 base_url，
// 取不到时兜底官方常量。
func deepseekDefaultToolsBaseURL() string {
	profile := LookupProviderProfile(PlatformDeepseek)
	if profile == nil {
		return DefaultDeepseekBaseURL
	}
	modes := make([]string, 0, len(profile.Modes))
	for mode := range profile.Modes {
		modes = append(modes, mode)
	}
	sort.Strings(modes)
	for _, mode := range modes {
		if base := strings.TrimSpace(profile.Modes[mode].BaseURLs[APIProtocolChatCompletions]); base != "" {
			return base
		}
	}
	return DefaultDeepseekBaseURL
}

// normalizeDeepseekBaseURL 去掉尾斜杠与已带的 /v1 后缀，统一交由下方拼接补回
// /v1，避免 base_url 含 /v1 时拼出 /v1/v1。
func normalizeDeepseekBaseURL(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	return strings.TrimSuffix(base, "/v1")
}

// isValidDeepseekFileID 校验 fileID 是否落在闭集 [A-Za-z0-9_-]，防路径注入。
func isValidDeepseekFileID(fileID string) bool {
	if fileID == "" {
		return false
	}
	for i := 0; i < len(fileID); i++ {
		c := fileID[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// buildDeepseekToolsURL 构造 DeepSeek 独立工具接口的上游 URL。
// base_url 取自账号 credentials，为空时回退 provider profile 默认值；
// endpoint 必须在白名单内，fileID 仅在 files_get/files_delete 时必填且需通过
// 字符白名单校验。
func buildDeepseekToolsURL(account *Account, endpoint string, fileID string) (string, error) {
	if !deepseekToolsEndpoints[endpoint] {
		return "", fmt.Errorf("unsupported deepseek tools endpoint: %s", endpoint)
	}
	base := ""
	if account != nil {
		base = account.GetCredential("base_url")
	}
	if strings.TrimSpace(base) == "" {
		base = deepseekDefaultToolsBaseURL()
	}
	base = normalizeDeepseekBaseURL(base)
	if base == "" {
		return "", fmt.Errorf("deepseek base URL is required")
	}

	switch endpoint {
	case "files_create", "files_list":
		return base + "/v1/files", nil
	case "files_get", "files_delete":
		fileID = strings.TrimSpace(fileID)
		if !isValidDeepseekFileID(fileID) {
			// 不回显原始输入，避免把它写进日志与错误响应。
			return "", fmt.Errorf("invalid file id for deepseek tools url path")
		}
		return base + "/v1/files/" + fileID, nil
	default:
		return base + "/v1/tools/" + endpoint, nil
	}
}
