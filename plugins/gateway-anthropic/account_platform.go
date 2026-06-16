package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

const (
	// anthropicMessagesURL is the Anthropic Messages API endpoint.
	anthropicMessagesURL = "https://api.anthropic.com/v1/messages?beta=true"

	// anthropicAPIVersion is the required anthropic-version header value.
	anthropicAPIVersion = "2023-06-01"

	// defaultBetaHeader is the anthropic-beta header for OAuth accounts.
	defaultBetaHeader = "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14"

	// apiKeyBetaHeader is the anthropic-beta header for API key accounts.
	apiKeyBetaHeader = "claude-code-20250219,interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14"

	// Re-exported account-type identifiers from pluginsdk to keep the
	// rest of this package terse. The canonical definitions live in
	// plugin-sdk/account_type_constants.go.
	accountTypeOAuth      = pluginsdk.AccountTypeOAuth
	accountTypeSetupToken = pluginsdk.AccountTypeSetupToken
	accountTypeAPIKey     = pluginsdk.AccountTypeAPIKey
	accountTypeBedrock    = pluginsdk.AccountTypeBedrock

	// claudeCodeSystemPrompt is the minimal system prompt for test payloads.
	claudeCodeSystemPrompt = "You are Claude Code, Anthropic's official CLI for Claude."
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

// ValidateAccountData validates Anthropic credentials before the host persists.
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
	case accountTypeOAuth:
		if gatewayutil.CredStr(creds, "access_token") == "" && gatewayutil.CredStr(creds, "refresh_token") == "" {
			errs["credentials.access_token"] = "oauth account requires access_token or refresh_token"
		}
	case accountTypeSetupToken:
		if gatewayutil.CredStr(creds, "access_token") == "" && gatewayutil.CredStr(creds, "session_key") == "" && gatewayutil.CredStr(creds, "setup_token") == "" {
			errs["credentials.access_token"] = "setup-token account requires access_token, session_key, or setup_token"
		}
	case accountTypeAPIKey:
		if gatewayutil.CredStr(creds, "api_key") == "" {
			errs["credentials.api_key"] = "api_key is required"
		}
	case accountTypeBedrock:
		apiKey := gatewayutil.CredStr(creds, "api_key")
		accessKeyID := gatewayutil.CredStr(creds, "aws_access_key_id")
		secretKey := gatewayutil.CredStr(creds, "aws_secret_access_key")
		if apiKey == "" && (accessKeyID == "" || secretKey == "") {
			errs["credentials.aws_access_key_id"] = "bedrock requires api_key (cross-region) or aws_access_key_id + aws_secret_access_key"
		}
	case accountTypeServiceAccount:
		saJSON := gatewayutil.CredStr(creds, "service_account_json")
		if saJSON == "" {
			saJSON = gatewayutil.CredStr(creds, "service_account")
		}
		if saJSON == "" {
			// Also accept nested object form.
			if _, ok := creds["service_account_json"].(map[string]any); !ok {
				if _, ok := creds["service_account"].(map[string]any); !ok {
					errs["credentials.service_account_json"] = "service_account requires service_account_json"
				}
			}
		}
	}
	return errs
}

// TestConnection performs a connectivity test against the Anthropic Messages API.
// It sends a minimal streaming request and relays the SSE events back through
// the gRPC stream. Vertex accounts use a dedicated test path with Google
// OAuth2 token exchange and the Vertex rawPredict endpoint.
func (s *accountPlatformServer) TestConnection(
	req *pb.TestConnectionRequest,
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
) error {
	model := req.GetModelId()
	if model == "" {
		model = defaultTestModel
	}

	if req.GetAccountType() == accountTypeServiceAccount {
		return s.testVertexConnection(req, stream, model)
	}

	creds, err := parseCredentials(req.GetCredentialsJson())
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, "failed to parse credentials: "+err.Error())
	}

	authToken, apiURL, useBearer, err := resolveAuth(req.GetAccountType(), creds)
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, err.Error())
	}

	_ = stream.Send(&pb.TestConnectionEvent{Type: "test_start", Model: model})

	httpReq, err := buildTestRequest(
		stream.Context(), model, useBearer, authToken, apiURL,
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

	return processClaudeStream(stream, resp.Body)
}

// testVertexConnection performs a connectivity test against the Vertex AI
// endpoint. It exchanges the service account credentials for an access
// token and sends a minimal streaming request to the Vertex rawPredict API.
func (s *accountPlatformServer) testVertexConnection(
	req *pb.TestConnectionRequest,
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	model string,
) error {
	ctx := stream.Context()

	creds := parseVertexCredentialsRaw(req.GetCredentialsJson())

	// Exchange service account credentials for access token.
	// Use the account's proxy URL for the token endpoint request.
	accessToken, err := exchangeVertexServiceAccountToken(ctx, req.GetProxyUrl(), creds.serviceAccountJSON)
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, "token exchange failed: "+err.Error())
	}

	// Normalize model for Vertex.
	vertexModel := normalizeVertexAnthropicModelID(model)

	_ = stream.Send(&pb.TestConnectionEvent{Type: "test_start", Model: model})

	// Build test payload and transform for Vertex.
	payload := buildTestPayload(vertexModel)
	payloadBytes, _ := json.Marshal(payload)
	vertexBody, err := buildVertexAnthropicRequestBody(payloadBytes)
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, "failed to build vertex body: "+err.Error())
	}

	// Resolve project ID and location.
	projectID, err := creds.resolveProjectID()
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, "vertex credentials: "+err.Error())
	}
	location := creds.locationForModel(vertexModel)

	// Build URL (streaming test).
	targetURL, err := buildVertexAnthropicURL(projectID, location, vertexModel, true)
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, "failed to build vertex url: "+err.Error())
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(vertexBody))
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, "failed to create request: "+err.Error())
	}
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, "request failed: "+err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return gatewayutil.SendErrorEnd(stream, fmt.Sprintf(
			"Vertex API returned %d: %s", resp.StatusCode, string(body),
		))
	}

	return processClaudeStream(stream, resp.Body)
}

// --- credential helpers ---

// anthropicCredentials holds the parsed credential fields relevant to testing.
type anthropicCredentials struct {
	AccessToken string `json:"access_token"`
	APIKey      string `json:"api_key"`
	BaseURL     string `json:"base_url"`
}

func parseCredentials(raw []byte) (*anthropicCredentials, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty credentials")
	}
	var c anthropicCredentials
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// resolveAuth determines the auth token, API URL, and whether to use
// Bearer auth (OAuth) vs x-api-key (API key).
func resolveAuth(
	accountType string, creds *anthropicCredentials,
) (authToken, apiURL string, useBearer bool, err error) {
	switch accountType {
	case accountTypeOAuth, accountTypeSetupToken:
		useBearer = true
		authToken = strings.TrimSpace(creds.AccessToken)
		if authToken == "" {
			return "", "", false, fmt.Errorf("no access token available")
		}
		apiURL = anthropicMessagesURL

	case accountTypeAPIKey:
		authToken = strings.TrimSpace(creds.APIKey)
		if authToken == "" {
			return "", "", false, fmt.Errorf("no API key available")
		}
		baseURL := strings.TrimSpace(creds.BaseURL)
		if baseURL == "" {
			baseURL = "https://api.anthropic.com"
		}
		apiURL = strings.TrimSuffix(baseURL, "/") + "/v1/messages?beta=true"

	default:
		return "", "", false, fmt.Errorf("unsupported account type for test: %s", accountType)
	}
	return authToken, apiURL, useBearer, nil
}

// --- request building ---

func buildTestRequest(
	ctx context.Context,
	model string,
	useBearer bool,
	authToken, apiURL string,
) (*http.Request, error) {
	payload := buildTestPayload(model)
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	if useBearer {
		req.Header.Set("anthropic-beta", defaultBetaHeader)
		req.Header.Set("Authorization", "Bearer "+authToken)
	} else {
		req.Header.Set("anthropic-beta", apiKeyBetaHeader)
		req.Header.Set("x-api-key", authToken)
	}

	return req, nil
}

func buildTestPayload(model string) map[string]any {
	return map[string]any{
		"model": model,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "text",
						"text": "hi",
						"cache_control": map[string]string{
							"type": "ephemeral",
						},
					},
				},
			},
		},
		"system": []map[string]any{
			{
				"type": "text",
				"text": claudeCodeSystemPrompt,
				"cache_control": map[string]string{
					"type": "ephemeral",
				},
			},
		},
		"max_tokens":  1024,
		"temperature": 1,
		"stream":      true,
	}
}

// --- SSE stream processing ---

// processClaudeStream reads the SSE stream from the Anthropic Messages API and
// forwards events through the gRPC stream.
func processClaudeStream(
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

// handleSSEEvent processes a single parsed SSE event from the Anthropic
// Messages API. Returns true if the stream should be terminated.
func handleSSEEvent(
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	data map[string]any,
) bool {
	eventType, _ := data["type"].(string)

	switch eventType {
	case "content_block_delta":
		if delta, ok := data["delta"].(map[string]any); ok {
			if text, ok := delta["text"].(string); ok && text != "" {
				_ = stream.Send(&pb.TestConnectionEvent{
					Type: "content",
					Text: text,
				})
			}
		}

	case "message_stop":
		_ = stream.Send(&pb.TestConnectionEvent{
			Type:    "test_end",
			Success: true,
		})
		return true

	case "error":
		msg := extractErrorMessage(data)
		_ = gatewayutil.SendErrorEnd(stream, msg)
		return true
	}
	return false
}

// --- error helpers ---

func extractErrorMessage(data map[string]any) string {
	if errData, ok := data["error"].(map[string]any); ok {
		if msg, ok := errData["message"].(string); ok && msg != "" {
			return msg
		}
	}
	return "unknown error"
}

// RefreshTier refreshes account tier/plan info.
// Anthropic does not expose a standard tier API.
func (s *accountPlatformServer) RefreshTier(
	ctx context.Context,
	req *pb.RefreshTierRequest,
) (*pb.RefreshTierResponse, error) {
	return &pb.RefreshTierResponse{
		Success: false,
		Error:   "gateway-anthropic: tier refresh not supported",
	}, nil
}

// ExecuteCustomAction handles plugin-defined custom actions.
// Currently no custom actions are defined for Anthropic.
func (s *accountPlatformServer) ExecuteCustomAction(
	ctx context.Context,
	req *pb.ExecuteCustomActionRequest,
) (*pb.ExecuteCustomActionResponse, error) {
	return &pb.ExecuteCustomActionResponse{
		Success: false,
		Error:   "gateway-anthropic: unknown action: " + req.GetActionId(),
	}, nil
}
