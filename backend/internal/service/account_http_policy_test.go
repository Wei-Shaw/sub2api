//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/stretchr/testify/require"
)

func TestAccountHTTPPolicyPreservesOtherURLRestrictions(t *testing.T) {
	cfg := &config.Config{}
	openai := &OpenAIGatewayService{cfg: cfg}
	validators := map[string]func(string) (string, error){
		"openai":                  openai.validateUpstreamBaseURL,
		"anthropic":               (&GatewayService{cfg: cfg}).validateUpstreamBaseURL,
		"gemini":                  (&GeminiMessagesCompatService{cfg: cfg}).validateUpstreamBaseURL,
		"account test and models": (&AccountTestService{cfg: cfg}).validateUpstreamBaseURL,
		"grok":                    func(raw string) (string, error) { return grokOperatorPolicyValidator(cfg)(raw) },
		"account balance":         func(raw string) (string, error) { return cnValidateProbeURL(cfg, raw) },
	}
	for name, validate := range validators {
		t.Run(name, func(t *testing.T) {
			cfg.Security.URLAllowlist.Enabled = false
			for _, raw := range []string{"http://new-upstream.example:8080/v1", "https://new-upstream.example/v1"} {
				got, err := validate(raw)
				require.NoError(t, err)
				require.Equal(t, raw, got)
			}
			for _, raw := range []string{"file:///tmp/key", "ftp://new-upstream.example", "http://new-upstream.example:99999"} {
				_, err := validate(raw)
				require.Error(t, err)
			}
			cfg.Security.URLAllowlist.Enabled = true
			cfg.Security.URLAllowlist.UpstreamHosts = []string{"new-upstream.example", "127.0.0.1"}
			_, err := validate("http://new-upstream.example/v1")
			require.NoError(t, err)
			for _, raw := range []string{"http://unlisted.example/v1", "http://127.0.0.1/v1"} {
				_, err := validate(raw)
				require.Error(t, err)
			}
		})
	}
	for _, enabled := range []bool{false, true} {
		cfg.Security.URLAllowlist.Enabled = enabled
		_, err := openai.validateOutboundURL("http://new-upstream.example/image.png")
		require.Error(t, err, "account exception must not reach media fetches")
		_, err = urlvalidator.ValidateHTTPSURL("http://new-upstream.example/video.mp4", urlvalidator.ValidationOptions{})
		require.Error(t, err)
	}
}

func TestAccountHTTPModelsRequestUsesConfiguredEndpoint(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "http://new-upstream.example:8080/v1", "api_key": "test-only"}}
	req, err := buildOpenAIAPIKeyModelsRequest(context.Background(), account, svc.validateUpstreamBaseURL)
	require.NoError(t, err)
	require.Equal(t, "http://new-upstream.example:8080/v1/models", req.URL.String())
	require.Equal(t, "Bearer test-only", req.Header.Get("Authorization"))
}

func TestAccountHTTPBaseURLSavedWithoutReplacingKey(t *testing.T) {
	repo := &updateAccountCredsRepoStub{account: &Account{
		ID: 202, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"base_url": "https://old.example", "api_key": "test-existing"},
	}}
	svc := &adminServiceImpl{accountRepo: repo}
	updated, err := svc.UpdateAccount(context.Background(), 202, &UpdateAccountInput{
		Credentials: map[string]any{"base_url": "http://new-upstream.example:8080/v1"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, "http://new-upstream.example:8080/v1", updated.Credentials["base_url"])
	require.Equal(t, "test-existing", updated.Credentials["api_key"])
}

func TestAccountHTTPModelListRoundTrip(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer api-key-test" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"http-proof"}]}`))
	}))
	defer upstream.Close()
	svc := newCodexModelsAPIKeyTestService(&codexModelsHTTPUpstreamStub{
		do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
			return upstream.Client().Do(req)
		},
	})
	svc.cfg = &config.Config{}
	account := newCodexModelsAPIKeyTestAccount(upstream.URL + "/v1")
	account.Credentials["api_key"] = "api-key-test"
	result, err := svc.FetchOpenAIModelsList(context.Background(), account)
	require.NoError(t, err)
	require.Contains(t, string(result.Body), "http-proof")
}
