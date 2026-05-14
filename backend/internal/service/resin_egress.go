package service

import (
	"context"
	"strings"

	resinpkg "github.com/Wei-Shaw/sub2api/internal/pkg/resin"
)

func withAccountResinContext(ctx context.Context, accountID int64) context.Context {
	return resinpkg.WithAccountID(ctx, accountID)
}

func resolveAccountSharedProxyURL(ctx context.Context, proxyRepo ProxyRepository, account *Account) string {
	return resolveAccountStoredProxyURL(ctx, proxyRepo, account)
}

func resolveAccountDedicatedProxyURL(ctx context.Context, proxyRepo ProxyRepository, account *Account) string {
	if account != nil && account.ID > 0 {
		if account.Proxy != nil {
			if cfg, err := account.Proxy.ResinConfig(); err == nil && cfg != nil {
				return cfg.ForwardProxyURLForAccount(account.ID)
			}
		}
		if account.ProxyID != nil && proxyRepo != nil {
			if proxy, err := proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && proxy != nil {
				if cfg, cfgErr := proxy.ResinConfig(); cfgErr == nil && cfg != nil {
					return cfg.ForwardProxyURLForAccount(account.ID)
				}
			}
		}
	}
	return resolveAccountStoredProxyURL(ctx, proxyRepo, account)
}

func resolveAccountStoredProxyURL(ctx context.Context, proxyRepo ProxyRepository, account *Account) string {
	if account == nil {
		return ""
	}
	if account.Proxy != nil {
		return strings.TrimSpace(account.Proxy.URL())
	}
	if account.ProxyID != nil && proxyRepo != nil {
		if proxy, err := proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && proxy != nil {
			return strings.TrimSpace(proxy.URL())
		}
	}
	return ""
}
