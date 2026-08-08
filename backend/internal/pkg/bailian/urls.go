package bailian

import (
	"net/url"
	"strings"
)

// DefaultBaseURL is the public DashScope endpoint (Beijing region). Accounts
// may override it with a workspace-scoped domain such as
// https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com.
const DefaultBaseURL = "https://dashscope.aliyuncs.com"

const (
	// VideoSynthesisPath creates an async video generation task
	// (requires the X-DashScope-Async: enable header).
	VideoSynthesisPath = "/api/v1/services/aigc/video-generation/video-synthesis"
	taskPathPrefix     = "/api/v1/tasks/"
	// CompatibleChatCompletionsPath is the OpenAI-compatible text endpoint.
	CompatibleChatCompletionsPath = "/compatible-mode/v1/chat/completions"
)

// NormalizeAPIRoot strips trailing slashes and API prefixes that users
// commonly paste along with the host (/compatible-mode[/v1], /api/v1), so a
// single stored base_url can serve both the native and compatible endpoints.
func NormalizeAPIRoot(base string) string {
	root := strings.TrimRight(strings.TrimSpace(base), "/")
	for _, suffix := range []string{"/compatible-mode/v1", "/compatible-mode", "/api/v1"} {
		if strings.HasSuffix(root, suffix) {
			root = strings.TrimRight(strings.TrimSuffix(root, suffix), "/")
			break
		}
	}
	if root == "" {
		root = DefaultBaseURL
	}
	return root
}

func VideoSynthesisURL(root string) string {
	return NormalizeAPIRoot(root) + VideoSynthesisPath
}

func TaskURL(root, taskID string) string {
	return NormalizeAPIRoot(root) + taskPathPrefix + url.PathEscape(taskID)
}

func ChatCompletionsURL(root string) string {
	return NormalizeAPIRoot(root) + CompatibleChatCompletionsPath
}
