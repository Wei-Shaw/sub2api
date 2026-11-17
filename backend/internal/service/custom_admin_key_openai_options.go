package service

import (
	"encoding/json"
	"slices"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 只接受对接客户端实际需要的字段，不能写入额度快照、任意 extra 或影子关系。
func validateScopedOpenAIOptions(raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	invalid := infraerrors.BadRequest("SCOPED_ACCOUNT_OPTIONS", "身份与连接选项格式无效或包含不支持的字段")
	if json.Unmarshal(raw, &fields) != nil || len(fields) == 0 {
		return invalid
	}
	for key, value := range fields {
		var text string
		var flag bool
		switch key {
		case "email", "privacy_mode":
			// privacy_mode 仅兼容旧客户端入参，创建前会移除，服务端仍独立核验。
			if string(value) == "null" || json.Unmarshal(value, &text) != nil || len(text) > 512 {
				return invalid
			}
		case "openai_oauth_responses_websockets_v2_mode":
			if json.Unmarshal(value, &text) != nil || !slices.Contains([]string{"off", "shared", "dedicated", "ctx_pool", "passthrough", "http_bridge"}, text) {
				return invalid
			}

		case "openai_oauth_responses_websockets_v2_enabled", "openai_long_context_billing_enabled":
			if string(value) == "null" || json.Unmarshal(value, &flag) != nil {
				return invalid
			}
		default:
			return invalid
		}
	}
	return nil
}
