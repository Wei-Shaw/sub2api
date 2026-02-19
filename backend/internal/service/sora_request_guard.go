package service

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/soraerror"
	"github.com/google/uuid"
)

type soraChallengeCooldownEntry struct {
	Until      time.Time
	StatusCode int
	CFRay      string
REDACTED

type soraSidecarSessionEntry struct {
	SessionKey string
	ExpiresAt  time.Time
	LastUsedAt time.Time
REDACTED

func (c *SoraDirectClient) cloudflareChallengeCooldownSeconds() int {
	if c == nil || c.cfg == nil {
		return 900
REDACTED
	cooldown := c.cfg.Sora.Client.CloudflareChallengeCooldownSeconds
	if cooldown <= 0 {
		return 0
REDACTED
	return cooldown
REDACTED

func (c *SoraDirectClient) checkCloudflareChallengeCooldown(account *Account, proxyURL string) error {
	if c == nil {
		return nil
REDACTED
	if account == nil || account.ID <= 0 {
		return nil
REDACTED
	cooldownSeconds := c.cloudflareChallengeCooldownSeconds()
	if cooldownSeconds <= 0 {
		return nil
REDACTED
	key := soraAccountProxyKey(account, proxyURL)
	now := time.Now()

	c.challengeCooldownMu.RLock()
	entry, ok := c.challengeCooldowns[key]
	c.challengeCooldownMu.RUnlock()
	if !ok {
		return nil
REDACTED
	if !entry.Until.After(now) {
		c.challengeCooldownMu.Lock()
		delete(c.challengeCooldowns, key)
		c.challengeCooldownMu.Unlock()
		return nil
REDACTED

	remaining := int(math.Ceil(entry.Until.Sub(now).Seconds()))
	if remaining < 1 {
		remaining = 1
REDACTED
	message := fmt.Sprintf("Sora request cooling down due to recent Cloudflare challenge. Retry in %d seconds.", remaining)
	if entry.CFRay != "" {
		message = fmt.Sprintf("%s (last cf-ray: %s)", message, entry.CFRay)
REDACTED
	return &SoraUpstreamError{
		StatusCode: http.StatusTooManyRequests,
		Message:    message,
		Headers:    make(http.Header),
REDACTED
REDACTED

func (c *SoraDirectClient) recordCloudflareChallengeCooldown(account *Account, proxyURL string, statusCode int, headers http.Header, body []byte) {
	if c == nil {
		return
REDACTED
	if account == nil || account.ID <= 0 {
		return
REDACTED
	cooldownSeconds := c.cloudflareChallengeCooldownSeconds()
	if cooldownSeconds <= 0 {
		return
REDACTED
	key := soraAccountProxyKey(account, proxyURL)
	now := time.Now()
	until := now.Add(time.Duration(cooldownSeconds) * time.Second)
	cfRay := soraerror.ExtractCloudflareRayID(headers, body)

	c.challengeCooldownMu.Lock()
	c.cleanupExpiredChallengeCooldownsLocked(now)
	existing, ok := c.challengeCooldowns[key]
	if ok && existing.Until.After(until) {
		until = existing.Until
		if cfRay == "" {
			cfRay = existing.CFRay
	REDACTED
REDACTED
	c.challengeCooldowns[key] = soraChallengeCooldownEntry{
		Until:      until,
		StatusCode: statusCode,
		CFRay:      cfRay,
REDACTED
	c.challengeCooldownMu.Unlock()

	if c.debugEnabled() {
		remain := int(math.Ceil(until.Sub(now).Seconds()))
		if remain < 0 {
			remain = 0
	REDACTED
		c.debugLogf("cloudflare_challenge_cooldown_set key=%s status=%d remain_s=%d cf_ray=%s", key, statusCode, remain, cfRay)
REDACTED
REDACTED

func (c *SoraDirectClient) sidecarSessionKey(account *Account, proxyURL string) string {
	if c == nil || !c.sidecarSessionReuseEnabled() {
		return ""
REDACTED
	if account == nil || account.ID <= 0 {
		return ""
REDACTED
	key := soraAccountProxyKey(account, proxyURL)
	now := time.Now()
	ttlSeconds := c.sidecarSessionTTLSeconds()

	c.sidecarSessionMu.Lock()
	defer c.sidecarSessionMu.Unlock()
	c.cleanupExpiredSidecarSessionsLocked(now)
	if existing, exists := c.sidecarSessions[key]; exists {
		existing.LastUsedAt = now
		c.sidecarSessions[key] = existing
		return existing.SessionKey
REDACTED

	expiresAt := now.Add(time.Duration(ttlSeconds) * time.Second)
	if ttlSeconds <= 0 {
		expiresAt = now.Add(365 * 24 * time.Hour)
REDACTED
	newEntry := soraSidecarSessionEntry{
		SessionKey: "sora-" + uuid.NewString(),
		ExpiresAt:  expiresAt,
		LastUsedAt: now,
REDACTED
	c.sidecarSessions[key] = newEntry

	if c.debugEnabled() {
		c.debugLogf("sidecar_session_created key=%s ttl_s=%d", key, ttlSeconds)
REDACTED
	return newEntry.SessionKey
REDACTED

func (c *SoraDirectClient) cleanupExpiredChallengeCooldownsLocked(now time.Time) {
	if c == nil || len(c.challengeCooldowns) == 0 {
		return
REDACTED
	for key, entry := range c.challengeCooldowns {
		if !entry.Until.After(now) {
			delete(c.challengeCooldowns, key)
	REDACTED
REDACTED
REDACTED

func (c *SoraDirectClient) cleanupExpiredSidecarSessionsLocked(now time.Time) {
	if c == nil || len(c.sidecarSessions) == 0 {
		return
REDACTED
	for key, entry := range c.sidecarSessions {
		if !entry.ExpiresAt.After(now) {
			delete(c.sidecarSessions, key)
	REDACTED
REDACTED
REDACTED

func soraAccountProxyKey(account *Account, proxyURL string) string {
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
REDACTED
	return fmt.Sprintf("account:%d|proxy:%s", accountID, normalizeSoraProxyKey(proxyURL))
REDACTED

func normalizeSoraProxyKey(proxyURL string) string {
	raw := strings.TrimSpace(proxyURL)
	if raw == "" {
		return "direct"
REDACTED
	parsed, err := url.Parse(raw)
	if err != nil {
		return strings.ToLower(raw)
REDACTED
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	port := strings.TrimSpace(parsed.Port())
	if host == "" {
		return strings.ToLower(raw)
REDACTED
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
REDACTED
	if port != "" {
		host = host + ":" + port
REDACTED
	if scheme == "" {
		scheme = "proxy"
REDACTED
	return scheme + "://" + host
REDACTED
