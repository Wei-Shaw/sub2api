package service

import (
	"crypto/sha256"
	"fmt"
	"net"
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
	if account == nil {
		return "", false
	}
	explicit := strings.TrimSpace(account.GetCredential("base_url"))
	bases, _ := account.Credentials["api_base_urls"].(map[string]any)
	if account.IsAdaptiveAPIProtocol() {
		if anthropic {
			if value, _ := bases[APIProtocolAnthropic].(string); strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value), false
			}
		}
		if value, _ := bases[APIProtocolChatCompletions].(string); strings.TrimSpace(value) != "" {
			explicit = strings.TrimSpace(value)
		}
	}
	if explicit != "" {
		return explicit, anthropic && !account.IsAnthropicProtocol()
	}
	// An implicit profile default is safe only when the credentials contain
	// no third-party origin. Sparse adaptive profiles are valid API inputs.
	for _, value := range bases {
		if configured, ok := value.(string); ok && strings.TrimSpace(configured) != "" && !isDeepseekOfficialHost(configured) {
			return "", false
		}
	}
	if account.IsAnthropicProtocol() {
		return account.GetAnthropicProtocolBaseURL(), false
	}
	return account.GetOpenAIBaseURL(), anthropic
}

// Compare the storage namespace, including a relay's path prefix and configured
// routing query, without storing credentials embedded in that query.
func deepseekStorageOrigin(u *url.URL) string {
	path := strings.TrimRight(u.EscapedPath(), "/")
	if i := strings.LastIndex(path, "/files/"); i >= 0 && !strings.Contains(path[i+7:], "/") {
		path = path[:i]
	} else {
		for _, suffix := range []string{"/chat/completions", "/responses", "/messages", "/files"} {
			if strings.HasSuffix(path, suffix) {
				path = strings.TrimSuffix(path, suffix)
				break
			}
		}
	}
	path = strings.TrimSuffix(strings.TrimSuffix(path, "/v1"), "/anthropic")
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	defaultPort := (port == "443" && u.Scheme == "https") || (port == "80" && u.Scheme == "http")
	if port != "" && !defaultPort {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	origin := strings.ToLower(u.Scheme) + "://" + host + path
	if u.RawQuery != "" {
		origin += fmt.Sprintf("?sha256=%x", sha256.Sum256([]byte(u.RawQuery)))
	}
	return origin
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
	if base == "" {
		return nil, fmt.Errorf("no unambiguous configured DeepSeek tools origin")
	}
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
