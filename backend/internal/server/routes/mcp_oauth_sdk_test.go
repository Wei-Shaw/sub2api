//go:build mcp_go_client_oauth

package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/stretchr/testify/require"
)

// The SDK drives discovery, dynamic registration, PKCE generation, code
// exchange, transport authentication, and refresh. Only the browser UI is
// simulated through the same preview/consent REST API used by the SPA.
func TestMCPOAuthOfficialGoSDKInteroperability(t *testing.T) {
	f := newMCPOAuthHTTPFixture(t, service.RoleUser)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var browserApprovals atomic.Int32
	oauth, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{
			Metadata: &oauthex.ClientRegistrationMetadata{
				ClientName: "Official Go SDK", RedirectURIs: []string{"http://127.0.0.1:37777/callback"},
				TokenEndpointAuthMethod: "none", GrantTypes: []string{"authorization_code", "refresh_token"}, ResponseTypes: []string{"code"},
			},
		},
		AuthorizationCodeFetcher: func(_ context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			authorizationURL, err := url.Parse(args.URL)
			if err != nil {
				return nil, err
			}
			if authorizationURL.Scheme+"://"+authorizationURL.Host+authorizationURL.Path != f.server.URL+"/oauth/mcp/authorize" {
				return nil, fmt.Errorf("unexpected authorization endpoint")
			}
			q := authorizationURL.Query()
			if q.Get("resource") != f.resource || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
				return nil, fmt.Errorf("SDK authorization did not bind resource and S256 PKCE")
			}
			preview := f.request(t, http.MethodGet, "/api/v1/user/mcp/authorize?"+q.Encode(), "browser-session", "", nil, false)
			if preview.status != http.StatusOK {
				return nil, fmt.Errorf("authorization preview returned %d: %s", preview.status, preview.body)
			}
			input := service.MCPOAuthConsentInput{Approved: true, Scope: service.MCPScopeUser,
				Authorization: service.MCPOAuthAuthorizeInput{
					ResponseType: q.Get("response_type"), ClientID: q.Get("client_id"), RedirectURI: q.Get("redirect_uri"),
					Scope: q.Get("scope"), State: q.Get("state"), Resource: q.Get("resource"),
					CodeChallenge: q.Get("code_challenge"), CodeChallengeMethod: q.Get("code_challenge_method"),
				},
			}
			decided := f.json(t, http.MethodPost, "/api/v1/user/mcp/authorize", "browser-session", input)
			if decided.status != http.StatusOK {
				return nil, fmt.Errorf("authorization consent returned %d: %s", decided.status, decided.body)
			}
			var result struct {
				Data struct {
					RedirectURL string `json:"redirect_url"`
				} `json:"data"`
			}
			if err := json.Unmarshal(decided.body, &result); err != nil {
				return nil, err
			}
			redirect, err := url.Parse(result.Data.RedirectURL)
			if err != nil {
				return nil, err
			}
			browserApprovals.Add(1)
			return &auth.AuthorizationResult{Code: redirect.Query().Get("code"), State: redirect.Query().Get("state")}, nil
		},
	})
	require.NoError(t, err)
	client := mcp.NewClient(&mcp.Implementation{Name: "oauth-interop-test", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: f.resource, HTTPClient: f.server.Client(), OAuthHandler: oauth}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	require.Equal(t, int32(1), browserApprovals.Load())
	listed, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	require.Contains(t, names, "user_get_api_key")
	require.NotContains(t, names, "user_create_api_key")
	require.NotContains(t, names, "admin_get_dashboard")
	called, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "user_get_api_key", Arguments: map[string]any{"id": 9}})
	require.NoError(t, err)
	require.False(t, called.IsError)
	encoded, err := json.Marshal(called)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "owner-key")
	require.NotContains(t, string(encoded), "must-not-leak")
	source, err := oauth.TokenSource(ctx)
	require.NoError(t, err)
	initial, err := source.Token()
	require.NoError(t, err)
	oldAccess, oldRefresh := initial.AccessToken, initial.RefreshToken
	require.NotEmpty(t, oldRefresh)
	// oauth2 caches the returned token pointer. Expire that cache entry to drive
	// the SDK's real refresh request without sleeping for the access lifetime.
	initial.Expiry = time.Now().Add(-time.Minute)
	_, err = session.ListTools(ctx, nil)
	require.NoError(t, err)
	rotated, err := source.Token()
	require.NoError(t, err)
	require.NotEqual(t, oldAccess, rotated.AccessToken)
	require.NotEqual(t, oldRefresh, rotated.RefreshToken)
	require.Equal(t, int32(1), browserApprovals.Load(), "refresh must not require another browser authorization")
	_, err = f.oauth.Authenticate(ctx, oldAccess)
	require.NoError(t, err, "ordinary refresh preserves in-flight access tokens")
}
