package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	"io"
	"net/http"
	"regexp"
	"strings"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

const (
	// defaultTestModel is the default model for testing OpenAI accounts.
	defaultTestModel = "gpt-4o"

	// chatgptCodexAPIURL is the ChatGPT internal API for OAuth accounts.
	chatgptCodexAPIURL = "https://chatgpt.com/backend-api/codex/responses"

	// defaultOpenAIBaseURL is the default base URL for API key accounts.
	defaultOpenAIBaseURL = "https://api.openai.com"

	// defaultInstructions is a minimal instruction for the test payload.
	defaultInstructions = "You are a helpful assistant."

	// Re-exported account-type identifiers from pluginsdk.
	accountTypeOAuth      = pluginsdk.AccountTypeOAuth
	accountTypeSetupToken = pluginsdk.AccountTypeSetupToken
	accountTypeAPIKey     = pluginsdk.AccountTypeAPIKey
	accountTypeUpstream   = pluginsdk.AccountTypeUpstream

	// OpenAI OAuth constants — mirrored from backend/internal/pkg/openai/oauth.go.
	// The plugin cannot import host internal packages, so these are duplicated.
	openaiTokenURL     = "https://auth.openai.com/oauth/token"
	openaiDefaultCID   = "app_EMoamEEZ73f0CkXaXp7hrann"
	openaiRefreshScope = "openid profile email"
)

// sseDataPrefix matches SSE data lines with optional whitespace after colon.
var sseDataPrefix = regexp.MustCompile(`^data:\s*`)

// accountPlatformServer implements the AccountPlatformExtension gRPC service.
// It handles account-level operations delegated from the host: credential
// validation, test connection, token refresh, and model listing.
type accountPlatformServer struct {
	pb.UnimplementedAccountPlatformExtensionServer
	httpClient *http.Client
}

func newAccountPlatformServer() *accountPlatformServer {
	return &accountPlatformServer{
		httpClient: &http.Client{},
	}
}

// ValidateAccountData validates OpenAI credentials before the host persists.
// For OAuth accounts: requires access_token or refresh_token.
// For API key accounts: requires api_key with "sk-" prefix.
// For upstream accounts: requires api_key or base_url.
func (s *accountPlatformServer) ValidateAccountData(
	ctx context.Context,
	req *pb.ValidateAccountDataRequest,
) (*pb.ValidateAccountDataResponse, error) {
	var creds map[string]any
	if len(req.GetCredentialsJson()) > 0 {
		if err := json.Unmarshal(req.GetCredentialsJson(), &creds); err != nil {
			return &pb.ValidateAccountDataResponse{
				Valid:       false,
				FieldErrors: map[string]string{"credentials": "invalid JSON"},
			}, nil
		}
	}

	fieldErrors := validateCredentialsByType(req.GetAccountType(), creds)
	if len(fieldErrors) > 0 {
		return &pb.ValidateAccountDataResponse{
			Valid:       false,
			FieldErrors: fieldErrors,
		}, nil
	}

	return &pb.ValidateAccountDataResponse{Valid: true}, nil
}

// validateCredentialsByType checks required fields per account type.
func validateCredentialsByType(accountType string, creds map[string]any) map[string]string {
	errs := make(map[string]string)
	switch accountType {
	case accountTypeOAuth, accountTypeSetupToken:
		if gatewayutil.CredStr(creds, "access_token") == "" && gatewayutil.CredStr(creds, "refresh_token") == "" {
			errs["credentials.access_token"] = "oauth account requires access_token or refresh_token"
		}
	case accountTypeAPIKey:
		apiKey := gatewayutil.CredStr(creds, "api_key")
		if apiKey == "" {
			errs["credentials.api_key"] = "api_key is required"
		} else if !strings.HasPrefix(apiKey, "sk-") {
			errs["credentials.api_key"] = "api_key must start with sk-"
		}
	case accountTypeUpstream:
		if gatewayutil.CredStr(creds, "api_key") == "" && gatewayutil.CredStr(creds, "base_url") == "" {
			errs["credentials.api_key"] = "upstream account requires api_key or base_url"
		}
	}
	return errs
}

// TestConnection performs a connectivity test against the OpenAI API.
// It sends a minimal streaming request to the OpenAI Responses API and
// relays the SSE events back through the gRPC stream.
func (s *accountPlatformServer) TestConnection(
	req *pb.TestConnectionRequest,
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
) error {
	creds, err := parseCredentials(req.GetCredentialsJson())
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, "failed to parse credentials: "+err.Error())
	}

	model := req.GetModelId()
	if model == "" {
		model = defaultTestModel
	}

	authToken, apiURL, isOAuth, err := resolveAuth(req.GetAccountType(), creds)
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, err.Error())
	}

	_ = stream.Send(&pb.TestConnectionEvent{Type: "test_start", Model: model})

	httpReq, err := buildTestRequest(
		stream.Context(), model, isOAuth, authToken, apiURL, creds,
	)
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, "failed to create request: "+err.Error())
	}

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, "request failed: "+err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return gatewayutil.SendErrorEnd(stream, fmt.Sprintf(
			"API returned %d: %s", resp.StatusCode, string(body),
		))
	}

	return processOpenAIStream(stream, resp.Body)
}

// --- credential helpers ---

// openAICredentials holds the parsed credential fields relevant to testing.
type openAICredentials struct {
	AccessToken      string `json:"access_token"`
	APIKey           string `json:"api_key"`
	BaseURL          string `json:"base_url"`
	ChatGPTAccountID string `json:"chatgpt_account_id"`
}

func parseCredentials(raw []byte) (*openAICredentials, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty credentials")
	}
	var c openAICredentials
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// resolveAuth determines the auth token, API URL, and whether the account
// is OAuth-based.
func resolveAuth(
	accountType string, creds *openAICredentials,
) (authToken, apiURL string, isOAuth bool, err error) {
	switch accountType {
	case accountTypeOAuth, accountTypeSetupToken:
		isOAuth = true
		authToken = strings.TrimSpace(creds.AccessToken)
		if authToken == "" {
			return "", "", false, fmt.Errorf("no access token available")
		}
		apiURL = chatgptCodexAPIURL

	case accountTypeAPIKey:
		authToken = strings.TrimSpace(creds.APIKey)
		if authToken == "" {
			return "", "", false, fmt.Errorf("no API key available")
		}
		baseURL := strings.TrimSpace(creds.BaseURL)
		if baseURL == "" {
			baseURL = defaultOpenAIBaseURL
		}
		apiURL = strings.TrimSuffix(baseURL, "/") + "/v1/responses"

	default:
		return "", "", false, fmt.Errorf("unsupported account type: %s", accountType)
	}
	return authToken, apiURL, isOAuth, nil
}

// --- request building ---

func buildTestRequest(
	ctx context.Context,
	model string,
	isOAuth bool,
	authToken, apiURL string,
	creds *openAICredentials,
) (*http.Request, error) {
	payload := buildTestPayload(model, isOAuth)
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	if isOAuth {
		req.Host = "chatgpt.com"
		req.Header.Set("Accept", "text/event-stream")
		if id := strings.TrimSpace(creds.ChatGPTAccountID); id != "" {
			req.Header.Set("chatgpt-account-id", id)
		}
	}
	return req, nil
}

func buildTestPayload(model string, isOAuth bool) map[string]any {
	payload := map[string]any{
		"model": model,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "hi"},
				},
			},
		},
		"stream":       true,
		"instructions": defaultInstructions,
	}
	if isOAuth {
		payload["store"] = false
	}
	return payload
}

// --- SSE stream processing ---

// processOpenAIStream reads the SSE stream from the OpenAI Responses API and
// forwards events through the gRPC stream.
func processOpenAIStream(
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	body io.Reader,
) error {
	reader := bufio.NewReader(body)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				_ = stream.Send(&pb.TestConnectionEvent{
					Type:    "test_end",
					Success: true,
				})
				return nil
			}
			return gatewayutil.SendErrorEnd(stream, "stream read error: "+err.Error())
		}

		line = strings.TrimSpace(line)
		if line == "" || !sseDataPrefix.MatchString(line) {
			continue
		}

		jsonStr := sseDataPrefix.ReplaceAllString(line, "")
		if jsonStr == "[DONE]" {
			_ = stream.Send(&pb.TestConnectionEvent{
				Type:    "test_end",
				Success: true,
			})
			return nil
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		if handleSSEEvent(stream, data) {
			return nil
		}
	}
}

// handleSSEEvent processes a single parsed SSE event. Returns true if the
// stream should be terminated (completed or failed).
func handleSSEEvent(
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	data map[string]any,
) bool {
	eventType, _ := data["type"].(string)

	switch eventType {
	case "response.output_text.delta":
		if delta, ok := data["delta"].(string); ok && delta != "" {
			_ = stream.Send(&pb.TestConnectionEvent{
				Type: "content",
				Text: delta,
			})
		}

	case "response.completed", "response.done":
		_ = stream.Send(&pb.TestConnectionEvent{
			Type:    "test_end",
			Success: true,
		})
		return true

	case "response.failed":
		msg := extractFailedMessage(data)
		_ = gatewayutil.SendErrorEnd(stream, msg)
		return true

	case "error":
		msg := extractErrorMessage(data)
		_ = gatewayutil.SendErrorEnd(stream, msg)
		return true
	}
	return false
}

// --- error helpers ---

func extractFailedMessage(data map[string]any) string {
	if resp, ok := data["response"].(map[string]any); ok {
		if errData, ok := resp["error"].(map[string]any); ok {
			if msg, ok := errData["message"].(string); ok && msg != "" {
				return msg
			}
		}
	}
	return "OpenAI response failed"
}

func extractErrorMessage(data map[string]any) string {
	if errData, ok := data["error"].(map[string]any); ok {
		if msg, ok := errData["message"].(string); ok && msg != "" {
			return msg
		}
	}
	return "unknown error"
}

// RefreshTier refreshes account tier/plan info.
// OpenAI does not expose a standard tier API; this queries the ChatGPT
// backend-api for plan_type when OAuth accounts are used.
// TODO: implement via chatgpt.com/backend-api/me endpoint.
func (s *accountPlatformServer) RefreshTier(
	ctx context.Context,
	req *pb.RefreshTierRequest,
) (*pb.RefreshTierResponse, error) {
	return &pb.RefreshTierResponse{
		Success: false,
		Error:   "gateway-openai: tier refresh not yet implemented",
	}, nil
}

// ExecuteCustomAction handles plugin-defined custom actions.
// Currently no custom actions are defined for OpenAI.
func (s *accountPlatformServer) ExecuteCustomAction(
	ctx context.Context,
	req *pb.ExecuteCustomActionRequest,
) (*pb.ExecuteCustomActionResponse, error) {
	return &pb.ExecuteCustomActionResponse{
		Success: false,
		Error:   "gateway-openai: unknown action: " + req.GetActionId(),
	}, nil
}
