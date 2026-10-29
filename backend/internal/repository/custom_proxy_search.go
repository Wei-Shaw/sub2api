package repository

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/proxy"
)

// proxyOptionSearchPredicate 数字关键词同时匹配端口和 ID；文本搜索保持参数化。
func proxyOptionSearchPredicate(search string) predicate.Proxy {
	filters := []predicate.Proxy{proxy.NameContainsFold(search), proxy.HostContainsFold(search)}
	if n, err := strconv.ParseInt(search, 10, 64); err == nil && n > 0 {
		if n <= 65535 {
			filters = append(filters, proxy.PortEQ(int(n)))
		}
		filters = append(filters, proxy.IDEQ(n))
	}
	return proxy.Or(filters...)
}
