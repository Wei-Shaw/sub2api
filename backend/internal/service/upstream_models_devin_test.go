package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type devinModelWiringUpstream struct {
	t        *testing.T
	requests int
}

func (u *devinModelWiringUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.requests++
	require.Equal(u.t, http.MethodPost, req.Method)
	require.True(u.t, HTTPUpstreamRedirectsDisabled(req.Context()))
	require.True(u.t, HTTPUpstreamPublicHostsOnly(req.Context()))
	require.Equal(u.t, "server.codeium.com", req.URL.Host)
	require.Equal(u.t, "/exa.api_server_pb.ApiServerService/GetCliModelConfigs", req.URL.Path,
		"Devin listing must use the native hook, never /v1/models or models.dev enrichment")
	// An empty protobuf catalog exercises the supported CLI seed fallback.
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
}
func (u *devinModelWiringUpstream) DoWithTLS(req *http.Request, proxy string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, accountID, concurrency)
}

func TestDevinModelCatalogDispatchSkipsUnrelatedCapabilityRegistry(t *testing.T) {
	upstream := &devinModelWiringUpstream{t: t}
	service := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
	account := &Account{Platform: PlatformDevin, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "cli-session"}}
	catalog, err := service.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"swe-1-6", "swe-1-6-fast"}, catalog.Models)
	require.Equal(t, 1, upstream.requests)
	require.Len(t, catalog.Warnings, 1)
	require.Equal(t, UpstreamModelMetadataIncompleteCode, catalog.Warnings[0].Code)
	for _, metadata := range catalog.Metadata {
		require.Nil(t, metadata.Reasoning, "do not invent unsupported reasoning capabilities")
		require.Zero(t, metadata.ContextWindow, "do not invent context limits")
	}
	require.Nil(t, account.GetUpstreamModelMetadataSnapshot(), "incomplete capabilities must not be persisted")
}

func TestDevinModelListDispatchRejectsUnsupportedTypeBeforeGenericTransport(t *testing.T) {
	service := &AccountTestService{}
	_, _, err := service.fetchUpstreamModelList(context.Background(), &Account{Platform: PlatformDevin, Type: AccountTypeAPIKey})
	require.ErrorContains(t, err, "Devin")
	require.NotContains(t, err.Error(), "HTTP client is not configured")
}

func TestDevinConnectivityDispatchDoesNotFallThroughToClaude(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/accounts/1/test", nil)
	repo := &devinAdminAccountRepo{account: &Account{ID: 1, Platform: PlatformDevin, Type: AccountTypeOAuth}}
	service := &AccountTestService{accountRepo: repo}
	err := service.TestAccountConnection(ctx, 1, "", "", "")
	require.Error(t, err)
	require.Contains(t, recorder.Body.String(), "Devin connection test failed")
	require.Equal(t, "text/event-stream", recorder.Result().Header.Get("Content-Type"))
	require.Equal(t, "no-cache", recorder.Result().Header.Get("Cache-Control"))
	require.Equal(t, "no", recorder.Result().Header.Get("X-Accel-Buffering"))
	require.NotContains(t, recorder.Body.String(), "claude-")
}
