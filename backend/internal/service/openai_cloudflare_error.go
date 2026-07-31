package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/util/httputil"
)

var openAICloudflareNotFoundBody = []byte(`{"error":{"type":"not_found_error","message":"Not found"}}`)

func isOpenAICloudflareForbiddenResponse(account *Account, statusCode int, headers http.Header, body []byte) bool {
	return account != nil &&
		account.Platform == PlatformOpenAI &&
		statusCode == http.StatusForbidden &&
		httputil.IsCloudflareGeneratedErrorResponse(statusCode, headers, body)
}

// normalizeOpenAICloudflareForbiddenResponse converts a Cloudflare-generated
// 403 into a local OpenAI-shaped 404. The original response body is replaced so
// Cloudflare HTML is never exposed and later error handlers cannot mistake the
// response for an OpenAI account-permission failure.
func normalizeOpenAICloudflareForbiddenResponse(account *Account, resp *http.Response, body []byte) ([]byte, bool) {
	if resp == nil || !isOpenAICloudflareForbiddenResponse(account, resp.StatusCode, resp.Header, body) {
		return body, false
	}

	normalizedBody := append([]byte(nil), openAICloudflareNotFoundBody...)
	resp.StatusCode = http.StatusNotFound
	resp.Status = fmt.Sprintf("%d %s", http.StatusNotFound, http.StatusText(http.StatusNotFound))
	if resp.Header == nil {
		resp.Header = make(http.Header)
	}
	resp.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Content-Length")
	resp.Header.Del("Transfer-Encoding")
	resp.ContentLength = int64(len(normalizedBody))
	resp.Body = io.NopCloser(bytes.NewReader(normalizedBody))
	return normalizedBody, true
}
