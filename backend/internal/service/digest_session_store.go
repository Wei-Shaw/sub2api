package service

import (
	"strconv"
	"strings"
	"time"
)

// digestSessionTTL 摘要会话默认 TTL
const digestSessionTTL = 5 * time.Minute

// sessionEntry flat cache 条目
type sessionEntry struct {
	uuid      string
	accountID int64
	expiresAt time.Time
}

func (e *sessionEntry) sessionExpiresAt() time.Time { return e.expiresAt }
func (e *sessionEntry) sessionCost() int            { return len(e.uuid) }
func (e *sessionEntry) cloneSessionValue() sessionMemoryValue {
	cloned := *e
	cloned.uuid = strings.Clone(e.uuid)
	return &cloned
}

// DigestSessionStore 内存摘要会话存储（有界 TTL 缓存，增量摘要索引）
// key: "{groupID}:{prefixHash}|{digestChain}" → *sessionEntry
type DigestSessionStore struct {
	cache sessionMemoryCache
	ttl   time.Duration
}

// NewDigestSessionStore 创建内存摘要会话存储
func NewDigestSessionStore() *DigestSessionStore {
	return &DigestSessionStore{
		ttl: digestSessionTTL,
	}
}

func (s *DigestSessionStore) Close() { s.cache.Close() }

// Save 保存摘要会话。oldDigestChain 为 Find 返回的 matchedChain，用于删旧 key。
func (s *DigestSessionStore) Save(groupID int64, prefixHash, digestChain, uuid string, accountID int64, oldDigestChain string) {
	if digestChain == "" {
		return
	}
	ns := buildNS(groupID, prefixHash)
	ttl := s.ttl
	if ttl <= 0 {
		ttl = digestSessionTTL
	}
	s.cache.Store(ns+digestChain, &sessionEntry{uuid: uuid, accountID: accountID, expiresAt: time.Now().Add(ttl)})
	if oldDigestChain != "" && oldDigestChain != digestChain {
		s.cache.Delete(ns + oldDigestChain)
	}
}

// Find 查找摘要会话，在消息边界上返回最长匹配及对应 matchedChain。
func (s *DigestSessionStore) Find(groupID int64, prefixHash, digestChain string) (uuid string, accountID int64, matchedChain string, found bool) {
	if digestChain == "" {
		return "", 0, "", false
	}
	ns := buildNS(groupID, prefixHash)
	if val, chain, ok := s.cache.longestDigestPrefix(ns, digestChain); ok {
		if e, ok := val.(*sessionEntry); ok {
			return e.uuid, e.accountID, chain, true
		}
	}
	return "", 0, "", false
}

// buildNS 构建 namespace 前缀
func buildNS(groupID int64, prefixHash string) string {
	return strconv.FormatInt(groupID, 10) + ":" + prefixHash + "|"
}
