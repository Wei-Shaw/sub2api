package service

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const grokEndpointUnsupportedTTL = 24 * time.Hour

const grokEndpointUnsupportedResponseBody = `{"error":{"type":"upstream_error","message":"Grok upstream endpoint is temporarily unavailable after a previous 405 response"}}`

type grokEndpointUnsupportedKey struct {
	accountID int64
	scheme    string
	host      string
	method    string
	path      string
}

func newGrokEndpointUnsupportedKey(account *Account, req *http.Request) (grokEndpointUnsupportedKey, bool) {
	if account == nil || !account.IsGrok() || account.ID <= 0 || req == nil || req.URL == nil {
		return grokEndpointUnsupportedKey{}, false
	}
	method := strings.TrimSpace(req.Method)
	if method == "" {
		return grokEndpointUnsupportedKey{}, false
	}
	return grokEndpointUnsupportedKey{
		accountID: account.ID,
		scheme:    req.URL.Scheme,
		host:      req.URL.Host,
		method:    method,
		path:      canonicalGrokEndpointCachePath(req.URL.Path),
	}, true
}

// canonicalGrokEndpointCachePath keeps the configured base path in the key but
// collapses per-request video IDs. A 405 on /videos/{id} describes endpoint
// capability, not that individual video resource.
func canonicalGrokEndpointCachePath(path string) string {
	path = strings.TrimSuffix(path, "/")
	const videosMarker = "/videos/"
	markerIndex := strings.LastIndex(path, videosMarker)
	if markerIndex < 0 {
		return path
	}
	remainder := path[markerIndex+len(videosMarker):]
	if remainder == "" {
		return path
	}
	if !strings.Contains(remainder, "/") {
		return path[:markerIndex] + videosMarker + ":id"
	}
	if strings.Count(remainder, "/") == 1 && strings.HasSuffix(remainder, "/content") {
		return path[:markerIndex] + videosMarker + ":id/content"
	}
	return path
}

func (s *OpenAIGatewayService) grokEndpointCacheTime() time.Time {
	if s != nil && s.grokEndpointNow != nil {
		return s.grokEndpointNow()
	}
	return time.Now()
}

func (s *OpenAIGatewayService) isGrokEndpointUnsupported(key grokEndpointUnsupportedKey, now time.Time) bool {
	if s == nil {
		return false
	}
	s.grokEndpointUnsupportedMu.RLock()
	expiresAt, ok := s.grokEndpointUnsupported[key]
	s.grokEndpointUnsupportedMu.RUnlock()
	if !ok {
		return false
	}
	if now.Before(expiresAt) {
		return true
	}

	s.grokEndpointUnsupportedMu.Lock()
	if current, exists := s.grokEndpointUnsupported[key]; exists && !now.Before(current) {
		delete(s.grokEndpointUnsupported, key)
	}
	s.grokEndpointUnsupportedMu.Unlock()
	return false
}

func (s *OpenAIGatewayService) cacheGrokEndpointUnsupported(key grokEndpointUnsupportedKey, now time.Time) time.Time {
	expiresAt := now.Add(grokEndpointUnsupportedTTL)
	s.grokEndpointUnsupportedMu.Lock()
	if s.grokEndpointUnsupported == nil {
		s.grokEndpointUnsupported = make(map[grokEndpointUnsupportedKey]time.Time)
	}
	for cachedKey, cachedUntil := range s.grokEndpointUnsupported {
		if !now.Before(cachedUntil) {
			delete(s.grokEndpointUnsupported, cachedKey)
		}
	}
	s.grokEndpointUnsupported[key] = expiresAt
	s.grokEndpointUnsupportedMu.Unlock()
	return expiresAt
}

// doOpenAICompatibleUpstream adds endpoint-scoped negative caching around Grok
// traffic. A real 405 records only this account, origin, method, and endpoint for
// 24 hours. Cache hits return the same failover-class status without opening an
// upstream connection, so the handler can immediately select another account.
func (s *OpenAIGatewayService) doOpenAICompatibleUpstream(req *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	if s == nil || s.httpUpstream == nil {
		return nil, fmt.Errorf("openai-compatible upstream is not configured")
	}
	if account == nil {
		return nil, fmt.Errorf("openai-compatible account is required")
	}

	key, cacheable := newGrokEndpointUnsupportedKey(account, req)
	now := s.grokEndpointCacheTime()
	if cacheable && s.isGrokEndpointUnsupported(key, now) {
		return &http.Response{
			StatusCode:    http.StatusMethodNotAllowed,
			Status:        fmt.Sprintf("%d %s", http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed)),
			Header:        http.Header{"Content-Type": []string{"application/json"}},
			Body:          io.NopCloser(strings.NewReader(grokEndpointUnsupportedResponseBody)),
			ContentLength: int64(len(grokEndpointUnsupportedResponseBody)),
			Request:       req,
		}, nil
	}

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err == nil && resp != nil && cacheable && resp.StatusCode == http.StatusMethodNotAllowed {
		expiresAt := s.cacheGrokEndpointUnsupported(key, now)
		slog.Warn("grok_endpoint_method_not_allowed_cached",
			"account_id", account.ID,
			"method", key.method,
			"endpoint", key.path,
			"expires_at", expiresAt,
		)
	}
	return resp, err
}
