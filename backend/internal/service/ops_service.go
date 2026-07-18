package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrOpsDisabled = infraerrors.NotFound("OPS_DISABLED", "Ops monitoring is disabled")

const (
	opsMaxStoredErrorBodyBytes = 20 * 1024
	// OpsErrorLogQueueBodyMaxBytes bounds attacker-controlled response data while
	// it waits in the asynchronous error-log queue.
	OpsErrorLogQueueBodyMaxBytes = 8 * 1024

	opsRuntimeSettingsRefreshInterval = 30 * time.Second
	opsRuntimeSettingsRefreshJitter   = 20
	opsRuntimeSettingsRefreshTimeout  = 3 * time.Second
	opsRuntimeSettingsFailureLogEvery = time.Minute
)

type opsRuntimeSettingsSnapshot struct {
	monitoringEnabled bool
	advanced          OpsAdvancedSettings
REDACTED

type OpsRuntimeSettingsRefreshHealth struct {
	Running      bool   `json:"running"`
	SuccessTotal uint64 `json:"success_total"`
	FailureTotal uint64 `json:"failure_total"`
REDACTED

// OpsService provides ingestion and query APIs for the Ops monitoring module.
type OpsService struct {
	opsRepo     OpsRepository
	settingRepo SettingRepository
	cfg         *config.Config

	accountRepo AccountRepository
	userRepo    UserRepository

	// getAccountAvailability is a unit-test hook for overriding account availability lookup.
	getAccountAvailability func(ctx context.Context, platformFilter string, groupIDFilter *int64) (*OpsAccountAvailability, error)

	concurrencyService          *ConcurrencyService
	gatewayService              *GatewayService
	openAIGatewayService        *OpenAIGatewayService
	geminiCompatService         *GeminiMessagesCompatService
	antigravityGatewayService   *AntigravityGatewayService
	systemLogSink               *OpsSystemLogSink
	ingressRejectAggregator     *OpsIngressRejectAggregator
	authCacheInvalidationWorker *AuthCacheInvalidationWorker
	apiKeyService               *APIKeyService

	// cleanupReloader 由 wire 在 OpsCleanupService 构造完成后通过 SetCleanupReloader 注入。
	// 解耦避免 OpsService -> OpsCleanupService 的硬依赖（cleanup 也读 settings，会循环）。
	cleanupReloader CleanupReloader

	// quotaAutoPauseSink 由 wire 注入（通常是 SettingService.SetOpenAIQuotaAutoPauseSettings）。
	// UpdateOpsAdvancedSettings 写入新配置后调用，把最新的 quota auto-pause 全局默认阈值
	// 立即同步到调度热路径读取的内存缓存，避免下次请求才能感知新值。
	quotaAutoPauseSink func(OpsOpenAIAccountQuotaAutoPauseSettings)

	// Published snapshots are immutable. Gateway reads are lock-free; the mutex
	// only serializes startup and administrative updates.
	runtimeSettings   atomic.Pointer[opsRuntimeSettingsSnapshot]
	runtimeSettingsMu sync.Mutex

	runtimeRefreshMu             sync.Mutex
	runtimeRefreshCancel         context.CancelFunc
	runtimeRefreshDone           chan struct{REDACTED
	runtimeRefreshRunning        atomic.Bool
	runtimeRefreshSuccess        atomic.Uint64
	runtimeRefreshFailure        atomic.Uint64
	runtimeRefreshLastFailureLog atomic.Int64
REDACTED

// CleanupReloader 由 OpsCleanupService 实现。
// UpdateOpsAdvancedSettings 写入新配置后调用 Reload，让 schedule/enabled 改动立刻生效。
type CleanupReloader interface {
	Reload(ctx context.Context) error
REDACTED

// SetCleanupReloader 由 wire 注入 cleanup hook（构造期循环依赖的解耦点）。
func (s *OpsService) SetCleanupReloader(r CleanupReloader) {
	if s == nil {
		return
REDACTED
	s.cleanupReloader = r
REDACTED

// SetOpenAIQuotaAutoPauseSettingsSink 由 wire 注入，把最新的 quota auto-pause 全局默认
// 阈值 push 到调度热路径读取的内存缓存。同 SetCleanupReloader 的解耦目的：避免 OpsService
// 持有 *SettingService 引入循环依赖。
func (s *OpsService) SetOpenAIQuotaAutoPauseSettingsSink(sink func(OpsOpenAIAccountQuotaAutoPauseSettings)) {
	if s == nil {
		return
REDACTED
	s.quotaAutoPauseSink = sink
REDACTED

func NewOpsService(
	opsRepo OpsRepository,
	settingRepo SettingRepository,
	cfg *config.Config,
	accountRepo AccountRepository,
	userRepo UserRepository,
	concurrencyService *ConcurrencyService,
	gatewayService *GatewayService,
	openAIGatewayService *OpenAIGatewayService,
	geminiCompatService *GeminiMessagesCompatService,
	antigravityGatewayService *AntigravityGatewayService,
	systemLogSink *OpsSystemLogSink,
) *OpsService {
	svc := &OpsService{
		opsRepo:     opsRepo,
		settingRepo: settingRepo,
		cfg:         cfg,

		accountRepo: accountRepo,
		userRepo:    userRepo,

		concurrencyService:        concurrencyService,
		gatewayService:            gatewayService,
		openAIGatewayService:      openAIGatewayService,
		geminiCompatService:       geminiCompatService,
		antigravityGatewayService: antigravityGatewayService,
		systemLogSink:             systemLogSink,
REDACTED
	svc.initRuntimeSettings(context.Background())
	svc.applyRuntimeLogConfigOnStartup(context.Background())
	return svc
REDACTED

func (s *OpsService) RequireMonitoringEnabled(ctx context.Context) error {
	if s.IsMonitoringEnabled(ctx) {
		return nil
REDACTED
	return ErrOpsDisabled
REDACTED

func (s *OpsService) IsMonitoringEnabled(ctx context.Context) bool {
	_ = ctx
	// Hard switch: disable ops entirely.
	if s.cfg != nil && !s.cfg.Ops.Enabled {
		return false
REDACTED
	if snapshot := s.runtimeSettings.Load(); snapshot != nil {
		return snapshot.monitoringEnabled
REDACTED
	// Directly assembled test services and failed cold loads remain fail-open,
	// without turning a request into a settings-table lookup.
	return true
REDACTED

func parseOpsMonitoringEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "false", "0", "off", "disabled":
		return false
	default:
		return true
REDACTED
REDACTED

func (s *OpsService) initRuntimeSettings(ctx context.Context) {
	if s == nil {
		return
REDACTED
	defaults := defaultOpsAdvancedSettings()
	s.runtimeSettings.Store(&opsRuntimeSettingsSnapshot{monitoringEnabled: true, advanced: *defaultsREDACTED)
	_ = s.RefreshRuntimeSettings(ctx)
REDACTED

// RefreshRuntimeSettings is the cold-path database load used at startup and by
// explicit administrative refreshes. Request processing only reads the atomic
// snapshot.
func (s *OpsService) RefreshRuntimeSettings(ctx context.Context) error {
	if s == nil || s.settingRepo == nil {
		return nil
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	s.runtimeSettingsMu.Lock()
	defer s.runtimeSettingsMu.Unlock()

	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyOpsMonitoringEnabled,
		SettingKeyOpsAdvancedSettings,
REDACTED)
	if err != nil {
		return err
REDACTED

	monitoringEnabled := true
	if raw, ok := values[SettingKeyOpsMonitoringEnabled]; ok {
		monitoringEnabled = parseOpsMonitoringEnabled(raw)
REDACTED
	advanced := defaultOpsAdvancedSettings()
	if raw, ok := values[SettingKeyOpsAdvancedSettings]; ok {
		if err := json.Unmarshal([]byte(raw), advanced); err != nil {
			advanced = defaultOpsAdvancedSettings()
	REDACTED
REDACTED
	normalizeOpsAdvancedSettings(advanced)

	s.runtimeSettings.Store(&opsRuntimeSettingsSnapshot{monitoringEnabled: monitoringEnabled, advanced: *advancedREDACTED)
	return nil
REDACTED

// StartRuntimeSettingsRefresh keeps DB-backed Ops settings converged across
// application instances without putting database I/O on request paths.
func (s *OpsService) StartRuntimeSettingsRefresh(ctx context.Context) {
	s.startRuntimeSettingsRefresh(ctx, opsRuntimeSettingsRefreshInterval, opsRuntimeSettingsRefreshJitter, opsRuntimeSettingsRefreshTimeout)
REDACTED

func (s *OpsService) startRuntimeSettingsRefresh(ctx context.Context, interval time.Duration, jitterPercent int, timeout time.Duration) {
	if s == nil || s.settingRepo == nil {
		return
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if interval <= 0 {
		interval = opsRuntimeSettingsRefreshInterval
REDACTED
	if timeout <= 0 {
		timeout = opsRuntimeSettingsRefreshTimeout
REDACTED
	if jitterPercent < 0 {
		jitterPercent = 0
REDACTED
	if jitterPercent > 100 {
		jitterPercent = 100
REDACTED

	s.runtimeRefreshMu.Lock()
	if s.runtimeRefreshCancel != nil {
		s.runtimeRefreshMu.Unlock()
		return
REDACTED
	refreshCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{REDACTED)
	s.runtimeRefreshCancel = cancel
	s.runtimeRefreshDone = done
	s.runtimeRefreshRunning.Store(true)
	s.runtimeRefreshMu.Unlock()

	go func() {
		defer close(done)
		defer s.runtimeRefreshRunning.Store(false)
		for {
			delay := jitterDuration(interval, jitterPercent)
			timer := time.NewTimer(delay)
			select {
			case <-refreshCtx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
				REDACTED
			REDACTED
				return
			case <-timer.C:
		REDACTED

			attemptCtx, attemptCancel := context.WithTimeout(refreshCtx, timeout)
			err := s.RefreshRuntimeSettings(attemptCtx)
			attemptCancel()
			if err != nil {
				s.runtimeRefreshFailure.Add(1)
				s.logRuntimeSettingsRefreshFailure(err)
				continue
		REDACTED
			s.runtimeRefreshSuccess.Add(1)
	REDACTED
REDACTED()
REDACTED

func jitterDuration(base time.Duration, percent int) time.Duration {
	if base <= 0 || percent <= 0 {
		return base
REDACTED
	delta := float64(percent) / 100
	factor := 1 - delta + rand.Float64()*(2*delta)
	if factor <= 0 {
		return base
REDACTED
	return time.Duration(float64(base) * factor)
REDACTED

func (s *OpsService) logRuntimeSettingsRefreshFailure(err error) {
	if s == nil || err == nil {
		return
REDACTED
	now := time.Now().Unix()
	for {
		last := s.runtimeRefreshLastFailureLog.Load()
		if last != 0 && now-last < int64(opsRuntimeSettingsFailureLogEvery/time.Second) {
			return
	REDACTED
		if s.runtimeRefreshLastFailureLog.CompareAndSwap(last, now) {
			log.Printf("[Ops] runtime settings refresh failed: %v", err)
			return
	REDACTED
REDACTED
REDACTED

// StopRuntimeSettingsRefresh is idempotent and waits for an in-flight refresh
// to observe cancellation before returning.
func (s *OpsService) StopRuntimeSettingsRefresh() {
	if s == nil {
		return
REDACTED
	s.runtimeRefreshMu.Lock()
	cancel := s.runtimeRefreshCancel
	done := s.runtimeRefreshDone
	s.runtimeRefreshMu.Unlock()
	if cancel != nil {
		cancel()
REDACTED
	if done != nil {
		<-done
REDACTED
	s.runtimeRefreshMu.Lock()
	if s.runtimeRefreshDone == done {
		s.runtimeRefreshCancel = nil
		s.runtimeRefreshDone = nil
REDACTED
	s.runtimeRefreshMu.Unlock()
REDACTED

func (s *OpsService) RuntimeSettingsRefreshHealth() OpsRuntimeSettingsRefreshHealth {
	if s == nil {
		return OpsRuntimeSettingsRefreshHealth{REDACTED
REDACTED
	return OpsRuntimeSettingsRefreshHealth{
		Running:      s.runtimeRefreshRunning.Load(),
		SuccessTotal: s.runtimeRefreshSuccess.Load(),
		FailureTotal: s.runtimeRefreshFailure.Load(),
REDACTED
REDACTED

// SetMonitoringEnabled publishes an already-persisted admin setting without a
// database round trip.
func (s *OpsService) SetMonitoringEnabled(enabled bool) {
	if s == nil {
		return
REDACTED
	s.runtimeSettingsMu.Lock()
	current := s.runtimeSettings.Load()
	next := &opsRuntimeSettingsSnapshot{monitoringEnabled: enabled, advanced: *defaultOpsAdvancedSettings()REDACTED
	if current != nil {
		next.advanced = current.advanced
REDACTED
	s.runtimeSettings.Store(next)
	s.runtimeSettingsMu.Unlock()
REDACTED

func (s *OpsService) storeAdvancedSettingsSnapshot(cfg *OpsAdvancedSettings) {
	if s == nil || cfg == nil {
		return
REDACTED
	s.runtimeSettingsMu.Lock()
	current := s.runtimeSettings.Load()
	next := &opsRuntimeSettingsSnapshot{monitoringEnabled: true, advanced: *cfgREDACTED
	if current != nil {
		next.monitoringEnabled = current.monitoringEnabled
REDACTED
	s.runtimeSettings.Store(next)
	s.runtimeSettingsMu.Unlock()
REDACTED

// SanitizeOpsErrorBodyForQueue removes credentials and truncates the body
// before it can consume capacity in the asynchronous queue.
func SanitizeOpsErrorBodyForQueue(raw string) (string, bool) {
	return sanitizeErrorBodyForStorage(raw, OpsErrorLogQueueBodyMaxBytes)
REDACTED

// SanitizeOpsUpstreamErrorsForQueue bounds and serializes attempt-level data
// before the entry can consume asynchronous queue capacity.
func SanitizeOpsUpstreamErrorsForQueue(entry *OpsInsertErrorLogInput) error {
	return sanitizeOpsUpstreamErrors(entry)
REDACTED

func (s *OpsService) RecordError(ctx context.Context, entry *OpsInsertErrorLogInput) error {
	prepared, ok, err := s.prepareErrorLogInput(ctx, entry)
	if err != nil {
		log.Printf("[Ops] RecordError prepare failed: %v", err)
		return err
REDACTED
	if !ok {
		return nil
REDACTED

	if _, err := s.opsRepo.InsertErrorLog(ctx, prepared); err != nil {
		// Never bubble up to gateway; best-effort logging.
		log.Printf("[Ops] RecordError failed: %v", err)
		return err
REDACTED
	return nil
REDACTED

func (s *OpsService) RecordErrorBatch(ctx context.Context, entries []*OpsInsertErrorLogInput) error {
	if len(entries) == 0 {
		return nil
REDACTED
	prepared := make([]*OpsInsertErrorLogInput, 0, len(entries))
	for _, entry := range entries {
		item, ok, err := s.prepareErrorLogInput(ctx, entry)
		if err != nil {
			log.Printf("[Ops] RecordErrorBatch prepare failed: %v", err)
			continue
	REDACTED
		if ok {
			prepared = append(prepared, item)
	REDACTED
REDACTED
	if len(prepared) == 0 {
		return nil
REDACTED
	if len(prepared) == 1 {
		_, err := s.opsRepo.InsertErrorLog(ctx, prepared[0])
		if err != nil {
			log.Printf("[Ops] RecordErrorBatch single insert failed: %v", err)
	REDACTED
		return err
REDACTED

	if _, err := s.opsRepo.BatchInsertErrorLogs(ctx, prepared); err != nil {
		log.Printf("[Ops] RecordErrorBatch failed, fallback to single inserts: %v", err)
		var firstErr error
		for _, entry := range prepared {
			if _, insertErr := s.opsRepo.InsertErrorLog(ctx, entry); insertErr != nil {
				log.Printf("[Ops] RecordErrorBatch fallback insert failed: %v", insertErr)
				if firstErr == nil {
					firstErr = insertErr
			REDACTED
		REDACTED
	REDACTED
		return firstErr
REDACTED
	return nil
REDACTED

func (s *OpsService) prepareErrorLogInput(ctx context.Context, entry *OpsInsertErrorLogInput) (*OpsInsertErrorLogInput, bool, error) {
	if entry == nil {
		return nil, false, nil
REDACTED
	if !s.IsMonitoringEnabled(ctx) {
		return nil, false, nil
REDACTED
	if s.opsRepo == nil {
		return nil, false, nil
REDACTED

	// Ensure timestamps are always populated.
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
REDACTED

	// Ensure required fields exist (DB has NOT NULL constraints).
	entry.ErrorPhase = strings.TrimSpace(entry.ErrorPhase)
	entry.ErrorType = strings.TrimSpace(entry.ErrorType)
	if entry.ErrorPhase == "" {
		entry.ErrorPhase = "internal"
REDACTED
	if entry.ErrorType == "" {
		entry.ErrorType = "api_error"
REDACTED

	// Credential acquisition is a gateway/account-auth stage, not an inference
	// HTTP attempt. Enforce that ownership at the persistence boundary so an
	// earlier inference attempt cannot leak its status or text into top-level
	// auth fields even if a caller supplied stale single-value context.
	for i := len(entry.UpstreamErrors) - 1; i >= 0; i-- {
		last := entry.UpstreamErrors[i]
		if last == nil {
			continue
	REDACTED
		if last.Stage == string(GatewayFailureStageAccountAuth) {
			entry.ErrorPhase = string(GatewayFailureStageAccountAuth)
			entry.ErrorOwner = "provider"
			entry.ErrorSource = "gateway"
			code := 0
			entry.UpstreamStatusCode = &code
			entry.UpstreamErrorMessage = nil
			if message := strings.TrimSpace(last.Message); message != "" {
				entry.UpstreamErrorMessage = &message
		REDACTED
			entry.UpstreamErrorDetail = nil
			if detail := strings.TrimSpace(last.Detail); detail != "" {
				entry.UpstreamErrorDetail = &detail
		REDACTED
	REDACTED
		break
REDACTED

	// Sanitize + truncate error_body to avoid storing sensitive data.
	if strings.TrimSpace(entry.ErrorBody) != "" {
		sanitized, _ := sanitizeErrorBodyForStorage(entry.ErrorBody, opsMaxStoredErrorBodyBytes)
		entry.ErrorBody = sanitized
REDACTED

	// Sanitize upstream error context if provided by gateway services.
	if entry.UpstreamStatusCode != nil && *entry.UpstreamStatusCode <= 0 && entry.ErrorPhase != string(GatewayFailureStageAccountAuth) {
		entry.UpstreamStatusCode = nil
REDACTED
	if entry.UpstreamErrorMessage != nil {
		msg := strings.TrimSpace(*entry.UpstreamErrorMessage)
		msg = sanitizeUpstreamErrorMessage(msg)
		msg = truncateString(msg, 2048)
		if strings.TrimSpace(msg) == "" {
			entry.UpstreamErrorMessage = nil
	REDACTED else {
			entry.UpstreamErrorMessage = &msg
	REDACTED
REDACTED
	if entry.UpstreamErrorDetail != nil {
		detail := strings.TrimSpace(*entry.UpstreamErrorDetail)
		if detail == "" {
			entry.UpstreamErrorDetail = nil
	REDACTED else {
			sanitized, _ := sanitizeErrorBodyForStorage(detail, opsMaxStoredErrorBodyBytes)
			if strings.TrimSpace(sanitized) == "" {
				entry.UpstreamErrorDetail = nil
		REDACTED else {
				entry.UpstreamErrorDetail = &sanitized
		REDACTED
	REDACTED
REDACTED

	if err := sanitizeOpsUpstreamErrors(entry); err != nil {
		return nil, false, err
REDACTED

	return entry, true, nil
REDACTED

func sanitizeOpsUpstreamErrors(entry *OpsInsertErrorLogInput) error {
	if entry == nil || len(entry.UpstreamErrors) == 0 {
		return nil
REDACTED

	const maxEvents = 16
	events := entry.UpstreamErrors
	if len(events) > maxEvents {
		events = events[len(events)-maxEvents:]
REDACTED

	sanitized := make([]*OpsUpstreamErrorEvent, 0, len(events))
	for _, ev := range events {
		if ev == nil {
			continue
	REDACTED
		out := *ev

		out.Platform = truncateString(strings.TrimSpace(out.Platform), 32)
		out.AccountName = truncateString(strings.TrimSpace(out.AccountName), 128)
		out.UpstreamRequestID = truncateString(strings.TrimSpace(out.UpstreamRequestID), 128)
		out.UpstreamURL = truncateString(strings.TrimSpace(out.UpstreamURL), 2048)
		if body := strings.TrimSpace(out.UpstreamResponseBody); body != "" {
			out.UpstreamResponseBody, _ = sanitizeErrorBodyForStorage(body, OpsErrorLogQueueBodyMaxBytes)
	REDACTED else {
			out.UpstreamResponseBody = ""
	REDACTED
		out.Kind = truncateString(strings.TrimSpace(out.Kind), 64)
		out.Stage = truncateString(strings.TrimSpace(out.Stage), 64)
		out.Scope = truncateString(strings.TrimSpace(out.Scope), 64)
		out.Reason = truncateString(strings.TrimSpace(out.Reason), 128)

		if out.AccountID < 0 {
			out.AccountID = 0
	REDACTED
		if out.UpstreamStatusCode < 0 {
			out.UpstreamStatusCode = 0
	REDACTED
		if out.AtUnixMs < 0 {
			out.AtUnixMs = 0
	REDACTED

		msg := sanitizeUpstreamErrorMessage(strings.TrimSpace(out.Message))
		msg = truncateString(msg, 2048)
		out.Message = msg

		detail := strings.TrimSpace(out.Detail)
		if detail != "" {
			// Keep upstream detail small while the event waits in the queue.
			sanitizedDetail, _ := sanitizeErrorBodyForStorage(detail, OpsErrorLogQueueBodyMaxBytes)
			out.Detail = sanitizedDetail
	REDACTED else {
			out.Detail = ""
	REDACTED

		// Drop fully-empty events (can happen if only status code was known).
		if out.UpstreamStatusCode == 0 && out.Message == "" && out.Detail == "" {
			continue
	REDACTED

		evCopy := out
		sanitized = append(sanitized, &evCopy)
REDACTED

	entry.UpstreamErrorsJSON = marshalOpsUpstreamErrors(sanitized)
	entry.UpstreamErrors = nil
	return nil
REDACTED

func (s *OpsService) GetErrorLogs(ctx context.Context, filter *OpsErrorLogFilter) (*OpsErrorLogList, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
REDACTED
	if s.opsRepo == nil {
		return &OpsErrorLogList{Errors: []*OpsErrorLog{REDACTED, Total: 0, Page: 1, PageSize: 20REDACTED, nil
REDACTED
	result, err := s.opsRepo.ListErrorLogs(ctx, filter)
	if err != nil {
		log.Printf("[Ops] GetErrorLogs failed: %v", err)
		return nil, err
REDACTED

	return result, nil
REDACTED

// ListUserErrorRequests 返回某个用户自己的错误请求（精简脱敏）。
// 强制：仅当前用户、View=all（含业务限流/余额类）、排除 count_tokens 噪声。
func (s *OpsService) ListUserErrorRequests(ctx context.Context, userID int64, filter *OpsErrorLogFilter) (*UserErrorRequestList, error) {
	if filter == nil {
		filter = &OpsErrorLogFilter{REDACTED
REDACTED
	f := *filter // 拷贝快照，避免原地篡改调用方的 filter（slice 字段只读，浅拷贝足够）
	filter = &f
	uid := userID
	filter.UserID = &uid
	// APIKeyID 透传：保留 handler 传入的值。安全由 buildOpsErrorLogsWhere 的
	// "user_id = 自己 AND api_key_id = X" 双重约束保证——传入他人 key 只会得到空集，无泄露。
	filter.View = "all"
	filter.ExcludeCountTokens = true
	filter.ModelFuzzy = true // 用户端模型过滤走 ILIKE 模糊；管理端不设此字段，保持精确
	// 防御：用户端不接受这些 admin-only / 特殊维度
	filter.UserQuery = ""
	filter.Owner = ""
	filter.Source = ""
	// 清空 Phase 是防御:用户端一律改走 category→ErrorPhasesAny/ErrorTypesAny
	//（纯 ANY 过滤,不影响 status>=400 子句）。守卫豁免现在还需要
	// IncludeRecoveredUpstream(用户端永不设置),recovered upstream
	//（error_phase='upstream' 但 status<400,最终成功返回）记录对用户不可见——符合预期。
	filter.Phase = ""
	filter.IncludeRecoveredUpstream = false

	list, err := s.opsRepo.ListErrorLogs(ctx, filter)
	if err != nil {
		return nil, err
REDACTED
	items := make([]*UserErrorRequest, 0, len(list.Errors))
	for _, e := range list.Errors {
		if r := ToUserErrorRequest(e); r != nil {
			items = append(items, r)
	REDACTED
REDACTED
	return &UserErrorRequestList{
		Items:    items,
		Total:    list.Total,
		Page:     list.Page,
		PageSize: list.PageSize,
REDACTED, nil
REDACTED

func (s *OpsService) GetErrorLogByID(ctx context.Context, id int64) (*OpsErrorLogDetail, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
REDACTED
	if s.opsRepo == nil {
		return nil, infraerrors.NotFound("OPS_ERROR_NOT_FOUND", "ops error log not found")
REDACTED
	detail, err := s.opsRepo.GetErrorLogByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("OPS_ERROR_NOT_FOUND", "ops error log not found")
	REDACTED
		return nil, infraerrors.InternalServer("OPS_ERROR_LOAD_FAILED", "Failed to load ops error log").WithCause(err)
REDACTED
	return detail, nil
REDACTED

// GetUserErrorRequestDetail 返回某用户自己某条错误请求的脱敏详情(含 error_body)。
// 安全:强制按用户归属校验;非本人记录一律返回 NotFound(不泄露存在性)。
func (s *OpsService) GetUserErrorRequestDetail(ctx context.Context, userID, id int64) (*UserErrorRequestDetail, error) {
	if s.opsRepo == nil {
		return nil, infraerrors.NotFound("OPS_ERROR_NOT_FOUND", "ops error log not found")
REDACTED
	if id <= 0 {
		return nil, infraerrors.BadRequest("OPS_ERROR_INVALID_ID", "invalid error id")
REDACTED
	detail, err := s.opsRepo.GetErrorLogByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("OPS_ERROR_NOT_FOUND", "ops error log not found")
	REDACTED
		return nil, infraerrors.InternalServer("OPS_ERROR_LOAD_FAILED", "Failed to load ops error log").WithCause(err)
REDACTED
	// 归属只能由通过鉴权时写入的 user_id 确定。
	ownedDirectly := detail.UserID != nil && *detail.UserID == userID
	if !ownedDirectly {
		return nil, infraerrors.NotFound("OPS_ERROR_NOT_FOUND", "ops error log not found")
REDACTED
	return ToUserErrorRequestDetail(detail), nil
REDACTED

func (s *OpsService) UpdateErrorResolution(ctx context.Context, errorID int64, resolved bool, resolvedByUserID *int64) error {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return err
REDACTED
	if s.opsRepo == nil {
		return infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
REDACTED
	if errorID <= 0 {
		return infraerrors.BadRequest("OPS_ERROR_INVALID_ID", "invalid error id")
REDACTED
	// Best-effort ensure the error exists
	if _, err := s.opsRepo.GetErrorLogByID(ctx, errorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return infraerrors.NotFound("OPS_ERROR_NOT_FOUND", "ops error log not found")
	REDACTED
		return infraerrors.InternalServer("OPS_ERROR_LOAD_FAILED", "Failed to load ops error log").WithCause(err)
REDACTED
	return s.opsRepo.UpdateErrorResolution(ctx, errorID, resolved, resolvedByUserID, nil)
REDACTED

func sanitizeAndTrimJSONPayload(raw []byte, maxBytes int) (jsonString string, truncated bool, bytesLen int) {
	bytesLen = len(raw)
	if len(raw) == 0 {
		return "", false, 0
REDACTED

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		// If it is not valid JSON, fall back to the caller's non-JSON handling.
		return "", false, bytesLen
REDACTED

	decoded = redactSensitiveJSON(decoded)

	encoded, err := json.Marshal(decoded)
	if err != nil {
		return "", false, bytesLen
REDACTED
	if len(encoded) <= maxBytes {
		return string(encoded), false, bytesLen
REDACTED

	// Trim conversation history to keep the most recent context.
	if root, ok := decoded.(map[string]any); ok {
		if trimmed, ok := trimConversationArrays(root, maxBytes); ok {
			encoded2, err2 := json.Marshal(trimmed)
			if err2 == nil && len(encoded2) <= maxBytes {
				return string(encoded2), true, bytesLen
		REDACTED
			// Fallthrough: keep shrinking.
			decoded = trimmed
	REDACTED

		essential := shrinkToEssentials(root)
		encoded3, err3 := json.Marshal(essential)
		if err3 == nil && len(encoded3) <= maxBytes {
			return string(encoded3), true, bytesLen
	REDACTED
REDACTED

	// Last resort: keep JSON shape but drop big fields.
	// This avoids downstream code that expects certain top-level keys from crashing.
	if root, ok := decoded.(map[string]any); ok {
		placeholder := shallowCopyMap(root)
		placeholder["payload_truncated"] = true

		// Replace potentially huge arrays/strings, but keep the keys present.
		for _, k := range []string{"messages", "contents", "input", "prompt"REDACTED {
			if _, exists := placeholder[k]; exists {
				placeholder[k] = []any{REDACTED
		REDACTED
	REDACTED
		for _, k := range []string{"text"REDACTED {
			if _, exists := placeholder[k]; exists {
				placeholder[k] = ""
		REDACTED
	REDACTED

		encoded4, err4 := json.Marshal(placeholder)
		if err4 == nil {
			if len(encoded4) <= maxBytes {
				return string(encoded4), true, bytesLen
		REDACTED
	REDACTED
REDACTED

	// Final fallback: minimal valid JSON.
	encoded4, err4 := json.Marshal(map[string]any{"payload_truncated": trueREDACTED)
	if err4 != nil {
		return "", true, bytesLen
REDACTED
	return string(encoded4), true, bytesLen
REDACTED

func redactSensitiveJSON(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			if isSensitiveKey(k) {
				out[k] = "[REDACTED]"
				continue
		REDACTED
			out[k] = redactSensitiveJSON(vv)
	REDACTED
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, vv := range t {
			out = append(out, redactSensitiveJSON(vv))
	REDACTED
		return out
	default:
		return v
REDACTED
REDACTED

func isSensitiveKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return false
REDACTED

	// Token 计数 / 预算字段不是凭据，应保留用于排错。
	// 白名单保持尽量窄，避免误把真实敏感信息"反脱敏"。
	switch k {
	case "max_tokens",
		"max_output_tokens",
		"max_input_tokens",
		"max_completion_tokens",
		"max_tokens_to_sample",
		"budget_tokens",
		"prompt_tokens",
		"completion_tokens",
		"input_tokens",
		"output_tokens",
		"total_tokens",
		"token_count",
		"cache_creation_input_tokens",
		"cache_read_input_tokens":
		return false
REDACTED

	// Exact matches (common credential fields).
	switch k {
	case "authorization",
		"proxy-authorization",
		"x-api-key",
		"api_key",
		"apikey",
		"access_token",
		"refresh_token",
		"id_token",
		"session_token",
		"token",
		"password",
		"passwd",
		"passphrase",
		"secret",
		"client_secret",
		"private_key",
		"jwt",
		"signature",
		"accesskeyid",
		"secretaccesskey":
		return true
REDACTED

	// Suffix matches.
	for _, suffix := range []string{
		"_secret",
		"_token",
		"_id_token",
		"_session_token",
		"_password",
		"_passwd",
		"_passphrase",
		"_key",
		"secret_key",
		"private_key",
REDACTED {
		if strings.HasSuffix(k, suffix) {
			return true
	REDACTED
REDACTED

	// Substring matches (conservative, but errs on the side of privacy).
	for _, sub := range []string{
		"secret",
		"token",
		"password",
		"passwd",
		"passphrase",
		"privatekey",
		"private_key",
		"apikey",
		"api_key",
		"accesskeyid",
		"secretaccesskey",
		"bearer",
		"cookie",
		"credential",
		"session",
		"jwt",
		"signature",
REDACTED {
		if strings.Contains(k, sub) {
			return true
	REDACTED
REDACTED

	return false
REDACTED

func trimConversationArrays(root map[string]any, maxBytes int) (map[string]any, bool) {
	// Supported: anthropic/openai: messages; gemini: contents.
	if out, ok := trimArrayField(root, "messages", maxBytes); ok {
		return out, true
REDACTED
	if out, ok := trimArrayField(root, "contents", maxBytes); ok {
		return out, true
REDACTED
	return root, false
REDACTED

func trimArrayField(root map[string]any, field string, maxBytes int) (map[string]any, bool) {
	raw, ok := root[field]
	if !ok {
		return nil, false
REDACTED
	arr, ok := raw.([]any)
	if !ok || len(arr) == 0 {
		return nil, false
REDACTED

	// Keep at least the last message/content. Use binary search so we don't marshal O(n) times.
	// We are dropping from the *front* of the array (oldest context first).
	lo := 0
	hi := len(arr) - 1 // inclusive; hi ensures at least one item remains

	var best map[string]any
	found := false

	for lo <= hi {
		mid := (lo + hi) / 2
		candidateArr := arr[mid:]
		if len(candidateArr) == 0 {
			lo = mid + 1
			continue
	REDACTED

		next := shallowCopyMap(root)
		next[field] = candidateArr
		encoded, err := json.Marshal(next)
		if err != nil {
			// If marshal fails, try dropping more.
			lo = mid + 1
			continue
	REDACTED

		if len(encoded) <= maxBytes {
			best = next
			found = true
			// Try to keep more context by dropping fewer items.
			hi = mid - 1
			continue
	REDACTED

		// Need to drop more.
		lo = mid + 1
REDACTED

	if found {
		return best, true
REDACTED

	// Nothing fit (even with only one element); return the smallest slice and let the
	// caller fall back to shrinkToEssentials().
	next := shallowCopyMap(root)
	next[field] = arr[len(arr)-1:]
	return next, true
REDACTED

func shrinkToEssentials(root map[string]any) map[string]any {
	out := make(map[string]any)
	for _, key := range []string{
		"model",
		"stream",
		"max_tokens",
		"max_output_tokens",
		"max_input_tokens",
		"max_completion_tokens",
		"thinking",
		"temperature",
		"top_p",
		"top_k",
REDACTED {
		if v, ok := root[key]; ok {
			out[key] = v
	REDACTED
REDACTED

	// Keep only the last element of the conversation array.
	if v, ok := root["messages"]; ok {
		if arr, ok := v.([]any); ok && len(arr) > 0 {
			out["messages"] = []any{arr[len(arr)-1]REDACTED
	REDACTED
REDACTED
	if v, ok := root["contents"]; ok {
		if arr, ok := v.([]any); ok && len(arr) > 0 {
			out["contents"] = []any{arr[len(arr)-1]REDACTED
	REDACTED
REDACTED
	return out
REDACTED

func shallowCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
REDACTED
	return out
REDACTED

func sanitizeErrorBodyForStorage(raw string, maxBytes int) (sanitized string, truncated bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
REDACTED

	// Prefer JSON-safe sanitization when possible.
	if out, trunc, _ := sanitizeAndTrimJSONPayload([]byte(raw), maxBytes); out != "" {
		return out, trunc
REDACTED

	// Non-JSON: best-effort truncate.
	if maxBytes > 0 && len(raw) > maxBytes {
		return truncateString(raw, maxBytes), true
REDACTED
	return raw, false
REDACTED
