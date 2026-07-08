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

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func (s *OpenAIGatewayService) buildOpenAIResponsesWSURL(account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
REDACTED
	var targetURL string
	switch account.Type {
	case AccountTypeOAuth:
		targetURL = chatgptCodexURL
	case AccountTypeAPIKey:
		baseURL := account.GetOpenAIBaseURL()
		if baseURL == "" {
			targetURL = openaiPlatformAPIURL
	REDACTED else {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return "", err
		REDACTED
			targetURL = buildOpenAIResponsesURL(validatedURL)
	REDACTED
	default:
		targetURL = openaiPlatformAPIURL
REDACTED

	parsed, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil {
		return "", fmt.Errorf("invalid target url: %w", err)
REDACTED
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	case "wss", "ws":
		// 保持不变
	default:
		return "", fmt.Errorf("unsupported scheme for ws: %s", parsed.Scheme)
REDACTED
	return parsed.String(), nil
REDACTED

func (s *OpenAIGatewayService) buildOpenAIWSHeaders(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	decision OpenAIWSProtocolDecision,
	isCodexCLI bool,
	turnState string,
	turnMetadata string,
	promptCacheKey string,
) (http.Header, openAIWSSessionHeaderResolution, error) {
	headers := make(http.Header)
	headers.Set("authorization", "Bearer "+token)

	sessionResolution := resolveOpenAIWSSessionHeaders(c, promptCacheKey)
	if c != nil && c.Request != nil {
		if v := strings.TrimSpace(c.Request.Header.Get("accept-language")); v != "" {
			headers.Set("accept-language", v)
	REDACTED
REDACTED
	// OAuth 账号：将 apiKeyID 混入 session 标识符，防止跨用户会话碰撞。
	if account != nil && account.Type == AccountTypeOAuth {
		apiKeyID := getAPIKeyIDFromContext(c)
		if sessionResolution.SessionID != "" {
			headers.Set("session_id", isolateOpenAISessionID(apiKeyID, sessionResolution.SessionID))
	REDACTED
		if sessionResolution.ConversationID != "" {
			headers.Set("conversation_id", isolateOpenAISessionID(apiKeyID, sessionResolution.ConversationID))
	REDACTED
REDACTED else {
		if sessionResolution.SessionID != "" {
			headers.Set("session_id", sessionResolution.SessionID)
	REDACTED
		if sessionResolution.ConversationID != "" {
			headers.Set("conversation_id", sessionResolution.ConversationID)
	REDACTED
REDACTED
	if state := strings.TrimSpace(turnState); state != "" {
		headers.Set(openAIWSTurnStateHeader, state)
REDACTED
	if metadata := strings.TrimSpace(turnMetadata); metadata != "" {
		headers.Set(openAIWSTurnMetadataHeader, metadata)
REDACTED

	if account != nil && account.Type == AccountTypeOAuth {
		if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, headers, account); err != nil {
			return nil, sessionResolution, fmt.Errorf("resolve chatgpt account headers: %w", err)
	REDACTED
		headers.Set("originator", resolveOpenAIUpstreamOriginator(c, isCodexCLI))
REDACTED

	betaValue := openAIWSBetaV2Value
	if decision.Transport == OpenAIUpstreamTransportResponsesWebsocket {
		betaValue = openAIWSBetaV1Value
REDACTED
	headers.Set("OpenAI-Beta", betaValue)

	customUA := ""
	if account != nil {
		customUA = account.GetOpenAIUserAgent()
REDACTED
	if strings.TrimSpace(customUA) != "" {
		headers.Set("user-agent", customUA)
REDACTED else if c != nil {
		if ua := strings.TrimSpace(c.GetHeader("User-Agent")); ua != "" {
			headers.Set("user-agent", ua)
	REDACTED
REDACTED
	if s != nil && s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		headers.Set("user-agent", codexCLIUserAgent)
REDACTED
	if account != nil && account.Type == AccountTypeOAuth && !openai.IsCodexCLIRequest(headers.Get("user-agent")) {
		headers.Set("user-agent", codexCLIUserAgent)
REDACTED

	// 账号级请求头覆写（仅 openai api_key 账号启用时生效；OAuth 路径 no-op）。
	// 覆盖所有 WS 模式（ctx_pool/dedicated/passthrough）的握手头。
	account.ApplyHeaderOverrides(headers)

	return headers, sessionResolution, nil
REDACTED

func (s *OpenAIGatewayService) buildOpenAIWSCreatePayload(reqBody map[string]any, account *Account) map[string]any {
	// OpenAI WS Mode 协议：response.create 字段与 HTTP /responses 基本一致。
	// 保留 stream 字段（与 Codex CLI 一致），仅移除 background。
	payload := make(map[string]any, len(reqBody)+1)
	for k, v := range reqBody {
		payload[k] = v
REDACTED

	delete(payload, "background")
	if _, exists := payload["stream"]; !exists {
		payload["stream"] = true
REDACTED
	payload["type"] = "response.create"

	// OAuth 默认保持 store=false，避免误依赖服务端历史。
	if account != nil && account.Type == AccountTypeOAuth && !s.isOpenAIWSStoreRecoveryAllowed(account) {
		payload["store"] = false
REDACTED
	return payload
REDACTED

func setOpenAIWSTurnMetadata(payload map[string]any, turnMetadata string) {
	if len(payload) == 0 {
		return
REDACTED
	metadata := strings.TrimSpace(turnMetadata)
	if metadata == "" {
		return
REDACTED

	switch existing := payload["client_metadata"].(type) {
	case map[string]any:
		existing[openAIWSTurnMetadataHeader] = metadata
		payload["client_metadata"] = existing
	case map[string]string:
		next := make(map[string]any, len(existing)+1)
		for k, v := range existing {
			next[k] = v
	REDACTED
		next[openAIWSTurnMetadataHeader] = metadata
		payload["client_metadata"] = next
	default:
		payload["client_metadata"] = map[string]any{
			openAIWSTurnMetadataHeader: metadata,
	REDACTED
REDACTED
REDACTED

func (s *OpenAIGatewayService) isOpenAIWSStoreRecoveryAllowed(account *Account) bool {
	if account != nil && account.IsOpenAIWSAllowStoreRecoveryEnabled() {
		return true
REDACTED
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.AllowStoreRecovery {
		return true
REDACTED
	return false
REDACTED

func (s *OpenAIGatewayService) isOpenAIWSStoreDisabledInRequest(reqBody map[string]any, account *Account) bool {
	if account != nil && account.Type == AccountTypeOAuth && !s.isOpenAIWSStoreRecoveryAllowed(account) {
		return true
REDACTED
	if len(reqBody) == 0 {
		return false
REDACTED
	rawStore, ok := reqBody["store"]
	if !ok {
		return false
REDACTED
	storeEnabled, ok := rawStore.(bool)
	if !ok {
		return false
REDACTED
	return !storeEnabled
REDACTED

func (s *OpenAIGatewayService) isOpenAIWSStoreDisabledInRequestRaw(reqBody []byte, account *Account) bool {
	if account != nil && account.Type == AccountTypeOAuth && !s.isOpenAIWSStoreRecoveryAllowed(account) {
		return true
REDACTED
	if len(reqBody) == 0 {
		return false
REDACTED
	storeValue := gjson.GetBytes(reqBody, "store")
	if !storeValue.Exists() {
		return false
REDACTED
	if storeValue.Type != gjson.True && storeValue.Type != gjson.False {
		return false
REDACTED
	return !storeValue.Bool()
REDACTED

func (s *OpenAIGatewayService) openAIWSStoreDisabledConnMode() string {
	if s == nil || s.cfg == nil {
		return openAIWSStoreDisabledConnModeStrict
REDACTED
	mode := strings.ToLower(strings.TrimSpace(s.cfg.Gateway.OpenAIWS.StoreDisabledConnMode))
	switch mode {
	case openAIWSStoreDisabledConnModeStrict, openAIWSStoreDisabledConnModeAdaptive, openAIWSStoreDisabledConnModeOff:
		return mode
	case "":
		// 兼容旧配置：仅配置了布尔开关时按旧语义推导。
		if s.cfg.Gateway.OpenAIWS.StoreDisabledForceNewConn {
			return openAIWSStoreDisabledConnModeStrict
	REDACTED
		return openAIWSStoreDisabledConnModeOff
	default:
		return openAIWSStoreDisabledConnModeStrict
REDACTED
REDACTED

func shouldForceNewConnOnStoreDisabled(mode, lastFailureReason string) bool {
	switch mode {
	case openAIWSStoreDisabledConnModeOff:
		return false
	case openAIWSStoreDisabledConnModeAdaptive:
		reason := strings.TrimPrefix(strings.TrimSpace(lastFailureReason), "prewarm_")
		switch reason {
		case "policy_violation", "message_too_big", "auth_failed", "write_request", "write":
			return true
		default:
			return false
	REDACTED
	default:
		return true
REDACTED
REDACTED

func dropPreviousResponseIDFromRawPayload(payload []byte) ([]byte, bool, error) {
	return dropPreviousResponseIDFromRawPayloadWithDeleteFn(payload, sjson.DeleteBytes)
REDACTED

func dropPreviousResponseIDFromRawPayloadWithDeleteFn(
	payload []byte,
	deleteFn func([]byte, string) ([]byte, error),
) ([]byte, bool, error) {
	if len(payload) == 0 {
		return payload, false, nil
REDACTED
	if !gjson.GetBytes(payload, "previous_response_id").Exists() {
		return payload, false, nil
REDACTED
	if deleteFn == nil {
		deleteFn = sjson.DeleteBytes
REDACTED

	updated := payload
	for i := 0; i < openAIWSMaxPrevResponseIDDeletePasses &&
		gjson.GetBytes(updated, "previous_response_id").Exists(); i++ {
		next, err := deleteFn(updated, "previous_response_id")
		if err != nil {
			return payload, false, err
	REDACTED
		updated = next
REDACTED
	return updated, !gjson.GetBytes(updated, "previous_response_id").Exists(), nil
REDACTED

func setPreviousResponseIDToRawPayload(payload []byte, previousResponseID string) ([]byte, error) {
	normalizedPrevID := strings.TrimSpace(previousResponseID)
	if len(payload) == 0 || normalizedPrevID == "" {
		return payload, nil
REDACTED
	updated, err := sjson.SetBytes(payload, "previous_response_id", normalizedPrevID)
	if err == nil {
		return updated, nil
REDACTED

	var reqBody map[string]any
	if unmarshalErr := json.Unmarshal(payload, &reqBody); unmarshalErr != nil {
		return nil, err
REDACTED
	reqBody["previous_response_id"] = normalizedPrevID
	rebuilt, marshalErr := json.Marshal(reqBody)
	if marshalErr != nil {
		return nil, marshalErr
REDACTED
	return rebuilt, nil
REDACTED

func shouldInferIngressFunctionCallOutputPreviousResponseID(
	storeDisabled bool,
	turn int,
	signals ToolContinuationSignals,
	currentPreviousResponseID string,
	expectedPreviousResponseID string,
) bool {
	if !storeDisabled || turn <= 1 || !signals.HasFunctionCallOutput {
		return false
REDACTED
	if strings.TrimSpace(currentPreviousResponseID) != "" {
		return false
REDACTED
	if signals.HasFunctionCallOutputMissingCallID {
		return false
REDACTED
	// If the client already sent the actual tool-call context, treat this as
	// a full replay / self-contained continuation payload rather than
	// downgrading it into an inferred delta continuation. item_reference alone
	// is not enough on the store=false WS path: it still needs a valid prior
	// response anchor so upstream can resolve the referenced function_call.
	if signals.HasToolCallContext {
		return false
REDACTED
	return strings.TrimSpace(expectedPreviousResponseID) != ""
REDACTED

func alignStoreDisabledPreviousResponseID(
	payload []byte,
	expectedPreviousResponseID string,
) ([]byte, bool, error) {
	if len(payload) == 0 {
		return payload, false, nil
REDACTED
	expected := strings.TrimSpace(expectedPreviousResponseID)
	if expected == "" {
		return payload, false, nil
REDACTED
	current := openAIWSPayloadStringFromRaw(payload, "previous_response_id")
	if current == "" || current == expected {
		return payload, false, nil
REDACTED

	withoutPrev, removed, dropErr := dropPreviousResponseIDFromRawPayload(payload)
	if dropErr != nil {
		return payload, false, dropErr
REDACTED
	if !removed {
		return payload, false, nil
REDACTED
	updated, setErr := setPreviousResponseIDToRawPayload(withoutPrev, expected)
	if setErr != nil {
		return payload, false, setErr
REDACTED
	return updated, true, nil
REDACTED

func cloneOpenAIWSPayloadBytes(payload []byte) []byte {
	if len(payload) == 0 {
		return nil
REDACTED
	cloned := make([]byte, len(payload))
	copy(cloned, payload)
	return cloned
REDACTED

func cloneOpenAIWSRawMessages(items []json.RawMessage) []json.RawMessage {
	if items == nil {
		return nil
REDACTED
	cloned := make([]json.RawMessage, 0, len(items))
	for idx := range items {
		cloned = append(cloned, json.RawMessage(cloneOpenAIWSPayloadBytes(items[idx])))
REDACTED
	return cloned
REDACTED

func normalizeOpenAIWSJSONForCompare(raw []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("json is empty")
REDACTED
	var decoded any
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		return nil, err
REDACTED
	return json.Marshal(decoded)
REDACTED

func normalizeOpenAIWSJSONForCompareOrRaw(raw []byte) []byte {
	normalized, err := normalizeOpenAIWSJSONForCompare(raw)
	if err != nil {
		return bytes.TrimSpace(raw)
REDACTED
	return normalized
REDACTED

func normalizeOpenAIWSPayloadWithoutInputAndPreviousResponseID(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, errors.New("payload is empty")
REDACTED
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, err
REDACTED
	delete(decoded, "input")
	delete(decoded, "previous_response_id")
	return json.Marshal(decoded)
REDACTED

func openAIWSExtractNormalizedInputSequence(payload []byte) ([]json.RawMessage, bool, error) {
	if len(payload) == 0 {
		return nil, false, nil
REDACTED
	inputValue := gjson.GetBytes(payload, "input")
	if !inputValue.Exists() {
		return nil, false, nil
REDACTED
	if inputValue.Type == gjson.JSON {
		raw := strings.TrimSpace(inputValue.Raw)
		if strings.HasPrefix(raw, "[") {
			var items []json.RawMessage
			if err := json.Unmarshal([]byte(raw), &items); err != nil {
				return nil, true, err
		REDACTED
			return items, true, nil
	REDACTED
		return []json.RawMessage{json.RawMessage(raw)REDACTED, true, nil
REDACTED
	if inputValue.Type == gjson.String {
		encoded, _ := json.Marshal(inputValue.String())
		return []json.RawMessage{encodedREDACTED, true, nil
REDACTED
	return []json.RawMessage{json.RawMessage(inputValue.Raw)REDACTED, true, nil
REDACTED

func openAIWSInputIsPrefixExtended(previousPayload, currentPayload []byte) (bool, error) {
	previousItems, previousExists, prevErr := openAIWSExtractNormalizedInputSequence(previousPayload)
	if prevErr != nil {
		return false, prevErr
REDACTED
	currentItems, currentExists, currentErr := openAIWSExtractNormalizedInputSequence(currentPayload)
	if currentErr != nil {
		return false, currentErr
REDACTED
	if !previousExists && !currentExists {
		return true, nil
REDACTED
	if !previousExists {
		return len(currentItems) == 0, nil
REDACTED
	if !currentExists {
		return len(previousItems) == 0, nil
REDACTED
	if len(currentItems) < len(previousItems) {
		return false, nil
REDACTED

	for idx := range previousItems {
		previousNormalized := normalizeOpenAIWSJSONForCompareOrRaw(previousItems[idx])
		currentNormalized := normalizeOpenAIWSJSONForCompareOrRaw(currentItems[idx])
		if !bytes.Equal(previousNormalized, currentNormalized) {
			return false, nil
	REDACTED
REDACTED
	return true, nil
REDACTED

func openAIWSRawItemsHasPrefix(items []json.RawMessage, prefix []json.RawMessage) bool {
	if len(prefix) == 0 {
		return true
REDACTED
	if len(items) < len(prefix) {
		return false
REDACTED
	for idx := range prefix {
		previousNormalized := normalizeOpenAIWSJSONForCompareOrRaw(prefix[idx])
		currentNormalized := normalizeOpenAIWSJSONForCompareOrRaw(items[idx])
		if !bytes.Equal(previousNormalized, currentNormalized) {
			return false
	REDACTED
REDACTED
	return true
REDACTED

func openAIWSRawItemsHasFunctionCallOutput(items []json.RawMessage) bool {
	for _, item := range items {
		if isCodexToolCallOutputItemType(gjson.GetBytes(item, "type").String()) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func openAIWSRawItemsHaveToolCallContextForOutputs(items []json.RawMessage) bool {
	if len(items) == 0 {
		return false
REDACTED
	contextCallIDs := make(map[string]struct{REDACTED)
	outputCallIDs := make(map[string]struct{REDACTED)
	for _, item := range items {
		itemType := gjson.GetBytes(item, "type").String()
		callID := strings.TrimSpace(gjson.GetBytes(item, "call_id").String())
		switch {
		case isCodexToolCallContextItemType(itemType):
			if callID != "" {
				contextCallIDs[callID] = struct{REDACTED{REDACTED
		REDACTED
		case isCodexToolCallOutputItemType(itemType):
			if callID == "" {
				return false
		REDACTED
			outputCallIDs[callID] = struct{REDACTED{REDACTED
	REDACTED
REDACTED
	if len(outputCallIDs) == 0 || len(contextCallIDs) == 0 {
		return false
REDACTED
	for callID := range outputCallIDs {
		if _, ok := contextCallIDs[callID]; !ok {
			return false
	REDACTED
REDACTED
	return true
REDACTED

func openAIWSRawPayloadHasToolCallOutput(payload []byte) bool {
	if len(payload) == 0 {
		return false
REDACTED
	input := gjson.GetBytes(payload, "input")
	if !input.Exists() {
		return false
REDACTED
	if input.IsArray() {
		for _, item := range input.Array() {
			if isCodexToolCallOutputItemType(item.Get("type").String()) {
				return true
		REDACTED
	REDACTED
		return false
REDACTED
	if input.Type == gjson.JSON {
		return isCodexToolCallOutputItemType(input.Get("type").String())
REDACTED
	return false
REDACTED

func buildOpenAIWSReplayInputSequence(
	previousFullInput []json.RawMessage,
	previousFullInputExists bool,
	currentPayload []byte,
	hasPreviousResponseID bool,
) ([]json.RawMessage, bool, error) {
	currentItems, currentExists, currentErr := openAIWSExtractNormalizedInputSequence(currentPayload)
	if currentErr != nil {
		return nil, false, currentErr
REDACTED
	if !hasPreviousResponseID {
		return cloneOpenAIWSRawMessages(currentItems), currentExists, nil
REDACTED
	if !previousFullInputExists {
		return cloneOpenAIWSRawMessages(currentItems), currentExists, nil
REDACTED
	if !currentExists || len(currentItems) == 0 {
		return cloneOpenAIWSRawMessages(previousFullInput), true, nil
REDACTED
	if openAIWSRawItemsHasPrefix(currentItems, previousFullInput) {
		return cloneOpenAIWSRawMessages(currentItems), true, nil
REDACTED
	merged := make([]json.RawMessage, 0, len(previousFullInput)+len(currentItems))
	merged = append(merged, cloneOpenAIWSRawMessages(previousFullInput)...)
	merged = append(merged, cloneOpenAIWSRawMessages(currentItems)...)
	return merged, true, nil
REDACTED

func setOpenAIWSPayloadInputSequence(
	payload []byte,
	fullInput []json.RawMessage,
	fullInputExists bool,
) ([]byte, error) {
	if !fullInputExists {
		return payload, nil
REDACTED
	// Preserve [] vs null semantics when input exists but is empty.
	inputForMarshal := fullInput
	if inputForMarshal == nil {
		inputForMarshal = []json.RawMessage{REDACTED
REDACTED
	inputRaw, marshalErr := json.Marshal(inputForMarshal)
	if marshalErr != nil {
		return nil, marshalErr
REDACTED
	return sjson.SetRawBytes(payload, "input", inputRaw)
REDACTED

func shouldKeepIngressPreviousResponseID(
	previousPayload []byte,
	currentPayload []byte,
	lastTurnResponseID string,
	hasFunctionCallOutput bool,
) (bool, string, error) {
	if hasFunctionCallOutput {
		return true, "has_function_call_output", nil
REDACTED
	currentPreviousResponseID := strings.TrimSpace(openAIWSPayloadStringFromRaw(currentPayload, "previous_response_id"))
	if currentPreviousResponseID == "" {
		return false, "missing_previous_response_id", nil
REDACTED
	expectedPreviousResponseID := strings.TrimSpace(lastTurnResponseID)
	if expectedPreviousResponseID == "" {
		return false, "missing_last_turn_response_id", nil
REDACTED
	if currentPreviousResponseID != expectedPreviousResponseID {
		return false, "previous_response_id_mismatch", nil
REDACTED
	if len(previousPayload) == 0 {
		return false, "missing_previous_turn_payload", nil
REDACTED

	previousComparable, previousComparableErr := normalizeOpenAIWSPayloadWithoutInputAndPreviousResponseID(previousPayload)
	if previousComparableErr != nil {
		return false, "non_input_compare_error", previousComparableErr
REDACTED
	currentComparable, currentComparableErr := normalizeOpenAIWSPayloadWithoutInputAndPreviousResponseID(currentPayload)
	if currentComparableErr != nil {
		return false, "non_input_compare_error", currentComparableErr
REDACTED
	if !bytes.Equal(previousComparable, currentComparable) {
		return false, "non_input_changed", nil
REDACTED
	return true, "strict_incremental_ok", nil
REDACTED

type openAIWSIngressPreviousTurnStrictState struct {
	nonInputComparable []byte
REDACTED

func buildOpenAIWSIngressPreviousTurnStrictState(payload []byte) (*openAIWSIngressPreviousTurnStrictState, error) {
	if len(payload) == 0 {
		return nil, nil
REDACTED
	nonInputComparable, nonInputErr := normalizeOpenAIWSPayloadWithoutInputAndPreviousResponseID(payload)
	if nonInputErr != nil {
		return nil, nonInputErr
REDACTED
	return &openAIWSIngressPreviousTurnStrictState{
		nonInputComparable: nonInputComparable,
REDACTED, nil
REDACTED

func shouldKeepIngressPreviousResponseIDWithStrictState(
	previousState *openAIWSIngressPreviousTurnStrictState,
	currentPayload []byte,
	lastTurnResponseID string,
	hasFunctionCallOutput bool,
) (bool, string, error) {
	if hasFunctionCallOutput {
		return true, "has_function_call_output", nil
REDACTED
	currentPreviousResponseID := strings.TrimSpace(openAIWSPayloadStringFromRaw(currentPayload, "previous_response_id"))
	if currentPreviousResponseID == "" {
		return false, "missing_previous_response_id", nil
REDACTED
	expectedPreviousResponseID := strings.TrimSpace(lastTurnResponseID)
	if expectedPreviousResponseID == "" {
		return false, "missing_last_turn_response_id", nil
REDACTED
	if currentPreviousResponseID != expectedPreviousResponseID {
		return false, "previous_response_id_mismatch", nil
REDACTED
	if previousState == nil {
		return false, "missing_previous_turn_payload", nil
REDACTED

	currentComparable, currentComparableErr := normalizeOpenAIWSPayloadWithoutInputAndPreviousResponseID(currentPayload)
	if currentComparableErr != nil {
		return false, "non_input_compare_error", currentComparableErr
REDACTED
	if !bytes.Equal(previousState.nonInputComparable, currentComparable) {
		return false, "non_input_changed", nil
REDACTED
	return true, "strict_incremental_ok", nil
REDACTED
