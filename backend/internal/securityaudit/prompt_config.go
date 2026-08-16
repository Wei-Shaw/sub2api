package securityaudit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/keywordmatcher"
)

const (
	DefaultWorkerCount     = 4
	MaxWorkerCount         = 32
	DefaultQueueCapacity   = 32768
	MaxQueueCapacity       = 100000
	DefaultTimeoutMS       = 3000
	MinTimeoutMS           = 100
	MaxTimeoutMS           = 30000
	DefaultInputLimit      = 4000
	MinInputLimit          = 128
	MaxInputLimit          = 100000
	DefaultPayloadTTL      = 30 * time.Minute
	MaxBlockedKeywords     = 10000
	MaxBlockedKeywordRunes = 200
)

type SecretEncryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// ConfigStore is the injectable boundary between hot-path prompt auditing and
// the concrete settings/PostgreSQL/Redis-backed configuration manager.
type ConfigStore interface {
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
	Active() (ActiveConfig, bool)
	EffectiveMode() Mode
	// BlockingActivationDegraded is true when storage intent requires blocking
	// but no usable blocking snapshot is active (cold start or failed reload).
	// It must stay false when blocking is not intended, even if config is
	// untrusted—otherwise default-off deployments fail closed for all traffic.
	BlockingActivationDegraded() bool
	Public() (PublicConfig, error)
	Save(ctx context.Context, req UpdateConfigRequest, actorID int64) (PublicConfig, error)
	RuntimeState() (expected int64, active int64, loadedAt *time.Time, loadError string)
	Encrypt(value string) (string, error)
	Decrypt(value string) (string, error)
}

type StorageEndpoint struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Protocol        string `json:"protocol"`
	BaseURL         string `json:"base_url"`
	Model           string `json:"model"`
	TokenCiphertext string `json:"token_ciphertext,omitempty"`
	TimeoutMS       int    `json:"timeout_ms"`
	InputLimit      int    `json:"input_limit"`
	Enabled         bool   `json:"enabled"`
}

type storageConfig struct {
	Enabled bool `json:"enabled"`
	// BlockingEnabled is retained as a compatibility aggregate for older
	// clients. New configurations use the two independent flags below.
	BlockingEnabled        bool              `json:"blocking_enabled"`
	BlockingLatestTurnOnly bool              `json:"blocking_latest_turn_only"`
	KeywordBlockingEnabled bool              `json:"keyword_blocking_enabled"`
	AIBlockingEnabled      bool              `json:"ai_blocking_enabled"`
	StorePassEvents        bool              `json:"store_pass_events"`
	PreHashCheckEnabled    bool              `json:"pre_hash_check_enabled"`
	BlockedKeywords        []string          `json:"blocked_keywords"`
	KeywordBlockingMode    string            `json:"keyword_blocking_mode"`
	Strategy               string            `json:"strategy"`
	WorkerCount            int               `json:"worker_count"`
	QueueCapacity          int               `json:"queue_capacity"`
	Scanners               []string          `json:"scanners"`
	AllGroups              bool              `json:"all_groups"`
	GroupIDs               []int64           `json:"group_ids"`
	ProxyID                *int64            `json:"proxy_id,omitempty"`
	Endpoints              []StorageEndpoint `json:"endpoints"`
	ConfigVersion          int64             `json:"config_version"`
	UpdatedAt              time.Time         `json:"updated_at"`
	UpdatedBy              int64             `json:"updated_by"`
	ChangeSummary          string            `json:"change_summary"`
}

type ActiveEndpoint struct {
	ID         string
	Name       string
	Protocol   string
	BaseURL    string
	Model      string
	Token      string
	TimeoutMS  int
	InputLimit int
	Enabled    bool
	ProxyID    *int64
	// TokenInvalid marks an endpoint whose persisted token ciphertext cannot be
	// decrypted with the current encryption key (key changed or auto-generated
	// on restart). The endpoint is kept visible for admins but excluded from
	// runtime use until the token is re-entered or cleared (issue #4887).
	TokenInvalid bool
}

type ActiveConfig struct {
	RiskControlEnabled bool
	Enabled            bool
	// BlockingEnabled is the legacy aggregate. The effective behavior is driven
	// by KeywordBlockingEnabled and AIBlockingEnabled.
	BlockingEnabled         bool
	BlockingLatestTurnOnly  bool
	KeywordBlockingEnabled  bool
	AIBlockingEnabled       bool
	StorePassEvents         bool
	PreHashCheckEnabled     bool
	BlockedKeywords         []string
	KeywordBlockingMode     string
	Strategy                string
	WorkerCount             int
	QueueCapacity           int
	Scanners                []string
	AllGroups               bool
	GroupIDs                []int64
	ProxyID                 *int64
	Endpoints               []ActiveEndpoint
	ConfigVersion           int64
	UpdatedAt               time.Time
	UpdatedBy               int64
	ChangeSummary           string
	keywordMatcher          *keywordmatcher.Matcher
	blockingFlagsConfigured bool
}

type PublicEndpoint struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Protocol    string `json:"protocol"`
	BaseURL     string `json:"base_url"`
	Model       string `json:"model"`
	TimeoutMS   int    `json:"timeout_ms"`
	InputLimit  int    `json:"input_limit"`
	Enabled     bool   `json:"enabled"`
	HasToken    bool   `json:"has_token"`
	TokenStatus string `json:"token_status"`
}

type PublicConfig struct {
	Enabled                bool             `json:"enabled"`
	BlockingEnabled        bool             `json:"blocking_enabled"`
	BlockingLatestTurnOnly bool             `json:"blocking_latest_turn_only"`
	KeywordBlockingEnabled bool             `json:"keyword_blocking_enabled"`
	AIBlockingEnabled      bool             `json:"ai_blocking_enabled"`
	StorePassEvents        bool             `json:"store_pass_events"`
	PreHashCheckEnabled    bool             `json:"pre_hash_check_enabled"`
	BlockedKeywords        []string         `json:"blocked_keywords"`
	KeywordBlockingMode    string           `json:"keyword_blocking_mode"`
	EffectiveMode          Mode             `json:"effective_mode"`
	Strategy               string           `json:"strategy"`
	WorkerCount            int              `json:"worker_count"`
	QueueCapacity          int              `json:"queue_capacity"`
	Scanners               []string         `json:"scanners"`
	AllGroups              bool             `json:"all_groups"`
	GroupIDs               []int64          `json:"group_ids"`
	ProxyID                *int64           `json:"proxy_id"`
	Endpoints              []PublicEndpoint `json:"endpoints"`
	ConfigVersion          int64            `json:"config_version"`
	UpdatedAt              time.Time        `json:"updated_at"`
	UpdatedBy              int64            `json:"updated_by"`
	ChangeSummary          string           `json:"change_summary"`
}

type UpdateEndpoint struct {
	ID         string `json:"id" binding:"required"`
	Name       string `json:"name" binding:"required"`
	Protocol   string `json:"protocol"`
	BaseURL    string `json:"base_url" binding:"required"`
	Model      string `json:"model"`
	Token      string `json:"token,omitempty"`
	ClearToken bool   `json:"clear_token"`
	TimeoutMS  int    `json:"timeout_ms"`
	InputLimit int    `json:"input_limit"`
	Enabled    bool   `json:"enabled"`
}

type UpdateConfigRequest struct {
	ExpectedConfigVersion  int64            `json:"expected_config_version" binding:"required"`
	Enabled                bool             `json:"enabled"`
	BlockingEnabled        bool             `json:"blocking_enabled"`
	BlockingLatestTurnOnly bool             `json:"blocking_latest_turn_only"`
	KeywordBlockingEnabled *bool            `json:"keyword_blocking_enabled"`
	AIBlockingEnabled      *bool            `json:"ai_blocking_enabled"`
	StorePassEvents        bool             `json:"store_pass_events"`
	PreHashCheckEnabled    bool             `json:"pre_hash_check_enabled"`
	BlockedKeywords        []string         `json:"blocked_keywords"`
	KeywordBlockingMode    string           `json:"keyword_blocking_mode"`
	Strategy               string           `json:"strategy"`
	WorkerCount            int              `json:"worker_count"`
	QueueCapacity          int              `json:"queue_capacity"`
	Scanners               []string         `json:"scanners"`
	AllGroups              bool             `json:"all_groups"`
	GroupIDs               []int64          `json:"group_ids"`
	ProxyID                *int64           `json:"proxy_id"`
	Endpoints              []UpdateEndpoint `json:"endpoints"`
}

func DefaultStorageConfig() storageConfig {
	return storageConfig{
		Enabled:                false,
		BlockingEnabled:        false,
		BlockingLatestTurnOnly: false,
		StorePassEvents:        false,
		PreHashCheckEnabled:    false,
		BlockedKeywords:        []string{},
		KeywordBlockingMode:    PromptKeywordModeAIOnly,
		Strategy:               "priority",
		WorkerCount:            DefaultWorkerCount,
		QueueCapacity:          DefaultQueueCapacity,
		Scanners:               append([]string(nil), AllScannerIDs...),
		AllGroups:              true,
		GroupIDs:               []int64{},
		Endpoints:              []StorageEndpoint{},
		ConfigVersion:          1,
	}
}

func ParseStorageConfig(raw string) (storageConfig, error) {
	cfg := DefaultStorageConfig()
	if strings.TrimSpace(raw) == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return storageConfig{}, fmt.Errorf("decode prompt audit config: %w", err)
	}
	// Older persisted configs only have blocking_enabled. Treat that value as
	// both synchronous switches once, while allowing a new config to explicitly
	// disable either switch without being overwritten by the legacy aggregate.
	var flags struct {
		KeywordBlockingEnabled *bool `json:"keyword_blocking_enabled"`
		AIBlockingEnabled      *bool `json:"ai_blocking_enabled"`
	}
	if err := json.Unmarshal([]byte(raw), &flags); err != nil {
		return storageConfig{}, fmt.Errorf("decode prompt audit flags: %w", err)
	}
	if flags.KeywordBlockingEnabled == nil {
		cfg.KeywordBlockingEnabled = cfg.BlockingEnabled
	}
	if flags.AIBlockingEnabled == nil {
		cfg.AIBlockingEnabled = cfg.BlockingEnabled
	}
	normalizeStorageConfig(&cfg)
	if err := validateStorageConfig(cfg); err != nil {
		return storageConfig{}, err
	}
	return cfg, nil
}

func normalizeStorageConfig(cfg *storageConfig) {
	if cfg == nil {
		return
	}
	if cfg.ConfigVersion < 1 {
		cfg.ConfigVersion = 1
	}
	if strings.TrimSpace(cfg.Strategy) == "" {
		cfg.Strategy = "priority"
	}
	if cfg.WorkerCount == 0 {
		cfg.WorkerCount = DefaultWorkerCount
	}
	if cfg.QueueCapacity == 0 {
		cfg.QueueCapacity = DefaultQueueCapacity
	}
	cfg.KeywordBlockingMode = normalizePromptKeywordBlockingMode(cfg.KeywordBlockingMode)
	cfg.BlockingEnabled = cfg.KeywordBlockingEnabled || cfg.AIBlockingEnabled
	if len(cfg.Scanners) == 0 && !promptKeywordModeSkipsAI(cfg.KeywordBlockingMode) {
		cfg.Scanners = append([]string(nil), AllScannerIDs...)
	}
	cfg.Scanners = canonicalScannerIDs(cfg.Scanners)
	cfg.GroupIDs = canonicalInt64s(cfg.GroupIDs)
	if cfg.ProxyID != nil && *cfg.ProxyID <= 0 {
		cfg.ProxyID = nil
	}
	cfg.BlockedKeywords = keywordmatcher.Normalize(cfg.BlockedKeywords, MaxBlockedKeywords, MaxBlockedKeywordRunes)
	// Preserve an invalid blocking-without-audit combination so validation can
	// reject it instead of silently changing administrator intent.
	for i := range cfg.Endpoints {
		ep := &cfg.Endpoints[i]
		ep.ID = strings.TrimSpace(ep.ID)
		ep.Name = strings.TrimSpace(ep.Name)
		ep.Protocol = strings.TrimSpace(ep.Protocol)
		if ep.Protocol == "" {
			ep.Protocol = ProtocolOpenAICompatible
		}
		ep.BaseURL = strings.TrimSpace(ep.BaseURL)
		ep.Model = normalizeModelForEndpoint(ep.Protocol, ep.BaseURL, ep.Model)
		if ep.TimeoutMS == 0 {
			ep.TimeoutMS = DefaultTimeoutMS
		}
		if ep.InputLimit == 0 {
			ep.InputLimit = DefaultInputLimit
		}
	}
}

func validateStorageConfig(cfg storageConfig) error {
	if (cfg.BlockingEnabled || cfg.KeywordBlockingEnabled || cfg.AIBlockingEnabled) && !cfg.Enabled {
		return infraerrors.BadRequest(ErrorCodeRequiresEnabled, "开启同步阻止前必须先启用提示词审计")
	}
	if cfg.Strategy != "priority" {
		return infraerrors.BadRequest("prompt_audit_invalid_strategy", "提示词审计策略仅支持 priority")
	}
	if cfg.WorkerCount < 1 || cfg.WorkerCount > MaxWorkerCount {
		return infraerrors.BadRequest("prompt_audit_invalid_worker_count", "Worker 数量超出允许范围")
	}
	if cfg.QueueCapacity < 1 || cfg.QueueCapacity > MaxQueueCapacity {
		return infraerrors.BadRequest("prompt_audit_invalid_queue_capacity", "队列容量超出允许范围")
	}
	if !cfg.AllGroups && len(cfg.GroupIDs) == 0 {
		return infraerrors.BadRequest("prompt_audit_groups_required", "指定分组模式至少需要选择一个分组")
	}
	if !promptKeywordModeSkipsAI(cfg.KeywordBlockingMode) && len(cfg.Scanners) == 0 {
		return infraerrors.BadRequest("prompt_audit_scanners_required", "至少需要启用一个风险分类")
	}
	seen := make(map[string]struct{}, len(cfg.Endpoints))
	enabled := 0
	for _, ep := range cfg.Endpoints {
		if ep.ID == "" || ep.Name == "" {
			return infraerrors.BadRequest("prompt_audit_invalid_endpoint", "审计节点 ID 和名称不能为空")
		}
		if _, ok := seen[ep.ID]; ok {
			return infraerrors.BadRequest("prompt_audit_duplicate_endpoint", "审计节点 ID 不能重复")
		}
		seen[ep.ID] = struct{}{}
		if !isPromptAuditProtocol(ep.Protocol) {
			return infraerrors.BadRequest("prompt_audit_invalid_endpoint_protocol", "审计节点协议无效")
		}
		if _, err := NormalizeBaseURL(ep.BaseURL); err != nil {
			return err
		}
		if ep.TimeoutMS < MinTimeoutMS || ep.TimeoutMS > MaxTimeoutMS {
			return infraerrors.BadRequest("prompt_audit_invalid_timeout", "审计节点超时超出允许范围")
		}
		if ep.InputLimit < MinInputLimit || ep.InputLimit > MaxInputLimit {
			return infraerrors.BadRequest("prompt_audit_invalid_input_limit", "审计节点输入上限超出允许范围")
		}
		if ep.Enabled {
			enabled++
		}
	}
	if cfg.Enabled && enabled == 0 && !promptKeywordModeSkipsAI(cfg.KeywordBlockingMode) {
		return infraerrors.BadRequest("prompt_audit_endpoint_required", "启用提示词审计前至少需要启用一个审计节点")
	}
	return nil
}

func validateUpdateConfigRequest(req UpdateConfigRequest) error {
	if strings.TrimSpace(req.Strategy) != "priority" {
		return infraerrors.BadRequest("prompt_audit_invalid_strategy", "提示词审计策略仅支持 priority")
	}
	if req.WorkerCount < 1 || req.WorkerCount > MaxWorkerCount {
		return infraerrors.BadRequest("prompt_audit_invalid_worker_count", "Worker 数量超出允许范围")
	}
	if req.QueueCapacity < 1 || req.QueueCapacity > MaxQueueCapacity {
		return infraerrors.BadRequest("prompt_audit_invalid_queue_capacity", "队列容量超出允许范围")
	}
	keywordBlocking, aiBlocking := updateBlockingFlags(req)
	if (keywordBlocking || aiBlocking) && !req.Enabled {
		return infraerrors.BadRequest(ErrorCodeRequiresEnabled, "开启同步阻止前必须先启用提示词审计")
	}
	keywordMode := req.KeywordBlockingMode
	if keywordMode == "" {
		keywordMode = PromptKeywordModeAIOnly
	}
	if !isPromptKeywordMode(keywordMode) {
		return infraerrors.BadRequest("prompt_audit_invalid_keyword_mode", "提示词审计关键词模式无效")
	}
	if !promptKeywordModeSkipsAI(keywordMode) && len(req.Scanners) == 0 {
		return infraerrors.BadRequest("prompt_audit_scanners_required", "至少需要启用一个风险分类")
	}
	for _, scanner := range req.Scanners {
		if _, ok := ScannerCatalog[NormalizeCategory(scanner)]; !ok {
			return infraerrors.BadRequest("prompt_audit_invalid_scanner", "提示词审计风险分类无效")
		}
	}
	if !req.AllGroups {
		if len(req.GroupIDs) == 0 {
			return infraerrors.BadRequest("prompt_audit_groups_required", "指定分组模式至少需要选择一个分组")
		}
		for _, groupID := range req.GroupIDs {
			if groupID <= 0 {
				return infraerrors.BadRequest("prompt_audit_invalid_group", "提示词审计分组 ID 无效")
			}
		}
	}
	for _, endpoint := range req.Endpoints {
		if endpoint.Protocol != "" && !isPromptAuditProtocol(strings.TrimSpace(endpoint.Protocol)) {
			return infraerrors.BadRequest("prompt_audit_invalid_endpoint_protocol", "审计节点协议无效")
		}
		if endpoint.TimeoutMS < MinTimeoutMS || endpoint.TimeoutMS > MaxTimeoutMS {
			return infraerrors.BadRequest("prompt_audit_invalid_timeout", "审计节点超时超出允许范围")
		}
		if endpoint.InputLimit < MinInputLimit || endpoint.InputLimit > MaxInputLimit {
			return infraerrors.BadRequest("prompt_audit_invalid_input_limit", "审计节点输入上限超出允许范围")
		}
	}
	return nil
}

func isPromptKeywordMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case PromptKeywordModeAIOnly, PromptKeywordModeKeywordOnly, PromptKeywordModeKeywordAndAI:
		return true
	default:
		return false
	}
}

func normalizePromptKeywordBlockingMode(mode string) string {
	mode = strings.TrimSpace(mode)
	if !isPromptKeywordMode(mode) {
		return PromptKeywordModeAIOnly
	}
	return mode
}

func updateBlockingFlags(req UpdateConfigRequest) (keywordBlocking, aiBlocking bool) {
	keywordBlocking, aiBlocking = req.BlockingEnabled, req.BlockingEnabled
	if req.KeywordBlockingEnabled != nil {
		keywordBlocking = *req.KeywordBlockingEnabled
	}
	if req.AIBlockingEnabled != nil {
		aiBlocking = *req.AIBlockingEnabled
	}
	return keywordBlocking, aiBlocking
}

func promptKeywordModeSkipsAI(mode string) bool {
	return normalizePromptKeywordBlockingMode(mode) == PromptKeywordModeKeywordOnly
}

func (cfg ActiveConfig) MatchBlockedKeyword(text string) (string, bool) {
	if cfg.keywordMatcher != nil {
		return cfg.keywordMatcher.Match(text)
	}
	return keywordmatcher.New(cfg.BlockedKeywords).Match(text)
}

func isPromptAuditProtocol(protocol string) bool {
	switch strings.TrimSpace(protocol) {
	case ProtocolOpenAICompatible, ProtocolOpenAIModeration, ProtocolNemotronSafety:
		return true
	default:
		return false
	}
}

func defaultModelForProtocol(protocol string) string {
	switch strings.TrimSpace(protocol) {
	case ProtocolOpenAIModeration:
		return DefaultModerationModel
	case ProtocolNemotronSafety:
		return DefaultNemotronModel
	default:
		return DefaultGuardModel
	}
}

func normalizeModelForEndpoint(protocol, baseURL, model string) string {
	protocol = strings.TrimSpace(protocol)
	model = strings.TrimSpace(model)
	if protocol == ProtocolNemotronSafety && (model == "" || model == DefaultNemotronModel) {
		if isOpenRouterBaseURL(baseURL) {
			return OpenRouterNemotronModel
		}
	}
	if model != "" {
		return model
	}
	return defaultModelForProtocol(protocol)
}

func isOpenRouterBaseURL(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	return err == nil && strings.EqualFold(parsed.Hostname(), "openrouter.ai")
}

func (cfg ActiveConfig) EffectiveMode() Mode {
	if !cfg.RiskControlEnabled || !cfg.Enabled {
		return ModeOff
	}
	if cfg.hasSynchronousBlocking() {
		return ModeBlocking
	}
	if cfg.shouldRunAsyncAudit() {
		return ModeAsync
	}
	return ModeOff
}

func (cfg ActiveConfig) effectiveKeywordBlockingEnabled() bool {
	if cfg.blockingFlagsConfigured || cfg.KeywordBlockingEnabled || cfg.AIBlockingEnabled {
		return cfg.KeywordBlockingEnabled
	}
	return cfg.BlockingEnabled
}

func (cfg ActiveConfig) effectiveAIBlockingEnabled() bool {
	if cfg.blockingFlagsConfigured || cfg.KeywordBlockingEnabled || cfg.AIBlockingEnabled {
		return cfg.AIBlockingEnabled
	}
	return cfg.BlockingEnabled
}

func (cfg ActiveConfig) keywordModeUsesKeywords() bool {
	return normalizePromptKeywordBlockingMode(cfg.KeywordBlockingMode) != PromptKeywordModeAIOnly
}

func (cfg ActiveConfig) keywordModeUsesAI() bool {
	return normalizePromptKeywordBlockingMode(cfg.KeywordBlockingMode) != PromptKeywordModeKeywordOnly
}

func (cfg ActiveConfig) hasSynchronousBlocking() bool {
	return (cfg.keywordModeUsesKeywords() && cfg.effectiveKeywordBlockingEnabled()) ||
		(cfg.keywordModeUsesAI() && cfg.effectiveAIBlockingEnabled())
}

// configIntentHasSynchronousBlocking is the single fail-closed / save / reload
// predicate. A switch that the current keyword mode cannot use must not count
// as "we intended to block".
func configIntentHasSynchronousBlocking(keywordMode string, keywordEnabled, aiEnabled bool) bool {
	return ActiveConfig{
		KeywordBlockingEnabled:  keywordEnabled,
		AIBlockingEnabled:       aiEnabled,
		KeywordBlockingMode:     normalizePromptKeywordBlockingMode(keywordMode),
		blockingFlagsConfigured: true,
	}.HasSynchronousBlocking()
}

// shouldRunAsyncAudit reports whether at least one selected audit mechanism
// must run after the request has been admitted. It is intentionally separate
// from EffectiveMode so a keyword sync block can coexist with async AI audit.
func (cfg ActiveConfig) shouldRunAsyncAudit() bool {
	if !cfg.RiskControlEnabled || !cfg.Enabled {
		return false
	}
	return (cfg.keywordModeUsesKeywords() && !cfg.effectiveKeywordBlockingEnabled()) ||
		(cfg.keywordModeUsesAI() && !cfg.effectiveAIBlockingEnabled())
}

func (cfg ActiveConfig) HasSynchronousBlocking() bool { return cfg.hasSynchronousBlocking() }
func (cfg ActiveConfig) ShouldRunAsyncAudit() bool    { return cfg.shouldRunAsyncAudit() }

func (cfg ActiveConfig) IncludesGroup(groupID *int64) bool {
	if cfg.AllGroups {
		return true
	}
	if groupID == nil {
		return false
	}
	i := sort.Search(len(cfg.GroupIDs), func(i int) bool { return cfg.GroupIDs[i] >= *groupID })
	return i < len(cfg.GroupIDs) && cfg.GroupIDs[i] == *groupID
}

func (cfg ActiveConfig) EnabledEndpoints() []ActiveEndpoint {
	result := make([]ActiveEndpoint, 0, len(cfg.Endpoints))
	for _, ep := range cfg.Endpoints {
		if ep.Enabled {
			result = append(result, ep)
		}
	}
	return result
}

// InvalidTokenEndpointIDs lists endpoints whose stored token could not be
// decrypted with the current encryption key.
func (cfg ActiveConfig) InvalidTokenEndpointIDs() []string {
	ids := make([]string, 0)
	for _, ep := range cfg.Endpoints {
		if ep.TokenInvalid {
			ids = append(ids, ep.ID)
		}
	}
	return ids
}

func PublicFromStorage(cfg storageConfig, riskControlEnabled bool, invalidTokenEndpointIDs []string) PublicConfig {
	invalid := make(map[string]struct{}, len(invalidTokenEndpointIDs))
	for _, id := range invalidTokenEndpointIDs {
		invalid[id] = struct{}{}
	}
	scanners := append([]string{}, cfg.Scanners...)
	groupIDs := append([]int64{}, cfg.GroupIDs...)
	endpoints := make([]PublicEndpoint, 0, len(cfg.Endpoints))
	for _, ep := range cfg.Endpoints {
		hasToken := strings.TrimSpace(ep.TokenCiphertext) != ""
		status := "missing"
		if hasToken {
			status = "configured"
			if _, ok := invalid[ep.ID]; ok {
				status = "invalid"
			}
		}
		endpoints = append(endpoints, PublicEndpoint{
			ID: ep.ID, Name: ep.Name, Protocol: ep.Protocol, BaseURL: ep.BaseURL,
			Model: ep.Model, TimeoutMS: ep.TimeoutMS, InputLimit: ep.InputLimit,
			Enabled: ep.Enabled, HasToken: hasToken, TokenStatus: status,
		})
	}
	active := ActiveConfig{
		RiskControlEnabled: riskControlEnabled, Enabled: cfg.Enabled, BlockingEnabled: cfg.BlockingEnabled,
		BlockingLatestTurnOnly: cfg.BlockingLatestTurnOnly,
		KeywordBlockingEnabled: cfg.KeywordBlockingEnabled, AIBlockingEnabled: cfg.AIBlockingEnabled,
		KeywordBlockingMode: cfg.KeywordBlockingMode, blockingFlagsConfigured: true,
	}
	return PublicConfig{
		Enabled: cfg.Enabled, BlockingEnabled: cfg.BlockingEnabled,
		BlockingLatestTurnOnly: cfg.BlockingLatestTurnOnly,
		KeywordBlockingEnabled: cfg.KeywordBlockingEnabled, AIBlockingEnabled: cfg.AIBlockingEnabled,
		StorePassEvents: cfg.StorePassEvents, PreHashCheckEnabled: cfg.PreHashCheckEnabled,
		BlockedKeywords: append([]string{}, cfg.BlockedKeywords...), KeywordBlockingMode: normalizePromptKeywordBlockingMode(cfg.KeywordBlockingMode),
		EffectiveMode: active.EffectiveMode(), Strategy: cfg.Strategy, WorkerCount: cfg.WorkerCount,
		QueueCapacity: cfg.QueueCapacity, Scanners: scanners, AllGroups: cfg.AllGroups,
		GroupIDs: groupIDs, ProxyID: cloneInt64Ptr(cfg.ProxyID), Endpoints: endpoints, ConfigVersion: cfg.ConfigVersion,
		UpdatedAt: cfg.UpdatedAt, UpdatedBy: cfg.UpdatedBy, ChangeSummary: cfg.ChangeSummary,
	}
}

func ActiveFromStorage(cfg storageConfig, riskControlEnabled bool, encryptor SecretEncryptor) (ActiveConfig, error) {
	active := ActiveConfig{
		RiskControlEnabled: riskControlEnabled, Enabled: cfg.Enabled, BlockingEnabled: cfg.BlockingEnabled,
		BlockingLatestTurnOnly: cfg.BlockingLatestTurnOnly,
		KeywordBlockingEnabled: cfg.KeywordBlockingEnabled, AIBlockingEnabled: cfg.AIBlockingEnabled,
		StorePassEvents: cfg.StorePassEvents, PreHashCheckEnabled: cfg.PreHashCheckEnabled,
		BlockedKeywords:     append([]string(nil), cfg.BlockedKeywords...),
		KeywordBlockingMode: normalizePromptKeywordBlockingMode(cfg.KeywordBlockingMode),
		Strategy:            cfg.Strategy, WorkerCount: cfg.WorkerCount,
		QueueCapacity: cfg.QueueCapacity, Scanners: append([]string(nil), cfg.Scanners...), AllGroups: cfg.AllGroups,
		GroupIDs: append([]int64(nil), cfg.GroupIDs...), ProxyID: cloneInt64Ptr(cfg.ProxyID), ConfigVersion: cfg.ConfigVersion,
		UpdatedAt: cfg.UpdatedAt, UpdatedBy: cfg.UpdatedBy, ChangeSummary: cfg.ChangeSummary,
		Endpoints:               make([]ActiveEndpoint, 0, len(cfg.Endpoints)),
		blockingFlagsConfigured: true,
	}
	for _, ep := range cfg.Endpoints {
		token := ""
		tokenInvalid := false
		if ep.TokenCiphertext != "" {
			if encryptor == nil {
				return ActiveConfig{}, fmt.Errorf("prompt audit secret encryptor unavailable")
			}
			plain, err := encryptor.Decrypt(ep.TokenCiphertext)
			if err != nil {
				// An undecryptable token (encryption key changed or regenerated)
				// must not take the whole config down: admins would otherwise be
				// locked out of the real config version and unable to recover
				// (issue #4887). Keep the ciphertext persisted, but exclude the
				// endpoint from runtime use until the token is re-entered.
				tokenInvalid = true
			} else {
				token = plain
			}
		}
		active.Endpoints = append(active.Endpoints, ActiveEndpoint{
			ID: ep.ID, Name: ep.Name, Protocol: ep.Protocol, BaseURL: ep.BaseURL, Model: ep.Model,
			Token: token, TimeoutMS: ep.TimeoutMS, InputLimit: ep.InputLimit,
			Enabled: ep.Enabled && !tokenInvalid, ProxyID: cloneInt64Ptr(cfg.ProxyID), TokenInvalid: tokenInvalid,
		})
	}
	active.keywordMatcher = keywordmatcher.New(active.BlockedKeywords)
	return active, nil
}

func changeSummary(cfg storageConfig) string {
	summary := struct {
		Enabled                bool   `json:"enabled"`
		BlockingEnabled        bool   `json:"blocking_enabled"`
		BlockingLatestTurnOnly bool   `json:"blocking_latest_turn_only"`
		KeywordBlockingEnabled bool   `json:"keyword_blocking_enabled"`
		AIBlockingEnabled      bool   `json:"ai_blocking_enabled"`
		StorePassEvents        bool   `json:"store_pass_events"`
		PreHashCheckEnabled    bool   `json:"pre_hash_check_enabled"`
		EndpointCount          int    `json:"endpoint_count"`
		ScannerCount           int    `json:"scanner_count"`
		AllGroups              bool   `json:"all_groups"`
		GroupCount             int    `json:"group_count"`
		GroupHash              string `json:"group_hash"`
		KeywordMode            string `json:"keyword_mode"`
		KeywordCount           int    `json:"keyword_count"`
		ProxyID                *int64 `json:"proxy_id,omitempty"`
	}{
		Enabled: cfg.Enabled, BlockingEnabled: cfg.BlockingEnabled,
		BlockingLatestTurnOnly: cfg.BlockingLatestTurnOnly,
		KeywordBlockingEnabled: cfg.KeywordBlockingEnabled, AIBlockingEnabled: cfg.AIBlockingEnabled,
		StorePassEvents: cfg.StorePassEvents, PreHashCheckEnabled: cfg.PreHashCheckEnabled,
		EndpointCount: len(cfg.Endpoints), ScannerCount: len(cfg.Scanners), AllGroups: cfg.AllGroups,
		GroupCount: len(cfg.GroupIDs), KeywordMode: cfg.KeywordBlockingMode, KeywordCount: len(cfg.BlockedKeywords),
		ProxyID: cloneInt64Ptr(cfg.ProxyID),
	}
	rawGroups, _ := json.Marshal(cfg.GroupIDs)
	digest := sha256.Sum256(rawGroups)
	summary.GroupHash = hex.EncodeToString(digest[:])
	raw, _ := json.Marshal(summary)
	return string(raw)
}

func canonicalInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func canonicalScannerIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		id := NormalizeCategory(value)
		if _, ok := ScannerCatalog[id]; ok {
			seen[id] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for _, id := range AllScannerIDs {
		if _, ok := seen[id]; ok {
			result = append(result, id)
		}
	}
	return result
}
