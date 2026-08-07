package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetOpenAICodexRoutingHintCanonicalizesOfficialServiceTiers(t *testing.T) {
	oauthAccount := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	tests := []struct {
		name        string
		model       string
		serviceTier string
		want        string
REDACTED{
		{name: "fast alias", model: "gpt-5.6", serviceTier: " fast ", want: "model=gpt-5.6;tier=priority"REDACTED,
		{name: "priority", model: "gpt-5.6", serviceTier: "priority", want: "model=gpt-5.6;tier=priority"REDACTED,
		{name: "flex", model: "gpt-5.6", serviceTier: "flex", want: "model=gpt-5.6;tier=flex"REDACTED,
		{name: "explicit default sentinel", model: "gpt-5.6", serviceTier: "default", want: "model=gpt-5.6"REDACTED,
		{name: "omitted tier", model: "gpt-5.6", want: "model=gpt-5.6"REDACTED,
		{name: "auto is not expanded without catalog support", model: "gpt-5.6", serviceTier: "auto", want: "model=gpt-5.6"REDACTED,
		{name: "scale is not expanded without catalog support", model: "gpt-5.6", serviceTier: "scale", want: "model=gpt-5.6"REDACTED,
		{name: "unknown tier does not expand protocol", model: "gpt-5.6", serviceTier: "turbo", want: "model=gpt-5.6"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := make(http.Header)
			setOpenAICodexRoutingHint(headers, oauthAccount, tt.model, tt.serviceTier)
			require.Equal(t, tt.want, headers.Get(openAICodexRoutingHintHeader))
	REDACTED)
REDACTED

	t.Run("invalid header value is omitted", func(t *testing.T) {
		headers := make(http.Header)
		setOpenAICodexRoutingHint(headers, oauthAccount, "gpt-5.6\ninvalid", "priority")
		require.Empty(t, headers.Get(openAICodexRoutingHintHeader))
REDACTED)

	for _, model := range []string{"gpt-5.6;evil", "gpt=5.6"REDACTED {
		t.Run("delimiter in model is omitted: "+model, func(t *testing.T) {
			headers := make(http.Header)
			setOpenAICodexRoutingHint(headers, oauthAccount, model, "priority")
			require.Empty(t, headers.Get(openAICodexRoutingHintHeader))
	REDACTED)
REDACTED

	t.Run("api key strips gateway-owned hint in every key casing", func(t *testing.T) {
		headers := make(http.Header)
		headers[openAICodexRoutingHintHeader] = []string{"lowercase-spoof"REDACTED
		headers["X-Codex-Routing-Hint"] = []string{"canonical-spoof"REDACTED
		setOpenAICodexRoutingHint(headers, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED, "gpt-5.6", "priority")
		for key := range headers {
			require.False(t, strings.EqualFold(key, openAICodexRoutingHintHeader))
	REDACTED
REDACTED)

	t.Run("oauth replaces spoofed lowercase hint", func(t *testing.T) {
		headers := make(http.Header)
		headers[openAICodexRoutingHintHeader] = []string{"model=spoof;tier=flex"REDACTED
		setOpenAICodexRoutingHint(headers, oauthAccount, "gpt-5.6", "priority")
		require.Equal(t, "model=gpt-5.6;tier=priority", headers.Get(openAICodexRoutingHintHeader))
		require.Len(t, headers, 1)
REDACTED)
REDACTED

func TestOpenAIOAuthHTTPBuildersSendRoutingHintFromFinalBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oauthAccount := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"chatgpt_account_id": "test-account",
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{REDACTED

	tests := []struct {
		name string
		body []byte
		want string
REDACTED{
		{name: "fast", body: []byte(`{"model":"gpt-5.6-codex","service_tier":"fast"REDACTED`), want: "model=gpt-5.6-codex;tier=priority"REDACTED,
		{name: "flex", body: []byte(`{"model":"gpt-5.6-codex","service_tier":"flex"REDACTED`), want: "model=gpt-5.6-codex;tier=flex"REDACTED,
		{name: "default", body: []byte(`{"model":"gpt-5.6-codex","service_tier":"default"REDACTED`), want: "model=gpt-5.6-codex"REDACTED,
		{name: "omitted", body: []byte(`{"model":"gpt-5.6-codex"REDACTED`), want: "model=gpt-5.6-codex"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, passthrough := range []bool{false, trueREDACTED {
				mode := "ordinary"
				if passthrough {
					mode = "passthrough"
			REDACTED
				t.Run(mode, func(t *testing.T) {
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(tt.body))

					var req *http.Request
					var err error
					if passthrough {
						req, err = svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, oauthAccount, tt.body, "test-token")
				REDACTED else {
						req, err = svc.buildUpstreamRequest(context.Background(), c, oauthAccount, tt.body, "test-token", false, "", true)
				REDACTED
				REDACTED
					require.Equal(t, tt.want, req.Header.Get(openAICodexRoutingHintHeader))
			REDACTED)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestOpenAIHTTPPassthroughStripsOnlyOAuthLegacyResponsesBeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTED,
	REDACTED,
REDACTEDREDACTED
	body := []byte(`{"model":"gpt-5.6-codex","service_tier":"priority"REDACTED`)

	build := func(t *testing.T, account *Account, betaValues []string, rawLowercaseKey bool) http.Header {
	REDACTED
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
		if rawLowercaseKey {
			c.Request.Header["openai-beta"] = append([]string(nil), betaValues...)
	REDACTED else {
			for _, value := range betaValues {
				c.Request.Header.Add("OpenAI-Beta", value)
		REDACTED
	REDACTED

		req, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "test-token")
	REDACTED
		return req.Header
REDACTED

	oauth := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"chatgpt_account_id": "test-account",
	REDACTED,
REDACTED
	apiKey := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key": "test-api-key",
	REDACTED,
REDACTED

	t.Run("oauth legacy only is removed including raw lowercase key", func(t *testing.T) {
		headers := build(t, oauth, []string{"responses=experimental"REDACTED, true)
		require.Empty(t, headers.Values("OpenAI-Beta"))
REDACTED)

	t.Run("oauth mixed beta preserves independent tokens", func(t *testing.T) {
		headers := build(t, oauth, []string{
			"responses=experimental, future_feature=v1",
			"another_feature=v2, RESPONSES=EXPERIMENTAL",
	REDACTED, false)
		require.Equal(t, []string{"future_feature=v1", "another_feature=v2"REDACTED, headers.Values("OpenAI-Beta"))
REDACTED)

	t.Run("api key explicit beta remains caller controlled", func(t *testing.T) {
		headers := build(t, apiKey, []string{"responses=experimental, future_feature=v1"REDACTED, false)
		require.Equal(t, []string{"responses=experimental, future_feature=v1"REDACTED, headers.Values("OpenAI-Beta"))
REDACTED)
REDACTED

func TestBuildOpenAIWSHeadersSendsOAuthRoutingHintOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	svc := &OpenAIGatewayService{REDACTED
	decision := OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2REDACTED

	build := func(t *testing.T, account *Account, tier string) http.Header {
		headers, _, err := svc.buildOpenAIWSHeaders(
			context.Background(),
			c,
			account,
			"test-token",
			decision,
			true,
			"",
			"",
			"",
			"gpt-5.6-codex",
			tier,
		)
	REDACTED
		return headers
REDACTED

	oauthAccount := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"chatgpt_account_id": "test-account",
	REDACTED,
REDACTED
	require.Equal(t, "model=gpt-5.6-codex;tier=priority", build(t, oauthAccount, "fast").Get(openAICodexRoutingHintHeader))
	require.Equal(t, "model=gpt-5.6-codex", build(t, oauthAccount, "default").Get(openAICodexRoutingHintHeader))
	require.Empty(t, build(t, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED, "priority").Get(openAICodexRoutingHintHeader))
REDACTED

func TestOpenAIRoutingDiagnosticsUseFinalDerivedValuesOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureStructuredLog(t)
	defer restore()

	account := &Account{
		ID:       917,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"chatgpt_account_id": "chatgpt-account",
	REDACTED,
REDACTED
	body := []byte(`{"model":"gpt-5.6-codex","service_tier":"fast"REDACTED`)
	svc := &OpenAIGatewayService{REDACTED

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Authorization", "Bearer caller-secret")
	c.Request.Header.Set(openAICodexRoutingHintHeader, "model=caller-secret")
	_, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "oauth-secret", false, "", true)
REDACTED

	decision := OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2REDACTED
	_, _, err = svc.buildOpenAIWSHeaders(
		context.Background(), c, account, "oauth-secret", decision, true,
		"", "", "", "gpt-5.6-codex", "fast",
	)
REDACTED

	require.True(t, logSink.ContainsMessageAtLevel("openai routing decision", "debug"))
	require.True(t, logSink.ContainsFieldValue("account_id", "917"))
	require.True(t, logSink.ContainsFieldValue("final_model", "gpt-5.6-codex"))
	require.True(t, logSink.ContainsFieldValue("final_service_tier", "priority"))
	require.True(t, logSink.ContainsFieldValue("routing_hint_generated", "true"))
	require.True(t, logSink.ContainsFieldValue("transport", "http"))
	require.True(t, logSink.ContainsFieldValue("transport", string(OpenAIUpstreamTransportResponsesWebsocketV2)))
	require.True(t, logSink.ContainsFieldValue("ws_affinity_decision", "not_applicable"))
	require.True(t, logSink.ContainsFieldValue("ws_affinity_decision", "soft_routing_hint"))
	require.False(t, logSink.ContainsFieldValue("authorization", "caller-secret"))
	require.False(t, logSink.ContainsFieldValue("credentials", "oauth-secret"))
	require.False(t, logSink.ContainsFieldValue("routing_hint", "caller-secret"))
REDACTED

func TestOpenAIWSConnPoolPreferredContinuationIgnoresRoutingHintChanges(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2

	pool := newOpenAIWSConnPool(cfg)
	dialer := &openAIWSCountingDialer{REDACTED
	pool.setClientDialerForTest(dialer)
	account := &Account{ID: 913, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED

	acquire := func(t *testing.T, hint, preferred string, forcePreferred bool) *openAIWSConnLease {
	REDACTED
		headers := make(http.Header)
		if hint != "" {
			headers.Set(openAICodexRoutingHintHeader, hint)
	REDACTED
		lease, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
			Account:            account,
			WSURL:              "wss://example.com/v1/responses",
			Headers:            headers,
			PreferredConnID:    preferred,
			ForcePreferredConn: forcePreferred,
	REDACTED)
	REDACTED
		require.NotNil(t, lease)
		return lease
REDACTED

	standard := acquire(t, "model=gpt-5.6-codex", "", false)
	connID := standard.ConnID()
	standard.Release()

	priority := acquire(t, "model=gpt-5.6-codex;tier=priority", connID, true)
	require.True(t, priority.Reused())
	require.Equal(t, connID, priority.ConnID())
	priority.Release()

	standardAgain := acquire(t, "model=gpt-5.6-codex", connID, true)
	require.True(t, standardAgain.Reused())
	require.Equal(t, connID, standardAgain.ConnID())
	standardAgain.Release()

	require.Equal(t, 1, dialer.DialCount(), "routing hint is dial-time advisory, not continuation compatibility")
REDACTED

func TestOpenAIWSConnPoolUsesRoutingHintAsSoftDialAffinity(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 4
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 4

	pool := newOpenAIWSConnPool(cfg)
	dialer := &openAIWSCountingDialer{REDACTED
	pool.setClientDialerForTest(dialer)
	account := &Account{ID: 913, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED

	acquire := func(t *testing.T, hint string) *openAIWSConnLease {
		headers := make(http.Header)
		headers.Set(openAICodexRoutingHintHeader, hint)
		lease, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
			Account: account,
			WSURL:   "wss://example.com/v1/responses",
			Headers: headers,
	REDACTED)
	REDACTED
		require.NotNil(t, lease)
		return lease
REDACTED

	priority := acquire(t, "model=gpt-5.6-codex;tier=priority")
	priorityConnID := priority.ConnID()
	priority.Release()

	priorityAgain := acquire(t, "model=gpt-5.6-codex;tier=priority")
	require.True(t, priorityAgain.Reused())
	require.Equal(t, priorityConnID, priorityAgain.ConnID())
	priorityAgain.Release()

	flex := acquire(t, "model=gpt-5.6-codex;tier=flex")
	require.False(t, flex.Reused())
	require.NotEqual(t, priorityConnID, flex.ConnID())
	flex.Release()

	otherModel := acquire(t, "model=gpt-5.5-codex;tier=priority")
	require.False(t, otherModel.Reused())
	require.NotEqual(t, priorityConnID, otherModel.ConnID())
	otherModel.Release()

	defaultTier := acquire(t, "model=gpt-5.6-codex")
	require.False(t, defaultTier.Reused())
	require.NotEqual(t, priorityConnID, defaultTier.ConnID())
	defaultConnID := defaultTier.ConnID()
	defaultTier.Release()

	defaultAgain := acquire(t, "model=gpt-5.6-codex")
	require.True(t, defaultAgain.Reused())
	require.Equal(t, defaultConnID, defaultAgain.ConnID())
	defaultAgain.Release()

	require.Equal(t, 4, dialer.DialCount())
REDACTED
