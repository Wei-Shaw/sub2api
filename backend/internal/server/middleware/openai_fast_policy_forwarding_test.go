package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAPIKeyAuthForwardsUserScopedOpenAIFastPolicyToUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamBodies := make(chan []byte, 2)
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
	REDACTED
		upstreamBodies <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_test","object":"response","model":"gpt-5","status":"completed","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2REDACTEDREDACTED`))
REDACTED))
	defer upstreamServer.Close()

	settings := &service.OpenAIFastPolicySettings{
		Rules: []service.OpenAIFastPolicyRule{
			{
				ServiceTier: service.OpenAIFastTierPriority,
				Action:      service.BetaPolicyActionFilter,
				Scope:       service.BetaPolicyScopeAll,
		REDACTED,
			{
				ServiceTier: service.OpenAIFastTierPriority,
				Action:      service.BetaPolicyActionPass,
				Scope:       service.BetaPolicyScopeAll,
				UserIDs:     []int64{42REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	settingsJSON, err := json.Marshal(settings)
REDACTED

	cfg := &config.Config{RunMode: config.RunModeSimpleREDACTED
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true

	settingService := service.NewSettingService(&openAIFastPolicyForwardingSettingRepo{
		value: string(settingsJSON),
REDACTED, cfg)
	gatewayService := service.NewOpenAIGatewayService(
		nil, nil, nil, nil, nil, nil, nil, cfg,
		nil, nil, nil, nil, nil, &openAIFastPolicyForwardingHTTPUpstream{client: upstreamServer.Client()REDACTED,
		nil, nil, nil, nil, nil, nil, settingService, nil,
	)

	groupID := int64(101)
	group := &service.Group{
		ID:       groupID,
		Name:     "openai",
		Status:   service.StatusActive,
		Platform: service.PlatformOpenAI,
		Hydrated: true,
REDACTED
	apiKeys := map[string]*service.APIKey{
		"key-user-42": newOpenAIFastPolicyForwardingAPIKey(1, "key-user-42", 42, groupID, group),
		"key-user-43": newOpenAIFastPolicyForwardingAPIKey(2, "key-user-43", 43, groupID, group),
REDACTED
	apiKeyService := service.NewAPIKeyService(&openAIFastPolicyForwardingAPIKeyRepo{apiKeys: apiKeysREDACTED, nil, nil, nil, nil, nil, cfg)
	account := &service.Account{
		ID:          900,
		Name:        "openai-upstream",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": upstreamServer.URL,
	REDACTED,
		Extra: map[string]any{"use_responses_api": trueREDACTED,
REDACTED

	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
	router.POST("/v1/responses", func(c *gin.Context) {
		body, readErr := io.ReadAll(c.Request.Body)
		if readErr != nil {
			c.Status(http.StatusBadRequest)
			return
	REDACTED
		service.SetOpenAIClientTransport(c, service.OpenAIClientTransportHTTP)
		if _, forwardErr := gatewayService.Forward(c.Request.Context(), c, account, body); forwardErr != nil {
			c.Status(http.StatusBadGateway)
			return
	REDACTED
		c.Status(http.StatusOK)
REDACTED)

	send := func(apiKey string) {
		request := httptest.NewRequest(
			http.MethodPost,
			"/v1/responses",
			bytes.NewBufferString(`{"model":"gpt-5","stream":false,"service_tier":"priority","input":"hi"REDACTED`),
		)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("x-api-key", apiKey)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code)
REDACTED

	send("key-user-42")
	send("key-user-43")

	allowedUserBody := <-upstreamBodies
	otherUserBody := <-upstreamBodies
	require.Equal(t, service.OpenAIFastTierPriority, gjson.GetBytes(allowedUserBody, "service_tier").String())
	require.False(t, gjson.GetBytes(otherUserBody, "service_tier").Exists())
REDACTED

func newOpenAIFastPolicyForwardingAPIKey(id int64, key string, userID, groupID int64, group *service.Group) *service.APIKey {
	return &service.APIKey{
		ID:      id,
		UserID:  userID,
		Key:     key,
		Status:  service.StatusActive,
		GroupID: &groupID,
		User: &service.User{
			ID:          userID,
			Role:        service.RoleUser,
			Status:      service.StatusActive,
			Balance:     10,
			Concurrency: 1,
	REDACTED,
		Group: group,
REDACTED
REDACTED

type openAIFastPolicyForwardingAPIKeyRepo struct {
	service.APIKeyRepository
	apiKeys map[string]*service.APIKey
REDACTED

func (r *openAIFastPolicyForwardingAPIKeyRepo) GetByKeyForAuth(_ context.Context, key string) (*service.APIKey, error) {
	apiKey, ok := r.apiKeys[key]
	if !ok {
		return nil, service.ErrAPIKeyNotFound
REDACTED
	clone := *apiKey
	return &clone, nil
REDACTED

func (r *openAIFastPolicyForwardingAPIKeyRepo) UpdateLastUsed(context.Context, int64, time.Time) error {
	return nil
REDACTED

type openAIFastPolicyForwardingSettingRepo struct {
	service.SettingRepository
	value string
REDACTED

func (r *openAIFastPolicyForwardingSettingRepo) GetValue(context.Context, string) (string, error) {
	return r.value, nil
REDACTED

type openAIFastPolicyForwardingHTTPUpstream struct {
	client *http.Client
REDACTED

func (u *openAIFastPolicyForwardingHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.client.Do(req)
REDACTED

func (u *openAIFastPolicyForwardingHTTPUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
REDACTED
