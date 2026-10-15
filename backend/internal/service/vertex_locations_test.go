package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVertexAPIHost(t *testing.T) {
	require.Equal(t, "aiplatform.googleapis.com", vertexAPIHost("global"))
	require.Equal(t, "aiplatform.us.rep.googleapis.com", vertexAPIHost("us"))
	require.Equal(t, "aiplatform.eu.rep.googleapis.com", vertexAPIHost("eu"))
	require.Equal(t, "us-central1-aiplatform.googleapis.com", vertexAPIHost("us-central1"))
}

func TestAccountVertexLocationResolvesMultiRegionPerModel(t *testing.T) {
	account := func(creds map[string]any) *Account {
		return &Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: creds}
	}
	us := account(map[string]any{"location": "us"})
	require.Equal(t, "us", us.VertexLocation("gemini-3.7-flash"))
	require.Equal(t, "us", us.VertexLocation("publishers/google/models/gemini-3.8-flash"))
	require.Equal(t, "global", us.VertexLocation("gemini-3.1-pro-preview"))
	require.Equal(t, "global", us.VertexLocation("gemini-2.5-flash"))
	require.Equal(t, "global", us.VertexLocation(""))

	eu := account(map[string]any{"vertex_location": "eu"})
	require.Equal(t, "eu", eu.VertexLocation("gemini-3.1-flash-lite"))
	require.Equal(t, "global", eu.VertexLocation("gemini-3-pro-image"))

	regional := account(map[string]any{"location": "us-central1"})
	require.Equal(t, "us-central1", regional.VertexLocation("gemini-3.1-pro-preview"))
	require.Equal(t, "us-central1", regional.VertexLocation(""))

	global := account(map[string]any{"location": "global"})
	require.Equal(t, "global", global.VertexLocation("gemini-3.7-flash"))

	override := account(map[string]any{"location": "us", "vertex_model_locations": map[string]any{"gemini-3.7-flash": "global", "gemini-2.5-flash": "us"}})
	require.Equal(t, "global", override.VertexLocation("gemini-3.7-flash"))
	require.Equal(t, "us", override.VertexLocation("gemini-2.5-flash"))
	require.Equal(t, "us", override.VertexLocation("gemini-3.8-flash"))

	require.Equal(t, vertexDefaultLocation, account(nil).VertexLocation("gemini-3.7-flash"))

	anthropic := &Account{Platform: PlatformAnthropic, Type: AccountTypeServiceAccount, Credentials: map[string]any{"location": "us"}}
	require.Equal(t, "us", anthropic.VertexLocation("claude-sonnet-4-5@20250929"))
}

func TestBuildVertexGeminiAccountURLMultiRegion(t *testing.T) {
	account := &Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: map[string]any{"project_id": "p1", "location": "us"}}

	got, err := buildVertexGeminiAccountURL(account, "gemini-3.7-flash", "streamGenerateContent", true, "")
	require.NoError(t, err)
	require.Equal(t, "https://aiplatform.us.rep.googleapis.com/v1/projects/p1/locations/us/publishers/google/models/gemini-3.7-flash:streamGenerateContent?alt=sse", got)

	got, err = buildVertexGeminiAccountURL(account, "gemini-3.1-pro-preview", "generateContent", false, "")
	require.NoError(t, err)
	require.Equal(t, "https://aiplatform.googleapis.com/v1/projects/p1/locations/global/publishers/google/models/gemini-3.1-pro-preview:generateContent", got)

	got, err = buildVertexGeminiAccountURL(account, "gemini-3.7-flash", "generateContent", false, "global")
	require.NoError(t, err)
	require.Equal(t, "https://aiplatform.googleapis.com/v1/projects/p1/locations/global/publishers/google/models/gemini-3.7-flash:generateContent", got)

	global := &Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: map[string]any{"project_id": "p1", "location": "global"}}
	got, err = buildVertexGeminiAccountURL(global, "gemini-3.7-flash", "countTokens", false, "us")
	require.NoError(t, err)
	require.Equal(t, "https://aiplatform.us.rep.googleapis.com/v1/projects/p1/locations/us/publishers/google/models/gemini-3.7-flash:countTokens", got)
}

func TestBuildVertexAnthropicURLMultiRegionHost(t *testing.T) {
	got, err := buildVertexAnthropicURL("my-project", "us", "claude-sonnet-4-5@20250929", true)
	require.NoError(t, err)
	require.Equal(t, "https://aiplatform.us.rep.googleapis.com/v1/projects/my-project/locations/us/publishers/anthropic/models/claude-sonnet-4-5@20250929:streamRawPredict", got)
}

func TestGeminiCachedContentLocationInBody(t *testing.T) {
	cases := map[string]string{
		`{"cachedContent":"projects/123/locations/us/cachedContents/456","contents":[]}`:               "us",
		`{"contents":[],"cachedContent":"projects/p-1/locations/global/cachedContents/456"}`:           "global",
		`{"generateContentRequest":{"cachedContent":"projects/123/locations/eu/cachedContents/9"}}`:    "eu",
		`{"cachedContent":"cachedContents/abc","contents":[]}`:                                         "",
		`{"contents":[{"role":"user","parts":[{"text":"projects/1/locations/us/cachedContents/2"}]}]}`: "",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`:                                       "",
		`{"cachedContent":"projects/123/locations/us/cachedContents/../x","contents":[]}`:              "",
	}
	for body, want := range cases {
		require.Equal(t, want, geminiCachedContentLocationInBody([]byte(body)), body)
	}
}

func TestGeminiCachedContentVertexURLsMultiRegion(t *testing.T) {
	account := &Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: map[string]any{"project_id": "proj-1", "location": "us"}}

	model, err := GeminiCachedContentUpstreamModelResource(account, "gemini-3.7-flash")
	require.NoError(t, err)
	require.Equal(t, "projects/proj-1/locations/us/publishers/google/models/gemini-3.7-flash", model)

	collection, err := geminiCachedContentCollectionURL(account, "", "gemini-3.7-flash")
	require.NoError(t, err)
	require.Equal(t, "https://aiplatform.us.rep.googleapis.com/v1/projects/proj-1/locations/us/cachedContents", collection)

	model, err = GeminiCachedContentUpstreamModelResource(account, "gemini-3.1-pro-preview")
	require.NoError(t, err)
	require.Equal(t, "projects/proj-1/locations/global/publishers/google/models/gemini-3.1-pro-preview", model)

	collection, err = geminiCachedContentCollectionURL(account, "", "gemini-3.1-pro-preview")
	require.NoError(t, err)
	require.Equal(t, "https://aiplatform.googleapis.com/v1/projects/proj-1/locations/global/cachedContents", collection)

	resource, err := geminiCachedContentResourceURL(account, "", "projects/123/locations/us/cachedContents/456")
	require.NoError(t, err)
	require.Equal(t, "https://aiplatform.us.rep.googleapis.com/v1/projects/123/locations/us/cachedContents/456", resource)

	resource, err = geminiCachedContentResourceURL(account, "", "projects/123/locations/global/cachedContents/456")
	require.NoError(t, err)
	require.Equal(t, "https://aiplatform.googleapis.com/v1/projects/123/locations/global/cachedContents/456", resource)
}

func TestBuildVertexPublisherModelsURLMultiRegionHost(t *testing.T) {
	u, err := buildVertexPublisherModelsURL("eu", "")
	require.NoError(t, err)
	require.Equal(t, "https://aiplatform.eu.rep.googleapis.com/v1beta1/publishers/google/models?pageSize=300", u)
}

func TestBuildVertexBatchPredictionJobsEndpointMultiRegionHost(t *testing.T) {
	got, err := BuildVertexBatchPredictionJobsEndpoint("", "my-project", "us")
	require.NoError(t, err)
	require.Equal(t, "https://aiplatform.us.rep.googleapis.com/v1/projects/my-project/locations/us/batchPredictionJobs", got)
}
