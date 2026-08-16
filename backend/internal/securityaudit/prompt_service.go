package securityaudit

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type PromptService struct {
	config    ConfigStore
	repo      *PostgreSQLRepository
	payload   *RedisPayloadStore
	enqueuer  *Enqueuer
	runner    *Runner
	evaluator *GuardEvaluator
	scanner   *OpenAICompatibleScanner
	metrics   *AtomicMetrics
	hashCache PromptAuditHashCache
	clock     Clock

	lifecycleMu  sync.Mutex
	cancel       context.CancelFunc
	background   context.Context
	enqueueWG    sync.WaitGroup
	enqueueSlots chan struct{}
	probeMu      sync.RWMutex
	probes       map[string]ProbeResult
}

func NewPromptService(
	config ConfigStore,
	repo *PostgreSQLRepository,
	payload *RedisPayloadStore,
	scanner *OpenAICompatibleScanner,
	metrics *AtomicMetrics,
	hashCache PromptAuditHashCache,
) *PromptService {
	resultCache := NewPromptResultCache()
	enqueuer := NewEnqueuer(config, repo, payload, metrics)
	evaluator := NewGuardEvaluator(scanner, repo, metrics, resultCache)
	runner := NewRunnerWithHashCache(config, repo, payload, scanner, metrics, resultCache, hashCache)
	return &PromptService{
		config: config, repo: repo, payload: payload, scanner: scanner, metrics: metrics, hashCache: hashCache,
		enqueuer: enqueuer, evaluator: evaluator, runner: runner, clock: realClock{},
		enqueueSlots: make(chan struct{}, 128), probes: map[string]ProbeResult{},
	}
}

func (s *PromptService) Start(ctx context.Context) error {
	if s == nil || s.config == nil || s.runner == nil {
		return errors.New("prompt audit service unavailable")
	}
	s.lifecycleMu.Lock()
	if s.cancel != nil {
		s.lifecycleMu.Unlock()
		return nil
	}
	background, cancel := context.WithCancel(ctx)
	s.background, s.cancel = background, cancel
	s.lifecycleMu.Unlock()
	configErr := s.config.Start(background)
	workerErr := s.runner.Start(background)
	return errors.Join(configErr, workerErr)
}

func (s *PromptService) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.lifecycleMu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
	}
	var workerErr error
	if s.runner != nil {
		workerErr = s.runner.Shutdown(ctx)
	}
	done := make(chan struct{})
	go func() { s.enqueueWG.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		if workerErr == nil {
			workerErr = ctx.Err()
		}
	}
	var configErr error
	if s.config != nil {
		configErr = s.config.Shutdown(ctx)
	}
	if workerErr != nil {
		return workerErr
	}
	return configErr
}

func (s *PromptService) EffectiveMode() Mode {
	if s == nil || s.config == nil {
		return ModeOff
	}
	return s.config.EffectiveMode()
}

func (s *PromptService) Enqueue(_ context.Context, req Request) error {
	return s.enqueueAsyncAudit(req, false)
}

func (s *PromptService) enqueueAsyncAudit(req Request, allowMixedMode bool) error {
	if s == nil || s.enqueuer == nil {
		return nil
	}
	mode := s.EffectiveMode()
	if mode != ModeAsync && (!allowMixedMode || mode != ModeBlocking) {
		return nil
	}
	select {
	case s.enqueueSlots <- struct{}{}:
	default:
		if s.metrics != nil {
			s.metrics.IncDropped()
		}
		LogWarn(EventEnqueueDropped, map[string]any{"request_id": req.RequestID, "status": "dropped", "error_code": "local_enqueue_busy"})
		return nil
	}
	s.lifecycleMu.Lock()
	background := s.background
	s.lifecycleMu.Unlock()
	if background == nil {
		<-s.enqueueSlots
		return errors.New("prompt audit service not started")
	}
	requestCopy := req.Clone()
	s.enqueueWG.Add(1)
	go func() {
		defer s.enqueueWG.Done()
		defer func() { <-s.enqueueSlots }()
		ctx, cancel := context.WithTimeout(background, 2*time.Second)
		defer cancel()
		_ = s.enqueuer.Enqueue(ctx, requestCopy)
	}()
	return nil
}

// PreCheck is the cheap admission path for async audit. A digest is only
// considered a block after an earlier async audit has persisted it; Redis
// failures deliberately fail open here so the existing async queue behavior is
// preserved.
func (s *PromptService) PreCheck(ctx context.Context, req Request) (*PromptDecision, error) {
	allow := &PromptDecision{Kind: DecisionAllow, AllowNextStage: true}
	if s == nil || s.config == nil || s.hashCache == nil {
		return allow, nil
	}
	cfg, ok := s.config.Active()
	if !ok || !cfg.RiskControlEnabled || !cfg.Enabled || !cfg.PreHashCheckEnabled || !cfg.IncludesGroup(req.GroupID) {
		return allow, nil
	}
	// Hashes are produced by the asynchronous full-context audit, so keep this
	// lookup on the same full transcript even when synchronous AI blocking is
	// narrowed to the latest turn.
	snapshot, err := ExtractBlockingPromptSnapshot(req, false)
	if err != nil {
		return allow, nil
	}
	matched, err := s.hashCache.HasFlaggedPromptHash(ctx, snapshot.PromptHash)
	if err != nil {
		LogWarn(EventGuardFailed, mergeLogFields(snapshotLogFields(snapshot), map[string]any{
			"status": "hash_check_failed", "error_code": ErrorCodePromptAuditHashCacheUnavailable,
		}))
		return allow, nil
	}
	if !matched {
		return allow, nil
	}
	result := newPromptHashBlockResult(snapshot.PromptHash)
	s.recordBlockingResult(ctx, snapshot, cfg, result)
	if s.metrics != nil {
		s.metrics.Observe(DecisionBlock, 0)
	}
	LogWarn(EventGuardBlocked, mergeLogFields(snapshotLogFields(snapshot), map[string]any{
		"decision": DecisionBlock, "action": result.Action, "prompt_hash": snapshot.PromptHash,
		"status": "blocked", "error_code": ErrorCodeBlocked, "stage": snapshot.Stage,
	}))
	return &PromptDecision{Kind: DecisionBlock, ErrorCode: ErrorCodeBlocked, Result: result, AllowNextStage: false}, nil
}

func (s *PromptService) Evaluate(ctx context.Context, req Request) (*PromptDecision, error) {
	if s == nil || s.config == nil {
		return nil, &GuardError{Code: ErrorCodeUnavailable}
	}
	if s.config.BlockingActivationDegraded() {
		return nil, &GuardError{Code: ErrorCodeUnavailable}
	}
	cfg, ok := s.config.Active()
	if !ok {
		if s.config.EffectiveMode() == ModeBlocking {
			return nil, &GuardError{Code: ErrorCodeUnavailable}
		}
		return &PromptDecision{Kind: DecisionAllow, AllowNextStage: true}, nil
	}
	if cfg.EffectiveMode() != ModeBlocking || !cfg.IncludesGroup(req.GroupID) {
		return &PromptDecision{Kind: DecisionAllow, AllowNextStage: true}, nil
	}
	snapshot, err := ExtractBlockingPromptSnapshot(req, cfg.BlockingLatestTurnOnly)
	if errors.Is(err, ErrNoPromptText) {
		return &PromptDecision{Kind: DecisionAllow, AllowNextStage: true}, nil
	}
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeInvalidResponse, Cause: err}
	}
	if cfg.keywordModeUsesKeywords() && cfg.effectiveKeywordBlockingEnabled() {
		if result, handled := cfg.keywordResult(snapshot.ScanText, 0); handled {
			kind := DecisionAllow
			allowNextStage := true
			if result.Action == ActionBlock {
				kind = DecisionBlock
				allowNextStage = false
			}
			if result.Decision != EventPass || cfg.StorePassEvents {
				s.recordBlockingResult(ctx, snapshot, cfg, result)
			}
			if s.metrics != nil {
				s.metrics.Observe(kind, 0)
			}
			decision := &PromptDecision{Kind: kind, Result: result, AllowNextStage: allowNextStage}
			if kind == DecisionBlock {
				decision.ErrorCode = ErrorCodeBlocked
				LogWarn(EventGuardBlocked, mergeLogFields(snapshotLogFields(snapshot), map[string]any{
					"decision": kind, "action": result.Action, "matched_keyword": result.MatchedKeyword,
					"status": "blocked", "error_code": ErrorCodeBlocked, "stage": snapshot.Stage,
				}))
			}
			return decision, nil
		}
	}
	if !cfg.keywordModeUsesAI() || !cfg.effectiveAIBlockingEnabled() {
		// A mixed policy (keyword sync + AI async) reaches this path after a
		// keyword miss. Enqueueing is best effort and never delays admission.
		if cfg.shouldRunAsyncAudit() {
			_ = s.enqueueAsyncAudit(req, true)
		}
		return &PromptDecision{Kind: DecisionAllow, AllowNextStage: true}, nil
	}
	if s.evaluator == nil {
		return nil, &GuardError{Code: ErrorCodeUnavailable}
	}
	return s.evaluator.Evaluate(ctx, cfg, snapshot)
}

func (s *PromptService) recordBlockingResult(ctx context.Context, snapshot PromptSnapshot, cfg ActiveConfig, result *NormalizedResult) {
	if s == nil || s.repo == nil || result == nil {
		return
	}
	if _, err := s.repo.RecordBlocking(ctx, snapshot.Redacted(), cfg.ConfigVersion, result, cfg.StorePassEvents); err != nil {
		if s.metrics != nil {
			s.metrics.IncRecordFailed()
		}
		LogWarn(EventResultRecordFailed, mergeLogFields(snapshotLogFields(snapshot), map[string]any{
			"decision": result.Decision, "error_code": "result_record_failed", "stage": snapshot.Stage,
			"status": "failed",
		}))
	}
}

func (s *PromptService) GetConfig() (PublicConfig, error) { return s.config.Public() }

func (s *PromptService) SaveConfig(ctx context.Context, req UpdateConfigRequest, actorID int64) (PublicConfig, error) {
	if req.ProxyID != nil && *req.ProxyID > 0 {
		if s.scanner == nil {
			return PublicConfig{}, infraerrors.ServiceUnavailable("prompt_audit_proxy_unavailable", "提示词审计代理服务暂不可用")
		}
		if err := s.scanner.validateProxy(ctx, req.ProxyID); err != nil {
			return PublicConfig{}, infraerrors.BadRequest("prompt_audit_invalid_proxy", "提示词审计代理服务器不存在、未启用或已过期").WithCause(err)
		}
	}
	return s.config.Save(ctx, req, actorID)
}

func normalizePromptAuditHash(promptHash string) (string, error) {
	promptHash = strings.ToLower(strings.TrimSpace(promptHash))
	if len(promptHash) != 64 {
		return "", infraerrors.BadRequest(ErrorCodePromptAuditHashInvalid, "提示词哈希必须是 64 位 SHA-256")
	}
	if _, err := hex.DecodeString(promptHash); err != nil {
		return "", infraerrors.BadRequest(ErrorCodePromptAuditHashInvalid, "提示词哈希必须是有效的十六进制 SHA-256")
	}
	return promptHash, nil
}

func (s *PromptService) DeleteFlaggedPromptHash(ctx context.Context, promptHash string) (*PromptAuditDeleteHashResult, error) {
	promptHash, err := normalizePromptAuditHash(promptHash)
	if err != nil {
		return nil, err
	}
	if s == nil || s.hashCache == nil {
		return nil, infraerrors.ServiceUnavailable(ErrorCodePromptAuditHashCacheUnavailable, "提示词审计哈希缓存暂不可用")
	}
	deleted, err := s.hashCache.DeleteFlaggedPromptHash(ctx, promptHash)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable(ErrorCodePromptAuditHashCacheUnavailable, "提示词审计哈希缓存暂不可用").WithCause(err)
	}
	return &PromptAuditDeleteHashResult{PromptHash: promptHash, Deleted: deleted}, nil
}

func (s *PromptService) ClearFlaggedPromptHashes(ctx context.Context) (*PromptAuditClearHashesResult, error) {
	if s == nil || s.hashCache == nil {
		return nil, infraerrors.ServiceUnavailable(ErrorCodePromptAuditHashCacheUnavailable, "提示词审计哈希缓存暂不可用")
	}
	deleted, err := s.hashCache.ClearFlaggedPromptHashes(ctx)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable(ErrorCodePromptAuditHashCacheUnavailable, "提示词审计哈希缓存暂不可用").WithCause(err)
	}
	return &PromptAuditClearHashesResult{Deleted: deleted}, nil
}

func (s *PromptService) Runtime(ctx context.Context) RuntimeSnapshot {
	expected, activeVersion, loadedAt, loadError := s.config.RuntimeState()
	cfg, hasConfig := s.config.Active()
	mode := s.EffectiveMode()
	workerTotal, queueCapacity := 0, 0
	if hasConfig {
		workerTotal, queueCapacity = cfg.WorkerCount, cfg.QueueCapacity
	}
	runtime := RuntimeSnapshot{
		ProcessStatus: "disabled", EffectiveMode: mode, ExpectedConfigVersion: expected,
		ActiveConfigVersion: activeVersion, ConfigLoadedAt: loadedAt, ConfigLoadError: loadError,
		WorkerTotal: workerTotal, QueueCapacity: queueCapacity, DatabaseStatus: "ok", RedisStatus: "ok",
		Endpoints: s.probeSnapshot(), GuardMetrics: s.metrics.Snapshot(),
	}
	if s.repo != nil {
		stats, err := s.repo.QueueStats(ctx)
		if err != nil {
			runtime.DatabaseStatus = "error"
			runtime.LastErrorCode = "database_unavailable"
		} else {
			runtime.Queue = stats
		}
	} else {
		runtime.DatabaseStatus = "error"
	}
	if s.payload == nil || s.payload.Ping(ctx) != nil {
		runtime.RedisStatus = "error"
		if runtime.LastErrorCode == "" {
			runtime.LastErrorCode = "payload_store_unavailable"
		}
	}
	if s.hashCache != nil {
		count, err := s.hashCache.CountFlaggedPromptHashes(ctx)
		if err != nil {
			runtime.RedisStatus = "error"
			if runtime.LastErrorCode == "" {
				runtime.LastErrorCode = ErrorCodePromptAuditHashCacheUnavailable
			}
		} else {
			runtime.FlaggedHashCount = count
		}
	}
	activeWorkers, processed, failed, heartbeat, lastProcessed, workerCode, workerMessage := s.runner.Snapshot()
	runtime.WorkerActive, runtime.ProcessedTotal, runtime.FailedTotal = activeWorkers, processed, failed
	if s.metrics != nil {
		auditMetrics := s.metrics.AuditSnapshot()
		runtime.EnqueuedTotal, runtime.DroppedTotal = auditMetrics.Enqueued, auditMetrics.Dropped
	}
	runtime.WorkerHeartbeatAt, runtime.LastProcessedAt = heartbeat, lastProcessed
	if workerCode != "" {
		runtime.LastErrorCode, runtime.LastErrorMessage = workerCode, workerMessage
	}
	if mode != ModeOff {
		runtime.ProcessStatus = "running"
		if loadError != "" || runtime.DatabaseStatus != "ok" || runtime.RedisStatus != "ok" || activeVersion != expected {
			runtime.ProcessStatus = "degraded"
		}
		if heartbeat == nil || s.clock.Now().Sub(*heartbeat) > 10*time.Second {
			runtime.ProcessStatus = "degraded"
		}
	}
	return runtime
}

type ProbeRequest struct {
	Endpoint UpdateEndpoint `json:"endpoint"`
	// ProxyID nil 表示沿用已保存配置，<=0 表示强制直连，>0 表示使用指定代理。
	ProxyID *int64 `json:"proxy_id"`
}

func (s *PromptService) Probe(ctx context.Context, request ProbeRequest) ProbeResult {
	started := s.clock.Now()
	endpoint, tokenApplied, err := s.resolveProbeEndpoint(request.Endpoint)
	if err != nil {
		return s.finishProbe(request.Endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: "endpoint_invalid", Message: "审计节点配置无效"})
	}
	LogInfo(EventProbeStarted, map[string]any{"guard_endpoint_id": endpoint.ID, "status": "started"})
	endpoint.ProxyID = s.resolveProbeProxyID(request.ProxyID)
	client, err := s.scanner.clientFor(ctx, endpoint)
	if err != nil {
		return s.finishProbe(endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: "endpoint_unsafe", Message: "审计节点地址不在允许范围", TokenApplied: tokenApplied})
	}
	if endpoint.Protocol == ProtocolOpenAIModeration || endpoint.Protocol == ProtocolNemotronSafety {
		result, scanErr := s.scanner.Scan(ctx, endpoint, "Hello", AllScannerIDs)
		if scanErr == nil && result != nil {
			message := "审计节点 Moderation API 调用正常"
			if endpoint.Protocol == ProtocolNemotronSafety {
				message = "审计节点 Nemotron 模型调用正常"
			}
			return s.finishProbe(endpoint.ID, started, ProbeResult{OK: true, Status: "healthy", Message: message, HTTPStatus: http.StatusOK, TokenApplied: tokenApplied})
		}
		code, status, retryable := guardErrorCode(scanErr), 0, false
		var guardErr *GuardError
		if errors.As(scanErr, &guardErr) {
			status, retryable = guardErr.HTTPStatus, guardErr.Retryable
		}
		if code == "" {
			code = ErrorCodeInvalidResponse
		}
		message := "审计节点 Moderation API 调用失败"
		if endpoint.Protocol == ProtocolNemotronSafety {
			message = "审计节点 Nemotron 模型调用失败"
		}
		return s.finishProbe(endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: code, Message: message, HTTPStatus: status, Retryable: retryable, TokenApplied: tokenApplied})
	}
	modelsURL, _ := ModelsURL(endpoint.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return s.finishProbe(endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: "probe_request_invalid", Message: "无法创建探测请求", TokenApplied: tokenApplied})
	}
	if endpoint.Token != "" {
		req.Header.Set("Authorization", "Bearer "+endpoint.Token)
	}
	resp, err := client.Do(req)
	if err != nil {
		code := "connection_failed"
		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			code = "timeout"
		}
		return s.finishProbe(endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: code, Message: "无法连接审计节点", Retryable: true, TokenApplied: tokenApplied})
	}
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxGuardResponseBytes+1))
	_ = resp.Body.Close()
	if readErr != nil {
		return s.finishProbe(endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: "response_read_failed", Message: "审计节点响应读取失败", HTTPStatus: resp.StatusCode, Retryable: true, TokenApplied: tokenApplied})
	}
	if int64(len(responseBody)) > maxGuardResponseBytes {
		return s.finishProbe(endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: "response_too_large", Message: "审计节点响应无效", HTTPStatus: resp.StatusCode, TokenApplied: tokenApplied})
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && modelsResponseReady(responseBody, endpoint.Model) {
		return s.finishProbe(endpoint.ID, started, ProbeResult{OK: true, Status: "healthy", Message: "审计节点连接正常", HTTPStatus: resp.StatusCode, TokenApplied: tokenApplied})
	}
	if (resp.StatusCode >= 200 && resp.StatusCode < 300) || resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		result, scanErr := s.scanner.Scan(ctx, endpoint, "Hello", AllScannerIDs)
		if scanErr == nil && result != nil {
			return s.finishProbe(endpoint.ID, started, ProbeResult{OK: true, Status: "healthy", Message: "审计节点模型调用正常", HTTPStatus: http.StatusOK, TokenApplied: tokenApplied})
		}
		code, status, retryable := guardErrorCode(scanErr), 0, false
		var guardErr *GuardError
		if errors.As(scanErr, &guardErr) {
			status, retryable = guardErr.HTTPStatus, guardErr.Retryable
		}
		if code == "" {
			code = ErrorCodeInvalidResponse
		}
		return s.finishProbe(endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: code, Message: "审计节点模型调用失败", HTTPStatus: status, Retryable: retryable, TokenApplied: tokenApplied})
	}
	code, retryable := "probe_http_error", resp.StatusCode == 429 || resp.StatusCode >= 500
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		code = "authentication_failed"
	}
	return s.finishProbe(endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: code, Message: "审计节点探测失败", HTTPStatus: resp.StatusCode, Retryable: retryable, TokenApplied: tokenApplied})
}

func modelsResponseReady(body []byte, model string) bool {
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &response) != nil || response.Data == nil {
		return false
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return true
	}
	for _, item := range response.Data {
		if strings.TrimSpace(item.ID) == model {
			return true
		}
	}
	return false
}

func (s *PromptService) resolveProbeProxyID(requested *int64) *int64 {
	if requested != nil {
		if *requested <= 0 {
			return nil
		}
		return cloneInt64Ptr(requested)
	}
	if s != nil && s.config != nil {
		if cfg, ok := s.config.Active(); ok {
			return cloneInt64Ptr(cfg.ProxyID)
		}
	}
	return nil
}

func (s *PromptService) resolveProbeEndpoint(input UpdateEndpoint) (ActiveEndpoint, bool, error) {
	baseURL, err := NormalizeBaseURL(input.BaseURL)
	if err != nil {
		return ActiveEndpoint{}, false, err
	}
	token := strings.TrimSpace(input.Token)
	if token == "" {
		if cfg, ok := s.config.Active(); ok {
			for _, endpoint := range cfg.Endpoints {
				if endpoint.ID != strings.TrimSpace(input.ID) {
					continue
				}
				// Reuse a stored credential only when the probe targets the same
				// normalized base URL. Otherwise an admin probe could exfiltrate
				// the Guard token to an attacker-controlled HTTPS host.
				if endpoint.BaseURL == baseURL {
					token = endpoint.Token
				}
				break
			}
		}
	}
	protocol := strings.TrimSpace(input.Protocol)
	if protocol == "" {
		protocol = ProtocolOpenAICompatible
	}
	model := strings.TrimSpace(input.Model)
	model = normalizeModelForEndpoint(protocol, baseURL, model)
	timeout := input.TimeoutMS
	if timeout == 0 {
		timeout = DefaultTimeoutMS
	}
	limit := input.InputLimit
	if limit == 0 {
		limit = DefaultInputLimit
	}
	storage := storageConfig{Enabled: false, Strategy: "priority", WorkerCount: DefaultWorkerCount, QueueCapacity: DefaultQueueCapacity, Scanners: append([]string(nil), AllScannerIDs...), AllGroups: true,
		Endpoints: []StorageEndpoint{{ID: strings.TrimSpace(input.ID), Name: strings.TrimSpace(input.Name), Protocol: protocol, BaseURL: baseURL, Model: model, TimeoutMS: timeout, InputLimit: limit}}}
	if storage.Endpoints[0].ID == "" {
		storage.Endpoints[0].ID = "probe"
	}
	if storage.Endpoints[0].Name == "" {
		storage.Endpoints[0].Name = "Probe"
	}
	if err := validateStorageConfig(storage); err != nil {
		return ActiveEndpoint{}, false, err
	}
	return ActiveEndpoint{ID: storage.Endpoints[0].ID, Name: storage.Endpoints[0].Name, Protocol: protocol, BaseURL: baseURL, Model: model, Token: token, TimeoutMS: timeout, InputLimit: limit, Enabled: true}, token != "", nil
}

func (s *PromptService) finishProbe(id string, started time.Time, result ProbeResult) ProbeResult {
	result.CheckedAt = s.clock.Now()
	result.LatencyMS = int(result.CheckedAt.Sub(started).Milliseconds())
	if result.OK {
		LogInfo(EventProbeFinished, map[string]any{"guard_endpoint_id": id, "status": result.Status, "latency_ms": result.LatencyMS, "http_status": result.HTTPStatus})
	} else {
		LogWarn(EventProbeFailed, map[string]any{"guard_endpoint_id": id, "status": result.Status, "latency_ms": result.LatencyMS, "http_status": result.HTTPStatus, "error_code": result.ErrorCode, "retryable": result.Retryable})
	}
	s.probeMu.Lock()
	s.probes[id] = result
	s.probeMu.Unlock()
	return result
}

func (s *PromptService) probeSnapshot() map[string]ProbeResult {
	s.probeMu.RLock()
	defer s.probeMu.RUnlock()
	result := make(map[string]ProbeResult, len(s.probes))
	for id, probe := range s.probes {
		result[id] = probe
	}
	return result
}

func (s *PromptService) ListEvents(ctx context.Context, filter EventFilter, page, pageSize int) (*EventPage, error) {
	return s.repo.ListEvents(ctx, filter, page, pageSize)
}
func (s *PromptService) GetEvent(ctx context.Context, id int64) (*Event, error) {
	return s.repo.GetEvent(ctx, id)
}

func (s *PromptService) DeleteEvent(ctx context.Context, id int64) (*DeleteResult, error) {
	result, err := s.repo.DeleteEvent(ctx, id)
	if err == nil {
		s.deletePayloads(ctx, result.JobIDs)
	}
	return result, err
}
func (s *PromptService) DeleteEventsByIDs(ctx context.Context, ids []int64) (*DeleteResult, error) {
	result, err := s.repo.DeleteEventsByIDs(ctx, ids)
	if err == nil {
		s.deletePayloads(ctx, result.JobIDs)
	}
	return result, err
}

type deleteClaims struct {
	FilterHash    string    `json:"filter_hash"`
	SnapshotMaxID int64     `json:"snapshot_max_id"`
	AdminID       int64     `json:"admin_id"`
	IssuedAt      time.Time `json:"issued_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func (s *PromptService) PreviewDelete(ctx context.Context, filter EventFilter, adminID int64) (*DeletePreview, error) {
	preview, err := s.repo.PreviewDelete(ctx, filter)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	expires := now.Add(5 * time.Minute)
	claimsRaw, _ := json.Marshal(deleteClaims{FilterHash: preview.FilterHash, SnapshotMaxID: preview.SnapshotMaxID, AdminID: adminID, IssuedAt: now, ExpiresAt: expires})
	token, err := s.config.Encrypt(string(claimsRaw))
	if err != nil {
		return nil, err
	}
	preview.ConfirmationToken, preview.ExpiresAt = token, expires
	LogInfo(EventDeletePreviewed, map[string]any{"user_id": adminID, "status": "previewed"})
	return preview, nil
}

type DeleteByFilterRequest struct {
	Filter            EventFilter `json:"filter"`
	SnapshotMaxID     int64       `json:"snapshot_max_id"`
	CursorID          int64       `json:"cursor_id,omitempty"`
	FilterHash        string      `json:"filter_hash"`
	ConfirmationToken string      `json:"confirmation_token"`
	Confirm           bool        `json:"confirm"`
}

func (s *PromptService) DeleteByFilter(ctx context.Context, request DeleteByFilterRequest, adminID int64) (*DeleteResult, error) {
	if !request.Confirm {
		return nil, errors.New("prompt audit filter delete requires confirm=true")
	}
	plain, err := s.config.Decrypt(strings.TrimSpace(request.ConfirmationToken))
	if err != nil {
		return nil, errors.New("prompt audit confirmation token invalid")
	}
	var claims deleteClaims
	if json.Unmarshal([]byte(plain), &claims) != nil {
		return nil, errors.New("prompt audit confirmation token invalid")
	}
	computed := FilterHash(request.Filter, request.SnapshotMaxID)
	if claims.AdminID != adminID || claims.SnapshotMaxID != request.SnapshotMaxID || claims.FilterHash != request.FilterHash || request.FilterHash != computed || !s.clock.Now().Before(claims.ExpiresAt) {
		return nil, errors.New("prompt audit confirmation token does not match deletion request")
	}
	if request.CursorID < 0 || request.CursorID > request.SnapshotMaxID {
		return nil, errors.New("prompt audit delete cursor is outside the preview snapshot")
	}
	// Keep each HTTP request bounded. The frontend continues with the returned
	// cursor, so a large deletion cannot occupy one request until completion.
	result, err := s.repo.DeleteEventsByFilterBatch(ctx, request.Filter, request.SnapshotMaxID, request.CursorID, 1000)
	if err == nil {
		s.deletePayloads(ctx, result.JobIDs)
		LogWarn(EventEventsFilterDeleted, map[string]any{"user_id": adminID, "status": "deleted"})
	}
	return result, err
}

func (s *PromptService) deletePayloads(ctx context.Context, jobIDs []int64) {
	for _, id := range jobIDs {
		_ = s.payload.Delete(ctx, id)
	}
}

func parseTimeQuery(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}
