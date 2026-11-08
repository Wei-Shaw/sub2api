package service

import (
	"context"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// NormalizeAccountProxyMode 显式模式与旧 proxy_id/random_proxy 互斥，省略时保留原绑定语义。
func NormalizeAccountProxyMode(mode string, id *int64, random bool) (string, error) {
	if id != nil && *id < 0 {
		return "", infraerrors.BadRequest("PROXY_SELECTION_INVALID", "代理 ID 无效")
	}
	if random {
		if mode != "" && mode != "random" {
			return "", infraerrors.BadRequest("PROXY_SELECTION_CONFLICT", "代理选择参数冲突")
		}
		mode = "random"
	}
	switch mode {
	case "":
		return "", nil
	case "specified":
		if id == nil || *id <= 0 {
			return "", infraerrors.BadRequest("PROXY_SELECTION_INVALID", "指定代理须提供正数 proxy_id")
		}
	case "direct":
		if id != nil && *id != 0 {
			return "", infraerrors.BadRequest("PROXY_SELECTION_CONFLICT", "直连模式不能同时指定代理")
		}
	case "random", "least_used":
		if id != nil && *id != 0 {
			return "", infraerrors.BadRequest("PROXY_SELECTION_CONFLICT", "自动选择不能同时指定代理")
		}
	default:
		return "", infraerrors.BadRequest("PROXY_SELECTION_INVALID", "代理模式仅支持 direct、specified、random、least_used")
	}
	return mode, nil
}

func (s *adminServiceImpl) scopedAccountProxySelection(ctx context.Context, mode string, id *int64) (*int64, error) {
	normalized, err := NormalizeAccountProxyMode(mode, id, false)
	if err != nil {
		return nil, err
	}
	switch normalized {
	case "", "specified":
		return id, nil
	case "direct":
		zero := int64(0)
		return &zero, nil
	}
	if dbent.TxFromContext(ctx) == nil {
		return nil, fmt.Errorf("自动代理选择需要账户写事务")
	}
	repo, ok := s.proxyRepo.(interface {
		SelectAccountProxy(context.Context, string) (int64, error)
	})
	if !ok {
		return nil, fmt.Errorf("自动代理选择服务不可用")
	}
	selected, err := repo.SelectAccountProxy(ctx, normalized)
	if err != nil {
		return nil, err
	}
	if selected <= 0 {
		return nil, infraerrors.BadRequest("PROXY_POOL_EMPTY", "没有启用且未过期的代理")
	}
	return &selected, nil
}
