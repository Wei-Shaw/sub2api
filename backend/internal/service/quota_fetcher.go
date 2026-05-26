package service

import (
	"context"
)

// QuotaFetcher 额度获取接口，各平台实现此接口
type QuotaFetcher interface {
	// CanFetch 检查是否可以获取此账户的额度
	CanFetch(account *Account) bool
	// FetchQuota 获取账户额度信息
	FetchQuota(ctx context.Context, account *Account, proxyURL string) (*QuotaResult, error)
}

// QuotaResult 额度获取结果
type QuotaResult struct {
	UsageInfo *UsageInfo     // 转换后的使用信息
	Raw       map[string]any // 原始响应，可存入 account.Extra
}
