package securityaudit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	MaxTimeoutMS         = 30000
	DefaultInputLimit    = 4000
	MinInputLimit        = 128
	MaxInputLimit        = 100000
	DefaultPayloadTTL    = 30 * time.Minute
)

type SecretEncryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
REDACTED

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
REDACTED

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
REDACTED

type storageConfig struct {
	Enabled         bool              `json:"enabled"`
	BlockingEnabled bool              `json:"blocking_enabled"`
	StorePassEvents bool              `json:"store_pass_events"`
	Strategy        string            `json:"strategy"`
	WorkerCount     int               `json:"worker_count"`
	QueueCapacity   int               `json:"queue_capacity"`
	Scanners        []string          `json:"scanners"`
	AllGroups       bool              `json:"all_groups"`
	GroupIDs        []int64           `json:"group_ids"`
	Endpoints       []StorageEndpoint `json:"endpoints"`
	ConfigVersion   int64             `json:"config_version"`
	UpdatedAt       time.Time         `json:"updated_at"`
	UpdatedBy       int64             `json:"updated_by"`
	ChangeSummary   string            `json:"change_summary"`
REDACTED

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
	// TokenInvalid marks an endpoint whose persisted token ciphertext cannot be
	// decrypted with the current encryption key (key changed or auto-generated
	// on restart). The endpoint is kept visible for admins but excluded from
	// runtime use until the token is re-entered or cleared (issue #4887).
	TokenInvalid bool
REDACTED

type ActiveConfig struct {
	RiskControlEnabled bool
	Enabled            bool
	BlockingEnabled    bool
	StorePassEvents    bool
	Strategy           string
	WorkerCount        int
	QueueCapacity      int
	Scanners           []string
	AllGroups          bool
	GroupIDs           []int64
	Endpoints          []ActiveEndpoint
	ConfigVersion      int64
	UpdatedAt          time.Time
	UpdatedBy          int64
	ChangeSummary      string
REDACTED

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
REDACTED

type PublicConfig struct {
	Enabled         bool             `json:"enabled"`
	BlockingEnabled bool             `json:"blocking_enabled"`
	StorePassEvents bool             `json:"store_pass_events"`
	EffectiveMode   Mode             `json:"effective_mode"`
	Strategy        string           `json:"strategy"`
	WorkerCount     int              `json:"worker_count"`
	QueueCapacity   int              `json:"queue_capacity"`
	Scanners        []string         `json:"scanners"`
	AllGroups       bool             `json:"all_groups"`
	GroupIDs        []int64          `json:"group_ids"`
	Endpoints       []PublicEndpoint `json:"endpoints"`
	ConfigVersion   int64            `json:"config_version"`
	UpdatedAt       time.Time        `json:"updated_at"`
	UpdatedBy       int64            `json:"updated_by"`
	ChangeSummary   string           `json:"change_summary"`
REDACTED

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
REDACTED

type UpdateConfigRequest struct {
	ExpectedConfigVersion int64            `json:"expected_config_version" binding:"required"`
	Enabled               bool             `json:"enabled"`
	BlockingEnabled       bool             `json:"blocking_enabled"`
	StorePassEvents       bool             `json:"store_pass_events"`
	Strategy              string           `json:"strategy"`
	WorkerCount           int              `json:"worker_count"`
	QueueCapacity         int              `json:"queue_capacity"`
	Scanners              []string         `json:"scanners"`
	AllGroups             bool             `json:"all_groups"`
	GroupIDs              []int64          `json:"group_ids"`
	Endpoints             []UpdateEndpoint `json:"endpoints"`
REDACTED

func DefaultStorageConfig() storageConfig {
	return storageConfig{
		Enabled:         false,
		BlockingEnabled: false,
		StorePassEvents: false,
		Strategy:        "priority",
		WorkerCount:     DefaultWorkerCount,
		QueueCapacity:   DefaultQueueCapacity,
		Scanners:        append([]string(nil), AllScannerIDs...),
		AllGroups:       true,
		GroupIDs:        []int64{REDACTED,
		Endpoints:       []StorageEndpoint{REDACTED,
		ConfigVersion:   1,
REDACTED
REDACTED

func ParseStorageConfig(raw string) (storageConfig, error) {
	cfg := DefaultStorageConfig()
	if strings.TrimSpace(raw) == "" {
		return cfg, nil
REDACTED
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return storageConfig{REDACTED, fmt.Errorf("decode prompt audit config: %w", err)
REDACTED
	normalizeStorageConfig(&cfg)
	if err := validateStorageConfig(cfg); err != nil {
		return storageConfig{REDACTED, err
REDACTED
	return cfg, nil
REDACTED

func normalizeStorageConfig(cfg *storageConfig) {
	if cfg == nil {
		return
REDACTED
	if cfg.ConfigVersion < 1 {
		cfg.ConfigVersion = 1
REDACTED
	if strings.TrimSpace(cfg.Strategy) == "" {
		cfg.Strategy = "priority"
REDACTED
	if cfg.WorkerCount == 0 {
		cfg.WorkerCount = DefaultWorkerCount
REDACTED
	if cfg.QueueCapacity == 0 {
		cfg.QueueCapacity = DefaultQueueCapacity
REDACTED
	if len(cfg.Scanners) == 0 {
		cfg.Scanners = append([]string(nil), AllScannerIDs...)
REDACTED
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
	REDACTED
		ep.BaseURL = strings.TrimSpace(ep.BaseURL)
		ep.Model = strings.TrimSpace(ep.Model)
		if ep.Model == "" {
			ep.Model = DefaultGuardModel
	REDACTED
		if ep.TimeoutMS == 0 {
			ep.TimeoutMS = DefaultTimeoutMS
	REDACTED
		if ep.InputLimit == 0 {
			ep.InputLimit = DefaultInputLimit
	REDACTED
REDACTED
REDACTED

func validateStorageConfig(cfg storageConfig) error {
	if cfg.BlockingEnabled && !cfg.Enabled {
		return infraerrors.BadRequest(ErrorCodeRequiresEnabled, "开启同步阻止前必须先启用提示词审计")
REDACTED
	if cfg.Strategy != "priority" {
		return infraerrors.BadRequest("prompt_audit_invalid_strategy", "提示词审计策略仅支持 priority")
REDACTED
	if cfg.WorkerCount < 1 || cfg.WorkerCount > MaxWorkerCount {
		return infraerrors.BadRequest("prompt_audit_invalid_worker_count", "Worker 数量超出允许范围")
REDACTED
	if cfg.QueueCapacity < 1 || cfg.QueueCapacity > MaxQueueCapacity {
		return infraerrors.BadRequest("prompt_audit_invalid_queue_capacity", "队列容量超出允许范围")
REDACTED
	if !cfg.AllGroups && len(cfg.GroupIDs) == 0 {
		return infraerrors.BadRequest("prompt_audit_groups_required", "指定分组模式至少需要选择一个分组")
REDACTED
	if len(cfg.Scanners) == 0 {
		return infraerrors.BadRequest("prompt_audit_scanners_required", "至少需要启用一个风险分类")
REDACTED
	seen := make(map[string]struct{REDACTED, len(cfg.Endpoints))
	enabled := 0
	for _, ep := range cfg.Endpoints {
		if ep.ID == "" || ep.Name == "" {
			return infraerrors.BadRequest("prompt_audit_invalid_endpoint", "审计节点 ID 和名称不能为空")
	REDACTED
		if _, ok := seen[ep.ID]; ok {
			return infraerrors.BadRequest("prompt_audit_duplicate_endpoint", "审计节点 ID 不能重复")
	REDACTED
		seen[ep.ID] = struct{REDACTED{REDACTED
		if ep.Protocol != "openai_compatible" {
			return infraerrors.BadRequest("prompt_audit_invalid_endpoint_protocol", "审计节点仅支持 OpenAI 兼容协议")
	REDACTED
		if _, err := NormalizeBaseURL(ep.BaseURL); err != nil {
			return err
	REDACTED
		if ep.TimeoutMS < MinTimeoutMS || ep.TimeoutMS > MaxTimeoutMS {
			return infraerrors.BadRequest("prompt_audit_invalid_timeout", "审计节点超时超出允许范围")
	REDACTED
		if ep.InputLimit < MinInputLimit || ep.InputLimit > MaxInputLimit {
			return infraerrors.BadRequest("prompt_audit_invalid_input_limit", "审计节点输入上限超出允许范围")
	REDACTED
		if ep.Enabled {
			enabled++
	REDACTED
REDACTED
	if cfg.Enabled && enabled == 0 {
		return infraerrors.BadRequest("prompt_audit_endpoint_required", "启用提示词审计前至少需要启用一个审计节点")
REDACTED
	return nil
REDACTED

func validateUpdateConfigRequest(req UpdateConfigRequest) error {
	if strings.TrimSpace(req.Strategy) != "priority" {
		return infraerrors.BadRequest("prompt_audit_invalid_strategy", "提示词审计策略仅支持 priority")
REDACTED
	if req.WorkerCount < 1 || req.WorkerCount > MaxWorkerCount {
		return infraerrors.BadRequest("prompt_audit_invalid_worker_count", "Worker 数量超出允许范围")
REDACTED
	if req.QueueCapacity < 1 || req.QueueCapacity > MaxQueueCapacity {
		return infraerrors.BadRequest("prompt_audit_invalid_queue_capacity", "队列容量超出允许范围")
REDACTED
	if len(req.Scanners) == 0 {
		return infraerrors.BadRequest("prompt_audit_scanners_required", "至少需要启用一个风险分类")
REDACTED
	for _, scanner := range req.Scanners {
		if _, ok := ScannerCatalog[NormalizeCategory(scanner)]; !ok {
			return infraerrors.BadRequest("prompt_audit_invalid_scanner", "提示词审计风险分类无效")
	REDACTED
REDACTED
	if !req.AllGroups {
		if len(req.GroupIDs) == 0 {
			return infraerrors.BadRequest("prompt_audit_groups_required", "指定分组模式至少需要选择一个分组")
	REDACTED
		for _, groupID := range req.GroupIDs {
			if groupID <= 0 {
				return infraerrors.BadRequest("prompt_audit_invalid_group", "提示词审计分组 ID 无效")
		REDACTED
	REDACTED
REDACTED
	for _, endpoint := range req.Endpoints {
		if endpoint.TimeoutMS < MinTimeoutMS || endpoint.TimeoutMS > MaxTimeoutMS {
			return infraerrors.BadRequest("prompt_audit_invalid_timeout", "审计节点超时超出允许范围")
	REDACTED
		if endpoint.InputLimit < MinInputLimit || endpoint.InputLimit > MaxInputLimit {
			return infraerrors.BadRequest("prompt_audit_invalid_input_limit", "审计节点输入上限超出允许范围")
	REDACTED
REDACTED
	return nil
REDACTED

func (cfg ActiveConfig) EffectiveMode() Mode {
	if !cfg.RiskControlEnabled || !cfg.Enabled {
		return ModeOff
REDACTED
	if cfg.BlockingEnabled {
		return ModeBlocking
REDACTED
	return ModeAsync
REDACTED

func (cfg ActiveConfig) IncludesGroup(groupID *int64) bool {
	if cfg.AllGroups {
		return true
REDACTED
	if groupID == nil {
		return false
REDACTED
	i := sort.Search(len(cfg.GroupIDs), func(i int) bool { return cfg.GroupIDs[i] >= *groupID REDACTED)
	return i < len(cfg.GroupIDs) && cfg.GroupIDs[i] == *groupID
REDACTED

func (cfg ActiveConfig) EnabledEndpoints() []ActiveEndpoint {
	result := make([]ActiveEndpoint, 0, len(cfg.Endpoints))
	for _, ep := range cfg.Endpoints {
		if ep.Enabled {
			result = append(result, ep)
	REDACTED
REDACTED
	return result
REDACTED

// InvalidTokenEndpointIDs lists endpoints whose stored token could not be
// decrypted with the current encryption key.
func (cfg ActiveConfig) InvalidTokenEndpointIDs() []string {
	ids := make([]string, 0)
	for _, ep := range cfg.Endpoints {
		if ep.TokenInvalid {
			ids = append(ids, ep.ID)
	REDACTED
REDACTED
	return ids
REDACTED

func PublicFromStorage(cfg storageConfig, riskControlEnabled bool, invalidTokenEndpointIDs []string) PublicConfig {
	invalid := make(map[string]struct{REDACTED, len(invalidTokenEndpointIDs))
	for _, id := range invalidTokenEndpointIDs {
		invalid[id] = struct{REDACTED{REDACTED
REDACTED
	scanners := append([]string{REDACTED, cfg.Scanners...)
	groupIDs := append([]int64{REDACTED, cfg.GroupIDs...)
	endpoints := make([]PublicEndpoint, 0, len(cfg.Endpoints))
	for _, ep := range cfg.Endpoints {
		hasToken := strings.TrimSpace(ep.TokenCiphertext) != ""
		status := "missing"
		if hasToken {
			status = "configured"
			if _, ok := invalid[ep.ID]; ok {
				status = "invalid"
		REDACTED
	REDACTED
		endpoints = append(endpoints, PublicEndpoint{
			ID: ep.ID, Name: ep.Name, Protocol: ep.Protocol, BaseURL: ep.BaseURL,
			Model: ep.Model, TimeoutMS: ep.TimeoutMS, InputLimit: ep.InputLimit,
			Enabled: ep.Enabled, HasToken: hasToken, TokenStatus: status,
	REDACTED)
REDACTED
	active := ActiveConfig{RiskControlEnabled: riskControlEnabled, Enabled: cfg.Enabled, BlockingEnabled: cfg.BlockingEnabledREDACTED
	return PublicConfig{
		Enabled: cfg.Enabled, BlockingEnabled: cfg.BlockingEnabled, StorePassEvents: cfg.StorePassEvents,
		EffectiveMode: active.EffectiveMode(), Strategy: cfg.Strategy, WorkerCount: cfg.WorkerCount,
		QueueCapacity: cfg.QueueCapacity, Scanners: scanners, AllGroups: cfg.AllGroups,
		GroupIDs: groupIDs, Endpoints: endpoints, ConfigVersion: cfg.ConfigVersion,
		UpdatedAt: cfg.UpdatedAt, UpdatedBy: cfg.UpdatedBy, ChangeSummary: cfg.ChangeSummary,
REDACTED
REDACTED

func ActiveFromStorage(cfg storageConfig, riskControlEnabled bool, encryptor SecretEncryptor) (ActiveConfig, error) {
	active := ActiveConfig{
		RiskControlEnabled: riskControlEnabled, Enabled: cfg.Enabled, BlockingEnabled: cfg.BlockingEnabled,
		StorePassEvents: cfg.StorePassEvents, Strategy: cfg.Strategy, WorkerCount: cfg.WorkerCount,
		QueueCapacity: cfg.QueueCapacity, Scanners: append([]string(nil), cfg.Scanners...), AllGroups: cfg.AllGroups,
		GroupIDs: append([]int64(nil), cfg.GroupIDs...), ConfigVersion: cfg.ConfigVersion,
		UpdatedAt: cfg.UpdatedAt, UpdatedBy: cfg.UpdatedBy, ChangeSummary: cfg.ChangeSummary,
		Endpoints: make([]ActiveEndpoint, 0, len(cfg.Endpoints)),
REDACTED
	for _, ep := range cfg.Endpoints {
		token := ""
		tokenInvalid := false
		if ep.TokenCiphertext != "" {
			if encryptor == nil {
				return ActiveConfig{REDACTED, fmt.Errorf("prompt audit secret encryptor unavailable")
		REDACTED
			plain, err := encryptor.Decrypt(ep.TokenCiphertext)
			if err != nil {
				// An undecryptable token (encryption key changed or regenerated)
				// must not take the whole config down: admins would otherwise be
				// locked out of the real config version and unable to recover
				// (issue #4887). Keep the ciphertext persisted, but exclude the
				// endpoint from runtime use until the token is re-entered.
				tokenInvalid = true
		REDACTED else {
				token = plain
		REDACTED
	REDACTED
		active.Endpoints = append(active.Endpoints, ActiveEndpoint{
			ID: ep.ID, Name: ep.Name, Protocol: ep.Protocol, BaseURL: ep.BaseURL, Model: ep.Model,
			Token: token, TimeoutMS: ep.TimeoutMS, InputLimit: ep.InputLimit,
			Enabled: ep.Enabled && !tokenInvalid, TokenInvalid: tokenInvalid,
	REDACTED)
REDACTED
	return active, nil
REDACTED

func changeSummary(cfg storageConfig) string {
	summary := struct {
		Enabled         bool   `json:"enabled"`
		BlockingEnabled bool   `json:"blocking_enabled"`
		StorePassEvents bool   `json:"store_pass_events"`
		EndpointCount   int    `json:"endpoint_count"`
		ScannerCount    int    `json:"scanner_count"`
		AllGroups       bool   `json:"all_groups"`
		GroupCount      int    `json:"group_count"`
		GroupHash       string `json:"group_hash"`
REDACTED{cfg.Enabled, cfg.BlockingEnabled, cfg.StorePassEvents, len(cfg.Endpoints), len(cfg.Scanners), cfg.AllGroups, len(cfg.GroupIDs), ""REDACTED
	rawGroups, _ := json.Marshal(cfg.GroupIDs)
	digest := sha256.Sum256(rawGroups)
	summary.GroupHash = hex.EncodeToString(digest[:])
	raw, _ := json.Marshal(summary)
	return string(raw)
REDACTED

func canonicalInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{REDACTED, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
	REDACTED
		if _, ok := seen[value]; ok {
			continue
	REDACTED
		seen[value] = struct{REDACTED{REDACTED
		result = append(result, value)
REDACTED
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] REDACTED)
	return result
REDACTED

func canonicalScannerIDs(values []string) []string {
	seen := make(map[string]struct{REDACTED, len(values))
	for _, value := range values {
		id := NormalizeCategory(value)
		if _, ok := ScannerCatalog[id]; ok {
			seen[id] = struct{REDACTED{REDACTED
	REDACTED
REDACTED
	result := make([]string, 0, len(seen))
	for _, id := range AllScannerIDs {
		if _, ok := seen[id]; ok {
			result = append(result, id)
	REDACTED
REDACTED
	return result
REDACTED
