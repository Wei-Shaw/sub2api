//go:build unit

package service

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newGrokCacheTestContext(apiKeyID int64) *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if apiKeyID > 0 {
		c.Set("api_key", &APIKey{ID: apiKeyID, Group: &Group{Platform: PlatformGrokREDACTEDREDACTED)
REDACTED
	return c
REDACTED

func TestResolveGrokCacheIdentityStableAcrossAppendOnlyTurns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newGrokCacheTestContext(101)
	round1 := []byte(`{"model":"grok","instructions":"be concise","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"REDACTEDREDACTED],"input":[{"role":"user","content":"first question"REDACTED]REDACTED`)
	round2 := []byte(`{"model":"grok","instructions":"be concise","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"REDACTEDREDACTED],"input":[{"role":"user","content":"first question"REDACTED,{"role":"assistant","content":"first answer"REDACTED,{"role":"user","content":"second question"REDACTED]REDACTED`)

	first := resolveGrokCacheIdentity(c, round1, "", "grok-4.5")
	second := resolveGrokCacheIdentity(c, round2, "", "grok-4.5")

	require.NotEmpty(t, first)
	require.Len(t, first, 36)
	require.Equal(t, first, second)
REDACTED

func TestResolveGrokCacheIdentityStableAcrossIndependentPromptsWithSamePrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newGrokCacheTestContext(102)
	firstBody := []byte(`{"model":"grok","instructions":"be concise","tools":[{"type":"function","name":"lookup"REDACTED],"input":[{"role":"user","content":"Question A"REDACTED]REDACTED`)
	secondBody := []byte(`{"model":"grok","instructions":"be concise","tools":[{"type":"function","name":"lookup"REDACTED],"input":[{"role":"user","content":"Question B"REDACTED]REDACTED`)

	first := resolveGrokCacheIdentity(c, firstBody, "", "grok-4.5")
	second := resolveGrokCacheIdentity(c, secondBody, "", "grok-4.5")

	require.NotEmpty(t, first)
	require.Equal(t, first, second)
REDACTED

func TestResolveGrokCacheIdentityStablePrefixIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	baseBody := []byte(`{"model":"grok","instructions":"be concise","tools":[{"type":"function","name":"lookup"REDACTED],"input":[{"role":"system","content":"System A"REDACTED,{"role":"user","content":"Question A"REDACTED]REDACTED`)
	differentInstructions := []byte(`{"model":"grok","instructions":"be detailed","tools":[{"type":"function","name":"lookup"REDACTED],"input":[{"role":"system","content":"System A"REDACTED,{"role":"user","content":"Question B"REDACTED]REDACTED`)
	differentSystem := []byte(`{"model":"grok","instructions":"be concise","tools":[{"type":"function","name":"lookup"REDACTED],"input":[{"role":"system","content":"System B"REDACTED,{"role":"user","content":"Question B"REDACTED]REDACTED`)
	differentTools := []byte(`{"model":"grok","instructions":"be concise","tools":[{"type":"function","name":"search"REDACTED],"input":[{"role":"system","content":"System A"REDACTED,{"role":"user","content":"Question B"REDACTED]REDACTED`)

	base := resolveGrokCacheIdentity(newGrokCacheTestContext(103), baseBody, "", "grok-4.5")
	require.NotEqual(t, base, resolveGrokCacheIdentity(newGrokCacheTestContext(104), baseBody, "", "grok-4.5"))
	require.NotEqual(t, base, resolveGrokCacheIdentity(newGrokCacheTestContext(103), baseBody, "", "grok-4.3"))
	require.NotEqual(t, base, resolveGrokCacheIdentity(newGrokCacheTestContext(103), differentInstructions, "", "grok-4.5"))
	require.NotEqual(t, base, resolveGrokCacheIdentity(newGrokCacheTestContext(103), differentSystem, "", "grok-4.5"))
	require.NotEqual(t, base, resolveGrokCacheIdentity(newGrokCacheTestContext(103), differentTools, "", "grok-4.5"))
REDACTED

func TestResolveGrokCacheIdentityFallsBackWhenStablePrefixIsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newGrokCacheTestContext(105)
	firstBody := []byte(`{"model":"grok","tools":[],"input":"Question A"REDACTED`)
	secondBody := []byte(`{"model":"grok","tools":[],"input":"Question B"REDACTED`)

	first := resolveGrokCacheIdentity(c, firstBody, "", "grok-4.5")
	second := resolveGrokCacheIdentity(c, secondBody, "", "grok-4.5")

	require.NotEmpty(t, first)
	require.NotEmpty(t, second)
	require.NotEqual(t, first, second)
REDACTED

func TestResolveGrokCacheIdentitySkipsUnanchoredFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newGrokCacheTestContext(106)
	tests := [][]byte{
		[]byte(`{"model":"grok"REDACTED`),
		[]byte(`{"model":"grok","messages":[{"role":"assistant","content":"answer"REDACTED]REDACTED`),
		[]byte(`{"model":"grok","messages":[{"role":"user","content":""REDACTED]REDACTED`),
		[]byte(`{"model":"grok","input":"  "REDACTED`),
REDACTED

	for _, body := range tests {
		require.Empty(t, resolveGrokCacheIdentity(c, body, "", "grok-4.5"))
REDACTED
REDACTED

func TestResolveGrokCacheIdentityIsolatesAPIKeyAndMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"grok","input":"same prompt"REDACTED`)

	base := resolveGrokCacheIdentity(newGrokCacheTestContext(201), body, "", "grok-4.5")
	otherTenant := resolveGrokCacheIdentity(newGrokCacheTestContext(202), body, "", "grok-4.5")
	otherModel := resolveGrokCacheIdentity(newGrokCacheTestContext(201), body, "", "grok-4.3")

	require.NotEmpty(t, base)
	require.NotEqual(t, base, otherTenant)
	require.NotEqual(t, base, otherModel)
REDACTED

func TestResolveGrokCacheIdentityUsesAndIsolatesNativeConversationHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newGrokCacheTestContext(301)
	c.Request.Header.Set(grokConversationIDHeader, "raw-native-conversation")
	body1 := []byte(`{"model":"grok","input":"one"REDACTED`)
	body2 := []byte(`{"model":"grok","input":"different body that must not replace the explicit session"REDACTED`)

	first := resolveGrokCacheIdentity(c, body1, "body-cache-key", "grok-4.5")
	second := resolveGrokCacheIdentity(c, body2, "another-body-cache-key", "grok-4.5")

	require.Equal(t, "raw-native-conversation", (&OpenAIGatewayService{REDACTED).ExtractSessionID(c, body1))
	require.Equal(t, first, second)
	require.NotEqual(t, "raw-native-conversation", first)
	require.NotContains(t, first, "raw-native-conversation")
REDACTED

func TestResolveGrokCacheIdentityExplicitHeaderPriority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"grok","prompt_cache_key":"body-key","input":"hi"REDACTED`)
	c := newGrokCacheTestContext(401)
	c.Request.Header.Set(grokConversationIDHeader, "grok-key")
	c.Request.Header.Set("conversation_id", "conversation-key")
	c.Request.Header.Set("session_id", "session-key")

	got := resolveGrokCacheIdentity(c, body, "explicit-argument", "grok-4.5")
	onlySession := newGrokCacheTestContext(401)
	onlySession.Request.Header.Set("session_id", "session-key")
	want := resolveGrokCacheIdentity(onlySession, []byte(`{"model":"grok","input":"unrelated"REDACTED`), "", "grok-4.5")

	require.Equal(t, want, got)
REDACTED

func TestResolveGrokCacheIdentityFailsClosedWithoutAPIKeyContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newGrokCacheTestContext(0)
	c.Request.Header.Set(grokConversationIDHeader, "native-session")

	require.Empty(t, resolveGrokCacheIdentity(c, []byte(`{"model":"grok","input":"hi"REDACTED`), "", "grok-4.5"))
	require.Empty(t, resolveGrokCacheIdentity(nil, []byte(`{"model":"grok","prompt_cache_key":"key"REDACTED`), "key", "grok-4.5"))
REDACTED

func TestGrokConversationHeaderIsScopedToGrokRequestScheduling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"grok","prompt_cache_key":"body-session","input":"hi"REDACTED`)

	grokContext := newGrokCacheTestContext(601)
	grokContext.Request.Header.Set(grokConversationIDHeader, "native-grok-session")
	require.Equal(t, "native-grok-session", (&OpenAIGatewayService{REDACTED).ExtractSessionID(grokContext, body))

	openAIContext := newGrokCacheTestContext(601)
	openAIContext.Set("api_key", &APIKey{ID: 601, Group: &Group{Platform: PlatformOpenAIREDACTEDREDACTED)
	openAIContext.Request.Header.Set(grokConversationIDHeader, "must-be-ignored")
	require.Equal(t, "body-session", (&OpenAIGatewayService{REDACTED).ExtractSessionID(openAIContext, body))

	withoutGrokHeader := newGrokCacheTestContext(601)
	withoutGrokHeader.Set("api_key", &APIKey{ID: 601, Group: &Group{Platform: PlatformOpenAIREDACTEDREDACTED)
	require.Equal(t,
		(&OpenAIGatewayService{REDACTED).GenerateSessionHash(withoutGrokHeader, body),
		(&OpenAIGatewayService{REDACTED).GenerateSessionHash(openAIContext, body),
	)
REDACTED

func TestApplyGrokCacheIdentityWritesResponsesBodyAndHeader(t *testing.T) {
	sourceBody := []byte(`{"model":"grok-4.5","prompt_cache_key":"raw-client-key"REDACTED`)
	body, err := applyGrokResponsesCacheIdentity(sourceBody, sourceBody, "isolated-id", true)
REDACTED
	require.Equal(t, "isolated-id", gjson.GetBytes(body, "prompt_cache_key").String())
	require.Equal(t, "web_search", gjson.GetBytes(body, "tools.0.type").String())
	require.Equal(t, "x_search", gjson.GetBytes(body, "tools.1.type").String())
	require.Equal(t, grokFreeCacheDisabledToolChoice, gjson.GetBytes(body, "tool_choice").String())

	headers := make(http.Header)
	headers.Set(grokConversationIDHeader, "spoofed-client-value")
	applyGrokCacheHeaders(headers, "isolated-id")
	require.Equal(t, "isolated-id", headers.Get(grokConversationIDHeader))
	applyGrokCacheHeaders(headers, "")
	require.Empty(t, headers.Get(grokConversationIDHeader))

	chatBody, err := stripGrokChatPromptCacheKey(body)
REDACTED
	require.False(t, gjson.GetBytes(chatBody, "prompt_cache_key").Exists())

	unscopedSourceBody := []byte(`{"model":"grok","prompt_cache_key":"raw-client-key"REDACTED`)
	unscopedBody, err := applyGrokResponsesCacheIdentity(unscopedSourceBody, unscopedSourceBody, "", true)
REDACTED
	require.False(t, gjson.GetBytes(unscopedBody, "prompt_cache_key").Exists())
	require.False(t, gjson.GetBytes(unscopedBody, "tools").Exists())
	require.False(t, gjson.GetBytes(unscopedBody, "tool_choice").Exists())
REDACTED

func TestGrokFreeMessagesClientToolCacheDefaultsOnForKnownFree(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(901, "access-token")
	account.Credentials["subscription_tier"] = " FREE "
	tests := []struct {
		name           string
		toolChoiceJSON string
		wantChoice     bool
REDACTED{
		{name: "missing tool choice"REDACTED,
		{name: "automatic tool choice", toolChoiceJSON: `,"tool_choice":"auto"`, wantChoice: trueREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intentBody := []byte(`{"model":"grok","tools":[{"type":"function","name":"lookup","description":"look up a value","parameters":{"type":"object"REDACTEDREDACTED,{"type":"function","name":"save","parameters":{"type":"object"REDACTEDREDACTED]` + tt.toolChoiceJSON + `REDACTED`)
			body, err := applyGrokResponsesCacheIdentity(intentBody, intentBody, "isolated-id", true)
		REDACTED
			body, err = applyGrokFreeMessagesFunctionToolCacheRoute(body, intentBody, account, "isolated-id")

		REDACTED
			require.Equal(t, "isolated-id", gjson.GetBytes(body, "prompt_cache_key").String())
			tools := gjson.GetBytes(body, "tools").Array()
			require.Len(t, tools, 4)
			require.Equal(t, "function", tools[0].Get("type").String())
			require.Equal(t, "lookup", tools[0].Get("name").String())
			require.Equal(t, "function", tools[1].Get("type").String())
			require.Equal(t, "save", tools[1].Get("name").String())
			require.Equal(t, "web_search", tools[2].Get("type").String())
			require.Equal(t, "x_search", tools[3].Get("type").String())
			require.Equal(t, tt.wantChoice, gjson.GetBytes(body, "tool_choice").Exists())
	REDACTED)
REDACTED
REDACTED

func TestGrokFreeMessagesClientToolCacheDefaultsWithMissingAccountSetting(t *testing.T) {
	body := []byte(`{"model":"grok","tools":[{"type":"function","name":"view_image","parameters":{"type":"object"REDACTEDREDACTED],"tool_choice":"auto"REDACTED`)
	tests := []struct {
		name  string
		extra map[string]any
REDACTED{
		{name: "nil extra"REDACTED,
		{name: "empty extra", extra: map[string]any{REDACTEDREDACTED,
		{name: "unrelated extra", extra: map[string]any{"other_setting": trueREDACTEDREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := healthyGrokOAuthGatewayTestAccount(90101, "access-token")
			account.Credentials["subscription_tier"] = "free"
			account.Extra = tt.extra

			patched, err := applyGrokFreeMessagesFunctionToolCacheRoute(body, body, account, "isolated-id")

		REDACTED
			tools := gjson.GetBytes(patched, "tools").Array()
			require.Len(t, tools, 3)
			require.Equal(t, "web_search", tools[1].Get("type").String())
			require.Equal(t, "x_search", tools[2].Get("type").String())
	REDACTED)
REDACTED
REDACTED

func TestApplyGrokCacheIdentityAppendsNativeToolsWhenSearchPresent(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(901, "access-token")
	account.Credentials["subscription_tier"] = " FREE "

	// Function tools INCLUDING web_search → convert + complement with x_search.
	intentBody := []byte(`{"model":"grok","tools":[{"type":"function","name":"lookup","description":"look up a value","parameters":{"type":"object"REDACTEDREDACTED,{"type":"function","name":"web_search","description":"search","parameters":{"type":"object"REDACTEDREDACTED]REDACTED`)
	body, err := applyGrokResponsesCacheIdentity(intentBody, intentBody, "isolated-id", true)
REDACTED
	body, err = applyGrokFreeMessagesFunctionToolCacheRoute(body, intentBody, account, "isolated-id")
REDACTED

	tools := gjson.GetBytes(body, "tools").Array()
	require.Len(t, tools, 3, "lookup(function) + web_search(native) + x_search(native)")
	require.Equal(t, "function", tools[0].Get("type").String())
	require.Equal(t, "lookup", tools[0].Get("name").String())
	require.Equal(t, "web_search", tools[1].Get("type").String())
	require.Equal(t, "x_search", tools[2].Get("type").String())
REDACTED

func TestGrokFreeClientToolCacheAccountOptIn(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(9011, "access-token")
	account.Credentials["subscription_tier"] = "free"
	account.Extra = map[string]any{grokClientToolCacheOptInExtraKey: trueREDACTED
	intentBody := []byte(`{"model":"grok","tools":[{"type":"function","name":"view_image","parameters":{"type":"object"REDACTEDREDACTED,{"type":"function","name":"read_file","parameters":{"type":"object"REDACTEDREDACTED],"tool_choice":"auto"REDACTED`)

	body, err := applyGrokResponsesCacheIdentity(intentBody, intentBody, "isolated-id", true)
REDACTED
	body, err = applyGrokFreeMessagesFunctionToolCacheRoute(body, intentBody, account, "isolated-id")
REDACTED

	tools := gjson.GetBytes(body, "tools").Array()
	require.Len(t, tools, 4)
	require.Equal(t, "view_image", tools[0].Get("name").String())
	require.Equal(t, "read_file", tools[1].Get("name").String())
	require.Equal(t, "web_search", tools[2].Get("type").String())
	require.Equal(t, "x_search", tools[3].Get("type").String())
REDACTED

func TestGrokFreeMessagesClientToolCacheAccountOptOut(t *testing.T) {
	body := []byte(`{"model":"grok","tools":[{"type":"function","name":"view_image","parameters":{"type":"object"REDACTEDREDACTED],"tool_choice":"auto"REDACTED`)
	tests := []struct {
		name  string
		value any
REDACTED{
		{name: "explicit false", value: falseREDACTED,
		{name: "string false is malformed", value: "false"REDACTED,
		{name: "numeric value is malformed", value: 1REDACTED,
		{name: "null value is malformed", value: nilREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := healthyGrokOAuthGatewayTestAccount(90111, "access-token")
			account.Credentials["subscription_tier"] = "free"
			account.Extra = map[string]any{grokClientToolCacheOptInExtraKey: tt.valueREDACTED

			patched, err := applyGrokFreeMessagesFunctionToolCacheRoute(body, body, account, "isolated-id")

		REDACTED
			require.JSONEq(t, string(body), string(patched))
	REDACTED)
REDACTED
REDACTED

func TestGrokFreeClientToolCacheRequestOptInOverridesAccountOptOut(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(9014, "access-token")
	account.Credentials["subscription_tier"] = "free"
	account.Extra = map[string]any{grokClientToolCacheOptInExtraKey: falseREDACTED
	c := newGrokCacheTestContext(9014)
	c.Request.Header.Set(grokClientToolCacheOptInHeader, "prefer-cache")
	intentBody := []byte(`{"model":"grok","tools":[{"type":"function","name":"view_image","parameters":{"type":"object"REDACTEDREDACTED],"tool_choice":"auto"REDACTED`)

	body, err := applyGrokResponsesCacheIdentity(intentBody, intentBody, "isolated-id", true)
REDACTED
	body, err = applyGrokFreeRequestToolCacheRoute(c, body, intentBody, account, "isolated-id")
REDACTED

	tools := gjson.GetBytes(body, "tools").Array()
	require.Len(t, tools, 3)
	require.Equal(t, "view_image", tools[0].Get("name").String())
	require.Equal(t, "web_search", tools[1].Get("type").String())
	require.Equal(t, "x_search", tools[2].Get("type").String())
REDACTED

func TestGrokFreeChatRequestClientToolCacheDefaultsOnWithoutClientFingerprint(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(90140, "access-token")
	account.Credentials["subscription_tier"] = "free"
	c := newGrokCacheTestContext(90140)
	c.Request.URL.Path = "/v1/chat/completions"
	body := []byte(`{"model":"grok","tools":[{"type":"function","name":"view_image","parameters":{"type":"object"REDACTEDREDACTED],"tool_choice":"auto"REDACTED`)

	patched, err := applyGrokFreeRequestToolCacheRoute(c, body, body, account, "isolated-id")

REDACTED
	tools := gjson.GetBytes(patched, "tools").Array()
	require.Len(t, tools, 3)
	require.Equal(t, "view_image", tools[0].Get("name").String())
	require.Equal(t, "web_search", tools[1].Get("type").String())
	require.Equal(t, "x_search", tools[2].Get("type").String())
REDACTED

func TestGrokFreeClientToolCacheClaudeDesktopResponsesAutoOptIn(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(90141, "access-token")
	account.Credentials["subscription_tier"] = "free"
	body := []byte(`{"model":"grok","tools":[{"type":"function","name":"Read","parameters":{"type":"object"REDACTEDREDACTED,{"type":"function","name":"Edit","parameters":{"type":"object"REDACTEDREDACTED],"tool_choice":"auto"REDACTED`)

	for _, xApp := range []string{"cli", "cli-bg"REDACTED {
		t.Run(xApp, func(t *testing.T) {
			c := newGrokCacheTestContext(90141)
			// The desktop marker text is intentionally not required; CC Switch and
			// Claude Desktop may change the descriptive User-Agent suffix.
			c.Request.Header.Set("User-Agent", "claude-cli/2.1.215 (external, future-desktop, agent-sdk/0.3.215)")
			c.Request.Header.Set("X-App", xApp)
			c.Request.Header.Set("anthropic-client-platform", "desktop_app")
			c.Request.Header.Set("X-Claude-Code-Session-Id", "desktop-session-1")

			patched, err := applyGrokFreeRequestToolCacheRoute(c, body, body, account, "isolated-id")

		REDACTED
			tools := gjson.GetBytes(patched, "tools").Array()
			require.Len(t, tools, 4)
			require.Equal(t, "Read", tools[0].Get("name").String())
			require.Equal(t, "Edit", tools[1].Get("name").String())
			require.Equal(t, "web_search", tools[2].Get("type").String())
			require.Equal(t, "x_search", tools[3].Get("type").String())
	REDACTED)
REDACTED
REDACTED

func TestGrokFreeClientToolCacheClaudeDesktopFingerprintRequiresAllSignals(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(90142, "access-token")
	account.Credentials["subscription_tier"] = "free"
	account.Extra = map[string]any{grokClientToolCacheOptInExtraKey: falseREDACTED
	body := []byte(`{"model":"grok","tools":[{"type":"function","name":"Read","parameters":{"type":"object"REDACTEDREDACTED],"tool_choice":"auto"REDACTED`)

	tests := []struct {
		name     string
		path     string
		ua       string
		xApp     string
		platform string
		session  string
REDACTED{
		{
			name:     "chat path",
			path:     "/v1/chat/completions",
			ua:       "claude-cli/2.1.215 (external, claude-desktop-3p, agent-sdk/0.3.215)",
			xApp:     "cli",
			platform: "desktop_app",
			session:  "desktop-session-1",
	REDACTED,
		{
			name:     "compact responses path",
			path:     "/v1/responses/compact",
			ua:       "claude-cli/2.1.215 (external, claude-desktop-3p, agent-sdk/0.3.215)",
			xApp:     "cli",
			platform: "desktop_app",
			session:  "desktop-session-1",
	REDACTED,
		{
			name:     "non claude cli user agent",
			path:     "/v1/responses",
			ua:       "Mozilla/5.0 (claude-desktop-3p)",
			xApp:     "cli",
			platform: "desktop_app",
			session:  "desktop-session-1",
	REDACTED,
		{
			name:     "missing x app",
			path:     "/v1/responses",
			ua:       "claude-cli/2.1.215 (external, claude-desktop-3p, agent-sdk/0.3.215)",
			platform: "desktop_app",
			session:  "desktop-session-1",
	REDACTED,
		{
			name:     "wrong x app",
			path:     "/v1/responses",
			ua:       "claude-cli/2.1.215 (external, claude-desktop-3p, agent-sdk/0.3.215)",
			xApp:     "desktop",
			platform: "desktop_app",
			session:  "desktop-session-1",
	REDACTED,
		{
			name:    "missing client platform",
			path:    "/v1/responses",
			ua:      "claude-cli/2.1.215 (external, claude-desktop-3p, agent-sdk/0.3.215)",
			xApp:    "cli",
			session: "desktop-session-1",
	REDACTED,
		{
			name:     "wrong client platform",
			path:     "/v1/responses",
			ua:       "claude-cli/2.1.215 (external, claude-desktop-3p, agent-sdk/0.3.215)",
			xApp:     "cli",
			platform: "web",
			session:  "desktop-session-1",
	REDACTED,
		{
			name:     "missing session header",
			path:     "/v1/responses",
			ua:       "claude-cli/2.1.215 (external, claude-desktop-3p, agent-sdk/0.3.215)",
			xApp:     "cli",
			platform: "desktop_app",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newGrokCacheTestContext(90142)
			c.Request.URL.Path = tt.path
			if tt.ua != "" {
				c.Request.Header.Set("User-Agent", tt.ua)
		REDACTED
			if tt.xApp != "" {
				c.Request.Header.Set("X-App", tt.xApp)
		REDACTED
			if tt.platform != "" {
				c.Request.Header.Set("anthropic-client-platform", tt.platform)
		REDACTED
			if tt.session != "" {
				c.Request.Header.Set("X-Claude-Code-Session-Id", tt.session)
		REDACTED

			patched, err := applyGrokFreeRequestToolCacheRoute(c, body, body, account, "isolated-id")

		REDACTED
			require.JSONEq(t, string(body), string(patched))
	REDACTED)
REDACTED
REDACTED

func TestGrokFreeClientToolCacheExplicitRequestOptOut(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(90144, "access-token")
	account.Credentials["subscription_tier"] = "free"
	body := []byte(`{"model":"grok","tools":[{"type":"function","name":"Read","parameters":{"type":"object"REDACTEDREDACTED],"tool_choice":"auto"REDACTED`)

	for _, value := range []string{"0", "false", "no", "off"REDACTED {
		t.Run(value, func(t *testing.T) {
			c := newGrokCacheTestContext(90144)
			c.Request.URL.Path = "/v1/chat/completions"
			c.Request.Header.Set(grokClientToolCacheOptInHeader, value)

			patched, err := applyGrokFreeRequestToolCacheRoute(c, body, body, account, "isolated-id")

		REDACTED
			require.JSONEq(t, string(body), string(patched))
	REDACTED)
REDACTED
REDACTED

func TestGrokFreeClientToolCacheClaudeDesktopAutoOptInDoesNotOverridePaidTier(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(90143, "access-token")
	account.Credentials["subscription_tier"] = "supergrok"
	c := newGrokCacheTestContext(90143)
	c.Request.Header.Set("User-Agent", "claude-cli/2.1.215 (external, claude-desktop-3p, agent-sdk/0.3.215)")
	c.Request.Header.Set("X-App", "cli")
	c.Request.Header.Set("anthropic-client-platform", "desktop_app")
	c.Request.Header.Set("X-Claude-Code-Session-Id", "desktop-session-1")
	body := []byte(`{"model":"grok","tools":[{"type":"function","name":"Read","parameters":{"type":"object"REDACTEDREDACTED],"tool_choice":"auto"REDACTED`)

	patched, err := applyGrokFreeRequestToolCacheRoute(c, body, body, account, "isolated-id")

REDACTED
	require.JSONEq(t, string(body), string(patched))
REDACTED

func TestGrokFreeRequestClientSearchFunctionUsesDefaultAccountPolicy(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(9015, "access-token")
	account.Credentials["subscription_tier"] = "free"
	c := newGrokCacheTestContext(9015)
	body := []byte(`{"model":"grok","tools":[{"type":"function","name":"view_image","parameters":{"type":"object"REDACTEDREDACTED,{"type":"function","name":"web_search","parameters":{"type":"object"REDACTEDREDACTED],"tool_choice":"auto"REDACTED`)

	patched, err := applyGrokFreeRequestToolCacheRoute(c, body, body, account, "isolated-id")

REDACTED
	tools := gjson.GetBytes(patched, "tools").Array()
	require.Len(t, tools, 3)
	require.Equal(t, "view_image", tools[0].Get("name").String())
	require.Equal(t, "web_search", tools[1].Get("type").String())
	require.Equal(t, "x_search", tools[2].Get("type").String())
REDACTED

func TestGrokFreeRequestToolChoiceNoneUsesSafeCacheRoute(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(9016, "access-token")
	account.Credentials["subscription_tier"] = "free"
	c := newGrokCacheTestContext(9016)
	body := []byte(`{"model":"grok","tools":[{"type":"function","name":"view_image","parameters":{"type":"object"REDACTEDREDACTED],"tool_choice":"none"REDACTED`)

	patched, err := applyGrokFreeRequestToolCacheRoute(c, body, body, account, "isolated-id")

REDACTED
	tools := gjson.GetBytes(patched, "tools").Array()
	require.Len(t, tools, 3)
	require.Equal(t, "view_image", tools[0].Get("name").String())
	require.Equal(t, "web_search", tools[1].Get("type").String())
	require.Equal(t, "x_search", tools[2].Get("type").String())
	require.Equal(t, "none", gjson.GetBytes(patched, "tool_choice").String())
REDACTED

func TestApplyGrokCacheIdentityRecognizesResponsesLiteAdditionalTools(t *testing.T) {
	intentBody := []byte(`{"model":"grok","input":[{"type":"additional_tools","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"REDACTEDREDACTED]REDACTED,{"type":"message","role":"user","content":"hello"REDACTED]REDACTED`)
	patchedBody := []byte(`{"model":"grok-4.5","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"REDACTEDREDACTED],"input":[{"type":"message","role":"user","content":"hello"REDACTED]REDACTED`)

	patched, err := applyGrokResponsesCacheIdentity(patchedBody, intentBody, "isolated-id", true)

REDACTED
	tools := gjson.GetBytes(patched, "tools").Array()
	require.Len(t, tools, 1)
	require.Equal(t, "lookup", tools[0].Get("name").String())
	require.False(t, gjson.GetBytes(patched, "tool_choice").Exists())
	require.Equal(t, "isolated-id", gjson.GetBytes(patched, "prompt_cache_key").String())
REDACTED

func TestGrokFreeCacheRoutePreservesMixedSupportedToolsWithSearchIntent(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(9012, "access-token")
	account.Credentials["subscription_tier"] = "free"
	intentBody := []byte(`{"model":"grok","tools":[{"type":"function","name":"view_image","parameters":{"type":"object"REDACTEDREDACTED,{"type":"shell"REDACTED,{"type":"web_search"REDACTED],"tool_choice":"auto"REDACTED`)

	body, err := applyGrokResponsesCacheIdentity(intentBody, intentBody, "isolated-id", true)
REDACTED
	body, err = applyGrokFreeMessagesFunctionToolCacheRoute(body, intentBody, account, "isolated-id")
REDACTED

	tools := gjson.GetBytes(body, "tools").Array()
	require.Len(t, tools, 4)
	require.Equal(t, "function", tools[0].Get("type").String())
	require.Equal(t, "shell", tools[1].Get("type").String())
	require.Equal(t, "web_search", tools[2].Get("type").String())
	require.Equal(t, "x_search", tools[3].Get("type").String())
REDACTED

func TestGrokClientToolCacheOptInDoesNotOverridePaidTier(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(9013, "access-token")
	account.Credentials["subscription_tier"] = "supergrok"
	account.Extra = map[string]any{grokClientToolCacheOptInExtraKey: trueREDACTED
	body := []byte(`{"model":"grok","tools":[{"type":"function","name":"view_image","parameters":{"type":"object"REDACTEDREDACTED],"tool_choice":"auto"REDACTED`)

	patched, err := applyGrokFreeMessagesFunctionToolCacheRoute(body, body, account, "isolated-id")

REDACTED
	require.JSONEq(t, string(body), string(patched))
REDACTED

func TestApplyGrokCacheIdentityRequiresPatchedFunctionTools(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(902, "access-token")
	account.Credentials["subscription_tier"] = "free"
	intentBody := []byte(`{"model":"grok","tools":[{"type":"function","name":"lookup"REDACTED],"tool_choice":"auto"REDACTED`)
	tests := []struct {
		name        string
		patchedBody string
REDACTED{
		{name: "missing tools", patchedBody: `{"model":"grok-4.5"REDACTED`REDACTED,
		{name: "empty tools", patchedBody: `{"model":"grok-4.5","tools":[]REDACTED`REDACTED,
		{name: "native tools only", patchedBody: `{"model":"grok-4.5","tools":[{"type":"web_search"REDACTED]REDACTED`REDACTED,
		{name: "unexpected patched tool", patchedBody: `{"model":"grok-4.5","tools":[{"type":"function","name":"lookup"REDACTED,{"type":"namespace","name":"server"REDACTED]REDACTED`REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeTools := gjson.Get(tt.patchedBody, "tools")
			body, err := applyGrokResponsesCacheIdentity([]byte(tt.patchedBody), intentBody, "isolated-id", true)
		REDACTED
			body, err = applyGrokFreeMessagesFunctionToolCacheRoute(body, intentBody, account, "isolated-id")

		REDACTED
			require.Equal(t, "isolated-id", gjson.GetBytes(body, "prompt_cache_key").String())
			afterTools := gjson.GetBytes(body, "tools")
			require.Equal(t, beforeTools.Exists(), afterTools.Exists())
			require.Equal(t, beforeTools.Raw, afterTools.Raw)
	REDACTED)
REDACTED
REDACTED

func TestGrokFreeMessagesFunctionToolCacheRouteRequiresKnownFreeTier(t *testing.T) {
	// Include web_search as a function to also cover its conversion to a native tool.
	intentBody := []byte(`{"model":"grok","tools":[{"type":"function","name":"lookup"REDACTED,{"type":"function","name":"web_search"REDACTED],"tool_choice":"auto"REDACTED`)
	tests := []struct {
		name    string
		account *Account
		wantMix bool
REDACTED{
		{
			name: "free credential tier",
			account: func() *Account {
				a := healthyGrokOAuthGatewayTestAccount(910, "access-token")
				a.Credentials["subscription_tier"] = "free"
				return a
		REDACTED(),
			wantMix: true,
	REDACTED,
		{
			name: "free billing tier",
			account: func() *Account {
				a := healthyGrokOAuthGatewayTestAccount(911, "access-token")
				a.Extra = map[string]any{grokBillingExtraKey: map[string]any{"plan": "FREE"REDACTEDREDACTED
				return a
		REDACTED(),
			wantMix: true,
	REDACTED,
		{
			name: "free successful billing has blank plan",
			account: func() *Account {
				a := healthyGrokOAuthGatewayTestAccount(9111, "access-token")
				a.Extra = map[string]any{grokBillingExtraKey: map[string]any{
					"status_code":        http.StatusOK,
					"source":             "billing_probe",
					"monthly_updated_at": "2026-07-15T05:00:00Z",
			REDACTEDREDACTED
				return a
		REDACTED(),
			wantMix: true,
	REDACTED,
		{
			name: "free rolling token quota",
			account: func() *Account {
				a := healthyGrokOAuthGatewayTestAccount(9112, "access-token")
				a.Extra = map[string]any{grokQuotaSnapshotExtraKey: map[string]any{
					"headers_observed": true,
					"tokens":           map[string]any{"limit": xai.GrokFreeRolling24hTokenLimitREDACTED,
			REDACTEDREDACTED
				return a
		REDACTED(),
			wantMix: true,
	REDACTED,
		{
			name: "legacy free rolling token quota",
			account: func() *Account {
				a := healthyGrokOAuthGatewayTestAccount(9113, "access-token")
				a.Extra = map[string]any{grokQuotaSnapshotExtraKey: map[string]any{
					"headers_observed": true,
					"tokens":           map[string]any{"limit": int64(2_000_000)REDACTED,
			REDACTEDREDACTED
				return a
		REDACTED(),
			wantMix: true,
	REDACTED,
		{
			name: "supergrok remains unchanged",
			account: func() *Account {
				a := healthyGrokOAuthGatewayTestAccount(912, "access-token")
				a.Credentials["subscription_tier"] = "supergrok"
				return a
		REDACTED(),
	REDACTED,
		{
			name: "paid billing overrides stale free quota",
			account: func() *Account {
				a := healthyGrokOAuthGatewayTestAccount(9121, "access-token")
				a.Extra = map[string]any{
					grokBillingExtraKey: map[string]any{"plan": "SuperGrok", "status_code": http.StatusOKREDACTED,
					grokQuotaSnapshotExtraKey: map[string]any{
						"headers_observed": true,
						"tokens":           map[string]any{"limit": int64(2_000_000)REDACTED,
				REDACTED,
			REDACTED
				return a
		REDACTED(),
	REDACTED,
		{
			name: "partial billing without monthly evidence remains unknown",
			account: func() *Account {
				a := healthyGrokOAuthGatewayTestAccount(9122, "access-token")
				a.Extra = map[string]any{grokBillingExtraKey: map[string]any{
					"status_code":    http.StatusOK,
					"source":         "billing_probe",
					"partial":        true,
					"failed_windows": []string{"monthly"REDACTED,
			REDACTEDREDACTED
				return a
		REDACTED(),
	REDACTED,
		{
			name:    "unknown tier remains unchanged",
			account: healthyGrokOAuthGatewayTestAccount(913, "access-token"),
	REDACTED,
		{
			name: "api key remains unchanged",
			account: &Account{
				ID:       914,
				Platform: PlatformGrok,
				Type:     AccountTypeAPIKey,
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := applyGrokFreeMessagesFunctionToolCacheRoute(intentBody, intentBody, tt.account, "isolated-id")

		REDACTED
			tools := gjson.GetBytes(body, "tools").Array()
			if tt.wantMix {
				require.Len(t, tools, 3)
				require.Equal(t, "web_search", tools[1].Get("type").String())
				require.Equal(t, "x_search", tools[2].Get("type").String())
				return
		REDACTED
			require.Len(t, tools, 2, "non-free accounts should not get native search injected")
	REDACTED)
REDACTED
REDACTED

func TestGrokFreeMessagesFunctionToolCacheRouteRequiresIdentity(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(915, "access-token")
	account.Credentials["subscription_tier"] = "free"
	body := []byte(`{"model":"grok","tools":[{"type":"function","name":"lookup"REDACTED],"tool_choice":"auto"REDACTED`)

	patched, err := applyGrokFreeMessagesFunctionToolCacheRoute(body, body, account, "")

REDACTED
	require.JSONEq(t, string(body), string(patched))
	require.Len(t, gjson.GetBytes(patched, "tools").Array(), 1)
REDACTED

func TestApplyGrokCacheIdentityPreservesIneligibleClientToolFields(t *testing.T) {
	tests := []struct {
		name string
		body string
REDACTED{
		{
			name: "empty tools array",
			body: `{"model":"grok","tools":[]REDACTED`,
	REDACTED,
		{
			name: "null tools",
			body: `{"model":"grok","tools":nullREDACTED`,
	REDACTED,
		{
			name: "tool choice only",
			body: `{"model":"grok","tool_choice":{"type":"function","name":"lookup"REDACTEDREDACTED`,
	REDACTED,
		{
			name: "null tool choice",
			body: `{"model":"grok","tool_choice":nullREDACTED`,
	REDACTED,
		{
			name: "native tool with auto choice",
			body: `{"model":"grok","tools":[{"type":"web_search"REDACTED],"tool_choice":"auto"REDACTED`,
	REDACTED,
		{
			name: "function with required choice",
			body: `{"model":"grok","tools":[{"type":"function","name":"lookup"REDACTED],"tool_choice":"required"REDACTED`,
	REDACTED,
		{
			name: "function with none choice",
			body: `{"model":"grok","tools":[{"type":"function","name":"lookup"REDACTED],"tool_choice":"none"REDACTED`,
	REDACTED,
		{
			name: "function with specific choice",
			body: `{"model":"grok","tools":[{"type":"function","name":"lookup"REDACTED],"tool_choice":{"type":"function","name":"lookup"REDACTEDREDACTED`,
	REDACTED,
		{
			name: "function with object auto choice",
			body: `{"model":"grok","tools":[{"type":"function","name":"lookup"REDACTED],"tool_choice":{"type":"auto"REDACTEDREDACTED`,
	REDACTED,
		{
			name: "function mixed with unsupported tool",
			body: `{"model":"grok","tools":[{"type":"function","name":"lookup"REDACTED,{"type":"namespace","name":"client_tools"REDACTED],"tool_choice":"auto"REDACTED`,
	REDACTED,
		{
			name: "unsupported tool only",
			body: `{"model":"grok","tools":[{"type":"namespace","name":"client_tools"REDACTED]REDACTED`,
	REDACTED,
		{
			name: "chat completions function shape",
			body: `{"model":"grok","tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"REDACTEDREDACTEDREDACTED],"tool_choice":"auto"REDACTED`,
	REDACTED,
		{
			name: "incomplete responses function",
			body: `{"model":"grok","tools":[{"type":"function","parameters":{"type":"object"REDACTEDREDACTED]REDACTED`,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeTools := gjson.Get(tt.body, "tools")
			beforeChoice := gjson.Get(tt.body, "tool_choice")
			body, err := applyGrokResponsesCacheIdentity([]byte(tt.body), []byte(tt.body), "isolated-id", true)
		REDACTED
			require.Equal(t, "isolated-id", gjson.GetBytes(body, "prompt_cache_key").String())
			require.Equal(t, beforeTools.Exists(), gjson.GetBytes(body, "tools").Exists())
			require.Equal(t, beforeTools.Raw, gjson.GetBytes(body, "tools").Raw)
			require.Equal(t, beforeChoice.Exists(), gjson.GetBytes(body, "tool_choice").Exists())
			require.Equal(t, beforeChoice.Raw, gjson.GetBytes(body, "tool_choice").Raw)
	REDACTED)
REDACTED
REDACTED

func TestApplyGrokCacheIdentityUsesPreSanitizationToolIntent(t *testing.T) {
	tests := []struct {
		name       string
		intentBody string
REDACTED{
		{
			name:       "unsupported tools removed by sanitizer",
			intentBody: `{"model":"grok","tools":[{"type":"namespace","name":"client_tools"REDACTED]REDACTED`,
	REDACTED,
		{
			name:       "tool choice removed with unsupported tool",
			intentBody: `{"model":"grok","tools":[{"type":"namespace","name":"client_tools"REDACTED],"tool_choice":{"type":"namespace","name":"client_tools"REDACTEDREDACTED`,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is the shape apply receives after patchGrokResponsesBody has
			// removed unsupported tools and their associated tool_choice.
			patchedBody := []byte(`{"model":"grok-4.5","input":"hello"REDACTED`)
			body, err := applyGrokResponsesCacheIdentity(patchedBody, []byte(tt.intentBody), "isolated-id", true)

		REDACTED
			require.Equal(t, "isolated-id", gjson.GetBytes(body, "prompt_cache_key").String())
			require.False(t, gjson.GetBytes(body, "tools").Exists())
			require.False(t, gjson.GetBytes(body, "tool_choice").Exists())
	REDACTED)
REDACTED
REDACTED

func TestApplyGrokCacheIdentityWithoutFreeTierRoutingOnlyWritesIdentity(t *testing.T) {
	sourceBody := []byte(`{"model":"grok-4.5","input":"hello"REDACTED`)
	body, err := applyGrokResponsesCacheIdentity(sourceBody, sourceBody, "isolated-id", false)

REDACTED
	require.Equal(t, "isolated-id", gjson.GetBytes(body, "prompt_cache_key").String())
	require.False(t, gjson.GetBytes(body, "tools").Exists())
	require.False(t, gjson.GetBytes(body, "tool_choice").Exists())
REDACTED

func TestGrokCompactRequestSkipsCacheIdentityAndNativeTools(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newGrokCacheTestContext(701)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	body := []byte(`{"model":"grok","input":"compact this","prompt_cache_key":"raw-client-key"REDACTED`)

	identity := resolveGrokCacheIdentity(c, body, "", "grok-4.5")
	patched, err := applyGrokResponsesCacheIdentity(body, body, identity, true)

REDACTED
	require.Empty(t, identity)
	require.False(t, gjson.GetBytes(patched, "prompt_cache_key").Exists())
	require.False(t, gjson.GetBytes(patched, "tools").Exists())
	require.False(t, gjson.GetBytes(patched, "tool_choice").Exists())
REDACTED

func TestResolveGrokCacheIdentityConcurrentDeterminism(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const workers = 50
	body := []byte(`{"model":"grok","messages":[{"role":"system","content":"stable"REDACTED,{"role":"user","content":"hello"REDACTED]REDACTED`)
	identities := make(chan string, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			identities <- resolveGrokCacheIdentity(newGrokCacheTestContext(501), body, "", "grok-4.5")
	REDACTED()
REDACTED
	wg.Wait()
	close(identities)

	var first string
	for identity := range identities {
		if first == "" {
			first = identity
	REDACTED
		require.Equal(t, first, identity)
REDACTED
	require.NotEmpty(t, first)
REDACTED
