package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// SingleAnthropicPassthroughAccount identifies an unambiguous upstream catalog
// and Files namespace. A pool is not one upstream namespace: keep its existing
// aggregate Models behavior rather than silently choosing a random account.
func (s *GatewayService) SingleAnthropicPassthroughAccount(ctx context.Context, groupID *int64) (*Account, error) {
	if groupID == nil || s == nil || s.accountRepo == nil {
		return nil, nil
	}
	// Count configured accounts, including unavailable ones. A temporary
	// cooldown must not make a pool look like a single Files namespace.
	accounts, err := s.accountRepo.ListAllWithFilters(ctx, "", "", "", "", *groupID, "")
	if err != nil {
		return nil, err
	}
	if len(accounts) != 1 || !accounts[0].IsAnthropicAPIKeyPassthroughEnabled() {
		return nil, nil
	}
	if !accounts[0].IsSchedulable() {
		return nil, fmt.Errorf("native upstream unavailable")
	}
	return &accounts[0], nil
}

// ForwardAnthropicAuxiliary retains Models metadata, pagination and Files
// multipart/download bytes. It shares the Messages destination and auth rules.
func (s *GatewayService) ForwardAnthropicAuxiliary(ctx context.Context, c *gin.Context, account *Account, endpoint string) error {
	supportedEndpoint := endpoint == "/models" || strings.HasPrefix(endpoint, "/models/") || endpoint == "/files" || strings.HasPrefix(endpoint, "/files/")
	if account == nil || !account.IsAnthropicAPIKeyPassthroughEnabled() ||
		!supportedEndpoint {
		return fmt.Errorf("unsupported Anthropic passthrough endpoint")
	}

	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil || tokenType != "apikey" {
		return fmt.Errorf("upstream API key unavailable")
	}
	var body io.Reader
	if c.Request.Body != nil && c.Request.Body != http.NoBody {
		body = c.Request.Body
	}
	req, err := s.buildAnthropicPassthroughRequest(ctx, c, account, c.Request.Method, endpoint, body, token)
	if err != nil {
		return err
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return err
	}
	if resp == nil || resp.Body == nil {
		return fmt.Errorf("empty upstream response")
	}
	defer func() { _ = resp.Body.Close() }()
	c.Set(AnthropicPassthroughResponseContextKey, true)
	writeAnthropicPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Status(resp.StatusCode)
	c.Writer.WriteHeaderNow()
	_, err = io.Copy(c.Writer, resp.Body)
	return err
}
