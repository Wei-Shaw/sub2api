package service

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	openAIAttestationHeader              = "X-OAI-Attestation"
	openAIAttestationForwardedContextKey = "openai_attestation_forwarded"
)

// ErrOpenAIAttestationRedirect 表示携带证明的请求试图跟随重定向。
var ErrOpenAIAttestationRedirect = errors.New("openai attestation redirect blocked")

// copyOpenAIAttestationHeader 仅向 OpenAI OAuth 上游复制客户端证明头。
// 证明内容由官方客户端生成，这里不解析、不改写，也不验证其内部结构。
func copyOpenAIAttestationHeader(dst, src http.Header, account *Account) bool {
	if dst == nil {
		return false
	}

	// 先清理目标头的所有大小写形式，避免调用方此前的通用复制越过账号边界。
	deleteHeaderAllForms(dst, openAIAttestationHeader)
	for key := range dst {
		if strings.EqualFold(key, openAIAttestationHeader) {
			delete(dst, key)
		}
	}
	value, ok := openAIAttestationHeaderValue(src, account)
	if !ok {
		return false
	}

	// 直接赋值可保持 opaque 值逐字不变，避免 Get/Trim 等读取方式改动内容。
	dst[http.CanonicalHeaderKey(openAIAttestationHeader)] = []string{value}
	return true
}

func openAIAttestationHeaderValue(src http.Header, account *Account) (string, bool) {
	if src == nil || account == nil || !account.IsOpenAIOAuth() {
		return "", false
	}

	var value string
	valueCount := 0
	for key, values := range src {
		if !strings.EqualFold(key, openAIAttestationHeader) {
			continue
		}
		for _, candidate := range values {
			valueCount++
			if valueCount > 1 {
				return "", false
			}
			value = candidate
		}
	}
	if valueCount != 1 || value == "" {
		return "", false
	}
	return value, true
}

func isOpenAIAttestationResponsesPath(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	path := strings.TrimRight(strings.TrimSpace(c.Request.URL.Path), "/")
	responsesIndex := strings.LastIndex(path, "/responses")
	if responsesIndex < 0 {
		return false
	}
	suffix := path[responsesIndex+len("/responses"):]
	return suffix == "" || suffix == "/compact"
}

func hasOpenAIAttestationHeader(headers http.Header) bool {
	for key, values := range headers {
		if strings.EqualFold(key, openAIAttestationHeader) && len(values) > 0 {
			return true
		}
	}
	return false
}

// CheckOpenAIAttestationRedirect 禁止携带证明的 HTTP 或 WebSocket 握手跟随重定向。
// 上游端点不应依赖重定向；失败关闭可避免 opaque 证明被复制到其它目标。
func CheckOpenAIAttestationRedirect(req *http.Request, via []*http.Request) error {
	if req != nil && hasOpenAIAttestationHeader(req.Header) {
		return ErrOpenAIAttestationRedirect
	}
	for _, previous := range via {
		if previous != nil && hasOpenAIAttestationHeader(previous.Header) {
			return ErrOpenAIAttestationRedirect
		}
	}
	return nil
}

func markOpenAIAttestationForwarded(c *gin.Context) {
	if c != nil {
		c.Set(openAIAttestationForwardedContextKey, true)
	}
}

// IsOpenAIAttestationForwarded 报告当前逻辑请求是否已向 OpenAI OAuth 上游发送证明。
func IsOpenAIAttestationForwarded(c *gin.Context) bool {
	if c == nil {
		return false
	}
	forwarded, ok := c.Get(openAIAttestationForwardedContextKey)
	value, isBool := forwarded.(bool)
	return ok && isBool && value
}
