package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildVertexGeminiURL(t *testing.T) {
	got, err := buildVertexGeminiURL("my-project", "us-central1", "gemini-3-pro", "streamGenerateContent", true)
REDACTED
	require.Equal(t, "https://us-central1-aiplatform.googleapis.com/v1/projects/my-project/locations/us-central1/publishers/google/models/gemini-3-pro:streamGenerateContent?alt=sse", got)
REDACTED

func TestBuildVertexGeminiURLUsesGlobalEndpointHost(t *testing.T) {
	got, err := buildVertexGeminiURL("my-project", "global", "gemini-3-flash-preview", "streamGenerateContent", true)
REDACTED
	require.Equal(t, "https://aiplatform.googleapis.com/v1/projects/my-project/locations/global/publishers/google/models/gemini-3-flash-preview:streamGenerateContent?alt=sse", got)
REDACTED

func TestBuildVertexAnthropicURL(t *testing.T) {
	got, err := buildVertexAnthropicURL("my-project", "us-east5", "claude-sonnet-4-5@20250929", false)
REDACTED
	require.Equal(t, "https://us-east5-aiplatform.googleapis.com/v1/projects/my-project/locations/us-east5/publishers/anthropic/models/claude-sonnet-4-5@20250929:rawPredict", got)
REDACTED

func TestBuildVertexAnthropicURLUsesGlobalEndpointHost(t *testing.T) {
	got, err := buildVertexAnthropicURL("my-project", "global", "claude-haiku-4-5@20251001", true)
REDACTED
	require.Equal(t, "https://aiplatform.googleapis.com/v1/projects/my-project/locations/global/publishers/anthropic/models/claude-haiku-4-5@20251001:streamRawPredict", got)
REDACTED

func TestNormalizeVertexAnthropicModelID(t *testing.T) {
	require.Equal(t, "claude-sonnet-4-5@20250929", normalizeVertexAnthropicModelID("claude-sonnet-4-5-20250929"))
	require.Equal(t, "claude-sonnet-4-5@20250929", normalizeVertexAnthropicModelID("claude-sonnet-4-5@20250929"))
	require.Equal(t, "claude-sonnet-4-6", normalizeVertexAnthropicModelID("claude-sonnet-4-6"))
REDACTED

func TestBuildVertexAnthropicRequestBody(t *testing.T) {
	got, err := buildVertexAnthropicRequestBody([]byte(`{"model":"claude-sonnet-4-5","anthropic_version":"2023-06-01","max_tokens":64,"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`))
REDACTED
	require.Equal(t, "", gjson.GetBytes(got, "model").String())
	require.Equal(t, vertexAnthropicVersion, gjson.GetBytes(got, "anthropic_version").String())
	require.Equal(t, int64(64), gjson.GetBytes(got, "max_tokens").Int())
	require.Equal(t, "hi", gjson.GetBytes(got, "messages.0.content").String())
REDACTED

func TestBuildVertexGeminiURLRejectsInvalidLocation(t *testing.T) {
	_, err := buildVertexGeminiURL("my-project", "us-central1/path", "gemini-3-pro", "generateContent", false)
REDACTED
	require.Contains(t, err.Error(), "invalid vertex location")
REDACTED

func TestParseVertexServiceAccountKey(t *testing.T) {
	raw := `{
		"type": "service_account",
		"project_id": "vertex-proj",
		"private_key_id": "kid",
		"private_key": "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n",
		"client_email": "svc@vertex-proj.iam.gserviceaccount.com"
REDACTED`
	account := &Account{
REDACTED
REDACTED
REDACTED
			"service_account_json": raw,
	REDACTED,
REDACTED
	key, err := parseVertexServiceAccountKey(account)
REDACTED
	require.Equal(t, "vertex-proj", key.ProjectID)
	require.Equal(t, "svc@vertex-proj.iam.gserviceaccount.com", key.ClientEmail)
	require.Equal(t, vertexDefaultTokenURL, key.TokenURI)
	require.True(t, strings.Contains(key.PrivateKey, "BEGIN PRIVATE KEY"))
REDACTED
