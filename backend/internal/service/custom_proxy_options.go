package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// ListProxyOptions 为受限管理密钥提供有界的代理候选。
func (s *adminServiceImpl) ListProxyOptions(ctx context.Context, page, pageSize int, search string) ([]Proxy, int64, error) {
	repo, ok := s.proxyRepo.(interface {
		ListOptions(context.Context, pagination.PaginationParams, string) ([]Proxy, *pagination.PaginationResult, error)
	})
	if !ok {
		return nil, 0, fmt.Errorf("代理分页查询不可用")
	}
	proxies, result, err := repo.ListOptions(ctx, pagination.PaginationParams{Page: page, PageSize: pageSize}, strings.TrimSpace(search))
	if err != nil {
		return nil, 0, err
	}
	return proxies, result.Total, nil
}

// SampleProxyOptions 为不支持分页的受限客户端提供全池随机候选。
func (s *adminServiceImpl) SampleProxyOptions(ctx context.Context, search string) ([]Proxy, int64, error) {
	repo, ok := s.proxyRepo.(interface {
		SampleOptions(context.Context, string) ([]Proxy, int64, error)
	})
	if !ok {
		return nil, 0, fmt.Errorf("随机代理候选查询不可用")
	}
	return repo.SampleOptions(ctx, strings.TrimSpace(search))
}
