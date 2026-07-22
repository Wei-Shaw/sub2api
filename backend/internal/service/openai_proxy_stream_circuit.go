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
REDACTED

type openAIProxyStreamCircuitEntry struct {
	failureCount int
	windowStart  time.Time
	blockedUntil time.Time
	lastTouched  time.Time
REDACTED

// openAIProxyStreamCircuit is an in-process, proxy-ID keyed circuit. It is
// intentionally bounded and ephemeral: a restart clears observations, while a
// tripped entry expires automatically after its TTL.
type openAIProxyStreamCircuit struct {
	mu       sync.Mutex
	settings openAIProxyStreamCircuitSettings
	entries  map[int64]openAIProxyStreamCircuitEntry
REDACTED

func resolveOpenAIProxyStreamCircuitSettings(s *OpenAIGatewayService) openAIProxyStreamCircuitSettings {
	settings := openAIProxyStreamCircuitSettings{
		failureThreshold: defaultOpenAIProxyStreamFailureThreshold,
		failureWindow:    defaultOpenAIProxyStreamFailureWindow,
		quarantineTTL:    defaultOpenAIProxyStreamQuarantineTTL,
		maxEntries:       defaultOpenAIProxyStreamCircuitMaxEntries,
REDACTED
	if s == nil || s.cfg == nil {
		return settings
REDACTED
	cfg := s.cfg.Gateway.OpenAIProxyStreamCircuit
	if cfg.FailureThreshold > 0 {
		settings.failureThreshold = cfg.FailureThreshold
REDACTED
	if cfg.WindowSeconds > 0 {
		settings.failureWindow = time.Duration(cfg.WindowSeconds) * time.Second
REDACTED
	if cfg.TTLSeconds > 0 {
		settings.quarantineTTL = time.Duration(cfg.TTLSeconds) * time.Second
REDACTED
	return settings
REDACTED

func newOpenAIProxyStreamCircuit(settings openAIProxyStreamCircuitSettings) *openAIProxyStreamCircuit {
	if settings.failureThreshold <= 0 {
		settings.failureThreshold = defaultOpenAIProxyStreamFailureThreshold
REDACTED
	if settings.failureWindow <= 0 {
		settings.failureWindow = defaultOpenAIProxyStreamFailureWindow
REDACTED
	if settings.quarantineTTL <= 0 {
		settings.quarantineTTL = defaultOpenAIProxyStreamQuarantineTTL
REDACTED
	if settings.maxEntries <= 0 {
		settings.maxEntries = defaultOpenAIProxyStreamCircuitMaxEntries
REDACTED
	return &openAIProxyStreamCircuit{
		settings: settings,
		entries:  make(map[int64]openAIProxyStreamCircuitEntry),
REDACTED
REDACTED

func (s *OpenAIGatewayService) getOpenAIProxyStreamCircuit() *openAIProxyStreamCircuit {
	if s == nil {
		return nil
REDACTED
	s.openaiProxyStreamCircuitOnce.Do(func() {
		if s.openaiProxyStreamCircuit == nil {
			s.openaiProxyStreamCircuit = newOpenAIProxyStreamCircuit(resolveOpenAIProxyStreamCircuitSettings(s))
	REDACTED
REDACTED)
	return s.openaiProxyStreamCircuit
REDACTED

func (c *openAIProxyStreamCircuit) recordFailure(proxyID int64, now time.Time) (bool, time.Time) {
	if c == nil || proxyID <= 0 {
		return false, time.Time{REDACTED
REDACTED
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[proxyID]
	if exists && now.Before(entry.blockedUntil) {
		entry.lastTouched = now
		c.entries[proxyID] = entry
		return false, entry.blockedUntil
REDACTED
	if !exists {
		c.ensureCapacityLocked(now)
REDACTED
	if entry.windowStart.IsZero() || now.Before(entry.windowStart) || now.Sub(entry.windowStart) > c.settings.failureWindow {
		entry.failureCount = 0
		entry.windowStart = now
		entry.blockedUntil = time.Time{REDACTED
REDACTED
	entry.failureCount++
	entry.lastTouched = now
	tripped := entry.failureCount >= c.settings.failureThreshold
	if tripped {
		entry.blockedUntil = now.Add(c.settings.quarantineTTL)
REDACTED
	c.entries[proxyID] = entry
	return tripped, entry.blockedUntil
REDACTED

func (c *openAIProxyStreamCircuit) recordSuccess(proxyID int64) bool {
	if c == nil || proxyID <= 0 {
		return false
REDACTED
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[proxyID]; !ok {
		return false
REDACTED
	delete(c.entries, proxyID)
	return true
REDACTED

func (c *openAIProxyStreamCircuit) isBlocked(proxyID int64, now time.Time) bool {
	if c == nil || proxyID <= 0 {
		return false
REDACTED
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[proxyID]
	if !ok || entry.blockedUntil.IsZero() {
		return false
REDACTED
	if !now.Before(entry.blockedUntil) {
		delete(c.entries, proxyID)
		return false
REDACTED
	return true
REDACTED

func (c *openAIProxyStreamCircuit) ensureCapacityLocked(now time.Time) {
	if len(c.entries) < c.settings.maxEntries {
		return
REDACTED
	for proxyID, entry := range c.entries {
		staleObservation := entry.blockedUntil.IsZero() && now.Sub(entry.lastTouched) > c.settings.failureWindow
		expiredQuarantine := !entry.blockedUntil.IsZero() && !now.Before(entry.blockedUntil)
		if staleObservation || expiredQuarantine {
			delete(c.entries, proxyID)
	REDACTED
REDACTED
	if len(c.entries) < c.settings.maxEntries {
		return
REDACTED
	var oldestProxyID int64
	var oldest time.Time
	for proxyID, entry := range c.entries {
		if oldestProxyID == 0 || entry.lastTouched.Before(oldest) {
			oldestProxyID = proxyID
			oldest = entry.lastTouched
	REDACTED
REDACTED
	if oldestProxyID > 0 {
		delete(c.entries, oldestProxyID)
REDACTED
REDACTED

func openAIProxyStreamCircuitProxyID(account *Account) (int64, bool) {
	if account == nil || account.Platform != PlatformOpenAI || account.ProxyID == nil || *account.ProxyID <= 0 {
		return 0, false
REDACTED
	return *account.ProxyID, true
REDACTED

func (s *OpenAIGatewayService) recordOpenAIProxyStreamDisconnect(account *Account, streamErr error, upstreamRequestID string) {
	proxyID, ok := openAIProxyStreamCircuitProxyID(account)
	if !ok || streamErr == nil || errors.Is(streamErr, context.Canceled) || errors.Is(streamErr, context.DeadlineExceeded) {
		return
REDACTED
	circuit := s.getOpenAIProxyStreamCircuit()
	tripped, until := circuit.recordFailure(proxyID, time.Now())
	if !tripped {
		return
REDACTED
	logger.L().With(zap.String("component", "service.openai_gateway")).Warn(
		"openai.proxy_quarantined_stream_disconnect",
		zap.Int64("proxy_id", proxyID),
		zap.Int64("account_id", account.ID),
		zap.Time("until", until),
		zap.String("upstream_request_id", upstreamRequestID),
		zap.String("error", sanitizeUpstreamErrorMessage(streamErr.Error())),
	)
REDACTED

func (s *OpenAIGatewayService) clearOpenAIProxyStreamDisconnect(account *Account) {
	proxyID, ok := openAIProxyStreamCircuitProxyID(account)
	if !ok {
		return
REDACTED
	if circuit := s.getOpenAIProxyStreamCircuit(); circuit != nil {
		circuit.recordSuccess(proxyID)
REDACTED
REDACTED

func (s *OpenAIGatewayService) isOpenAIProxyStreamQuarantined(account *Account) bool {
	proxyID, ok := openAIProxyStreamCircuitProxyID(account)
	if !ok {
		return false
REDACTED
	circuit := s.getOpenAIProxyStreamCircuit()
	return circuit != nil && circuit.isBlocked(proxyID, time.Now())
REDACTED
