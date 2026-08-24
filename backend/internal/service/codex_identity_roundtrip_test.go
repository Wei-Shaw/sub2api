package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestApplyCodexIdentityPlanProfileAdapterHeaderBodyCoherence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	input := codexRuntimeAttemptInput(t)
	profile, err := ResolveCodexRuntimeProfile(CodexOSProfilePolicy{
		OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceDesktop,
		Architecture: CodexArchX8664, SlotCount: 1, Epoch: 3, CatalogVersion: 1,
	})
	require.NoError(t, err)
	input.Profile = profile
	input.Slot.Epoch = 3
	plan, err := BuildCodexIdentityAttemptPlan(input)
	require.NoError(t, err)

	payload := []byte(`{
		"model":"gpt-5.6-sol",
		"client_metadata":{
			"os":"linux","arch":"arm64","surface":"cli","originator":"codex_cli_rs",
			"x-codex-installation-id":"client-install","session-id":"client-session","x-codex-window-id":"client-window:0",
			"conversation_id":"client-conversation",
			"x-codex-turn-metadata":"{\"os\":\"linux\",\"arch\":\"arm64\",\"session_id\":\"client-session\"}"
		},
		"metadata":{"os":"linux","session_id":"client-session"},
		"input":[{"role":"user","content":"client-session must remain ordinary content"}]
	}`)
	rewritten, changed, err := ApplyCodexIdentityPlanToJSON(payload, plan)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "windows", gjson.GetBytes(rewritten, "client_metadata.os").String())
	require.Equal(t, "x86_64", gjson.GetBytes(rewritten, "client_metadata.arch").String())
	require.Equal(t, "desktop", gjson.GetBytes(rewritten, "client_metadata.surface").String())
	require.Equal(t, profile.Originator, gjson.GetBytes(rewritten, "client_metadata.originator").String())
	require.Equal(t, profile.Version, gjson.GetBytes(rewritten, "client_metadata.version").String())
	require.Equal(t, profile.AppBuild, gjson.GetBytes(rewritten, "client_metadata.app_build").String())
	require.Equal(t, profile.UserAgent, gjson.GetBytes(rewritten, "client_metadata.user_agent").String())
	require.False(t, gjson.GetBytes(rewritten, "client_metadata.x-codex-installation-id").Exists())
	require.False(t, gjson.GetBytes(rewritten, "client_metadata.session-id").Exists())
	require.False(t, gjson.GetBytes(rewritten, "client_metadata.x-codex-window-id").Exists())
	require.Equal(t, plan.UpstreamValue(CodexIdentityConversation), gjson.GetBytes(rewritten, "client_metadata.conversation_id").String())
	require.Equal(t, "client-session must remain ordinary content", gjson.GetBytes(rewritten, "input.0.content").String())
	require.Equal(t, "windows", gjson.GetBytes(rewritten, "metadata.os").String())
	require.Equal(t, plan.UpstreamValue(CodexIdentitySession), gjson.GetBytes(rewritten, "metadata.session_id").String())

	embedded := gjson.GetBytes(rewritten, "client_metadata.x-codex-turn-metadata").String()
	require.Equal(t, "windows", gjson.Get(embedded, "os").String())
	require.Equal(t, "x86_64", gjson.Get(embedded, "arch").String())
	require.Equal(t, plan.UpstreamValue(CodexIdentitySession), gjson.Get(embedded, "session_id").String())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	account := &Account{ID: input.AccountID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	stageCodexIdentityAttemptPlan(c, plan)
	headers := http.Header{
		"Originator": {"client"}, "User-Agent": {"client/1"},
		"X-Codex-Turn-Metadata": {`{"session_id":"client-session","conversation_id":"client-conversation","os":"linux","arch":"arm64","surface":"cli","workspace":"/home/alice/private-project","cwd":"/home/alice/private-project","git_remote":"git@example.com:private/project.git","git_sha":"0123456789012345678901234567890123456789","sandbox":"read-only"}`},
	}
	require.True(t, applyStagedCodexProfileHeaders(c, account, headers))
	enforceCodexIdentityHeadersForAttempt(c, account, headers, "")
	require.Equal(t, profile.UserAgent, headers.Get("user-agent"))
	require.Equal(t, profile.Originator, headers.Get("originator"))
	require.Equal(t, profile.Version, headers.Get("version"))
	require.Equal(t, plan.UpstreamValue(CodexIdentityConversation), headers.Get("conversation_id"))
	headerMetadata := headers.Get("x-codex-turn-metadata")
	require.Equal(t, plan.UpstreamValue(CodexIdentitySession), gjson.Get(headerMetadata, "session_id").String())
	require.Equal(t, plan.UpstreamValue(CodexIdentityConversation), gjson.Get(headerMetadata, "conversation_id").String())
	require.Equal(t, "windows", gjson.Get(headerMetadata, "os").String())
	require.Equal(t, "x86_64", gjson.Get(headerMetadata, "arch").String())
	require.Equal(t, "desktop", gjson.Get(headerMetadata, "surface").String())
	require.Equal(t, plan.UpstreamValue(CodexIdentityWorkspace), gjson.Get(headerMetadata, "workspace").String())
	require.Equal(t, plan.UpstreamValue(CodexIdentityWorkspace), gjson.Get(headerMetadata, "cwd").String())
	require.Equal(t, plan.UpstreamValue(CodexIdentityGitRemote), gjson.Get(headerMetadata, "git_remote").String())
	require.Equal(t, plan.UpstreamValue(CodexIdentityGitCommit), gjson.Get(headerMetadata, "git_sha").String())
	require.Equal(t, "read-only", gjson.Get(headerMetadata, "sandbox").String())
	require.NotContains(t, headerMetadata, "/home/alice/private-project")
	require.NotContains(t, headerMetadata, "git@example.com:private/project.git")
	require.Equal(t, gjson.GetBytes(rewritten, "client_metadata.user_agent").String(), headers.Get("user-agent"))
	require.Equal(t, gjson.GetBytes(rewritten, "client_metadata.originator").String(), headers.Get("originator"))
	require.Equal(t, gjson.GetBytes(rewritten, "client_metadata.version").String(), headers.Get("version"))
}

func TestApplyCodexIdentityPlanRejectsMalformedKnownContainers(t *testing.T) {
	plan := codexRuntimeRoundtripPlan(t)
	for _, payload := range [][]byte{
		[]byte(`{"client_metadata":"not-an-object"}`),
		[]byte(`{"metadata":42}`),
		[]byte(`{"client_metadata":{"turn_metadata":[]}}`),
		[]byte(`{"request":"not-an-object"}`),
	} {
		rewritten, changed, err := ApplyCodexIdentityPlanToJSON(payload, plan)
		require.Error(t, err)
		require.False(t, changed)
		require.Equal(t, payload, rewritten)
	}
}

func TestCodexIdentityRoundTripSupportsMetadataOnlyContainers(t *testing.T) {
	for _, tt := range []struct {
		name       string
		payload    []byte
		assertPath string
		want       string
	}{
		{name: "metadata", payload: []byte(`{"metadata":{"session_id":"metadata-session","os":"linux"}}`), assertPath: "metadata.session_id", want: "metadata-session"},
		{name: "request client metadata", payload: []byte(`{"request":{"client_metadata":{"session_id":"request-session","os":"linux"}}}`), assertPath: "request.client_metadata.session_id", want: "request-session"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := codexRuntimeAttemptInput(t)
			input.Source = ExtractCodexIdentitySource(nil, tt.payload)
			plan, err := BuildCodexIdentityAttemptPlan(input)
			require.NoError(t, err)
			rewritten, changed, err := ApplyCodexIdentityPlanToJSON(tt.payload, plan)
			require.NoError(t, err)
			require.True(t, changed)
			restored, changed, err := RestoreCodexIdentityJSON(rewritten, plan)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, tt.want, gjson.GetBytes(restored, tt.assertPath).String())
			require.Equal(t, "linux", gjson.GetBytes(restored, strings.TrimSuffix(tt.assertPath, "session_id")+"os").String())
		})
	}
}

func TestCodexProfileRoundTripRestoresFieldsWhoseCanonicalValueIsEmpty(t *testing.T) {
	profile, err := ResolveCodexRuntimeProfile(CodexOSProfilePolicy{
		OSClass: CodexOSGeneric, CanonicalSurface: CodexSurfaceSDK,
		SlotCount: 1, Epoch: 1, CatalogVersion: 1,
	})
	require.NoError(t, err)
	payload := []byte(`{"client_metadata":{"session_id":"client-session","arch":"arm64","architecture":"arm64","app_build":"client-build"}}`)
	input := codexRuntimeAttemptInput(t)
	input.Profile = profile
	input.Source = ExtractCodexIdentitySource(nil, payload)
	plan, err := BuildCodexIdentityAttemptPlan(input)
	require.NoError(t, err)
	rewritten, changed, err := ApplyCodexIdentityPlanToJSON(payload, plan)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "", gjson.GetBytes(rewritten, "client_metadata.arch").String())
	require.Equal(t, "", gjson.GetBytes(rewritten, "client_metadata.app_build").String())
	restored, changed, err := RestoreCodexIdentityJSON(rewritten, plan)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "arm64", gjson.GetBytes(restored, "client_metadata.arch").String())
	require.Equal(t, "arm64", gjson.GetBytes(restored, "client_metadata.architecture").String())
	require.Equal(t, "client-build", gjson.GetBytes(restored, "client_metadata.app_build").String())
}

func TestRestoreCodexIdentityJSONKnownFieldsOnly(t *testing.T) {
	plan := codexRuntimeRoundtripPlan(t)
	install := plan.UpstreamValue(CodexIdentityInstallation)
	session := plan.UpstreamValue(CodexIdentitySession)
	conversation := plan.UpstreamValue(CodexIdentityConversation)
	thread := plan.UpstreamValue(CodexIdentityThread)
	turn := plan.UpstreamValue(CodexIdentityTurn)
	workspace := plan.UpstreamValue(CodexIdentityWorkspace)
	remote := plan.UpstreamValue(CodexIdentityGitRemote)
	commit := plan.UpstreamValue(CodexIdentityGitCommit)
	metadata, err := json.Marshal(map[string]any{
		"installation_id": install,
		"session_id":      session,
		"thread_id":       thread,
		"turn_id":         turn,
		"sandbox":         "read-only",
	})
	require.NoError(t, err)

	payload, err := json.Marshal(map[string]any{
		"type":            "response.completed",
		"session_id":      session,
		"conversation_id": conversation,
		"client_metadata": map[string]any{
			"x-codex-installation-id":   install,
			"thread_id":                 thread,
			"workspace_path":            workspace,
			"git_remote_url":            remote,
			"git_sha":                   commit,
			"x-codex-turn-metadata":     string(metadata),
			"unrelated_identity_shadow": session,
		},
		"headers": map[string]any{
			"X-Codex-Installation-Id": []any{install},
		},
		"response": map[string]any{
			"metadata": map[string]any{"turn_id": turn},
			"output": []any{
				map[string]any{
					"type":       "message",
					"session_id": session,
					"content":    "ordinary output contains " + session,
				},
			},
		},
		"unknown": map[string]any{"session_id": session},
	})
	require.NoError(t, err)

	restored, changed, err := RestoreCodexIdentityJSON(payload, plan)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "client-session", gjson.GetBytes(restored, "session_id").String())
	require.Equal(t, "client-conversation", gjson.GetBytes(restored, "conversation_id").String())
	require.Equal(t, "client-install", gjson.GetBytes(restored, "client_metadata.x-codex-installation-id").String())
	require.Equal(t, "client-thread", gjson.GetBytes(restored, "client_metadata.thread_id").String())
	require.Equal(t, "/home/alice/private-project", gjson.GetBytes(restored, "client_metadata.workspace_path").String())
	require.Equal(t, "git@example.com:private/project.git", gjson.GetBytes(restored, "client_metadata.git_remote_url").String())
	require.Equal(t, "0123456789012345678901234567890123456789", gjson.GetBytes(restored, "client_metadata.git_sha").String())
	require.Equal(t, "client-install", gjson.GetBytes(restored, "headers.X-Codex-Installation-Id.0").String())

	embedded := gjson.GetBytes(restored, "client_metadata.x-codex-turn-metadata").String()
	require.Equal(t, "client-install", gjson.Get(embedded, "installation_id").String())
	require.Equal(t, "client-session", gjson.Get(embedded, "session_id").String())
	require.Equal(t, "client-thread", gjson.Get(embedded, "thread_id").String())
	require.Equal(t, "client-turn", gjson.Get(embedded, "turn_id").String())
	require.Equal(t, "read-only", gjson.Get(embedded, "sandbox").String())

	// Model output and unknown containers are deliberately outside the identity
	// response schema, even when their keys look like identity fields.
	require.Equal(t, session, gjson.GetBytes(restored, "response.output.0.session_id").String())
	require.Equal(t, "ordinary output contains "+session, gjson.GetBytes(restored, "response.output.0.content").String())
	require.Equal(t, session, gjson.GetBytes(restored, "unknown.session_id").String())
	require.Equal(t, session, gjson.GetBytes(restored, "client_metadata.unrelated_identity_shadow").String())
}

func TestRestoreCodexIdentityJSONNoPlanAndMalformed(t *testing.T) {
	payload := []byte(`{"session_id":"unchanged"}`)
	restored, changed, err := RestoreCodexIdentityJSON(payload, nil)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, payload, restored)

	plan := codexRuntimeRoundtripPlan(t)
	malformed := []byte(`{"session_id":`)
	restored, changed, err = RestoreCodexIdentityJSON(malformed, plan)
	require.Error(t, err)
	require.False(t, changed)
	require.Equal(t, malformed, restored)

	trailing := []byte(`{"session_id":"x"}{"session_id":"y"}`)
	_, _, err = RestoreCodexIdentityJSON(trailing, plan)
	require.ErrorContains(t, err, "trailing value")
}

func TestRestoreCodexIdentitySSEPreservesFramingAndNonJSON(t *testing.T) {
	plan := codexRuntimeRoundtripPlan(t)
	session := plan.UpstreamValue(CodexIdentitySession)
	thread := plan.UpstreamValue(CodexIdentityThread)
	stream := "event: response.completed\r\n" +
		`data: {"response":{"metadata":{"session_id":"` + session + `","thread_id":"` + thread + `"},"output":[{"type":"message","content":"` + session + `"}]}}` + "\r\n\r\n" +
		"data: [DONE]\r\n\r\n" +
		": keepalive " + session + "\r\n\r\n" +
		"data: not-json " + session + "\r\n\r\n"

	restored, changed, err := RestoreCodexIdentitySSE([]byte(stream), plan)
	require.NoError(t, err)
	require.True(t, changed)
	body := string(restored)
	require.Contains(t, body, "\r\n")
	require.Contains(t, body, `"session_id":"client-session"`)
	require.Contains(t, body, `"thread_id":"client-thread"`)
	require.Contains(t, body, `"content":"`+session+`"`)
	require.Contains(t, body, "data: [DONE]\r\n")
	require.Contains(t, body, ": keepalive "+session)
	require.Contains(t, body, "data: not-json "+session)
}

func TestRestoreCodexIdentitySSEMultiLineAndConcatenatedJSON(t *testing.T) {
	plan := codexRuntimeRoundtripPlan(t)
	session := plan.UpstreamValue(CodexIdentitySession)
	thread := plan.UpstreamValue(CodexIdentityThread)
	multiLine := "event: response.metadata\n" +
		"data: {\"client_metadata\":\n" +
		"data: {\"session_id\":\"" + session + "\"}}\n\n"
	restored, changed, err := RestoreCodexIdentitySSE([]byte(multiLine), plan)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, strings.Count(string(restored), "data:"))
	require.Contains(t, string(restored), `"session_id":"client-session"`)

	concatenated := "data: {\"type\":\"response.created\",\"session_id\":\"" + session + "\"}{\"type\":\"response.completed\",\"thread_id\":\"" + thread + "\"}\n\n"
	restored, changed, err = RestoreCodexIdentitySSE([]byte(concatenated), plan)
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, string(restored), `"session_id":"client-session"`)
	require.Contains(t, string(restored), `"thread_id":"client-thread"`)
	require.Contains(t, string(restored), `"type":"response.created"`)
	require.Contains(t, string(restored), `"type":"response.completed"`)
}

func TestRestoreCodexIdentityWSEvent(t *testing.T) {
	plan := codexRuntimeRoundtripPlan(t)
	turn := plan.UpstreamValue(CodexIdentityTurn)
	window := plan.UpstreamValue(CodexIdentityWindow)
	payload := []byte(`{"type":"response.created","response":{"client_metadata":{"turn_id":"` + turn + `","window_id":"` + window + `"},"output_text":"` + turn + `"}}`)
	restored, changed, err := RestoreCodexIdentityWSEvent(payload, plan)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "client-turn", gjson.GetBytes(restored, "response.client_metadata.turn_id").String())
	require.Equal(t, "client-window:0", gjson.GetBytes(restored, "response.client_metadata.window_id").String())
	require.Equal(t, turn, gjson.GetBytes(restored, "response.output_text").String())
}

func TestRestoreCodexIdentityRemovesGatewayAddedAliasesAcrossJSONSSEAndWS(t *testing.T) {
	input := codexRuntimeAttemptInput(t)
	input.Source = CodexIdentitySource{}
	input.ConversationKey = "gateway-generated-conversation"
	input.RequestNonce = "gateway-generated-request"
	plan, err := BuildCodexIdentityAttemptPlan(input)
	require.NoError(t, err)
	installation := plan.UpstreamValue(CodexIdentityInstallation)
	session := plan.UpstreamValue(CodexIdentitySession)
	payload := []byte(`{"type":"response.completed","response":{"client_metadata":{"installation_id":"` + installation + `","session_id":"` + session + `"},"output_text":"` + installation + `"}}`)

	jsonBody, changed, err := RestoreCodexIdentityJSON(payload, plan)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(jsonBody, "response.client_metadata.installation_id").Exists())
	require.False(t, gjson.GetBytes(jsonBody, "response.client_metadata.session_id").Exists())
	require.Equal(t, installation, gjson.GetBytes(jsonBody, "response.output_text").String())

	sseBody, changed, err := RestoreCodexIdentitySSE(append([]byte("data: "), append(payload, '\n', '\n')...), plan)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotContains(t, string(sseBody), `"installation_id"`)
	require.Contains(t, string(sseBody), `"output_text":"`+installation+`"`)

	wsBody, changed, err := RestoreCodexIdentityWSEvent(payload, plan)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(wsBody, "response.client_metadata.installation_id").Exists())
	require.Equal(t, installation, gjson.GetBytes(wsBody, "response.output_text").String())
}

func codexRuntimeRoundtripPlan(t *testing.T) *CodexIdentityAttemptPlan {
	t.Helper()
	plan, err := BuildCodexIdentityAttemptPlan(codexRuntimeAttemptInput(t))
	require.NoError(t, err)
	require.NotNil(t, plan)
	return plan
}
