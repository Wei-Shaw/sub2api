package service

import (
	"net/http"
	"strings"
)

const openAIRequestHeaderPassthroughExtraKey = "openai_request_header_passthrough"

func (a *Account) IsOpenAIRequestHeaderPassthroughEnabled() bool {
	if a == nil || a.Platform != PlatformOpenAI || a.Extra == nil {
		return false
	}
	enabled, ok := a.Extra[openAIRequestHeaderPassthroughExtraKey].(bool)
	return ok && enabled
}

var openAIRequestHeaderPassthroughBlockedHeaders = map[string]struct{}{
	"authorization": {}, "connection": {}, "content-length": {}, "cookie": {}, "host": {},
	"keep-alive": {}, "proxy-authenticate": {}, "proxy-authorization": {}, "proxy-connection": {},
	"sec-websocket-accept": {}, "sec-websocket-extensions": {}, "sec-websocket-key": {},
	"sec-websocket-protocol": {}, "sec-websocket-version": {}, "te": {}, "trailer": {},
	"transfer-encoding": {}, "upgrade": {}, "x-api-key": {}, "x-goog-api-key": {},
}

func copyOpenAIInboundHeaders(dst, src http.Header, passthrough bool, allowTimeoutHeaders bool) {
	if dst == nil || src == nil {
		return
	}
	for name, values := range src {
		lowerName := strings.ToLower(strings.TrimSpace(name))
		if passthrough {
			if _, blocked := openAIRequestHeaderPassthroughBlockedHeaders[lowerName]; blocked {
				continue
			}
		} else if !isOpenAIPassthroughAllowedRequestHeader(lowerName, allowTimeoutHeaders) {
			continue
		}
		for _, value := range values {
			dst.Add(name, value)
		}
	}
}
