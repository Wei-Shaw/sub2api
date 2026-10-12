package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// Marks provider-owned responses so middleware can preserve their contract.
const AnthropicPassthroughResponseContextKey = "anthropic_passthrough_upstream_response"

// copyAnthropicEndToEndHeaders preserves extension headers while removing
// connection-specific fields and credentials in both directions. Authentication
// is replaced separately on requests.
func copyAnthropicEndToEndHeaders(dst, src http.Header, request bool) {
	blocked := map[string]bool{
		"connection": true, "proxy-connection": true, "keep-alive": true,
		"proxy-authenticate": true, "proxy-authorization": true,
		"te": true, "trailer": true, "transfer-encoding": true,
		"upgrade": true, "content-length": true, "host": true,
		"authorization": true, "x-api-key": true, "x-goog-api-key": true,
		"cookie": true, "cookie2": true,
	}
	for key, values := range src {
		if strings.EqualFold(key, "Connection") {
			for _, value := range values {
				for _, token := range strings.Split(value, ",") {
					blocked[strings.ToLower(strings.TrimSpace(token))] = true
				}
			}
		}
	}
	if !request {
		blocked["set-cookie"] = true
		blocked["set-cookie2"] = true
	}
	forwarded := make(http.Header)
	for key, values := range src {
		if !blocked[strings.ToLower(key)] {
			canonical := http.CanonicalHeaderKey(key)
			forwarded[canonical] = append(forwarded[canonical], values...)
		}
	}
	for key, values := range forwarded {
		// Local middleware may already have set a differently-cased spelling.
		// HTTP field names are case-insensitive: replace that value, not add a
		// conflicting second request-id or content-type field.
		for existing := range dst {
			if strings.EqualFold(existing, key) {
				delete(dst, existing)
			}
		}
		dst[key] = values
	}
}

// buildAnthropicPassthroughRequest never parses or repairs the business body.
// The configured base URL selects the destination; the inbound query is retained.
func (s *GatewayService) buildAnthropicPassthroughRequest(ctx context.Context, c *gin.Context, account *Account, method, endpoint string, body io.Reader, token string) (*http.Request, error) {
	baseURL := strings.TrimRight(account.GetBaseURL(), "/")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	validated, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	target, err := url.Parse(strings.TrimRight(validated, "/") + "/v1" + endpoint)
	if err != nil {
		return nil, err
	}
	if c != nil && c.Request != nil {
		target.RawQuery = c.Request.URL.RawQuery
		// Gateway authentication in a query must not become upstream credentials.
		query := target.Query()
		if query.Has("api_key") || query.Has("key") {
			query.Del("api_key")
			query.Del("key")
			target.RawQuery = query.Encode()
		}
	}
	req, err := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(ctx), method, target.String(), body)
	if err != nil {
		return nil, err
	}
	if c != nil && c.Request != nil {
		copyAnthropicEndToEndHeaders(req.Header, c.Request.Header, true)
	}
	// Compression negotiation belongs to this HTTP hop. Metering and the SSE
	// observer consume decoded business bytes; do not forward a caller encoding
	// preference that would disable the transport's transparent decoding.
	req.Header.Set("Accept-Encoding", "identity")
	if req.Header.Get("Content-Type") == "" && body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if req.Header.Get("Anthropic-Version") == "" {
		req.Header.Set("Anthropic-Version", "2023-06-01")
	}
	// Strict mode ignores compatibility header overrides, just as it ignores
	// model/body rewrites. The selected account supplies authentication only.
	setAnthropicAPIKeyAuthHeader(req.Header, account, token, account.GetBaseURL())
	return req, nil
}

// scanAnthropicWireLines retains CRLF/LF and an unterminated last line. Parsing
// usage is observational: the exact upstream bytes are written to the client.
func scanAnthropicWireLines(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i+1], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
