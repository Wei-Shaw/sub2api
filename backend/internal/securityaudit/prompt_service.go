package securityaudit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
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
	clock     Clock

	lifecycleMu  sync.Mutex
	cancel       context.CancelFunc
	background   context.Context
	enqueueWG    sync.WaitGroup
	enqueueSlots chan struct{REDACTED
	probeMu      sync.RWMutex
	probes       map[string]ProbeResult
REDACTED

func NewPromptService(
	config ConfigStore,
	repo *PostgreSQLRepository,
	payload *RedisPayloadStore,
	scanner *OpenAICompatibleScanner,
	metrics *AtomicMetrics,
) *PromptService {
	enqueuer := NewEnqueuer(config, repo, payload, metrics)
	evaluator := NewGuardEvaluator(scanner, repo, metrics)
	runner := NewRunner(config, repo, payload, scanner, metrics)
	return &PromptService{
		config: config, repo: repo, payload: payload, scanner: scanner, metrics: metrics,
		enqueuer: enqueuer, evaluator: evaluator, runner: runner, clock: realClock{REDACTED,
		enqueueSlots: make(chan struct{REDACTED, 128), probes: map[string]ProbeResult{REDACTED,
REDACTED
REDACTED

func (s *PromptService) Start(ctx context.Context) error {
	if s == nil || s.config == nil || s.runner == nil {
		return errors.New("prompt audit service unavailable")
REDACTED
	s.lifecycleMu.Lock()
	if s.cancel != nil {
		s.lifecycleMu.Unlock()
		return nil
REDACTED
	background, cancel := context.WithCancel(ctx)
	s.background, s.cancel = background, cancel
	s.lifecycleMu.Unlock()
	configErr := s.config.Start(background)
	workerErr := s.runner.Start(background)
	return errors.Join(configErr, workerErr)
REDACTED

func (s *PromptService) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
REDACTED
	s.lifecycleMu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
REDACTED
	var workerErr error
	if s.runner != nil {
		workerErr = s.runner.Shutdown(ctx)
REDACTED
	done := make(chan struct{REDACTED)
	go func() { s.enqueueWG.Wait(); close(done) REDACTED()
	select {
	case <-done:
	case <-ctx.Done():
		if workerErr == nil {
			workerErr = ctx.Err()
	REDACTED
REDACTED
	var configErr error
	if s.config != nil {
		configErr = s.config.Shutdown(ctx)
REDACTED
	if workerErr != nil {
		return workerErr
REDACTED
	return configErr
REDACTED

func (s *PromptService) EffectiveMode() Mode {
	if s == nil || s.config == nil {
		return ModeOff
REDACTED
	return s.config.EffectiveMode()
REDACTED

func (s *PromptService) Enqueue(_ context.Context, req Request) error {
	if s == nil || s.enqueuer == nil || s.EffectiveMode() != ModeAsync {
		return nil
REDACTED
	select {
	case s.enqueueSlots <- struct{REDACTED{REDACTED:
	default:
		if s.metrics != nil {
			s.metrics.IncDropped()
	REDACTED
		LogWarn(EventEnqueueDropped, map[string]any{"request_id": req.RequestID, "status": "dropped", "error_code": "local_enqueue_busy"REDACTED)
		return nil
REDACTED
	s.lifecycleMu.Lock()
	background := s.background
	s.lifecycleMu.Unlock()
	if background == nil {
		<-s.enqueueSlots
		return errors.New("prompt audit service not started")
REDACTED
	requestCopy := req.Clone()
	s.enqueueWG.Add(1)
	go func() {
		defer s.enqueueWG.Done()
		defer func() { <-s.enqueueSlots REDACTED()
		ctx, cancel := context.WithTimeout(background, 2*time.Second)
		defer cancel()
		_ = s.enqueuer.Enqueue(ctx, requestCopy)
REDACTED()
	return nil
REDACTED

func (s *PromptService) Evaluate(ctx context.Context, req Request) (*PromptDecision, error) {
	if s == nil || s.config == nil || s.evaluator == nil {
		return nil, &GuardError{Code: ErrorCodeUnavailableREDACTED
REDACTED
	if s.config.BlockingActivationDegraded() {
		return nil, &GuardError{Code: ErrorCodeUnavailableREDACTED
REDACTED
	cfg, ok := s.config.Active()
	if !ok {
		if s.config.EffectiveMode() == ModeBlocking {
			return nil, &GuardError{Code: ErrorCodeUnavailableREDACTED
	REDACTED
		return &PromptDecision{Kind: DecisionAllow, AllowNextStage: trueREDACTED, nil
REDACTED
	if cfg.EffectiveMode() != ModeBlocking || !cfg.IncludesGroup(req.GroupID) {
		return &PromptDecision{Kind: DecisionAllow, AllowNextStage: trueREDACTED, nil
REDACTED
	snapshot, err := ExtractPromptSnapshot(req)
	if errors.Is(err, ErrNoPromptText) {
		return &PromptDecision{Kind: DecisionAllow, AllowNextStage: trueREDACTED, nil
REDACTED
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeInvalidResponse, Cause: errREDACTED
REDACTED
	return s.evaluator.Evaluate(ctx, cfg, snapshot)
REDACTED

func (s *PromptService) GetConfig() (PublicConfig, error) { return s.config.Public() REDACTED

func (s *PromptService) SaveConfig(ctx context.Context, req UpdateConfigRequest, actorID int64) (PublicConfig, error) {
	return s.config.Save(ctx, req, actorID)
REDACTED

func (s *PromptService) Runtime(ctx context.Context) RuntimeSnapshot {
	expected, activeVersion, loadedAt, loadError := s.config.RuntimeState()
	cfg, hasConfig := s.config.Active()
	mode := s.EffectiveMode()
	workerTotal, queueCapacity := 0, 0
	if hasConfig {
		workerTotal, queueCapacity = cfg.WorkerCount, cfg.QueueCapacity
REDACTED
	runtime := RuntimeSnapshot{
		ProcessStatus: "disabled", EffectiveMode: mode, ExpectedConfigVersion: expected,
		ActiveConfigVersion: activeVersion, ConfigLoadedAt: loadedAt, ConfigLoadError: loadError,
		WorkerTotal: workerTotal, QueueCapacity: queueCapacity, DatabaseStatus: "ok", RedisStatus: "ok",
		Endpoints: s.probeSnapshot(), GuardMetrics: s.metrics.Snapshot(),
REDACTED
	if s.repo != nil {
		stats, err := s.repo.QueueStats(ctx)
		if err != nil {
			runtime.DatabaseStatus = "error"
			runtime.LastErrorCode = "database_unavailable"
	REDACTED else {
			runtime.Queue = stats
	REDACTED
REDACTED else {
		runtime.DatabaseStatus = "error"
REDACTED
	if s.payload == nil || s.payload.Ping(ctx) != nil {
		runtime.RedisStatus = "error"
		if runtime.LastErrorCode == "" {
			runtime.LastErrorCode = "payload_store_unavailable"
	REDACTED
REDACTED
	activeWorkers, processed, failed, heartbeat, lastProcessed, workerCode, workerMessage := s.runner.Snapshot()
	runtime.WorkerActive, runtime.ProcessedTotal, runtime.FailedTotal = activeWorkers, processed, failed
	if s.metrics != nil {
		auditMetrics := s.metrics.AuditSnapshot()
		runtime.EnqueuedTotal, runtime.DroppedTotal = auditMetrics.Enqueued, auditMetrics.Dropped
REDACTED
	runtime.WorkerHeartbeatAt, runtime.LastProcessedAt = heartbeat, lastProcessed
	if workerCode != "" {
		runtime.LastErrorCode, runtime.LastErrorMessage = workerCode, workerMessage
REDACTED
	if mode != ModeOff {
		runtime.ProcessStatus = "running"
		if loadError != "" || runtime.DatabaseStatus != "ok" || runtime.RedisStatus != "ok" || activeVersion != expected {
			runtime.ProcessStatus = "degraded"
	REDACTED
		if heartbeat == nil || s.clock.Now().Sub(*heartbeat) > 10*time.Second {
			runtime.ProcessStatus = "degraded"
	REDACTED
REDACTED
	return runtime
REDACTED

type ProbeRequest struct {
	Endpoint UpdateEndpoint `json:"endpoint"`
REDACTED

func (s *PromptService) Probe(ctx context.Context, request ProbeRequest) ProbeResult {
	started := s.clock.Now()
	endpoint, tokenApplied, err := s.resolveProbeEndpoint(request.Endpoint)
	if err != nil {
		return s.finishProbe(request.Endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: "endpoint_invalid", Message: "审计节点配置无效"REDACTED)
REDACTED
	LogInfo(EventProbeStarted, map[string]any{"guard_endpoint_id": endpoint.ID, "status": "started"REDACTED)
	client, err := NewSecureHTTPClient(endpoint)
	if err != nil {
		return s.finishProbe(endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: "endpoint_unsafe", Message: "审计节点地址不在允许范围", TokenApplied: tokenAppliedREDACTED)
REDACTED
	modelsURL, _ := ModelsURL(endpoint.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return s.finishProbe(endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: "probe_request_invalid", Message: "无法创建探测请求", TokenApplied: tokenAppliedREDACTED)
REDACTED
	if endpoint.Token != "" {
		req.Header.Set("Authorization", "Bearer "+endpoint.Token)
REDACTED
	resp, err := client.Do(req)
	if err != nil {
		code := "connection_failed"
		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			code = "timeout"
	REDACTED
		return s.finishProbe(endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: code, Message: "无法连接审计节点", Retryable: true, TokenApplied: tokenAppliedREDACTED)
REDACTED
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxGuardResponseBytes+1))
	_ = resp.Body.Close()
	if readErr != nil {
		return s.finishProbe(endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: "response_read_failed", Message: "审计节点响应读取失败", HTTPStatus: resp.StatusCode, Retryable: true, TokenApplied: tokenAppliedREDACTED)
REDACTED
	if int64(len(responseBody)) > maxGuardResponseBytes {
		return s.finishProbe(endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: "response_too_large", Message: "审计节点响应无效", HTTPStatus: resp.StatusCode, TokenApplied: tokenAppliedREDACTED)
REDACTED
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && modelsResponseReady(responseBody, endpoint.Model) {
		return s.finishProbe(endpoint.ID, started, ProbeResult{OK: true, Status: "healthy", Message: "审计节点连接正常", HTTPStatus: resp.StatusCode, TokenApplied: tokenAppliedREDACTED)
REDACTED
	if (resp.StatusCode >= 200 && resp.StatusCode < 300) || resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		result, scanErr := s.scanner.Scan(ctx, endpoint, "Hello", AllScannerIDs)
		if scanErr == nil && result != nil {
			return s.finishProbe(endpoint.ID, started, ProbeResult{OK: true, Status: "healthy", Message: "审计节点模型调用正常", HTTPStatus: http.StatusOK, TokenApplied: tokenAppliedREDACTED)
	REDACTED
		code, status, retryable := guardErrorCode(scanErr), 0, false
		var guardErr *GuardError
		if errors.As(scanErr, &guardErr) {
			status, retryable = guardErr.HTTPStatus, guardErr.Retryable
	REDACTED
		if code == "" {
			code = ErrorCodeInvalidResponse
	REDACTED
		return s.finishProbe(endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: code, Message: "审计节点模型调用失败", HTTPStatus: status, Retryable: retryable, TokenApplied: tokenAppliedREDACTED)
REDACTED
	code, retryable := "probe_http_error", resp.StatusCode == 429 || resp.StatusCode >= 500
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		code = "authentication_failed"
REDACTED
	return s.finishProbe(endpoint.ID, started, ProbeResult{Status: "failed", ErrorCode: code, Message: "审计节点探测失败", HTTPStatus: resp.StatusCode, Retryable: retryable, TokenApplied: tokenAppliedREDACTED)
REDACTED

func modelsResponseReady(body []byte, model string) bool {
	var response struct {
		Data []struct {
			ID string `json:"id"`
	REDACTED `json:"data"`
REDACTED
	if json.Unmarshal(body, &response) != nil || response.Data == nil {
		return false
REDACTED
	model = strings.TrimSpace(model)
	if model == "" {
		return true
REDACTED
	for _, item := range response.Data {
		if strings.TrimSpace(item.ID) == model {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func (s *PromptService) resolveProbeEndpoint(input UpdateEndpoint) (ActiveEndpoint, bool, error) {
	baseURL, err := NormalizeBaseURL(input.BaseURL)
	if err != nil {
		return ActiveEndpoint{REDACTED, false, err
REDACTED
	token := strings.TrimSpace(input.Token)
	if token == "" {
		if cfg, ok := s.config.Active(); ok {
			for _, endpoint := range cfg.Endpoints {
				if endpoint.ID != strings.TrimSpace(input.ID) {
					continue
			REDACTED
				// Reuse a stored credential only when the probe targets the same
				// normalized base URL. Otherwise an admin probe could exfiltrate
				// the Guard token to an attacker-controlled HTTPS host.
				if endpoint.BaseURL == baseURL {
					token = endpoint.Token
			REDACTED
				break
		REDACTED
	REDACTED
REDACTED
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = DefaultGuardModel
REDACTED
	timeout := input.TimeoutMS
	if timeout == 0 {
		timeout = DefaultTimeoutMS
REDACTED
	limit := input.InputLimit
	if limit == 0 {
		limit = DefaultInputLimit
REDACTED
	storage := storageConfig{Enabled: false, Strategy: "priority", WorkerCount: DefaultWorkerCount, QueueCapacity: DefaultQueueCapacity, Scanners: append([]string(nil), AllScannerIDs...), AllGroups: true,
		Endpoints: []StorageEndpoint{{ID: strings.TrimSpace(input.ID), Name: strings.TrimSpace(input.Name), Protocol: "openai_compatible", BaseURL: baseURL, Model: model, TimeoutMS: timeout, InputLimit: limitREDACTEDREDACTEDREDACTED
	if storage.Endpoints[0].ID == "" {
		storage.Endpoints[0].ID = "probe"
REDACTED
	if storage.Endpoints[0].Name == "" {
		storage.Endpoints[0].Name = "Probe"
REDACTED
	if err := validateStorageConfig(storage); err != nil {
		return ActiveEndpoint{REDACTED, false, err
REDACTED
	return ActiveEndpoint{ID: storage.Endpoints[0].ID, Name: storage.Endpoints[0].Name, Protocol: "openai_compatible", BaseURL: baseURL, Model: model, Token: token, TimeoutMS: timeout, InputLimit: limit, Enabled: trueREDACTED, token != "", nil
REDACTED

func (s *PromptService) finishProbe(id string, started time.Time, result ProbeResult) ProbeResult {
	result.CheckedAt = s.clock.Now()
	result.LatencyMS = int(result.CheckedAt.Sub(started).Milliseconds())
	if result.OK {
		LogInfo(EventProbeFinished, map[string]any{"guard_endpoint_id": id, "status": result.Status, "latency_ms": result.LatencyMS, "http_status": result.HTTPStatusREDACTED)
REDACTED else {
		LogWarn(EventProbeFailed, map[string]any{"guard_endpoint_id": id, "status": result.Status, "latency_ms": result.LatencyMS, "http_status": result.HTTPStatus, "error_code": result.ErrorCode, "retryable": result.RetryableREDACTED)
REDACTED
	s.probeMu.Lock()
	s.probes[id] = result
	s.probeMu.Unlock()
	return result
REDACTED

func (s *PromptService) probeSnapshot() map[string]ProbeResult {
	s.probeMu.RLock()
	defer s.probeMu.RUnlock()
	result := make(map[string]ProbeResult, len(s.probes))
	for id, probe := range s.probes {
		result[id] = probe
REDACTED
	return result
REDACTED

func (s *PromptService) ListEvents(ctx context.Context, filter EventFilter, page, pageSize int) (*EventPage, error) {
	return s.repo.ListEvents(ctx, filter, page, pageSize)
REDACTED
func (s *PromptService) GetEvent(ctx context.Context, id int64) (*Event, error) {
	return s.repo.GetEvent(ctx, id)
REDACTED

func (s *PromptService) DeleteEvent(ctx context.Context, id int64) (*DeleteResult, error) {
	result, err := s.repo.DeleteEvent(ctx, id)
	if err == nil {
		s.deletePayloads(ctx, result.JobIDs)
REDACTED
	return result, err
REDACTED
func (s *PromptService) DeleteEventsByIDs(ctx context.Context, ids []int64) (*DeleteResult, error) {
	result, err := s.repo.DeleteEventsByIDs(ctx, ids)
	if err == nil {
		s.deletePayloads(ctx, result.JobIDs)
REDACTED
	return result, err
REDACTED

type deleteClaims struct {
	FilterHash    string    `json:"filter_hash"`
	SnapshotMaxID int64     `json:"snapshot_max_id"`
	AdminID       int64     `json:"admin_id"`
	IssuedAt      time.Time `json:"issued_at"`
	ExpiresAt     time.Time `json:"expires_at"`
REDACTED

func (s *PromptService) PreviewDelete(ctx context.Context, filter EventFilter, adminID int64) (*DeletePreview, error) {
	preview, err := s.repo.PreviewDelete(ctx, filter)
	if err != nil {
		return nil, err
REDACTED
	now := s.clock.Now()
	expires := now.Add(5 * time.Minute)
	claimsRaw, _ := json.Marshal(deleteClaims{FilterHash: preview.FilterHash, SnapshotMaxID: preview.SnapshotMaxID, AdminID: adminID, IssuedAt: now, ExpiresAt: expiresREDACTED)
	token, err := s.config.Encrypt(string(claimsRaw))
	if err != nil {
		return nil, err
REDACTED
	preview.ConfirmationToken, preview.ExpiresAt = token, expires
	LogInfo(EventDeletePreviewed, map[string]any{"user_id": adminID, "status": "previewed"REDACTED)
	return preview, nil
REDACTED

type DeleteByFilterRequest struct {
	Filter            EventFilter `json:"filter"`
	SnapshotMaxID     int64       `json:"snapshot_max_id"`
	FilterHash        string      `json:"filter_hash"`
	ConfirmationToken string      `json:"confirmation_token"`
	Confirm           bool        `json:"confirm"`
REDACTED

func (s *PromptService) DeleteByFilter(ctx context.Context, request DeleteByFilterRequest, adminID int64) (*DeleteResult, error) {
	if !request.Confirm {
		return nil, errors.New("prompt audit filter delete requires confirm=true")
REDACTED
	plain, err := s.config.Decrypt(strings.TrimSpace(request.ConfirmationToken))
	if err != nil {
		return nil, errors.New("prompt audit confirmation token invalid")
REDACTED
	var claims deleteClaims
	if json.Unmarshal([]byte(plain), &claims) != nil {
		return nil, errors.New("prompt audit confirmation token invalid")
REDACTED
	computed := FilterHash(request.Filter, request.SnapshotMaxID)
	if claims.AdminID != adminID || claims.SnapshotMaxID != request.SnapshotMaxID || claims.FilterHash != request.FilterHash || request.FilterHash != computed || !s.clock.Now().Before(claims.ExpiresAt) {
		return nil, errors.New("prompt audit confirmation token does not match deletion request")
REDACTED
	result, err := s.repo.DeleteEventsByFilter(ctx, request.Filter, request.SnapshotMaxID, 200)
	if err == nil {
		s.deletePayloads(ctx, result.JobIDs)
		LogWarn(EventEventsFilterDeleted, map[string]any{"user_id": adminID, "status": "deleted"REDACTED)
REDACTED
	return result, err
REDACTED

func (s *PromptService) deletePayloads(ctx context.Context, jobIDs []int64) {
	for _, id := range jobIDs {
		_ = s.payload.Delete(ctx, id)
REDACTED
REDACTED

func parseTimeQuery(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return nil
REDACTED
	parsed = parsed.UTC()
	return &parsed
REDACTED
