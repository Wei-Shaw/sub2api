package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"golang.org/x/sync/errgroup"
)

// ChannelMonitorRepository 渠道监控数据访问接口。
// 入参/返回的指针类型均使用 service 包的 ChannelMonitor 模型，
// repository 实现负责与 ent 模型互转，并保持 api_key_encrypted 字段为密文。
type ChannelMonitorRepository interface {
	// CRUD
	Create(ctx context.Context, m *ChannelMonitor) error
	GetByID(ctx context.Context, id int64) (*ChannelMonitor, error)
	Update(ctx context.Context, m *ChannelMonitor) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params ChannelMonitorListParams) ([]*ChannelMonitor, int64, error)
	FindByDuplicateOperationID(ctx context.Context, operationID string) (*ChannelMonitor, error)

	// 调度器辅助
	ListEnabled(ctx context.Context) ([]*ChannelMonitor, error)
	MarkChecked(ctx context.Context, id int64, checkedAt time.Time) error
	InsertHistoryBatch(ctx context.Context, rows []*ChannelMonitorHistoryRow) error
	DeleteHistoryBefore(ctx context.Context, before time.Time) (int64, error)

	// 历史记录
	ListHistory(ctx context.Context, monitorID int64, model string, limit int) ([]*ChannelMonitorHistoryEntry, error)

	// 用户视图聚合
	ListLatestPerModel(ctx context.Context, monitorID int64) ([]*ChannelMonitorLatest, error)
	ComputeAvailability(ctx context.Context, monitorID int64, windowDays int) ([]*ChannelMonitorAvailability, error)

	// 批量聚合（admin/user list 用，避免 N+1）
	ListLatestForMonitorIDs(ctx context.Context, ids []int64) (map[int64][]*ChannelMonitorLatest, error)
	ComputeAvailabilityForMonitors(ctx context.Context, ids []int64, windowDays int) (map[int64][]*ChannelMonitorAvailability, error)
	// ListRecentHistoryForMonitors 批量取多个 monitor 各自主模型（primaryModels[monitorID]）最近 perMonitorLimit 条历史。
	// 返回的 entry 已按 checked_at DESC 排序（最新在前），不含 message 字段。
	ListRecentHistoryForMonitors(ctx context.Context, ids []int64, primaryModels map[int64]string, perMonitorLimit int) (map[int64][]*ChannelMonitorHistoryEntry, error)

	// ---------- 聚合维护（OpsCleanupService 调用） ----------

	// UpsertDailyRollupsFor 把 targetDate 当天的明细按 (monitor_id, model, bucket_date)
	// 聚合到 channel_monitor_daily_rollups。targetDate 会被截断到日期；
	// 用 ON CONFLICT DO UPDATE 实现幂等回填，返回 upsert 影响的行数。
	UpsertDailyRollupsFor(ctx context.Context, targetDate time.Time) (int64, error)
	// DeleteRollupsBefore 软删 bucket_date < beforeDate 的聚合行，返回删除行数。
	DeleteRollupsBefore(ctx context.Context, beforeDate time.Time) (int64, error)
	// LoadAggregationWatermark 读 watermark（id=1）。
	// 返回 nil 表示从未聚合过；watermark 表本身预期已存在单行（migration 110 写入）。
	LoadAggregationWatermark(ctx context.Context) (*time.Time, error)
	// UpdateAggregationWatermark 写 watermark（UPSERT 到 id=1）。
	UpdateAggregationWatermark(ctx context.Context, date time.Time) error
REDACTED

// channelMonitorRuntimeReader is the optional settings view used to gate V1
// active probes by channel_monitor_enabled + channel_monitor_mode.
type channelMonitorRuntimeReader interface {
	GetChannelMonitorRuntime(ctx context.Context) ChannelMonitorRuntime
REDACTED

// ChannelMonitorService 渠道监控管理服务。
type ChannelMonitorService struct {
	repo      ChannelMonitorRepository
	encryptor SecretEncryptor
	// settings is optional; when nil, RunCheck fails closed for active probes
	// (mode defaults to v2 / retired) so tests without settings never hit upstream.
	settings channelMonitorRuntimeReader
	// scheduler 由 wire 通过 SetScheduler 注入；CRUD 后调用对应钩子即时同步任务。
	// 测试或未注入场景下保持 nil，所有钩子调用变为 no-op。
	scheduler MonitorScheduler
	// quotaFetcher 由 wire 通过 SetQuotaFetcher 注入（accountUsage/CN 服务在本服务
	// 之后构造，构造参数注入会破坏既有依赖顺序）。nil 时 fail-closed：
	// 配额模式的检测产出「未配置」错误快照，Create/Update 关联账号直接报错。
	quotaFetcher *ChannelMonitorQuotaFetcher
REDACTED

const maxChannelMonitorNameRunes = 100

// ChannelMonitorDuplicateOperationIDMetadataKey is stored in the existing
// extra_headers JSON column to avoid a schema migration. The colon makes it an
// invalid HTTP header name, and repository adapters remove it before exposing
// ExtraHeaders to the service layer.
const ChannelMonitorDuplicateOperationIDMetadataKey = "sub2api:duplicate_operation_id"

// NewChannelMonitorService 创建渠道监控服务实例。
func NewChannelMonitorService(repo ChannelMonitorRepository, encryptor SecretEncryptor) *ChannelMonitorService {
	return &ChannelMonitorService{repo: repo, encryptor: encryptorREDACTED
REDACTED

// SetRuntimeReader injects the settings reader used to gate active probes.
// Optional: when unset, active probes are treated as mode=v2 (retired).
func (s *ChannelMonitorService) SetRuntimeReader(r channelMonitorRuntimeReader) {
	if s == nil {
		return
REDACTED
	s.settings = r
REDACTED

func (s *ChannelMonitorService) probeRuntime(ctx context.Context) ChannelMonitorRuntime {
	if s == nil || s.settings == nil {
		return ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV2REDACTED
REDACTED
	return s.settings.GetChannelMonitorRuntime(ctx)
REDACTED

// ---------- CRUD ----------

// List 列表查询（支持 provider/enabled/search 过滤 + 分页）。
// 返回的 ChannelMonitor.APIKey 已解密为明文，handler 层负责脱敏。
func (s *ChannelMonitorService) List(ctx context.Context, params ChannelMonitorListParams) ([]*ChannelMonitor, int64, error) {
	if params.Page < 1 {
		params.Page = 1
REDACTED
	if params.PageSize < 1 || params.PageSize > 200 {
		params.PageSize = 20
REDACTED
	items, total, err := s.repo.List(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("list channel monitors: %w", err)
REDACTED
	for _, it := range items {
		s.decryptInPlace(it)
REDACTED
	return items, total, nil
REDACTED

// Get 查询单个监控（解密 API Key）。
func (s *ChannelMonitorService) Get(ctx context.Context, id int64) (*ChannelMonitor, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED
	s.decryptInPlace(m)
	return m, nil
REDACTED

// Create 创建监控（内部加密 api_key）。
func (s *ChannelMonitorService) Create(ctx context.Context, p ChannelMonitorCreateParams) (*ChannelMonitor, error) {
	if err := validateCreateParams(p); err != nil {
		return nil, err
REDACTED
	if err := validateBodyModeForProtocol(p.Provider, p.APIMode, p.BodyOverrideMode, p.BodyOverride); err != nil {
		return nil, err
REDACTED
	if err := validateExtraHeaders(p.ExtraHeaders); err != nil {
		return nil, err
REDACTED
	if err := s.validateLinkedAccount(ctx, p.Provider, p.AccountID); err != nil {
		return nil, err
REDACTED
	checkMode := defaultCheckMode(p.CheckMode)
	encrypted, err := s.encryptor.Encrypt(p.APIKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt api key: %w", err)
REDACTED
	m := &ChannelMonitor{
		Name:             strings.TrimSpace(p.Name),
		Provider:         p.Provider,
		APIMode:          defaultAPIMode(p.APIMode),
		Endpoint:         normalizeEndpoint(p.Endpoint),
		APIKey:           encrypted, // 注意：传入 repository 时该字段为密文
		PrimaryModel:     normalizeMonitorPrimaryModel(p.Provider, checkMode, p.PrimaryModel),
		ExtraModels:      normalizeModels(p.ExtraModels),
		GroupName:        strings.TrimSpace(p.GroupName),
		Enabled:          p.Enabled,
		IntervalSeconds:  p.IntervalSeconds,
		JitterSeconds:    p.JitterSeconds,
		CreatedBy:        p.CreatedBy,
		TemplateID:       p.TemplateID,
		ExtraHeaders:     emptyHeadersIfNil(p.ExtraHeaders),
		BodyOverrideMode: defaultBodyMode(p.BodyOverrideMode),
		BodyOverride:     p.BodyOverride,
		CheckMode:        checkMode,
		AccountID:        cloneInt64Pointer(p.AccountID),
REDACTED
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, fmt.Errorf("create channel monitor: %w", err)
REDACTED
	// 不再调 s.Get 重走解密链：已知刚加密的明文，直接构造响应。
	// 这样可避免 SecretEncryptor 解密失败时 APIKey 被静默清空的问题（见 Fix 4）。
	m.APIKey = strings.TrimSpace(p.APIKey)
	if s.scheduler != nil {
		s.scheduler.Schedule(m)
REDACTED
	return m, nil
REDACTED

// Duplicate creates an independent, disabled copy of an existing monitor.
// The API key stays server-side: it is decrypted only long enough to encrypt a
// fresh ciphertext for the new row. Runtime state and history are not copied.
func (s *ChannelMonitorService) Duplicate(
	ctx context.Context,
	id, createdBy int64,
	actorScope, operationKey string,
) (*ChannelMonitor, error) {
	operationID := duplicateChannelMonitorOperationID(id, actorScope, operationKey)
	existing, err := s.RecoverDuplicate(ctx, id, actorScope, operationKey)
	if err != nil {
		return nil, err
REDACTED
	if existing != nil {
		return existing, nil
REDACTED

	source, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED

	plainAPIKey, err := s.decryptAPIKeyForDuplicate(source)
	if err != nil {
		return nil, err
REDACTED
	encryptedAPIKey, err := s.encryptor.Encrypt(plainAPIKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt duplicate channel monitor api key: %w", err)
REDACTED
	bodyOverride, err := cloneChannelMonitorJSONMap(source.BodyOverride)
	if err != nil {
		return nil, fmt.Errorf("clone duplicate channel monitor body override: %w", err)
REDACTED

	duplicate := &ChannelMonitor{
		Name:                 duplicateChannelMonitorName(source.Name),
		Provider:             source.Provider,
		APIMode:              source.APIMode,
		Endpoint:             source.Endpoint,
		APIKey:               encryptedAPIKey,
		PrimaryModel:         source.PrimaryModel,
		ExtraModels:          append([]string{REDACTED, source.ExtraModels...),
		GroupName:            source.GroupName,
		Enabled:              false,
		IntervalSeconds:      source.IntervalSeconds,
		JitterSeconds:        source.JitterSeconds,
		CreatedBy:            createdBy,
		TemplateID:           cloneInt64Pointer(source.TemplateID),
		ExtraHeaders:         cloneChannelMonitorHeaders(source.ExtraHeaders),
		BodyOverrideMode:     source.BodyOverrideMode,
		BodyOverride:         bodyOverride,
		CheckMode:            defaultCheckMode(source.CheckMode),
		AccountID:            cloneInt64Pointer(source.AccountID),
		DuplicateOperationID: operationID,
REDACTED
	if err := s.repo.Create(ctx, duplicate); err != nil {
		return nil, fmt.Errorf("duplicate channel monitor: %w", err)
REDACTED

	// Match Create/Update response semantics: repository receives ciphertext,
	// while handlers receive plaintext only so they can return the masked form.
	duplicate.APIKey = plainAPIKey
	return duplicate, nil
REDACTED

// RecoverDuplicate performs a read-only lookup for a duplicate that was
// already committed for the same actor, source monitor, and idempotency key.
// It deliberately never repeats the create side effect.
func (s *ChannelMonitorService) RecoverDuplicate(
	ctx context.Context,
	id int64,
	actorScope, operationKey string,
) (*ChannelMonitor, error) {
	operationID := duplicateChannelMonitorOperationID(id, actorScope, operationKey)
	if operationID == "" {
		return nil, nil
REDACTED
	monitor, err := s.repo.FindByDuplicateOperationID(ctx, operationID)
	if err != nil {
		return nil, fmt.Errorf("find duplicate channel monitor operation: %w", err)
REDACTED
	if monitor == nil {
		return nil, nil
REDACTED
	s.decryptInPlace(monitor)
	return monitor, nil
REDACTED

func duplicateChannelMonitorOperationID(sourceID int64, actorScope, operationKey string) string {
	operationKey = strings.TrimSpace(operationKey)
	if operationKey == "" {
		return ""
REDACTED
	actorScope = strings.TrimSpace(actorScope)
	if actorScope == "" {
		actorScope = "admin:0"
REDACTED
	payload := "admin.channel_monitors.duplicate\x00" + actorScope + "\x00" + strconv.FormatInt(sourceID, 10) + "\x00" + operationKey
	digest := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", digest)
REDACTED

func (s *ChannelMonitorService) decryptAPIKeyForDuplicate(source *ChannelMonitor) (string, error) {
	if source == nil || strings.TrimSpace(source.APIKey) == "" {
		return "", ErrChannelMonitorAPIKeyDecryptFailed
REDACTED
	plain, err := s.encryptor.Decrypt(source.APIKey)
	if err != nil {
		slog.Warn("channel_monitor: decrypt api key for duplicate failed",
			"monitor_id", source.ID, "error", err)
		return "", ErrChannelMonitorAPIKeyDecryptFailed
REDACTED
	// quota 模式明文为空串是合法状态（api_key_encrypted 存的是加密空串）：
	// 重加密空串即可。若在此报错，克隆出的配额监控会被 runner 当作
	// 解密失败而 Unschedule，静默停摆。
	if strings.TrimSpace(plain) == "" {
		if monitorCheckModeUsesQuota(defaultCheckMode(source.CheckMode)) {
			return "", nil
	REDACTED
		slog.Warn("channel_monitor: decrypted api key for duplicate is empty",
			"monitor_id", source.ID)
		return "", ErrChannelMonitorAPIKeyDecryptFailed
REDACTED
	return plain, nil
REDACTED

func duplicateChannelMonitorName(sourceName string) string {
	const suffix = " (Copy)"
	nameRunes := []rune(strings.TrimSpace(sourceName))
	maxBaseRunes := maxChannelMonitorNameRunes - len([]rune(suffix))
	if len(nameRunes) > maxBaseRunes {
		nameRunes = nameRunes[:maxBaseRunes]
REDACTED
	return string(nameRunes) + suffix
REDACTED

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
REDACTED
	cloned := *value
	return &cloned
REDACTED

func cloneChannelMonitorHeaders(source map[string]string) map[string]string {
	if source == nil {
		return map[string]string{REDACTED
REDACTED
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
REDACTED
	return cloned
REDACTED

func cloneChannelMonitorJSONMap(source map[string]any) (map[string]any, error) {
	if source == nil {
		return nil, nil
REDACTED
	payload, err := json.Marshal(source)
	if err != nil {
		return nil, err
REDACTED
	cloned := make(map[string]any, len(source))
	if err := json.Unmarshal(payload, &cloned); err != nil {
		return nil, err
REDACTED
	return cloned, nil
REDACTED

// validateCreateParams 把 Create 入参的所有校验聚拢为一个函数，避免 Create 主体超过 30 行。
// 按 check_mode 分支：probe 沿用 endpoint+api_key 必填；quota 只需关联账号；
// quota_probe 两者皆需。
func validateCreateParams(p ChannelMonitorCreateParams) error {
	if err := validateProvider(p.Provider); err != nil {
		return err
REDACTED
	checkMode := defaultCheckMode(p.CheckMode)
	if err := validateCheckMode(p.Provider, checkMode); err != nil {
		return err
REDACTED
	if err := validateAPIMode(p.Provider, p.APIMode); err != nil {
		return err
REDACTED
	if err := validateInterval(p.IntervalSeconds); err != nil {
		return err
REDACTED
	if err := validateJitter(p.JitterSeconds, p.IntervalSeconds); err != nil {
		return err
REDACTED
	usesQuota := monitorCheckModeUsesQuota(checkMode)
	// probe 分支（含 quota_probe 的探活部分）仍需 endpoint + api_key；
	// quota 模式 endpoint/api_key 留空，避免要求用户填无意义的占位值。
	if checkMode != MonitorCheckModeQuota {
		if err := validateEndpoint(p.Endpoint); err != nil {
			return err
	REDACTED
		if strings.TrimSpace(p.APIKey) == "" {
			return ErrChannelMonitorMissingAPIKey
	REDACTED
REDACTED
	if usesQuota && (p.AccountID == nil || *p.AccountID <= 0) {
		return ErrChannelMonitorAccountRequired
REDACTED
	if normalizeMonitorPrimaryModel(p.Provider, checkMode, p.PrimaryModel) == "" {
		return ErrChannelMonitorMissingPrimaryModel
REDACTED
	return nil
REDACTED

// validateLinkedAccount 校验关联账号存在且平台与监控 provider 一致。
// fetcher 未注入时 fail-closed（拒绝创建配额监控，而不是创建后静默坏）。
func (s *ChannelMonitorService) validateLinkedAccount(ctx context.Context, provider string, accountID *int64) error {
	if accountID == nil || *accountID <= 0 {
		return nil
REDACTED
	if s.quotaFetcher == nil {
		return ErrChannelMonitorAccountRequired
REDACTED
	account, err := s.quotaFetcher.LoadAccount(ctx, *accountID)
	if err != nil || account == nil {
		return ErrChannelMonitorAccountRequired
REDACTED
	if account.Platform != provider {
		return ErrChannelMonitorProviderIncompatible
REDACTED
	return nil
REDACTED

// Update 更新监控。APIKey 字段：nil 或空字符串 = 不修改；非空 = 加密后覆盖。
func (s *ChannelMonitorService) Update(ctx context.Context, id int64, p ChannelMonitorUpdateParams) (*ChannelMonitor, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED
	if err := applyMonitorUpdate(existing, p); err != nil {
		return nil, err
REDACTED

	newPlainAPIKey, apiKeyUpdated, err := s.applyAPIKeyUpdate(existing, p.APIKey)
	if err != nil {
		return nil, err
REDACTED
	if err := s.validateProbeAPIKey(existing, newPlainAPIKey); err != nil {
		return nil, err
REDACTED
	if p.Provider != nil || p.CheckMode != nil || p.AccountID != nil {
		if err := s.revalidateLinkedAccount(ctx, existing); err != nil {
			return nil, err
	REDACTED
REDACTED

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("update channel monitor: %w", err)
REDACTED

	// 不再调 s.Get 重走解密链：避免二次解密带来的"密文被静默清空"风险（与 Create 一致）。
	if apiKeyUpdated {
		existing.APIKey = newPlainAPIKey
REDACTED else {
		s.decryptInPlace(existing)
REDACTED
	if s.scheduler != nil {
		// Schedule 内部根据 Enabled 自动选择 Unschedule 或重建任务，
		// IntervalSeconds 变化也会被自然吸收（旧 task 取消 + 新 task 用新 interval）。
		s.scheduler.Schedule(existing)
REDACTED
	return existing, nil
REDACTED

// validateMonitorModeFields 校验 check_mode 与其它字段的组合约束
// （在 provider/check_mode/account_id/endpoint 全部应用后调用）：
//   - quota / quota_probe 必须关联账号
//   - probe / quota_probe 必须持有 endpoint（探活目标）
func validateMonitorModeFields(m *ChannelMonitor) error {
	checkMode := defaultCheckMode(m.CheckMode)
	if monitorCheckModeUsesQuota(checkMode) && m.AccountID == nil {
		return ErrChannelMonitorAccountRequired
REDACTED
	if checkMode != MonitorCheckModeQuota && strings.TrimSpace(m.Endpoint) == "" {
		return ErrChannelMonitorInvalidEndpoint
REDACTED
	return nil
REDACTED

// validateProbeAPIKey 探活模式（probe / quota_probe）必须持有可用明文 key：
// 存量密文解密为空串（quota 监控切回探活但未重填 key）时拒绝。
// 密文损坏的情况交给既有 APIKeyDecryptFailed 链路（Get/RunCheck 会显式报错）。
func (s *ChannelMonitorService) validateProbeAPIKey(m *ChannelMonitor, newPlainKey string) error {
	if defaultCheckMode(m.CheckMode) == MonitorCheckModeQuota {
		return nil
REDACTED
	if strings.TrimSpace(newPlainKey) != "" {
		return nil
REDACTED
	if strings.TrimSpace(m.APIKey) == "" {
		return ErrChannelMonitorMissingAPIKey
REDACTED
	plain, err := s.encryptor.Decrypt(m.APIKey)
	if err != nil {
		return nil
REDACTED
	if strings.TrimSpace(plain) == "" {
		return ErrChannelMonitorMissingAPIKey
REDACTED
	return nil
REDACTED

// revalidateLinkedAccount 在 provider/check_mode/account_id 任一变化后复核关联账号：
//   - 账号已被删除或平台失配：probe 模式自动解绑（静默修复），
//     quota 模式显式报错（配额监控必须有可用数据源）
func (s *ChannelMonitorService) revalidateLinkedAccount(ctx context.Context, m *ChannelMonitor) error {
	usesQuota := monitorCheckModeUsesQuota(defaultCheckMode(m.CheckMode))
	if m.AccountID == nil {
		if usesQuota {
			return ErrChannelMonitorAccountRequired
	REDACTED
		return nil
REDACTED
	if s.quotaFetcher == nil {
		return ErrChannelMonitorAccountRequired
REDACTED
	account, err := s.quotaFetcher.LoadAccount(ctx, *m.AccountID)
	if err != nil || account == nil {
		if usesQuota {
			return ErrChannelMonitorAccountRequired
	REDACTED
		m.AccountID = nil
		return nil
REDACTED
	if account.Platform != m.Provider {
		if usesQuota {
			return ErrChannelMonitorProviderIncompatible
	REDACTED
		m.AccountID = nil
		return nil
REDACTED
	return nil
REDACTED

// applyAPIKeyUpdate 处理 Update 中的 APIKey 字段：
//   - 入参 raw 为 nil 或空白：不修改 existing.APIKey（仍为密文），返回 updated=false
//   - 非空：加密后写入 existing.APIKey；同时把明文返回给调用方，
//     供写库成功后塞回 existing 避免把密文吐回客户端
func (s *ChannelMonitorService) applyAPIKeyUpdate(existing *ChannelMonitor, raw *string) (plain string, updated bool, err error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return "", false, nil
REDACTED
	plain = strings.TrimSpace(*raw)
	encrypted, encErr := s.encryptor.Encrypt(plain)
	if encErr != nil {
		return "", false, fmt.Errorf("encrypt api key: %w", encErr)
REDACTED
	existing.APIKey = encrypted
	return plain, true, nil
REDACTED

// Delete 删除监控（历史通过外键 CASCADE 自动清理）。
func (s *ChannelMonitorService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete channel monitor: %w", err)
REDACTED
	if s.scheduler != nil {
		s.scheduler.Unschedule(id)
REDACTED
	return nil
REDACTED

// ListHistory 列出某个监控最近的检测历史。
// model 为空表示返回所有模型；limit <= 0 时使用默认值，超过上限会被截断。
func (s *ChannelMonitorService) ListHistory(ctx context.Context, id int64, model string, limit int) ([]*ChannelMonitorHistoryEntry, error) {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return nil, err
REDACTED
	if limit <= 0 {
		limit = MonitorHistoryDefaultLimit
REDACTED
	if limit > MonitorHistoryMaxLimit {
		limit = MonitorHistoryMaxLimit
REDACTED
	entries, err := s.repo.ListHistory(ctx, id, strings.TrimSpace(model), limit)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
REDACTED
	return entries, nil
REDACTED

// ---------- 业务 ----------

// RunCheck 同步触发对一个监控的检测：并发跑 primary + extra 模型，
// 写历史记录并更新 last_checked_at。返回每个模型的检测结果。
// 仅当 channel_monitor_enabled=true 且 channel_monitor_mode=v1 时真正探测；
// mode=v2 时返回 ErrChannelMonitorActiveProbesRetired，不产生上游流量。
//
// 按 check_mode 分派：probe（默认，现状探活）/ quota（仅查关联账号配额，
// 零 LLM 成本）/ quota_probe（探活 + 配额快照挂主模型行）。
func (s *ChannelMonitorService) RunCheck(ctx context.Context, id int64) ([]*CheckResult, error) {
	rt := s.probeRuntime(ctx)
	if !rt.Enabled {
		return nil, ErrChannelMonitorDisabled
REDACTED
	if !rt.ActiveProbesAllowed() {
		return nil, ErrChannelMonitorActiveProbesRetired
REDACTED
	m, err := s.Get(ctx, id) // 已解密 APIKey
	if err != nil {
		return nil, err
REDACTED
	checkMode := defaultCheckMode(m.CheckMode)
	if checkMode != MonitorCheckModeQuota && m.APIKeyDecryptFailed {
		return nil, ErrChannelMonitorAPIKeyDecryptFailed
REDACTED

	var results []*CheckResult
	switch checkMode {
	case MonitorCheckModeQuota:
		results = s.runQuotaOnlyCheck(ctx, m)
	case MonitorCheckModeQuotaProbe:
		results = s.runChecksConcurrent(ctx, m)
		attachQuotaSnapshot(results, s.fetchQuotaSnapshot(ctx, m))
	default:
		results = s.runChecksConcurrent(ctx, m)
REDACTED
	s.persistCheckResults(ctx, m, results)
	return results, nil
REDACTED

// runQuotaOnlyCheck quota 模式：一次配额抓取 → 单条 CheckResult
// （Model=PrimaryModel，默认 "quota"；无 ping/latency，状态由快照推导）。
func (s *ChannelMonitorService) runQuotaOnlyCheck(ctx context.Context, m *ChannelMonitor) []*CheckResult {
	snapshot := s.fetchQuotaSnapshot(ctx, m)
	res := deriveQuotaCheckResult(snapshot, m.PrimaryModel, time.Now())
	res.Quota = snapshot
	return []*CheckResult{resREDACTED
REDACTED

// fetchQuotaSnapshot 抓取关联账号配额。未关联账号 / fetcher 未注入时返回
// 显式错误快照（不返回 error，保证检测周期与历史时间线连续）。
func (s *ChannelMonitorService) fetchQuotaSnapshot(ctx context.Context, m *ChannelMonitor) *domain.MonitorQuotaSnapshot {
	if m.AccountID == nil {
		return quotaErrorSnapshot("usage", "linked account not found", time.Now())
REDACTED
	if s.quotaFetcher == nil {
		return quotaErrorSnapshot("usage", "quota fetcher is not configured", time.Now())
REDACTED
	return s.quotaFetcher.Fetch(ctx, *m.AccountID)
REDACTED

// attachQuotaSnapshot quota_probe：把配额快照挂到主模型行（results[0]）。
// 配额失败不改变探活状态，仅在探活 message 为空时附注失败原因。
func attachQuotaSnapshot(results []*CheckResult, snapshot *domain.MonitorQuotaSnapshot) {
	if len(results) == 0 || snapshot == nil {
		return
REDACTED
	primary := results[0]
	primary.Quota = snapshot
	if !snapshot.Success && strings.TrimSpace(primary.Message) == "" {
		primary.Message = truncateMessage("quota fetch failed: " + snapshot.Error)
REDACTED
REDACTED

// persistCheckResults 写入本次检测的历史记录并更新 last_checked_at。
// 任一写库失败都只记日志，不影响调用方拿到 results（与 MVP 期望一致：宁可漏记历史也要先返回结果）。
func (s *ChannelMonitorService) persistCheckResults(ctx context.Context, m *ChannelMonitor, results []*CheckResult) {
	rows := make([]*ChannelMonitorHistoryRow, 0, len(results))
	for _, r := range results {
		rows = append(rows, &ChannelMonitorHistoryRow{
			MonitorID:     m.ID,
			Model:         r.Model,
			Status:        r.Status,
			LatencyMs:     r.LatencyMs,
			PingLatencyMs: r.PingLatencyMs,
			Message:       r.Message,
			CheckedAt:     r.CheckedAt,
			Quota:         r.Quota,
	REDACTED)
REDACTED
	if err := s.repo.InsertHistoryBatch(ctx, rows); err != nil {
		slog.Error("channel_monitor: insert history failed",
			"monitor_id", m.ID, "name", m.Name, "error", err)
REDACTED
	if err := s.repo.MarkChecked(ctx, m.ID, time.Now()); err != nil {
		slog.Error("channel_monitor: mark checked failed",
			"monitor_id", m.ID, "error", err)
REDACTED
REDACTED

// runChecksConcurrent 对 primary + extra 模型并发执行检测。
// errgroup 仅用于等待，不传播错误（每个 model 失败都已打包进 CheckResult）。
func (s *ChannelMonitorService) runChecksConcurrent(ctx context.Context, m *ChannelMonitor) []*CheckResult {
	models := append([]string{m.PrimaryModelREDACTED, m.ExtraModels...)
	results := make([]*CheckResult, len(models))

	// ping 共享一次，所有模型记录同一个 ping 延迟。
	pingMs := pingEndpointOrigin(ctx, m.Endpoint)

	// 所有模型共用同一份 CheckOptions（来自监控的快照字段）。
	opts := &CheckOptions{
		APIMode:          m.APIMode,
		ExtraHeaders:     m.ExtraHeaders,
		BodyOverrideMode: m.BodyOverrideMode,
		BodyOverride:     m.BodyOverride,
REDACTED

	var eg errgroup.Group
	var mu sync.Mutex
	for i, model := range models {
		i, model := i, model
		eg.Go(func() error {
			r := runCheckForModel(ctx, m.Provider, m.Endpoint, m.APIKey, model, opts)
			r.PingLatencyMs = pingMs
			mu.Lock()
			results[i] = r
			mu.Unlock()
			return nil
	REDACTED)
REDACTED
	_ = eg.Wait()
	return results
REDACTED

// ---------- 调度器协作 ----------

// SetScheduler 由 wire 在 runner 构造后注入，用于在 CRUD 时即时同步任务表。
// 通过 setter 注入避免 service ↔ runner 的依赖环。
func (s *ChannelMonitorService) SetScheduler(sched MonitorScheduler) {
	s.scheduler = sched
REDACTED

// SetQuotaFetcher 由 wire 注入配额抓取器（账号侧用量服务聚合）。
func (s *ChannelMonitorService) SetQuotaFetcher(fetcher *ChannelMonitorQuotaFetcher) {
	if s == nil {
		return
REDACTED
	s.quotaFetcher = fetcher
REDACTED

// ListEnabledMonitors 返回所有 enabled=true 的监控（解密后），供 runner 启动时建立任务表。
func (s *ChannelMonitorService) ListEnabledMonitors(ctx context.Context) ([]*ChannelMonitor, error) {
	all, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return nil, err
REDACTED
	for _, m := range all {
		s.decryptInPlace(m)
REDACTED
	return all, nil
REDACTED

// cleanupOldHistory 删除 monitorHistoryRetentionDays 天之前的明细历史记录。
// 由 RunDailyMaintenance 调用；SoftDeleteMixin 自动把 DELETE 改为 UPDATE deleted_at。
func (s *ChannelMonitorService) cleanupOldHistory(ctx context.Context) error {
	before := time.Now().UTC().AddDate(0, 0, -monitorHistoryRetentionDays)
	deleted, err := s.repo.DeleteHistoryBefore(ctx, before)
	if err != nil {
		return fmt.Errorf("delete history before %s: %w", before.Format(time.RFC3339), err)
REDACTED
	if deleted > 0 {
		slog.Info("channel_monitor: history cleanup",
			"deleted_rows", deleted, "before", before.Format(time.RFC3339))
REDACTED
	return nil
REDACTED

// RunDailyMaintenance 每日维护任务：聚合昨天之前未聚合的明细，软删过期明细和聚合。
// 由 OpsCleanupService 的 cron 调度触发（共享 schedule 和 leader lock）。
//
// 幂等性：
//   - watermark 保证已聚合的日期不会重复处理；
//   - UpsertDailyRollupsFor 内部使用 ON CONFLICT DO UPDATE，同一日重复跑结果一致。
//
// 每一步失败都只记 slog.Warn，整体函数始终返回 nil 让后续步骤能继续跑
// （与 OpsCleanupService.runCleanupOnce 风格一致）。
func (s *ChannelMonitorService) RunDailyMaintenance(ctx context.Context) error {
	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)

	if err := s.runDailyAggregation(ctx, today); err != nil {
		slog.Warn("channel_monitor: maintenance step failed",
			"step", "aggregate", "error", err)
REDACTED
	if err := s.cleanupOldHistory(ctx); err != nil {
		slog.Warn("channel_monitor: maintenance step failed",
			"step", "prune_history", "error", err)
REDACTED
	if err := s.cleanupOldRollups(ctx, today); err != nil {
		slog.Warn("channel_monitor: maintenance step failed",
			"step", "prune_rollups", "error", err)
REDACTED
	return nil
REDACTED

// runDailyAggregation 从 watermark+1 聚合到昨天（UTC）。
// 首次跑（watermark nil）：从 today-monitorRollupRetentionDays 开始回填。
// 每次最多聚合 monitorMaintenanceMaxDaysPerRun 天，避免长事务。
func (s *ChannelMonitorService) runDailyAggregation(ctx context.Context, today time.Time) error {
	watermark, err := s.repo.LoadAggregationWatermark(ctx)
	if err != nil {
		return fmt.Errorf("load watermark: %w", err)
REDACTED

	start := s.resolveAggregationStart(watermark, today)
	if !start.Before(today) {
		return nil // 没有需要聚合的日期
REDACTED

	iterations := 0
	for d := start; d.Before(today); d = d.Add(24 * time.Hour) {
		if iterations >= monitorMaintenanceMaxDaysPerRun {
			slog.Info("channel_monitor: maintenance aggregation capped",
				"max_days", monitorMaintenanceMaxDaysPerRun,
				"next_resume", d.Format("2006-01-02"))
			break
	REDACTED
		affected, upErr := s.repo.UpsertDailyRollupsFor(ctx, d)
		if upErr != nil {
			return fmt.Errorf("upsert rollups for %s: %w", d.Format("2006-01-02"), upErr)
	REDACTED
		if err := s.repo.UpdateAggregationWatermark(ctx, d); err != nil {
			return fmt.Errorf("update watermark to %s: %w", d.Format("2006-01-02"), err)
	REDACTED
		slog.Info("channel_monitor: rollups upserted",
			"date", d.Format("2006-01-02"), "affected_rows", affected)
		iterations++
REDACTED
	return nil
REDACTED

// resolveAggregationStart 计算本次聚合起点：
//   - watermark == nil：today - monitorRollupRetentionDays（首次回填最多 30 天）
//   - watermark != nil：*watermark + 1 day
func (s *ChannelMonitorService) resolveAggregationStart(watermark *time.Time, today time.Time) time.Time {
	if watermark == nil {
		return today.AddDate(0, 0, -monitorRollupRetentionDays)
REDACTED
	return watermark.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
REDACTED

// cleanupOldRollups 软删 bucket_date < today - monitorRollupRetentionDays 的日聚合行。
func (s *ChannelMonitorService) cleanupOldRollups(ctx context.Context, today time.Time) error {
	cutoff := today.AddDate(0, 0, -monitorRollupRetentionDays)
	deleted, err := s.repo.DeleteRollupsBefore(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("delete rollups before %s: %w", cutoff.Format("2006-01-02"), err)
REDACTED
	if deleted > 0 {
		slog.Info("channel_monitor: rollups cleanup",
			"deleted_rows", deleted, "before", cutoff.Format("2006-01-02"))
REDACTED
	return nil
REDACTED

// ---------- helpers ----------

// decryptInPlace 把 ChannelMonitor.APIKey 从密文解密为明文。
// 解密失败时把字段清空 + 设置 APIKeyDecryptFailed=true（不返回错误，避免阻断列表渲染）。
// runner / RunCheck 必须读取该标志位并拒绝执行检测。
func (s *ChannelMonitorService) decryptInPlace(m *ChannelMonitor) {
	if m == nil || m.APIKey == "" {
		return
REDACTED
	plain, err := s.encryptor.Decrypt(m.APIKey)
	if err != nil {
		slog.Warn("channel_monitor: decrypt api key failed",
			"monitor_id", m.ID, "error", err)
		m.APIKey = ""
		m.APIKeyDecryptFailed = true
		return
REDACTED
	m.APIKey = plain
REDACTED

// applyMonitorUpdate 把 update params 中非 nil 的字段应用到 existing 上。
// APIKey 字段在调用方单独处理（涉及加密）。
//
// 行数稍超过 30：这是逐字段平铺的 dispatcher，每个 if 都是 1-3 行的"非 nil 则覆盖"模式，
// 拆分反而会增加跳转噪音、影响可读性，故保留为单函数。
func applyMonitorUpdate(existing *ChannelMonitor, p ChannelMonitorUpdateParams) error {
	providerChanged := false
	if p.Name != nil {
		existing.Name = strings.TrimSpace(*p.Name)
REDACTED
	if p.Provider != nil {
		if err := validateProvider(*p.Provider); err != nil {
			return err
	REDACTED
		providerChanged = existing.Provider != *p.Provider
		existing.Provider = *p.Provider
REDACTED
	if p.CheckMode != nil {
		mode := defaultCheckMode(*p.CheckMode)
		if err := validateCheckMode(existing.Provider, mode); err != nil {
			return err
	REDACTED
		existing.CheckMode = mode
REDACTED
	if p.AccountID != nil {
		if *p.AccountID > 0 {
			id := *p.AccountID
			existing.AccountID = &id
	REDACTED else {
			existing.AccountID = nil // 0 = 清空关联
	REDACTED
REDACTED
	if p.Endpoint != nil {
		// quota 模式允许清空 endpoint（校验由 validateMonitorModeFields 兜底）。
		if strings.TrimSpace(*p.Endpoint) != "" {
			if err := validateEndpoint(*p.Endpoint); err != nil {
				return err
		REDACTED
	REDACTED
		existing.Endpoint = normalizeEndpoint(*p.Endpoint)
REDACTED
	// 模式与字段的组合校验（provider/check_mode/account_id/endpoint 全部应用后）。
	if err := validateMonitorModeFields(existing); err != nil {
		return err
REDACTED
	if p.PrimaryModel != nil {
		primaryModel := normalizeMonitorPrimaryModel(existing.Provider, defaultCheckMode(existing.CheckMode), *p.PrimaryModel)
		if primaryModel == "" {
			return ErrChannelMonitorMissingPrimaryModel
	REDACTED
		existing.PrimaryModel = primaryModel
REDACTED else if providerChanged && existing.Provider == MonitorProviderGrok {
		existing.PrimaryModel = MonitorDefaultGrokModel
REDACTED
	if p.ExtraModels != nil {
		existing.ExtraModels = normalizeModels(*p.ExtraModels)
REDACTED
	if p.GroupName != nil {
		existing.GroupName = strings.TrimSpace(*p.GroupName)
REDACTED
	if p.Enabled != nil {
		existing.Enabled = *p.Enabled
REDACTED
	if p.IntervalSeconds != nil {
		if err := validateInterval(*p.IntervalSeconds); err != nil {
			return err
	REDACTED
		existing.IntervalSeconds = *p.IntervalSeconds
REDACTED
	if p.JitterSeconds != nil {
		existing.JitterSeconds = *p.JitterSeconds
REDACTED
	if p.IntervalSeconds != nil || p.JitterSeconds != nil {
		// interval 与 jitter 任一变化都需要重新校验组合约束（interval - jitter >= 下限）。
		if err := validateJitter(existing.JitterSeconds, existing.IntervalSeconds); err != nil {
			return err
	REDACTED
REDACTED
	return applyMonitorAdvancedUpdate(existing, p, providerChanged)
REDACTED

// applyMonitorAdvancedUpdate 处理自定义请求快照相关字段，从 applyMonitorUpdate 拆出避免过长。
func applyMonitorAdvancedUpdate(existing *ChannelMonitor, p ChannelMonitorUpdateParams, providerChanged bool) error {
	if p.ClearTemplate {
		existing.TemplateID = nil
REDACTED else if p.TemplateID != nil {
		id := *p.TemplateID
		existing.TemplateID = &id
REDACTED
	if p.ExtraHeaders != nil {
		if err := validateExtraHeaders(*p.ExtraHeaders); err != nil {
			return err
	REDACTED
		existing.ExtraHeaders = emptyHeadersIfNil(*p.ExtraHeaders)
REDACTED
	newAPIMode := defaultAPIMode(existing.APIMode)
	if p.APIMode != nil {
		newAPIMode = defaultAPIMode(*p.APIMode)
REDACTED else if existing.Provider != MonitorProviderOpenAI {
		newAPIMode = MonitorAPIModeChatCompletions
REDACTED
	if err := validateAPIMode(existing.Provider, newAPIMode); err != nil {
		return err
REDACTED
	// BodyOverrideMode / BodyOverride 联合校验，和模板一致。
	newMode := existing.BodyOverrideMode
	newBody := existing.BodyOverride
	if p.BodyOverrideMode != nil {
		newMode = *p.BodyOverrideMode
REDACTED
	if p.BodyOverride != nil {
		newBody = *p.BodyOverride
REDACTED
	if providerChanged || p.APIMode != nil || p.BodyOverrideMode != nil || p.BodyOverride != nil {
		if err := validateBodyModeForProtocol(existing.Provider, newAPIMode, newMode, newBody); err != nil {
			return err
	REDACTED
		existing.BodyOverrideMode = defaultBodyMode(newMode)
		existing.BodyOverride = newBody
REDACTED
	existing.APIMode = newAPIMode
	return nil
REDACTED
