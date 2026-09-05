package service

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

const openCodeSessionHeader = "X-OpenCode-Session"

// applyOpenCodeSessionHeader forwards the caller-owned conversation identifier
// only to OpenCode's official API origin. The caller applies this after account
// header overrides so a per-conversation value cannot be replaced by a fixed
// account-wide override.
func applyOpenCodeSessionHeader(c *gin.Context, account *Account, targetURL string, headers http.Header) {
	if c == nil || c.Request == nil || account == nil || account.Type != AccountTypeAPIKey || headers == nil {
		return
	}

	parsed, err := url.Parse(targetURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || !strings.EqualFold(parsed.Hostname(), "opencode.ai") {
		return
	}

	sessionID := strings.TrimSpace(c.GetHeader(openCodeSessionHeader))
	if sessionID == "" {
		return
	}
	for key := range headers {
		if strings.EqualFold(key, openCodeSessionHeader) {
			delete(headers, key)
		}
	}
	headers.Set(openCodeSessionHeader, sessionID)
}
