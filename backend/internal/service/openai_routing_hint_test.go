package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
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

	t.Run("api key is untouched", func(t *testing.T) {
		headers := make(http.Header)
		headers.Set(openAICodexRoutingHintHeader, "caller-owned")
		setOpenAICodexRoutingHint(headers, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED, "gpt-5.6", "priority")
		require.Equal(t, "caller-owned", headers.Get(openAICodexRoutingHintHeader))
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

func TestOpenAIWSConnPoolDoesNotReuseDifferentRoutingHints(t *testing.T) {
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
	flexHeaders := make(http.Header)
	flexHeaders.Set(openAICodexRoutingHintHeader, "model=gpt-5.6-codex;tier=flex")
	_, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account:            account,
		WSURL:              "wss://example.com/v1/responses",
		Headers:            flexHeaders,
		PreferredConnID:    priorityConnID,
		ForcePreferredConn: true,
REDACTED)
	require.ErrorIs(t, err, errOpenAIWSPreferredConnUnavailable)

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
