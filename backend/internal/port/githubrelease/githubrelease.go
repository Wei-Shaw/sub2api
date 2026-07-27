// Package githubrelease contains the port interface for the GitHub release
// bounded context: read-only access to upstream release metadata and asset
// downloads used by the self-update flow. The contract references only a
// domain DTO so the repository layer can implement it without importing
// internal/service. The service package keeps a type alias to the interface
// so existing call sites and test stubs continue to satisfy the contract.
package githubrelease

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// GitHubReleaseClient 获取 GitHub release 信息的接口。
type GitHubReleaseClient interface {
	FetchLatestRelease(ctx context.Context, repo string) (*domain.GitHubRelease, error)
	FetchRecentReleases(ctx context.Context, repo string, perPage int) ([]*domain.GitHubRelease, error)
	DownloadFile(ctx context.Context, url, dest string, maxSize int64) error
	FetchChecksumFile(ctx context.Context, url string) ([]byte, error)
}
