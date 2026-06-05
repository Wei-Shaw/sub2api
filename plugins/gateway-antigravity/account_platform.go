package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	"io"
	"net/http"
	"strings"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

const (
	// defaultTestModel is the default model for testing Antigravity accounts.
	defaultTestModel = "claude-sonnet-4-5"

	// antigravityProdBaseURL is the Antigravity API production endpoint.
	antigravityProdBaseURL = "https://cloudcode-pa.googleapis.com"

	// Re-exported account-type identifiers from pluginsdk.
	accountTypeOAuth    = pluginsdk.AccountTypeOAuth
	accountTypeAPIKey   = pluginsdk.AccountTypeAPIKey
	accountTypeUpstream = pluginsdk.AccountTypeUpstream

	// Google OAuth token endpoint for token refresh.
	googleTokenURL = "https://oauth2.googleapis.com/token"

	// Google OAuth client ID for Antigravity.
	googleClientID = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
)

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

// ValidateAccountData validates Antigravity credentials before the host persists.
// For OAuth accounts: requires access_token or refresh_token.
// For API key accounts: requires api_key.
// For upstream accounts: requires base_url and api_key.
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
	case accountTypeAPIKey:
		if gatewayutil.CredStr(creds, "api_key") == "" {
			errs["credentials.api_key"] = "api_key is required"
		}
	case accountTypeUpstream:
		if gatewayutil.CredStr(creds, "api_key") == "" {
			errs["credentials.api_key"] = "upstream account requires api_key"
		}
		if gatewayutil.CredStr(creds, "base_url") == "" {
			errs["credentials.base_url"] = "upstream account requires base_url"
		}
	}
	return errs
}

// TestConnection performs a connectivity test against the Antigravity API.
// For OAuth accounts: calls the Antigravity v1internal streamGenerateContent
// endpoint with a minimal Gemini-format request.
// For API key accounts: routes to Claude or Gemini test based on model prefix.
// For upstream accounts: routes to Claude or Gemini test based on model prefix.
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

	_ = stream.Send(&pb.TestConnectionEvent{Type: "test_start", Model: model})

	switch req.GetAccountType() {
	case accountTypeOAuth:
		return s.testOAuthConnection(stream, creds, model)
	case accountTypeAPIKey, accountTypeUpstream:
		return s.testAPIKeyConnection(stream, req.GetAccountType(), creds, model)
	default:
		return gatewayutil.SendErrorEnd(stream, "unsupported account type: "+req.GetAccountType())
	}
}

// testOAuthConnection tests an OAuth account by calling the Antigravity
// v1internal streamGenerateContent endpoint with a minimal request.
func (s *accountPlatformServer) testOAuthConnection(
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	creds *antigravityCredentials,
	model string,
) error {
	accessToken := strings.TrimSpace(creds.AccessToken)
	if accessToken == "" {
		return gatewayutil.SendErrorEnd(stream, "no access token available")
	}

	projectID := strings.TrimSpace(creds.ProjectID)

	payload := buildAntigravityTestPayload(projectID, model)
	body, _ := json.Marshal(payload)

	apiURL := antigravityProdBaseURL + "/v1internal:streamGenerateContent?alt=sse"
	httpReq, err := http.NewRequestWithContext(
		stream.Context(), http.MethodPost, apiURL, bytes.NewReader(body),
	)
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, "failed to create request: "+err.Error())
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("User-Agent", antigravityUserAgent)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, "request failed: "+err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return gatewayutil.SendErrorEnd(stream, fmt.Sprintf(
			"API returned %d: %s", resp.StatusCode, string(respBody),
		))
	}

	return processAntigravityStream(stream, resp.Body)
}

// testAPIKeyConnection tests an API key or upstream account.
// Claude models go to the Anthropic Messages API; Gemini models go to
// the Gemini generateContent endpoint. This mirrors routeAntigravityTest
// in the host.
func (s *accountPlatformServer) testAPIKeyConnection(
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	accountType string,
	creds *antigravityCredentials,
	model string,
) error {
	apiKey := strings.TrimSpace(creds.APIKey)
	if apiKey == "" {
		return gatewayutil.SendErrorEnd(stream, "no API key available")
	}

	baseURL := strings.TrimSpace(creds.BaseURL)

	if strings.HasPrefix(model, "gemini-") {
		return s.testGeminiAPIKey(stream, apiKey, baseURL, model)
	}
	return s.testClaudeAPIKey(stream, apiKey, baseURL, model)
}

// testClaudeAPIKey tests a Claude model via the Anthropic Messages API.
func (s *accountPlatformServer) testClaudeAPIKey(
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	apiKey, baseURL, model string,
) error {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	apiURL := strings.TrimSuffix(baseURL, "/") + "/v1/messages"

	payload := map[string]any{
		"model":      model,
		"max_tokens": 16,
		"stream":     true,
		"messages": []map[string]any{
			{"role": "user", "content": "hi"},
		},
	}
	body, _ := json.Marshal(payload)

	httpReq, err := http.NewRequestWithContext(
		stream.Context(), http.MethodPost, apiURL, bytes.NewReader(body),
	)
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, "failed to create request: "+err.Error())
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Api-Key", apiKey)
	httpReq.Header.Set("Anthropic-Version", "2023-06-01")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, "request failed: "+err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return gatewayutil.SendErrorEnd(stream, fmt.Sprintf(
			"API returned %d: %s", resp.StatusCode, string(respBody),
		))
	}

	return processClaudeStream(stream, resp.Body)
}

// testGeminiAPIKey tests a Gemini model via the generateContent API.
func (s *accountPlatformServer) testGeminiAPIKey(
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	apiKey, baseURL, model string,
) error {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	apiURL := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse",
		strings.TrimSuffix(baseURL, "/"), model)

	payload := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": "hi"},
				},
			},
		},
		"generationConfig": map[string]any{
			"maxOutputTokens": 16,
		},
	}
	body, _ := json.Marshal(payload)

	httpReq, err := http.NewRequestWithContext(
		stream.Context(), http.MethodPost, apiURL, bytes.NewReader(body),
	)
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, "failed to create request: "+err.Error())
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", apiKey)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return gatewayutil.SendErrorEnd(stream, "request failed: "+err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return gatewayutil.SendErrorEnd(stream, fmt.Sprintf(
			"API returned %d: %s", resp.StatusCode, string(respBody),
		))
	}

	return processGeminiStream(stream, resp.Body)
}

// --- credential helpers ---

// antigravityCredentials holds the parsed credential fields relevant to testing.
type antigravityCredentials struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	APIKey       string `json:"api_key"`
	BaseURL      string `json:"base_url"`
	ProjectID    string `json:"project_id"`
	Email        string `json:"email"`
	ClientSecret string `json:"client_secret"`
}

func parseCredentials(raw []byte) (*antigravityCredentials, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty credentials")
	}
	var c antigravityCredentials
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// RefreshTier refreshes account tier/plan info from the Antigravity API.
// Calls loadCodeAssist to retrieve the current tier and plan_type.
// TODO: implement full tier refresh via loadCodeAssist endpoint.
func (s *accountPlatformServer) RefreshTier(
	ctx context.Context,
	req *pb.RefreshTierRequest,
) (*pb.RefreshTierResponse, error) {
	return &pb.RefreshTierResponse{
		Success: false,
		Error:   "gateway-antigravity: tier refresh not yet implemented",
	}, nil
}

// ExecuteCustomAction handles plugin-defined custom actions.
// Currently no custom actions are defined for Antigravity.
func (s *accountPlatformServer) ExecuteCustomAction(
	ctx context.Context,
	req *pb.ExecuteCustomActionRequest,
) (*pb.ExecuteCustomActionResponse, error) {
	return &pb.ExecuteCustomActionResponse{
		Success: false,
		Error:   "gateway-antigravity: unknown action: " + req.GetActionId(),
	}, nil
}
