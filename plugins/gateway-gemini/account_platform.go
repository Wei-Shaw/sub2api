package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

const (
	// defaultTestModel is the default model for testing Gemini accounts.
	defaultTestModel = "gemini-2.5-flash"

	// aiStudioBaseURL is the Google AI Studio generativelanguage API base.
	aiStudioBaseURL = "https://generativelanguage.googleapis.com"

	// Re-exported account-type identifiers from pluginsdk.
	accountTypeOAuth          = pluginsdk.AccountTypeOAuth
	accountTypeAPIKey         = pluginsdk.AccountTypeAPIKey
	accountTypeServiceAccount = pluginsdk.AccountTypeServiceAccount

	// vertexDefaultLocation is the default GCP region for Vertex AI.
	vertexDefaultLocation = "us-central1"

	// defaultTextTestPrompt is the default prompt for text model tests.
	defaultTextTestPrompt = "hi"

	// defaultImageTestPrompt is the default prompt for image model tests.
	defaultImageTestPrompt = "Generate a cute orange cat astronaut sticker on a clean pastel background."
)

// vertexLocationPattern validates GCP location format.
var vertexLocationPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// accountPlatformServer implements the AccountPlatformExtension gRPC service.
type accountPlatformServer struct {
	pb.UnimplementedAccountPlatformExtensionServer
	httpClient *http.Client
}

func newAccountPlatformServer() *accountPlatformServer {
	return &accountPlatformServer{
		httpClient: &http.Client{},
	}
}

// ValidateAccountData validates Gemini credentials before the host persists.
func (s *accountPlatformServer) ValidateAccountData(
	_ context.Context,
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
	case accountTypeServiceAccount:
		saJSON := gatewayutil.CredStr(creds, "service_account_json")
		if saJSON == "" {
			errs["credentials.service_account_json"] = "service_account_json is required"
			break
		}
		var sa map[string]any
		if err := json.Unmarshal([]byte(saJSON), &sa); err != nil {
			errs["credentials.service_account_json"] = "service_account_json must be valid JSON"
		}
	}
	return errs
}

// TestConnection performs a connectivity test against the Gemini API.
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

	httpReq, err := s.buildTestRequest(stream.Context(), req.GetAccountType(), model, creds)
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

	return processGeminiStream(stream, resp.Body)
}

// ExecuteCustomAction handles plugin-defined custom actions.
// Currently no custom actions are defined for Gemini.
func (s *accountPlatformServer) ExecuteCustomAction(
	_ context.Context,
	req *pb.ExecuteCustomActionRequest,
) (*pb.ExecuteCustomActionResponse, error) {
	return &pb.ExecuteCustomActionResponse{
		Success: false,
		Error:   "gateway-gemini: unknown action: " + req.GetActionId(),
	}, nil
}

// --- credential helpers ---

type geminiCredentials struct {
	AccessToken        string `json:"access_token"`
	RefreshToken       string `json:"refresh_token"`
	APIKey             string `json:"api_key"`
	BaseURL            string `json:"base_url"`
	OAuthType          string `json:"oauth_type"`
	ProjectID          string `json:"project_id"`
	ServiceAccountJSON string `json:"service_account_json"`
	VertexProjectID    string `json:"vertex_project_id"`
	VertexLocation     string `json:"vertex_location"`
	ClientID           string `json:"client_id"`
	ClientSecret       string `json:"client_secret"`
	TierID             string `json:"tier_id"`
}

func parseCredentials(raw []byte) (*geminiCredentials, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty credentials")
	}
	var c geminiCredentials
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// --- request building ---

// buildTestRequest constructs the HTTP request for testing a Gemini account.
func (s *accountPlatformServer) buildTestRequest(
	ctx context.Context,
	accountType, model string,
	creds *geminiCredentials,
) (*http.Request, error) {
	payload := createGeminiTestPayload(model)

	switch accountType {
	case accountTypeAPIKey:
		return buildAPIKeyRequest(ctx, model, creds, payload)
	case accountTypeOAuth:
		return buildOAuthRequest(ctx, model, creds, payload)
	case accountTypeServiceAccount:
		return buildServiceAccountRequest(ctx, model, creds, payload)
	default:
		return nil, fmt.Errorf("unsupported account type: %s", accountType)
	}
}

func buildAPIKeyRequest(
	ctx context.Context, model string, creds *geminiCredentials, payload []byte,
) (*http.Request, error) {
	apiKey := strings.TrimSpace(creds.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("no API key available")
	}

	baseURL := strings.TrimSpace(creds.BaseURL)
	if baseURL == "" {
		baseURL = aiStudioBaseURL
	}
	fullURL := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse",
		strings.TrimRight(baseURL, "/"), model)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)
	return req, nil
}

func buildOAuthRequest(
	ctx context.Context, model string, creds *geminiCredentials, payload []byte,
) (*http.Request, error) {
	accessToken := strings.TrimSpace(creds.AccessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("no access token available")
	}

	baseURL := strings.TrimSpace(creds.BaseURL)
	if baseURL == "" {
		baseURL = aiStudioBaseURL
	}
	fullURL := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse",
		strings.TrimRight(baseURL, "/"), model)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	return req, nil
}

func buildServiceAccountRequest(
	ctx context.Context, model string, creds *geminiCredentials, payload []byte,
) (*http.Request, error) {
	accessToken := strings.TrimSpace(creds.AccessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("no access token available for service account")
	}

	fullURL, err := buildVertexGeminiURL(creds, model)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	return req, nil
}

// buildVertexGeminiURL constructs the Vertex AI Gemini API URL.
func buildVertexGeminiURL(creds *geminiCredentials, model string) (string, error) {
	projectID := strings.TrimSpace(creds.VertexProjectID)
	if projectID == "" {
		return "", fmt.Errorf("vertex_project_id is required for service accounts")
	}

	location := strings.TrimSpace(creds.VertexLocation)
	if location == "" {
		location = vertexDefaultLocation
	}
	if !vertexLocationPattern.MatchString(location) {
		return "", fmt.Errorf("invalid vertex location: %s", location)
	}

	host := fmt.Sprintf("%s-aiplatform.googleapis.com", location)
	if location == "global" {
		host = "aiplatform.googleapis.com"
	}

	u := fmt.Sprintf(
		"https://%s/v1/projects/%s/locations/%s/publishers/google/models/%s:streamGenerateContent?alt=sse",
		host,
		url.PathEscape(projectID),
		url.PathEscape(location),
		url.PathEscape(model),
	)
	return u, nil
}
