package service

import (
	"fmt"
	"net/url"
	"strings"
)

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

// Select the same configured origin as chat traffic for the requested
// protocol. An implicit adaptive Anthropic profile default could point to a
// different host than the account's custom Chat Completions endpoint.
func deepseekToolsBase(account *Account, anthropic bool) (base string, deriveAnthropic bool) {
	if account.IsAnthropicProtocol() {
		return account.GetAnthropicProtocolBaseURL(), false
	}
	if anthropic && account.IsAdaptiveAPIProtocol() {
		bases, _ := account.Credentials["api_base_urls"].(map[string]any)
		if explicit, _ := bases[APIProtocolAnthropic].(string); strings.TrimSpace(explicit) != "" {
			return account.GetAnthropicProtocolBaseURL(), false
		}
	}
	return account.GetOpenAIBaseURL(), anthropic
}

func buildDeepseekToolsURL(account *Account, endpoint, fileID string, anthropic bool, validate func(string) (string, error)) (*url.URL, error) {
	var path string
	switch endpoint {
	case "files_create":
		path = "/v1/files"
	case "files_get", "files_delete":
		if !isValidDeepseekFileID(fileID) {
			return nil, fmt.Errorf("invalid file id for deepseek tools url path")
		}
		path = "/v1/files/" + fileID
	case "search", "fetch", "ocr", "asr", "tts":
		if anthropic {
			return nil, fmt.Errorf("endpoint %s does not support Anthropic files", endpoint)
		}
		path = "/v1/tools/" + endpoint
	default:
		return nil, fmt.Errorf("unsupported deepseek tools endpoint: %s", endpoint)
	}
	base, deriveAnthropic := deepseekToolsBase(account, anthropic)
	validated, err := validate(base)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(validated)
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/v1")
	if !anthropic && account.IsAnthropicProtocol() {
		prefix = strings.TrimSuffix(prefix, "/anthropic")
	}
	if deriveAnthropic && !strings.HasSuffix(prefix, "/anthropic") {
		prefix += "/anthropic"
	}
	u.Path = prefix + path
	return u, nil
}
