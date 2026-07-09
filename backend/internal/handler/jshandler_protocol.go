package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// inferJSToFormat maps inbound API format to upstream protocol family for CPA-style scripts.
func inferJSToFormat(c *gin.Context, sourceFormat string, account *service.Account) string {
	if account == nil {
		return ""
	}
	platform := string(account.Platform)
	switch sourceFormat {
	case "anthropic_messages":
		return platform
	case "openai_chat":
		if account.Platform != service.PlatformOpenAI {
			return "openai_chat"
		}
		if v, ok := c.Get("openai_js_protocol"); ok {
			if s, ok := v.(string); ok && s == "openai_chat" {
				if account.Type == service.AccountTypeOAuth {
					return "codex"
				}
			}
		}
		if body := openAIForwardBodyFromContext(c); len(body) > 0 {
			if gjson.GetBytes(body, "input").Exists() && !gjson.GetBytes(body, "messages").Exists() {
				return "codex"
			}
		}
		if account.Type == service.AccountTypeOAuth {
			return "codex"
		}
		return "openai"
	case "openai_responses":
		if account.Platform != service.PlatformOpenAI {
			return "openai_responses"
		}
		if account.Type == service.AccountTypeOAuth {
			return "codex"
		}
		if body := openAIForwardBodyFromContext(c); len(body) > 0 {
			if gjson.GetBytes(body, "input").Exists() && !gjson.GetBytes(body, "messages").Exists() {
				return "codex"
			}
		}
		return "openai"
	default:
		return platform
	}
}

func openAIForwardBodyFromContext(c *gin.Context) []byte {
	if c == nil {
		return nil
	}
	if v, ok := c.Get("openai_forward_body"); ok {
		if b, ok := v.([]byte); ok {
			return b
		}
	}
	return nil
}