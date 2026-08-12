package securityaudit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	DefaultWorkerCount   = 4
	MaxWorkerCount       = 32
	DefaultQueueCapacity = 32768
	MaxQueueCapacity     = 100000
	DefaultTimeoutMS     = 3000
	MinTimeoutMS         = 100
	MaxTimeoutMS         = 60000
	DefaultInputLimit    = 4000
	MinInputLimit        = 128
	MaxInputLimit        = 100000
	DefaultPayloadTTL    = 30 * time.Minute
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
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	Protocol            string  `json:"protocol"`
	BaseURL             string  `json:"base_url"`
	Model               string  `json:"model"`
	TokenCiphertext     string  `json:"token_ciphertext,omitempty"`
	TimeoutMS           int     `json:"timeout_ms"`
	InputLimit          int     `json:"input_limit"`
	Enabled             bool    `json:"enabled"`
	EngineType          string  `json:"engine_type,omitempty"`
	SchemaVersion       int     `json:"schema_version,omitempty"`
	SystemGuidance      string  `json:"system_guidance,omitempty"`
	ConfidenceThreshold float64 `json:"confidence_threshold,omitempty"`
	JSONOutputMode      string  `json:"json_output_mode,omitempty"`
	SampleRate          float64 `json:"sample_rate,omitempty"`
	MaxOutputTokens     int     `json:"max_output_tokens,omitempty"`
	ReasoningEffort     string  `json:"reasoning_effort,omitempty"`
	Stage               string  `json:"stage,omitempty"`
	FailurePolicy       string  `json:"failure_policy,omitempty"`
	CompositionMode     string  `json:"composition_mode,omitempty"`
}

type storageConfig struct {
	Enabled                bool              `json:"enabled"`
	BlockingEnabled        bool              `json:"blocking_enabled"`
	BlockingLatestTurnOnly bool              `json:"blocking_latest_turn_only"`
	StorePassEvents        bool              `json:"store_pass_events"`
	Strategy               string            `json:"strategy"`
	WorkerCount            int               `json:"worker_count"`
	QueueCapacity          int               `json:"queue_capacity"`
	Scanners               []string          `json:"scanners"`
	AllGroups              bool              `json:"all_groups"`
	GroupIDs               []int64           `json:"group_ids"`
	Endpoints              []StorageEndpoint `json:"endpoints"`
	ConfigVersion          int64             `json:"config_version"`
	UpdatedAt              time.Time         `json:"updated_at"`
	UpdatedBy              int64             `json:"updated_by"`
	ChangeSummary          string            `json:"change_summary"`
}

type ActiveEndpoint struct {
	ID                  string
	Name                string
	Protocol            string
	BaseURL             string
	Model               string
	Token               string
	TimeoutMS           int
	InputLimit          int
	Enabled             bool
	EngineType          string
	SchemaVersion       int
	SystemGuidance      string
	ConfidenceThreshold float64
	JSONOutputMode      string
	SampleRate          float64
	MaxOutputTokens     int
	ReasoningEffort     string
	Stage               string
	FailurePolicy       string
	CompositionMode     string
	// TokenInvalid marks an endpoint whose persisted token ciphertext cannot be
	// decrypted with the current encryption key (key changed or auto-generated
	// on restart). The endpoint is kept visible for admins but excluded from
	// runtime use until the token is re-entered or cleared (issue #4887).
	TokenInvalid bool
}

type ActiveConfig struct {
	RiskControlEnabled     bool
	Enabled                bool
	BlockingEnabled        bool
	BlockingLatestTurnOnly bool
	StorePassEvents        bool
	Strategy               string
	WorkerCount            int
	QueueCapacity          int
	Scanners               []string
	AllGroups              bool
	GroupIDs               []int64
	Endpoints              []ActiveEndpoint
	ConfigVersion          int64
	UpdatedAt              time.Time
	UpdatedBy              int64
	ChangeSummary          string
}

type PublicEndpoint struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	Protocol            string  `json:"protocol"`
	BaseURL             string  `json:"base_url"`
	Model               string  `json:"model"`
	TimeoutMS           int     `json:"timeout_ms"`
	InputLimit          int     `json:"input_limit"`
	Enabled             bool    `json:"enabled"`
	HasToken            bool    `json:"has_token"`
	TokenStatus         string  `json:"token_status"`
	EngineType          string  `json:"engine_type"`
	SchemaVersion       int     `json:"schema_version"`
	SystemGuidance      string  `json:"system_guidance"`
	ConfidenceThreshold float64 `json:"confidence_threshold"`
	JSONOutputMode      string  `json:"json_output_mode"`
	SampleRate          float64 `json:"sample_rate"`
	MaxOutputTokens     int     `json:"max_output_tokens"`
	ReasoningEffort     string  `json:"reasoning_effort"`
	Stage               string  `json:"stage"`
	FailurePolicy       string  `json:"failure_policy"`
	CompositionMode     string  `json:"composition_mode"`
}

type PublicConfig struct {
	Enabled                bool             `json:"enabled"`
	BlockingEnabled        bool             `json:"blocking_enabled"`
	BlockingLatestTurnOnly bool             `json:"blocking_latest_turn_only"`
	StorePassEvents        bool             `json:"store_pass_events"`
	EffectiveMode          Mode             `json:"effective_mode"`
	Strategy               string           `json:"strategy"`
	WorkerCount            int              `json:"worker_count"`
	QueueCapacity          int              `json:"queue_capacity"`
	Scanners               []string         `json:"scanners"`
	AllGroups              bool             `json:"all_groups"`
	GroupIDs               []int64          `json:"group_ids"`
	Endpoints              []PublicEndpoint `json:"endpoints"`
	ConfigVersion          int64            `json:"config_version"`
	UpdatedAt              time.Time        `json:"updated_at"`
	UpdatedBy              int64            `json:"updated_by"`
	ChangeSummary          string           `json:"change_summary"`
}

type UpdateEndpoint struct {
	ID                  string  `json:"id" binding:"required"`
	Name                string  `json:"name" binding:"required"`
	Protocol            string  `json:"protocol"`
	BaseURL             string  `json:"base_url" binding:"required"`
	Model               string  `json:"model"`
	Token               string  `json:"token,omitempty"`
	ClearToken          bool    `json:"clear_token"`
	TimeoutMS           int     `json:"timeout_ms"`
	InputLimit          int     `json:"input_limit"`
	Enabled             bool    `json:"enabled"`
	EngineType          string  `json:"engine_type"`
	SchemaVersion       int     `json:"schema_version"`
	SystemGuidance      string  `json:"system_guidance"`
	ConfidenceThreshold float64 `json:"confidence_threshold"`
	JSONOutputMode      string  `json:"json_output_mode"`
	SampleRate          float64 `json:"sample_rate"`
	MaxOutputTokens     int     `json:"max_output_tokens"`
	ReasoningEffort     string  `json:"reasoning_effort"`
	Stage               string  `json:"stage"`
	FailurePolicy       string  `json:"failure_policy"`
	CompositionMode     string  `json:"composition_mode"`
}

type UpdateConfigRequest struct {
	ExpectedConfigVersion  int64            `json:"expected_config_version" binding:"required"`
	Enabled                bool             `json:"enabled"`
	BlockingEnabled        bool             `json:"blocking_enabled"`
	BlockingLatestTurnOnly bool             `json:"blocking_latest_turn_only"`
	StorePassEvents        bool             `json:"store_pass_events"`
	Strategy               string           `json:"strategy"`
	WorkerCount            int              `json:"worker_count"`
	QueueCapacity          int              `json:"queue_capacity"`
	Scanners               []string         `json:"scanners"`
	AllGroups              bool             `json:"all_groups"`
	GroupIDs               []int64          `json:"group_ids"`
	Endpoints              []UpdateEndpoint `json:"endpoints"`
}

func DefaultStorageConfig() storageConfig {
	return storageConfig{
		Enabled:                false,
		BlockingEnabled:        false,
		BlockingLatestTurnOnly: false,
		StorePassEvents:        false,
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
	if len(cfg.Scanners) == 0 {
		cfg.Scanners = append([]string(nil), AllScannerIDs...)
	}
	cfg.Scanners = canonicalScannerIDs(cfg.Scanners)
	cfg.GroupIDs = canonicalInt64s(cfg.GroupIDs)
	// Preserve an invalid blocking-without-audit combination so validation can
	// reject it instead of silently changing administrator intent.
	for i := range cfg.Endpoints {
		ep := &cfg.Endpoints[i]
		ep.ID = strings.TrimSpace(ep.ID)
		ep.Name = strings.TrimSpace(ep.Name)
		ep.Protocol = strings.TrimSpace(ep.Protocol)
		if ep.Protocol == "" {
			ep.Protocol = "openai_compatible"
		}
		ep.BaseURL = strings.TrimSpace(ep.BaseURL)
		ep.Model = strings.TrimSpace(ep.Model)
		if ep.Model == "" {
			ep.Model = DefaultGuardModel
		}
		if ep.TimeoutMS == 0 {
			ep.TimeoutMS = DefaultTimeoutMS
		}
		if ep.InputLimit == 0 {
			ep.InputLimit = DefaultInputLimit
		}
		normalizeEndpointPolicy(ep)
	}
}

func normalizeEndpointPolicy(ep *StorageEndpoint) {
	ep.EngineType = strings.TrimSpace(ep.EngineType)
	if ep.EngineType == "" {
		ep.EngineType = EngineQwen3Guard
	}
	if ep.SchemaVersion == 0 {
		ep.SchemaVersion = GenericSchemaVersion
	}
	ep.SystemGuidance = strings.TrimSpace(ep.SystemGuidance)
	if ep.ConfidenceThreshold == 0 {
		ep.ConfidenceThreshold = DefaultConfidenceThreshold
	}
	ep.JSONOutputMode = strings.TrimSpace(ep.JSONOutputMode)
	if ep.JSONOutputMode == "" {
		ep.JSONOutputMode = "plain_json"
	}
	if ep.SampleRate == 0 {
		ep.SampleRate = 1
	}
	if ep.MaxOutputTokens == 0 {
		ep.MaxOutputTokens = DefaultGenericMaxOutputTokens
	}
	ep.ReasoningEffort = strings.TrimSpace(ep.ReasoningEffort)
	if ep.ReasoningEffort == "" {
		ep.ReasoningEffort = "low"
	}
	ep.Stage = strings.TrimSpace(ep.Stage)
	if ep.Stage == "" {
		ep.Stage = "shadow"
	}
	ep.FailurePolicy = strings.TrimSpace(ep.FailurePolicy)
	if ep.FailurePolicy == "" {
		ep.FailurePolicy = "fail_open"
	}
	ep.CompositionMode = strings.TrimSpace(ep.CompositionMode)
	if ep.CompositionMode == "" {
		ep.CompositionMode = "keyword_first"
	}
}

func validateEndpointPolicy(ep StorageEndpoint) error {
	if ep.EngineType != EngineQwen3Guard && ep.EngineType != EngineGenericLLM {
		return infraerrors.BadRequest("prompt_audit_invalid_engine_type", "审计引擎类型无效")
	}
	if ep.EngineType == EngineQwen3Guard {
		return nil
	}
	if ep.SchemaVersion != GenericSchemaVersion {
		return infraerrors.BadRequest("prompt_audit_invalid_schema_version", "通用审计协议版本无效")
	}
	if len([]rune(ep.SystemGuidance)) > MaxGenericSystemGuidanceRunes {
		return infraerrors.BadRequest("prompt_audit_guidance_too_long", "通用审计策略说明过长")
	}
	if ep.ConfidenceThreshold < 0 || ep.ConfidenceThreshold > 1 {
		return infraerrors.BadRequest("prompt_audit_invalid_confidence_threshold", "通用审计置信度阈值无效")
	}
	if ep.JSONOutputMode != "plain_json" && ep.JSONOutputMode != "json_schema" {
		return infraerrors.BadRequest("prompt_audit_invalid_json_output_mode", "通用审计 JSON 输出模式无效")
	}
	if ep.SampleRate <= 0 || ep.SampleRate > 1 {
		return infraerrors.BadRequest("prompt_audit_invalid_sample_rate", "通用审计采样率无效")
	}
	if ep.MaxOutputTokens < 16 || ep.MaxOutputTokens > MaxGenericMaxOutputTokens {
		return infraerrors.BadRequest("prompt_audit_invalid_max_output_tokens", "通用审计输出 Token 上限无效")
	}
	if ep.ReasoningEffort != "low" && ep.ReasoningEffort != "high" && ep.ReasoningEffort != "xhigh" && ep.ReasoningEffort != "max" {
		return infraerrors.BadRequest("prompt_audit_invalid_reasoning_effort", "通用审计推理强度无效")
	}
	if ep.Stage != "shadow" && ep.Stage != "warn" && ep.Stage != "block" {
		return infraerrors.BadRequest("prompt_audit_invalid_stage", "通用审计阶段无效")
	}
	if ep.FailurePolicy != "fail_open" && ep.FailurePolicy != "fail_closed" {
		return infraerrors.BadRequest("prompt_audit_invalid_failure_policy", "通用审计失败策略无效")
	}
	if ep.CompositionMode != "keyword_first" && ep.CompositionMode != "llm_only" && ep.CompositionMode != "combined" {
		return infraerrors.BadRequest("prompt_audit_invalid_composition_mode", "通用审计组合模式无效")
	}
	if ep.Stage == "block" && ep.SampleRate != 1 {
		return infraerrors.BadRequest("prompt_audit_block_requires_full_sampling", "阻止阶段必须使用完整采样")
	}
	if ep.FailurePolicy == "fail_closed" && ep.Stage != "block" {
		return infraerrors.BadRequest("prompt_audit_fail_closed_requires_block", "仅阻止阶段可启用失败关闭")
	}
	return nil
}

func validateStorageConfig(cfg storageConfig) error {
	if cfg.BlockingEnabled && !cfg.Enabled {
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
	if len(cfg.Scanners) == 0 {
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
		if ep.Protocol != "openai_compatible" {
			return infraerrors.BadRequest("prompt_audit_invalid_endpoint_protocol", "审计节点仅支持 OpenAI 兼容协议")
		}
		if _, err := NormalizeBaseURL(ep.BaseURL); err != nil {
			return err
		}
		if ep.EngineType == EngineGenericLLM {
			parsed, _ := url.Parse(ep.BaseURL)
			if literal := net.ParseIP(parsed.Hostname()); literal != nil && !isPublicPromptAuditIP(literal) {
				return infraerrors.BadRequest("prompt_audit_unsafe_destination", "通用审计节点必须使用公网地址")
			}
		}
		if ep.TimeoutMS < MinTimeoutMS || ep.TimeoutMS > MaxTimeoutMS {
			return infraerrors.BadRequest("prompt_audit_invalid_timeout", "审计节点超时超出允许范围")
		}
		if ep.InputLimit < MinInputLimit || ep.InputLimit > MaxInputLimit {
			return infraerrors.BadRequest("prompt_audit_invalid_input_limit", "审计节点输入上限超出允许范围")
		}
		if err := validateEndpointPolicy(ep); err != nil {
			return err
		}
		if ep.Enabled {
			enabled++
		}
	}
	if cfg.Enabled && enabled == 0 {
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
	if len(req.Scanners) == 0 {
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
		if endpoint.TimeoutMS < MinTimeoutMS || endpoint.TimeoutMS > MaxTimeoutMS {
			return infraerrors.BadRequest("prompt_audit_invalid_timeout", "审计节点超时超出允许范围")
		}
		if endpoint.InputLimit < MinInputLimit || endpoint.InputLimit > MaxInputLimit {
			return infraerrors.BadRequest("prompt_audit_invalid_input_limit", "审计节点输入上限超出允许范围")
		}
		policy := StorageEndpoint{EngineType: endpoint.EngineType, SchemaVersion: endpoint.SchemaVersion, SystemGuidance: endpoint.SystemGuidance, ConfidenceThreshold: endpoint.ConfidenceThreshold, JSONOutputMode: endpoint.JSONOutputMode, SampleRate: endpoint.SampleRate, MaxOutputTokens: endpoint.MaxOutputTokens, ReasoningEffort: endpoint.ReasoningEffort, Stage: endpoint.Stage, FailurePolicy: endpoint.FailurePolicy, CompositionMode: endpoint.CompositionMode}
		normalizeEndpointPolicy(&policy)
		if err := validateEndpointPolicy(policy); err != nil {
			return err
		}
	}
	return nil
}

func (cfg ActiveConfig) EffectiveMode() Mode {
	if !cfg.RiskControlEnabled || !cfg.Enabled {
		return ModeOff
	}
	if cfg.BlockingEnabled {
		return ModeBlocking
	}
	return ModeAsync
}

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
			EngineType: ep.EngineType, SchemaVersion: ep.SchemaVersion, SystemGuidance: ep.SystemGuidance,
			ConfidenceThreshold: ep.ConfidenceThreshold, JSONOutputMode: ep.JSONOutputMode, SampleRate: ep.SampleRate,
			MaxOutputTokens: ep.MaxOutputTokens, ReasoningEffort: ep.ReasoningEffort, Stage: ep.Stage, FailurePolicy: ep.FailurePolicy, CompositionMode: ep.CompositionMode,
		})
	}
	active := ActiveConfig{RiskControlEnabled: riskControlEnabled, Enabled: cfg.Enabled, BlockingEnabled: cfg.BlockingEnabled}
	return PublicConfig{
		Enabled: cfg.Enabled, BlockingEnabled: cfg.BlockingEnabled, BlockingLatestTurnOnly: cfg.BlockingLatestTurnOnly, StorePassEvents: cfg.StorePassEvents,
		EffectiveMode: active.EffectiveMode(), Strategy: cfg.Strategy, WorkerCount: cfg.WorkerCount,
		QueueCapacity: cfg.QueueCapacity, Scanners: scanners, AllGroups: cfg.AllGroups,
		GroupIDs: groupIDs, Endpoints: endpoints, ConfigVersion: cfg.ConfigVersion,
		UpdatedAt: cfg.UpdatedAt, UpdatedBy: cfg.UpdatedBy, ChangeSummary: cfg.ChangeSummary,
	}
}

func ActiveFromStorage(cfg storageConfig, riskControlEnabled bool, encryptor SecretEncryptor) (ActiveConfig, error) {
	active := ActiveConfig{
		RiskControlEnabled: riskControlEnabled, Enabled: cfg.Enabled, BlockingEnabled: cfg.BlockingEnabled,
		BlockingLatestTurnOnly: cfg.BlockingLatestTurnOnly,
		StorePassEvents:        cfg.StorePassEvents, Strategy: cfg.Strategy, WorkerCount: cfg.WorkerCount,
		QueueCapacity: cfg.QueueCapacity, Scanners: append([]string(nil), cfg.Scanners...), AllGroups: cfg.AllGroups,
		GroupIDs: append([]int64(nil), cfg.GroupIDs...), ConfigVersion: cfg.ConfigVersion,
		UpdatedAt: cfg.UpdatedAt, UpdatedBy: cfg.UpdatedBy, ChangeSummary: cfg.ChangeSummary,
		Endpoints: make([]ActiveEndpoint, 0, len(cfg.Endpoints)),
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
			Enabled: ep.Enabled && !tokenInvalid, TokenInvalid: tokenInvalid,
			EngineType: ep.EngineType, SchemaVersion: ep.SchemaVersion, SystemGuidance: ep.SystemGuidance,
			ConfidenceThreshold: ep.ConfidenceThreshold, JSONOutputMode: ep.JSONOutputMode, SampleRate: ep.SampleRate,
			MaxOutputTokens: ep.MaxOutputTokens, ReasoningEffort: ep.ReasoningEffort, Stage: ep.Stage, FailurePolicy: ep.FailurePolicy, CompositionMode: ep.CompositionMode,
		})
	}
	return active, nil
}

func changeSummary(cfg storageConfig) string {
	summary := struct {
		Enabled                bool   `json:"enabled"`
		BlockingEnabled        bool   `json:"blocking_enabled"`
		BlockingLatestTurnOnly bool   `json:"blocking_latest_turn_only"`
		StorePassEvents        bool   `json:"store_pass_events"`
		EndpointCount          int    `json:"endpoint_count"`
		ScannerCount           int    `json:"scanner_count"`
		AllGroups              bool   `json:"all_groups"`
		GroupCount             int    `json:"group_count"`
		GroupHash              string `json:"group_hash"`
	}{cfg.Enabled, cfg.BlockingEnabled, cfg.BlockingLatestTurnOnly, cfg.StorePassEvents, len(cfg.Endpoints), len(cfg.Scanners), cfg.AllGroups, len(cfg.GroupIDs), ""}
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
