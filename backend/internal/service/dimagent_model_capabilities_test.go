package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestParseDimAgentModelCapabilitiesMapsLiveMetadata(t *testing.T) {
	body := []byte(`{"object":"list","data":[{"id":"deepseek-v4.1-flash","dim":{"display_name":"DeepSeek V4.1 Flash","type":"chat","limit":{"context":1000000,"output":384000},"modalities":{"input":["text","image"],"output":["text"]},"tool_call":true,"structured_output":true,"reasoning":{"supported":true,"effort":"high","effortOptions":["high","max"]},"reasoning_options":[{"type":"effort","values":["low","high","max"]}]}}]}`)

	models, err := ParseDimAgentModelCapabilities(body)
	require.NoError(t, err)
	require.Len(t, models, 1)
	model := models[0]
	require.Equal(t, "deepseek-v4.1-flash", model.ID)
	require.Equal(t, "DeepSeek V4.1 Flash", model.DisplayName)
	require.Equal(t, 1000000, model.ContextWindow)
	require.Equal(t, 384000, model.MaxOutputTokens)
	require.Equal(t, []string{"text", "image"}, model.InputModalities)
	require.True(t, model.ToolCall)
	require.True(t, model.StructuredOutput)
	require.Equal(t, "high", model.Reasoning.DefaultLevel)
	require.Equal(t, []DimAgentReasoningEffort{
		{Level: "low", WireValue: "low"},
		{Level: "high", WireValue: "high"},
		{Level: "max", WireValue: "max"},
	}, model.Reasoning.Efforts)
}

func TestParseDimAgentModelCapabilitiesUsesOfficialFallback(t *testing.T) {
	body := []byte(`{"data":[{"id":"glm-5.3","dim":null},{"id":"seed-2.0-mini","dim":null},{"id":"seedream-5.0","dim":null},{"id":"unknown","dim":null}]}`)

	models, err := ParseDimAgentModelCapabilities(body)
	require.NoError(t, err)
	require.Len(t, models, 2, "image-only and unknown entries must not be exposed by a chat provider")
	require.Equal(t, DimAgentModelCapability{
		ID: "glm-5.3", DisplayName: "GLM-5.3", Protocol: "openai-completions",
		ContextWindow: 1000000, MaxOutputTokens: 131072, InputModalities: []string{"text"},
		Reasoning: &DimAgentReasoningCapability{Supported: true, DefaultLevel: "high", Efforts: []DimAgentReasoningEffort{{Level: "high", WireValue: "high"}, {Level: "max", WireValue: "max"}}},
	}, models[0])
	require.Equal(t, "seed-2.0-mini", models[1].ID)
	require.Equal(t, 128000, models[1].ContextWindow)
	require.Nil(t, models[1].Reasoning)
}

func TestParseDimAgentModelCapabilitiesMapsNoneToOff(t *testing.T) {
	body := []byte(`{"data":[{"id":"mimo-v2.6-flash","dim":{"name":"MiMo","limit":{"context":1048576,"output":131072},"reasoning":{"supported":true,"effortOptions":["none","high"]},"modalities":["text","image"]}}]}`)
	models, err := ParseDimAgentModelCapabilities(body)
	require.NoError(t, err)
	require.Equal(t, []DimAgentReasoningEffort{{Level: "off", WireValue: "none"}, {Level: "high", WireValue: "high"}}, models[0].Reasoning.Efforts)
}

func TestParseDimAgentModelCapabilitiesRejectsDuplicateOrEmpty(t *testing.T) {
	_, err := ParseDimAgentModelCapabilities([]byte(`{"data":[]}`))
	require.Error(t, err)
	_, err = ParseDimAgentModelCapabilities([]byte(`{"data":[{"id":"glm-5.3","dim":null},{"id":"glm-5.3","dim":null}]}`))
	require.Error(t, err)
}

func TestMergeDimAgentModelCapabilitiesUsesSafeIntersection(t *testing.T) {
	a := []DimAgentModelCapability{{
		ID: "m", DisplayName: "M", Protocol: "openai-completions", ContextWindow: 1000, MaxOutputTokens: 400,
		InputModalities: []string{"text", "image"}, ToolCall: true, StructuredOutput: true,
		Reasoning: &DimAgentReasoningCapability{Supported: true, DefaultLevel: "high", Efforts: []DimAgentReasoningEffort{{Level: "low", WireValue: "low"}, {Level: "high", WireValue: "high"}}},
	}}
	b := []DimAgentModelCapability{{
		ID: "m", DisplayName: "M2", Protocol: "openai-completions", ContextWindow: 800, MaxOutputTokens: 300,
		InputModalities: []string{"text"}, ToolCall: false, StructuredOutput: true,
		Reasoning: &DimAgentReasoningCapability{Supported: true, DefaultLevel: "low", Efforts: []DimAgentReasoningEffort{{Level: "high", WireValue: "high"}, {Level: "max", WireValue: "max"}}},
	}}

	merged, err := MergeDimAgentModelCapabilities([][]DimAgentModelCapability{a, b})
	require.NoError(t, err)
	require.Len(t, merged, 1)
	require.Equal(t, 800, merged[0].ContextWindow)
	require.Equal(t, 300, merged[0].MaxOutputTokens)
	require.Equal(t, []string{"text"}, merged[0].InputModalities)
	require.False(t, merged[0].ToolCall)
	require.True(t, merged[0].StructuredOutput)
	require.Empty(t, merged[0].Reasoning.DefaultLevel, "conflicting defaults must not be guessed")
	require.Equal(t, []DimAgentReasoningEffort{{Level: "high", WireValue: "high"}}, merged[0].Reasoning.Efforts)
}

func TestMergeDimAgentModelCapabilitiesDropsModelsNotCommonToEveryAccount(t *testing.T) {
	common := DimAgentModelCapability{ID: "common", DisplayName: "Common", Protocol: "openai-completions", ContextWindow: 100, MaxOutputTokens: 10, InputModalities: []string{"text"}}
	onlyA := DimAgentModelCapability{ID: "only-a", DisplayName: "Only A", Protocol: "openai-completions", ContextWindow: 100, MaxOutputTokens: 10, InputModalities: []string{"text"}}
	merged, err := MergeDimAgentModelCapabilities([][]DimAgentModelCapability{{common, onlyA}, {common}})
	require.NoError(t, err)
	require.Equal(t, []string{"common"}, dimAgentCapabilityIDs(merged))
}

func TestDimAgentCapabilityEnvelopeIsStableJSON(t *testing.T) {
	envelope := DimAgentModelCapabilitiesEnvelope{Object: "model_capability_list", CatalogVersion: "sha256:test", CatalogScope: DimAgentCatalogScope{Platform: PlatformDimAgent, GroupID: 11}, Data: []DimAgentModelCapability{{ID: "m", DisplayName: "M", Protocol: "openai-completions", ContextWindow: 1, MaxOutputTokens: 1, InputModalities: []string{"text"}}}}
	encoded, err := json.Marshal(envelope)
	require.NoError(t, err)
	require.JSONEq(t, `{"object":"model_capability_list","catalog_version":"sha256:test","catalog_scope":{"platform":"dimagent","group_id":11},"data":[{"id":"m","display_name":"M","protocol":"openai-completions","context_window":1,"max_output_tokens":1,"input_modalities":["text"],"tool_call":false,"structured_output":false}]}`, string(encoded))
	require.NotEqual(t, DimAgentCatalogVersion(11, envelope.Data), DimAgentCatalogVersion(12, envelope.Data), "catalog versions must be scoped to the authenticated group")
}

func dimAgentCapabilityTestConfig() *config.Config {
	return &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false, AllowInsecureHTTP: true}}}
}

func dimAgentCapabilityTestAccount() *Account {
	return &Account{
		ID: 901, Name: "dimagent-subscription", Platform: PlatformDimAgent,
		Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "dim-access-token", "refresh_token": "dim-refresh-token",
			"expires_at":     time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			"relay_base_url": "https://dimagent.example/v1",
		},
	}
}

type dimAgentCapabilityAccountRepoStub struct {
	AccountRepository
	accounts []Account
	err      error
}

func (s *dimAgentCapabilityAccountRepoStub) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	return append([]Account(nil), s.accounts...), s.err
}
func (s *dimAgentCapabilityAccountRepoStub) Update(context.Context, *Account) error { return nil }

func dimAgentCapabilityTestService(accounts []Account, response func(*http.Request, int64) (*http.Response, error)) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		accountRepo: &dimAgentCapabilityAccountRepoStub{accounts: accounts},
		cfg:         dimAgentCapabilityTestConfig(),
		httpUpstream: &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
			return response(req, accountID)
		}},
		dimAgentTokenProvider: &DimAgentTokenProvider{},
	}
}

type dimAgentCapabilityOAuthStub struct{}

func (s *dimAgentCapabilityOAuthStub) ExchangeCode(context.Context, string, string, string, string) (*DimAgentOAuthTokenResponse, error) {
	return nil, errors.New("not used")
}
func (s *dimAgentCapabilityOAuthStub) RefreshToken(context.Context, string, string, string) (*DimAgentOAuthTokenResponse, error) {
	return &DimAgentOAuthTokenResponse{AccessToken: "refreshed-token", RefreshToken: "refresh", TokenType: "Bearer", ExpiresIn: 3600}, nil
}

func TestFetchDimAgentModelsListRefreshesOnceAfterUnauthorized(t *testing.T) {
	account := dimAgentCapabilityTestAccount()
	repo := &dimAgentCapabilityAccountRepoStub{accounts: []Account{*account}}
	var calls atomic.Int32
	service := dimAgentCapabilityTestService([]Account{*account}, func(req *http.Request, _ int64) (*http.Response, error) {
		call := calls.Add(1)
		if call == 1 {
			return &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: http.NoBody}, nil
		}
		require.Equal(t, "Bearer refreshed-token", req.Header.Get("Authorization"))
		return ordinaryModelsUpstreamResponse(`{"data":[{"id":"glm-5.3","dim":null}]}`), nil
	})
	service.accountRepo = repo
	service.dimAgentTokenProvider = NewDimAgentTokenProvider(repo, NewDimAgentOAuthService(nil, &dimAgentCapabilityOAuthStub{}), nil)

	response, err := service.FetchOpenAIModelsList(context.Background(), account)
	require.NoError(t, err)
	require.Contains(t, string(response.Body), `"glm-5.3"`)
	require.EqualValues(t, 2, calls.Load())
	require.Equal(t, "refreshed-token", account.DimAgentAccessToken())
}

func TestFetchDimAgentGroupCapabilitiesUsesKeyGroupAndCachesAccountCatalog(t *testing.T) {
	account := *dimAgentCapabilityTestAccount()
	account.ID = 11
	account.Credentials["access_token"] = "group-token"
	var calls atomic.Int32
	service := dimAgentCapabilityTestService([]Account{account}, func(req *http.Request, accountID int64) (*http.Response, error) {
		calls.Add(1)
		require.EqualValues(t, 11, accountID)
		require.Equal(t, "dim", req.URL.Query().Get("type"))
		require.Equal(t, "Bearer group-token", req.Header.Get("Authorization"))
		require.Contains(t, req.Header.Get("User-Agent"), "deepseek-harness/")
		return ordinaryModelsUpstreamResponse(`{"data":[{"id":"deepseek-v4.1-flash","dim":{"display_name":"Flash","limit":{"context":1000000,"output":384000},"reasoning":{"supported":true,"effortOptions":["low","high","max"]},"modalities":["text","image"]}},{"id":"glm-5.3","dim":null}]}`), nil
	})
	group := &Group{ID: 77, Platform: PlatformDimAgent, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"glm-5.3", "deepseek-v4.1-flash"}}}

	first, err := service.FetchDimAgentGroupCapabilities(context.Background(), group)
	require.NoError(t, err)
	second, err := service.FetchDimAgentGroupCapabilities(context.Background(), group)
	require.NoError(t, err)
	require.Equal(t, first.CatalogVersion, second.CatalogVersion)
	require.Equal(t, []string{"glm-5.3", "deepseek-v4.1-flash"}, dimAgentCapabilityIDs(first.Data))
	require.EqualValues(t, 1, calls.Load(), "the existing models cache must serve the second catalog read")
}

func TestFetchDimAgentGroupCapabilitiesMergesMultipleAccountsConservatively(t *testing.T) {
	a, b := *dimAgentCapabilityTestAccount(), *dimAgentCapabilityTestAccount()
	a.ID, b.ID = 1, 2
	a.Credentials["access_token"] = "a"
	b.Credentials["access_token"] = "b"
	service := dimAgentCapabilityTestService([]Account{a, b}, func(_ *http.Request, accountID int64) (*http.Response, error) {
		if accountID == 1 {
			return ordinaryModelsUpstreamResponse(`{"data":[{"id":"m","dim":{"name":"M","limit":{"context":1000,"output":400},"modalities":["text","image"],"tool_call":true,"reasoning":{"supported":true,"effort":"high","effortOptions":["low","high"]}}}]}`), nil
		}
		return ordinaryModelsUpstreamResponse(`{"data":[{"id":"m","dim":{"name":"M2","limit":{"context":800,"output":300},"modalities":["text"],"tool_call":false,"reasoning":{"supported":true,"effort":"low","effortOptions":["high","max"]}}}]}`), nil
	})
	catalog, err := service.FetchDimAgentGroupCapabilities(context.Background(), &Group{ID: 9, Platform: PlatformDimAgent})
	require.NoError(t, err)
	require.Len(t, catalog.Data, 1)
	require.Equal(t, 800, catalog.Data[0].ContextWindow)
	require.Equal(t, 300, catalog.Data[0].MaxOutputTokens)
	require.Equal(t, []string{"text"}, catalog.Data[0].InputModalities)
	require.Equal(t, []DimAgentReasoningEffort{{Level: "high", WireValue: "high"}}, catalog.Data[0].Reasoning.Efforts)
	require.Empty(t, catalog.Data[0].Reasoning.DefaultLevel)
}

func TestFetchDimAgentGroupCapabilitiesFailsClosedWhenAnyAccountCatalogFails(t *testing.T) {
	a, b := *dimAgentCapabilityTestAccount(), *dimAgentCapabilityTestAccount()
	a.ID, b.ID = 1, 2
	service := dimAgentCapabilityTestService([]Account{a, b}, func(_ *http.Request, accountID int64) (*http.Response, error) {
		if accountID == 1 {
			return ordinaryModelsUpstreamResponse(`{"data":[{"id":"m","dim":{"name":"M","limit":{"context":1000,"output":400},"modalities":["text"]}}]}`), nil
		}
		return nil, errors.New("offline")
	})
	_, err := service.FetchDimAgentGroupCapabilities(context.Background(), &Group{ID: 9, Platform: PlatformDimAgent})
	require.Equal(t, http.StatusBadGateway, infraerrors.Code(err))
}

func TestFetchDimAgentGroupCapabilitiesRejectsWrongGroupOrNoAccounts(t *testing.T) {
	service := dimAgentCapabilityTestService(nil, func(*http.Request, int64) (*http.Response, error) { return nil, errors.New("must not run") })
	_, err := service.FetchDimAgentGroupCapabilities(context.Background(), &Group{ID: 1, Platform: PlatformOpenAI})
	require.Equal(t, http.StatusNotFound, infraerrors.Code(err))
	_, err = service.FetchDimAgentGroupCapabilities(context.Background(), &Group{ID: 2, Platform: PlatformDimAgent})
	require.Equal(t, http.StatusServiceUnavailable, infraerrors.Code(err))
}
