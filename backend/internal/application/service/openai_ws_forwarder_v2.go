package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/Wei-Shaw/sub2api/internal/shared/openaitiming"
	"github.com/Wei-Shaw/sub2api/internal/shared/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func (s *OpenAIGatewayService) forwardOpenAIWSV2(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	reqBody map[string]any,
	token string,
	decision OpenAIWSProtocolDecision,
	isCodexCLI bool,
	reqStream bool,
	originalModel string,
	mappedModel string,
	startTime time.Time,
	attempt int,
	lastFailureReason string,
	agentTaskRecoveryTried *bool,
	fingerprintIDs *codexFingerprintIDs,
	allowAuthRecovery bool,
) (*OpenAIForwardResult, error) {
	if s == nil || account == nil {
		return nil, wrapOpenAIWSFallback("invalid_state", errors.New("service or account is nil"))
	}
	visibleOutputTTFT := s.useOpenAIVisibleOutputTTFT(ctx)
	responseModelObserver := &upstreamResponseModelObserver{}
	timingCollector := &openaitiming.Collector{}

	wsURL, err := s.buildOpenAIResponsesWSURLWithContext(ctx, account, token)
	if err != nil {
		return nil, wrapOpenAIWSFallback("build_ws_url", err)
	}
	wsHost := "-"
	wsPath := "-"
	if parsed, parseErr := url.Parse(wsURL); parseErr == nil && parsed != nil {
		if h := strings.TrimSpace(parsed.Host); h != "" {
			wsHost = normalizeOpenAIWSLogValue(h)
		}
		if p := strings.TrimSpace(parsed.Path); p != "" {
			wsPath = normalizeOpenAIWSLogValue(p)
		}
	}
	logOpenAIWSModeDebug(
		"dial_target account_id=%d account_type=%s ws_host=%s ws_path=%s",
		account.ID,
		account.Type,
		wsHost,
		wsPath,
	)

	payload := s.buildOpenAIWSCreatePayload(reqBody, account)
	if account.Platform == PlatformOpenAI {
		sanitizeOpenAIResponsesAccessProgramsMap(payload)
	}
	if s.IsDistillationGroupRequest(c, account) {
		var cleaned map[string]any
		if err := json.Unmarshal(stripDistillationCacheFields(payloadAsJSONBytes(payload)), &cleaned); err == nil {
			payload = cleaned
		}
	}
	reasoningEffort := ""
	if effort := extractOpenAIReasoningEffort(reqBody, mappedModel, originalModel); effort != nil {
		reasoningEffort = strings.TrimSpace(*effort)
	}
	firstOutputTimeout := time.Duration(0)
	if account.Platform == PlatformOpenAI {
		firstOutputTimeout = s.openAIFirstOutputTimeoutWithContext(ctx, reasoningEffort)
	}
	firstOutputDeadline := startTime.Add(firstOutputTimeout)
	firstOutputComplete := false
	payloadStrategy, removedKeys := applyOpenAIWSRetryPayloadStrategy(payload, attempt)
	previousResponseID := openAIWSPayloadString(payload, "previous_response_id")
	previousResponseIDKind := ClassifyOpenAIPreviousResponseIDKind(previousResponseID)
	promptCacheKey := openAIWSPayloadString(payload, "prompt_cache_key")
	if account.IsOpenAIOAuth() && (fingerprintIDs == nil || fingerprintIDs.mode == codexFingerprintOff) {
		ids := resolveCodexOutboundSessionIDs(c, account, payloadAsJSONBytes(payload), promptCacheKey)
		sanitizeOpenAICodexClientMetadataPayload(payload, account, ids)
		if metadata, ok := payload["client_metadata"].(map[string]any); ok {
			stageCodexMetadataProjection(metadata, account, ids, nil, c)
		}
	}
	_, hasTools := payload["tools"]
	debugEnabled := isOpenAIWSModeDebugEnabled()
	var payloadJSON []byte
	resolvePayloadJSON := func() []byte {
		if payloadJSON == nil {
			payloadJSON = payloadAsJSONBytes(payload)
		}
		return payloadJSON
	}
	resolvePayloadBytes := func() int { return len(resolvePayloadJSON()) }
	streamValue := "-"
	if raw, ok := payload["stream"]; ok {
		streamValue = normalizeOpenAIWSLogValue(strings.TrimSpace(fmt.Sprintf("%v", raw)))
	}
	turnState := ""
	turnMetadata := ""
	if c != nil && c.Request != nil {
		turnState = strings.TrimSpace(c.GetHeader(openAIWSTurnStateHeader))
		turnMetadata = strings.TrimSpace(c.GetHeader(openAIWSTurnMetadataHeader))
	}
	turnMetadata = rewriteCodexTurnMetadataValue(turnMetadata, fingerprintIDs)
	setOpenAIWSTurnMetadata(payload, turnMetadata)
	payloadEventType := openAIWSPayloadString(payload, "type")
	if payloadEventType == "" {
		payloadEventType = "response.create"
	}
	if s.shouldEmitOpenAIWSPayloadSchema(attempt) {
		logOpenAIWSModeInfo(
			"[debug] payload_schema account_id=%d attempt=%d event=%s payload_keys=%s payload_bytes=%d payload_key_sizes=%s input_summary=%s stream=%s payload_strategy=%s removed_keys=%s has_previous_response_id=%v has_prompt_cache_key=%v has_tools=%v",
			account.ID,
			attempt,
			payloadEventType,
			normalizeOpenAIWSLogValue(strings.Join(sortedKeys(payload), ",")),
			resolvePayloadBytes(),
			normalizeOpenAIWSLogValue(summarizeOpenAIWSPayloadKeySizes(payload, openAIWSPayloadKeySizeTopN)),
			normalizeOpenAIWSLogValue(summarizeOpenAIWSInput(payload["input"])),
			streamValue,
			normalizeOpenAIWSLogValue(payloadStrategy),
			normalizeOpenAIWSLogValue(strings.Join(removedKeys, ",")),
			previousResponseID != "",
			promptCacheKey != "",
			hasTools,
		)
	}

	stateStore := s.getOpenAIWSStateStore()
	groupID := getOpenAIGroupIDFromContext(c)
	sessionHash := s.GenerateSessionHash(c, nil)
	if sessionHash == "" {
		var legacySessionHash string
		sessionHash, legacySessionHash = openAIWSSessionHashesFromID(promptCacheKey)
		attachOpenAILegacySessionHashToGin(c, legacySessionHash)
	}
	if turnState == "" && stateStore != nil && sessionHash != "" {
		savedTurnState, ok, stateErr := stateStore.GetSessionTurnState(ctx, groupID, sessionHash)
		logOpenAIWSSessionTurnStateWarn("get", groupID, sessionHash, stateErr)
		if stateErr == nil && ok {
			turnState = savedTurnState
		}
	}
	replayModel := openAIWSPayloadString(payload, "model")
	stageOpenAICodexTurnStateModel(c, replayModel)
	turnState = s.resolveCodexTurnStateReplay(c, account, replayModel, turnState)
	preferredConnID := ""
	if stateStore != nil && previousResponseID != "" {
		if connID, ok := stateStore.GetResponseConn(previousResponseID); ok {
			preferredConnID = connID
		}
	}
	forcePreferredConn := codexContinuationRequiresExactConnection(c)
	if forcePreferredConn {
		preferredConnID = s.codexContinuationPreferredConnection(c, stateStore, previousResponseID)
		if preferredConnID == "" {
			return nil, newCodexContinuationTerminalError(
				"Codex incremental continuation requires its original upstream WebSocket connection",
				nil,
			)
		}
	}
	storeDisabled := s.isOpenAIWSStoreDisabledInRequest(reqBody, account)
	if !forcePreferredConn && stateStore != nil && storeDisabled && previousResponseID == "" && sessionHash != "" {
		if connID, ok := stateStore.GetSessionConn(groupID, sessionHash); ok {
			preferredConnID = connID
		}
	}
	storeDisabledConnMode := s.openAIWSStoreDisabledConnMode()
	forceNewConnByPolicy := shouldForceNewConnOnStoreDisabled(storeDisabledConnMode, lastFailureReason)
	forceNewConn := forceNewConnByPolicy && storeDisabled && previousResponseID == "" && sessionHash != "" && preferredConnID == ""
	// All payload mutations above are complete. Share one immutable snapshot
	// throughout header projection instead of encoding the full input per reader.
	headerPayload := resolvePayloadJSON()
	stageCodexOutboundSessionBody(c, headerPayload)
	wsHeaders, sessionResolution, buildHdrErr := s.buildOpenAIWSHeaders(
		ctx,
		c,
		account,
		token,
		decision,
		isCodexCLI,
		turnState,
		turnMetadata,
		promptCacheKey,
		openAIWSPayloadString(payload, "model"),
		openAIWSPayloadString(payload, "service_tier"),
	)
	if buildHdrErr != nil {
		return nil, fmt.Errorf("build ws headers: %w", buildHdrErr)
	}
	if s.CodexSimulationRequestEnabled(c) {
		wsHeaders.Del(CodexProjectIDHeader)
	}
	stageCodexFingerprintIDs(c, fingerprintIDs)
	applyCodexOutboundSessionHeaders(c, account, headerPayload, promptCacheKey, wsHeaders, fingerprintIDs)
	applyCodexFingerprintWSHeaders(wsHeaders, fingerprintIDs)
	applyOpenAIResponsesLiteWebSocketHeader(wsHeaders, headerPayload)
	applyOpenAICodexSemanticRequestHeaders(wsHeaders, c, account, headerPayload)
	logOpenAIWSModeDebug(
		"acquire_start account_id=%d account_type=%s transport=%s preferred_conn_id=%s has_previous_response_id=%v session_hash=%s has_turn_state=%v turn_state_len=%d has_turn_metadata=%v turn_metadata_len=%d store_disabled=%v store_disabled_conn_mode=%s retry_last_reason=%s force_new_conn=%v header_user_agent=%s header_openai_beta=%s header_originator=%s header_accept_language=%s header_session_id=%s header_conversation_id=%s session_id_source=%s conversation_id_source=%s has_prompt_cache_key=%v has_chatgpt_account_id=%v has_authorization=%v has_session_id=%v has_conversation_id=%v proxy_enabled=%v",
		account.ID,
		account.Type,
		normalizeOpenAIWSLogValue(string(decision.Transport)),
		truncateOpenAIWSLogValue(preferredConnID, openAIWSIDValueMaxLen),
		previousResponseID != "",
		truncateOpenAIWSLogValue(sessionHash, 12),
		turnState != "",
		len(turnState),
		turnMetadata != "",
		len(turnMetadata),
		storeDisabled,
		normalizeOpenAIWSLogValue(storeDisabledConnMode),
		truncateOpenAIWSLogValue(lastFailureReason, openAIWSLogValueMaxLen),
		forceNewConn,
		openAIWSHeaderValueForLog(wsHeaders, "user-agent"),
		openAIWSHeaderValueForLog(wsHeaders, "openai-beta"),
		openAIWSHeaderValueForLog(wsHeaders, "originator"),
		openAIWSHeaderValueForLog(wsHeaders, "accept-language"),
		openAIWSHeaderValueForLog(wsHeaders, "session_id"),
		openAIWSHeaderValueForLog(wsHeaders, "conversation_id"),
		normalizeOpenAIWSLogValue(sessionResolution.SessionSource),
		normalizeOpenAIWSLogValue(sessionResolution.ConversationSource),
		promptCacheKey != "",
		hasOpenAIWSHeader(wsHeaders, "chatgpt-account-id"),
		hasOpenAIWSHeader(wsHeaders, "authorization"),
		hasOpenAIWSHeader(wsHeaders, "session_id"),
		hasOpenAIWSHeader(wsHeaders, "conversation_id"),
		account.ProxyID != nil && account.Proxy != nil,
	)

	acquireCtx, acquireCancel := context.WithTimeout(ctx, s.openAIWSAcquireTimeout())
	defer acquireCancel()

	lease, err := s.getOpenAIWSConnPool().Acquire(acquireCtx, openAIWSAcquireRequest{
		Account:      account,
		WSURL:        wsURL,
		Headers:      wsHeaders,
		TLSProfile:   s.resolveTLSProfile(account),
		ProofFactory: s.newOpenAIWSProofFactory(account, wsURL, fingerprintIDs, wsHeaders),
		HandshakeObserver: func(dialCtx context.Context, headers http.Header) {
			s.captureIntegrityState(dialCtx, headers)
		},
		PreferredConnID:    preferredConnID,
		ForceNewConn:       forceNewConn,
		ForcePreferredConn: forcePreferredConn,
		EgressRoute:        account.EgressRoute(),
		ProxyURL: func() string {
			if account.ProxyID != nil && account.Proxy != nil {
				return account.Proxy.URL()
			}
			return ""
		}(),
	})
	if err != nil {
		if forcePreferredConn {
			if ctx != nil && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, newCodexContinuationTerminalError(
				"Codex incremental continuation could not reacquire its original upstream WebSocket connection",
				err,
			)
		}
		var agentDialErr *openAIWSDialError
		if s.isAgentIdentityAccount(ctx, account) && errors.As(err, &agentDialErr) && isAgentIdentityTaskInvalidWSDialError(agentDialErr) && agentTaskRecoveryTried != nil && !*agentTaskRecoveryTried {
			*agentTaskRecoveryTried = true
			if recoveryErr := s.recoverAgentIdentityTask(ctx, account, account.GetCredential("task_id")); recoveryErr != nil {
				return nil, fmt.Errorf("agent identity task recovery failed: %w", recoveryErr)
			}
			return nil, &agentIdentityTaskRecoveredError{}
		}
		s.handleOpenAIWSDialTransientFailure(ctx, account, mappedModel, err)
		dialStatus, dialClass, dialCloseStatus, dialCloseReason, dialRespServer, dialRespVia, dialRespCFRay, dialRespReqID := summarizeOpenAIWSDialError(err)
		logOpenAIWSModeInfo(
			"acquire_fail account_id=%d account_type=%s transport=%s reason=%s dial_status=%d dial_class=%s dial_close_status=%s dial_close_reason=%s dial_resp_server=%s dial_resp_via=%s dial_resp_cf_ray=%s dial_resp_x_request_id=%s cause=%s preferred_conn_id=%s force_new_conn=%v ws_host=%s ws_path=%s proxy_enabled=%v",
			account.ID,
			account.Type,
			normalizeOpenAIWSLogValue(string(decision.Transport)),
			normalizeOpenAIWSLogValue(classifyOpenAIWSAcquireError(err)),
			dialStatus,
			dialClass,
			dialCloseStatus,
			truncateOpenAIWSLogValue(dialCloseReason, openAIWSHeaderValueMaxLen),
			dialRespServer,
			dialRespVia,
			dialRespCFRay,
			dialRespReqID,
			truncateOpenAIWSLogValue(err.Error(), openAIWSLogValueMaxLen),
			truncateOpenAIWSLogValue(preferredConnID, openAIWSIDValueMaxLen),
			forceNewConn,
			wsHost,
			wsPath,
			account.ProxyID != nil && account.Proxy != nil,
		)
		var dialErr *openAIWSDialError
		if errors.As(err, &dialErr) && dialErr != nil && dialErr.StatusCode == http.StatusTooManyRequests {
			s.persistOpenAIWSRateLimitSignal(ctx, account, dialErr.ResponseHeaders, nil, "rate_limit_exceeded", "rate_limit_error", strings.TrimSpace(err.Error()))
		}
		if account.IsCodexPrewarmContinuationEnabled() && dialErr != nil && shouldFailoverOpenAIWSPreOutputFailure(dialErr.StatusCode, dialErr.ResponseBody) {
			return nil, newOpenAIWSPreOutputFailoverError(
				dialErr.StatusCode,
				dialErr.ResponseHeaders,
				dialErr.ResponseBody,
				dialErr.Error(),
			)
		}
		return nil, wrapOpenAIWSFallback(classifyOpenAIWSAcquireError(err), err)
	}
	// cleanExit 标记正常终端事件退出，此时上游不会再发送帧，连接可安全归还复用。
	// 所有异常路径（读写错误、error 事件等）已在各自分支中提前调用 MarkBroken，
	// 因此 defer 中只需处理正常退出时不 MarkBroken 即可。
	cleanExit := false
	defer func() {
		if !cleanExit {
			lease.MarkBroken()
		}
		lease.Release()
	}()
	connID := strings.TrimSpace(lease.ConnID())
	logOpenAIWSModeDebug(
		"connected account_id=%d account_type=%s transport=%s conn_id=%s conn_reused=%v conn_pick_ms=%d queue_wait_ms=%d has_previous_response_id=%v",
		account.ID,
		account.Type,
		normalizeOpenAIWSLogValue(string(decision.Transport)),
		connID,
		lease.Reused(),
		lease.ConnPickDuration().Milliseconds(),
		lease.QueueWaitDuration().Milliseconds(),
		previousResponseID != "",
	)
	if previousResponseID != "" {
		logOpenAIWSModeInfo(
			"continuation_probe account_id=%d account_type=%s conn_id=%s previous_response_id=%s previous_response_id_kind=%s preferred_conn_id=%s conn_reused=%v store_disabled=%v session_hash=%s header_session_id=%s header_conversation_id=%s session_id_source=%s conversation_id_source=%s has_turn_state=%v turn_state_len=%d has_prompt_cache_key=%v",
			account.ID,
			account.Type,
			truncateOpenAIWSLogValue(connID, openAIWSIDValueMaxLen),
			truncateOpenAIWSLogValue(previousResponseID, openAIWSIDValueMaxLen),
			normalizeOpenAIWSLogValue(previousResponseIDKind),
			truncateOpenAIWSLogValue(preferredConnID, openAIWSIDValueMaxLen),
			lease.Reused(),
			storeDisabled,
			truncateOpenAIWSLogValue(sessionHash, 12),
			openAIWSHeaderValueForLog(wsHeaders, "session_id"),
			openAIWSHeaderValueForLog(wsHeaders, "conversation_id"),
			normalizeOpenAIWSLogValue(sessionResolution.SessionSource),
			normalizeOpenAIWSLogValue(sessionResolution.ConversationSource),
			turnState != "",
			len(turnState),
			promptCacheKey != "",
		)
	}
	if c != nil {
		SetOpsLatencyMs(c, OpsOpenAIWSConnPickMsKey, lease.ConnPickDuration().Milliseconds())
		SetOpsLatencyMs(c, OpsOpenAIWSQueueWaitMsKey, lease.QueueWaitDuration().Milliseconds())
		c.Set(OpsOpenAIWSConnReusedKey, lease.Reused())
		if connID != "" {
			c.Set(OpsOpenAIWSConnIDKey, connID)
		}
	}

	handshakeTurnState := strings.TrimSpace(lease.HandshakeHeader(openAIWSTurnStateHeader))
	logOpenAIWSModeDebug(
		"handshake account_id=%d conn_id=%s has_turn_state=%v turn_state_len=%d",
		account.ID,
		connID,
		handshakeTurnState != "",
		len(handshakeTurnState),
	)
	observedTurnState := handshakeTurnState
	if lease.Reused() {
		observedTurnState = ""
	}
	s.observeOpenAICodexTurnState(ctx, c, account, mappedModel, observedTurnState)
	if handshakeTurnState != "" {
		if stateStore != nil && sessionHash != "" {
			stateErr := stateStore.BindSessionTurnState(ctx, groupID, sessionHash, handshakeTurnState, s.openAIWSSessionStickyTTL())
			logOpenAIWSSessionTurnStateWarn("set", groupID, sessionHash, stateErr)
		}
		if c != nil {
			c.Header(http.CanonicalHeaderKey(openAIWSTurnStateHeader), handshakeTurnState)
		}
	}

	if err := s.performOpenAIWSGeneratePrewarm(
		ctx,
		lease,
		decision,
		payload,
		previousResponseID,
		reqBody,
		account,
		stateStore,
		groupID,
		&openAIWSPrewarmFirstOutputGuard{
			c:               c,
			startedAt:       startTime,
			timeout:         firstOutputTimeout,
			originalModel:   originalModel,
			reasoningEffort: reasoningEffort,
		},
	); err != nil {
		return nil, err
	}
	// Account-level prewarm continuation mutates the business payload after the
	// warmup response ID is known. Refresh derived log/size fields before write.
	previousResponseID = openAIWSPayloadString(payload, "previous_response_id")
	payloadJSON = nil

	if err := lease.WriteJSONWithContextTimeout(ctx, payload, s.openAIWSWriteTimeout()); err != nil {
		lease.MarkBroken()
		logOpenAIWSModeInfo(
			"write_request_fail account_id=%d conn_id=%s cause=%s payload_bytes=%d",
			account.ID,
			connID,
			truncateOpenAIWSLogValue(err.Error(), openAIWSLogValueMaxLen),
			resolvePayloadBytes(),
		)
		return nil, wrapOpenAIWSFallback("write_request", err)
	}
	if debugEnabled {
		logOpenAIWSModeDebug(
			"write_request_sent account_id=%d conn_id=%s stream=%v payload_bytes=%d previous_response_id=%s",
			account.ID,
			connID,
			reqStream,
			resolvePayloadBytes(),
			truncateOpenAIWSLogValue(previousResponseID, openAIWSIDValueMaxLen),
		)
	}

	usage := &OpenAIUsage{}
	imageCounter := newOpenAIImageOutputCounter()
	var firstTokenMs *int
	var localFirstEventTTFTMs *int
	streamOutputStarted := false
	responseID := ""
	var finalResponse []byte
	wroteDownstream := false
	needModelReplace := originalModel != mappedModel
	var mappedModelBytes []byte
	if needModelReplace && mappedModel != "" {
		mappedModelBytes = []byte(mappedModel)
	}
	bufferedStreamEvents := make([][]byte, 0, 4)
	bufferedStreamEventBytes := 0
	bufferedRateLimitsEventBytes := 0
	eventCount := 0
	tokenEventCount := 0
	terminalEventCount := 0
	bufferedEventCount := 0
	flushedBufferedEventCount := 0
	firstEventType := ""
	lastEventType := ""
	upstreamTerminalEvent := ""

	var flusher http.Flusher
	if reqStream {
		if s.responseHeaderFilter != nil {
			responseheaders.WriteFilteredHeaders(c.Writer.Header(), http.Header{}, s.responseHeaderFilter)
		}
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		f, ok := c.Writer.(http.Flusher)
		if !ok {
			lease.MarkBroken()
			return nil, wrapOpenAIWSFallback("streaming_not_supported", errors.New("streaming not supported"))
		}
		flusher = f
	}

	clientDisconnected := false
	flushBatchSize := s.openAIWSEventFlushBatchSize()
	flushInterval := s.openAIWSEventFlushInterval()
	pendingFlushEvents := 0
	lastFlushAt := time.Now()
	flushStreamWriter := func(force bool) {
		if clientDisconnected || flusher == nil || pendingFlushEvents <= 0 {
			return
		}
		if !force && flushBatchSize > 1 && pendingFlushEvents < flushBatchSize {
			if flushInterval <= 0 || time.Since(lastFlushAt) < flushInterval {
				return
			}
		}
		flusher.Flush()
		pendingFlushEvents = 0
		lastFlushAt = time.Now()
	}
	emitStreamMessage := func(message []byte, forceFlush bool) {
		if clientDisconnected {
			return
		}
		frame := make([]byte, 0, len(message)+8)
		frame = append(frame, "data: "...)
		frame = append(frame, message...)
		frame = append(frame, '\n', '\n')
		_, wErr := c.Writer.Write(frame)
		if wErr == nil {
			wroteDownstream = true
			pendingFlushEvents++
			flushStreamWriter(forceFlush)
			return
		}
		clientDisconnected = true
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI WS Mode] client disconnected, continue draining upstream: account=%d", account.ID)
	}
	flushBufferedStreamEvents := func(reason string, forceFlush bool) {
		if len(bufferedStreamEvents) == 0 {
			return
		}
		flushed := len(bufferedStreamEvents)
		for _, buffered := range bufferedStreamEvents {
			emitStreamMessage(buffered, false)
		}
		bufferedStreamEvents = bufferedStreamEvents[:0]
		bufferedStreamEventBytes = 0
		bufferedRateLimitsEventBytes = 0
		flushStreamWriter(forceFlush)
		flushedBufferedEventCount += flushed
		if debugEnabled {
			logOpenAIWSModeDebug(
				"buffer_flush account_id=%d conn_id=%s reason=%s flushed=%d total_flushed=%d client_disconnected=%v",
				account.ID,
				connID,
				truncateOpenAIWSLogValue(reason, openAIWSLogValueMaxLen),
				flushed,
				flushedBufferedEventCount,
				clientDisconnected,
			)
		}
	}

	readTimeout := s.openAIWSReadTimeout()
	var pendingJSONDocuments [][]byte
	responseTurnState := ""

	for {
		var message []byte
		var readErr error
		messageValid := false
		if len(pendingJSONDocuments) > 0 {
			message = pendingJSONDocuments[0]
			pendingJSONDocuments = pendingJSONDocuments[1:]
			// Queued documents were decoded and validated by the classifier.
			messageValid = true
		} else {
			readCtx := ctx
			var cancelRead context.CancelFunc
			if firstOutputTimeout > 0 && !firstOutputComplete {
				remaining := time.Until(firstOutputDeadline)
				if remaining <= 0 {
					readErr = context.DeadlineExceeded
				} else {
					readCtx, cancelRead = context.WithTimeout(ctx, remaining)
				}
			}
			if readErr == nil {
				message, readErr = lease.ReadMessageWithContextTimeout(readCtx, readTimeout)
			}
			if cancelRead != nil {
				cancelRead()
			}
			if readErr != nil && firstOutputTimeout > 0 && !firstOutputComplete && ctx.Err() == nil && !time.Now().Before(firstOutputDeadline) {
				lease.MarkBroken()
				return nil, s.newOpenAIFirstOutputTimeoutError(
					ctx, c, account, startTime, originalModel, reasoningEffort,
					firstOutputTimeout, "websocket_semantic_output", lease.HandshakeHeaders(),
				)
			}
			if readErr == nil {
				documents, repaired, valid := prepareOpenAIWSJSONDocuments(message)
				messageValid = valid
				if repaired {
					logOpenAIWSModeInfo(
						"concatenated_json_repaired account_id=%d conn_id=%s documents=%d bytes=%d",
						account.ID,
						truncateOpenAIWSLogValue(connID, openAIWSIDValueMaxLen),
						len(documents),
						len(message),
					)
					message = documents[0]
					pendingJSONDocuments = append(pendingJSONDocuments, documents[1:]...)
					messageValid = true
				}
			}
		}
		if readErr == nil && !messageValid {
			eventType, _, _ := parseOpenAIWSEventEnvelope(message)
			if eventType == "" {
				eventType = "unknown"
			}
			lease.MarkBroken()
			logOpenAIWSModeInfo(
				"invalid_event_json account_id=%d conn_id=%s event_type=%s bytes=%d wrote_downstream=%v",
				account.ID,
				truncateOpenAIWSLogValue(connID, openAIWSIDValueMaxLen),
				truncateOpenAIWSLogValue(eventType, openAIWSLogValueMaxLen),
				len(message),
				wroteDownstream,
			)
			if !wroteDownstream {
				return nil, wrapOpenAIWSFallback("invalid_event_json", errors.New("upstream websocket returned malformed Responses event JSON"))
			}
			return nil, errors.New("upstream websocket returned malformed Responses event JSON after downstream output")
		}
		if readErr != nil {
			lease.MarkBroken()
			closeStatus, closeReason := summarizeOpenAIWSReadCloseError(readErr)
			logOpenAIWSModeInfo(
				"read_fail account_id=%d conn_id=%s wrote_downstream=%v close_status=%s close_reason=%s cause=%s events=%d token_events=%d terminal_events=%d buffered_pending=%d buffered_flushed=%d first_event=%s last_event=%s",
				account.ID,
				connID,
				wroteDownstream,
				closeStatus,
				closeReason,
				truncateOpenAIWSLogValue(readErr.Error(), openAIWSLogValueMaxLen),
				eventCount,
				tokenEventCount,
				terminalEventCount,
				len(bufferedStreamEvents),
				flushedBufferedEventCount,
				truncateOpenAIWSLogValue(firstEventType, openAIWSLogValueMaxLen),
				truncateOpenAIWSLogValue(lastEventType, openAIWSLogValueMaxLen),
			)
			if !wroteDownstream {
				return nil, wrapOpenAIWSFallback(classifyOpenAIWSReadFallbackReason(readErr), readErr)
			}
			if clientDisconnected {
				break
			}
			setOpsUpstreamError(c, 0, sanitizeUpstreamErrorMessage(readErr.Error()), "")
			return nil, fmt.Errorf("openai ws read event: %w", readErr)
		}
		if normalized, changed := normalizeCompletedImageGenerationStatus(message); changed {
			message = normalized
		}

		eventType, eventResponseID, responseField := parseOpenAIWSEventEnvelope(message)
		if eventType == "" {
			continue
		}
		s.observeCodexEncryptedContentPayload(ctx, c, account, mappedModel, message, "ws")
		MarkOpenAIVerificationRecommendation(c, message, http.StatusOK)
		if eventType == "codex.response.metadata" {
			metadataHeaders := make(http.Header)
			responseTurnState = captureOpenAICodexTurnStateMetadata(metadataHeaders, message)
		}
		responseModelObserver.ObserveOpenAI(message, eventType)
		timingCollector.Observe(message, eventType)
		eventCount++
		if firstEventType == "" {
			firstEventType = eventType
		}
		lastEventType = eventType

		if responseID == "" && eventResponseID != "" {
			responseID = eventResponseID
		}

		isTokenEvent := isOpenAIWSTokenEvent(eventType)
		startsStreamOutput := isTokenEvent || isLegacyOpenAIWSTokenEvent(eventType)
		isTTFTEvent := isOpenAIWSTTFTEvent(eventType, visibleOutputTTFT)
		if isTokenEvent {
			tokenEventCount++
		}
		if startsStreamOutput {
			streamOutputStarted = true
		}
		isTerminalEvent := isOpenAIWSTerminalEvent(eventType)
		if isTerminalEvent {
			terminalEventCount++
		}
		if !firstOutputComplete && (isTerminalEvent || openAIStreamDataStartsClientOutput(string(message), eventType)) {
			firstOutputComplete = true
		}
		isLocalFirstEventTTFT := isOpenAILocalFirstEventType(eventType)
		if isTTFTEvent && (firstTokenMs == nil || isLocalFirstEventTTFT) {
			ms := int(time.Since(startTime).Milliseconds())
			if isLocalFirstEventTTFT {
				if localFirstEventTTFTMs == nil {
					localFirstEventTTFTMs = &ms
				}
			} else if firstTokenMs == nil {
				firstTokenMs = &ms
			}
		}
		if debugEnabled && shouldLogOpenAIWSEvent(eventCount, eventType) {
			logOpenAIWSModeDebug(
				"event_received account_id=%d conn_id=%s idx=%d type=%s bytes=%d token=%v terminal=%v buffered_pending=%d",
				account.ID,
				connID,
				eventCount,
				truncateOpenAIWSLogValue(eventType, openAIWSLogValueMaxLen),
				len(message),
				isTokenEvent,
				isTerminalEvent,
				len(bufferedStreamEvents),
			)
		}

		if !clientDisconnected {
			if needModelReplace && len(mappedModelBytes) > 0 && openAIWSEventMayContainModel(eventType) && bytes.Contains(message, mappedModelBytes) {
				message = replaceOpenAIWSMessageModel(message, mappedModel, originalModel)
			}
			if openAIWSEventMayContainToolCalls(eventType) && openAIWSMessageLikelyContainsToolCalls(message) {
				if corrected, changed := s.toolCorrector.CorrectToolCallsInSSEBytes(message); changed {
					message = corrected
				}
			}
		}
		if openAIWSEventShouldParseUsage(eventType) {
			parseOpenAIWSResponseUsageFromCompletedEvent(message, usage)
		}
		imageCounter.AddSSEData(message)

		if eventType == "error" || eventType == "response.failed" {
			markOpenAICyberPolicyEvent(c, message, http.StatusOK, usage)
		}

		if eventType == "error" {
			errCodeRaw, errTypeRaw, errMsgRaw := parseOpenAIWSErrorEventFields(message)
			if allowAuthRecovery && !wroteDownstream && s.openAITokenProvider != nil &&
				isOpenAIWSTokenAuthRecoveryCandidate(message, account) {
				if refreshErr := s.openAITokenProvider.RefreshAfterUnauthorized(ctx, account); refreshErr == nil {
					lease.MarkBroken()
					return nil, &openAIWSAuthRecoveredError{}
				}
			}
			errMsg := strings.TrimSpace(errMsgRaw)
			if errMsg == "" {
				errMsg = "Upstream websocket error"
			}
			fallbackReason, canFallback := classifyOpenAIWSErrorEventFromRaw(errCodeRaw, errTypeRaw, errMsgRaw)
			statusCode := openAIWSErrorHTTPStatusFromRaw(errCodeRaw, errTypeRaw)
			if account.IsCodexPrewarmContinuationEnabled() && !wroteDownstream && fallbackReason != "upgrade_required" && fallbackReason != "ws_unsupported" &&
				fallbackReason != "ws_connection_limit_reached" && fallbackReason != "previous_response_not_found" &&
				fallbackReason != "invalid_encrypted_content" {
				if failoverErr := s.handleOpenAIWSPreOutputFailure(
					ctx,
					account,
					mappedModel,
					statusCode,
					lease.HandshakeHeaders(),
					message,
					errMsg,
				); failoverErr != nil {
					lease.MarkBroken()
					return nil, failoverErr
				}
			}
			s.handleOpenAIWSErrorEventTransientFailure(ctx, account, mappedModel, lease.HandshakeHeaders(), message)
			s.persistOpenAIWSRateLimitSignal(ctx, account, lease.HandshakeHeaders(), message, errCodeRaw, errTypeRaw, errMsgRaw)
			errCode, errType, errMessage := summarizeOpenAIWSErrorEventFieldsFromRaw(errCodeRaw, errTypeRaw, errMsgRaw)
			logOpenAIWSModeInfo(
				"error_event account_id=%d conn_id=%s idx=%d fallback_reason=%s can_fallback=%v err_code=%s err_type=%s err_message=%s",
				account.ID,
				connID,
				eventCount,
				truncateOpenAIWSLogValue(fallbackReason, openAIWSLogValueMaxLen),
				canFallback,
				errCode,
				errType,
				errMessage,
			)
			if fallbackReason == "previous_response_not_found" {
				logOpenAIWSModeInfo(
					"previous_response_not_found_diag account_id=%d account_type=%s conn_id=%s previous_response_id=%s previous_response_id_kind=%s response_id=%s event_idx=%d req_stream=%v store_disabled=%v conn_reused=%v session_hash=%s header_session_id=%s header_conversation_id=%s session_id_source=%s conversation_id_source=%s has_turn_state=%v turn_state_len=%d has_prompt_cache_key=%v err_code=%s err_type=%s err_message=%s",
					account.ID,
					account.Type,
					connID,
					truncateOpenAIWSLogValue(previousResponseID, openAIWSIDValueMaxLen),
					normalizeOpenAIWSLogValue(previousResponseIDKind),
					truncateOpenAIWSLogValue(responseID, openAIWSIDValueMaxLen),
					eventCount,
					reqStream,
					storeDisabled,
					lease.Reused(),
					truncateOpenAIWSLogValue(sessionHash, 12),
					openAIWSHeaderValueForLog(wsHeaders, "session_id"),
					openAIWSHeaderValueForLog(wsHeaders, "conversation_id"),
					normalizeOpenAIWSLogValue(sessionResolution.SessionSource),
					normalizeOpenAIWSLogValue(sessionResolution.ConversationSource),
					turnState != "",
					len(turnState),
					promptCacheKey != "",
					errCode,
					errType,
					errMessage,
				)
			}
			// error 事件后连接不再可复用，避免回池后污染下一请求。
			lease.MarkBroken()
			if !wroteDownstream && (isOpenAIWSOverloadError(errCodeRaw, errTypeRaw, errMsgRaw) || isOpenAIWSOverloadPayload(message)) {
				return nil, newOpenAIWSOverloadFailoverError(
					lease.HandshakeHeaders(), message, errMsg,
				)
			}
			if account.BypassesLocalOpenAI429SchedulingBlocks() && !wroteDownstream &&
				isOpenAIWSRateLimitError(errCodeRaw, errTypeRaw, errMsgRaw) {
				return nil, &UpstreamFailoverError{
					StatusCode:      http.StatusTooManyRequests,
					ResponseBody:    append([]byte(nil), message...),
					ResponseHeaders: cloneHeader(lease.HandshakeHeaders()),
				}
			}
			if !wroteDownstream && canFallback {
				return nil, wrapOpenAIWSFallback(fallbackReason, errors.New(errMsg))
			}
			statusCode = openAIWSErrorHTTPStatusFromRaw(errCodeRaw, errTypeRaw)
			if isOpenAIWSOverloadError(errCodeRaw, errTypeRaw, errMsgRaw) {
				statusCode = http.StatusServiceUnavailable
			}
			setOpsUpstreamError(c, statusCode, errMsg, "")
			if reqStream && !clientDisconnected {
				flushBufferedStreamEvents("error_event", false)
				emitStreamMessage(message, true)
			}
			if !reqStream {
				c.JSON(statusCode, gin.H{
					"error": gin.H{
						"type":    "upstream_error",
						"message": errMsg,
					},
				})
			}
			return nil, fmt.Errorf("openai ws error event: %s", errMsg)
		}

		if eventType == "response.failed" && account.IsCodexPrewarmContinuationEnabled() && !wroteDownstream {
			errCodeRaw, errTypeRaw, _ := parseOpenAIWSErrorEventFields(message)
			statusCode := openAIWSPayloadStatusCode(message)
			if statusCode == 0 {
				statusCode = openAIWSErrorHTTPStatusFromRaw(errCodeRaw, errTypeRaw)
			}
			if failoverErr := s.handleOpenAIWSPreOutputFailure(
				ctx,
				account,
				mappedModel,
				statusCode,
				lease.HandshakeHeaders(),
				message,
				extractUpstreamErrorMessage(message),
			); failoverErr != nil {
				lease.MarkBroken()
				return nil, failoverErr
			}
		}

		if eventType == "response.failed" && !wroteDownstream && IsOpenAIGenericUpstreamFailureBody(message) {
			s.handleOpenAIWSTerminalTransientFailure(ctx, account, mappedModel, lease.HandshakeHeaders(), message)
			lease.MarkBroken()
			return nil, newOpenAIUpstreamFailoverError(
				http.StatusBadGateway,
				lease.HandshakeHeaders(),
				append([]byte(nil), message...),
				openAIGenericUpstreamFailureMessage,
				false,
			)
		}

		if eventType == "response.failed" && !wroteDownstream && isOpenAIWSOverloadPayload(message) {
			s.handleOpenAIWSTerminalTransientFailure(ctx, account, mappedModel, lease.HandshakeHeaders(), message)
			lease.MarkBroken()
			return nil, newOpenAIWSOverloadFailoverError(
				lease.HandshakeHeaders(), message,
				strings.TrimSpace(gjson.GetBytes(message, "response.error.message").String()),
			)
		}

		if eventType == "response.failed" && account.BypassesLocalOpenAI429SchedulingBlocks() &&
			!wroteDownstream && openAIWSPayloadStatusCode(message) == http.StatusTooManyRequests {
			s.handleOpenAIWSTerminalTransientFailure(ctx, account, mappedModel, lease.HandshakeHeaders(), message)
			lease.MarkBroken()
			return nil, &UpstreamFailoverError{
				StatusCode:      http.StatusTooManyRequests,
				ResponseBody:    append([]byte(nil), message...),
				ResponseHeaders: cloneHeader(lease.HandshakeHeaders()),
			}
		}

		if reqStream {
			// 在首个客户端语义输出前先缓冲事件（如 response.created），
			// 以便上游早期断连时仍可安全回退到 HTTP，不给下游发送半截流。
			shouldBuffer := !streamOutputStarted && !startsStreamOutput && !isTerminalEvent
			if shouldBuffer {
				isRateLimitsPreamble := account.BypassesLocalOpenAI429SchedulingBlocks() &&
					isOpenAIWSRateLimitsPreamble(eventType)
				if isRateLimitsPreamble && bufferedRateLimitsEventBytes+len(message) > openAIStreamPreOutputBufferLimit {
					if debugEnabled {
						logOpenAIWSModeDebug(
							"buffer_rate_limits_drop account_id=%d conn_id=%s idx=%d bytes=%d buffered_rate_limits_bytes=%d",
							account.ID,
							connID,
							eventCount,
							len(message),
							bufferedRateLimitsEventBytes,
						)
					}
					continue
				}
				buffered := make([]byte, len(message))
				copy(buffered, message)
				bufferedStreamEvents = append(bufferedStreamEvents, buffered)
				bufferedStreamEventBytes += len(buffered)
				if isRateLimitsPreamble {
					bufferedRateLimitsEventBytes += len(buffered)
				}
				bufferedEventCount++
				if debugEnabled && shouldLogOpenAIWSBufferedEvent(bufferedEventCount) {
					logOpenAIWSModeDebug(
						"buffer_enqueue account_id=%d conn_id=%s idx=%d event_idx=%d event_type=%s buffer_size=%d",
						account.ID,
						connID,
						bufferedEventCount,
						eventCount,
						truncateOpenAIWSLogValue(eventType, openAIWSLogValueMaxLen),
						len(bufferedStreamEvents),
					)
				}
				if bufferedStreamEventBytes-bufferedRateLimitsEventBytes >= openAIStreamPreOutputBufferLimit {
					// The preamble is about to become client-visible. It is no longer
					// safe to replay this attempt on another account.
					firstOutputComplete = true
					flushBufferedStreamEvents("buffer_limit", true)
				}
			} else {
				// Release the buffered preamble and the event that crossed the
				// semantic-output boundary in one downstream flush. Otherwise a
				// configured event batch can hold the boundary event until later.
				flushBufferedStreamEvents(eventType, false)
				emitStreamMessage(message, startsStreamOutput || isTerminalEvent)
			}
		} else {
			if responseField.Exists() && responseField.Type == gjson.JSON {
				finalResponse = []byte(responseField.Raw)
			}
		}

		if isTerminalEvent {
			upstreamTerminalEvent = s.handleOpenAIWSTerminalTransientFailure(ctx, account, mappedModel, lease.HandshakeHeaders(), message)
			// A terminal event must be the final JSON document in its WS message.
			// Ignore any tail for the completed client turn, but never reuse the
			// ambiguous upstream connection for another request.
			cleanExit = len(pendingJSONDocuments) == 0
			break
		}
	}

	if !reqStream {
		if len(finalResponse) == 0 {
			logOpenAIWSModeInfo(
				"missing_final_response account_id=%d conn_id=%s events=%d token_events=%d terminal_events=%d wrote_downstream=%v",
				account.ID,
				connID,
				eventCount,
				tokenEventCount,
				terminalEventCount,
				wroteDownstream,
			)
			if !wroteDownstream {
				return nil, wrapOpenAIWSFallback("missing_final_response", errors.New("no terminal response payload"))
			}
			return nil, errors.New("ws finished without final response")
		}

		if needModelReplace {
			finalResponse = s.replaceModelInResponseBody(finalResponse, mappedModel, originalModel)
		}
		finalResponse = s.correctToolCallsInResponseBody(finalResponse)
		populateOpenAIUsageFromResponseJSON(finalResponse, usage)
		if responseID == "" {
			responseID = strings.TrimSpace(gjson.GetBytes(finalResponse, "id").String())
		}

		c.Data(http.StatusOK, "application/json", finalResponse)
	} else {
		flushStreamWriter(true)
	}

	if responseID != "" && stateStore != nil {
		ttl := s.openAIWSResponseStickyTTL()
		logOpenAIWSBindResponseAccountWarn(groupID, account.ID, responseID, stateStore.BindResponseAccount(ctx, groupID, responseID, account.ID, ttl))
		stateStore.BindResponseConn(responseID, lease.ConnID(), ttl)
	}
	if stateStore != nil && storeDisabled && sessionHash != "" {
		stateStore.BindSessionConn(groupID, sessionHash, lease.ConnID(), s.openAIWSSessionStickyTTL())
	}
	if localFirstEventTTFTMs != nil &&
		(upstreamTerminalEvent == "response.completed" || upstreamTerminalEvent == "response.done") {
		firstTokenMs = localFirstEventTTFTMs
	}
	if upstreamTerminalEvent == "response.completed" || upstreamTerminalEvent == "response.done" {
		s.observeOpenAICodexTurnState(ctx, c, account, mappedModel, responseTurnState)
	}
	firstTokenMsValue := -1
	if firstTokenMs != nil {
		firstTokenMsValue = *firstTokenMs
	}
	logOpenAIWSModeDebug(
		"completed account_id=%d conn_id=%s response_id=%s stream=%v duration_ms=%d events=%d token_events=%d terminal_events=%d buffered_events=%d buffered_flushed=%d first_event=%s last_event=%s first_token_ms=%d wrote_downstream=%v client_disconnected=%v",
		account.ID,
		connID,
		truncateOpenAIWSLogValue(strings.TrimSpace(responseID), openAIWSIDValueMaxLen),
		reqStream,
		time.Since(startTime).Milliseconds(),
		eventCount,
		tokenEventCount,
		terminalEventCount,
		bufferedEventCount,
		flushedBufferedEventCount,
		truncateOpenAIWSLogValue(firstEventType, openAIWSLogValueMaxLen),
		truncateOpenAIWSLogValue(lastEventType, openAIWSLogValueMaxLen),
		firstTokenMsValue,
		wroteDownstream,
		clientDisconnected,
	)

	result := &OpenAIForwardResult{
		RequestID:                     responseID,
		Usage:                         *usage,
		Model:                         originalModel,
		UpstreamModel:                 mappedModel,
		UpstreamResponseModel:         responseModelObserver.Model(),
		OpenAITiming:                  timingCollector.Snapshot(),
		UpstreamResponseModelConflict: responseModelObserver.Conflict(),
		ImageCount:                    imageCounter.Count(),
		ImageOutputSizes:              imageCounter.Sizes(),
		ServiceTier:                   extractOpenAIServiceTier(reqBody),
		ReasoningEffort:               extractOpenAIReasoningEffort(reqBody, mappedModel, originalModel),
		Stream:                        reqStream,
		OpenAIWSMode:                  true,
		UpstreamTerminalEvent:         upstreamTerminalEvent,
		ResponseHeaders:               lease.HandshakeHeaders(),
		Duration:                      time.Since(startTime),
		FirstTokenMs:                  firstTokenMs,
	}
	if result.SucceededForScheduling() {
		s.completeCodexSimulationSuccess(ctx, c, account, responseID, lease.ConnID())
	}
	return result, nil
}

// ProxyResponsesWebSocketFromClient 处理客户端入站 WebSocket（OpenAI Responses WS Mode）并转发到上游。
// 当前实现按“单请求 -> 终止事件 -> 下一请求”的顺序代理，适配 Codex CLI 的 turn 模式。
// stripCodexSparkImageGenerationToolFromRawPayload removes the image_generation
// tool from a raw /responses payload when the upstream model is gpt-5.3-codex-spark.
// Spark rejects that tool upstream with HTTP 400 (invalid_request_error, param=tools);
// Codex clients advertise it by default. Returns the (possibly unchanged) payload,
// whether it changed, and any JSON decode error.
func stripCodexSparkImageGenerationToolFromRawPayload(payload []byte, model string) ([]byte, bool, error) {
	if !isCodexSparkModel(model) {
		return payload, false, nil
	}
	return stripOpenAIImageGenerationToolsFromRawPayload(payload)
}
