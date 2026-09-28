package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestModelConfigSearchIncludesEntriesWithoutReasoningLevels(t *testing.T) {
	yes, no := true, false
	models := map[string]modelsDevModel{
		"toggle":        {Reasoning: &yes, ReasoningOptions: []modelsDevReasoningOption{{Type: "toggle"}}},
		"budget":        {Reasoning: &yes, ReasoningOptions: []modelsDevReasoningOption{{Type: "budget"}}},
		"no-reasoning":  {Reasoning: &no},
		"none-only":     {Reasoning: &yes, ReasoningOptions: []modelsDevReasoningOption{{Type: "effort", Values: []any{nil, "none"}}}},
		"contradictory": {Reasoning: &no, ReasoningOptions: []modelsDevReasoningOption{{Type: "effort", Values: []any{"high"}}}},
		"z-explicit":    {Reasoning: &yes, ReasoningOptions: []modelsDevReasoningOption{{Type: "effort", Values: []any{nil, "low", "high", "max"}}}},
		"z-inferred":    {ReasoningOptions: []modelsDevReasoningOption{{Type: "effort", Values: []any{"high"}}}},
	}
	s := &AccountTestService{modelMetadataRegistryAt: time.Now(), modelMetadataRegistry: map[string]modelsDevProvider{"provider": {Models: models}}}
	results, err := s.SearchModelConfigs(context.Background(), "provider")
	require.NoError(t, err)
	require.Len(t, results, len(models))
	all, err := s.SearchModelConfigs(context.Background(), "toggle")
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.NotContains(t, all[0].Fields, "supported_reasoning_levels")
	explicit, err := s.SearchModelConfigs(context.Background(), "z-explicit")
	require.NoError(t, err)
	require.Len(t, explicit, 1)
	require.Contains(t, string(explicit[0].Fields["supported_reasoning_levels"]), `"effort":"max"`)
	for i := 0; i < 110; i++ {
		models[fmt.Sprintf("a-missing-%03d", i)] = modelsDevModel{Reasoning: &yes}
	}
	results, err = s.SearchModelConfigs(context.Background(), "provider")
	require.NoError(t, err)
	require.Len(t, results, 100)
	catalog, err := s.ListModelConfigs(context.Background())
	require.NoError(t, err)
	require.Len(t, catalog, len(models), "the initial download must not inherit the search result limit")
	require.Greater(t, len(catalog), 100)
	for _, entry := range catalog {
		require.NotContains(t, entry.Fields, "model_messages", "the catalogue should not serialize unused instruction templates")
	}
}

type modelConfigSourceRepo struct {
	AccountRepository
	accounts []Account
}

func (r modelConfigSourceRepo) ListByGroup(context.Context, int64) ([]Account, error) {
	return r.accounts, nil
}

func (r modelConfigSourceRepo) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	return r.accounts, nil
}

func TestCompositeModelConfigUsesResponsesRouteAndTargetAccountMapping(t *testing.T) {
	account := newCodexModelsAPIKeyTestAccount("https://composite-config.example/v1")
	account.Status, account.Schedulable = StatusActive, true
	account.Credentials["model_mapping"] = map[string]any{"routed-model": "real-upstream-model"}
	wrongAccount := *account
	wrongAccount.ID, wrongAccount.Platform = 99, PlatformMiniMax
	wrongAccount.Credentials = map[string]any{"model_mapping": map[string]any{"public-model": "wrong-model"}}
	routes := []CompositeModelRoute{{ID: 1, GroupID: 7, Enabled: true, PublicModel: "public-model", MatchType: CompositeRouteMatchExact, Endpoint: CompositeRouteEndpointResponses, TargetPlatform: PlatformOpenAI, UpstreamModel: "routed-model"}}
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, id int64, _ int) (*http.Response, error) {
		require.Equal(t, account.ID, id)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"models":[{"slug":"real-upstream-model","display_name":"Route metadata","future_field":true}]}`))}, nil
	}}
	s := &AccountTestService{accountRepo: modelConfigSourceRepo{accounts: []Account{wrongAccount, *account}}, openaiGatewayService: newCodexModelsAPIKeyTestService(upstream), compositeResolver: NewCompositeRouteResolver(compositeRouteRepoStub{routes: routes})}
	fields, err := s.FetchGroupUpstreamModelConfig(context.Background(), &Group{ID: 7, Platform: PlatformComposite}, "public-model")
	require.NoError(t, err)
	require.JSONEq(t, `"Route metadata"`, string(fields["display_name"]))
	require.NotContains(t, fields, "slug")
	// 模型没有 Responses 路由时，不应改用其它协议的上游配置。
	routes[0].Endpoint = CompositeRouteEndpointMessages
	_, err = s.FetchGroupUpstreamModelConfig(context.Background(), &Group{ID: 7, Platform: PlatformComposite}, "public-model")
	require.ErrorContains(t, err, "no unambiguous")
	// 未配置显式路由时，停用账号不能制造跨平台归属冲突。
	s.compositeResolver = nil
	account.Credentials["model_mapping"] = map[string]any{"public-model": "real-upstream-model"}
	wrongAccount.Status = StatusDisabled
	s.accountRepo = modelConfigSourceRepo{accounts: []Account{wrongAccount, *account}}
	_, err = s.FetchGroupUpstreamModelConfig(context.Background(), &Group{ID: 7, Platform: PlatformComposite}, "public-model")
	require.NoError(t, err)
	wrongAccount.Status = StatusActive
	s.accountRepo = modelConfigSourceRepo{accounts: []Account{wrongAccount, *account}}
	_, err = s.FetchGroupUpstreamModelConfig(context.Background(), &Group{ID: 7, Platform: PlatformComposite}, "public-model")
	require.ErrorContains(t, err, "no unambiguous")
}

func TestCompositeCodexRouteCandidatesRequireCallableTarget(t *testing.T) {
	account := Account{ID: 1, Platform: PlatformMiniMax, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"MiniMax-M3": "MiniMax-M3"}}}
	routes := []CompositeModelRoute{
		{ID: 1, GroupID: 7, Enabled: true, PublicModel: "team-model", TargetPlatform: PlatformMiniMax, UpstreamModel: "MiniMax-M3", Endpoint: CompositeRouteEndpointResponses},
		{ID: 2, GroupID: 7, Enabled: true, PublicModel: "missing-model", TargetPlatform: PlatformOpenAI, UpstreamModel: "gpt-6-astra", Endpoint: CompositeRouteEndpointResponses},
		{ID: 3, GroupID: 7, Enabled: true, PublicModel: "messages-only", TargetPlatform: PlatformMiniMax, UpstreamModel: "MiniMax-M3", Endpoint: CompositeRouteEndpointMessages},
		{ID: 4, GroupID: 7, Enabled: false, PublicModel: "disabled-model", TargetPlatform: PlatformMiniMax, UpstreamModel: "MiniMax-M3", Endpoint: CompositeRouteEndpointResponses},
		{ID: 5, GroupID: 7, Enabled: true, PublicModel: "alias-", MatchType: CompositeRouteMatchPrefix, TargetPlatform: PlatformMiniMax, UpstreamModel: "MiniMax-M3", Endpoint: CompositeRouteEndpointResponses},
	}
	s := &GatewayService{accountRepo: modelConfigSourceRepo{accounts: []Account{account}}, compositeResolver: NewCompositeRouteResolver(compositeRouteRepoStub{routes: routes})}
	group := &Group{ID: 7, Platform: PlatformComposite, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"team-model", "alias-selected", "alias-*"}}}
	require.ElementsMatch(t, []string{"team-model", "alias-selected"}, s.GetCompositeCodexRouteModelIDs(context.Background(), group))
}

func TestModelConfigUpstreamImportPreservesRawFieldsAndResolvesAlias(t *testing.T) {
	account := newCodexModelsAPIKeyTestAccount("https://model-config.example/v1")
	account.Status, account.Schedulable = StatusActive, true
	account.Credentials["model_mapping"] = map[string]any{"my-model": "upstream-model"}
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"models":[{"slug":"upstream-model","future_capability":{"mode":"custom"},"model_messages":{"instructions_template":"provider instructions","permissions":null}}]}`))}, nil
	}}
	s := &AccountTestService{accountRepo: modelConfigSourceRepo{accounts: []Account{*account}}, openaiGatewayService: newCodexModelsAPIKeyTestService(upstream)}
	fields, err := s.FetchGroupUpstreamModelConfig(context.Background(), &Group{ID: 1, Platform: PlatformOpenAI}, "my-model")
	require.NoError(t, err)
	require.NotContains(t, fields, "slug")
	require.JSONEq(t, `{"mode":"custom"}`, string(fields["future_capability"]))
	require.JSONEq(t, `{"instructions_template":"provider instructions","permissions":null}`, string(fields["model_messages"]))
	require.NotContains(t, fields, "context_window", "do not import generated placeholders as upstream facts")
	_, err = s.FetchGroupUpstreamModelConfig(context.Background(), &Group{ID: 1, Platform: PlatformOpenAI}, "not-mapped")
	require.Error(t, err)
}
