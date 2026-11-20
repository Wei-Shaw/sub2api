package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGPT55RetirementModelSupport(t *testing.T) {
	for _, typ := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
		for _, passthrough := range []bool{false, true} {
			t.Run(typ+"/passthrough="+map[bool]string{true: "true", false: "false"}[passthrough], func(t *testing.T) {
				account := &Account{Platform: PlatformOpenAI, Type: typ, Extra: map[string]any{"openai_passthrough": passthrough}}
				for _, model := range []string{"gpt-5.5", "openai/gpt-5.5-high", "gpt-5.5-2026-04-23", "GPT_5.5", "gpt-5.5-openai-compact"} {
					require.Equal(t, typ == AccountTypeAPIKey, account.IsModelSupported(model), model)
				}
				require.True(t, account.IsModelSupported("gpt-6.1-sol"))
			})
		}
	}
	for _, mapping := range []map[string]any{
		{"legacy": "gpt-5.5"}, {"legacy*": "gpt-5.5"},
	} {
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"model_mapping": mapping}}
		require.False(t, account.IsModelSupported("legacy"))
	}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.5": "gpt-6.1-sol"}}}
	require.True(t, account.IsModelSupported("gpt-5.5"), "explicit administrator replacement remains authoritative")
	account.Type = AccountTypeAPIKey
	require.True(t, account.IsModelSupported("gpt-5.5"))
	parent := int64(3)
	shadow := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, ParentAccountID: &parent}
	require.False(t, shadow.IsModelSupported("gpt-5.5"))
	require.False(t, isRetiredOpenAISubscriptionModel(&Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}, "gpt-5.5"))
	require.False(t, isRetiredOpenAISubscriptionModel(account, "gpt-5.50"))
}

func TestGPT55RetirementCatalogs(t *testing.T) {
	oauth := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	api := Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	group := &Group{ID: 1, Platform: PlatformOpenAI}
	for _, tc := range []struct {
		name      string
		accounts  []Account
		wantGPT55 bool
	}{
		{"oauth", []Account{oauth}, false}, {"api", []Account{api}, true}, {"mixed", []Account{oauth, api}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{1: tc.accounts}}
			gateway := &GatewayService{accountRepo: repo}
			models := gateway.GetAvailableModels(context.Background(), &group.ID, PlatformOpenAI)
			if models != nil {
				require.Equal(t, tc.wantGPT55, containsModelID(models, "gpt-5.5"))
			}
			body, err := buildCodexModelsManifestForAccounts(PlatformOpenAI, []string{"gpt-5.5", "gpt-6.1-sol"}, tc.accounts, group, nil, true)
			require.NoError(t, err)
			require.Equal(t, tc.wantGPT55, containsManifestModel(body, "gpt-5.5"))
			// A stale/pinned upstream body must be filtered before evaluating its old ETag.
			original := []byte(`{"models":[{"slug":"gpt-5.5","unknown":1},{"slug":"gpt-6.1-sol","unknown":2}],"unknown_top":3}`)
			etag := CodexModelsManifestETag(original)
			manifest := &OpenAIModelsResponse{Body: original, ETag: etag}
			service := &OpenAIGatewayService{accountRepo: repo}
			require.NoError(t, service.MergeGroupConfiguredCodexModels(context.Background(), group, manifest, etag))
			if !tc.wantGPT55 {
				require.False(t, manifest.NotModified)
				require.NotEqual(t, etag, manifest.ETag)
				require.False(t, containsManifestModel(manifest.Body, "gpt-5.5"))
				require.Equal(t, int64(3), gjson.GetBytes(manifest.Body, "unknown_top").Int())
				require.Equal(t, int64(2), gjson.GetBytes(manifest.Body, "models.0.unknown").Int())
			}
		})
	}
	// All configured entries retired: plain discovery must remain an authoritative empty list.
	oauth.Credentials = map[string]any{"model_mapping": map[string]any{"gpt-5.5": "gpt-5.5"}}
	repo := codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{1: {oauth}}}
	models := (&GatewayService{accountRepo: repo}).GetAvailableModels(context.Background(), &group.ID, PlatformOpenAI)
	require.NotNil(t, models)
	require.Empty(t, models)
}

func containsModelID(models []string, id string) bool {
	for _, model := range models {
		if model == id {
			return true
		}
	}
	return false
}
func containsManifestModel(body []byte, id string) bool {
	for _, m := range gjson.GetBytes(body, "models").Array() {
		if m.Get("slug").String() == id {
			return true
		}
	}
	return false
}

func TestGPT55RetirementAccountProjection(t *testing.T) {
	body := []byte(`{"object":"list","data":[{"id":"gpt-5.5"},{"id":"gpt-6.1-sol","unknown":true}],"unknown_top":true}`)
	for _, typ := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		account := &Account{Platform: PlatformOpenAI, Type: typ}
		got, err := projectAccountModelsBody(body, account, nil, false)
		require.NoError(t, err)
		require.Equal(t, typ == AccountTypeAPIKey, strings.Contains(string(got), "gpt-5.5"))
		require.True(t, gjson.GetBytes(got, "unknown_top").Bool())
		require.True(t, json.Valid(got))
	}
}

func TestGPT55RetirementProjectionUsesPublicNames(t *testing.T) {
	for _, codex := range []bool{false, true} {
		field, idField := "data", "id"
		if codex {
			field, idField = "models", "slug"
		}
		for _, tc := range []struct {
			name        string
			mapping     map[string]any
			passthrough bool
			api         bool
			want        map[string]string
		}{
			{"one-hop", map[string]any{"legacy": "gpt-5.5", "gpt-5.5": "gpt-6.1-sol"}, false, false, map[string]string{"gpt-5.5": "live"}},
			{"crossed", map[string]any{"gpt-6.1-sol": "gpt-5.5", "gpt-5.5": "gpt-6.1-sol"}, false, false, map[string]string{"gpt-5.5": "live"}},
			{"wildcard", map[string]any{"legacy*": "gpt-5.5", "gpt-5.5": "gpt-6.1-sol"}, false, false, map[string]string{"gpt-5.5": "live"}},
			{"passthrough", map[string]any{"gpt-6.1-sol": "gpt-5.5", "gpt-5.5": "gpt-6.1-sol"}, true, false, map[string]string{"gpt-6.1-sol": "live"}},
			{"api", map[string]any{"legacy": "gpt-5.5", "gpt-5.5": "gpt-6.1-sol"}, false, true, map[string]string{"legacy": "retired", "gpt-5.5": "live"}},
		} {
			t.Run(field+"/"+tc.name, func(t *testing.T) {
				account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
					Credentials: map[string]any{"model_mapping": tc.mapping}, Extra: map[string]any{"openai_passthrough": tc.passthrough}}
				if tc.api {
					account.Type = AccountTypeAPIKey
				}
				group := &Group{Platform: PlatformOpenAI, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"legacy", "gpt-5.5", "gpt-6.1-sol"}}}
				body, err := json.Marshal(map[string]any{field: []map[string]any{
					{idField: "gpt-5.5", "source": "retired"}, {idField: "gpt-6.1-sol", "source": "live"},
				}, "unknown_top": true})
				require.NoError(t, err)
				original := string(body)
				got, err := projectAccountModelsBody(body, account, group, codex)
				require.NoError(t, err)
				require.Equal(t, original, string(body), "shared upstream cache must remain unchanged")
				require.True(t, gjson.GetBytes(got, "unknown_top").Bool())
				entries := gjson.GetBytes(got, field).Array()
				require.Len(t, entries, len(tc.want), string(got))
				for _, entry := range entries {
					id := entry.Get(idField).String()
					require.Contains(t, tc.want, id)
					require.Equal(t, tc.want[id], entry.Get("source").String(), "metadata must come from the one-hop upstream target")
					require.True(t, account.IsModelSupported(id), "catalog and routing must agree for %s", id)
				}
				if codex {
					response := &OpenAIModelsResponse{Body: body, ETag: CodexModelsManifestETag(body)}
					require.NoError(t, ApplyPinnedCodexModelsMapping(response, account, group))
					require.JSONEq(t, string(got), string(response.Body))
					require.Equal(t, codexModelsManifestBodyETag(response.Body), response.ETag)
					group.ID = 1
					group.CodexModelsManifestConfig.Enabled = true
					service := &OpenAIGatewayService{accountRepo: codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{1: {*account}}}}
					require.NoError(t, service.MergeGroupConfiguredCodexModels(context.Background(), group, response, CodexModelsManifestETag(body)))
					require.False(t, response.NotModified, "an upstream ETag must not hide the projected catalog")
					require.Equal(t, codexModelsManifestBodyETag(response.Body), response.ETag)
					require.NoError(t, service.MergeGroupConfiguredCodexModels(context.Background(), group, response, response.ETag))
					require.True(t, response.NotModified, "the final public ETag must support conditional requests")
				}
			})
		}
	}
}

func TestGPT55RetirementCompactFallbackKeepsSelectedModel(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
			Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{"openai_passthrough": passthrough}}
		upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"compact","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`))}}
		svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.5"}}, httpUpstream: upstream}
		require.Empty(t, svc.resolveOpenAICompactFallbackModel(account, "gpt-6.1-sol"))
		c := newOpenAICompactFallbackTestContext(t, "/v1/responses/compact")
		_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-6.1-sol","input":[],"instructions":"compact","stream":false}`))
		require.NoError(t, err)
		require.Equal(t, "gpt-6.1-sol", gjson.GetBytes(upstream.lastBody, "model").String())
		account.Type = AccountTypeAPIKey
		require.Equal(t, "gpt-5.5", svc.resolveOpenAICompactFallbackModel(account, "gpt-6.1-sol"))
	}
}

func TestGPT55RetirementSchedulerAndDiagnosis(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1)
	oauth := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	api := Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}
	for _, typ := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
		account := oauth
		account.Type = typ
		scheduler := &defaultOpenAIAccountScheduler{}
		ok, _ := scheduler.isAccountRequestCompatibleReason(ctx, &account, OpenAIAccountScheduleRequest{RequestedModel: "gpt-5.5"})
		require.Equal(t, typ == AccountTypeAPIKey, ok)
		reason := openAICompatibleAccountEligibilityFailureReasonBeforeProfit(ctx, &account, PlatformOpenAI, "gpt-5.5", false, "")
		require.Equal(t, typ == AccountTypeAPIKey, reason == "")
	}
	oauth.Credentials = map[string]any{"compact_model_mapping": map[string]any{"gpt-6.1-sol": "gpt-5.5"}}
	scheduler := &defaultOpenAIAccountScheduler{}
	ok, reason := scheduler.isAccountRequestCompatibleReason(ctx, &oauth, OpenAIAccountScheduleRequest{RequestedModel: "gpt-6.1-sol", RequireCompact: true})
	require.False(t, ok)
	require.Equal(t, "model_not_supported", reason)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-6.1-sol"}}}
	require.Empty(t, svc.resolveOpenAICompactFallbackModel(&oauth, "gpt-6.1-sol"), "native compact retries must not use a retired explicit target")
	oauth.Credentials = map[string]any{"compact_model_mapping": map[string]any{"gpt-5.5": "gpt-6.1-sol"}}
	ok, _ = scheduler.isAccountRequestCompatibleReason(ctx, &oauth, OpenAIAccountScheduleRequest{RequestedModel: "gpt-5.5", RequireCompact: true})
	require.True(t, ok, "an explicit compact replacement remains usable")
	for _, accounts := range [][]Account{{oauth}, {oauth, api}} {
		svc := &OpenAIGatewayService{accountRepo: codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{groupID: accounts}}}
		diagnosis := svc.DiagnoseModelAvailabilityForPlatform(ctx, &groupID, "gpt-5.5", PlatformOpenAI)
		require.True(t, diagnosis.HasAccountsInPool)
		require.True(t, diagnosis.HasRetiredSubscriptionModel)
		require.Equal(t, len(accounts) == 2, diagnosis.HasModelSupport)
	}
}

func TestGPT55RetirementEmptyListingSource(t *testing.T) {
	// A configured alias targeting retired GPT-5.5 must not reappear in a generated catalog.
	account := Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"model_mapping": map[string]any{"legacy": "gpt-5.5"}}}
	require.Empty(t, openAIConfiguredCodexModelIDsForGroup([]Account{account}, nil))
	body, err := buildCodexModelsManifestForAccounts(PlatformComposite, []string{"gpt-5.5", "gpt-6.1-sol"}, []Account{account}, nil, nil, true)
	require.NoError(t, err)
	require.False(t, containsManifestModel(body, "gpt-5.5"))
}
