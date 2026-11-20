package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func antigravityTierTestMapping() map[string]any {
	return map[string]any{
		"gemini-4-hydrogen":        "gemini-4-hydrogen",
		"gemini-4-hydrogen-low":    "gemini-4-hydrogen-low",
		"gemini-4-hydrogen-medium": "gemini-4-hydrogen-medium",
		"gemini-4-hydrogen-high":   "gemini-4-hydrogen-high",
		"gemini-4-hydrogen-xhigh":  "gemini-4-hydrogen-xhigh",
	}
}

func TestCodexAntigravityTierCatalog(t *testing.T) {
	const groupID = 793
	account := newAntigravityCompatAccount(AccountTypeOAuth)
	account.Schedulable = true
	account.Credentials["model_mapping"] = antigravityTierTestMapping()
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{
		Models: map[string]UpstreamModelMetadata{
			"gemini-4-hydrogen-high": {ID: "gemini-4-hydrogen-high", InputModalities: []string{"text", "image"}},
		},
	})
	svc := &GatewayService{accountRepo: codexModelsVisibilityAccountRepo{
		byGroup: map[int64][]Account{groupID: {*account}},
	}}
	group := &Group{ID: groupID, Platform: PlatformAntigravity}
	modelIDs := []string{"gemini-4-hydrogen", "gemini-4-hydrogen-low", "gemini-4-hydrogen-medium", "gemini-4-hydrogen-high", "gemini-4-hydrogen-xhigh"}
	body, err := svc.BuildCodexModelsManifestForGroup(context.Background(), group, "", modelIDs)
	require.NoError(t, err)
	require.Equal(t, []string{"gemini-4-hydrogen"}, codexManifestModelSlugs(t, body))
	model := decodeCodexManifestModels(t, body)[0]
	require.Equal(t, []string{"low", "medium", "high", "xhigh"}, effortsFromManifestModel(t, model))
	require.Equal(t, "high", model["default_reasoning_level"])

	body, err = svc.BuildCodexModelsManifestForGroup(context.Background(), group, PlatformAntigravity, modelIDs)
	require.NoError(t, err)
	require.ElementsMatch(t, modelIDs, codexManifestModelSlugs(t, body))

	body, err = svc.BuildCodexModelsManifestForGroup(context.Background(), group, "", modelIDs[4:])
	require.NoError(t, err)
	require.Equal(t, modelIDs[4:], codexManifestModelSlugs(t, body))

	other := Account{ID: 794, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}
	svc.accountRepo = codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{groupID: {*account, other}}}
	group.Platform = PlatformComposite
	body, err = svc.BuildCodexModelsManifestForGroup(context.Background(), group, "", modelIDs)
	require.NoError(t, err)
	require.Equal(t, modelIDs[:1], codexManifestModelSlugs(t, body))

	other.Credentials = map[string]any{"model_mapping": map[string]any{modelIDs[4]: "gpt-5"}}
	svc.accountRepo = codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{groupID: {*account, other}}}
	body, err = svc.BuildCodexModelsManifestForGroup(context.Background(), group, "", modelIDs)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{modelIDs[0], modelIDs[4]}, codexManifestModelSlugs(t, body))
}

func TestCodexAntigravityTierResponses(t *testing.T) {
	for _, tc := range []struct {
		name, request, upstreamModel string
	}{
		{"extra high", `{"model":"gemini-4-hydrogen","reasoning":{"effort":"xhigh"},"stream":true,"input":"hello"}`, "gemini-4-hydrogen-xhigh"},
		{"explicit tier", `{"model":"gemini-4-hydrogen-medium","stream":true,"input":"hello"}`, "gemini-4-hydrogen-medium"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
			account := newAntigravityCompatAccount(AccountTypeOAuth)
			account.Credentials["model_mapping"] = antigravityTierTestMapping()
			body := []byte(tc.request)
			c, rec := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)
			_, err := svc.ForwardAsResponses(context.Background(), c, account, body, nil)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Len(t, upstream.requestBodies, 1)
			wire := gjson.ParseBytes(upstream.requestBodies[0])
			require.Equal(t, tc.upstreamModel, wire.Get("model").String())
			require.False(t, wire.Get("request.generationConfig.thinkingConfig.thinkingLevel").Exists())
			require.False(t, wire.Get("request.generationConfig.thinkingConfig.thinkingBudget").Exists())
		})
	}
}
