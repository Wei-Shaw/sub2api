package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"golang.org/x/sync/singleflight"
)

// cachedVersionBounds 缓存 Claude Code 版本号上下限（进程内缓存，60s TTL）
type cachedVersionBounds struct {
	min       string // 空字符串 = 不检查
	max       string // 空字符串 = 不检查
	expiresAt int64  // unix nano
REDACTED

// versionBoundsCache 版本号上下限进程内缓存
var versionBoundsCache atomic.Value // *cachedVersionBounds

// versionBoundsSF 防止缓存过期时 thundering herd
var versionBoundsSF singleflight.Group

// versionBoundsCacheTTL 缓存有效期
const versionBoundsCacheTTL = 60 * time.Second

// versionBoundsErrorTTL DB 错误时的短缓存，快速重试
const versionBoundsErrorTTL = 5 * time.Second

// versionBoundsDBTimeout singleflight 内 DB 查询超时，独立于请求 context
const versionBoundsDBTimeout = 5 * time.Second

// cachedBackendMode Backend Mode cache (in-process, 60s TTL)
type cachedBackendMode struct {
	value     bool
	expiresAt int64 // unix nano
REDACTED

var backendModeCache atomic.Value // *cachedBackendMode
var backendModeSF singleflight.Group

const backendModeCacheTTL = 60 * time.Second
const backendModeErrorTTL = 5 * time.Second
const backendModeDBTimeout = 5 * time.Second

// cachedGatewayForwardingSettings 缓存网关转发行为设置（进程内缓存，60s TTL）
type cachedGatewayForwardingSettings struct {
	fingerprintUnification           bool
	metadataPassthrough              bool
	cchSigning                       bool
	claudeOAuthSystemPromptInjection bool
	claudeOAuthSystemPrompt          string
	claudeOAuthSystemPromptBlocks    string
	anthropicCacheTTL1hInjection     bool
	rewriteMessageCacheControl       bool
	clientDatelineNormalization      bool
	expiresAt                        int64 // unix nano
REDACTED

var gatewayForwardingCache atomic.Value // *cachedGatewayForwardingSettings
var gatewayForwardingSF singleflight.Group

const gatewayForwardingCacheTTL = 60 * time.Second
const gatewayForwardingErrorTTL = 5 * time.Second
const gatewayForwardingDBTimeout = 5 * time.Second

// cachedAntigravityUserAgentVersion 缓存 Antigravity UA 版本号（进程内缓存，60s TTL）
type cachedAntigravityUserAgentVersion struct {
	version   string
	expiresAt int64 // unix nano
REDACTED

const antigravityUserAgentVersionCacheTTL = 60 * time.Second
const antigravityUserAgentVersionErrorTTL = 5 * time.Second
const antigravityUserAgentVersionDBTimeout = 5 * time.Second

// DefaultOpenAICodexUserAgent OpenAI Codex 默认 User-Agent（用于规避 Cloudflare 对浏览器 UA 的质询）
const DefaultOpenAICodexUserAgent = "codex-tui/0.125.0 (Ubuntu 22.4.0; x86_64) xterm-256color (codex-tui; 0.125.0)"

// cachedOpenAICodexUserAgent 缓存 OpenAI Codex UA（进程内缓存，60s TTL）
type cachedOpenAICodexUserAgent struct {
	value     string
	expiresAt int64 // unix nano
REDACTED

type cachedOpenAIQuotaAutoPauseSettings struct {
	settings  OpsOpenAIAccountQuotaAutoPauseSettings
	expiresAt int64
REDACTED

const openAICodexUserAgentCacheTTL = 60 * time.Second
const openAICodexUserAgentErrorTTL = 5 * time.Second
const openAICodexUserAgentDBTimeout = 5 * time.Second

const codexRestrictionPolicyCacheTTL = 60 * time.Second
const codexRestrictionPolicyDBTimeout = 5 * time.Second

// cachedCodexRestrictionPolicy codex_cli_only 全局加固策略缓存（进程内，60s TTL）。
// GetCodexRestrictionPolicy 在每个 codex_cli_only 账号的网关请求热路径上被调用，避免每次访问 DB。
type cachedCodexRestrictionPolicy struct {
	value     CodexRestrictionPolicy
	expiresAt int64 // unix nano
REDACTED

// cachedCyberSessionBlockRuntime cyber 会话屏蔽开关+TTL 进程内缓存（60s TTL）。
// GetCyberSessionBlockRuntime 在网关请求热路径上被调用，避免每次访问 DB。
type cachedCyberSessionBlockRuntime struct {
	enabled   bool
	ttl       time.Duration
	expiresAt int64 // unix nano
REDACTED

const cyberSessionBlockRuntimeCacheTTL = 60 * time.Second
const cyberSessionBlockRuntimeErrorTTL = 5 * time.Second
const cyberSessionBlockRuntimeDBTimeout = 5 * time.Second

const openAIQuotaAutoPauseSettingsCacheTTL = 60 * time.Second
const openAIQuotaAutoPauseSettingsErrorTTL = 5 * time.Second
const openAIQuotaAutoPauseSettingsDBTimeout = 5 * time.Second

const openAIQuotaAutoPauseSettingsRefreshKey = "openai_quota_auto_pause_settings"

// GetCyberSessionBlockRuntime 返回 (开关, TTL)，进程内缓存 ~60s，
// 供网关热路径读取时避免 DB 往返。
// 两个 setting key 在单次 singleflight 里一起读取，减少 DB 往返。
// 默认值：开关 false，TTL 1h（与粘性会话对齐）。
func (s *SettingService) GetCyberSessionBlockRuntime(ctx context.Context) (bool, time.Duration) {
	if cached, ok := s.cyberSessionBlockRuntimeCache.Load().(*cachedCyberSessionBlockRuntime); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.enabled, cached.ttl
	REDACTED
REDACTED
	result, _, _ := s.cyberSessionBlockRuntimeSF.Do("cyber_session_block_runtime", func() (any, error) {
		if cached, ok := s.cyberSessionBlockRuntimeCache.Load().(*cachedCyberSessionBlockRuntime); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached, nil
		REDACTED
	REDACTED
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cyberSessionBlockRuntimeDBTimeout)
		defer cancel()

		enabledVal, enabledErr := s.settingRepo.GetValue(dbCtx, SettingKeyCyberSessionBlockEnabled)
		ttlVal, ttlErr := s.settingRepo.GetValue(dbCtx, SettingKeyCyberSessionBlockTTLSeconds)

		if enabledErr != nil && !errors.Is(enabledErr, ErrSettingNotFound) {
			slog.Warn("failed to get cyber_session_block_enabled setting", "error", enabledErr)
			entry := &cachedCyberSessionBlockRuntime{
				enabled:   false,
				ttl:       time.Hour,
				expiresAt: time.Now().Add(cyberSessionBlockRuntimeErrorTTL).UnixNano(),
		REDACTED
			s.cyberSessionBlockRuntimeCache.Store(entry)
			return entry, nil
	REDACTED

		enabled := enabledErr == nil && strings.TrimSpace(enabledVal) == "true"

		ttl := time.Hour
		if ttlErr == nil {
			if n, perr := strconv.Atoi(strings.TrimSpace(ttlVal)); perr == nil && n > 0 {
				ttl = time.Duration(n) * time.Second
		REDACTED
	REDACTED

		entry := &cachedCyberSessionBlockRuntime{
			enabled:   enabled,
			ttl:       ttl,
			expiresAt: time.Now().Add(cyberSessionBlockRuntimeCacheTTL).UnixNano(),
	REDACTED
		s.cyberSessionBlockRuntimeCache.Store(entry)
		return entry, nil
REDACTED)
	if entry, ok := result.(*cachedCyberSessionBlockRuntime); ok && entry != nil {
		return entry.enabled, entry.ttl
REDACTED
	return false, time.Hour
REDACTED

// GetAntigravityUserAgentVersion 返回 Antigravity 上游请求使用的版本号。
// 后台设置优先；为空、缺失或非法时回退到 ANTIGRAVITY_USER_AGENT_VERSION / 内置默认值。
func (s *SettingService) GetAntigravityUserAgentVersion(ctx context.Context) string {
	fallback := antigravity.GetDefaultUserAgentVersion()
	if s == nil || s.settingRepo == nil {
		return fallback
REDACTED
	if cached, ok := s.antigravityUAVersionCache.Load().(*cachedAntigravityUserAgentVersion); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.version
	REDACTED
REDACTED

	result, _, _ := s.antigravityUAVersionSF.Do("antigravity_user_agent_version", func() (any, error) {
		if cached, ok := s.antigravityUAVersionCache.Load().(*cachedAntigravityUserAgentVersion); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.version, nil
		REDACTED
	REDACTED
		if ctx == nil {
			ctx = context.Background()
	REDACTED
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), antigravityUserAgentVersionDBTimeout)
		defer cancel()
		value, err := s.settingRepo.GetValue(dbCtx, SettingKeyAntigravityUserAgentVersion)
		if err != nil && !errors.Is(err, ErrSettingNotFound) {
			slog.Warn("failed to get antigravity user agent version setting", "error", err)
			s.antigravityUAVersionCache.Store(&cachedAntigravityUserAgentVersion{
				version:   fallback,
				expiresAt: time.Now().Add(antigravityUserAgentVersionErrorTTL).UnixNano(),
		REDACTED)
			return fallback, nil
	REDACTED
		version := antigravity.NormalizeUserAgentVersion(value)
		if version == "" {
			version = fallback
	REDACTED
		s.antigravityUAVersionCache.Store(&cachedAntigravityUserAgentVersion{
			version:   version,
			expiresAt: time.Now().Add(antigravityUserAgentVersionCacheTTL).UnixNano(),
	REDACTED)
		return version, nil
REDACTED)
	if version, ok := result.(string); ok && version != "" {
		return version
REDACTED
	return fallback
REDACTED

// GetOpenAICodexUserAgent 返回 OpenAI Codex 上游请求使用的 User-Agent。
// 后台设置优先；为空时回退到内置默认值。
func (s *SettingService) GetOpenAICodexUserAgent(ctx context.Context) string {
	fallback := DefaultOpenAICodexUserAgent
	if s == nil || s.settingRepo == nil {
		return fallback
REDACTED
	if cached, ok := s.openAICodexUACache.Load().(*cachedOpenAICodexUserAgent); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.value
	REDACTED
REDACTED

	result, _, _ := s.openAICodexUASF.Do("openai_codex_user_agent", func() (any, error) {
		if cached, ok := s.openAICodexUACache.Load().(*cachedOpenAICodexUserAgent); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.value, nil
		REDACTED
	REDACTED
		if ctx == nil {
			ctx = context.Background()
	REDACTED
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAICodexUserAgentDBTimeout)
		defer cancel()
		value, err := s.settingRepo.GetValue(dbCtx, SettingKeyOpenAICodexUserAgent)
		if err != nil && !errors.Is(err, ErrSettingNotFound) {
			slog.Warn("failed to get openai codex user agent setting", "error", err)
			s.openAICodexUACache.Store(&cachedOpenAICodexUserAgent{
				value:     fallback,
				expiresAt: time.Now().Add(openAICodexUserAgentErrorTTL).UnixNano(),
		REDACTED)
			return fallback, nil
	REDACTED
		ua := strings.TrimSpace(value)
		if ua == "" {
			ua = fallback
	REDACTED
		s.openAICodexUACache.Store(&cachedOpenAICodexUserAgent{
			value:     ua,
			expiresAt: time.Now().Add(openAICodexUserAgentCacheTTL).UnixNano(),
	REDACTED)
		return ua, nil
REDACTED)
	if ua, ok := result.(string); ok && ua != "" {
		return ua
REDACTED
	return fallback
REDACTED

var legacyClaudeCodeCodexWhitelistEntry = openai.AllowedClientEntry{
	Originator: "Claude Code",
	UAContains: []string{"Claude Code/"REDACTED,
REDACTED

// MigrateOpenAIAllowClaudeCodeCodexPluginSetting folds the deprecated global Claude Code
// plugin allow switch into codex_cli_only_whitelist. The app-server identity model is the
// same originator + UA marker pair, so runtime checks no longer need a separate flag.
func (s *SettingService) MigrateOpenAIAllowClaudeCodeCodexPluginSetting(ctx context.Context) error {
	if s == nil || s.settingRepo == nil {
		return nil
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), codexRestrictionPolicyDBTimeout)
	defer cancel()

	legacyValue, err := s.settingRepo.GetValue(dbCtx, SettingKeyOpenAIAllowClaudeCodeCodexPlugin)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return nil
	REDACTED
		return fmt.Errorf("get deprecated %s setting: %w", SettingKeyOpenAIAllowClaudeCodeCodexPlugin, err)
REDACTED
	if strings.TrimSpace(legacyValue) != "true" {
		return nil
REDACTED

	rawWhitelist, err := s.settingRepo.GetValue(dbCtx, SettingKeyCodexCLIOnlyWhitelist)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return fmt.Errorf("get %s setting: %w", SettingKeyCodexCLIOnlyWhitelist, err)
REDACTED

	var entries []openai.AllowedClientEntry
	if strings.TrimSpace(rawWhitelist) != "" {
		if err := json.Unmarshal([]byte(rawWhitelist), &entries); err != nil {
			return fmt.Errorf("parse %s setting: %w", SettingKeyCodexCLIOnlyWhitelist, err)
	REDACTED
REDACTED
	if codexClientEntriesContain(entries, legacyClaudeCodeCodexWhitelistEntry) {
		return nil
REDACTED

	entries = append(entries, legacyClaudeCodeCodexWhitelistEntry)
	encoded, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshal %s setting: %w", SettingKeyCodexCLIOnlyWhitelist, err)
REDACTED
	if err := s.settingRepo.Set(dbCtx, SettingKeyCodexCLIOnlyWhitelist, string(encoded)); err != nil {
		return fmt.Errorf("set %s setting: %w", SettingKeyCodexCLIOnlyWhitelist, err)
REDACTED
	s.codexRestrictionPolicySF.Forget("codex_restriction_policy")
	s.codexRestrictionPolicyCache.Store(&cachedCodexRestrictionPolicy{expiresAt: 0REDACTED)
	return nil
REDACTED

// MigrateCodexBodyFingerprintToSignals 把已废弃的 codex_cli_only_allow_body_engine_fingerprint
// 开关并入引擎指纹信号列表。幂等:信号键已存在(非空)则不动;缺失时写默认种子,
// 并把 body 路径行的 Required 设为旧 body 开关的值(旧 true ⇒ 勾上 body 行)。
func (s *SettingService) MigrateCodexBodyFingerprintToSignals(ctx context.Context) error {
	if s == nil || s.settingRepo == nil {
		return nil
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), codexRestrictionPolicyDBTimeout)
	defer cancel()

	if v, err := s.settingRepo.GetValue(dbCtx, SettingKeyCodexCLIOnlyEngineFingerprintSignals); err == nil && strings.TrimSpace(v) != "" {
		return nil // 已配置/已迁移
REDACTED else if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return fmt.Errorf("get %s setting: %w", SettingKeyCodexCLIOnlyEngineFingerprintSignals, err)
REDACTED

	bodyOn := false
	if v, err := s.settingRepo.GetValue(dbCtx, SettingKeyCodexCLIOnlyAllowBodyEngineFingerprint); err == nil {
		bodyOn = strings.TrimSpace(v) == "true"
REDACTED else if !errors.Is(err, ErrSettingNotFound) {
		return fmt.Errorf("get deprecated %s setting: %w", SettingKeyCodexCLIOnlyAllowBodyEngineFingerprint, err)
REDACTED

	seed := make([]openai.EngineFingerprintSignal, len(openai.DefaultEngineFingerprintSignals))
	copy(seed, openai.DefaultEngineFingerprintSignals)
	if bodyOn {
		for i := range seed {
			if seed[i].Type == openai.FingerprintSignalBodyPath {
				seed[i].Required = true
		REDACTED
	REDACTED
REDACTED
	encoded, err := json.Marshal(seed)
	if err != nil {
		return fmt.Errorf("marshal %s setting: %w", SettingKeyCodexCLIOnlyEngineFingerprintSignals, err)
REDACTED
	if err := s.settingRepo.Set(dbCtx, SettingKeyCodexCLIOnlyEngineFingerprintSignals, string(encoded)); err != nil {
		return fmt.Errorf("set %s setting: %w", SettingKeyCodexCLIOnlyEngineFingerprintSignals, err)
REDACTED
	s.codexRestrictionPolicySF.Forget("codex_restriction_policy")
	s.codexRestrictionPolicyCache.Store(&cachedCodexRestrictionPolicy{expiresAt: 0REDACTED)
	return nil
REDACTED

func codexClientEntriesContain(entries []openai.AllowedClientEntry, want openai.AllowedClientEntry) bool {
	wantOriginator := strings.TrimSpace(want.Originator)
	if wantOriginator == "" {
		return false
REDACTED
	wantMarkers := normalizedCodexClientMarkers(want.UAContains)
	if len(wantMarkers) == 0 {
		return false
REDACTED
	for _, entry := range entries {
		if !strings.EqualFold(strings.TrimSpace(entry.Originator), wantOriginator) {
			continue
	REDACTED
		gotMarkers := normalizedCodexClientMarkers(entry.UAContains)
		if len(gotMarkers) != len(wantMarkers) {
			continue
	REDACTED
		matched := true
		for marker := range wantMarkers {
			if _, ok := gotMarkers[marker]; !ok {
				matched = false
				break
		REDACTED
	REDACTED
		if matched {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func normalizedCodexClientMarkers(markers []string) map[string]struct{REDACTED {
	normalized := make(map[string]struct{REDACTED, len(markers))
	for _, marker := range markers {
		marker = strings.TrimSpace(marker)
		if marker == "" {
			continue
	REDACTED
		normalized[strings.ToLower(marker)] = struct{REDACTED{REDACTED
REDACTED
	return normalized
REDACTED

// GetCodexRestrictionPolicy 读取 codex_cli_only 全局加固策略（黑/白名单、最低版本、引擎指纹门）。
// 仅在调用方已确认账号 codex_cli_only 开启时读取；进程内 atomic.Value 缓存（60s TTL）避免热路径访问 DB。
// 任意键缺失/解析失败 → 安全默认：空名单、空版本、默认种子指纹信号。
func (s *SettingService) GetCodexRestrictionPolicy(ctx context.Context) CodexRestrictionPolicy {
	if cached, ok := s.codexRestrictionPolicyCache.Load().(*cachedCodexRestrictionPolicy); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.value
	REDACTED
REDACTED
	result, _, _ := s.codexRestrictionPolicySF.Do("codex_restriction_policy", func() (any, error) {
		if cached, ok := s.codexRestrictionPolicyCache.Load().(*cachedCodexRestrictionPolicy); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.value, nil
		REDACTED
	REDACTED
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), codexRestrictionPolicyDBTimeout)
		defer cancel()

		pol := CodexRestrictionPolicy{EngineFingerprintSignals: openai.DefaultEngineFingerprintSignalsREDACTED // 安全默认：默认种子指纹信号
		if v, err := s.settingRepo.GetValue(dbCtx, SettingKeyMinCodexVersion); err == nil {
			pol.MinCodexVersion = strings.TrimSpace(v)
	REDACTED
		if v, err := s.settingRepo.GetValue(dbCtx, SettingKeyMaxCodexVersion); err == nil {
			pol.MaxCodexVersion = strings.TrimSpace(v)
	REDACTED
		if v, err := s.settingRepo.GetValue(dbCtx, SettingKeyCodexCLIOnlyAllowAppServerClients); err == nil {
			pol.AllowAppServerClients = strings.TrimSpace(v) == "true" // 仅显式 "true" 开启
	REDACTED
		pol.EngineFingerprintSignals = s.loadEngineFingerprintSignals(dbCtx)
		pol.Whitelist = s.loadCodexClientEntries(dbCtx, SettingKeyCodexCLIOnlyWhitelist)
		pol.Blacklist = s.loadCodexClientEntries(dbCtx, SettingKeyCodexCLIOnlyBlacklist)

		s.codexRestrictionPolicyCache.Store(&cachedCodexRestrictionPolicy{
			value:     pol,
			expiresAt: time.Now().Add(codexRestrictionPolicyCacheTTL).UnixNano(),
	REDACTED)
		return pol, nil
REDACTED)
	if pol, ok := result.(CodexRestrictionPolicy); ok {
		return pol
REDACTED
	return CodexRestrictionPolicy{EngineFingerprintSignals: openai.DefaultEngineFingerprintSignalsREDACTED
REDACTED

// loadCodexClientEntries 读取并解析 []openai.AllowedClientEntry JSON 设置；缺失/空/非法 → nil（安全忽略）。
func (s *SettingService) loadCodexClientEntries(ctx context.Context, key string) []openai.AllowedClientEntry {
	v, err := s.settingRepo.GetValue(ctx, key)
	if err != nil || strings.TrimSpace(v) == "" {
		return nil
REDACTED
	var entries []openai.AllowedClientEntry
	if json.Unmarshal([]byte(v), &entries) != nil {
		return nil
REDACTED
	return entries
REDACTED

// loadEngineFingerprintSignals 读取引擎指纹信号列表;缺失/空/非法 → 默认种子。
func (s *SettingService) loadEngineFingerprintSignals(ctx context.Context) []openai.EngineFingerprintSignal {
	v, err := s.settingRepo.GetValue(ctx, SettingKeyCodexCLIOnlyEngineFingerprintSignals)
	if err != nil || strings.TrimSpace(v) == "" {
		return openai.DefaultEngineFingerprintSignals
REDACTED
	sigs, ok := openai.ParseEngineFingerprintSignals(v)
	if !ok {
		return openai.DefaultEngineFingerprintSignals
REDACTED
	return sigs
REDACTED

// ValidateCodexClientEntriesJSON 校验 codex_cli_only 名单 JSON 配置（黑名单语义）：
// 空=合法（禁用）；非空须为 []AllowedClientEntry 的 JSON 数组。黑名单是 OR 宽 deny，
// 允许 originator-only 条目，故不校验 ua_contains。白名单请用 ValidateCodexWhitelistEntriesJSON。
func ValidateCodexClientEntriesJSON(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
REDACTED
	var entries []openai.AllowedClientEntry
	if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
		return fmt.Errorf("must be empty or a valid JSON array of {originator, ua_containsREDACTED")
REDACTED
	return nil
REDACTED

// ValidateCodexWhitelistEntriesJSON 在 ValidateCodexClientEntriesJSON 的数组结构校验之上，额外要求
// 每条白名单条目「有可能命中」（openai.AllowedClientEntry.IsWhitelistable）。白名单是双因子 AND：
// originator-only、空或含空白 ua_contains 的条目会在运行时静默失效——这里让管理员在写入时即收到反馈，
// 而非存入永不命中的死规则。黑名单（OR 宽 deny）仍用 ValidateCodexClientEntriesJSON。
func ValidateCodexWhitelistEntriesJSON(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
REDACTED
	var entries []openai.AllowedClientEntry
	if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
		return fmt.Errorf("must be empty or a valid JSON array of {originator, ua_containsREDACTED")
REDACTED
	for i, e := range entries {
		if !e.IsWhitelistable() {
			return fmt.Errorf("entry %d: whitelist requires a non-empty originator and at least one non-empty ua_contains (double-factor AND; otherwise the rule never matches)", i)
	REDACTED
REDACTED
	return nil
REDACTED

// ValidateEngineFingerprintSignalsJSON 服务层包装,复用 openai 校验逻辑。
func ValidateEngineFingerprintSignalsJSON(raw string) error {
	return openai.ValidateEngineFingerprintSignalsJSON(raw)
REDACTED

// IsBackendModeEnabled checks if backend mode is enabled
// Uses in-process atomic.Value cache with 60s TTL, zero-lock hot path
func (s *SettingService) IsBackendModeEnabled(ctx context.Context) bool {
	if cached, ok := backendModeCache.Load().(*cachedBackendMode); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.value
	REDACTED
REDACTED
	result, _, _ := backendModeSF.Do("backend_mode", func() (any, error) {
		if cached, ok := backendModeCache.Load().(*cachedBackendMode); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.value, nil
		REDACTED
	REDACTED
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backendModeDBTimeout)
		defer cancel()
		value, err := s.settingRepo.GetValue(dbCtx, SettingKeyBackendModeEnabled)
		if err != nil {
			if errors.Is(err, ErrSettingNotFound) {
				// Setting not yet created (fresh install) - default to disabled with full TTL
				backendModeCache.Store(&cachedBackendMode{
					value:     false,
					expiresAt: time.Now().Add(backendModeCacheTTL).UnixNano(),
			REDACTED)
				return false, nil
		REDACTED
			slog.Warn("failed to get backend_mode_enabled setting", "error", err)
			backendModeCache.Store(&cachedBackendMode{
				value:     false,
				expiresAt: time.Now().Add(backendModeErrorTTL).UnixNano(),
		REDACTED)
			return false, nil
	REDACTED
		enabled := value == "true"
		backendModeCache.Store(&cachedBackendMode{
			value:     enabled,
			expiresAt: time.Now().Add(backendModeCacheTTL).UnixNano(),
	REDACTED)
		return enabled, nil
REDACTED)
	if val, ok := result.(bool); ok {
		return val
REDACTED
	return false
REDACTED

type gatewayForwardingSettingsResult struct {
	fp, mp, cch, claudeOAuthSystemPromptInjection, cacheTTL1h, rewriteMessageCacheControl bool
	clientDatelineNormalization                                                           bool
	claudeOAuthSystemPrompt, claudeOAuthSystemPromptBlocks                                string
REDACTED

func (s *SettingService) getGatewayForwardingSettingsCached(ctx context.Context) gatewayForwardingSettingsResult {
	if cached, ok := gatewayForwardingCache.Load().(*cachedGatewayForwardingSettings); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return gatewayForwardingSettingsResult{
				fp:                               cached.fingerprintUnification,
				mp:                               cached.metadataPassthrough,
				cch:                              cached.cchSigning,
				claudeOAuthSystemPromptInjection: cached.claudeOAuthSystemPromptInjection,
				claudeOAuthSystemPrompt:          cached.claudeOAuthSystemPrompt,
				claudeOAuthSystemPromptBlocks:    cached.claudeOAuthSystemPromptBlocks,
				cacheTTL1h:                       cached.anthropicCacheTTL1hInjection,
				rewriteMessageCacheControl:       cached.rewriteMessageCacheControl,
				clientDatelineNormalization:      cached.clientDatelineNormalization,
		REDACTED
	REDACTED
REDACTED
	val, _, _ := gatewayForwardingSF.Do("gateway_forwarding", func() (any, error) {
		if cached, ok := gatewayForwardingCache.Load().(*cachedGatewayForwardingSettings); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return gatewayForwardingSettingsResult{
					fp:                               cached.fingerprintUnification,
					mp:                               cached.metadataPassthrough,
					cch:                              cached.cchSigning,
					claudeOAuthSystemPromptInjection: cached.claudeOAuthSystemPromptInjection,
					claudeOAuthSystemPrompt:          cached.claudeOAuthSystemPrompt,
					claudeOAuthSystemPromptBlocks:    cached.claudeOAuthSystemPromptBlocks,
					cacheTTL1h:                       cached.anthropicCacheTTL1hInjection,
					rewriteMessageCacheControl:       cached.rewriteMessageCacheControl,
					clientDatelineNormalization:      cached.clientDatelineNormalization,
			REDACTED, nil
		REDACTED
	REDACTED
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), gatewayForwardingDBTimeout)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, []string{
			SettingKeyEnableFingerprintUnification,
			SettingKeyEnableMetadataPassthrough,
			SettingKeyEnableCCHSigning,
			SettingKeyEnableClaudeOAuthSystemPromptInjection,
			SettingKeyClaudeOAuthSystemPrompt,
			SettingKeyClaudeOAuthSystemPromptBlocks,
			SettingKeyEnableAnthropicCacheTTL1hInjection,
			SettingKeyRewriteMessageCacheControl,
			SettingKeyEnableClientDatelineNormalization,
	REDACTED)
		if err != nil {
			slog.Warn("failed to get gateway forwarding settings", "error", err)
			gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
				fingerprintUnification:           true,
				metadataPassthrough:              false,
				cchSigning:                       false,
				claudeOAuthSystemPromptInjection: true,
				anthropicCacheTTL1hInjection:     false,
				rewriteMessageCacheControl:       s.defaultRewriteMessageCacheControl(),
				clientDatelineNormalization:      true,
				expiresAt:                        time.Now().Add(gatewayForwardingErrorTTL).UnixNano(),
		REDACTED)
			return gatewayForwardingSettingsResult{fp: true, claudeOAuthSystemPromptInjection: true, rewriteMessageCacheControl: s.defaultRewriteMessageCacheControl(), clientDatelineNormalization: trueREDACTED, nil
	REDACTED
		fp := true
		if v, ok := values[SettingKeyEnableFingerprintUnification]; ok && v != "" {
			fp = v == "true"
	REDACTED
		mp := values[SettingKeyEnableMetadataPassthrough] == "true"
		cch := values[SettingKeyEnableCCHSigning] == "true"
		systemPromptInjection := true
		if v, ok := values[SettingKeyEnableClaudeOAuthSystemPromptInjection]; ok && v != "" {
			systemPromptInjection = v == "true"
	REDACTED
		systemPrompt := values[SettingKeyClaudeOAuthSystemPrompt]
		systemPromptBlocks := values[SettingKeyClaudeOAuthSystemPromptBlocks]
		cacheTTL1h := values[SettingKeyEnableAnthropicCacheTTL1hInjection] == "true"
		rewriteMessageCacheControl := s.defaultRewriteMessageCacheControl()
		if v, ok := values[SettingKeyRewriteMessageCacheControl]; ok && v != "" {
			rewriteMessageCacheControl = v == "true"
	REDACTED
		clientDatelineNormalization := true
		if v, ok := values[SettingKeyEnableClientDatelineNormalization]; ok && v != "" {
			clientDatelineNormalization = v == "true"
	REDACTED
		gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
			fingerprintUnification:           fp,
			metadataPassthrough:              mp,
			cchSigning:                       cch,
			claudeOAuthSystemPromptInjection: systemPromptInjection,
			claudeOAuthSystemPrompt:          systemPrompt,
			claudeOAuthSystemPromptBlocks:    systemPromptBlocks,
			anthropicCacheTTL1hInjection:     cacheTTL1h,
			rewriteMessageCacheControl:       rewriteMessageCacheControl,
			clientDatelineNormalization:      clientDatelineNormalization,
			expiresAt:                        time.Now().Add(gatewayForwardingCacheTTL).UnixNano(),
	REDACTED)
		return gatewayForwardingSettingsResult{
			fp:                               fp,
			mp:                               mp,
			cch:                              cch,
			claudeOAuthSystemPromptInjection: systemPromptInjection,
			claudeOAuthSystemPrompt:          systemPrompt,
			claudeOAuthSystemPromptBlocks:    systemPromptBlocks,
			cacheTTL1h:                       cacheTTL1h,
			rewriteMessageCacheControl:       rewriteMessageCacheControl,
			clientDatelineNormalization:      clientDatelineNormalization,
	REDACTED, nil
REDACTED)
	if r, ok := val.(gatewayForwardingSettingsResult); ok {
		return r
REDACTED
	return gatewayForwardingSettingsResult{fp: true, claudeOAuthSystemPromptInjection: true, clientDatelineNormalization: trueREDACTED
REDACTED

// GetGatewayForwardingSettings returns cached gateway forwarding settings.
// Uses in-process atomic.Value cache with 60s TTL, zero-lock hot path.
// Returns (fingerprintUnification, metadataPassthrough, cchSigning).
func (s *SettingService) GetGatewayForwardingSettings(ctx context.Context) (fingerprintUnification, metadataPassthrough, cchSigning bool) {
	result := s.getGatewayForwardingSettingsCached(ctx)
	return result.fp, result.mp, result.cch
REDACTED

// IsAnthropicCacheTTL1hInjectionEnabled 检查是否对 Anthropic OAuth/SetupToken 请求体注入 1h cache_control ttl。
func (s *SettingService) IsAnthropicCacheTTL1hInjectionEnabled(ctx context.Context) bool {
	return s.getGatewayForwardingSettingsCached(ctx).cacheTTL1h
REDACTED

// IsRewriteMessageCacheControlEnabled 检查是否启用 messages cache_control 改写。
func (s *SettingService) IsRewriteMessageCacheControlEnabled(ctx context.Context) bool {
	return s.getGatewayForwardingSettingsCached(ctx).rewriteMessageCacheControl
REDACTED

// IsClientDatelineNormalizationEnabled 检查是否启用 Anthropic OAuth/SetupToken 请求体
// 的客户端 dateline 归一化。默认开启。
func (s *SettingService) IsClientDatelineNormalizationEnabled(ctx context.Context) bool {
	return s.getGatewayForwardingSettingsCached(ctx).clientDatelineNormalization
REDACTED

// GetClaudeOAuthSystemPromptInjectionSettings returns the Claude OAuth mimic
// system block switch, legacy custom expansion prompt, and configurable blocks JSON.
// Empty values mean use the built-in Claude Code default blocks.
func (s *SettingService) GetClaudeOAuthSystemPromptInjectionSettings(ctx context.Context) (enabled bool, prompt string, blocks string) {
	result := s.getGatewayForwardingSettingsCached(ctx)
	return result.claudeOAuthSystemPromptInjection, result.claudeOAuthSystemPrompt, result.claudeOAuthSystemPromptBlocks
REDACTED

// GetClaudeCodeVersionBounds 获取 Claude Code 版本号上下限要求
// 使用进程内 atomic.Value 缓存，60 秒 TTL，热路径零锁开销
// singleflight 防止缓存过期时 thundering herd
// 返回空字符串表示不做对应方向的版本检查
func (s *SettingService) GetClaudeCodeVersionBounds(ctx context.Context) (min, max string) {
	if cached, ok := versionBoundsCache.Load().(*cachedVersionBounds); ok {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.min, cached.max
	REDACTED
REDACTED
	// singleflight: 同一时刻只有一个 goroutine 查询 DB，其余复用结果
	type bounds struct{ min, max string REDACTED
	result, err, _ := versionBoundsSF.Do("version_bounds", func() (any, error) {
		// 二次检查，避免排队的 goroutine 重复查询
		if cached, ok := versionBoundsCache.Load().(*cachedVersionBounds); ok {
			if time.Now().UnixNano() < cached.expiresAt {
				return bounds{cached.min, cached.maxREDACTED, nil
		REDACTED
	REDACTED
		// 使用独立 context：断开请求取消链，避免客户端断连导致空值被长期缓存
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), versionBoundsDBTimeout)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, []string{
			SettingKeyMinClaudeCodeVersion,
			SettingKeyMaxClaudeCodeVersion,
	REDACTED)
		if err != nil {
			// fail-open: DB 错误时不阻塞请求，但记录日志并使用短 TTL 快速重试
			slog.Warn("failed to get claude code version bounds setting, skipping version check", "error", err)
			versionBoundsCache.Store(&cachedVersionBounds{
				min:       "",
				max:       "",
				expiresAt: time.Now().Add(versionBoundsErrorTTL).UnixNano(),
		REDACTED)
			return bounds{"", ""REDACTED, nil
	REDACTED
		b := bounds{
			min: values[SettingKeyMinClaudeCodeVersion],
			max: values[SettingKeyMaxClaudeCodeVersion],
	REDACTED
		versionBoundsCache.Store(&cachedVersionBounds{
			min:       b.min,
			max:       b.max,
			expiresAt: time.Now().Add(versionBoundsCacheTTL).UnixNano(),
	REDACTED)
		return b, nil
REDACTED)
	if err != nil {
		return "", ""
REDACTED
	b, ok := result.(bounds)
	if !ok {
		return "", ""
REDACTED
	return b.min, b.max
REDACTED

// GetOpenAIQuotaAutoPauseSettings returns the current global default quota auto-pause
// settings. It is invoked on the OpenAI scheduling hot path (once per request) and is
// therefore designed to never block on the DB:
//
//   - Fresh cached value → returned immediately.
//   - Stale or empty cache → the last known value is returned, and a background
//     goroutine refreshes the cache via singleflight (stale-while-revalidate).
//   - First call with no cache yet → zero defaults are returned and the same async
//     refresh is kicked off; the next call gets the freshly populated value.
//
// Callers that need the freshly persisted value synchronously (tests, post-update
// confirmation, optional startup warm-up) should call WarmOpenAIQuotaAutoPauseSettings.
func (s *SettingService) GetOpenAIQuotaAutoPauseSettings(ctx context.Context) OpsOpenAIAccountQuotaAutoPauseSettings {
	if s == nil {
		return OpsOpenAIAccountQuotaAutoPauseSettings{REDACTED
REDACTED
	cached, _ := s.openAIQuotaAutoPauseSettingsCache.Load().(*cachedOpenAIQuotaAutoPauseSettings)
	now := time.Now().UnixNano()
	if cached != nil && now < cached.expiresAt {
		return cached.settings
REDACTED
	// Stale or unset: trigger background refresh without blocking this request.
	// singleflight.DoChan dedupes concurrent refreshes; we deliberately ignore the
	// returned channel — the result is observable via the atomic cache.
	s.openAIQuotaAutoPauseSettingsSF.DoChan(openAIQuotaAutoPauseSettingsRefreshKey, func() (any, error) {
		s.refreshOpenAIQuotaAutoPauseSettings(context.Background())
		return nil, nil
REDACTED)
	if cached != nil {
		return cached.settings // serve stale value while revalidating
REDACTED
	return OpsOpenAIAccountQuotaAutoPauseSettings{REDACTED
REDACTED

// WarmOpenAIQuotaAutoPauseSettings synchronously loads the quota auto-pause settings
// into the in-memory cache. Useful for application startup (so the first request hits
// a warm cache) and for tests that need deterministic reads immediately after
// constructing the service.
func (s *SettingService) WarmOpenAIQuotaAutoPauseSettings(ctx context.Context) OpsOpenAIAccountQuotaAutoPauseSettings {
	if s == nil {
		return OpsOpenAIAccountQuotaAutoPauseSettings{REDACTED
REDACTED
	s.refreshOpenAIQuotaAutoPauseSettings(ctx)
	cached, _ := s.openAIQuotaAutoPauseSettingsCache.Load().(*cachedOpenAIQuotaAutoPauseSettings)
	if cached == nil {
		return OpsOpenAIAccountQuotaAutoPauseSettings{REDACTED
REDACTED
	return cached.settings
REDACTED

// refreshOpenAIQuotaAutoPauseSettings reads the latest settings from the DB and stores
// them into the in-memory cache. On error it stores the prior value (or zero defaults
// if nothing is cached yet) with the shorter error TTL so the next refresh comes
// sooner. Always uses its own timeout-bounded context to keep refresh latency
// predictable regardless of the caller.
func (s *SettingService) refreshOpenAIQuotaAutoPauseSettings(ctx context.Context) {
	if s == nil || s.settingRepo == nil {
		return
REDACTED
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIQuotaAutoPauseSettingsDBTimeout)
	defer cancel()

	settings := OpsOpenAIAccountQuotaAutoPauseSettings{REDACTED
	ttl := openAIQuotaAutoPauseSettingsCacheTTL
	raw, err := s.settingRepo.GetValue(dbCtx, SettingKeyOpsAdvancedSettings)
	if err == nil {
		cfg := defaultOpsAdvancedSettings()
		if strings.TrimSpace(raw) != "" {
			if jsonErr := json.Unmarshal([]byte(raw), cfg); jsonErr == nil {
				normalizeOpsAdvancedSettings(cfg)
		REDACTED
	REDACTED
		settings = cfg.OpenAIAccountQuotaAutoPause
REDACTED else if !errors.Is(err, ErrSettingNotFound) {
		// Real error: keep serving prior value but refresh sooner.
		if prior, _ := s.openAIQuotaAutoPauseSettingsCache.Load().(*cachedOpenAIQuotaAutoPauseSettings); prior != nil {
			settings = prior.settings
	REDACTED
		ttl = openAIQuotaAutoPauseSettingsErrorTTL
REDACTED

	s.openAIQuotaAutoPauseSettingsCache.Store(&cachedOpenAIQuotaAutoPauseSettings{
		settings:  settings,
		expiresAt: time.Now().Add(ttl).UnixNano(),
REDACTED)
REDACTED

// SetOpenAIQuotaAutoPauseSettings writes the given settings directly into the in-memory
// cache. Called from settings-write code paths so that the next read reflects the new
// value immediately, without waiting for the background refresh.
func (s *SettingService) SetOpenAIQuotaAutoPauseSettings(settings OpsOpenAIAccountQuotaAutoPauseSettings) {
	if s == nil {
		return
REDACTED
	s.openAIQuotaAutoPauseSettingsCache.Store(&cachedOpenAIQuotaAutoPauseSettings{
		settings:  settings,
		expiresAt: time.Now().Add(openAIQuotaAutoPauseSettingsCacheTTL).UnixNano(),
REDACTED)
REDACTED
