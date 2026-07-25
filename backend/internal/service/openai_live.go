package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	coderws "github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const (
	defaultLiveMaxSessionDuration = time.Hour
	liveLeaseRefreshInterval      = 20 * time.Second
	liveRedisOperationTimeout     = 3 * time.Second
	liveClosedRecordTTL           = 24 * time.Hour
	liveObserverPollInterval      = 250 * time.Millisecond
	liveUpstreamBodyLimit         = 2 << 20
)

var (
	chatGPTLiveCallsURL        = "https://chatgpt.com/backend-api/codex/realtime/calls?intent=quicksilver&architecture=avas"
	chatGPTLiveSidebandBaseURL = "wss://chatgpt.com/backend-api/codex"
)

type liveFrameConn interface {
	ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error)
	WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error
	Close() error
REDACTED

func liveSidebandReadError(err error) error {
	if coderws.CloseStatus(err) == coderws.StatusNormalClosure {
		return ErrLiveCallNotFound
REDACTED
	return err
REDACTED

func hashLiveCallID(callID string) string {
	sum := sha256.Sum256([]byte(callID))
	return hex.EncodeToString(sum[:])
REDACTED

func liveGroupID(groupID *int64) int64 {
	if groupID == nil {
		return 0
REDACTED
	return *groupID
REDACTED

func liveOptionalID(value int64) *int64 {
	if value <= 0 {
		return nil
REDACTED
	result := value
	return &result
REDACTED

func (s *OpenAIGatewayService) liveStore() (LiveCallStore, error) {
	if s == nil || s.cache == nil {
		return nil, ErrLiveUnavailable
REDACTED
	store, ok := s.cache.(LiveCallStore)
	if !ok {
		return nil, ErrLiveUnavailable
REDACTED
	return store, nil
REDACTED

func (s *OpenAIGatewayService) liveConcurrencyCache() (LiveConcurrencyCache, error) {
	if s == nil || s.concurrencyService == nil || s.concurrencyService.cache == nil {
		return nil, ErrLiveUnavailable
REDACTED
	cache, ok := s.concurrencyService.cache.(LiveConcurrencyCache)
	if !ok {
		return nil, ErrLiveUnavailable
REDACTED
	return cache, nil
REDACTED

func (s *OpenAIGatewayService) liveMaxSessionDuration() time.Duration {
	if s != nil && s.cfg != nil && s.cfg.Gateway.Live.MaxSessionDurationSeconds > 0 {
		return time.Duration(s.cfg.Gateway.Live.MaxSessionDurationSeconds) * time.Second
REDACTED
	return defaultLiveMaxSessionDuration
REDACTED

func ValidateLiveCallRequest(request *LiveCallRequest) error {
	if request == nil || strings.TrimSpace(request.SDP) == "" {
		return errors.New("sdp is required")
REDACTED
	if len(request.Session) == 0 || !json.Valid(request.Session) {
		return errors.New("session must be valid JSON")
REDACTED
	var sessionObject map[string]json.RawMessage
	if err := json.Unmarshal(request.Session, &sessionObject); err != nil {
		return errors.New("session must be a JSON object")
REDACTED
	if sessionObject == nil {
		return errors.New("session must be a JSON object")
REDACTED
	return nil
REDACTED

// CreateLiveCall 创建 Frameless 会话。调用方须在调用期间持有普通用户槽位；
// 调度器持有的普通账号槽位会被同一个 Live 租约原子接替。
func (s *OpenAIGatewayService) CreateLiveCall(
	ctx context.Context,
	request *LiveCallRequest,
	identity LiveCallIdentity,
	userMaxConcurrency int,
) (*LiveCallCreated, error) {
	if err := ValidateLiveCallRequest(request); err != nil {
		return nil, err
REDACTED
	store, err := s.liveStore()
	if err != nil {
		return nil, err
REDACTED
	liveCache, err := s.liveConcurrencyCache()
	if err != nil {
		return nil, err
REDACTED

	excluded := make(map[int64]struct{REDACTED)
	var lastErr error
	for attempt := 0; attempt <= 3; attempt++ {
		selection, _, selectErr := s.SelectAccountWithSchedulerForCapability(
			ctx,
			identity.GroupID,
			"",
			uuid.NewString(),
			"",
			excluded,
			OpenAIUpstreamTransportHTTPSSE,
			OpenAIEndpointCapabilityLive,
			false,
			false,
			false,
		)
		if selectErr != nil {
			if lastErr != nil {
				return nil, lastErr
		REDACTED
			return nil, selectErr
	REDACTED
		if selection == nil || selection.Account == nil || !selection.Acquired {
			if selection != nil && selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
		REDACTED
			return nil, ErrLiveConcurrencyFull
	REDACTED

		account := selection.Account
		leaseID := generateRequestID()
		acquired, acquireErr := liveCache.AcquireLiveLease(
			ctx,
			account.ID,
			account.Concurrency,
			identity.UserID,
			userMaxConcurrency,
			identity.APIKeyID,
			leaseID,
			true,
		)
		if acquireErr != nil || !acquired {
			selection.ReleaseFunc()
			if acquireErr != nil {
				return nil, acquireErr
		REDACTED
			return nil, ErrLiveConcurrencyFull
	REDACTED

		created, createErr := s.createUpstreamLiveCall(ctx, account, request)
		selection.ReleaseFunc()
		if createErr != nil {
			s.releaseLiveLease(account.ID, identity.UserID, identity.APIKeyID, leaseID)
			if !s.shouldFailoverLiveCreateError(createErr) {
				return nil, createErr
		REDACTED
			excluded[account.ID] = struct{REDACTED{REDACTED
			lastErr = createErr
			continue
	REDACTED

		now := time.Now()
		model := strings.TrimSpace(gjson.GetBytes(request.Session, "model").String())
		if model == "" {
			model = "gpt-live"
	REDACTED
		record := &LiveCallRecord{
			CallID:          created.CallID,
			CallHash:        hashLiveCallID(created.CallID),
			AccountID:       account.ID,
			APIKeyID:        identity.APIKeyID,
			UserID:          identity.UserID,
			GroupID:         liveGroupID(identity.GroupID),
			SubscriptionID:  liveGroupID(identity.SubscriptionID),
			LeaseID:         leaseID,
			Model:           model,
			CreatedAt:       now,
			ExpiresAt:       now.Add(s.liveMaxSessionDuration()),
			Controller:      LiveControllerPending,
			UserAgent:       identity.UserAgent,
			IPAddress:       identity.IPAddress,
			InboundEndpoint: identity.InboundEndpoint,
	REDACTED
		mappingTTL := s.liveMaxSessionDuration() + 5*time.Minute
		if saveErr := store.SaveLiveCall(ctx, record, mappingTTL); saveErr != nil {
			s.releaseLiveLease(account.ID, identity.UserID, identity.APIKeyID, leaseID)
			return nil, fmt.Errorf("save live call mapping: %w", saveErr)
	REDACTED
		created.Account = account
		go s.observeLiveCall(record.CallHash)
		return created, nil
REDACTED
	if lastErr != nil {
		return nil, lastErr
REDACTED
	return nil, ErrLiveUnavailable
REDACTED

func (s *OpenAIGatewayService) shouldFailoverLiveCreateError(err error) bool {
	var upstreamErr *UpstreamFailoverError
	if !errors.As(err, &upstreamErr) {
		// 凭证读取和网络传输错误都可能只影响当前账号或代理。
		return true
REDACTED
	return s.shouldFailoverOpenAIUpstreamResponse(
		upstreamErr.StatusCode,
		"",
		upstreamErr.ResponseBody,
	)
REDACTED

func (s *OpenAIGatewayService) createUpstreamLiveCall(
	ctx context.Context,
	account *Account,
	request *LiveCallRequest,
) (*LiveCallCreated, error) {
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		logLiveCreateStageFailure(ctx, account.ID, "access_token", err)
		return nil, err
REDACTED
	body, err := json.Marshal(struct {
		SDP     string          `json:"sdp"`
		Session json.RawMessage `json:"session"`
REDACTED{
		SDP:     request.SDP,
		Session: request.Session,
REDACTED)
	if err != nil {
		return nil, err
REDACTED
	reqCtx := WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(ctx, HTTPUpstreamProfileOpenAI))
	upstreamReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, chatGPTLiveCallsURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED
	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		logLiveCreateStageFailure(ctx, account.ID, "authentication_headers", err)
		return nil, err
REDACTED
	for key, values := range authHeaders {
		for _, value := range values {
			upstreamReq.Header.Add(key, value)
	REDACTED
REDACTED
	upstreamReq.Host = "chatgpt.com"
	if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, upstreamReq.Header, account); err != nil {
		logLiveCreateStageFailure(ctx, account.ID, "account_headers", err)
		return nil, err
REDACTED
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", "application/sdp")
	applyLiveUpstreamIdentityHeaders(upstreamReq.Header)

	resp, err := s.httpUpstream.Do(upstreamReq, resolveAccountProxyURL(account), account.ID, account.Concurrency)
	if err != nil {
		logLiveCreateStageFailure(ctx, account.ID, "upstream_transport", err)
		return nil, err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, liveUpstreamBodyLimit+1))
	if readErr != nil {
		return nil, readErr
REDACTED
	if len(responseBody) > liveUpstreamBodyLimit {
		return nil, errors.New("live upstream response is too large")
REDACTED
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logLiveUpstreamFailure(ctx, account.ID, resp.StatusCode, resp.Header, responseBody)
		return nil, &UpstreamFailoverError{
			StatusCode:      resp.StatusCode,
			ResponseBody:    responseBody,
			ResponseHeaders: resp.Header.Clone(),
	REDACTED
REDACTED
	callID, err := liveCallIDFromLocation(resp.Header.Get("Location"))
	if err != nil {
		return nil, err
REDACTED
	return &LiveCallCreated{
		SDP:      responseBody,
		CallID:   callID,
		Location: resp.Header.Get("Location"),
REDACTED, nil
REDACTED

func logLiveCreateStageFailure(ctx context.Context, accountID int64, stage string, err error) {
	logger.FromContext(ctx).Warn(
		"OpenAI Live 创建阶段失败",
		zap.Int64("account_id", accountID),
		zap.String("stage", stage),
		zap.String("error_type", fmt.Sprintf("%T", err)),
	)
REDACTED

func logLiveUpstreamFailure(
	ctx context.Context,
	accountID int64,
	statusCode int,
	headers http.Header,
	body []byte,
) {
	errorType := strings.TrimSpace(gjson.GetBytes(body, "error.type").String())
	errorCode := strings.TrimSpace(gjson.GetBytes(body, "error.code").String())
	errorMessage := strings.TrimSpace(gjson.GetBytes(body, "error.message").String())
	if errorType == "" {
		errorType = strings.TrimSpace(gjson.GetBytes(body, "type").String())
REDACTED
	if errorCode == "" {
		errorCode = strings.TrimSpace(gjson.GetBytes(body, "code").String())
REDACTED
	if errorMessage == "" {
		errorMessage = strings.TrimSpace(gjson.GetBytes(body, "message").String())
REDACTED
	if errorMessage == "" {
		errorMessage = strings.TrimSpace(gjson.GetBytes(body, "detail").String())
REDACTED

	logger.FromContext(ctx).Warn(
		"OpenAI Live 上游拒绝请求",
		zap.Int64("account_id", accountID),
		zap.Int("upstream_status_code", statusCode),
		zap.String("upstream_error_type", truncateOpenAIWSLogValue(errorType, 120)),
		zap.String("upstream_error_code", truncateOpenAIWSLogValue(errorCode, 120)),
		zap.String("upstream_error_message", truncateOpenAIWSLogValue(errorMessage, 300)),
		zap.String("upstream_content_type", truncateOpenAIWSLogValue(headers.Get("Content-Type"), 120)),
		zap.String("upstream_server", truncateOpenAIWSLogValue(headers.Get("Server"), 120)),
		zap.String("upstream_cf_mitigated", truncateOpenAIWSLogValue(headers.Get("Cf-Mitigated"), 120)),
		zap.String("upstream_cf_ray", truncateOpenAIWSLogValue(headers.Get("Cf-Ray"), 120)),
		zap.String("upstream_request_id", truncateOpenAIWSLogValue(headers.Get("X-Request-Id"), 120)),
	)
REDACTED

func liveCallIDFromLocation(location string) (string, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return "", errors.New("live upstream response has no Location")
REDACTED
	parsed, err := url.Parse(location)
	if err != nil {
		return "", fmt.Errorf("parse live Location: %w", err)
REDACTED
	callID := strings.TrimSpace(path.Base(strings.TrimSuffix(parsed.Path, "/")))
	if callID == "" || callID == "." || callID == "codex" {
		return "", errors.New("live upstream Location has no call id")
REDACTED
	return callID, nil
REDACTED

func applyLiveUpstreamIdentityHeaders(headers http.Header) {
	headers.Set("OpenAI-Alpha", "quicksilver=v2")
	ensureCodexIdentityHeaders(headers)
	enforceCodexIdentityHeaders(headers)
	if strings.TrimSpace(headers.Get("session-id")) == "" {
		headers.Set("session-id", uuid.NewString())
REDACTED
	if strings.TrimSpace(headers.Get("thread-id")) == "" {
		headers.Set("thread-id", uuid.NewString())
REDACTED
	// Realtime/Live 不使用 Responses 的实验头。
	headers.Del("OpenAI-Beta")
REDACTED

func (s *OpenAIGatewayService) liveSidebandHeaders(ctx context.Context, account *Account) (http.Header, error) {
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED
	headers, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, err
REDACTED
	if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, headers, account); err != nil {
		return nil, err
REDACTED
	applyLiveUpstreamIdentityHeaders(headers)
	return headers, nil
REDACTED

func (s *OpenAIGatewayService) dialLiveSideband(ctx context.Context, record *LiveCallRecord) (liveFrameConn, error) {
	account, err := s.accountRepo.GetByID(ctx, record.AccountID)
	if err != nil {
		return nil, err
REDACTED
	if account == nil || !account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive) {
		return nil, ErrLiveUnavailable
REDACTED
	headers, err := s.liveSidebandHeaders(ctx, account)
	if err != nil {
		return nil, err
REDACTED
	target := strings.TrimRight(chatGPTLiveSidebandBaseURL, "/") + "/" + url.PathEscape(record.CallID)
	conn, status, _, err := s.getOpenAIWSPassthroughDialer().Dial(ctx, target, headers, resolveAccountProxyURL(account))
	if err != nil {
		return nil, fmt.Errorf("dial live sideband (status %d): %w", status, err)
REDACTED
	raw, ok := conn.(liveFrameConn)
	if !ok {
		_ = conn.Close()
		return nil, errors.New("live sideband transport does not support raw frames")
REDACTED
	return raw, nil
REDACTED

func (s *OpenAIGatewayService) GetLiveCallForIdentity(
	ctx context.Context,
	callID string,
	identity LiveCallIdentity,
) (*LiveCallRecord, error) {
	store, err := s.liveStore()
	if err != nil {
		return nil, err
REDACTED
	record, err := store.GetLiveCall(ctx, hashLiveCallID(callID))
	if err != nil {
		return nil, err
REDACTED
	if record.CallID != callID ||
		record.APIKeyID != identity.APIKeyID ||
		record.UserID != identity.UserID ||
		record.GroupID != liveGroupID(identity.GroupID) {
		return nil, ErrLiveIdentityMismatch
REDACTED
	if record.Controller == LiveControllerClosed {
		return nil, ErrLiveCallNotFound
REDACTED
	return record, nil
REDACTED

// ProxyLiveSideband 让认证后的客户端接管控制连接；媒体始终不经过这里。
func (s *OpenAIGatewayService) ProxyLiveSideband(
	ctx context.Context,
	record *LiveCallRecord,
	downstream *coderws.Conn,
) error {
	if record == nil || downstream == nil {
		return ErrLiveCallNotFound
REDACTED
	store, err := s.liveStore()
	if err != nil {
		return err
REDACTED
	owner := uuid.NewString()
	claimed, err := store.ClaimLiveController(ctx, record.CallHash, LiveControllerProxy, owner)
	if err != nil {
		return err
REDACTED
	if !claimed {
		return ErrLiveControllerChanged
REDACTED

	// observer 轮询到接管状态后会关闭旧控制连接；同一个 call 可重新加入。
	time.Sleep(liveObserverPollInterval)
	upstream, err := s.dialLiveSideband(ctx, record)
	if err != nil {
		_, _ = store.ReleaseLiveController(context.Background(), record.CallHash, owner)
		go s.observeLiveCall(record.CallHash)
		return err
REDACTED
	defer upstream.Close()
	downstream.SetReadLimit(openAIWSMessageReadLimitBytes)

	proxyCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 2)
	go func() {
		for {
			messageType, payload, readErr := downstream.Read(proxyCtx)
			if readErr != nil {
				errCh <- readErr
				return
		REDACTED
			if writeErr := upstream.WriteFrame(proxyCtx, messageType, payload); writeErr != nil {
				errCh <- writeErr
				return
		REDACTED
	REDACTED
REDACTED()
	go func() {
		for {
			messageType, payload, readErr := upstream.ReadFrame(proxyCtx)
			if readErr != nil {
				errCh <- liveSidebandReadError(readErr)
				return
		REDACTED
			if writeErr := downstream.Write(proxyCtx, messageType, payload); writeErr != nil {
				errCh <- writeErr
				return
		REDACTED
			if messageType == coderws.MessageText {
				eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
				if eventType == "session.closed" || eventType == "session.ended" {
					errCh <- ErrLiveCallNotFound
					return
			REDACTED
		REDACTED
	REDACTED
REDACTED()

	runErr := s.runLiveController(proxyCtx, record, upstream, errCh)
	cancel()
	_, _ = store.ReleaseLiveController(context.Background(), record.CallHash, owner)
	if errors.Is(runErr, ErrLiveCallNotFound) {
		s.finalizeLiveCall(record)
		return runErr
REDACTED
	if !errors.Is(runErr, context.DeadlineExceeded) && time.Now().Before(record.ExpiresAt) {
		go s.observeLiveCall(record.CallHash)
		return runErr
REDACTED
	s.finalizeLiveCall(record)
	return runErr
REDACTED

func (s *OpenAIGatewayService) runLiveController(
	ctx context.Context,
	record *LiveCallRecord,
	upstream liveFrameConn,
	errCh <-chan error,
) error {
	refreshTicker := time.NewTicker(liveLeaseRefreshInterval)
	defer refreshTicker.Stop()
	maxTimer := time.NewTimer(time.Until(record.ExpiresAt))
	defer maxTimer.Stop()
	for {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case err := <-errCh:
			return err
		case <-maxTimer.C:
			closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = upstream.WriteFrame(closeCtx, coderws.MessageText, []byte(`{"type":"session.close"REDACTED`))
			cancel()
			return context.DeadlineExceeded
		case <-refreshTicker.C:
			if !s.refreshLiveLease(record) {
				return ErrLiveUnavailable
		REDACTED
	REDACTED
REDACTED
REDACTED

func (s *OpenAIGatewayService) observeLiveCall(callHash string) {
	store, err := s.liveStore()
	if err != nil {
		return
REDACTED
	owner := uuid.NewString()
	claimed, err := store.ClaimLiveController(context.Background(), callHash, LiveControllerObserver, owner)
	if err != nil || !claimed {
		return
REDACTED
	for {
		record, getErr := store.GetLiveCall(context.Background(), callHash)
		if getErr != nil || record.Controller != LiveControllerObserver {
			return
	REDACTED
		if !time.Now().Before(record.ExpiresAt) {
			s.finalizeLiveCall(record)
			return
	REDACTED
		upstream, dialErr := s.dialLiveSideband(context.Background(), record)
		if dialErr != nil {
			if !s.waitForLiveObserverRetry(record) {
				return
		REDACTED
			continue
	REDACTED
		runErr := s.runLiveObserverConnection(record, upstream)
		_ = upstream.Close()
		if errors.Is(runErr, ErrLiveControllerChanged) {
			return
	REDACTED
		if errors.Is(runErr, context.DeadlineExceeded) || errors.Is(runErr, ErrLiveCallNotFound) {
			s.finalizeLiveCall(record)
			return
	REDACTED
		if !s.waitForLiveObserverRetry(record) {
			return
	REDACTED
REDACTED
REDACTED

func (s *OpenAIGatewayService) runLiveObserverConnection(record *LiveCallRecord, upstream liveFrameConn) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	frameCh := make(chan []byte, 1)
	errCh := make(chan error, 1)
	go func() {
		for {
			messageType, payload, err := upstream.ReadFrame(ctx)
			if err != nil {
				select {
				case errCh <- liveSidebandReadError(err):
				case <-ctx.Done():
			REDACTED
				return
		REDACTED
			if messageType == coderws.MessageText {
				select {
				case frameCh <- payload:
				case <-ctx.Done():
					return
			REDACTED
		REDACTED
	REDACTED
REDACTED()
	refreshTicker := time.NewTicker(liveLeaseRefreshInterval)
	defer refreshTicker.Stop()
	controllerTicker := time.NewTicker(liveObserverPollInterval)
	defer controllerTicker.Stop()
	maxTimer := time.NewTimer(time.Until(record.ExpiresAt))
	defer maxTimer.Stop()
	store, _ := s.liveStore()
	for {
		select {
		case payload := <-frameCh:
			eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
			if eventType == "session.closed" || eventType == "session.ended" {
				return ErrLiveCallNotFound
		REDACTED
		case err := <-errCh:
			return err
		case <-controllerTicker.C:
			controller, err := store.GetLiveController(context.Background(), record.CallHash)
			if err != nil {
				return err
		REDACTED
			if controller != LiveControllerObserver {
				return ErrLiveControllerChanged
		REDACTED
		case <-refreshTicker.C:
			if !s.refreshLiveLease(record) {
				return ErrLiveUnavailable
		REDACTED
		case <-maxTimer.C:
			closeCtx, closeCancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = upstream.WriteFrame(closeCtx, coderws.MessageText, []byte(`{"type":"session.close"REDACTED`))
			closeCancel()
			return context.DeadlineExceeded
	REDACTED
REDACTED
REDACTED

func (s *OpenAIGatewayService) waitForLiveObserverRetry(record *LiveCallRecord) bool {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	<-timer.C
	store, err := s.liveStore()
	if err != nil {
		return false
REDACTED
	controller, err := store.GetLiveController(context.Background(), record.CallHash)
	return err == nil && controller == LiveControllerObserver && time.Now().Before(record.ExpiresAt)
REDACTED

func (s *OpenAIGatewayService) refreshLiveLease(record *LiveCallRecord) bool {
	cache, err := s.liveConcurrencyCache()
	if err != nil {
		return false
REDACTED
	ctx, cancel := context.WithTimeout(context.Background(), liveRedisOperationTimeout)
	defer cancel()
	refreshed, err := cache.RefreshLiveLease(ctx, record.AccountID, record.UserID, record.APIKeyID, record.LeaseID)
	return err == nil && refreshed
REDACTED

func (s *OpenAIGatewayService) releaseLiveLease(accountID, userID, apiKeyID int64, leaseID string) {
	cache, err := s.liveConcurrencyCache()
	if err != nil {
		return
REDACTED
	ctx, cancel := context.WithTimeout(context.Background(), liveRedisOperationTimeout)
	defer cancel()
	_ = cache.ReleaseLiveLease(ctx, accountID, userID, apiKeyID, leaseID)
REDACTED

func (s *OpenAIGatewayService) finalizeLiveCall(record *LiveCallRecord) {
	if record == nil {
		return
REDACTED
	store, err := s.liveStore()
	if err != nil {
		return
REDACTED
	ctx, cancel := context.WithTimeout(context.Background(), liveRedisOperationTimeout)
	first, err := store.MarkLiveCallClosed(ctx, record.CallHash, liveClosedRecordTTL)
	cancel()
	if err != nil || !first {
		return
REDACTED
	s.releaseLiveLease(record.AccountID, record.UserID, record.APIKeyID, record.LeaseID)
	if s.usageLogRepo == nil {
		return
REDACTED
	duration := int(time.Since(record.CreatedAt).Milliseconds())
	if duration < 0 {
		duration = 0
REDACTED
	inboundEndpoint := record.InboundEndpoint
	upstreamEndpoint := "/backend-api/codex/realtime/calls"
	userAgent := record.UserAgent
	ipAddress := record.IPAddress
	billingType := int8(BillingTypeBalance)
	if record.SubscriptionID > 0 {
		billingType = BillingTypeSubscription
REDACTED
	_, _ = s.usageLogRepo.Create(context.Background(), &UsageLog{
		UserID:           record.UserID,
		APIKeyID:         record.APIKeyID,
		AccountID:        record.AccountID,
		RequestID:        record.CallHash,
		Model:            record.Model,
		RequestedModel:   record.Model,
		GroupID:          liveOptionalID(record.GroupID),
		SubscriptionID:   liveOptionalID(record.SubscriptionID),
		RateMultiplier:   1,
		BillingType:      billingType,
		RequestType:      RequestTypeLive,
		DurationMs:       &duration,
		UserAgent:        &userAgent,
		IPAddress:        &ipAddress,
		InboundEndpoint:  &inboundEndpoint,
		UpstreamEndpoint: &upstreamEndpoint,
		CreatedAt:        record.CreatedAt,
REDACTED)
REDACTED
