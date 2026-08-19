//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardGrokChatViaResponsesDropsRedundantViewImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := grokChatInlineImageRequest()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 7991REDACTED)

	account := grokChatBridgeTestAccount(799)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: accountREDACTED,
REDACTEDREDACTED
	upstream := &httpUpstreamRecorder{resp: grokChatBridgeCompletedResponse("resp_chat_image", 0)REDACTED
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
REDACTED

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.lastReq.URL.String())
	require.Equal(t, "input_image", gjson.GetBytes(upstream.lastBody, "input.0.content.1.type").String())
	assertGrokUpstreamKeepsOtherToolAndDropsViewImage(t, upstream.lastBody, "tools.#(name==\"%s\")")
REDACTED

func TestForwardGrokRawChatDropsRedundantViewImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := grokChatInlineImageRequest()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	account := &Account{
		ID: 800, Platform: PlatformGrok, Type: AccountTypeAPIKey, Concurrency: 1,
REDACTED"api_key": "test-key", "base_url": "https://grok.example.test/v1"REDACTED,
REDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl","object":"chat.completion","model":"grok-4.6","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1REDACTEDREDACTED`,
		)),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstreamREDACTED

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "https://grok.example.test/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "image_url", gjson.GetBytes(upstream.lastBody, "messages.0.content.1.type").String())
	assertGrokUpstreamKeepsOtherToolAndDropsViewImage(t, upstream.lastBody, "tools.#(function.name==\"%s\")")
REDACTED

func TestForwardGrokMessagesDropsRedundantViewImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"grok-4.6","max_tokens":32,"stream":false,
		"messages":[{"role":"user","content":[
			{"type":"text","text":"What text is in this image?"REDACTED,
			{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AA=="REDACTEDREDACTED
		]REDACTED],
		"tools":[
			{"name":"view_image","input_schema":{"type":"object","properties":{"path":{"type":"string"REDACTEDREDACTEDREDACTEDREDACTED,
			{"name":"shell_command","input_schema":{"type":"object","properties":{"cmd":{"type":"string"REDACTEDREDACTEDREDACTEDREDACTED
		],
		"tool_choice":{"type":"auto"REDACTED
REDACTED`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 7992REDACTED)

	account := healthyGrokOAuthGatewayTestAccount(801, "access-token")
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: accountREDACTED,
REDACTEDREDACTED
	upstream := &httpUpstreamRecorder{resp: grokMessagesSSECompletedResponse("resp_messages_image", 0)REDACTED
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
REDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.lastReq.URL.String())
	require.Equal(t, "input_image", gjson.GetBytes(upstream.lastBody, "input.0.content.1.type").String())
	assertGrokUpstreamKeepsOtherToolAndDropsViewImage(t, upstream.lastBody, "tools.#(name==\"%s\")")
REDACTED

func TestStripRedundantGrokChatViewImageToolLeavesNonTargetRequestsByteExact(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
REDACTED{
		{
			name: "current turn has no inline image",
			body: `{"messages":[{"role":"user","content":"Inspect a local image"REDACTED],"tools":[{"type":"function","function":{"name":"view_image"REDACTEDREDACTED]REDACTED`,
	REDACTED,
		{
			name: "inline image is only historical",
			body: `{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="REDACTEDREDACTED]REDACTED,{"role":"assistant","content":"Done"REDACTED,{"role":"user","content":"Inspect another local image"REDACTED],"tools":[{"type":"function","function":{"name":"view_image"REDACTEDREDACTED]REDACTED`,
	REDACTED,
		{
			name: "view image is explicitly selected",
			body: `{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="REDACTEDREDACTED]REDACTED],"tools":[{"type":"function","function":{"name":"view_image"REDACTEDREDACTED],"tool_choice":{"type":"function","function":{"name":"view_image"REDACTEDREDACTEDREDACTED`,
	REDACTED,
		{
			name: "required with view image as the only tool",
			body: `{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="REDACTEDREDACTED]REDACTED],"tools":[{"type":"function","function":{"name":"view_image"REDACTEDREDACTED],"tool_choice":"required"REDACTED`,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := []byte(tt.body)
			patched, err := stripRedundantGrokChatViewImageTool(body)
		REDACTED
			require.Equal(t, body, patched)
	REDACTED)
REDACTED
REDACTED

func TestStripRedundantGrokChatViewImageToolDropsOnlyToolMetadata(t *testing.T) {
	t.Parallel()
	body := []byte(`{
		"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="REDACTEDREDACTED]REDACTED],
		"tools":[{"type":"function","function":{"name":"view_image"REDACTEDREDACTED],
		"tool_choice":"auto",
		"parallel_tool_calls":true
REDACTED`)

	patched, err := stripRedundantGrokChatViewImageTool(body)
REDACTED
	require.False(t, gjson.GetBytes(patched, "tools").Exists())
	require.False(t, gjson.GetBytes(patched, "tool_choice").Exists())
	require.False(t, gjson.GetBytes(patched, "parallel_tool_calls").Exists())
REDACTED

func grokChatInlineImageRequest() []byte {
	return []byte(`{
		"model":"grok-4.6",
		"messages":[{"role":"user","content":[
			{"type":"text","text":"What text is in this image?"REDACTED,
			{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="REDACTEDREDACTED
		]REDACTED],
		"stream":false,
		"tools":[
			{"type":"function","function":{"name":"view_image","parameters":{"type":"object","properties":{"path":{"type":"string"REDACTEDREDACTEDREDACTEDREDACTEDREDACTED,
			{"type":"function","function":{"name":"shell_command","parameters":{"type":"object","properties":{"cmd":{"type":"string"REDACTEDREDACTEDREDACTEDREDACTEDREDACTED
		],
		"tool_choice":"auto"
REDACTED`)
REDACTED

func assertGrokUpstreamKeepsOtherToolAndDropsViewImage(t *testing.T, body []byte, pathTemplate string) {
REDACTED
	require.False(t, gjson.GetBytes(body, strings.Replace(pathTemplate, "%s", "view_image", 1)).Exists(), string(body))
	require.True(t, gjson.GetBytes(body, strings.Replace(pathTemplate, "%s", "shell_command", 1)).Exists(), string(body))
REDACTED
