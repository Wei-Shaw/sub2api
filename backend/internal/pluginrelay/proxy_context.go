package pluginrelay

import (
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

const ProxyIDHeader = "X-Sub2api-Proxy-Id"

func SetProxyID(header http.Header, proxyID *int64) {
	if header == nil {
		return
	}
	header.Del(ProxyIDHeader)
	if proxyID != nil && *proxyID > 0 {
		header.Set(ProxyIDHeader, strconv.FormatInt(*proxyID, 10))
	}
}

func StripProxyID(header http.Header) {
	if header != nil {
		header.Del(ProxyIDHeader)
	}
}

func IsAIRelayURL(target *url.URL) bool {
	if target == nil {
		return false
	}
	cleaned := path.Clean("/" + strings.TrimSpace(target.EscapedPath()))
	return strings.HasPrefix(cleaned, "/plugins/ai-relay/")
}

func PrepareUpstreamRequest(req *http.Request, proxyURL string) string {
	if req != nil && IsAIRelayURL(req.URL) {
		return ""
	}
	if req != nil {
		StripProxyID(req.Header)
	}
	return proxyURL
}
