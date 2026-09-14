package service

import (
	"strconv"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
)

// adobeClientCache 按账号 + 代理地址缓存 *adobe.Client。
//
// Client 内部持有带 TLS 指纹的连接池，每请求新建会白白丢掉连接复用，也会让
// 指纹握手的开销叠加到每一次出图上。tlsclient 的 CookieJar 是客户端级的：
// IMS Set-Cookie 会写进 jar，同代理多账号共用一个 Client 会串 ims_sid。
type adobeClientCache struct {
	clients sync.Map // accountID\x00proxyURL -> *adobe.Client
	// newClient 为空时用真实构造函数；单测替换它来注入假传输，避免打真网络。
	newClient func(proxyURL string) *adobe.Client
}

func adobeClientCacheKey(account *Account, proxyURL string) string {
	if account == nil || account.ID == 0 {
		return proxyURL
	}
	return strconv.FormatInt(account.ID, 10) + "\x00" + proxyURL
}

func (c *adobeClientCache) get(key, proxyURL string) *adobe.Client {
	if cached, ok := c.clients.Load(key); ok {
		if client, ok := cached.(*adobe.Client); ok {
			return client
		}
	}
	build := c.newClient
	if build == nil {
		build = func(proxy string) *adobe.Client {
			return adobe.NewClient(adobe.ClientConfig{ProxyURL: proxy})
		}
	}
	client := build(proxyURL)
	actual, _ := c.clients.LoadOrStore(key, client)
	if stored, ok := actual.(*adobe.Client); ok {
		return stored
	}
	return client
}

// clientForAccount 返回该账号对应的 Firefly 客户端（跟随账号的代理配置）。
// 账号为 nil 时仍只按 proxy 缓存，供没有账号上下文的路径使用。
func (c *adobeClientCache) clientForAccount(account *Account) *adobe.Client {
	proxyURL := accountProxyURL(account)
	return c.get(adobeClientCacheKey(account, proxyURL), proxyURL)
}
