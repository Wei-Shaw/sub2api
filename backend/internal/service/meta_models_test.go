//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	metapkg "github.com/Wei-Shaw/sub2api/internal/pkg/meta"
	"github.com/stretchr/testify/require"
)

type metaModelsUpstreamStub struct {
	body    string
	lastReq *http.Request
}

func (u *metaModelsUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	u.lastReq = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(u.body)),
	}, nil
}

func (u *metaModelsUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func TestProjectMetaModels(t *testing.T) {
	models := []metapkg.Model{
		metapkg.NewModel("muse-spark-1.3"),
		metapkg.NewModel("muse-wonder-1.0"),
	}

	require.Equal(t, models, projectMetaModels(models, nil))

	projected := projectMetaModels(models, map[string]string{
		"muse-spark":  "muse-spark-1.3",
		"muse-custom": "muse-custom",
	})
	require.Len(t, projected, 2)
	require.Equal(t, "muse-custom", projected[0].ID)
	require.Equal(t, "muse-spark", projected[1].ID)
	require.Equal(t, "muse-spark-1.3", projected[1].DisplayName)
}

func TestBuildMetaUpstreamModelsRequestSendsMuseIdentity(t *testing.T) {
	svc := &AccountTestService{cfg: upstreamModelSyncTestConfig()}
	account := metaTestAccount(map[string]any{"api_key": "sk-muse", "base_url": "https://relay.example/v1"}, AccountTypeAPIKey)

	req, err := svc.buildMetaUpstreamModelsRequest(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "https://relay.example/v1/models", req.URL.String())
	require.Equal(t, "Bearer sk-muse", req.Header.Get("Authorization"))
	require.Equal(t, "application/json", req.Header.Get("Accept"))
	require.Contains(t, req.Header.Get("User-Agent"), "muse-build/1.4.14")
	require.Equal(t, "tbh:exec", req.Header.Get("X-Client-Id"))
}

func TestFetchMetaAccountModelsReadsLiveCatalog(t *testing.T) {
	upstream := &metaModelsUpstreamStub{body: `{"data":[{"id":"muse-spark-1.3"},{"id":"muse-wonder-2.0"},{"id":"gpt-5"}]}`}
	svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
	account := metaTestAccount(map[string]any{"api_key": "sk-muse", "base_url": "https://relay.example/v1"}, AccountTypeAPIKey)

	models, err := svc.FetchMetaAccountModels(context.Background(), account)
	require.NoError(t, err)
	require.Len(t, models, 2)
	require.Equal(t, "muse-spark-1.3", models[0].ID)
	require.Equal(t, "muse-wonder-2.0", models[1].ID)
	require.Equal(t, "/v1/models", upstream.lastReq.URL.EscapedPath())
	require.Contains(t, upstream.lastReq.Header.Get("User-Agent"), "muse-build")
}

func TestMetaSyncUpstreamModelCatalogReturnsLiveModels(t *testing.T) {
	upstream := &metaModelsUpstreamStub{body: `{"data":[{"id":"muse-spark-1.3"},{"id":"muse-wonder-2.0"}]}`}
	svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
	account := metaTestAccount(map[string]any{"api_key": "sk-muse", "base_url": "https://relay.example/v1"}, AccountTypeAPIKey)

	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"muse-spark-1.3", "muse-wonder-2.0"}, catalog.Models)
}

func TestMetaBuildUpstreamModelsRequestFailsWithoutAPIKey(t *testing.T) {
	svc := &AccountTestService{cfg: upstreamModelSyncTestConfig()}
	account := metaTestAccount(map[string]any{"base_url": "https://relay.example/v1"}, AccountTypeAPIKey)

	_, err := svc.buildMetaUpstreamModelsRequest(context.Background(), account)
	require.Error(t, err)
}
