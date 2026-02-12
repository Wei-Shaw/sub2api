package service

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExtractOpenAIRequestMetaFromBody(t *testing.T) {
	tests := []struct {
		name          string
		body          []byte
		wantModel     string
		wantStream    bool
		wantPromptKey string
REDACTED{
		{
			name:          "完整字段",
			body:          []byte(`{"model":"gpt-5","stream":true,"prompt_cache_key":" ses-1 "REDACTED`),
			wantModel:     "gpt-5",
			wantStream:    true,
			wantPromptKey: "ses-1",
	REDACTED,
		{
			name:          "缺失可选字段",
			body:          []byte(`{"model":"gpt-4"REDACTED`),
			wantModel:     "gpt-4",
			wantStream:    false,
			wantPromptKey: "",
	REDACTED,
		{
			name:          "空请求体",
			body:          nil,
			wantModel:     "",
			wantStream:    false,
			wantPromptKey: "",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, stream, promptKey := extractOpenAIRequestMetaFromBody(tt.body)
			require.Equal(t, tt.wantModel, model)
			require.Equal(t, tt.wantStream, stream)
			require.Equal(t, tt.wantPromptKey, promptKey)
	REDACTED)
REDACTED
REDACTED

func TestExtractOpenAIReasoningEffortFromBody(t *testing.T) {
	tests := []struct {
		name      string
		body      []byte
		model     string
		wantNil   bool
		wantValue string
REDACTED{
		{
			name:      "优先读取 reasoning.effort",
			body:      []byte(`{"reasoning":{"effort":"medium"REDACTEDREDACTED`),
			model:     "gpt-5-high",
			wantNil:   false,
			wantValue: "medium",
	REDACTED,
		{
			name:      "兼容 reasoning_effort",
			body:      []byte(`{"reasoning_effort":"x-high"REDACTED`),
			model:     "",
			wantNil:   false,
			wantValue: "xhigh",
	REDACTED,
		{
			name:    "minimal 归一化为空",
			body:    []byte(`{"reasoning":{"effort":"minimal"REDACTEDREDACTED`),
			model:   "gpt-5-high",
			wantNil: true,
	REDACTED,
		{
			name:      "缺失字段时从模型后缀推导",
			body:      []byte(`{"input":"hi"REDACTED`),
			model:     "gpt-5-high",
			wantNil:   false,
			wantValue: "high",
	REDACTED,
		{
			name:    "未知后缀不返回",
			body:    []byte(`{"input":"hi"REDACTED`),
			model:   "gpt-5-unknown",
			wantNil: true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractOpenAIReasoningEffortFromBody(tt.body, tt.model)
			if tt.wantNil {
				require.Nil(t, got)
				return
		REDACTED
			require.NotNil(t, got)
			require.Equal(t, tt.wantValue, *got)
	REDACTED)
REDACTED
REDACTED

func TestGetOpenAIRequestBodyMap_UsesContextCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	cached := map[string]any{"model": "cached-model", "stream": trueREDACTED
	c.Set(OpenAIParsedRequestBodyKey, cached)

	got, err := getOpenAIRequestBodyMap(c, []byte(`{invalid-json`))
REDACTED
	require.Equal(t, cached, got)
REDACTED

func TestGetOpenAIRequestBodyMap_ParseErrorWithoutCache(t *testing.T) {
	_, err := getOpenAIRequestBodyMap(nil, []byte(`{invalid-json`))
REDACTED
	require.Contains(t, err.Error(), "parse request")
REDACTED
