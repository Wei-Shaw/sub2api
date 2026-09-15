package service

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
)

// adobeClientIdleTTL 是缓存条目闲置多久后被回收（账号删除、长期不用）。
const adobeClientIdleTTL = 30 * time.Minute

// adobeClientSweepInterval 限制惰性清理的频率，避免每次取客户端都遍历整张表。
const adobeClientSweepInterval = time.Minute

// adobeClientCache 按账号缓存 *adobe.Client。
//
// Client 内部持有带 TLS 指纹的连接池，每请求新建会白白丢掉连接复用，也会让
// 指纹握手的开销叠加到每一次出图上。tlsclient 的 CookieJar 是客户端级的：
// IMS Set-Cookie 会写进 jar，多账号共用一个 Client 会串 ims_sid。
//
// 每个账号只保留一个条目：代理或 cookie 变化时整个 Client 重建，旧 jar 里的会话 cookie
// 不会跟着新 cookie 一起发出去；闲置超过 adobeClientIdleTTL 的条目被惰性回收。
type adobeClientCache struct {
	mu        sync.Mutex
	clients   map[int64]*adobeClientEntry
	lastSweep time.Time
	// newClient 为空时用真实构造函数；单测替换它来注入假传输，避免打真网络。
	newClient func(proxyURL string) *adobe.Client
}

type adobeClientEntry struct {
	client      *adobe.Client
	fingerprint string
	lastUsed    time.Time
}

// adobeClientFingerprint 标识一个 Client 可复用的前提：同一代理、同一份 cookie。
// cookie 只存摘要，缓存里不留明文凭据。
func adobeClientFingerprint(account *Account, proxyURL string) string {
	sum := sha256.Sum256([]byte(account.GetCredential("cookie")))
	return proxyURL + "\x00" + hex.EncodeToString(sum[:])
}

func (c *adobeClientCache) build(proxyURL string) *adobe.Client {
	if c.newClient != nil {
		return c.newClient(proxyURL)
	}
	return adobe.NewClient(adobe.ClientConfig{ProxyURL: proxyURL})
}

// clientForAccount 返回该账号对应的 Firefly 客户端（跟随账号的代理配置）。
// 账号为 nil 或尚未落库（ID<=0）时不缓存，每次新建，避免按代理地址在账号之间共用 jar。
func (c *adobeClientCache) clientForAccount(account *Account) *adobe.Client {
	proxyURL := accountProxyURL(account)
	if account == nil || account.ID <= 0 {
		return c.build(proxyURL)
	}

	fingerprint := adobeClientFingerprint(account, proxyURL)
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.clients == nil {
		c.clients = make(map[int64]*adobeClientEntry)
	}
	c.sweepLocked(now)
	if entry, ok := c.clients[account.ID]; ok && entry.fingerprint == fingerprint {
		entry.lastUsed = now
		return entry.client
	}
	client := c.build(proxyURL)
	c.clients[account.ID] = &adobeClientEntry{client: client, fingerprint: fingerprint, lastUsed: now}
	return client
}

// sweepLocked 回收闲置条目；调用方持有 c.mu。
func (c *adobeClientCache) sweepLocked(now time.Time) {
	if now.Sub(c.lastSweep) < adobeClientSweepInterval {
		return
	}
	c.lastSweep = now
	for id, entry := range c.clients {
		if now.Sub(entry.lastUsed) > adobeClientIdleTTL {
			delete(c.clients, id)
		}
	}
}
