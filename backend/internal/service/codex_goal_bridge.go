package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	CodexGoalProtocolOpenAIResponses = "openai_responses"
	CodexGoalProtocolOpenAIChat      = "openai_chat_completions"
	CodexGoalProtocolAnthropic       = "anthropic_messages"
	CodexGoalProtocolGemini          = "gemini_generate_content"

	codexGoalImportSource = "codex_session"
)

// CodexGoalBridgeError is rendered by the HTTP layer using the caller's protocol shape.
type CodexGoalBridgeError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *CodexGoalBridgeError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func codexGoalBridgeError(status int, code, message string) *CodexGoalBridgeError {
	return &CodexGoalBridgeError{StatusCode: status, Code: code, Message: message}
}

type CodexGoalBridgeRequest struct {
	Protocol string
	Endpoint string
	Body     []byte
	GroupID  *int64
}

type CodexGoalBridgeResponse struct {
	Protocol      string
	Model         string
	Text          string
	AccountID     int64
	Account       *Account
	CreatedAt     time.Time
	Duration      time.Duration
	Stream        bool
	ToolEvents    []CodexGoalToolEvent
	FunctionCalls []CodexGoalFunctionCall
}

type CodexGoalBridgeStreamEventType string

const (
	CodexGoalBridgeStreamEventStart                  CodexGoalBridgeStreamEventType = "start"
	CodexGoalBridgeStreamEventDelta                  CodexGoalBridgeStreamEventType = "delta"
	CodexGoalBridgeStreamEventToolEvent              CodexGoalBridgeStreamEventType = "tool_event"
	CodexGoalBridgeStreamEventFunctionCallStart      CodexGoalBridgeStreamEventType = "function_call_start"
	CodexGoalBridgeStreamEventFunctionArgumentsDelta CodexGoalBridgeStreamEventType = "function_call_arguments_delta"
	CodexGoalBridgeStreamEventFunctionCallDone       CodexGoalBridgeStreamEventType = "function_call_done"
)

type CodexGoalBridgeStreamEvent struct {
	Type                     CodexGoalBridgeStreamEventType
	Protocol                 string
	Model                    string
	AccountID                int64
	Account                  *Account
	CreatedAt                time.Time
	Delta                    string
	DeferResponsesOutputItem bool
	ToolEvent                CodexGoalToolEvent
	ToolEventIndex           int
	FunctionCall             CodexGoalFunctionCall
	FunctionCallIndex        int
}

type CodexGoalBridgeStreamSink func(CodexGoalBridgeStreamEvent) error

type CodexGoalRunInput struct {
	Objective       string
	AuthJSON        []byte
	Command         string
	RequiredVersion string
	CWD             string
	Model           string
	ReasoningEffort string
	Timeout         time.Duration
	EnableWebSearch bool
	MCPServers      []CodexGoalMCPServerConfig
	ToolEventSink   CodexGoalToolEventSink
}

type CodexGoalMCPServerConfig struct {
	Label          string
	URL            string
	Command        string
	Args           []string
	Description    string
	HTTPHeaders    map[string]string
	EnabledTools   []string
	ApprovalMode   string
	ToolTimeoutSec int
	StartupTimeout int
}

type CodexGoalRequestFeatures struct {
	EnableWebSearch bool
	MCPServers      []CodexGoalMCPServerConfig
	FunctionTools   []CodexGoalFunctionToolConfig
	ToolChoice      CodexGoalToolChoice
}

type CodexGoalRunResult struct {
	Text                 string
	ToolEvents           []CodexGoalToolEvent
	RefreshedCredentials map[string]any
}

type CodexGoalToolEvent struct {
	Type        string
	ID          string
	Status      string
	ServerLabel string
	Name        string
	Arguments   string
	Output      string
	Error       string
	Query       string
	Action      json.RawMessage
}

type CodexGoalFunctionToolConfig struct {
	Name           string
	Description    string
	ParametersJSON string
	Strict         bool
}

type CodexGoalToolChoice struct {
	Mode string
	Name string
}

type CodexGoalFunctionCall struct {
	ID        string
	CallID    string
	Name      string
	Arguments string
}

type codexGoalStoredResponse struct {
	ID            string
	Protocol      string
	Model         string
	Objective     string
	Text          string
	FunctionCalls []CodexGoalFunctionCall
	ToolEvents    []CodexGoalToolEvent
	CreatedAt     time.Time
}

const codexGoalStoredResponseLimit = 512

var codexGoalStoredResponses = struct {
	sync.Mutex
	items map[string]codexGoalStoredResponse
	order []string
	path  string
}{
	items: map[string]codexGoalStoredResponse{},
}

type CodexGoalDeltaSink func(delta string) error
type CodexGoalToolEventSink func(event CodexGoalToolEvent) error

type CodexGoalRunner interface {
	RunGoal(ctx context.Context, input CodexGoalRunInput) (*CodexGoalRunResult, error)
}

type CodexGoalStreamingRunner interface {
	RunGoalStream(ctx context.Context, input CodexGoalRunInput, onDelta CodexGoalDeltaSink) (*CodexGoalRunResult, error)
}

type CodexGoalBridgeService struct {
	accountRepo AccountRepository
	cfg         *config.Config
	runner      CodexGoalRunner
}

func NewCodexGoalBridgeService(accountRepo AccountRepository, cfg *config.Config) *CodexGoalBridgeService {
	if cfg != nil && cfg.Gateway.CodexGoalBridge.Enabled {
		configureCodexGoalStoredResponsePersistence(cfg.Gateway.CodexGoalBridge.CWD)
	}
	return &CodexGoalBridgeService{
		accountRepo: accountRepo,
		cfg:         cfg,
		runner:      CodexAppServerGoalRunner{},
	}
}

func (s *CodexGoalBridgeService) IsEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.CodexGoalBridge.Enabled
}

func (s *CodexGoalBridgeService) SetRunnerForTesting(runner CodexGoalRunner) {
	s.runner = runner
}

func (s *CodexGoalBridgeService) Handle(ctx context.Context, req CodexGoalBridgeRequest) (*CodexGoalBridgeResponse, error) {
	return s.handle(ctx, req, nil)
}

func (s *CodexGoalBridgeService) HandleStream(ctx context.Context, req CodexGoalBridgeRequest, sink CodexGoalBridgeStreamSink) (*CodexGoalBridgeResponse, error) {
	return s.handle(ctx, req, sink)
}

func (s *CodexGoalBridgeService) handle(ctx context.Context, req CodexGoalBridgeRequest, sink CodexGoalBridgeStreamSink) (*CodexGoalBridgeResponse, error) {
	if !s.IsEnabled() {
		return nil, codexGoalBridgeError(404, "codex_goal_bridge_disabled", "Codex goal bridge is disabled")
	}
	startedAt := time.Now()
	objective, model, stream, features, err := ExtractCodexGoalObjectiveAndFeatures(req)
	if err != nil {
		return nil, err
	}
	account, err := s.selectCodexSessionAccount(ctx, req.GroupID)
	if err != nil {
		return nil, err
	}
	authJSON, err := BuildCodexGoalAuthJSON(account)
	if err != nil {
		return nil, err
	}
	bridgeCfg := s.cfg.Gateway.CodexGoalBridge
	if bridgeCfg.Model != "" {
		model = bridgeCfg.Model
	}
	timeout := time.Duration(bridgeCfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	command := strings.TrimSpace(bridgeCfg.CodexCommand)
	if command == "" {
		command = "codex"
	}
	cwd := strings.TrimSpace(bridgeCfg.CWD)
	if cwd == "" {
		cwd = "."
	}
	cwd = codexGoalBridgeCWD(cwd)
	if model == "" {
		model = "gpt-5.5"
	}
	createdAt := time.Now()
	deferFunctionToolOutput := len(features.FunctionTools) > 0 && (req.Protocol == CodexGoalProtocolOpenAIResponses || req.Protocol == CodexGoalProtocolOpenAIChat || req.Protocol == CodexGoalProtocolAnthropic || req.Protocol == CodexGoalProtocolGemini)
	deferStreamOutputItem := deferFunctionToolOutput && (req.Protocol == CodexGoalProtocolOpenAIResponses || req.Protocol == CodexGoalProtocolAnthropic || req.Protocol == CodexGoalProtocolGemini)
	if sink != nil {
		if err := sink(CodexGoalBridgeStreamEvent{
			Type:                     CodexGoalBridgeStreamEventStart,
			Protocol:                 req.Protocol,
			Model:                    model,
			AccountID:                account.ID,
			Account:                  account,
			CreatedAt:                createdAt,
			DeferResponsesOutputItem: deferStreamOutputItem,
		}); err != nil {
			return nil, err
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	attachments, err := s.stageCodexGoalRequestAttachments(runCtx, req, cwd)
	if err != nil {
		return nil, err
	}
	if section := codexGoalAttachmentSection(attachments); section != "" {
		objective = strings.TrimSpace(objective + "\n\n" + section)
	}
	objective = limitCodexGoalObjective(objective)
	runInput := CodexGoalRunInput{
		Objective:       objective,
		AuthJSON:        authJSON,
		Command:         command,
		RequiredVersion: strings.TrimSpace(bridgeCfg.RequiredVersion),
		CWD:             cwd,
		Model:           model,
		ReasoningEffort: strings.TrimSpace(bridgeCfg.ReasoningEffort),
		Timeout:         timeout,
		EnableWebSearch: features.EnableWebSearch,
		MCPServers:      features.MCPServers,
	}
	var result *CodexGoalRunResult
	if sink != nil {
		toolEventIndex := 0
		runInput.ToolEventSink = func(toolEvent CodexGoalToolEvent) error {
			event := CodexGoalBridgeStreamEvent{
				Type:           CodexGoalBridgeStreamEventToolEvent,
				Protocol:       req.Protocol,
				Model:          model,
				AccountID:      account.ID,
				Account:        account,
				CreatedAt:      createdAt,
				ToolEvent:      toolEvent,
				ToolEventIndex: toolEventIndex,
			}
			toolEventIndex++
			return sink(event)
		}
		if streamingRunner, ok := s.runner.(CodexGoalStreamingRunner); ok {
			var onDelta CodexGoalDeltaSink
			if deferFunctionToolOutput {
				functionStream := newCodexGoalFunctionCallStreamParser(features.FunctionTools, createdAt, func(event CodexGoalBridgeStreamEvent) error {
					event.Protocol = req.Protocol
					event.Model = model
					event.AccountID = account.ID
					event.Account = account
					event.CreatedAt = createdAt
					return sink(event)
				})
				onDelta = functionStream.Write
			} else {
				onDelta = func(delta string) error {
					if delta == "" {
						return nil
					}
					return sink(CodexGoalBridgeStreamEvent{
						Type:      CodexGoalBridgeStreamEventDelta,
						Protocol:  req.Protocol,
						Model:     model,
						AccountID: account.ID,
						Account:   account,
						CreatedAt: createdAt,
						Delta:     delta,
					})
				}
			}
			result, err = streamingRunner.RunGoalStream(runCtx, runInput, onDelta)
		} else {
			result, err = s.runner.RunGoal(runCtx, runInput)
			if err == nil && result != nil && result.Text != "" && !deferFunctionToolOutput {
				err = sink(CodexGoalBridgeStreamEvent{
					Type:      CodexGoalBridgeStreamEventDelta,
					Protocol:  req.Protocol,
					Model:     model,
					AccountID: account.ID,
					Account:   account,
					CreatedAt: createdAt,
					Delta:     result.Text,
				})
			}
		}
	} else {
		result, err = s.runner.RunGoal(runCtx, runInput)
	}
	if err != nil {
		return nil, codexGoalBridgeError(502, "codex_goal_bridge_failed", err.Error())
	}
	if result == nil {
		return nil, codexGoalBridgeError(502, "codex_goal_bridge_failed", "Codex goal bridge returned no result")
	}
	if err := s.persistCodexGoalRefreshedCredentials(ctx, account, result.RefreshedCredentials); err != nil {
		return nil, codexGoalBridgeError(502, "codex_goal_auth_persist_failed", err.Error())
	}
	text := strings.TrimSpace(result.Text)
	var functionCalls []CodexGoalFunctionCall
	if (req.Protocol == CodexGoalProtocolOpenAIResponses || req.Protocol == CodexGoalProtocolOpenAIChat || req.Protocol == CodexGoalProtocolAnthropic || req.Protocol == CodexGoalProtocolGemini) && len(features.FunctionTools) > 0 {
		text, functionCalls = parseCodexGoalFunctionCallOutput(text, features.FunctionTools, createdAt)
	}
	if text == "" && len(functionCalls) == 0 {
		return nil, codexGoalBridgeError(502, "codex_goal_empty_response", "Codex goal bridge returned an empty response")
	}
	if req.Protocol == CodexGoalProtocolOpenAIResponses {
		saveCodexGoalStoredResponse(codexGoalStoredResponse{
			ID:            codexGoalResponseID(createdAt),
			Protocol:      req.Protocol,
			Model:         model,
			Objective:     objective,
			Text:          text,
			FunctionCalls: append([]CodexGoalFunctionCall(nil), functionCalls...),
			ToolEvents:    append([]CodexGoalToolEvent(nil), result.ToolEvents...),
			CreatedAt:     createdAt,
		})
	}
	return &CodexGoalBridgeResponse{
		Protocol:      req.Protocol,
		Model:         model,
		Text:          text,
		AccountID:     account.ID,
		Account:       account,
		CreatedAt:     createdAt,
		Duration:      time.Since(startedAt),
		Stream:        stream,
		ToolEvents:    append([]CodexGoalToolEvent(nil), result.ToolEvents...),
		FunctionCalls: append([]CodexGoalFunctionCall(nil), functionCalls...),
	}, nil
}

func (s *CodexGoalBridgeService) selectCodexSessionAccount(ctx context.Context, groupID *int64) (*Account, error) {
	if s.accountRepo == nil {
		return nil, codexGoalBridgeError(503, "codex_goal_no_account_repository", "account repository is unavailable")
	}
	var accounts []Account
	var err error
	if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, PlatformOpenAI)
	} else {
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, PlatformOpenAI)
	}
	if err != nil {
		return nil, codexGoalBridgeError(503, "codex_goal_account_lookup_failed", err.Error())
	}
	sort.SliceStable(accounts, func(i, j int) bool {
		if accounts[i].Priority != accounts[j].Priority {
			return accounts[i].Priority > accounts[j].Priority
		}
		return accounts[i].ID < accounts[j].ID
	})
	for i := range accounts {
		if IsCodexGoalSessionAccount(&accounts[i]) {
			account := accounts[i]
			return &account, nil
		}
	}
	return nil, codexGoalBridgeError(503, "codex_goal_no_codex_oauth_account", "no schedulable OpenAI OAuth account with Codex-compatible tokens was found")
}

func (s *CodexGoalBridgeService) persistCodexGoalRefreshedCredentials(ctx context.Context, account *Account, refreshed map[string]any) error {
	merged, changed := mergeCodexGoalRefreshedCredentials(account, refreshed)
	if !changed {
		return nil
	}
	return persistAccountCredentials(ctx, s.accountRepo, account, merged)
}

func IsCodexGoalSessionAccount(account *Account) bool {
	if account == nil || !account.IsOpenAIOAuth() || !account.IsSchedulable() {
		return false
	}
	if account.IsOpenAITokenExpired() || account.GetOpenAIAccessToken() == "" {
		return false
	}
	if account.GetOpenAIRefreshToken() == "" || account.GetOpenAIIDToken() == "" {
		return false
	}
	return codexGoalAccountID(account) != ""
}

func BuildCodexGoalAuthJSON(account *Account) ([]byte, error) {
	if !IsCodexGoalSessionAccount(account) {
		return nil, codexGoalBridgeError(503, "codex_goal_invalid_account", "selected account is not a usable Codex OAuth account")
	}
	accountID := codexGoalAccountID(account)
	authMode := codexGoalFirstNonEmpty(account.GetCredential("auth_mode"), extraString(account.Extra, "auth_mode"), "chatgpt")
	payload := map[string]any{
		"auth_mode":      authMode,
		"OPENAI_API_KEY": nil,
		"tokens": map[string]any{
			"id_token":      account.GetOpenAIIDToken(),
			"access_token":  account.GetOpenAIAccessToken(),
			"refresh_token": account.GetOpenAIRefreshToken(),
			"account_id":    accountID,
		},
		"last_refresh": time.Now().UTC().Format(time.RFC3339),
	}
	return json.MarshalIndent(payload, "", "  ")
}

func readCodexGoalRefreshedCredentials(authJSONPath string) map[string]any {
	data, err := os.ReadFile(authJSONPath)
	if err != nil {
		return nil
	}
	credentials, err := extractCodexGoalAuthJSONCredentials(data)
	if err != nil {
		return nil
	}
	return credentials
}

func extractCodexGoalAuthJSONCredentials(data []byte) (map[string]any, error) {
	var raw map[string]any
	if err := decodeCodexGoalPayload(data, &raw); err != nil {
		return nil, err
	}
	tokens, _ := raw["tokens"].(map[string]any)
	credentials := map[string]any{}
	set := func(key, value string) {
		if value = strings.TrimSpace(value); value != "" {
			credentials[key] = value
		}
	}
	set("access_token", codexGoalFirstNonEmpty(
		codexGoalStringFromAny(tokens["access_token"]),
		codexGoalStringFromAny(tokens["accessToken"]),
		codexGoalStringFromAny(raw["access_token"]),
		codexGoalStringFromAny(raw["accessToken"]),
	))
	set("refresh_token", codexGoalFirstNonEmpty(
		codexGoalStringFromAny(tokens["refresh_token"]),
		codexGoalStringFromAny(tokens["refreshToken"]),
		codexGoalStringFromAny(raw["refresh_token"]),
		codexGoalStringFromAny(raw["refreshToken"]),
	))
	set("id_token", codexGoalFirstNonEmpty(
		codexGoalStringFromAny(tokens["id_token"]),
		codexGoalStringFromAny(tokens["idToken"]),
		codexGoalStringFromAny(raw["id_token"]),
		codexGoalStringFromAny(raw["idToken"]),
	))
	set("chatgpt_account_id", codexGoalFirstNonEmpty(
		codexGoalStringFromAny(tokens["account_id"]),
		codexGoalStringFromAny(tokens["accountId"]),
		codexGoalStringFromAny(tokens["chatgpt_account_id"]),
		codexGoalStringFromAny(tokens["chatgptAccountId"]),
		codexGoalStringFromAny(raw["account_id"]),
		codexGoalStringFromAny(raw["accountId"]),
		codexGoalStringFromAny(raw["chatgpt_account_id"]),
		codexGoalStringFromAny(raw["chatgptAccountId"]),
	))
	set("auth_mode", codexGoalStringFromAny(raw["auth_mode"]))
	set("last_refresh", codexGoalStringFromAny(raw["last_refresh"]))
	if expiresAt, ok := codexGoalTimeFromAny(codexGoalFirstPresent(tokens["expires_at"], tokens["expiresAt"], raw["expires_at"], raw["expiresAt"])); ok {
		credentials["expires_at"] = expiresAt.Format(time.RFC3339)
	} else if accessToken := codexGoalStringFromAny(credentials["access_token"]); accessToken != "" {
		if expiresAt, ok := codexGoalJWTExpiresAt(accessToken); ok {
			credentials["expires_at"] = expiresAt.Format(time.RFC3339)
		}
	}
	return credentials, nil
}

func mergeCodexGoalRefreshedCredentials(account *Account, refreshed map[string]any) (map[string]any, bool) {
	if account == nil || len(refreshed) == 0 {
		return nil, false
	}
	merged := cloneCredentials(account.Credentials)
	changed := false
	setString := func(key string) {
		value := strings.TrimSpace(codexGoalStringFromAny(refreshed[key]))
		if value == "" {
			return
		}
		if strings.TrimSpace(codexGoalStringFromAny(merged[key])) != value {
			merged[key] = value
			changed = true
		}
	}
	for _, key := range []string{
		"access_token",
		"refresh_token",
		"id_token",
		"chatgpt_account_id",
		"auth_mode",
		"last_refresh",
	} {
		setString(key)
	}
	refreshedAccessToken := strings.TrimSpace(codexGoalStringFromAny(refreshed["access_token"]))
	refreshedExpiresAt := strings.TrimSpace(codexGoalStringFromAny(refreshed["expires_at"]))
	if refreshedExpiresAt != "" {
		if strings.TrimSpace(codexGoalStringFromAny(merged["expires_at"])) != refreshedExpiresAt {
			merged["expires_at"] = refreshedExpiresAt
			changed = true
		}
	} else if refreshedAccessToken != "" && strings.TrimSpace(codexGoalStringFromAny(account.Credentials["access_token"])) != refreshedAccessToken {
		if _, ok := merged["expires_at"]; ok {
			delete(merged, "expires_at")
			changed = true
		}
	}
	return merged, changed
}

func codexGoalJWTExpiresAt(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := codexGoalDecodeJWTSegment(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp any `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, false
	}
	return codexGoalTimeFromAny(claims.Exp)
}

func codexGoalDecodeJWTSegment(segment string) ([]byte, error) {
	if decoded, err := base64.RawURLEncoding.DecodeString(segment); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(segment); err == nil {
		return decoded, nil
	}
	return base64.URLEncoding.DecodeString(segment)
}

func codexGoalTimeFromAny(value any) (time.Time, bool) {
	switch v := value.(type) {
	case nil:
		return time.Time{}, false
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return time.Time{}, false
		}
		if parsed, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return parsed.UTC(), true
		}
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return codexGoalUnixTime(n), true
		}
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return codexGoalUnixTime(n), true
		}
		if f, err := v.Float64(); err == nil {
			return codexGoalUnixTime(int64(f)), true
		}
	case float64:
		return codexGoalUnixTime(int64(v)), true
	case int:
		return codexGoalUnixTime(int64(v)), true
	case int64:
		return codexGoalUnixTime(v), true
	}
	return time.Time{}, false
}

func codexGoalUnixTime(value int64) time.Time {
	if value > 1_000_000_000_000 {
		return time.UnixMilli(value).UTC()
	}
	return time.Unix(value, 0).UTC()
}

func codexGoalAccountID(account *Account) string {
	if account == nil {
		return ""
	}
	return codexGoalFirstNonEmpty(
		account.GetCredential("chatgpt_account_id"),
		account.GetCredential("account_id"),
		extraString(account.Extra, "chatgpt_account_id"),
		extraString(account.Extra, "account_id"),
	)
}

func ExtractCodexGoalObjective(req CodexGoalBridgeRequest) (string, string, bool, error) {
	objective, model, stream, _, err := ExtractCodexGoalObjectiveAndFeatures(req)
	return objective, model, stream, err
}

func ExtractCodexGoalObjectiveAndFeatures(req CodexGoalBridgeRequest) (string, string, bool, CodexGoalRequestFeatures, error) {
	var payload map[string]any
	if err := decodeCodexGoalPayload(req.Body, &payload); err != nil {
		return "", "", false, CodexGoalRequestFeatures{}, codexGoalBridgeError(400, "invalid_request_body", err.Error())
	}
	payload = normalizeCodexGoalPayload(payload)
	previousContext, err := codexGoalPreviousResponseContext(payload, req.Protocol)
	if err != nil {
		return "", "", false, CodexGoalRequestFeatures{}, err
	}
	if err := rejectCodexGoalUnsupportedTooling(payload, req.Protocol); err != nil {
		return "", "", false, CodexGoalRequestFeatures{}, err
	}
	stream := boolFromAny(payload["stream"])
	model := strings.TrimSpace(codexGoalStringFromAny(payload["model"]))
	var objective string
	switch req.Protocol {
	case CodexGoalProtocolOpenAIResponses:
		objective = extractOpenAIResponsesObjective(payload)
	case CodexGoalProtocolOpenAIChat:
		objective = extractOpenAIChatObjective(payload)
	case CodexGoalProtocolAnthropic:
		objective = extractAnthropicMessagesObjective(payload)
	case CodexGoalProtocolGemini:
		if strings.Contains(strings.ToLower(req.Endpoint), "streamgeneratecontent") {
			stream = true
		}
		objective = extractGeminiGenerateContentObjective(payload)
		if model == "" {
			model = geminiModelFromEndpoint(req.Endpoint)
		}
	default:
		return "", "", false, CodexGoalRequestFeatures{}, codexGoalBridgeError(400, "codex_goal_unsupported_protocol", "unsupported Codex goal bridge protocol")
	}
	features, featureSections, err := extractCodexGoalRequestFeatures(payload, req.Protocol)
	if err != nil {
		return "", "", false, CodexGoalRequestFeatures{}, err
	}
	if len(featureSections) > 0 {
		objective = strings.TrimSpace(strings.Join([]string{objective, strings.Join(featureSections, "\n\n")}, "\n\n"))
	}
	if previousContext != "" {
		objective = strings.TrimSpace(previousContext + "\n\nCurrent request:\n" + objective)
	}
	objective = strings.TrimSpace(objective)
	if objective == "" {
		return "", "", false, CodexGoalRequestFeatures{}, codexGoalBridgeError(400, "codex_goal_empty_objective", "request did not contain text that can be converted into a Codex goal")
	}
	return objective, model, stream, features, nil
}

func codexGoalResponseID(createdAt time.Time) string {
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	return "resp_codex_goal_" + strconv.FormatInt(createdAt.UnixNano(), 10)
}

func codexGoalPreviousResponseContext(payload map[string]any, protocol string) (string, error) {
	if protocol != CodexGoalProtocolOpenAIResponses {
		return "", nil
	}
	previousID := strings.TrimSpace(codexGoalStringFromAny(payload["previous_response_id"]))
	if previousID == "" {
		return "", nil
	}
	previous, ok := loadCodexGoalStoredResponse(previousID)
	if !ok {
		return "", codexGoalBridgeError(400, "codex_goal_previous_response_not_found", "Codex goal bridge previous_response_id was not found in the response cache")
	}
	var b strings.Builder
	b.WriteString("Previous response context")
	if !previous.CreatedAt.IsZero() {
		b.WriteString(" (")
		b.WriteString(previous.CreatedAt.UTC().Format(time.RFC3339))
		b.WriteString(")")
	}
	b.WriteString(":\n")
	if previous.Objective != "" {
		b.WriteString("Previous request:\n")
		b.WriteString(previous.Objective)
		b.WriteString("\n\n")
	}
	if previous.Text != "" {
		b.WriteString("Previous assistant response:\n")
		b.WriteString(previous.Text)
	}
	for _, call := range previous.FunctionCalls {
		block := codexGoalLabeledBlock("Previous assistant function call", []string{
			codexGoalLabelValue("name", call.Name),
			codexGoalLabelValue("call_id", call.CallID),
			codexGoalLabelValue("arguments", call.Arguments),
		})
		if block == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(block)
	}
	for _, event := range previous.ToolEvents {
		block := codexGoalPreviousToolEventBlock(event)
		if block == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(block)
	}
	return strings.TrimSpace(b.String()), nil
}

func codexGoalPreviousToolEventBlock(event CodexGoalToolEvent) string {
	switch event.Type {
	case "web_search_call":
		return codexGoalLabeledBlock("Previous web search call", []string{
			codexGoalLabelValue("id", event.ID),
			codexGoalLabelValue("status", event.Status),
			codexGoalLabelValue("query", event.Query),
			codexGoalLabelValue("action", string(event.Action)),
		})
	case "mcp_call":
		return codexGoalLabeledBlock("Previous MCP tool call", []string{
			codexGoalLabelValue("id", event.ID),
			codexGoalLabelValue("server_label", event.ServerLabel),
			codexGoalLabelValue("name", event.Name),
			codexGoalLabelValue("arguments", event.Arguments),
			codexGoalLabelValue("output", event.Output),
			codexGoalLabelValue("error", event.Error),
		})
	default:
		return ""
	}
}

func saveCodexGoalStoredResponse(response codexGoalStoredResponse) {
	response.ID = strings.TrimSpace(response.ID)
	if response.ID == "" {
		return
	}
	codexGoalStoredResponses.Lock()
	defer codexGoalStoredResponses.Unlock()
	if codexGoalStoredResponses.items == nil {
		codexGoalStoredResponses.items = map[string]codexGoalStoredResponse{}
	}
	if _, exists := codexGoalStoredResponses.items[response.ID]; !exists {
		codexGoalStoredResponses.order = append(codexGoalStoredResponses.order, response.ID)
	}
	codexGoalStoredResponses.items[response.ID] = response
	for len(codexGoalStoredResponses.order) > codexGoalStoredResponseLimit {
		oldest := codexGoalStoredResponses.order[0]
		codexGoalStoredResponses.order = codexGoalStoredResponses.order[1:]
		delete(codexGoalStoredResponses.items, oldest)
	}
	persistCodexGoalStoredResponsesLocked()
}

func loadCodexGoalStoredResponse(id string) (codexGoalStoredResponse, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return codexGoalStoredResponse{}, false
	}
	codexGoalStoredResponses.Lock()
	defer codexGoalStoredResponses.Unlock()
	response, ok := codexGoalStoredResponses.items[id]
	return response, ok
}

func resetCodexGoalStoredResponsesForTesting() {
	codexGoalStoredResponses.Lock()
	defer codexGoalStoredResponses.Unlock()
	codexGoalStoredResponses.items = map[string]codexGoalStoredResponse{}
	codexGoalStoredResponses.order = nil
	codexGoalStoredResponses.path = ""
}

func configureCodexGoalStoredResponsePersistence(cwd string) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return
	}
	if !filepath.IsAbs(cwd) {
		if abs, err := filepath.Abs(cwd); err == nil {
			cwd = abs
		}
	}
	path := filepath.Join(cwd, ".codex-goal-bridge", "responses.json")
	codexGoalStoredResponses.Lock()
	defer codexGoalStoredResponses.Unlock()
	if codexGoalStoredResponses.path == path {
		return
	}
	codexGoalStoredResponses.path = path
	loadCodexGoalStoredResponsesLocked()
}

type codexGoalStoredResponseSnapshot struct {
	Items map[string]codexGoalStoredResponse `json:"items"`
	Order []string                           `json:"order"`
}

func loadCodexGoalStoredResponsesLocked() {
	if codexGoalStoredResponses.path == "" {
		return
	}
	data, err := os.ReadFile(codexGoalStoredResponses.path)
	if err != nil {
		return
	}
	var snapshot codexGoalStoredResponseSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return
	}
	if snapshot.Items == nil {
		return
	}
	if codexGoalStoredResponses.items == nil {
		codexGoalStoredResponses.items = map[string]codexGoalStoredResponse{}
	}
	seen := make(map[string]struct{}, len(codexGoalStoredResponses.order)+len(snapshot.Order))
	for _, id := range codexGoalStoredResponses.order {
		id = strings.TrimSpace(id)
		if id != "" {
			seen[id] = struct{}{}
		}
	}
	for _, id := range snapshot.Order {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		response, ok := snapshot.Items[id]
		if !ok {
			continue
		}
		if _, exists := codexGoalStoredResponses.items[id]; !exists {
			codexGoalStoredResponses.items[id] = response
		}
		if _, exists := seen[id]; !exists {
			codexGoalStoredResponses.order = append(codexGoalStoredResponses.order, id)
			seen[id] = struct{}{}
		}
	}
	for len(codexGoalStoredResponses.order) > codexGoalStoredResponseLimit {
		oldest := codexGoalStoredResponses.order[0]
		codexGoalStoredResponses.order = codexGoalStoredResponses.order[1:]
		delete(codexGoalStoredResponses.items, oldest)
	}
}

func persistCodexGoalStoredResponsesLocked() {
	if codexGoalStoredResponses.path == "" {
		return
	}
	snapshot := codexGoalStoredResponseSnapshot{
		Items: codexGoalStoredResponses.items,
		Order: codexGoalStoredResponses.order,
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(codexGoalStoredResponses.path), 0700); err != nil {
		return
	}
	tmpPath := codexGoalStoredResponses.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return
	}
	_ = os.Rename(tmpPath, codexGoalStoredResponses.path)
}

func normalizeCodexGoalPayload(payload map[string]any) map[string]any {
	if payload == nil {
		return payload
	}
	response, ok := payload["response"].(map[string]any)
	if !ok {
		return payload
	}
	normalized := make(map[string]any, len(response)+len(payload))
	for key, value := range response {
		normalized[key] = value
	}
	for key, value := range payload {
		if key == "response" {
			continue
		}
		if _, exists := normalized[key]; !exists {
			normalized[key] = value
		}
	}
	return normalized
}

func extractCodexGoalRequestFeatures(payload map[string]any, protocol string) (CodexGoalRequestFeatures, []string, error) {
	var features CodexGoalRequestFeatures
	if protocol == CodexGoalProtocolOpenAIChat {
		return extractCodexGoalChatRequestFeatures(payload)
	}
	if protocol == CodexGoalProtocolAnthropic {
		return extractCodexGoalAnthropicRequestFeatures(payload)
	}
	if protocol == CodexGoalProtocolGemini {
		return extractCodexGoalGeminiRequestFeatures(payload)
	}
	if protocol != CodexGoalProtocolOpenAIResponses {
		return features, nil, nil
	}
	features.ToolChoice = codexGoalToolChoiceFromAny(payload["tool_choice"])
	if features.ToolChoice.Mode == "none" {
		return features, nil, nil
	}
	tools, ok := payload["tools"].([]any)
	if !ok || len(tools) == 0 {
		if isCodexGoalForcedToolChoice(features.ToolChoice.Mode, payload["tool_choice"]) {
			return features, nil, codexGoalBridgeError(400, "codex_goal_tool_choice_unsupported", "Codex goal bridge cannot preserve tool_choice values that force a tool call without a supported function tool")
		}
		return features, nil, nil
	}
	var sections []string
	for i, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			return features, nil, codexGoalBridgeError(400, "codex_goal_tool_unsupported", "Codex goal bridge only supports Responses web_search and HTTPS remote MCP tools")
		}
		toolType := strings.ToLower(strings.TrimSpace(codexGoalStringFromAny(tool["type"])))
		switch {
		case isCodexGoalWebSearchTool(toolType):
			features.EnableWebSearch = true
		case toolType == "mcp":
			server, section, err := codexGoalMCPServerFromTool(i, tool)
			if err != nil {
				return features, nil, err
			}
			features.MCPServers = append(features.MCPServers, server)
			if section != "" {
				sections = append(sections, section)
			}
		case toolType == "function":
			functionTool, err := codexGoalFunctionToolFromTool(tool)
			if err != nil {
				return features, nil, err
			}
			features.FunctionTools = append(features.FunctionTools, functionTool)
		default:
			return features, nil, codexGoalBridgeError(400, "codex_goal_tool_unsupported", fmt.Sprintf("Codex goal bridge cannot preserve Responses tool type %q", toolType))
		}
	}
	forcedToolChoice := isCodexGoalForcedToolChoice(features.ToolChoice.Mode, payload["tool_choice"])
	if forcedToolChoice && len(features.FunctionTools) == 0 && !features.EnableWebSearch && len(features.MCPServers) == 0 {
		return features, nil, codexGoalBridgeError(400, "codex_goal_tool_choice_unsupported", "Codex goal bridge can only preserve forced tool_choice for supported tools")
	}
	if features.ToolChoice.Name != "" && !codexGoalFunctionToolExists(features.ToolChoice.Name, features.FunctionTools) && !codexGoalHostedToolChoiceExists(features.ToolChoice, features) {
		return features, nil, codexGoalBridgeError(400, "codex_goal_tool_choice_unsupported", fmt.Sprintf("Codex goal bridge tool_choice references unknown tool %q", features.ToolChoice.Name))
	}
	if features.EnableWebSearch {
		sections = append([]string{"Available hosted tool:\n- web_search: The client enabled OpenAI Responses web_search. Use live web search when the request needs current or external information, and include source URLs or citations in the final answer when web results are used."}, sections...)
	}
	if section := codexGoalHostedToolChoiceSection(features, forcedToolChoice); section != "" {
		sections = append(sections, section)
	}
	if len(features.FunctionTools) > 0 {
		sections = append(sections, codexGoalFunctionToolSection(features.FunctionTools, features.ToolChoice))
	}
	return features, sections, nil
}

func extractCodexGoalChatRequestFeatures(payload map[string]any) (CodexGoalRequestFeatures, []string, error) {
	var features CodexGoalRequestFeatures
	features.ToolChoice = codexGoalToolChoiceFromAny(payload["tool_choice"])
	if _, ok := payload["tool_choice"]; !ok {
		features.ToolChoice = codexGoalToolChoiceFromAny(payload["function_call"])
	}
	if features.ToolChoice.Mode == "none" {
		return features, nil, nil
	}
	tools, err := codexGoalChatFunctionTools(payload)
	if err != nil {
		return features, nil, err
	}
	features.FunctionTools = tools
	if isCodexGoalForcedToolChoice(features.ToolChoice.Mode, codexGoalFirstPresent(payload["tool_choice"], payload["function_call"])) && len(features.FunctionTools) == 0 {
		return features, nil, codexGoalBridgeError(400, "codex_goal_tool_choice_unsupported", "Codex goal bridge can only preserve forced Chat tool_choice/function_call for function tools")
	}
	if features.ToolChoice.Name != "" && !codexGoalFunctionToolExists(features.ToolChoice.Name, features.FunctionTools) {
		return features, nil, codexGoalBridgeError(400, "codex_goal_tool_choice_unsupported", fmt.Sprintf("Codex goal bridge tool_choice references unknown function %q", features.ToolChoice.Name))
	}
	if len(features.FunctionTools) == 0 {
		return features, nil, nil
	}
	return features, []string{codexGoalFunctionToolSection(features.FunctionTools, features.ToolChoice)}, nil
}

func extractCodexGoalAnthropicRequestFeatures(payload map[string]any) (CodexGoalRequestFeatures, []string, error) {
	var features CodexGoalRequestFeatures
	features.ToolChoice = codexGoalToolChoiceFromAny(payload["tool_choice"])
	if features.ToolChoice.Mode == "none" {
		return features, nil, nil
	}
	rawTools, ok := payload["tools"].([]any)
	if !ok || len(rawTools) == 0 {
		if isCodexGoalForcedToolChoice(features.ToolChoice.Mode, payload["tool_choice"]) {
			return features, nil, codexGoalBridgeError(400, "codex_goal_tool_choice_unsupported", "Codex goal bridge can only preserve forced Anthropic tool_choice when tools are provided")
		}
		return features, nil, nil
	}
	for _, rawTool := range rawTools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			return features, nil, codexGoalBridgeError(400, "codex_goal_tool_unsupported", "Codex goal bridge only supports Anthropic function-style tools")
		}
		functionTool, err := codexGoalFunctionToolFromTool(tool)
		if err != nil {
			return features, nil, err
		}
		features.FunctionTools = append(features.FunctionTools, functionTool)
	}
	if isCodexGoalForcedToolChoice(features.ToolChoice.Mode, payload["tool_choice"]) && len(features.FunctionTools) == 0 {
		return features, nil, codexGoalBridgeError(400, "codex_goal_tool_choice_unsupported", "Codex goal bridge can only preserve forced Anthropic tool_choice for function tools")
	}
	if features.ToolChoice.Name != "" && !codexGoalFunctionToolExists(features.ToolChoice.Name, features.FunctionTools) {
		return features, nil, codexGoalBridgeError(400, "codex_goal_tool_choice_unsupported", fmt.Sprintf("Codex goal bridge Anthropic tool_choice references unknown tool %q", features.ToolChoice.Name))
	}
	return features, []string{codexGoalFunctionToolSection(features.FunctionTools, features.ToolChoice)}, nil
}

func extractCodexGoalGeminiRequestFeatures(payload map[string]any) (CodexGoalRequestFeatures, []string, error) {
	var features CodexGoalRequestFeatures
	features.ToolChoice = codexGoalGeminiToolChoice(payload)
	if features.ToolChoice.Mode == "none" {
		return features, nil, nil
	}
	rawTools, ok := payload["tools"].([]any)
	if !ok || len(rawTools) == 0 {
		if isCodexGoalGeminiForcedToolChoice(features.ToolChoice.Mode) {
			return features, nil, codexGoalBridgeError(400, "codex_goal_tool_choice_unsupported", "Codex goal bridge can only preserve forced Gemini toolConfig when tools are provided")
		}
		return features, nil, nil
	}
	for _, rawTool := range rawTools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			return features, nil, codexGoalBridgeError(400, "codex_goal_tool_unsupported", "Codex goal bridge only supports Gemini functionDeclarations and googleSearch tools")
		}
		recognized := false
		if rawDecls, ok := codexGoalFirstPresent(tool["functionDeclarations"], tool["function_declarations"]).([]any); ok {
			recognized = true
			for _, rawDecl := range rawDecls {
				decl, ok := rawDecl.(map[string]any)
				if !ok {
					return features, nil, codexGoalBridgeError(400, "codex_goal_tool_unsupported", "Codex goal bridge only supports Gemini functionDeclarations objects")
				}
				functionTool, err := codexGoalFunctionToolFromTool(decl)
				if err != nil {
					return features, nil, err
				}
				features.FunctionTools = append(features.FunctionTools, functionTool)
			}
		}
		if _, ok := codexGoalFirstPresent(tool["googleSearch"], tool["google_search"], tool["googleSearchRetrieval"], tool["google_search_retrieval"]).(map[string]any); ok {
			recognized = true
			features.EnableWebSearch = true
		}
		if !recognized {
			return features, nil, codexGoalBridgeError(400, "codex_goal_tool_unsupported", "Codex goal bridge cannot preserve this Gemini tool type")
		}
	}
	allowedNames := codexGoalGeminiAllowedFunctionNames(payload)
	if len(allowedNames) > 0 {
		filtered, missing := filterCodexGoalFunctionToolsByName(features.FunctionTools, allowedNames)
		if len(missing) > 0 {
			return features, nil, codexGoalBridgeError(400, "codex_goal_tool_choice_unsupported", fmt.Sprintf("Codex goal bridge Gemini toolConfig references unknown function %q", missing[0]))
		}
		features.FunctionTools = filtered
		if len(allowedNames) == 1 && isCodexGoalGeminiForcedToolChoice(features.ToolChoice.Mode) {
			features.ToolChoice.Name = allowedNames[0]
			features.ToolChoice.Mode = "function"
		}
	}
	if isCodexGoalGeminiForcedToolChoice(features.ToolChoice.Mode) && len(features.FunctionTools) == 0 {
		return features, nil, codexGoalBridgeError(400, "codex_goal_tool_choice_unsupported", "Codex goal bridge can only preserve forced Gemini toolConfig for function declarations")
	}
	if features.ToolChoice.Name != "" && !codexGoalFunctionToolExists(features.ToolChoice.Name, features.FunctionTools) {
		return features, nil, codexGoalBridgeError(400, "codex_goal_tool_choice_unsupported", fmt.Sprintf("Codex goal bridge Gemini toolConfig references unknown function %q", features.ToolChoice.Name))
	}
	var sections []string
	if features.EnableWebSearch {
		sections = append(sections, "Available hosted tool:\n- google_search: The client enabled Gemini Google Search. Use live web search when the request needs current or external information, and include source URLs or citations in the final answer when web results are used.")
	}
	if len(features.FunctionTools) > 0 {
		sections = append(sections, codexGoalFunctionToolSection(features.FunctionTools, features.ToolChoice))
	}
	return features, sections, nil
}

func codexGoalChatFunctionTools(payload map[string]any) ([]CodexGoalFunctionToolConfig, error) {
	var tools []CodexGoalFunctionToolConfig
	if rawTools, ok := payload["tools"].([]any); ok {
		for _, rawTool := range rawTools {
			tool, ok := rawTool.(map[string]any)
			if !ok {
				return nil, codexGoalBridgeError(400, "codex_goal_tool_unsupported", "Codex goal bridge only supports Chat Completions function tools")
			}
			toolType := strings.ToLower(strings.TrimSpace(codexGoalStringFromAny(tool["type"])))
			if toolType != "function" {
				return nil, codexGoalBridgeError(400, "codex_goal_tool_unsupported", fmt.Sprintf("Codex goal bridge cannot preserve Chat Completions tool type %q", toolType))
			}
			functionTool, err := codexGoalFunctionToolFromTool(tool)
			if err != nil {
				return nil, err
			}
			tools = append(tools, functionTool)
		}
	}
	if rawFunctions, ok := payload["functions"].([]any); ok {
		for _, rawFunction := range rawFunctions {
			fn, ok := rawFunction.(map[string]any)
			if !ok {
				return nil, codexGoalBridgeError(400, "codex_goal_tool_unsupported", "Codex goal bridge only supports Chat Completions function definitions")
			}
			functionTool, err := codexGoalFunctionToolFromTool(fn)
			if err != nil {
				return nil, err
			}
			tools = append(tools, functionTool)
		}
	}
	return tools, nil
}

func rejectCodexGoalUnsupportedTooling(payload map[string]any, protocol string) error {
	switch protocol {
	case CodexGoalProtocolOpenAIResponses:
		return nil
	case CodexGoalProtocolOpenAIChat:
		return nil
	}
	return nil
}

func codexGoalToolChoiceMode(raw any) string {
	switch value := raw.(type) {
	case nil:
		return "auto"
	case string:
		mode := strings.ToLower(strings.TrimSpace(value))
		if mode == "" {
			return "auto"
		}
		return mode
	case map[string]any:
		if len(value) == 0 {
			return "auto"
		}
		if mode := strings.ToLower(strings.TrimSpace(codexGoalStringFromAny(value["type"]))); mode != "" {
			return mode
		}
		return "forced"
	default:
		if text := strings.ToLower(strings.TrimSpace(codexGoalStringFromAny(raw))); text != "" {
			return text
		}
		return "auto"
	}
}

func isCodexGoalForcedToolChoice(mode string, raw any) bool {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" || mode == "auto" || mode == "none" {
		return false
	}
	if _, ok := raw.(map[string]any); ok {
		return true
	}
	return mode == "required" || mode == "any" || mode == "tool" || mode == "function" || strings.HasPrefix(mode, "mcp") || strings.HasPrefix(mode, "web_search")
}

func nonEmptyArrayFromAny(raw any) bool {
	values, ok := raw.([]any)
	return ok && len(values) > 0
}

func codexGoalToolChoiceFromAny(raw any) CodexGoalToolChoice {
	mode := codexGoalToolChoiceMode(raw)
	choice := CodexGoalToolChoice{Mode: mode}
	if m, ok := raw.(map[string]any); ok {
		choice.Name = strings.TrimSpace(codexGoalStringFromAny(m["name"]))
		if choice.Name == "" {
			if fn, ok := m["function"].(map[string]any); ok {
				choice.Name = strings.TrimSpace(codexGoalStringFromAny(fn["name"]))
			}
		}
	}
	return choice
}

func codexGoalGeminiToolChoice(payload map[string]any) CodexGoalToolChoice {
	toolConfig, _ := codexGoalFirstPresent(payload["toolConfig"], payload["tool_config"]).(map[string]any)
	if toolConfig == nil {
		return CodexGoalToolChoice{Mode: "auto"}
	}
	functionCallingConfig, _ := codexGoalFirstPresent(toolConfig["functionCallingConfig"], toolConfig["function_calling_config"]).(map[string]any)
	if functionCallingConfig == nil {
		return CodexGoalToolChoice{Mode: "auto"}
	}
	mode := strings.ToLower(strings.TrimSpace(codexGoalStringFromAny(functionCallingConfig["mode"])))
	if mode == "" {
		mode = "auto"
	}
	choice := CodexGoalToolChoice{Mode: mode}
	allowedNames := stringSliceFromAny(codexGoalFirstPresent(functionCallingConfig["allowedFunctionNames"], functionCallingConfig["allowed_function_names"]))
	if len(allowedNames) == 1 && isCodexGoalGeminiForcedToolChoice(mode) {
		choice.Mode = "function"
		choice.Name = allowedNames[0]
	}
	return choice
}

func isCodexGoalGeminiForcedToolChoice(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "any", "required", "function", "tool":
		return true
	default:
		return false
	}
}

func codexGoalGeminiAllowedFunctionNames(payload map[string]any) []string {
	toolConfig, _ := codexGoalFirstPresent(payload["toolConfig"], payload["tool_config"]).(map[string]any)
	if toolConfig == nil {
		return nil
	}
	functionCallingConfig, _ := codexGoalFirstPresent(toolConfig["functionCallingConfig"], toolConfig["function_calling_config"]).(map[string]any)
	if functionCallingConfig == nil {
		return nil
	}
	return stringSliceFromAny(codexGoalFirstPresent(functionCallingConfig["allowedFunctionNames"], functionCallingConfig["allowed_function_names"]))
}

func filterCodexGoalFunctionToolsByName(tools []CodexGoalFunctionToolConfig, allowedNames []string) ([]CodexGoalFunctionToolConfig, []string) {
	if len(allowedNames) == 0 {
		return tools, nil
	}
	allowed := make(map[string]struct{}, len(allowedNames))
	var ordered []string
	for _, name := range allowedNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, exists := allowed[name]; !exists {
			ordered = append(ordered, name)
		}
		allowed[name] = struct{}{}
	}
	if len(allowed) == 0 {
		return tools, nil
	}
	var filtered []CodexGoalFunctionToolConfig
	found := make(map[string]struct{}, len(allowed))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if _, ok := allowed[name]; !ok {
			continue
		}
		filtered = append(filtered, tool)
		found[name] = struct{}{}
	}
	var missing []string
	for _, name := range ordered {
		if _, ok := found[name]; !ok {
			missing = append(missing, name)
		}
	}
	return filtered, missing
}

func isCodexGoalWebSearchTool(toolType string) bool {
	toolType = strings.ToLower(strings.TrimSpace(toolType))
	return toolType == "web_search" || strings.HasPrefix(toolType, "web_search_preview")
}

func codexGoalFunctionToolFromTool(tool map[string]any) (CodexGoalFunctionToolConfig, error) {
	name := strings.TrimSpace(codexGoalStringFromAny(tool["name"]))
	description := strings.TrimSpace(codexGoalStringFromAny(tool["description"]))
	parameters := tool["parameters"]
	strict := boolFromAny(tool["strict"])
	if parameters == nil {
		parameters = tool["input_schema"]
	}
	if nested, ok := tool["function"].(map[string]any); ok {
		if name == "" {
			name = strings.TrimSpace(codexGoalStringFromAny(nested["name"]))
		}
		if description == "" {
			description = strings.TrimSpace(codexGoalStringFromAny(nested["description"]))
		}
		if parameters == nil {
			parameters = nested["parameters"]
		}
		if !strict {
			strict = boolFromAny(nested["strict"])
		}
	}
	if name == "" {
		return CodexGoalFunctionToolConfig{}, codexGoalBridgeError(400, "codex_goal_function_tool_name_required", "Responses function tools require a name")
	}
	parametersJSON := compactCodexGoalJSON(parameters)
	if parametersJSON == "" {
		parametersJSON = `{"type":"object","properties":{}}`
	}
	return CodexGoalFunctionToolConfig{
		Name:           name,
		Description:    description,
		ParametersJSON: parametersJSON,
		Strict:         strict,
	}, nil
}

func codexGoalFunctionToolExists(name string, tools []CodexGoalFunctionToolConfig) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == name {
			return true
		}
	}
	return false
}

func codexGoalHostedToolChoiceExists(choice CodexGoalToolChoice, features CodexGoalRequestFeatures) bool {
	name := strings.TrimSpace(choice.Name)
	if name == "" {
		return false
	}
	if features.EnableWebSearch && isCodexGoalWebSearchTool(strings.ToLower(name)) {
		return true
	}
	for _, server := range features.MCPServers {
		if len(server.EnabledTools) == 0 {
			return true
		}
		if codexGoalStringSliceContains(server.EnabledTools, name) {
			return true
		}
	}
	return false
}

func codexGoalHostedToolChoiceSection(features CodexGoalRequestFeatures, forced bool) string {
	if !forced || (!features.EnableWebSearch && len(features.MCPServers) == 0) {
		return ""
	}
	if features.ToolChoice.Name != "" && codexGoalFunctionToolExists(features.ToolChoice.Name, features.FunctionTools) {
		return ""
	}
	var b strings.Builder
	b.WriteString("Tool choice for hosted/MCP tools:\n")
	if features.ToolChoice.Name != "" {
		b.WriteString("- The client explicitly selected tool ")
		b.WriteString(features.ToolChoice.Name)
		b.WriteString(". Use that hosted/MCP tool before writing the final answer.\n")
	} else {
		b.WriteString("- The client requires at least one enabled hosted or MCP tool call before the final answer.\n")
	}
	if features.EnableWebSearch {
		b.WriteString("- web_search is enabled and may satisfy this required tool choice when live web information is needed.\n")
	}
	for _, server := range features.MCPServers {
		if strings.TrimSpace(server.Label) == "" {
			continue
		}
		b.WriteString("- MCP server ")
		b.WriteString(server.Label)
		if len(server.EnabledTools) > 0 {
			b.WriteString(" exposes allowed tools: ")
			b.WriteString(strings.Join(server.EnabledTools, ", "))
		}
		b.WriteString(".\n")
	}
	return strings.TrimSpace(b.String())
}

func codexGoalStringSliceContains(values []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func codexGoalFunctionToolSection(tools []CodexGoalFunctionToolConfig, choice CodexGoalToolChoice) string {
	var b strings.Builder
	b.WriteString("Available client function tools:\n")
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(tool.Name)
		if tool.Description != "" {
			b.WriteString(": ")
			b.WriteString(tool.Description)
		}
		if tool.Strict {
			b.WriteString(" (strict schema)")
		}
		b.WriteString("\n  parameters: ")
		b.WriteString(tool.ParametersJSON)
		b.WriteString("\n")
	}
	switch choice.Mode {
	case "required", "any":
		b.WriteString("\nTool choice: you must return one function call instead of a final assistant message.\n")
	case "function", "tool", "forced":
		if choice.Name != "" {
			b.WriteString("\nTool choice: you must call function ")
			b.WriteString(choice.Name)
			b.WriteString(".\n")
		}
	}
	b.WriteString("\nIf a client function should be called, do not invent the function result and do not answer the user's task directly. Return only this exact XML-wrapped JSON shape, with arguments as a JSON object matching the selected function schema:\n")
	b.WriteString(`<codex_goal_function_call>{"name":"function_name","arguments":{}}</codex_goal_function_call>`)
	b.WriteString("\nIf no function call is needed, answer normally without this wrapper.")
	return strings.TrimSpace(b.String())
}

func codexGoalMCPServerFromTool(index int, tool map[string]any) (CodexGoalMCPServerConfig, string, error) {
	if strings.TrimSpace(codexGoalStringFromAny(tool["connector_id"])) != "" {
		return CodexGoalMCPServerConfig{}, "", codexGoalBridgeError(400, "codex_goal_mcp_connector_unsupported", "Codex goal bridge cannot preserve OpenAI connector_id MCP tools")
	}
	label := strings.TrimSpace(codexGoalStringFromAny(tool["server_label"]))
	if label == "" {
		label = fmt.Sprintf("mcp_%d", index+1)
	}
	serverURL := strings.TrimSpace(codexGoalStringFromAny(tool["server_url"]))
	if serverURL == "" {
		return CodexGoalMCPServerConfig{}, "", codexGoalBridgeError(400, "codex_goal_mcp_server_url_required", "Codex goal bridge MCP tools require server_url")
	}
	parsed, err := url.Parse(serverURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return CodexGoalMCPServerConfig{}, "", codexGoalBridgeError(400, "codex_goal_mcp_server_url_unsupported", "Codex goal bridge only accepts https:// remote MCP server_url values from API requests")
	}
	requireApproval := strings.TrimSpace(strings.ToLower(codexGoalStringFromAny(tool["require_approval"])))
	if requireApproval != "" && requireApproval != "never" {
		return CodexGoalMCPServerConfig{}, "", codexGoalBridgeError(400, "codex_goal_mcp_approval_unsupported", "Codex goal bridge only supports MCP tools with require_approval set to never")
	}
	command := "mcp-remote"
	if strings.Contains(strings.ToLower(parsed.Path), "sse") {
		command = "codex-mcp-sse-proxy"
	}
	server := CodexGoalMCPServerConfig{
		Command:        command,
		Args:           []string{serverURL},
		Label:          label,
		URL:            serverURL,
		Description:    strings.TrimSpace(codexGoalStringFromAny(tool["server_description"])),
		EnabledTools:   stringSliceFromAny(tool["allowed_tools"]),
		ApprovalMode:   "approve",
		StartupTimeout: 60,
	}
	if authorization := strings.TrimSpace(codexGoalStringFromAny(tool["authorization"])); authorization != "" {
		if !strings.Contains(authorization, " ") {
			authorization = "Bearer " + authorization
		}
		server.HTTPHeaders = map[string]string{"Authorization": authorization}
	}
	if headers, ok := tool["headers"].(map[string]any); ok {
		if server.HTTPHeaders == nil {
			server.HTTPHeaders = map[string]string{}
		}
		for key, value := range headers {
			if strings.TrimSpace(key) == "" {
				continue
			}
			if text := strings.TrimSpace(codexGoalStringFromAny(value)); text != "" {
				server.HTTPHeaders[key] = text
			}
		}
	}
	for _, key := range sortedStringKeys(server.HTTPHeaders) {
		server.Args = append(server.Args, "--header", key+": "+server.HTTPHeaders[key])
	}
	var line strings.Builder
	line.WriteString("Available remote MCP server:\n- ")
	line.WriteString(label)
	if server.Description != "" {
		line.WriteString(": ")
		line.WriteString(server.Description)
	}
	line.WriteString(" (configured for this Codex goal run)")
	if len(server.EnabledTools) > 0 {
		line.WriteString("\n  Allowed tools: ")
		line.WriteString(strings.Join(server.EnabledTools, ", "))
	}
	return server, line.String(), nil
}

type CodexAppServerGoalRunner struct{}

func (CodexAppServerGoalRunner) RunGoal(ctx context.Context, input CodexGoalRunInput) (*CodexGoalRunResult, error) {
	return (CodexAppServerGoalRunner{}).RunGoalStream(ctx, input, nil)
}

func (CodexAppServerGoalRunner) RunGoalStream(ctx context.Context, input CodexGoalRunInput, onDelta CodexGoalDeltaSink) (*CodexGoalRunResult, error) {
	if err := validateCodexGoalRunnerInput(input); err != nil {
		return nil, err
	}
	if input.RequiredVersion != "" {
		if err := verifyCodexVersion(ctx, input.Command, input.RequiredVersion); err != nil {
			return nil, err
		}
	}

	codexHome, err := os.MkdirTemp("", "sub2api-codex-goal-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary CODEX_HOME: %w", err)
	}
	defer os.RemoveAll(codexHome)
	authJSONPath := filepath.Join(codexHome, "auth.json")
	if err := os.WriteFile(authJSONPath, input.AuthJSON, 0600); err != nil {
		return nil, fmt.Errorf("write temporary codex auth.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(codexGoalConfigTOML(input)), 0600); err != nil {
		return nil, fmt.Errorf("write temporary codex config.toml: %w", err)
	}
	cwd := input.CWD
	if !filepath.IsAbs(cwd) {
		if abs, err := filepath.Abs(cwd); err == nil {
			cwd = abs
		}
	}

	args := []string{}
	if input.EnableWebSearch {
		args = append(args, "--search")
	}
	args = append(args, "--disable", "apps", "--disable", "plugins")
	args = append(args, "app-server", "--listen", "stdio://", "--enable", "goals")
	cmd := exec.CommandContext(ctx, input.Command, args...)
	cmd.Env = append(os.Environ(), "CODEX_HOME="+codexHome)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open codex app-server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open codex app-server stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("open codex app-server stderr: %w", err)
	}
	stderrDone := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(io.LimitReader(stderr, 64*1024))
		stderrDone <- string(data)
	}()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex app-server: %w", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	session := newCodexAppServerSession(stdin, stdout)
	if _, err := session.sendAndWait(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "sub2api-codex-goal-bridge",
			"title":   "sub2api Codex Goal Bridge",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{
			"experimentalApi":           true,
			"optOutNotificationMethods": []string{},
		},
	}); err != nil {
		return nil, withCodexStderr("initialize codex app-server", err, stderrDone)
	}
	sandbox, threadConfig := codexGoalThreadStartSandboxAndConfig(input)
	threadResult, err := session.sendAndWait(ctx, "thread/start", map[string]any{
		"model":          input.Model,
		"cwd":            cwd,
		"approvalPolicy": "never",
		"sandbox":        sandbox,
		"ephemeral":      false,
		"config":         threadConfig,
		"threadSource":   "user",
	})
	if err != nil {
		return nil, withCodexStderr("start codex thread", err, stderrDone)
	}
	threadID := stringFromNested(threadResult, "thread", "id")
	if threadID == "" {
		return nil, fmt.Errorf("start codex thread: response did not include thread.id")
	}
	goalID, err := session.send("thread/goal/set", map[string]any{
		"threadId":  threadID,
		"objective": input.Objective,
	})
	if err != nil {
		return nil, withCodexStderr("set codex goal", err, stderrDone)
	}
	var text strings.Builder
	var toolEvents []CodexGoalToolEvent
	allowedMCPServers := codexGoalAllowedMCPServers(input.MCPServers)
	goalResponseSeen := false
	turnCompleted := false
	for !goalResponseSeen || !turnCompleted {
		msg, err := session.read(ctx)
		if err != nil {
			return nil, withCodexStderr("wait for codex goal completion", err, stderrDone)
		}
		if msg.ID == goalID {
			if msg.Error != nil {
				return nil, withCodexStderr("set codex goal", errors.New(msg.Error.Message), stderrDone)
			}
			goalResponseSeen = true
			continue
		}
		if msg.Method == "item/agentMessage/delta" {
			var params struct {
				Delta string `json:"delta"`
			}
			_ = json.Unmarshal(msg.Params, &params)
			text.WriteString(params.Delta)
			if onDelta != nil && params.Delta != "" {
				if err := onDelta(params.Delta); err != nil {
					return nil, err
				}
			}
			continue
		}
		if msg.Method == "item/completed" {
			before := len(toolEvents)
			appendCodexGoalCompletedToolEvent(msg.Params, &toolEvents, input.EnableWebSearch, allowedMCPServers)
			if input.ToolEventSink != nil {
				for _, event := range toolEvents[before:] {
					if err := input.ToolEventSink(event); err != nil {
						return nil, err
					}
				}
			}
			continue
		}
		if msg.Method == "turn/completed" {
			turnCompleted = true
			if err := appendCompletedTurnText(msg.Params, &text); err != nil {
				return nil, withCodexStderr("codex turn failed", err, stderrDone)
			}
		}
	}
	return &CodexGoalRunResult{
		Text:                 text.String(),
		ToolEvents:           toolEvents,
		RefreshedCredentials: readCodexGoalRefreshedCredentials(authJSONPath),
	}, nil
}

func validateCodexGoalRunnerInput(input CodexGoalRunInput) error {
	if strings.TrimSpace(input.Command) == "" {
		return fmt.Errorf("codex command is empty")
	}
	if strings.TrimSpace(input.Objective) == "" {
		return fmt.Errorf("codex objective is empty")
	}
	if len(input.AuthJSON) == 0 {
		return fmt.Errorf("codex auth JSON is empty")
	}
	if strings.TrimSpace(input.CWD) == "" {
		return fmt.Errorf("codex cwd is empty")
	}
	if strings.TrimSpace(input.Model) == "" {
		return fmt.Errorf("codex model is empty")
	}
	return nil
}

func codexGoalThreadStartSandboxAndConfig(input CodexGoalRunInput) (string, map[string]any) {
	config := map[string]any{
		"features": map[string]any{"goals": true},
	}
	if !codexGoalNeedsMCPAdapterNetwork(input.MCPServers) {
		return "read-only", config
	}
	config["sandbox_workspace_write"] = map[string]any{
		"network_access": true,
	}
	return "workspace-write", config
}

func codexGoalNeedsMCPAdapterNetwork(servers []CodexGoalMCPServerConfig) bool {
	for _, server := range servers {
		if strings.TrimSpace(server.Command) != "" {
			return true
		}
	}
	return false
}

func verifyCodexVersion(ctx context.Context, command, required string) error {
	cmd := exec.CommandContext(ctx, command, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("check codex version: %w: %s", err, strings.TrimSpace(string(output)))
	}
	versionText := strings.TrimSpace(string(output))
	if !strings.Contains(versionText, required) {
		return fmt.Errorf("codex version %q does not match required version %q", versionText, required)
	}
	return nil
}

func codexGoalConfigTOML(input CodexGoalRunInput) string {
	var b strings.Builder
	b.WriteString("disable_response_storage = true\n")
	b.WriteString("model = ")
	b.WriteString(strconv.Quote(input.Model))
	b.WriteString("\n")
	if input.ReasoningEffort != "" {
		b.WriteString("model_reasoning_effort = ")
		b.WriteString(strconv.Quote(input.ReasoningEffort))
		b.WriteString("\n")
	}
	b.WriteString("\n[features]\ngoals = true\napps = false\nplugins = false\n")
	for _, server := range input.MCPServers {
		if strings.TrimSpace(server.Label) == "" || (strings.TrimSpace(server.URL) == "" && strings.TrimSpace(server.Command) == "") {
			continue
		}
		b.WriteString("\n[mcp_servers.")
		b.WriteString(strconv.Quote(server.Label))
		b.WriteString("]\n")
		if strings.TrimSpace(server.Command) != "" {
			b.WriteString("command = ")
			b.WriteString(strconv.Quote(strings.TrimSpace(server.Command)))
			b.WriteString("\n")
			if len(server.Args) > 0 {
				b.WriteString("args = ")
				b.WriteString(tomlStringArray(server.Args))
				b.WriteString("\n")
			}
		} else {
			b.WriteString("url = ")
			b.WriteString(strconv.Quote(server.URL))
			b.WriteString("\n")
		}
		if len(server.EnabledTools) > 0 {
			b.WriteString("enabled_tools = ")
			b.WriteString(tomlStringArray(server.EnabledTools))
			b.WriteString("\n")
		}
		approvalMode := strings.TrimSpace(server.ApprovalMode)
		if approvalMode == "" {
			approvalMode = "approve"
		}
		b.WriteString("default_tools_approval_mode = ")
		b.WriteString(strconv.Quote(approvalMode))
		b.WriteString("\n")
		if server.StartupTimeout > 0 {
			b.WriteString("startup_timeout_sec = ")
			b.WriteString(strconv.Itoa(server.StartupTimeout))
			b.WriteString("\n")
		}
		if server.ToolTimeoutSec > 0 {
			b.WriteString("tool_timeout_sec = ")
			b.WriteString(strconv.Itoa(server.ToolTimeoutSec))
			b.WriteString("\n")
		}
		if len(server.HTTPHeaders) > 0 && strings.TrimSpace(server.Command) == "" {
			b.WriteString("http_headers = ")
			b.WriteString(tomlStringMap(server.HTTPHeaders))
			b.WriteString("\n")
		}
	}
	return b.String()
}

type codexAppServerSession struct {
	encoder *json.Encoder
	scanner *bufio.Scanner
	nextID  int
}

type codexAppServerMessage struct {
	ID     int
	Method string
	Params json.RawMessage
	Result json.RawMessage
	Error  *codexAppServerError
}

type codexAppServerError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newCodexAppServerSession(stdin io.Writer, stdout io.Reader) *codexAppServerSession {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	return &codexAppServerSession{
		encoder: json.NewEncoder(stdin),
		scanner: scanner,
	}
}

func (s *codexAppServerSession) send(method string, params any) (int, error) {
	s.nextID++
	msg := map[string]any{
		"id":     s.nextID,
		"method": method,
		"params": params,
	}
	if err := s.encoder.Encode(msg); err != nil {
		return 0, err
	}
	return s.nextID, nil
}

func (s *codexAppServerSession) sendAndWait(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id, err := s.send(method, params)
	if err != nil {
		return nil, err
	}
	for {
		msg, err := s.read(ctx)
		if err != nil {
			return nil, err
		}
		if msg.ID != id {
			continue
		}
		if msg.Error != nil {
			return nil, errors.New(msg.Error.Message)
		}
		return msg.Result, nil
	}
}

func (s *codexAppServerSession) read(ctx context.Context) (codexAppServerMessage, error) {
	type rawMessage struct {
		ID     json.RawMessage      `json:"id"`
		Method string               `json:"method"`
		Params json.RawMessage      `json:"params"`
		Result json.RawMessage      `json:"result"`
		Error  *codexAppServerError `json:"error"`
	}
	if ctx.Err() != nil {
		return codexAppServerMessage{}, ctx.Err()
	}
	if !s.scanner.Scan() {
		if err := s.scanner.Err(); err != nil {
			return codexAppServerMessage{}, err
		}
		if ctx.Err() != nil {
			return codexAppServerMessage{}, ctx.Err()
		}
		return codexAppServerMessage{}, io.EOF
	}
	var raw rawMessage
	if err := json.Unmarshal(s.scanner.Bytes(), &raw); err != nil {
		return codexAppServerMessage{}, fmt.Errorf("decode codex app-server message: %w", err)
	}
	return codexAppServerMessage{
		ID:     rawCodexID(raw.ID),
		Method: raw.Method,
		Params: raw.Params,
		Result: raw.Result,
		Error:  raw.Error,
	}, nil
}

func rawCodexID(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		n, _ = strconv.Atoi(s)
		return n
	}
	return 0
}

func appendCompletedTurnText(params json.RawMessage, out *strings.Builder) error {
	var payload struct {
		Turn struct {
			Status string `json:"status"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
			Items []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"items"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(params, &payload); err != nil {
		return nil
	}
	if payload.Turn.Status == "failed" {
		if payload.Turn.Error != nil && payload.Turn.Error.Message != "" {
			return errors.New(payload.Turn.Error.Message)
		}
		return errors.New("codex turn failed")
	}
	if out.Len() == 0 {
		for _, item := range payload.Turn.Items {
			if item.Type == "agentMessage" && strings.TrimSpace(item.Text) != "" {
				if out.Len() > 0 {
					out.WriteString("\n\n")
				}
				out.WriteString(item.Text)
			}
		}
	}
	return nil
}

func appendCodexGoalCompletedToolEvent(params json.RawMessage, out *[]CodexGoalToolEvent, allowWebSearch bool, allowedMCPServers map[string]struct{}) {
	var payload struct {
		Item struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			Query     string          `json:"query"`
			Action    json.RawMessage `json:"action"`
			Server    string          `json:"server"`
			Tool      string          `json:"tool"`
			Arguments any             `json:"arguments"`
			Status    string          `json:"status"`
			Error     *struct {
				Message string `json:"message"`
			} `json:"error"`
			Result *struct {
				Content           []any `json:"content"`
				StructuredContent any   `json:"structuredContent"`
			} `json:"result"`
		} `json:"item"`
	}
	if err := json.Unmarshal(params, &payload); err != nil {
		return
	}
	switch payload.Item.Type {
	case "webSearch":
		if !allowWebSearch {
			return
		}
		*out = append(*out, CodexGoalToolEvent{
			Type:   "web_search_call",
			ID:     payload.Item.ID,
			Query:  payload.Item.Query,
			Action: cloneJSONRawMessage(payload.Item.Action),
		})
	case "mcpToolCall":
		if _, ok := allowedMCPServers[payload.Item.Server]; !ok {
			return
		}
		event := CodexGoalToolEvent{
			Type:        "mcp_call",
			ID:          payload.Item.ID,
			Status:      payload.Item.Status,
			ServerLabel: payload.Item.Server,
			Name:        payload.Item.Tool,
			Arguments:   compactCodexGoalJSON(payload.Item.Arguments),
		}
		if payload.Item.Error != nil {
			event.Error = payload.Item.Error.Message
		}
		if payload.Item.Result != nil {
			event.Output = codexGoalMCPResultText(payload.Item.Result.Content, payload.Item.Result.StructuredContent)
		}
		*out = append(*out, event)
	}
}

func codexGoalAllowedMCPServers(servers []CodexGoalMCPServerConfig) map[string]struct{} {
	allowed := make(map[string]struct{}, len(servers))
	for _, server := range servers {
		if label := strings.TrimSpace(server.Label); label != "" {
			allowed[label] = struct{}{}
		}
	}
	return allowed
}

func codexGoalMCPResultText(content []any, structured any) string {
	var texts []string
	for _, item := range content {
		m, ok := item.(map[string]any)
		if !ok {
			if text := strings.TrimSpace(codexGoalStringFromAny(item)); text != "" {
				texts = append(texts, text)
			}
			continue
		}
		if text := strings.TrimSpace(codexGoalStringFromAny(m["text"])); text != "" {
			texts = append(texts, text)
		} else if text := contentText(m); text != "" {
			texts = append(texts, text)
		}
	}
	if len(texts) > 0 {
		return strings.Join(texts, "\n")
	}
	return compactCodexGoalJSON(structured)
}

func cloneJSONRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	copied := make(json.RawMessage, len(raw))
	copy(copied, raw)
	return copied
}

func parseCodexGoalFunctionCallOutput(raw string, tools []CodexGoalFunctionToolConfig, createdAt time.Time) (string, []CodexGoalFunctionCall) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(tools) == 0 {
		return raw, nil
	}
	allowed := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if name := strings.TrimSpace(tool.Name); name != "" {
			allowed[name] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return raw, nil
	}
	jsonText, before, after, ok := codexGoalFunctionCallEnvelope(raw)
	if !ok {
		return raw, nil
	}
	calls := codexGoalFunctionCallsFromJSON(jsonText, allowed, createdAt)
	if len(calls) == 0 {
		return raw, nil
	}
	return strings.TrimSpace(strings.Join([]string{before, after}, "\n\n")), calls
}

type codexGoalFunctionCallStreamParser struct {
	raw             strings.Builder
	allowed         map[string]struct{}
	tools           []CodexGoalFunctionToolConfig
	createdAt       time.Time
	emit            func(CodexGoalBridgeStreamEvent) error
	startEmitted    bool
	doneEmitted     bool
	emittedArgsLen  int
	functionCall    CodexGoalFunctionCall
	functionCallIdx int
}

func newCodexGoalFunctionCallStreamParser(tools []CodexGoalFunctionToolConfig, createdAt time.Time, emit func(CodexGoalBridgeStreamEvent) error) *codexGoalFunctionCallStreamParser {
	allowed := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if name := strings.TrimSpace(tool.Name); name != "" {
			allowed[name] = struct{}{}
		}
	}
	return &codexGoalFunctionCallStreamParser{
		allowed:   allowed,
		tools:     append([]CodexGoalFunctionToolConfig(nil), tools...),
		createdAt: createdAt,
		emit:      emit,
	}
}

func (p *codexGoalFunctionCallStreamParser) Write(delta string) error {
	if p == nil || p.emit == nil || delta == "" || len(p.allowed) == 0 || p.doneEmitted {
		return nil
	}
	p.raw.WriteString(delta)
	inner, complete, ok := codexGoalFunctionCallStreamInner(p.raw.String())
	if !ok {
		return nil
	}
	if !p.startEmitted {
		call, ok := p.partialFunctionCall(inner)
		if !ok {
			return nil
		}
		p.functionCall = call
		p.startEmitted = true
		if err := p.emit(CodexGoalBridgeStreamEvent{
			Type:              CodexGoalBridgeStreamEventFunctionCallStart,
			FunctionCall:      call,
			FunctionCallIndex: p.functionCallIdx,
		}); err != nil {
			return err
		}
	}
	if p.startEmitted {
		if args, ok := codexGoalPartialJSONFieldRaw(inner, "arguments"); ok && len(args) > p.emittedArgsLen {
			argsDelta := args[p.emittedArgsLen:]
			p.emittedArgsLen = len(args)
			if err := p.emit(CodexGoalBridgeStreamEvent{
				Type:              CodexGoalBridgeStreamEventFunctionArgumentsDelta,
				Delta:             argsDelta,
				FunctionCall:      p.functionCall,
				FunctionCallIndex: p.functionCallIdx,
			}); err != nil {
				return err
			}
		}
	}
	if !complete {
		return nil
	}
	_, calls := parseCodexGoalFunctionCallOutput(p.raw.String(), p.tools, p.createdAt)
	if len(calls) > 0 {
		p.functionCall = calls[0]
		if !p.startEmitted {
			p.startEmitted = true
			if err := p.emit(CodexGoalBridgeStreamEvent{
				Type:              CodexGoalBridgeStreamEventFunctionCallStart,
				FunctionCall:      p.functionCall,
				FunctionCallIndex: p.functionCallIdx,
			}); err != nil {
				return err
			}
		}
		if args, ok := codexGoalPartialJSONFieldRaw(inner, "arguments"); ok && len(args) > p.emittedArgsLen {
			argsDelta := args[p.emittedArgsLen:]
			p.emittedArgsLen = len(args)
			if err := p.emit(CodexGoalBridgeStreamEvent{
				Type:              CodexGoalBridgeStreamEventFunctionArgumentsDelta,
				Delta:             argsDelta,
				FunctionCall:      p.functionCall,
				FunctionCallIndex: p.functionCallIdx,
			}); err != nil {
				return err
			}
		}
	}
	if !p.startEmitted {
		return nil
	}
	p.doneEmitted = true
	return p.emit(CodexGoalBridgeStreamEvent{
		Type:              CodexGoalBridgeStreamEventFunctionCallDone,
		FunctionCall:      p.functionCall,
		FunctionCallIndex: p.functionCallIdx,
	})
}

func (p *codexGoalFunctionCallStreamParser) partialFunctionCall(inner string) (CodexGoalFunctionCall, bool) {
	name, ok := codexGoalPartialJSONStringField(inner, "name")
	if !ok {
		return CodexGoalFunctionCall{}, false
	}
	name = strings.TrimSpace(name)
	if _, allowed := p.allowed[name]; !allowed {
		return CodexGoalFunctionCall{}, false
	}
	id, _ := codexGoalPartialJSONStringField(inner, "id")
	callID, _ := codexGoalPartialJSONStringField(inner, "call_id")
	if callID == "" {
		callID = fmt.Sprintf("call_codex_goal_%d_%d", p.createdAt.UnixNano(), p.functionCallIdx)
	}
	if id == "" {
		id = fmt.Sprintf("fc_codex_goal_%d_%d", p.createdAt.UnixNano(), p.functionCallIdx)
	}
	return CodexGoalFunctionCall{
		ID:     id,
		CallID: callID,
		Name:   name,
	}, true
}

func codexGoalFunctionCallStreamInner(raw string) (inner string, complete bool, ok bool) {
	const openTag = "<codex_goal_function_call>"
	const closeTag = "</codex_goal_function_call>"
	start := strings.Index(raw, openTag)
	if start < 0 {
		return "", false, false
	}
	inner = raw[start+len(openTag):]
	if end := strings.Index(inner, closeTag); end >= 0 {
		return inner[:end], true, true
	}
	return inner, false, true
}

func codexGoalPartialJSONStringField(raw, field string) (string, bool) {
	start, ok := codexGoalJSONFieldValueStart(raw, field)
	if !ok || start >= len(raw) {
		return "", false
	}
	if raw[start] != '"' {
		return "", false
	}
	var value string
	decoder := json.NewDecoder(strings.NewReader(raw[start:]))
	if err := decoder.Decode(&value); err != nil {
		return "", false
	}
	return value, true
}

func codexGoalPartialJSONFieldRaw(raw, field string) (string, bool) {
	start, ok := codexGoalJSONFieldValueStart(raw, field)
	if !ok || start >= len(raw) {
		return "", false
	}
	end, complete := codexGoalJSONValueEnd(raw[start:])
	if complete {
		return raw[start : start+end], true
	}
	return raw[start:], true
}

func codexGoalJSONFieldValueStart(raw, field string) (int, bool) {
	key := strconv.Quote(field)
	idx := strings.Index(raw, key)
	if idx < 0 {
		return 0, false
	}
	pos := idx + len(key)
	for pos < len(raw) && (raw[pos] == ' ' || raw[pos] == '\n' || raw[pos] == '\r' || raw[pos] == '\t') {
		pos++
	}
	if pos >= len(raw) || raw[pos] != ':' {
		return 0, false
	}
	pos++
	for pos < len(raw) && (raw[pos] == ' ' || raw[pos] == '\n' || raw[pos] == '\r' || raw[pos] == '\t') {
		pos++
	}
	if pos >= len(raw) {
		return 0, false
	}
	return pos, true
}

func codexGoalJSONValueEnd(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	switch raw[0] {
	case '{', '[':
		return codexGoalCompositeJSONValueEnd(raw)
	case '"':
		return codexGoalStringJSONValueEnd(raw)
	default:
		for i := 0; i < len(raw); i++ {
			switch raw[i] {
			case ',', '}', ']', '\n', '\r':
				if i == 0 {
					return 0, false
				}
				return i, true
			}
		}
		return len(raw), false
	}
}

func codexGoalCompositeJSONValueEnd(raw string) (int, bool) {
	var stack []byte
	inString := false
	escaped := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{', '[':
			stack = append(stack, ch)
		case '}', ']':
			if len(stack) == 0 {
				return 0, false
			}
			open := stack[len(stack)-1]
			if (open == '{' && ch != '}') || (open == '[' && ch != ']') {
				return 0, false
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i + 1, true
			}
		}
	}
	return len(raw), false
}

func codexGoalStringJSONValueEnd(raw string) (int, bool) {
	escaped := false
	for i := 1; i < len(raw); i++ {
		ch := raw[i]
		if escaped {
			escaped = false
			continue
		}
		switch ch {
		case '\\':
			escaped = true
		case '"':
			return i + 1, true
		}
	}
	return len(raw), false
}

func codexGoalFunctionCallEnvelope(raw string) (jsonText, before, after string, ok bool) {
	const openTag = "<codex_goal_function_call>"
	const closeTag = "</codex_goal_function_call>"
	start := strings.Index(raw, openTag)
	end := strings.LastIndex(raw, closeTag)
	if start >= 0 && end > start {
		before = strings.TrimSpace(raw[:start])
		jsonText = strings.TrimSpace(raw[start+len(openTag) : end])
		after = strings.TrimSpace(raw[end+len(closeTag):])
		return jsonText, before, after, true
	}
	if strings.HasPrefix(strings.TrimSpace(raw), "{") {
		return strings.TrimSpace(raw), "", "", true
	}
	return "", "", "", false
}

func codexGoalFunctionCallsFromJSON(jsonText string, allowed map[string]struct{}, createdAt time.Time) []CodexGoalFunctionCall {
	var payload any
	if err := json.Unmarshal([]byte(jsonText), &payload); err != nil {
		return nil
	}
	var rawCalls []any
	if m, ok := payload.(map[string]any); ok {
		if calls, ok := m["function_calls"].([]any); ok {
			rawCalls = calls
		} else if call, ok := m["function_call"]; ok {
			rawCalls = []any{call}
		} else {
			rawCalls = []any{m}
		}
	} else if calls, ok := payload.([]any); ok {
		rawCalls = calls
	}
	var calls []CodexGoalFunctionCall
	for _, rawCall := range rawCalls {
		m, ok := rawCall.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(codexGoalStringFromAny(m["name"]))
		if name == "" {
			if fn, ok := m["function"].(map[string]any); ok {
				name = strings.TrimSpace(codexGoalStringFromAny(fn["name"]))
				if _, exists := m["arguments"]; !exists {
					m["arguments"] = fn["arguments"]
				}
			}
		}
		if _, ok := allowed[name]; !ok {
			continue
		}
		arguments := codexGoalFunctionArguments(m["arguments"])
		if arguments == "" {
			arguments = "{}"
		}
		index := len(calls)
		callID := strings.TrimSpace(codexGoalStringFromAny(m["call_id"]))
		if callID == "" {
			callID = fmt.Sprintf("call_codex_goal_%d_%d", createdAt.UnixNano(), index)
		}
		id := strings.TrimSpace(codexGoalStringFromAny(m["id"]))
		if id == "" {
			id = fmt.Sprintf("fc_codex_goal_%d_%d", createdAt.UnixNano(), index)
		}
		calls = append(calls, CodexGoalFunctionCall{
			ID:        id,
			CallID:    callID,
			Name:      name,
			Arguments: arguments,
		})
	}
	return calls
}

func codexGoalFunctionArguments(raw any) string {
	switch value := raw.(type) {
	case nil:
		return ""
	case string:
		text := strings.TrimSpace(value)
		if text == "" {
			return ""
		}
		var js any
		if err := json.Unmarshal([]byte(text), &js); err == nil {
			return compactCodexGoalJSON(js)
		}
		return strconv.Quote(text)
	default:
		return compactCodexGoalJSON(value)
	}
}

func withCodexStderr(prefix string, err error, stderrDone <-chan string) error {
	select {
	case stderr := <-stderrDone:
		stderr = strings.TrimSpace(stderr)
		if stderr != "" {
			return fmt.Errorf("%s: %w: %s", prefix, err, stderr)
		}
	default:
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

func extractOpenAIResponsesObjective(payload map[string]any) string {
	var sections []string
	if instructions := contentText(payload["instructions"]); instructions != "" {
		sections = append(sections, "Instructions:\n"+instructions)
	}
	if input := inputSequenceText(payload["input"]); input != "" {
		sections = append(sections, input)
	}
	return strings.Join(sections, "\n\n")
}

func extractOpenAIChatObjective(payload map[string]any) string {
	return openAIChatMessagesText(payload["messages"])
}

func extractAnthropicMessagesObjective(payload map[string]any) string {
	var sections []string
	if system := contentText(payload["system"]); system != "" {
		sections = append(sections, "System:\n"+system)
	}
	if messages := messagesText(payload["messages"]); messages != "" {
		sections = append(sections, messages)
	}
	return strings.Join(sections, "\n\n")
}

func extractGeminiGenerateContentObjective(payload map[string]any) string {
	var sections []string
	if system := geminiContentText(payload["system_instruction"]); system != "" {
		sections = append(sections, "System:\n"+system)
	}
	if contents := geminiContentsText(payload["contents"]); contents != "" {
		sections = append(sections, contents)
	}
	return strings.Join(sections, "\n\n")
}

func messagesText(value any) string {
	items, ok := value.([]any)
	if !ok {
		return contentText(value)
	}
	var lines []string
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			if text := contentText(item); text != "" {
				lines = append(lines, text)
			}
			continue
		}
		role := strings.TrimSpace(codexGoalStringFromAny(m["role"]))
		text := contentText(m["content"])
		if text == "" {
			text = contentText(m["text"])
		}
		if text == "" {
			continue
		}
		if role != "" {
			lines = append(lines, strings.Title(role)+":\n"+text)
		} else {
			lines = append(lines, text)
		}
	}
	return strings.Join(lines, "\n\n")
}

func openAIChatMessagesText(value any) string {
	items, ok := value.([]any)
	if !ok {
		return contentText(value)
	}
	var lines []string
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			if text := contentText(item); text != "" {
				lines = append(lines, text)
			}
			continue
		}
		role := strings.TrimSpace(codexGoalStringFromAny(m["role"]))
		switch role {
		case "assistant":
			if text := contentText(m["content"]); text != "" {
				lines = append(lines, "Assistant:\n"+text)
			}
			if calls := openAIChatToolCallBlocks(m); len(calls) > 0 {
				lines = append(lines, calls...)
			}
		case "tool":
			callID := strings.TrimSpace(codexGoalStringFromAny(m["tool_call_id"]))
			output := contentText(m["content"])
			if output == "" {
				output = contentText(m["text"])
			}
			if block := codexGoalLabeledBlock("Tool result", []string{
				codexGoalLabelValue("call_id", callID),
				codexGoalLabelValue("output", output),
			}); block != "" {
				lines = append(lines, block)
			}
		case "function":
			name := strings.TrimSpace(codexGoalStringFromAny(m["name"]))
			output := contentText(m["content"])
			if block := codexGoalLabeledBlock("Function result", []string{
				codexGoalLabelValue("name", name),
				codexGoalLabelValue("output", output),
			}); block != "" {
				lines = append(lines, block)
			}
		default:
			text := contentText(m["content"])
			if text == "" {
				text = contentText(m["text"])
			}
			if text == "" {
				continue
			}
			if role != "" {
				lines = append(lines, strings.Title(role)+":\n"+text)
			} else {
				lines = append(lines, text)
			}
		}
	}
	return strings.Join(lines, "\n\n")
}

func openAIChatToolCallBlocks(message map[string]any) []string {
	var blocks []string
	if toolCalls, ok := message["tool_calls"].([]any); ok {
		for _, rawCall := range toolCalls {
			call, ok := rawCall.(map[string]any)
			if !ok {
				continue
			}
			function, _ := call["function"].(map[string]any)
			name := strings.TrimSpace(codexGoalStringFromAny(function["name"]))
			arguments := strings.TrimSpace(codexGoalStringFromAny(function["arguments"]))
			if arguments == "" {
				arguments = compactCodexGoalJSON(function["arguments"])
			}
			callID := strings.TrimSpace(codexGoalStringFromAny(call["id"]))
			if block := codexGoalLabeledBlock("Prior assistant function call", []string{
				codexGoalLabelValue("name", name),
				codexGoalLabelValue("call_id", callID),
				codexGoalLabelValue("arguments", arguments),
			}); block != "" {
				blocks = append(blocks, block)
			}
		}
	}
	if functionCall, ok := message["function_call"].(map[string]any); ok {
		name := strings.TrimSpace(codexGoalStringFromAny(functionCall["name"]))
		arguments := strings.TrimSpace(codexGoalStringFromAny(functionCall["arguments"]))
		if arguments == "" {
			arguments = compactCodexGoalJSON(functionCall["arguments"])
		}
		if block := codexGoalLabeledBlock("Prior assistant function call", []string{
			codexGoalLabelValue("name", name),
			codexGoalLabelValue("arguments", arguments),
		}); block != "" {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

func inputSequenceText(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var lines []string
		for _, item := range v {
			if text := inputItemText(item); text != "" {
				lines = append(lines, text)
			}
		}
		return strings.Join(lines, "\n\n")
	default:
		return inputItemText(value)
	}
}

func inputItemText(value any) string {
	m, ok := value.(map[string]any)
	if !ok {
		return contentText(value)
	}
	itemType := strings.TrimSpace(codexGoalStringFromAny(m["type"]))
	switch itemType {
	case "input_image", "image_url", "input_file", "file", "image", "document":
		return codexGoalMultimodalPlaceholderFromMap(m)
	case "function_call":
		name := strings.TrimSpace(codexGoalStringFromAny(m["name"]))
		callID := strings.TrimSpace(codexGoalStringFromAny(m["call_id"]))
		arguments := strings.TrimSpace(codexGoalStringFromAny(m["arguments"]))
		if arguments == "" {
			arguments = contentText(m["arguments"])
		}
		return codexGoalLabeledBlock("Prior assistant function call", []string{
			codexGoalLabelValue("name", name),
			codexGoalLabelValue("call_id", callID),
			codexGoalLabelValue("arguments", arguments),
		})
	case "function_call_output":
		callID := strings.TrimSpace(codexGoalStringFromAny(m["call_id"]))
		output := codexGoalFirstNonEmpty(contentText(m["output"]), contentText(m["content"]))
		return codexGoalLabeledBlock("Tool result", []string{
			codexGoalLabelValue("call_id", callID),
			codexGoalLabelValue("output", output),
		})
	case "mcp_call":
		name := strings.TrimSpace(codexGoalStringFromAny(m["name"]))
		serverLabel := strings.TrimSpace(codexGoalStringFromAny(m["server_label"]))
		output := codexGoalFirstNonEmpty(contentText(m["output"]), contentText(m["error"]))
		return codexGoalLabeledBlock("Prior MCP tool call", []string{
			codexGoalLabelValue("server", serverLabel),
			codexGoalLabelValue("name", name),
			codexGoalLabelValue("output", output),
		})
	case "mcp_list_tools":
		serverLabel := strings.TrimSpace(codexGoalStringFromAny(m["server_label"]))
		toolsJSON := compactCodexGoalJSON(m["tools"])
		return codexGoalLabeledBlock("Prior MCP tool list", []string{
			codexGoalLabelValue("server", serverLabel),
			codexGoalLabelValue("tools", toolsJSON),
		})
	case "web_search_call":
		action := compactCodexGoalJSON(m["action"])
		return codexGoalLabeledBlock("Prior web search call", []string{
			codexGoalLabelValue("action", action),
		})
	}
	role := strings.TrimSpace(codexGoalStringFromAny(m["role"]))
	text := contentText(m["content"])
	if text == "" {
		text = codexGoalFirstNonEmpty(contentText(m["text"]), contentText(m["input"]))
	}
	if text == "" {
		return ""
	}
	if role != "" {
		return strings.Title(role) + ":\n" + text
	}
	return text
}

func contentText(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, item := range v {
			if text := contentText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		itemType := strings.TrimSpace(codexGoalStringFromAny(v["type"]))
		switch itemType {
		case "input_image", "image_url", "input_file", "file", "image", "document":
			return codexGoalMultimodalPlaceholderFromMap(v)
		case "tool_use":
			return codexGoalLabeledBlock("Prior assistant tool call", []string{
				codexGoalLabelValue("name", codexGoalStringFromAny(v["name"])),
				codexGoalLabelValue("call_id", codexGoalStringFromAny(v["id"])),
				codexGoalLabelValue("arguments", compactCodexGoalJSON(v["input"])),
			})
		case "tool_result":
			return codexGoalLabeledBlock("Tool result", []string{
				codexGoalLabelValue("call_id", codexGoalStringFromAny(v["tool_use_id"])),
				codexGoalLabelValue("output", contentText(v["content"])),
			})
		}
		if text := codexGoalMultimodalPlaceholderFromMap(v); text != "" {
			return text
		}
		if text := codexGoalStringFromAny(v["text"]); text != "" {
			return strings.TrimSpace(text)
		}
		if text := codexGoalStringFromAny(v["input_text"]); text != "" {
			return strings.TrimSpace(text)
		}
		if text := contentText(v["content"]); text != "" {
			return text
		}
		return ""
	default:
		text := codexGoalStringFromAny(v)
		return strings.TrimSpace(text)
	}
}

func geminiContentsText(value any) string {
	items, ok := value.([]any)
	if !ok {
		return geminiContentText(value)
	}
	var lines []string
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			if text := geminiContentText(item); text != "" {
				lines = append(lines, text)
			}
			continue
		}
		role := strings.TrimSpace(codexGoalStringFromAny(m["role"]))
		text := geminiContentText(m)
		if text == "" {
			continue
		}
		if role != "" {
			lines = append(lines, strings.Title(role)+":\n"+text)
		} else {
			lines = append(lines, text)
		}
	}
	return strings.Join(lines, "\n\n")
}

func geminiContentText(value any) string {
	m, ok := value.(map[string]any)
	if !ok {
		return contentText(value)
	}
	if parts, ok := m["parts"].([]any); ok {
		var texts []string
		for _, part := range parts {
			if text := geminiPartText(part); text != "" {
				texts = append(texts, text)
			}
		}
		return strings.Join(texts, "\n")
	}
	return contentText(value)
}

func geminiPartText(value any) string {
	m, ok := value.(map[string]any)
	if !ok {
		return contentText(value)
	}
	if functionCall, ok := codexGoalFirstPresent(m["functionCall"], m["function_call"]).(map[string]any); ok {
		name := strings.TrimSpace(codexGoalStringFromAny(functionCall["name"]))
		callID := strings.TrimSpace(codexGoalStringFromAny(functionCall["id"]))
		arguments := compactCodexGoalJSON(codexGoalFirstPresent(functionCall["args"], functionCall["arguments"]))
		return codexGoalLabeledBlock("Prior assistant function call", []string{
			codexGoalLabelValue("name", name),
			codexGoalLabelValue("call_id", callID),
			codexGoalLabelValue("arguments", arguments),
		})
	}
	if functionResponse, ok := codexGoalFirstPresent(m["functionResponse"], m["function_response"]).(map[string]any); ok {
		name := strings.TrimSpace(codexGoalStringFromAny(functionResponse["name"]))
		callID := strings.TrimSpace(codexGoalStringFromAny(functionResponse["id"]))
		output := compactCodexGoalJSON(functionResponse["response"])
		if output == "" {
			output = contentText(functionResponse["content"])
		}
		return codexGoalLabeledBlock("Tool result", []string{
			codexGoalLabelValue("name", name),
			codexGoalLabelValue("call_id", callID),
			codexGoalLabelValue("output", output),
		})
	}
	return contentText(value)
}

func decodeCodexGoalPayload(body []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	return nil
}

func codexGoalStringFromAny(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func boolFromAny(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

func stringSliceFromAny(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if text := strings.TrimSpace(codexGoalStringFromAny(value)); text != "" {
			return []string{text}
		}
		return nil
	}
	var out []string
	for _, item := range items {
		if text := strings.TrimSpace(codexGoalStringFromAny(item)); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func tomlStringArray(values []string) string {
	var parts []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			parts = append(parts, strconv.Quote(value))
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func tomlStringMap(values map[string]string) string {
	keys := sortedStringKeys(values)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(values[key])
		if value == "" {
			continue
		}
		parts = append(parts, strconv.Quote(key)+" = "+strconv.Quote(value))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func codexGoalLabelValue(label, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return label + ": " + value
}

func codexGoalLabeledBlock(title string, lines []string) string {
	var nonEmpty []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmpty = append(nonEmpty, line)
		}
	}
	if len(nonEmpty) == 0 {
		return ""
	}
	return title + ":\n" + strings.Join(nonEmpty, "\n")
}

func compactCodexGoalJSON(value any) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func extraString(extra map[string]any, key string) string {
	if extra == nil {
		return ""
	}
	return strings.TrimSpace(codexGoalStringFromAny(extra[key]))
}

func codexGoalFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func codexGoalFirstPresent(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func geminiModelFromEndpoint(endpoint string) string {
	endpoint = strings.TrimPrefix(endpoint, "/")
	if endpoint == "" {
		return ""
	}
	if idx := strings.LastIndex(endpoint, "/"); idx >= 0 {
		endpoint = endpoint[idx+1:]
	}
	if idx := strings.Index(endpoint, ":"); idx >= 0 {
		endpoint = endpoint[:idx]
	}
	return strings.TrimSpace(endpoint)
}

func stringFromNested(raw json.RawMessage, path ...string) string {
	var current any
	if err := json.Unmarshal(raw, &current); err != nil {
		return ""
	}
	for _, key := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = m[key]
	}
	return strings.TrimSpace(codexGoalStringFromAny(current))
}
