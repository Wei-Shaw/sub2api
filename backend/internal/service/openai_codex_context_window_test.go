package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexContextWindowSyncRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name       string
		fields     string
		wantWindow int64
		wantMax    int64
	}{
		{"distinct windows", `"context_window":272000,"max_context_window":872000`, 272000, 872000},
		{"equal windows", `"context_window":272000,"max_context_window":272000`, 272000, 272000},
		{"default only", `"context_window":272000`, 272000, 272000},
		{"maximum only", `"max_context_window":872000`, 872000, 872000},
		{"registry style limit", `"limit":{"context":128000}`, 128000, 128000},
		{"zero maximum", `"context_window":272000,"max_context_window":0`, 272000, 272000},
		{"negative maximum", `"context_window":272000,"max_context_window":-1`, 272000, 272000},
		{"maximum below default", `"context_window":272000,"max_context_window":128000`, 128000, 128000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := codexContextWindowAccount(t, 1, `"context_window":64000`)
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"models":[{
					"slug":"gpt-6-astra","reasoning":true,
					"supported_reasoning_levels":[{"effort":"high"}],
					"input_modalities":["text","image"],%s
				}]}`, tc.fields))),
			}}
			repo := &upstreamModelMetadataRepoStub{}
			syncer := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
			catalog, err := syncer.SyncUpstreamModelCatalog(context.Background(), &account)
			require.NoError(t, err)
			require.Empty(t, catalog.Warnings)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, account.ID, repo.accountID)

			// Reload the JSON representation persisted in account.extra, replacing
			// the legacy snapshot so the test exercises resync as well as parsing.
			persisted, err := json.Marshal(repo.updates)
			require.NoError(t, err)
			account.Extra = nil
			require.NoError(t, json.Unmarshal(persisted, &account.Extra))
			model := codexContextWindowManifest(t, []Account{account})
			require.EqualValues(t, tc.wantWindow, model["context_window"])
			require.EqualValues(t, tc.wantMax, model["max_context_window"])
		})
	}
}

func TestCodexContextWindowAccountIntersection(t *testing.T) {
	for _, tc := range []struct {
		name       string
		accounts   []string
		wantWindow int64
		wantMax    int64
	}{
		{
			"independent minima",
			[]string{`"context_window":272000,"max_context_window":872000`, `"context_window":300000,"max_context_window":512000`},
			272000, 512000,
		},
		{
			"legacy snapshot caps maximum",
			[]string{`"context_window":272000,"max_context_window":872000`, `"context_window":300000`},
			272000, 300000,
		},
		{
			"legacy snapshot alone",
			[]string{`"context_window":272000`},
			272000, 272000,
		},
		{
			"invalid maximum uses known default",
			[]string{`"context_window":272000,"max_context_window":872000`, `"context_window":300000,"max_context_window":-1`},
			272000, 300000,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accounts := make([]Account, len(tc.accounts))
			for i, fields := range tc.accounts {
				accounts[i] = codexContextWindowAccount(t, int64(i+1), fields)
			}
			for range 2 {
				model := codexContextWindowManifest(t, accounts)
				require.EqualValues(t, tc.wantWindow, model["context_window"])
				require.EqualValues(t, tc.wantMax, model["max_context_window"])
				for i, j := 0, len(accounts)-1; i < j; i, j = i+1, j-1 {
					accounts[i], accounts[j] = accounts[j], accounts[i]
				}
			}
		})
	}
}

func TestCodexContextWindowRegistryEnrichment(t *testing.T) {
	for _, tc := range []struct {
		name       string
		fields     string
		wantWindow int64
		wantMax    int64
	}{
		{"preserve upstream limits", `"context_window":272000,"max_context_window":872000`, 272000, 872000},
		{"upstream default remains ceiling", `"context_window":272000`, 272000, 272000},
		{"registry supplies missing limits", `"description":"Model without context metadata"`, 1050000, 1050000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := codexContextWindowAccount(t, 1, `"context_window":64000`)
			account.Type = AccountTypeAPIKey
			account.Credentials["api_key"] = "test-key"
			account.Credentials["base_url"] = "https://provider.example/v1"
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
						`{"data":[{"id":"gpt-6-astra",%s}]}`, tc.fields))),
				},
				{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{"provider":{
						"id":"provider","api":"https://provider.example/v1","models":{
							"gpt-6-astra":{"id":"gpt-6-astra","reasoning":true,
							"reasoning_options":[{"type":"effort","values":["high"]}],
							"modalities":{"input":["text","image"]},"limit":{"context":1050000}}
						}
					}}`)),
				},
			}}
			syncer := &AccountTestService{
				accountRepo: &upstreamModelMetadataRepoStub{}, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig(),
			}
			catalog, err := syncer.SyncUpstreamModelCatalog(context.Background(), &account)
			require.NoError(t, err)
			require.Empty(t, catalog.Warnings)
			require.Len(t, upstream.requests, 2)
			model := codexContextWindowManifest(t, []Account{account})
			require.EqualValues(t, tc.wantWindow, model["context_window"])
			require.EqualValues(t, tc.wantMax, model["max_context_window"])
		})
	}
}

func codexContextWindowAccount(t *testing.T, id int64, fields string) Account {
	t.Helper()
	account := Account{
		ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "test-token",
			"model_mapping": map[string]any{"gpt-6-astra": "gpt-6-astra"},
		},
	}
	err := json.Unmarshal([]byte(fmt.Sprintf(`{"upstream_model_metadata":{
		"source":"upstream","models":{"gpt-6-astra":{
			"id":"gpt-6-astra","reasoning":true,
			"supported_reasoning_levels":["high"],"input_modalities":["text","image"],%s
		}}
	}}`, fields)), &account.Extra)
	require.NoError(t, err)
	return account
}

func codexContextWindowManifest(t *testing.T, accounts []Account) map[string]any {
	t.Helper()
	const groupID int64 = 7042
	svc := &GatewayService{accountRepo: codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{
		groupID: accounts,
	}}}
	body, err := svc.BuildCodexModelsManifestForGroup(
		context.Background(), &Group{ID: groupID, Platform: PlatformOpenAI}, "", []string{"gpt-6-astra"},
	)
	require.NoError(t, err)
	models := decodeCodexManifestModels(t, body)
	require.Len(t, models, 1)
	return models[0]
}
