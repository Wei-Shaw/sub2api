package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func (s *OpenAIGatewayService) isOpenAIWSGeneratePrewarmEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.PrewarmGenerateEnabled
REDACTED

// performOpenAIWSGeneratePrewarm 在 WSv2 下执行可选的 generate=false 预热。
// 预热默认关闭，仅在配置开启后生效；失败时按可恢复错误回退到 HTTP。
func (s *OpenAIGatewayService) performOpenAIWSGeneratePrewarm(
	ctx context.Context,
	lease *openAIWSConnLease,
	decision OpenAIWSProtocolDecision,
	payload map[string]any,
	previousResponseID string,
	reqBody map[string]any,
	account *Account,
	stateStore OpenAIWSStateStore,
	groupID int64,
) error {
	if s == nil {
		return nil
REDACTED
	if lease == nil || account == nil {
		logOpenAIWSModeInfo("prewarm_skip reason=invalid_state has_lease=%v has_account=%v", lease != nil, account != nil)
		return nil
REDACTED
	connID := strings.TrimSpace(lease.ConnID())
	if !s.isOpenAIWSGeneratePrewarmEnabled() {
		return nil
REDACTED
	if decision.Transport != OpenAIUpstreamTransportResponsesWebsocketV2 {
		logOpenAIWSModeInfo(
			"prewarm_skip account_id=%d conn_id=%s reason=transport_not_v2 transport=%s",
			account.ID,
			connID,
			normalizeOpenAIWSLogValue(string(decision.Transport)),
		)
		return nil
REDACTED
	if strings.TrimSpace(previousResponseID) != "" {
		logOpenAIWSModeInfo(
			"prewarm_skip account_id=%d conn_id=%s reason=has_previous_response_id previous_response_id=%s",
			account.ID,
			connID,
			truncateOpenAIWSLogValue(previousResponseID, openAIWSIDValueMaxLen),
		)
		return nil
REDACTED
	if lease.IsPrewarmed() {
		logOpenAIWSModeInfo("prewarm_skip account_id=%d conn_id=%s reason=already_prewarmed", account.ID, connID)
		return nil
REDACTED
	if NeedsToolContinuation(reqBody) {
		logOpenAIWSModeInfo("prewarm_skip account_id=%d conn_id=%s reason=tool_continuation", account.ID, connID)
		return nil
REDACTED
	prewarmStart := time.Now()
	logOpenAIWSModeInfo("prewarm_start account_id=%d conn_id=%s", account.ID, connID)

	prewarmPayload := make(map[string]any, len(payload)+1)
	for k, v := range payload {
		prewarmPayload[k] = v
REDACTED
	prewarmPayload["generate"] = false
	prewarmPayloadJSON := payloadAsJSONBytes(prewarmPayload)

	if err := lease.WriteJSONWithContextTimeout(ctx, prewarmPayload, s.openAIWSWriteTimeout()); err != nil {
		lease.MarkBroken()
		logOpenAIWSModeInfo(
			"prewarm_write_fail account_id=%d conn_id=%s cause=%s",
			account.ID,
			connID,
			truncateOpenAIWSLogValue(err.Error(), openAIWSLogValueMaxLen),
		)
		return wrapOpenAIWSFallback("prewarm_write", err)
REDACTED
	logOpenAIWSModeInfo("prewarm_write_sent account_id=%d conn_id=%s payload_bytes=%d", account.ID, connID, len(prewarmPayloadJSON))

	prewarmResponseID := ""
	prewarmEventCount := 0
	prewarmTerminalCount := 0
	for {
		message, readErr := lease.ReadMessageWithContextTimeout(ctx, s.openAIWSReadTimeout())
		if readErr != nil {
			lease.MarkBroken()
			closeStatus, closeReason := summarizeOpenAIWSReadCloseError(readErr)
			logOpenAIWSModeInfo(
				"prewarm_read_fail account_id=%d conn_id=%s close_status=%s close_reason=%s cause=%s events=%d",
				account.ID,
				connID,
				closeStatus,
				closeReason,
				truncateOpenAIWSLogValue(readErr.Error(), openAIWSLogValueMaxLen),
				prewarmEventCount,
			)
			return wrapOpenAIWSFallback("prewarm_"+classifyOpenAIWSReadFallbackReason(readErr), readErr)
	REDACTED

		eventType, eventResponseID, _ := parseOpenAIWSEventEnvelope(message)
		if eventType == "" {
			continue
	REDACTED
		prewarmEventCount++
		if prewarmResponseID == "" && eventResponseID != "" {
			prewarmResponseID = eventResponseID
	REDACTED
		if prewarmEventCount <= openAIWSPrewarmEventLogHead || eventType == "error" || isOpenAIWSTerminalEvent(eventType) {
			logOpenAIWSModeInfo(
				"prewarm_event account_id=%d conn_id=%s idx=%d type=%s bytes=%d",
				account.ID,
				connID,
				prewarmEventCount,
				truncateOpenAIWSLogValue(eventType, openAIWSLogValueMaxLen),
				len(message),
			)
	REDACTED

		if eventType == "error" {
			errCodeRaw, errTypeRaw, errMsgRaw := parseOpenAIWSErrorEventFields(message)
			s.persistOpenAIWSRateLimitSignal(ctx, account, lease.HandshakeHeaders(), message, errCodeRaw, errTypeRaw, errMsgRaw)
			errMsg := strings.TrimSpace(errMsgRaw)
			if errMsg == "" {
				errMsg = "OpenAI websocket prewarm error"
		REDACTED
			fallbackReason, canFallback := classifyOpenAIWSErrorEventFromRaw(errCodeRaw, errTypeRaw, errMsgRaw)
			errCode, errType, errMessage := summarizeOpenAIWSErrorEventFieldsFromRaw(errCodeRaw, errTypeRaw, errMsgRaw)
			logOpenAIWSModeInfo(
				"prewarm_error_event account_id=%d conn_id=%s idx=%d fallback_reason=%s can_fallback=%v err_code=%s err_type=%s err_message=%s",
				account.ID,
				connID,
				prewarmEventCount,
				truncateOpenAIWSLogValue(fallbackReason, openAIWSLogValueMaxLen),
				canFallback,
				errCode,
				errType,
				errMessage,
			)
			lease.MarkBroken()
			if canFallback {
				return wrapOpenAIWSFallback("prewarm_"+fallbackReason, errors.New(errMsg))
		REDACTED
			return wrapOpenAIWSFallback("prewarm_error_event", errors.New(errMsg))
	REDACTED

		if isOpenAIWSTerminalEvent(eventType) {
			prewarmTerminalCount++
			break
	REDACTED
REDACTED

	lease.MarkPrewarmed()
	if prewarmResponseID != "" && stateStore != nil {
		ttl := s.openAIWSResponseStickyTTL()
		logOpenAIWSBindResponseAccountWarn(groupID, account.ID, prewarmResponseID, stateStore.BindResponseAccount(ctx, groupID, prewarmResponseID, account.ID, ttl))
		stateStore.BindResponseConn(prewarmResponseID, lease.ConnID(), ttl)
REDACTED
	logOpenAIWSModeInfo(
		"prewarm_done account_id=%d conn_id=%s response_id=%s events=%d terminal_events=%d duration_ms=%d",
		account.ID,
		connID,
		truncateOpenAIWSLogValue(prewarmResponseID, openAIWSIDValueMaxLen),
		prewarmEventCount,
		prewarmTerminalCount,
		time.Since(prewarmStart).Milliseconds(),
	)
	return nil
REDACTED

func payloadAsJSON(payload map[string]any) string {
	return string(payloadAsJSONBytes(payload))
REDACTED

func payloadAsJSONBytes(payload map[string]any) []byte {
	if len(payload) == 0 {
		return []byte("{REDACTED")
REDACTED
	body, err := json.Marshal(payload)
	if err != nil {
		return []byte("{REDACTED")
REDACTED
	return body
REDACTED

func isOpenAIWSTerminalEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
REDACTED
REDACTED

func isOpenAIWSTokenEvent(eventType string) bool {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return false
REDACTED
	switch eventType {
	case "response.created", "response.in_progress", "response.output_item.added", "response.output_item.done":
		return false
REDACTED
	if strings.Contains(eventType, ".delta") {
		return true
REDACTED
	if strings.HasPrefix(eventType, "response.output_text") {
		return true
REDACTED
	if strings.HasPrefix(eventType, "response.output") {
		return true
REDACTED
	// 终止事件（response.completed/done/failed/...）由 isOpenAIWSTerminalEvent 单独处理。
	// 不能把它们当作 token event，否则当上游没有可识别的 delta 时，
	// firstTokenMs 会被填到终止时刻，等于把"总耗时"误报为"首 token 延迟"。
	return false
REDACTED

func replaceOpenAIWSMessageModel(message []byte, fromModel, toModel string) []byte {
	if len(message) == 0 {
		return message
REDACTED
	if strings.TrimSpace(fromModel) == "" || strings.TrimSpace(toModel) == "" || fromModel == toModel {
		return message
REDACTED
	if !bytes.Contains(message, []byte(`"model"`)) || !bytes.Contains(message, []byte(fromModel)) {
		return message
REDACTED
	modelValues := gjson.GetManyBytes(message, "model", "response.model")
	replaceModel := modelValues[0].Exists() && modelValues[0].Str == fromModel
	replaceResponseModel := modelValues[1].Exists() && modelValues[1].Str == fromModel
	if !replaceModel && !replaceResponseModel {
		return message
REDACTED
	updated := message
	if replaceModel {
		if next, err := sjson.SetBytes(updated, "model", toModel); err == nil {
			updated = next
	REDACTED
REDACTED
	if replaceResponseModel {
		if next, err := sjson.SetBytes(updated, "response.model", toModel); err == nil {
			updated = next
	REDACTED
REDACTED
	return updated
REDACTED

func populateOpenAIUsageFromResponseJSON(body []byte, usage *OpenAIUsage) {
	if usage == nil || len(body) == 0 {
		return
REDACTED
	values := gjson.GetManyBytes(
		body,
		"usage.input_tokens",
		"usage.output_tokens",
		"usage.input_tokens_details.cached_tokens",
	)
	usage.InputTokens = int(values[0].Int())
	usage.OutputTokens = int(values[1].Int())
	usage.CacheReadInputTokens = int(values[2].Int())
REDACTED

func getOpenAIGroupIDFromContext(c *gin.Context) int64 {
	if c == nil {
		return 0
REDACTED
	value, exists := c.Get("api_key")
	if !exists {
		return 0
REDACTED
	apiKey, ok := value.(*APIKey)
	if !ok || apiKey == nil || apiKey.GroupID == nil {
		return 0
REDACTED
	return *apiKey.GroupID
REDACTED

// SelectAccountByPreviousResponseID 按 previous_response_id 命中账号粘连。
// 未命中或账号不可用时返回 (nil, nil)，由调用方继续走常规调度。
func (s *OpenAIGatewayService) SelectAccountByPreviousResponseID(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	requestedModel string,
	excludedIDs map[int64]struct{REDACTED,
	requireCompact bool,
) (*AccountSelectionResult, error) {
	return s.selectAccountByPreviousResponseIDForCapability(ctx, groupID, previousResponseID, requestedModel, excludedIDs, "", requireCompact)
REDACTED

func (s *OpenAIGatewayService) selectAccountByPreviousResponseIDForCapability(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	requestedModel string,
	excludedIDs map[int64]struct{REDACTED,
	requiredCapability OpenAIEndpointCapability,
	requireCompact bool,
) (*AccountSelectionResult, error) {
	if s == nil {
		return nil, nil
REDACTED
	accountID, account, responseID, store := s.resolveAccountByPreviousResponseIDForCapability(ctx, groupID, previousResponseID, requestedModel, excludedIDs, requiredCapability, requireCompact)
	if accountID <= 0 || account == nil || store == nil {
		return nil, nil
REDACTED

	result, acquireErr := s.tryAcquireAccountSlot(ctx, accountID, account.Concurrency)
	if acquireErr == nil && result.Acquired {
		logOpenAIWSBindResponseAccountWarn(
			derefGroupID(groupID),
			accountID,
			responseID,
			store.BindResponseAccount(ctx, derefGroupID(groupID), responseID, accountID, s.openAIWSResponseStickyTTL()),
		)
		return &AccountSelectionResult{
			Account:     account,
			Acquired:    true,
			ReleaseFunc: result.ReleaseFunc,
	REDACTED, nil
REDACTED

	cfg := s.schedulingConfig()
	if s.concurrencyService != nil {
		return &AccountSelectionResult{
			Account: account,
			WaitPlan: &AccountWaitPlan{
				AccountID:      accountID,
				MaxConcurrency: account.Concurrency,
				Timeout:        cfg.StickySessionWaitTimeout,
				MaxWaiting:     cfg.StickySessionMaxWaiting,
		REDACTED,
	REDACTED, nil
REDACTED
	return nil, nil
REDACTED

func (s *OpenAIGatewayService) ResolveAccountIDByPreviousResponseIDForScheduler(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	requestedModel string,
	excludedIDs map[int64]struct{REDACTED,
	requiredCapability OpenAIEndpointCapability,
	requireCompact bool,
) int64 {
	accountID, _, _, _ := s.resolveAccountByPreviousResponseIDForCapability(ctx, groupID, previousResponseID, requestedModel, excludedIDs, requiredCapability, requireCompact)
	return accountID
REDACTED

func (s *OpenAIGatewayService) resolveAccountByPreviousResponseIDForCapability(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	requestedModel string,
	excludedIDs map[int64]struct{REDACTED,
	requiredCapability OpenAIEndpointCapability,
	requireCompact bool,
) (int64, *Account, string, OpenAIWSStateStore) {
	if s == nil {
		return 0, nil, "", nil
REDACTED
	responseID := strings.TrimSpace(previousResponseID)
	if responseID == "" {
		return 0, nil, "", nil
REDACTED
	store := s.getOpenAIWSStateStore()
	if store == nil {
		return 0, nil, "", nil
REDACTED

	accountID, err := store.GetResponseAccount(ctx, derefGroupID(groupID), responseID)
	if err != nil || accountID <= 0 {
		return 0, nil, "", nil
REDACTED
	if excludedIDs != nil {
		if _, excluded := excludedIDs[accountID]; excluded {
			return 0, nil, "", nil
	REDACTED
REDACTED

	account, err := s.getSchedulableAccount(ctx, accountID)
	if err != nil || account == nil {
		_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
		return 0, nil, "", nil
REDACTED
	// 非 WSv2 场景（如 force_http/全局关闭）不应使用 previous_response_id 粘连，
	// 以保持“回滚到 HTTP”后的历史行为一致性。
	if s.getOpenAIWSProtocolResolver().Resolve(account).Transport != OpenAIUpstreamTransportResponsesWebsocketV2 {
		return 0, nil, "", nil
REDACTED
	if shouldClearStickySession(account, requestedModel) || !account.IsOpenAI() || !account.IsSchedulable() {
		_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
		return 0, nil, "", nil
REDACTED
	if !parentHealthyForShadow(account, s.parentAccountLookup(ctx)) {
		_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
		return 0, nil, "", nil
REDACTED
	if requestedModel != "" && !account.IsModelSupported(requestedModel) {
		return 0, nil, "", nil
REDACTED
	if !account.SupportsOpenAIEndpointCapability(requiredCapability) {
		return 0, nil, "", nil
REDACTED
	// Quota auto-pause must also gate the previous_response_id sticky path; otherwise an
	// account over its 5h/7d threshold keeps serving the same response chain even though
	// normal scheduling skips it. Pause is transient, so fall through to normal scheduling
	// without deleting the binding (the window may reset before the next turn).
	if paused, _ := shouldAutoPauseOpenAIAccountByQuota(ctx, account); paused {
		return 0, nil, "", nil
REDACTED
	if s.schedulerSnapshot != nil && s.accountRepo != nil {
		latest, latestErr := s.accountRepo.GetByID(ctx, account.ID)
		if latestErr != nil || latest == nil {
			_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
			return 0, nil, "", nil
	REDACTED
		if shouldClearStickySession(latest, requestedModel) || !latest.IsOpenAI() || !latest.IsSchedulable() {
			_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
			return 0, nil, "", nil
	REDACTED
		if !parentHealthyForShadow(latest, s.parentAccountLookup(ctx)) {
			_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
			return 0, nil, "", nil
	REDACTED
		if requestedModel != "" && !latest.IsModelSupported(requestedModel) {
			return 0, nil, "", nil
	REDACTED
		if !latest.SupportsOpenAIEndpointCapability(requiredCapability) {
			return 0, nil, "", nil
	REDACTED
		if paused, _ := shouldAutoPauseOpenAIAccountByQuota(ctx, latest); paused {
			return 0, nil, "", nil
	REDACTED
		if s.isOpenAIAccountRuntimeBlocked(latest) {
			_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
			return 0, nil, "", nil
	REDACTED
		account = latest
REDACTED
	if requireCompact && openAICompactSupportTier(account) == 0 {
		_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
		return 0, nil, "", nil
REDACTED
	return accountID, account, responseID, store
REDACTED

func classifyOpenAIWSAcquireError(err error) string {
	if err == nil {
		return "acquire_conn"
REDACTED
	var dialErr *openAIWSDialError
	if errors.As(err, &dialErr) {
		switch dialErr.StatusCode {
		case 426:
			return "upgrade_required"
		case 401, 403:
			return "auth_failed"
		case 429:
			return "upstream_rate_limited"
	REDACTED
		if dialErr.StatusCode >= 500 {
			return "upstream_5xx"
	REDACTED
		return "dial_failed"
REDACTED
	if errors.Is(err, errOpenAIWSConnQueueFull) {
		return "conn_queue_full"
REDACTED
	if errors.Is(err, errOpenAIWSPreferredConnUnavailable) {
		return "preferred_conn_unavailable"
REDACTED
	if errors.Is(err, context.DeadlineExceeded) {
		return "acquire_timeout"
REDACTED
	return "acquire_conn"
REDACTED

func isOpenAIWSRateLimitError(codeRaw, errTypeRaw, msgRaw string) bool {
	code := strings.ToLower(strings.TrimSpace(codeRaw))
	errType := strings.ToLower(strings.TrimSpace(errTypeRaw))
	msg := strings.ToLower(strings.TrimSpace(msgRaw))

	if strings.Contains(errType, "rate_limit") || strings.Contains(errType, "usage_limit") {
		return true
REDACTED
	if strings.Contains(code, "rate_limit") || strings.Contains(code, "usage_limit") || strings.Contains(code, "insufficient_quota") {
		return true
REDACTED
	if strings.Contains(msg, "usage limit") && strings.Contains(msg, "reached") {
		return true
REDACTED
	if strings.Contains(msg, "rate limit") && (strings.Contains(msg, "reached") || strings.Contains(msg, "exceeded")) {
		return true
REDACTED
	return false
REDACTED

func (s *OpenAIGatewayService) persistOpenAIWSRateLimitSignal(ctx context.Context, account *Account, headers http.Header, responseBody []byte, codeRaw, errTypeRaw, msgRaw string) {
	if s == nil || s.rateLimitService == nil || account == nil || account.Platform != PlatformOpenAI {
		return
REDACTED
	if !isOpenAIWSRateLimitError(codeRaw, errTypeRaw, msgRaw) {
		return
REDACTED
	s.handleOpenAIAccountUpstreamError(ctx, account, http.StatusTooManyRequests, headers, responseBody)
REDACTED

func classifyOpenAIWSErrorEventFromRaw(codeRaw, errTypeRaw, msgRaw string) (string, bool) {
	code := strings.ToLower(strings.TrimSpace(codeRaw))
	errType := strings.ToLower(strings.TrimSpace(errTypeRaw))
	msg := strings.ToLower(strings.TrimSpace(msgRaw))

	switch code {
	case "upgrade_required":
		return "upgrade_required", true
	case "websocket_not_supported", "websocket_unsupported":
		return "ws_unsupported", true
	case "websocket_connection_limit_reached":
		return "ws_connection_limit_reached", true
	case "invalid_encrypted_content":
		return "invalid_encrypted_content", true
	case "previous_response_not_found":
		return "previous_response_not_found", true
REDACTED
	if isOpenAIWSRateLimitError(codeRaw, errTypeRaw, msgRaw) {
		return "upstream_rate_limited", false
REDACTED
	if strings.Contains(msg, "upgrade required") || strings.Contains(msg, "status 426") {
		return "upgrade_required", true
REDACTED
	if strings.Contains(errType, "upgrade") {
		return "upgrade_required", true
REDACTED
	if strings.Contains(msg, "websocket") && strings.Contains(msg, "unsupported") {
		return "ws_unsupported", true
REDACTED
	if strings.Contains(msg, "connection limit") && strings.Contains(msg, "websocket") {
		return "ws_connection_limit_reached", true
REDACTED
	if strings.Contains(msg, "invalid_encrypted_content") ||
		(strings.Contains(msg, "encrypted content") && strings.Contains(msg, "could not be verified")) {
		return "invalid_encrypted_content", true
REDACTED
	if strings.Contains(msg, "previous_response_not_found") ||
		(strings.Contains(msg, "previous response") && strings.Contains(msg, "not found")) {
		return "previous_response_not_found", true
REDACTED
	if strings.Contains(errType, "server_error") || strings.Contains(code, "server_error") {
		return "upstream_error_event", true
REDACTED
	return "event_error", false
REDACTED

func classifyOpenAIWSErrorEvent(message []byte) (string, bool) {
	if len(message) == 0 {
		return "event_error", false
REDACTED
	return classifyOpenAIWSErrorEventFromRaw(parseOpenAIWSErrorEventFields(message))
REDACTED

func openAIWSErrorHTTPStatusFromRaw(codeRaw, errTypeRaw string) int {
	code := strings.ToLower(strings.TrimSpace(codeRaw))
	errType := strings.ToLower(strings.TrimSpace(errTypeRaw))
	switch {
	case strings.Contains(errType, "invalid_request"),
		strings.Contains(code, "invalid_request"),
		strings.Contains(code, "bad_request"),
		code == "invalid_encrypted_content",
		code == "previous_response_not_found":
		return http.StatusBadRequest
	case strings.Contains(errType, "authentication"),
		strings.Contains(code, "invalid_api_key"),
		strings.Contains(code, "unauthorized"):
		return http.StatusUnauthorized
	case strings.Contains(errType, "permission"),
		strings.Contains(code, "forbidden"):
		return http.StatusForbidden
	case isOpenAIWSRateLimitError(codeRaw, errTypeRaw, ""):
		return http.StatusTooManyRequests
	default:
		return http.StatusBadGateway
REDACTED
REDACTED

func openAIWSErrorHTTPStatus(message []byte) int {
	if len(message) == 0 {
		return http.StatusBadGateway
REDACTED
	codeRaw, errTypeRaw, _ := parseOpenAIWSErrorEventFields(message)
	return openAIWSErrorHTTPStatusFromRaw(codeRaw, errTypeRaw)
REDACTED

func (s *OpenAIGatewayService) openAIWSFallbackCooldown() time.Duration {
	if s == nil || s.cfg == nil {
		return 30 * time.Second
REDACTED
	seconds := s.cfg.Gateway.OpenAIWS.FallbackCooldownSeconds
	if seconds <= 0 {
		return 0
REDACTED
	return time.Duration(seconds) * time.Second
REDACTED

func (s *OpenAIGatewayService) isOpenAIWSFallbackCooling(accountID int64) bool {
	if s == nil || accountID <= 0 {
		return false
REDACTED
	cooldown := s.openAIWSFallbackCooldown()
	if cooldown <= 0 {
		return false
REDACTED
	rawUntil, ok := s.openaiWSFallbackUntil.Load(accountID)
	if !ok || rawUntil == nil {
		return false
REDACTED
	until, ok := rawUntil.(time.Time)
	if !ok || until.IsZero() {
		s.openaiWSFallbackUntil.Delete(accountID)
		return false
REDACTED
	if time.Now().Before(until) {
		return true
REDACTED
	s.openaiWSFallbackUntil.Delete(accountID)
	return false
REDACTED

func (s *OpenAIGatewayService) markOpenAIWSFallbackCooling(accountID int64, _ string) {
	if s == nil || accountID <= 0 {
		return
REDACTED
	cooldown := s.openAIWSFallbackCooldown()
	if cooldown <= 0 {
		return
REDACTED
	s.openaiWSFallbackUntil.Store(accountID, time.Now().Add(cooldown))
REDACTED

func (s *OpenAIGatewayService) clearOpenAIWSFallbackCooling(accountID int64) {
	if s == nil || accountID <= 0 {
		return
REDACTED
	s.openaiWSFallbackUntil.Delete(accountID)
REDACTED
