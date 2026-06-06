package xai

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildAuthorizeURLUsesTrustedEndpointAndRequiredParams(t *testing.T) {
	authURL, err := BuildAuthorizeURL(AuthorizeParams{
		AuthorizationEndpoint: "https://auth.x.ai/oauth2/auth",
		State:                 "state-1",
		Nonce:                 "nonce-1",
		CodeChallenge:         "challenge-1",
		RedirectURI:           "http://127.0.0.1:56121/callback",
	})
	require.NoError(t, err)

	parsed, err := url.Parse(authURL)
	require.NoError(t, err)
	require.Equal(t, "https", parsed.Scheme)
	require.Equal(t, "auth.x.ai", parsed.Host)

	values := parsed.Query()
	require.Equal(t, "code", values.Get("response_type"))
	require.Equal(t, ClientID, values.Get("client_id"))
	require.Equal(t, "http://127.0.0.1:56121/callback", values.Get("redirect_uri"))
	require.Equal(t, Scope, values.Get("scope"))
	require.Equal(t, "challenge-1", values.Get("code_challenge"))
	require.Equal(t, "S256", values.Get("code_challenge_method"))
	require.Equal(t, "state-1", values.Get("state"))
	require.Equal(t, "nonce-1", values.Get("nonce"))
	require.Equal(t, "generic", values.Get("plan"))
	require.Equal(t, "cli-proxy-api", values.Get("referrer"))
}

func TestValidateOAuthEndpointRejectsUntrustedEndpoints(t *testing.T) {
	require.NoError(t, ValidateOAuthEndpoint("https://auth.x.ai/token", "token_endpoint"))
	require.NoError(t, ValidateOAuthEndpoint("https://login.auth.x.ai/token", "token_endpoint"))

	err := ValidateOAuthEndpoint("http://auth.x.ai/token", "token_endpoint")
	require.Error(t, err)
	require.Contains(t, err.Error(), "must use https")

	err = ValidateOAuthEndpoint("https://x.ai.evil.example/token", "token_endpoint")
	require.Error(t, err)
	require.Contains(t, err.Error(), "host is not trusted")
}

func TestGeneratePKCEChallenge(t *testing.T) {
	verifier := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"

	challenge := GenerateCodeChallenge(verifier)

	require.NotEmpty(t, challenge)
	require.NotContains(t, challenge, "+")
	require.NotContains(t, challenge, "/")
	require.False(t, strings.HasSuffix(challenge, "="))
}

func TestBuildResponsesURL(t *testing.T) {
	require.Equal(t, "https://api.x.ai/v1/responses", BuildResponsesURL(""))
	require.Equal(t, "https://api.x.ai/v1/responses", BuildResponsesURL("https://api.x.ai/v1"))
	require.Equal(t, "https://api.x.ai/v1/responses", BuildResponsesURL("https://api.x.ai/v1/"))
	require.Equal(t, "https://proxy.example/v1/responses", BuildResponsesURL("https://proxy.example/v1/responses"))
}

func TestBuildModelsURL(t *testing.T) {
	require.Equal(t, "https://api.x.ai/v1/models", BuildModelsURL(""))
	require.Equal(t, "https://api.x.ai/v1/models", BuildModelsURL("https://api.x.ai/v1"))
	require.Equal(t, "https://api.x.ai/v1/models", BuildModelsURL("https://api.x.ai/v1/"))
	require.Equal(t, "https://proxy.example/v1/models", BuildModelsURL("https://proxy.example/v1/models"))
	require.Equal(t, "https://proxy.example/v1/models", BuildModelsURL("https://proxy.example/v1/responses"))
}

func TestBuildImagesGenerationsURL(t *testing.T) {
	require.Equal(t, "https://api.x.ai/v1/images/generations", BuildImagesGenerationsURL(""))
	require.Equal(t, "https://api.x.ai/v1/images/generations", BuildImagesGenerationsURL("https://api.x.ai/v1"))
	require.Equal(t, "https://api.x.ai/v1/images/generations", BuildImagesGenerationsURL("https://api.x.ai/v1/"))
	require.Equal(t, "https://proxy.example/v1/images/generations", BuildImagesGenerationsURL("https://proxy.example/v1/images/generations"))
	require.Equal(t, "https://proxy.example/v1/images/generations", BuildImagesGenerationsURL("https://proxy.example/v1/responses"))
	require.Equal(t, "https://proxy.example/v1/images/generations", BuildImagesGenerationsURL("https://proxy.example/v1/models"))
}

func TestBuildVideosGenerationsURL(t *testing.T) {
	require.Equal(t, "https://api.x.ai/v1/videos/generations", BuildVideosGenerationsURL(""))
	require.Equal(t, "https://api.x.ai/v1/videos/generations", BuildVideosGenerationsURL("https://api.x.ai/v1"))
	require.Equal(t, "https://api.x.ai/v1/videos/generations", BuildVideosGenerationsURL("https://api.x.ai/v1/"))
	require.Equal(t, "https://proxy.example/v1/videos/generations", BuildVideosGenerationsURL("https://proxy.example/v1/videos/generations"))
	require.Equal(t, "https://proxy.example/v1/videos/generations", BuildVideosGenerationsURL("https://proxy.example/v1/responses"))
	require.Equal(t, "https://proxy.example/v1/videos/generations", BuildVideosGenerationsURL("https://proxy.example/v1/models"))
	require.Equal(t, "https://proxy.example/v1/videos/generations", BuildVideosGenerationsURL("https://proxy.example/v1/images/generations"))
}

func TestBuildVideoPollURL(t *testing.T) {
	require.Equal(t, "https://api.x.ai/v1/videos/request-1", BuildVideoPollURL("", "request-1"))
	require.Equal(t, "https://api.x.ai/v1/videos/request-1", BuildVideoPollURL("https://api.x.ai/v1", "request-1"))
	require.Equal(t, "https://proxy.example/v1/videos/request-1", BuildVideoPollURL("https://proxy.example/v1/videos/generations", "request-1"))
	require.Equal(t, "https://proxy.example/v1/videos/request%201", BuildVideoPollURL("https://proxy.example/v1/responses", "request 1"))
}
