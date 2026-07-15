package service

// This file contains the RTK control-plane contract. It intentionally does
// not depend on a concrete compression engine: request handlers can evaluate
// a stable policy snapshot before an engine is introduced or replaced.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/promptcompression/rtk"
)

const (
	RTKModeOff     = "off"
	RTKModeObserve = "observe"
	RTKModeEnforce = "enforce"
)

const (
	promptCompressionPolicySettingKey      = "prompt_compression_policy"
	promptCompressionGroupPolicySettingKey = "prompt_compression_group_policies"
	promptCompressionRuntimeSettingKey     = "prompt_compression_runtime"
)

var supportedRTKProtocols = map[string]struct{}{
	"anthropic": {},
	"chat":      {},
	"responses": {},
	"gemini":    {},
}

// PromptCompressionPolicy is the immutable request-level policy consumed by
// the engine. Group and request overrides are resolved into this structure.
type PromptCompressionPolicy struct {
	Enabled              bool     `json:"enabled"`
	Mode                 string   `json:"mode"`
	Intensity            string   `json:"intensity"`
	ProfileVersionID     string   `json:"profile_version_id,omitempty"`
	FilterPackVersionID  string   `json:"filter_pack_version_id,omitempty"`
	RolloutPercent       int      `json:"rollout_percent"`
	HoldoutPercent       int      `json:"holdout_percent"`
	AllowedProtocols     []string `json:"allowed_protocols"`
	MinCandidateTokens   int      `json:"min_candidate_tokens"`
	MinSavingsTokens     int      `json:"min_savings_tokens"`
	MaxBodyBytes         int64    `json:"max_body_bytes"`
	MaxResultBytes       int64    `json:"max_result_bytes"`
	MaxDurationMS        int      `json:"max_duration_ms"`
	AllowRequestOverride bool     `json:"allow_request_override"`
	Revision             uint64   `json:"revision"`
	ConfigHash           string   `json:"config_hash"`
}

// PromptCompressionGroupPolicy is the JSON-compatible group override. A mode
// of inherit leaves the deployment policy unchanged.
type PromptCompressionGroupPolicy struct {
	SchemaVersion       int      `json:"schema_version"`
	Mode                string   `json:"mode"`
	Intensity           string   `json:"intensity,omitempty"`
	ProfileVersionID    string   `json:"profile_version_id,omitempty"`
	FilterPackVersionID string   `json:"filter_pack_version_id,omitempty"`
	RolloutPercent      *int     `json:"rollout_percent,omitempty"`
	HoldoutPercent      *int     `json:"holdout_percent,omitempty"`
	Protocols           []string `json:"protocols,omitempty"`
	ModelAllowlist      []string `json:"model_allowlist,omitempty"`
	ModelBlocklist      []string `json:"model_blocklist,omitempty"`
	MinCandidateTokens  *int     `json:"min_candidate_tokens,omitempty"`
	MinSavingsTokens    *int     `json:"min_savings_tokens,omitempty"`
	AllowRequestForce   *bool    `json:"allow_request_force,omitempty"`
	ApplyToCountTokens  *bool    `json:"apply_to_count_tokens,omitempty"`
	ApplyToCompact      *bool    `json:"apply_to_compact,omitempty"`
}

type PromptCompressionFilterPack struct {
	ID        string       `json:"id"`
	Version   uint64       `json:"version"`
	Published bool         `json:"published"`
	Filters   []rtk.Filter `json:"filters"`
}

// RTKRuntimeState is process-shared emergency-stop state. It is separate
// from deployment config so operators can stop RTK without a restart.
type RTKRuntimeState struct {
	EmergencyStopped bool      `json:"emergency_stopped"`
	Revision         uint64    `json:"revision"`
	UpdatedAt        time.Time `json:"updated_at,omitempty"`
	UpdatedBy        string    `json:"updated_by,omitempty"`
	Reason           string    `json:"reason,omitempty"`
}

type PromptCompressionStatus struct {
	DeploymentEnabled bool                              `json:"deployment_enabled"`
	Mode              string                            `json:"mode"`
	Runtime           RTKRuntimeState                   `json:"runtime"`
	Policy            PromptCompressionPolicy           `json:"policy"`
	Telemetry         PromptCompressionTelemetrySummary `json:"telemetry"`
	EngineAvailable   bool                              `json:"engine_available"`
}

type PromptCompressionTelemetry struct {
	RequestID       string        `json:"request_id,omitempty"`
	GroupID         int64         `json:"group_id,omitempty"`
	APIKeyID        int64         `json:"api_key_id,omitempty"`
	Protocol        string        `json:"protocol,omitempty"`
	Model           string        `json:"model,omitempty"`
	Mode            string        `json:"mode,omitempty"`
	Outcome         string        `json:"outcome,omitempty"`
	SkipReason      string        `json:"skip_reason,omitempty"`
	ProfileRevision uint64        `json:"profile_revision,omitempty"`
	BeforeBytes     int           `json:"before_bytes,omitempty"`
	AfterBytes      int           `json:"after_bytes,omitempty"`
	BeforeTokens    int           `json:"before_tokens,omitempty"`
	AfterTokens     int           `json:"after_tokens,omitempty"`
	ChangedTargets  int           `json:"changed_targets,omitempty"`
	Duration        time.Duration `json:"duration,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
}

type PromptCompressionTelemetrySummary struct {
	Recorded uint64 `json:"recorded"`
	Dropped  uint64 `json:"dropped"`
	Applied  uint64 `json:"applied"`
	Skipped  uint64 `json:"skipped"`
	Failed   uint64 `json:"failed"`
}

type RTKPolicyRequest struct {
	Protocol         string
	Model            string
	GroupID          int64
	GroupPolicy      *PromptCompressionGroupPolicy
	RequestedMode    string
	RequestOverride  string
	CohortValue      string
	EndpointBypassed bool
}

type RTKPolicyDecision struct {
	Policy     PromptCompressionPolicy
	Enabled    bool
	Mode       string
	Cohort     string
	SkipReason string
}

// PromptCompressionService is a lightweight control-plane manager. The
// immutable snapshot is copied under a lock, so request hot paths never hold
// mutable configuration pointers.
type PromptCompressionService struct {
	cfg *config.Config

	mu             sync.RWMutex
	policy         PromptCompressionPolicy
	runtime        RTKRuntimeState
	engine         *rtk.Engine
	groupPolicies  map[int64]PromptCompressionGroupPolicy
	settingService *SettingService
	filterPacks    map[string]PromptCompressionFilterPack

	telemetryCh chan PromptCompressionTelemetry
	recorded    atomic.Uint64
	dropped     atomic.Uint64
	applied     atomic.Uint64
	skipped     atomic.Uint64
	failed      atomic.Uint64
}

func NewPromptCompressionService(cfg *config.Config, settings ...*SettingService) *PromptCompressionService {
	if cfg == nil {
		cfg = &config.Config{}
	}
	p := policyFromConfig(cfg)
	engine, _ := buildRTKEngine(p)
	var settingService *SettingService
	if len(settings) > 0 {
		settingService = settings[0]
	}
	svc := &PromptCompressionService{
		cfg:            cfg,
		policy:         p,
		engine:         engine,
		groupPolicies:  make(map[int64]PromptCompressionGroupPolicy),
		settingService: settingService,
		filterPacks:    make(map[string]PromptCompressionFilterPack),
		telemetryCh:    make(chan PromptCompressionTelemetry, 1024),
	}
	svc.loadPersistedState()
	return svc
}

func (s *PromptCompressionService) ValidateFilterPack(filters []rtk.Filter) error {
	_, err := rtk.NewEngine(rtk.Config{Mode: rtk.ModeObserve, MinCandidateBytes: 1, MinCandidateTokens: 1, MinSavedTokens: 1, MaxBodyBytes: 10 * 1024 * 1024, MaxResultBytes: 1024 * 1024}, filters)
	return err
}

func (s *PromptCompressionService) ListFilterPacks() []PromptCompressionFilterPack {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PromptCompressionFilterPack, 0, len(s.filterPacks))
	for _, pack := range s.filterPacks {
		pack.Filters = append([]rtk.Filter(nil), pack.Filters...)
		out = append(out, pack)
	}
	return out
}

func (s *PromptCompressionService) PublishFilterPack(pack PromptCompressionFilterPack) error {
	if strings.TrimSpace(pack.ID) == "" {
		return errors.New("filter pack id is required")
	}
	if err := s.ValidateFilterPack(pack.Filters); err != nil {
		return err
	}
	s.mu.Lock()
	pack.Version = s.filterPacks[pack.ID].Version + 1
	pack.Published = true
	s.filterPacks[pack.ID] = pack
	s.mu.Unlock()
	return nil
}

func (s *PromptCompressionService) loadPersistedState() {
	if s.settingService == nil {
		return
	}
	if raw, err := s.settingService.GetValue(context.Background(), promptCompressionPolicySettingKey); err == nil && strings.TrimSpace(raw) != "" {
		var p PromptCompressionPolicy
		if json.Unmarshal([]byte(raw), &p) == nil && validatePolicy(p) == nil {
			if engine, e := buildRTKEngine(p); e == nil {
				s.policy, s.engine = p, engine
			}
		}
	}
	if raw, err := s.settingService.GetValue(context.Background(), promptCompressionGroupPolicySettingKey); err == nil && strings.TrimSpace(raw) != "" {
		var policies map[int64]PromptCompressionGroupPolicy
		if json.Unmarshal([]byte(raw), &policies) == nil {
			s.groupPolicies = policies
		}
	}
	if raw, err := s.settingService.GetValue(context.Background(), promptCompressionRuntimeSettingKey); err == nil && strings.TrimSpace(raw) != "" {
		var state RTKRuntimeState
		if json.Unmarshal([]byte(raw), &state) == nil {
			s.runtime = state
		}
	}
}

func (s *PromptCompressionService) persistPolicy(p PromptCompressionPolicy) error {
	if s.settingService == nil {
		return nil
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.settingService.SetValue(context.Background(), promptCompressionPolicySettingKey, string(raw))
}

func (s *PromptCompressionService) persistGroupPolicies() error {
	if s.settingService == nil {
		return nil
	}
	s.mu.RLock()
	policies := make(map[int64]PromptCompressionGroupPolicy, len(s.groupPolicies))
	for id, p := range s.groupPolicies {
		policies[id] = p
	}
	s.mu.RUnlock()
	raw, err := json.Marshal(policies)
	if err != nil {
		return err
	}
	return s.settingService.SetValue(context.Background(), promptCompressionGroupPolicySettingKey, string(raw))
}

func (s *PromptCompressionService) persistRuntime() error {
	if s.settingService == nil {
		return nil
	}
	raw, err := json.Marshal(s.RuntimeState())
	if err != nil {
		return err
	}
	return s.settingService.SetValue(context.Background(), promptCompressionRuntimeSettingKey, string(raw))
}

func buildRTKEngine(p PromptCompressionPolicy) (*rtk.Engine, error) {
	return rtk.NewEngine(rtk.Config{
		Mode: rtk.Mode(p.Mode), Intensity: rtk.Intensity(strings.ToLower(strings.TrimSpace(p.Intensity))),
		MinCandidateBytes: 256, MinCandidateTokens: p.MinCandidateTokens,
		MinSavedTokens: p.MinSavingsTokens, MaxBodyBytes: int(p.MaxBodyBytes),
		MaxResultBytes: int(p.MaxResultBytes), MaxDuration: time.Duration(p.MaxDurationMS) * time.Millisecond,
	}, nil)
}

func policyFromConfig(cfg *config.Config) PromptCompressionPolicy {
	c := cfg.Gateway.PromptCompression
	protocols := normalizeProtocols(c.AllowedProtocols)
	if len(protocols) == 0 {
		protocols = []string{"anthropic", "chat", "responses", "gemini"}
	}
	mode := RTKModeOff
	if c.Enabled {
		mode = RTKModeObserve
	}
	intensity := strings.ToLower(strings.TrimSpace(c.Intensity))
	if intensity == "" {
		intensity = "balanced"
	}
	p := PromptCompressionPolicy{
		Enabled: c.Enabled, Mode: mode, Intensity: intensity, AllowedProtocols: protocols,
		MinCandidateTokens: c.MinCandidateTokens, MinSavingsTokens: c.MinSavingsTokens,
		MaxBodyBytes: c.MaxBodyBytes, MaxResultBytes: c.MaxResultBytes,
		MaxDurationMS: c.MaxDurationMS, AllowRequestOverride: c.AllowRequestOverride,
		RolloutPercent: 100, HoldoutPercent: 0,
	}
	p.ConfigHash = hashPolicy(p)
	return p
}

func hashPolicy(p PromptCompressionPolicy) string {
	// A deterministic hash is sufficient for diagnostics and multi-instance
	// divergence checks; it must not include request content.
	s := p.Mode + ":" + p.Intensity + ":" + strings.Join(p.AllowedProtocols, ",") + ":" +
		strconv.Itoa(p.MinCandidateTokens) + ":" + strconv.Itoa(p.MinSavingsTokens) + ":" +
		strconv.FormatInt(p.MaxBodyBytes, 10) + ":" + strconv.FormatInt(p.MaxResultBytes, 10)
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

func (s *PromptCompressionService) Snapshot() PromptCompressionPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p := s.policy
	p.AllowedProtocols = append([]string(nil), p.AllowedProtocols...)
	return p
}

func (s *PromptCompressionService) RuntimeState() RTKRuntimeState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.runtime
}

func (s *PromptCompressionService) GroupPolicy(groupID int64) *PromptCompressionGroupPolicy {
	if groupID <= 0 {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.groupPolicies[groupID]
	if !ok {
		return nil
	}
	return &p
}

func (s *PromptCompressionService) UpdateGroupPolicy(groupID int64, policy PromptCompressionGroupPolicy) error {
	if groupID <= 0 {
		return errors.New("group_id must be positive")
	}
	if policy.Mode == "" {
		policy.Mode = "inherit"
	}
	if policy.Mode != "inherit" && policy.Mode != RTKModeOff && policy.Mode != RTKModeObserve && policy.Mode != RTKModeEnforce {
		return errors.New("group policy mode must be inherit/off/observe/enforce")
	}
	if policy.Intensity != "" {
		policy.Intensity = strings.ToLower(strings.TrimSpace(policy.Intensity))
		if policy.Intensity != "safe" && policy.Intensity != "balanced" && policy.Intensity != "aggressive" {
			return errors.New("group policy intensity must be safe/balanced/aggressive")
		}
	}
	s.mu.Lock()
	s.groupPolicies[groupID] = policy
	s.mu.Unlock()
	if err := s.persistGroupPolicies(); err != nil {
		return err
	}
	return nil
}

func (s *PromptCompressionService) Status() PromptCompressionStatus {
	p := s.Snapshot()
	runtime := s.RuntimeState()
	s.mu.RLock()
	engineAvailable := s.engine != nil
	s.mu.RUnlock()
	return PromptCompressionStatus{
		DeploymentEnabled: s.cfg.Gateway.PromptCompression.Enabled,
		Mode:              p.Mode, Policy: p, Runtime: runtime,
		EngineAvailable: engineAvailable,
		Telemetry: PromptCompressionTelemetrySummary{
			Recorded: s.recorded.Load(), Dropped: s.dropped.Load(), Applied: s.applied.Load(),
			Skipped: s.skipped.Load(), Failed: s.failed.Load(),
		},
	}
}

func (s *PromptCompressionService) UpdatePolicy(policy PromptCompressionPolicy) error {
	if err := validatePolicy(policy); err != nil {
		return err
	}
	policy.AllowedProtocols = normalizeProtocols(policy.AllowedProtocols)
	policy.ConfigHash = hashPolicy(policy)
	engine, err := buildRTKEngine(policy)
	if err != nil {
		return err
	}
	s.mu.Lock()
	policy.Revision = s.policy.Revision + 1
	s.mu.Unlock()
	if err := s.persistPolicy(policy); err != nil {
		return err
	}
	s.mu.Lock()
	s.policy = policy
	s.engine = engine
	s.mu.Unlock()
	return nil
}

func validatePolicy(p PromptCompressionPolicy) error {
	switch p.Mode {
	case RTKModeOff, RTKModeObserve, RTKModeEnforce:
	default:
		return errors.New("prompt compression mode must be one of off/observe/enforce")
	}
	switch strings.ToLower(strings.TrimSpace(p.Intensity)) {
	case "", "safe", "balanced", "aggressive":
	default:
		return errors.New("prompt compression intensity must be safe/balanced/aggressive")
	}
	if p.RolloutPercent < 0 || p.RolloutPercent > 100 || p.HoldoutPercent < 0 || p.HoldoutPercent > 100 {
		return errors.New("prompt compression rollout and holdout must be between 0 and 100")
	}
	if p.MinCandidateTokens < 0 || p.MinSavingsTokens < 0 || p.MaxBodyBytes < 0 || p.MaxResultBytes < 0 || p.MaxDurationMS < 0 {
		return errors.New("prompt compression limits must be non-negative")
	}
	for _, protocol := range p.AllowedProtocols {
		if _, ok := supportedRTKProtocols[strings.ToLower(strings.TrimSpace(protocol))]; !ok {
			return errors.New("prompt compression policy contains unsupported protocol")
		}
	}
	return nil
}

func normalizeProtocols(protocols []string) []string {
	seen := make(map[string]struct{}, len(protocols))
	out := make([]string, 0, len(protocols))
	for _, raw := range protocols {
		protocol := strings.ToLower(strings.TrimSpace(raw))
		if _, ok := supportedRTKProtocols[protocol]; !ok {
			continue
		}
		if _, ok := seen[protocol]; ok {
			continue
		}
		seen[protocol] = struct{}{}
		out = append(out, protocol)
	}
	return out
}

// ResolvePolicy applies deployment guards and the group/request controls. It
// intentionally leaves stable rollout hashing to the caller, which owns the
// canonical session hash.
func (s *PromptCompressionService) ResolvePolicy(_ context.Context, req RTKPolicyRequest) RTKPolicyDecision {
	p := s.Snapshot()
	decision := RTKPolicyDecision{Policy: p, Mode: p.Mode}
	if !p.Enabled || !s.cfg.Gateway.PromptCompression.Enabled {
		decision.Mode, decision.SkipReason = RTKModeOff, "deployment_disabled"
		return decision
	}
	if s.RuntimeState().EmergencyStopped {
		decision.Mode, decision.SkipReason = RTKModeOff, "emergency_stopped"
		return decision
	}
	if req.EndpointBypassed {
		decision.Mode, decision.SkipReason = RTKModeOff, "endpoint_bypassed"
		return decision
	}
	if req.Protocol != "" {
		allowed := false
		for _, protocol := range p.AllowedProtocols {
			if strings.EqualFold(protocol, req.Protocol) {
				allowed = true
				break
			}
		}
		if !allowed {
			decision.Mode, decision.SkipReason = RTKModeOff, "protocol_not_allowed"
			return decision
		}
	}
	groupPolicy := req.GroupPolicy
	if groupPolicy == nil {
		groupPolicy = s.GroupPolicy(req.GroupID)
	}
	if groupPolicy != nil {
		applyGroupPolicy(&p, *groupPolicy)
		decision.Policy = p
		decision.Mode = p.Mode
	}
	if p.RolloutPercent < 100 || p.HoldoutPercent > 0 {
		bucket := stableRTKBucket(req.CohortValue)
		if bucket >= p.RolloutPercent {
			decision.Mode, decision.SkipReason = RTKModeOff, "rollout_holdout"
			if bucket < p.RolloutPercent+p.HoldoutPercent {
				decision.Cohort = "holdout"
			} else {
				decision.Cohort = "outside_rollout"
			}
			return decision
		}
		decision.Cohort = "treatment"
	}
	if req.RequestOverride == RTKModeOff {
		decision.Mode, decision.SkipReason = RTKModeOff, "request_disabled"
	} else if p.AllowRequestOverride && (req.RequestOverride == RTKModeObserve || req.RequestOverride == RTKModeEnforce) {
		decision.Mode = req.RequestOverride
	}
	return decision
}

func stableRTKBucket(value string) int {
	sum := sha256.Sum256([]byte(value))
	return int(sum[0]) * 100 / 256
}

// Prepare is the single service entry point shared by HTTP preview and gateway
// integrations. It freezes policy before invoking the immutable engine.
func (s *PromptCompressionService) Prepare(ctx context.Context, body []byte, req RTKPolicyRequest) (rtk.Result, RTKPolicyDecision) {
	decision := s.ResolvePolicy(ctx, req)
	result := rtk.Result{Body: body, Mode: rtk.Mode(decision.Mode), Outcome: "skipped", SkipReason: decision.SkipReason, BeforeBytes: len(body), AfterBytes: len(body)}
	if decision.Mode == RTKModeOff {
		if result.SkipReason == "" {
			result.SkipReason = "disabled"
		}
		return result, decision
	}
	s.mu.RLock()
	engine := s.engine
	var filters []rtk.Filter
	filterPackMissing := false
	if decision.Policy.FilterPackVersionID != "" {
		if pack, ok := s.filterPacks[decision.Policy.FilterPackVersionID]; ok {
			filters = append([]rtk.Filter(nil), pack.Filters...)
		} else {
			filterPackMissing = true
		}
	}
	s.mu.RUnlock()
	if engine == nil {
		result.Outcome, result.SkipReason = "fallback", "engine_unavailable"
		return result, decision
	}
	if filterPackMissing {
		result.Outcome, result.SkipReason = "fallback", "filter_pack_unavailable"
		return result, decision
	}
	result = engine.Prepare(ctx, body, rtk.Options{Protocol: normalizeEngineProtocol(req.Protocol), Model: req.Model, Mode: rtk.Mode(decision.Mode), Filters: filters})
	result.ProfileRevision = strconv.FormatUint(decision.Policy.Revision, 10)
	return result, decision
}

func normalizeEngineProtocol(protocol string) rtk.Protocol {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "chat", "chat_completions", "chat-completions":
		return rtk.ProtocolChat
	case "anthropic", "messages":
		return rtk.ProtocolAnthropic
	case "responses", "openai_responses", "openai-responses":
		return rtk.ProtocolResponses
	case "gemini", "gemini_native", "gemini-native":
		return rtk.ProtocolGemini
	default:
		return rtk.Protocol(protocol)
	}
}

func applyGroupPolicy(p *PromptCompressionPolicy, group PromptCompressionGroupPolicy) {
	if group.Mode != "" && group.Mode != "inherit" {
		if group.Mode == RTKModeOff || group.Mode == RTKModeObserve || group.Mode == RTKModeEnforce {
			p.Mode, p.Enabled = group.Mode, group.Mode != RTKModeOff
		}
	}
	if group.ProfileVersionID != "" {
		p.ProfileVersionID = group.ProfileVersionID
	}
	if group.Intensity != "" {
		p.Intensity = strings.ToLower(strings.TrimSpace(group.Intensity))
	}
	if group.FilterPackVersionID != "" {
		p.FilterPackVersionID = group.FilterPackVersionID
	}
	if group.RolloutPercent != nil {
		p.RolloutPercent = *group.RolloutPercent
	}
	if group.HoldoutPercent != nil {
		p.HoldoutPercent = *group.HoldoutPercent
	}
	if len(group.Protocols) > 0 {
		p.AllowedProtocols = normalizeProtocols(group.Protocols)
	}
	if group.MinCandidateTokens != nil {
		p.MinCandidateTokens = *group.MinCandidateTokens
	}
	if group.MinSavingsTokens != nil {
		p.MinSavingsTokens = *group.MinSavingsTokens
	}
	if group.AllowRequestForce != nil {
		p.AllowRequestOverride = *group.AllowRequestForce
	}
}

func (s *PromptCompressionService) EmergencyStop(actor, reason string) {
	s.mu.Lock()
	s.runtime = RTKRuntimeState{EmergencyStopped: true, Revision: s.runtime.Revision + 1, UpdatedAt: time.Now().UTC(), UpdatedBy: actor, Reason: reason}
	s.mu.Unlock()
	_ = s.persistRuntime()
}

func (s *PromptCompressionService) Resume(actor, reason string) {
	s.mu.Lock()
	s.runtime = RTKRuntimeState{EmergencyStopped: false, Revision: s.runtime.Revision + 1, UpdatedAt: time.Now().UTC(), UpdatedBy: actor, Reason: reason}
	s.mu.Unlock()
	_ = s.persistRuntime()
}

// RecordTelemetry is non-blocking by design. A full queue must never affect a
// gateway request. Consumers may later persist records in a repository.
func (s *PromptCompressionService) RecordTelemetry(event PromptCompressionTelemetry) bool {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	switch event.Outcome {
	case "applied":
		s.applied.Add(1)
	case "skipped":
		s.skipped.Add(1)
	case "failed":
		s.failed.Add(1)
	}
	select {
	case s.telemetryCh <- event:
		s.recorded.Add(1)
		return true
	default:
		s.dropped.Add(1)
		return false
	}
}

func (s *PromptCompressionService) DrainTelemetry(limit int) []PromptCompressionTelemetry {
	if limit <= 0 {
		limit = 100
	}
	out := make([]PromptCompressionTelemetry, 0, limit)
	for len(out) < limit {
		select {
		case event := <-s.telemetryCh:
			out = append(out, event)
		default:
			return out
		}
	}
	return out
}

func (s *PromptCompressionService) Preview(ctx context.Context, protocol string, body []byte) map[string]any {
	if ctx == nil {
		ctx = context.Background()
	}
	result, decision := s.Prepare(ctx, body, RTKPolicyRequest{Protocol: protocol})
	return map[string]any{
		"applied": result.Applied, "mode": decision.Mode, "skip_reason": result.SkipReason,
		"engine_available": true, "original_bytes": len(body), "effective_bytes": len(result.Body),
		"outcome": result.Outcome, "changed_targets": result.ChangedTargets,
		"before_tokens": result.BeforeTokens, "after_tokens": result.AfterTokens,
		"policy_revision": decision.Policy.Revision, "config_hash": decision.Policy.ConfigHash,
	}
}
