package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	defaultOpenAIProxyStreamFailureThreshold  = 2
	defaultOpenAIProxyStreamFailureWindow     = time.Minute
	defaultOpenAIProxyStreamQuarantineTTL     = 10 * time.Minute
	defaultOpenAIProxyStreamCircuitMaxEntries = 4096
)

type openAIProxyStreamCircuitSettings struct {
	failureThreshold int
	failureWindow    time.Duration
	quarantineTTL    time.Duration
	maxEntries       int
}

type openAIProxyStreamCircuitEntry struct {
	failureCount int
	windowStart  time.Time
	blockedUntil time.Time
	lastTouched  time.Time
}

// openAIProxyStreamCircuit is an in-process, proxy-ID keyed circuit. It is
// intentionally bounded and ephemeral: a restart clears observations, while a
// tripped entry expires automatically after its TTL.
type openAIProxyStreamCircuit struct {
	mu       sync.Mutex
	settings openAIProxyStreamCircuitSettings
	entries  map[int64]openAIProxyStreamCircuitEntry
}

func resolveOpenAIProxyStreamCircuitSettings(s *OpenAIGatewayService) openAIProxyStreamCircuitSettings {
	settings := openAIProxyStreamCircuitSettings{
		failureThreshold: defaultOpenAIProxyStreamFailureThreshold,
		failureWindow:    defaultOpenAIProxyStreamFailureWindow,
		quarantineTTL:    defaultOpenAIProxyStreamQuarantineTTL,
		maxEntries:       defaultOpenAIProxyStreamCircuitMaxEntries,
	}
	if s == nil || s.cfg == nil {
		return settings
	}
	cfg := s.cfg.Gateway.OpenAIProxyStreamCircuit
	if cfg.FailureThreshold > 0 {
		settings.failureThreshold = cfg.FailureThreshold
	}
	if cfg.WindowSeconds > 0 {
		settings.failureWindow = time.Duration(cfg.WindowSeconds) * time.Second
	}
	if cfg.TTLSeconds > 0 {
		settings.quarantineTTL = time.Duration(cfg.TTLSeconds) * time.Second
	}
	return settings
}

func newOpenAIProxyStreamCircuit(settings openAIProxyStreamCircuitSettings) *openAIProxyStreamCircuit {
	if settings.failureThreshold <= 0 {
		settings.failureThreshold = defaultOpenAIProxyStreamFailureThreshold
	}
	if settings.failureWindow <= 0 {
		settings.failureWindow = defaultOpenAIProxyStreamFailureWindow
	}
	if settings.quarantineTTL <= 0 {
		settings.quarantineTTL = defaultOpenAIProxyStreamQuarantineTTL
	}
	if settings.maxEntries <= 0 {
		settings.maxEntries = defaultOpenAIProxyStreamCircuitMaxEntries
	}
	return &openAIProxyStreamCircuit{
		settings: settings,
		entries:  make(map[int64]openAIProxyStreamCircuitEntry),
	}
}

func (s *OpenAIGatewayService) getOpenAIProxyStreamCircuit() *openAIProxyStreamCircuit {
	if s == nil {
		return nil
	}
	s.openaiProxyStreamCircuitOnce.Do(func() {
		if s.openaiProxyStreamCircuit == nil {
			s.openaiProxyStreamCircuit = newOpenAIProxyStreamCircuit(resolveOpenAIProxyStreamCircuitSettings(s))
		}
	})
	return s.openaiProxyStreamCircuit
}

func (c *openAIProxyStreamCircuit) recordFailure(proxyID int64, now time.Time) (bool, time.Time) {
	if c == nil || proxyID <= 0 {
		return false, time.Time{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[proxyID]
	if exists && now.Before(entry.blockedUntil) {
		entry.lastTouched = now
		c.entries[proxyID] = entry
		return false, entry.blockedUntil
	}
	if !exists {
		c.ensureCapacityLocked(now)
	}
	if entry.windowStart.IsZero() || now.Before(entry.windowStart) || now.Sub(entry.windowStart) > c.settings.failureWindow {
		entry.failureCount = 0
		entry.windowStart = now
		entry.blockedUntil = time.Time{}
	}
	entry.failureCount++
	entry.lastTouched = now
	tripped := entry.failureCount >= c.settings.failureThreshold
	if tripped {
		entry.blockedUntil = now.Add(c.settings.quarantineTTL)
	}
	c.entries[proxyID] = entry
	return tripped, entry.blockedUntil
}

func (c *openAIProxyStreamCircuit) recordSuccess(proxyID int64) bool {
	if c == nil || proxyID <= 0 {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[proxyID]; !ok {
		return false
	}
	delete(c.entries, proxyID)
	return true
}

func (c *openAIProxyStreamCircuit) isBlocked(proxyID int64, now time.Time) bool {
	if c == nil || proxyID <= 0 {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[proxyID]
	if !ok || entry.blockedUntil.IsZero() {
		return false
	}
	if !now.Before(entry.blockedUntil) {
		delete(c.entries, proxyID)
		return false
	}
	return true
}

func (c *openAIProxyStreamCircuit) ensureCapacityLocked(now time.Time) {
	if len(c.entries) < c.settings.maxEntries {
		return
	}
	for proxyID, entry := range c.entries {
		staleObservation := entry.blockedUntil.IsZero() && now.Sub(entry.lastTouched) > c.settings.failureWindow
		expiredQuarantine := !entry.blockedUntil.IsZero() && !now.Before(entry.blockedUntil)
		if staleObservation || expiredQuarantine {
			delete(c.entries, proxyID)
		}
	}
	if len(c.entries) < c.settings.maxEntries {
		return
	}
	var oldestProxyID int64
	var oldest time.Time
	for proxyID, entry := range c.entries {
		if oldestProxyID == 0 || entry.lastTouched.Before(oldest) {
			oldestProxyID = proxyID
			oldest = entry.lastTouched
		}
	}
	if oldestProxyID > 0 {
		delete(c.entries, oldestProxyID)
	}
}

func openAIProxyStreamCircuitProxyID(account *Account) (int64, bool) {
	if account == nil || account.Platform != PlatformOpenAI || account.ProxyID == nil || *account.ProxyID <= 0 {
		return 0, false
	}
	return *account.ProxyID, true
}

func (s *OpenAIGatewayService) recordOpenAIProxyStreamDisconnect(account *Account, streamErr error, upstreamRequestID string) {
	proxyID, ok := openAIProxyStreamCircuitProxyID(account)
	if !ok || streamErr == nil || errors.Is(streamErr, context.Canceled) || errors.Is(streamErr, context.DeadlineExceeded) {
		return
	}
	circuit := s.getOpenAIProxyStreamCircuit()
	tripped, until := circuit.recordFailure(proxyID, time.Now())
	if !tripped {
		return
	}
	logger.L().With(zap.String("component", "service.openai_gateway")).Warn(
		"openai.proxy_quarantined_stream_disconnect",
		zap.Int64("proxy_id", proxyID),
		zap.Int64("account_id", account.ID),
		zap.Time("until", until),
		zap.String("upstream_request_id", upstreamRequestID),
		zap.String("error", sanitizeUpstreamErrorMessage(streamErr.Error())),
	)
}

func (s *OpenAIGatewayService) clearOpenAIProxyStreamDisconnect(account *Account) {
	proxyID, ok := openAIProxyStreamCircuitProxyID(account)
	if !ok {
		return
	}
	if circuit := s.getOpenAIProxyStreamCircuit(); circuit != nil {
		circuit.recordSuccess(proxyID)
	}
}

func (s *OpenAIGatewayService) isOpenAIProxyStreamQuarantined(account *Account) bool {
	proxyID, ok := openAIProxyStreamCircuitProxyID(account)
	if !ok {
		return false
	}
	circuit := s.getOpenAIProxyStreamCircuit()
	return circuit != nil && circuit.isBlocked(proxyID, time.Now())
}
