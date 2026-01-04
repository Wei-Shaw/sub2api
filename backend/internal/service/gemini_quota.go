package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

type geminiModelClass string

const (
	geminiModelPro   geminiModelClass = "pro"
	geminiModelFlash geminiModelClass = "flash"
)

type GeminiQuota struct {
	// SharedRPD is a shared requests-per-day pool across models.
	// When SharedRPD > 0, callers should treat ProRPD/FlashRPD as not applicable for daily quota checks.
	SharedRPD int64 `json:"shared_rpd,omitempty"`
	// SharedRPM is a shared requests-per-minute pool across models.
	// When SharedRPM > 0, callers should treat ProRPM/FlashRPM as not applicable for minute quota checks.
	SharedRPM int64 `json:"shared_rpm,omitempty"`

	// Per-model quotas (AI Studio / API key).
	// A value of -1 means "unlimited" (pay-as-you-go).
	ProRPD   int64 `json:"pro_rpd,omitempty"`
	ProRPM   int64 `json:"pro_rpm,omitempty"`
	FlashRPD int64 `json:"flash_rpd,omitempty"`
	FlashRPM int64 `json:"flash_rpm,omitempty"`
REDACTED

type GeminiTierPolicy struct {
	Quota    GeminiQuota
	Cooldown time.Duration
REDACTED

type GeminiQuotaPolicy struct {
	tiers map[string]GeminiTierPolicy
REDACTED

type GeminiUsageTotals struct {
	ProRequests   int64
	FlashRequests int64
	ProTokens     int64
	FlashTokens   int64
	ProCost       float64
	FlashCost     float64
REDACTED

const geminiQuotaCacheTTL = time.Minute

type geminiQuotaOverridesV1 struct {
	Tiers map[string]config.GeminiTierQuotaConfig `json:"tiers"`
REDACTED

type geminiQuotaOverridesV2 struct {
	QuotaRules map[string]geminiQuotaRuleOverride `json:"quota_rules"`
REDACTED

type geminiQuotaRuleOverride struct {
	SharedRPD   *int64                    `json:"shared_rpd,omitempty"`
	SharedRPM   *int64                    `json:"rpm,omitempty"`
	GeminiPro   *geminiModelQuotaOverride `json:"gemini_pro,omitempty"`
	GeminiFlash *geminiModelQuotaOverride `json:"gemini_flash,omitempty"`
	Desc        *string                   `json:"desc,omitempty"`
REDACTED

type geminiModelQuotaOverride struct {
	RPD *int64 `json:"rpd,omitempty"`
	RPM *int64 `json:"rpm,omitempty"`
REDACTED

type GeminiQuotaService struct {
	cfg         *config.Config
	settingRepo SettingRepository
	mu          sync.Mutex
	cachedAt    time.Time
	policy      *GeminiQuotaPolicy
REDACTED

func NewGeminiQuotaService(cfg *config.Config, settingRepo SettingRepository) *GeminiQuotaService {
	return &GeminiQuotaService{
		cfg:         cfg,
		settingRepo: settingRepo,
REDACTED
REDACTED

func (s *GeminiQuotaService) Policy(ctx context.Context) *GeminiQuotaPolicy {
	if s == nil {
		return newGeminiQuotaPolicy()
REDACTED

	now := time.Now()
	s.mu.Lock()
	if s.policy != nil && now.Sub(s.cachedAt) < geminiQuotaCacheTTL {
		policy := s.policy
		s.mu.Unlock()
		return policy
REDACTED
	s.mu.Unlock()

	policy := newGeminiQuotaPolicy()
	if s.cfg != nil {
		policy.ApplyOverrides(s.cfg.Gemini.Quota.Tiers)
		if strings.TrimSpace(s.cfg.Gemini.Quota.Policy) != "" {
			raw := []byte(s.cfg.Gemini.Quota.Policy)
			var overridesV2 geminiQuotaOverridesV2
			if err := json.Unmarshal(raw, &overridesV2); err == nil && len(overridesV2.QuotaRules) > 0 {
				policy.ApplyQuotaRulesOverrides(overridesV2.QuotaRules)
		REDACTED else {
				var overridesV1 geminiQuotaOverridesV1
				if err := json.Unmarshal(raw, &overridesV1); err != nil {
					log.Printf("gemini quota: parse config policy failed: %v", err)
			REDACTED else {
					policy.ApplyOverrides(overridesV1.Tiers)
			REDACTED
		REDACTED
	REDACTED
REDACTED

	if s.settingRepo != nil {
		value, err := s.settingRepo.GetValue(ctx, SettingKeyGeminiQuotaPolicy)
		if err != nil && !errors.Is(err, ErrSettingNotFound) {
			log.Printf("gemini quota: load setting failed: %v", err)
	REDACTED else if strings.TrimSpace(value) != "" {
			raw := []byte(value)
			var overridesV2 geminiQuotaOverridesV2
			if err := json.Unmarshal(raw, &overridesV2); err == nil && len(overridesV2.QuotaRules) > 0 {
				policy.ApplyQuotaRulesOverrides(overridesV2.QuotaRules)
		REDACTED else {
				var overridesV1 geminiQuotaOverridesV1
				if err := json.Unmarshal(raw, &overridesV1); err != nil {
					log.Printf("gemini quota: parse setting failed: %v", err)
			REDACTED else {
					policy.ApplyOverrides(overridesV1.Tiers)
			REDACTED
		REDACTED
	REDACTED
REDACTED

	s.mu.Lock()
	s.policy = policy
	s.cachedAt = now
	s.mu.Unlock()

	return policy
REDACTED

func (s *GeminiQuotaService) QuotaForAccount(ctx context.Context, account *Account) (GeminiQuota, bool) {
	if account == nil || account.Platform != PlatformGemini {
		return GeminiQuota{REDACTED, false
REDACTED

	// Map (oauth_type + tier_id) to a canonical policy tier key.
	// This keeps the policy table stable even if upstream tier_id strings vary.
	tierKey := geminiQuotaTierKeyForAccount(account)
	if tierKey == "" {
		return GeminiQuota{REDACTED, false
REDACTED

	policy := s.Policy(ctx)
	return policy.QuotaForTier(tierKey)
REDACTED

func (s *GeminiQuotaService) CooldownForTier(ctx context.Context, tierID string) time.Duration {
	policy := s.Policy(ctx)
	return policy.CooldownForTier(tierID)
REDACTED

func (s *GeminiQuotaService) CooldownForAccount(ctx context.Context, account *Account) time.Duration {
	if s == nil || account == nil || account.Platform != PlatformGemini {
		return 5 * time.Minute
REDACTED
	tierKey := geminiQuotaTierKeyForAccount(account)
	if strings.TrimSpace(tierKey) == "" {
		return 5 * time.Minute
REDACTED
	return s.CooldownForTier(ctx, tierKey)
REDACTED

func newGeminiQuotaPolicy() *GeminiQuotaPolicy {
	return &GeminiQuotaPolicy{
		tiers: map[string]GeminiTierPolicy{
			// --- AI Studio / API Key (per-model) ---
			// aistudio_free:
			//   - gemini_pro:   50 RPD / 2 RPM
			//   - gemini_flash: 1500 RPD / 15 RPM
			GeminiTierAIStudioFree: {Quota: GeminiQuota{ProRPD: 50, ProRPM: 2, FlashRPD: 1500, FlashRPM: 15REDACTED, Cooldown: 30 * time.MinuteREDACTED,
			// aistudio_paid: -1 means "unlimited/pay-as-you-go" for RPD.
			GeminiTierAIStudioPaid: {Quota: GeminiQuota{ProRPD: -1, ProRPM: 1000, FlashRPD: -1, FlashRPM: 2000REDACTED, Cooldown: 5 * time.MinuteREDACTED,

			// --- Google One (shared pool) ---
			GeminiTierGoogleOneFree: {Quota: GeminiQuota{SharedRPD: 1000, SharedRPM: 60REDACTED, Cooldown: 30 * time.MinuteREDACTED,
			GeminiTierGoogleAIPro:   {Quota: GeminiQuota{SharedRPD: 1500, SharedRPM: 120REDACTED, Cooldown: 5 * time.MinuteREDACTED,
			GeminiTierGoogleAIUltra: {Quota: GeminiQuota{SharedRPD: 2000, SharedRPM: 120REDACTED, Cooldown: 5 * time.MinuteREDACTED,

			// --- GCP Code Assist (shared pool) ---
			GeminiTierGCPStandard:   {Quota: GeminiQuota{SharedRPD: 1500, SharedRPM: 120REDACTED, Cooldown: 5 * time.MinuteREDACTED,
			GeminiTierGCPEnterprise: {Quota: GeminiQuota{SharedRPD: 2000, SharedRPM: 120REDACTED, Cooldown: 5 * time.MinuteREDACTED,
	REDACTED,
REDACTED
REDACTED

func (p *GeminiQuotaPolicy) ApplyOverrides(tiers map[string]config.GeminiTierQuotaConfig) {
	if p == nil || len(tiers) == 0 {
		return
REDACTED
	for rawID, override := range tiers {
		tierID := normalizeGeminiTierID(rawID)
		if tierID == "" {
			continue
	REDACTED
		policy, ok := p.tiers[tierID]
		if !ok {
			policy = GeminiTierPolicy{Cooldown: 5 * time.MinuteREDACTED
	REDACTED
		// Backward-compatible overrides:
		// - If the tier uses shared quota, interpret pro_rpd as shared_rpd.
		// - Otherwise apply per-model overrides.
		if override.ProRPD != nil {
			if policy.Quota.SharedRPD > 0 {
				policy.Quota.SharedRPD = clampGeminiQuotaInt64WithUnlimited(*override.ProRPD)
		REDACTED else {
				policy.Quota.ProRPD = clampGeminiQuotaInt64WithUnlimited(*override.ProRPD)
		REDACTED
	REDACTED
		if override.FlashRPD != nil {
			if policy.Quota.SharedRPD > 0 {
				// No separate flash RPD for shared tiers.
		REDACTED else {
				policy.Quota.FlashRPD = clampGeminiQuotaInt64WithUnlimited(*override.FlashRPD)
		REDACTED
	REDACTED
		if override.CooldownMinutes != nil {
			minutes := clampGeminiQuotaInt(*override.CooldownMinutes)
			policy.Cooldown = time.Duration(minutes) * time.Minute
	REDACTED
		p.tiers[tierID] = policy
REDACTED
REDACTED

func (p *GeminiQuotaPolicy) ApplyQuotaRulesOverrides(rules map[string]geminiQuotaRuleOverride) {
	if p == nil || len(rules) == 0 {
		return
REDACTED
	for rawID, override := range rules {
		tierID := normalizeGeminiTierID(rawID)
		if tierID == "" {
			continue
	REDACTED
		policy, ok := p.tiers[tierID]
		if !ok {
			policy = GeminiTierPolicy{Cooldown: 5 * time.MinuteREDACTED
	REDACTED

		if override.SharedRPD != nil {
			policy.Quota.SharedRPD = clampGeminiQuotaInt64WithUnlimited(*override.SharedRPD)
	REDACTED
		if override.SharedRPM != nil {
			policy.Quota.SharedRPM = clampGeminiQuotaRPM(*override.SharedRPM)
	REDACTED
		if override.GeminiPro != nil {
			if override.GeminiPro.RPD != nil {
				policy.Quota.ProRPD = clampGeminiQuotaInt64WithUnlimited(*override.GeminiPro.RPD)
		REDACTED
			if override.GeminiPro.RPM != nil {
				policy.Quota.ProRPM = clampGeminiQuotaRPM(*override.GeminiPro.RPM)
		REDACTED
	REDACTED
		if override.GeminiFlash != nil {
			if override.GeminiFlash.RPD != nil {
				policy.Quota.FlashRPD = clampGeminiQuotaInt64WithUnlimited(*override.GeminiFlash.RPD)
		REDACTED
			if override.GeminiFlash.RPM != nil {
				policy.Quota.FlashRPM = clampGeminiQuotaRPM(*override.GeminiFlash.RPM)
		REDACTED
	REDACTED

		p.tiers[tierID] = policy
REDACTED
REDACTED

func (p *GeminiQuotaPolicy) QuotaForTier(tierID string) (GeminiQuota, bool) {
	policy, ok := p.policyForTier(tierID)
	if !ok {
		return GeminiQuota{REDACTED, false
REDACTED
	return policy.Quota, true
REDACTED

func (p *GeminiQuotaPolicy) CooldownForTier(tierID string) time.Duration {
	policy, ok := p.policyForTier(tierID)
	if ok && policy.Cooldown > 0 {
		return policy.Cooldown
REDACTED
	return 5 * time.Minute
REDACTED

func (p *GeminiQuotaPolicy) policyForTier(tierID string) (GeminiTierPolicy, bool) {
	if p == nil {
		return GeminiTierPolicy{REDACTED, false
REDACTED
	normalized := normalizeGeminiTierID(tierID)
	if policy, ok := p.tiers[normalized]; ok {
		return policy, true
REDACTED
	return GeminiTierPolicy{REDACTED, false
REDACTED

func normalizeGeminiTierID(tierID string) string {
	tierID = strings.TrimSpace(tierID)
	if tierID == "" {
		return ""
REDACTED
	// Prefer canonical mapping (handles legacy tier strings).
	if canonical := canonicalGeminiTierID(tierID); canonical != "" {
		return canonical
REDACTED
	// Accept older policy keys that used uppercase names.
	switch strings.ToUpper(tierID) {
	case "AISTUDIO_FREE":
		return GeminiTierAIStudioFree
	case "AISTUDIO_PAID":
		return GeminiTierAIStudioPaid
	case "GOOGLE_ONE_FREE":
		return GeminiTierGoogleOneFree
	case "GOOGLE_AI_PRO":
		return GeminiTierGoogleAIPro
	case "GOOGLE_AI_ULTRA":
		return GeminiTierGoogleAIUltra
	case "GCP_STANDARD":
		return GeminiTierGCPStandard
	case "GCP_ENTERPRISE":
		return GeminiTierGCPEnterprise
REDACTED
	return strings.ToLower(tierID)
REDACTED

func clampGeminiQuotaInt64WithUnlimited(value int64) int64 {
	if value < -1 {
		return 0
REDACTED
	return value
REDACTED

func clampGeminiQuotaInt(value int) int {
	if value < 0 {
		return 0
REDACTED
	return value
REDACTED

func clampGeminiQuotaRPM(value int64) int64 {
	if value < 0 {
		return 0
REDACTED
	return value
REDACTED

func geminiCooldownForTier(tierID string) time.Duration {
	policy := newGeminiQuotaPolicy()
	return policy.CooldownForTier(tierID)
REDACTED

func geminiQuotaTierKeyForAccount(account *Account) string {
	if account == nil || account.Platform != PlatformGemini {
		return ""
REDACTED

	// Note: GeminiOAuthType() already defaults legacy (project_id present) to code_assist.
	oauthType := strings.ToLower(strings.TrimSpace(account.GeminiOAuthType()))
	rawTier := strings.TrimSpace(account.GeminiTierID())

	// Prefer the canonical tier stored in credentials.
	if tierID := canonicalGeminiTierIDForOAuthType(oauthType, rawTier); tierID != "" && tierID != GeminiTierGoogleOneUnknown {
		return tierID
REDACTED

	// Fallback defaults when tier_id is missing or unknown.
	switch oauthType {
	case "google_one":
		return GeminiTierGoogleOneFree
	case "code_assist":
		return GeminiTierGCPStandard
	case "ai_studio":
		return GeminiTierAIStudioFree
	default:
		// API Key accounts (type=apikey) have empty oauth_type and are treated as AI Studio.
		return GeminiTierAIStudioFree
REDACTED
REDACTED

func geminiModelClassFromName(model string) geminiModelClass {
	name := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(name, "flash") || strings.Contains(name, "lite") {
		return geminiModelFlash
REDACTED
	return geminiModelPro
REDACTED

func geminiAggregateUsage(stats []usagestats.ModelStat) GeminiUsageTotals {
	var totals GeminiUsageTotals
	for _, stat := range stats {
		switch geminiModelClassFromName(stat.Model) {
		case geminiModelFlash:
			totals.FlashRequests += stat.Requests
			totals.FlashTokens += stat.TotalTokens
			totals.FlashCost += stat.ActualCost
		default:
			totals.ProRequests += stat.Requests
			totals.ProTokens += stat.TotalTokens
			totals.ProCost += stat.ActualCost
	REDACTED
REDACTED
	return totals
REDACTED

func geminiQuotaLocation() *time.Location {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return time.FixedZone("PST", -8*3600)
REDACTED
	return loc
REDACTED

func geminiDailyWindowStart(now time.Time) time.Time {
	loc := geminiQuotaLocation()
	localNow := now.In(loc)
	return time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
REDACTED

func geminiDailyResetTime(now time.Time) time.Time {
	loc := geminiQuotaLocation()
	localNow := now.In(loc)
	start := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	reset := start.Add(24 * time.Hour)
	if !reset.After(localNow) {
		reset = reset.Add(24 * time.Hour)
REDACTED
	return reset
REDACTED
