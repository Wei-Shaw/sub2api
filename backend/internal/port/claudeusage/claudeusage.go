// Package claudeusage contains the port interface for fetching Claude OAuth
// usage data from the Anthropic API. Moving this contract out of
// internal/service breaks the repository→service inversion: the repository
// layer can implement ClaudeUsageFetcher without importing internal/service.
//
// The concrete implementation (NewClaudeUsageFetcher) lives in
// internal/repository and depends on upstream.HTTPUpstream for the shared
// transport; this interface contract only depends on domain DTOs.
package claudeusage

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// ClaudeUsageFetcher fetches usage data from Anthropic OAuth API
type ClaudeUsageFetcher interface {
	FetchUsage(ctx context.Context, accessToken, proxyURL string) (*domain.ClaudeUsageResponse, error)
	// FetchUsageWithOptions 使用完整选项获取用量数据，支持 TLS 指纹和自定义 User-Agent
	FetchUsageWithOptions(ctx context.Context, opts *domain.ClaudeUsageFetchOptions) (*domain.ClaudeUsageResponse, error)
}
