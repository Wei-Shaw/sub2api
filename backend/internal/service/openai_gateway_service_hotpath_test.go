package service

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIRequestView_ExtractsRawScalars(t *testing.T) {
	view := newOpenAIRequestView([]byte(`{"model":" gpt-5 ","stream":true,"prompt_cache_key":" ses-1 ","previous_response_id":" resp-1 ","service_tier":" fast ","reasoning":{"effort":" medium "REDACTEDREDACTED`))

	require.Equal(t, "gpt-5", view.Model)
	require.True(t, view.Stream)
	require.Equal(t, "ses-1", view.PromptCacheKey)
	require.Equal(t, "resp-1", view.PreviousResponseID)
	require.Equal(t, "fast", view.ServiceTier)
	require.Equal(t, "medium", view.ReasoningEffort)
REDACTED

func TestOpenAIRequestView_DecodeKeepsFullMapBehavior(t *testing.T) {
	view := newOpenAIRequestView([]byte(`{"model":"gpt-5","stream":true,"input":[{"type":"message","content":"hi"REDACTED]REDACTED`))

	reqBody, err := view.Decode(nil)
REDACTED
	require.Equal(t, "gpt-5", reqBody["model"])
	require.IsType(t, []any{REDACTED, reqBody["input"])
REDACTED

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

func TestGetOpenAIRequestBodyMap_ParseError(t *testing.T) {
	_, err := getOpenAIRequestBodyMap(nil, []byte(`{invalid-json`))
REDACTED
	require.Contains(t, err.Error(), "parse request")
REDACTED

func TestGetOpenAIRequestBodyMap_DoesNotWriteContextCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	got, err := getOpenAIRequestBodyMap(c, []byte(`{"model":"gpt-5","stream":trueREDACTED`))
REDACTED
	require.Equal(t, "gpt-5", got["model"])
	require.Empty(t, c.Keys)
REDACTED

func TestSanitizeEmptyBase64InputImagesInOpenAIRequestBodyMap(t *testing.T) {
	var reqBody map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{
		"model":"gpt-5.4",
		"input":[
			{"role":"user","content":[
				{"type":"input_text","text":"Describe this"REDACTED,
				{"type":"input_image","image_url":"data:image/png;base64,   "REDACTED,
				{"type":"input_image","image_url":"data:image/png;base64,abc123"REDACTED
			]REDACTED,
			{"role":"user","content":[
				{"type":"input_image","image_url":"data:image/png;base64,"REDACTED
			]REDACTED,
			{"type":"input_image","image_url":"data:image/png;base64,"REDACTED,
			{"type":"input_image","image_url":"data:image/png;base64,top-level-valid"REDACTED
		]
REDACTED`), &reqBody))

	require.True(t, sanitizeEmptyBase64InputImagesInOpenAIRequestBodyMap(reqBody))

	normalized, err := json.Marshal(reqBody)
REDACTED
	require.JSONEq(t, `{
		"model":"gpt-5.4",
		"input":[
			{"role":"user","content":[
				{"type":"input_text","text":"Describe this"REDACTED,
				{"type":"input_image","image_url":"data:image/png;base64,abc123"REDACTED
			]REDACTED,
			{"type":"input_image","image_url":"data:image/png;base64,top-level-valid"REDACTED
		]
REDACTED`, string(normalized))
REDACTED

func TestSanitizeEmptyBase64InputImagesInOpenAIBody(t *testing.T) {
	body, changed, err := sanitizeEmptyBase64InputImagesInOpenAIBody([]byte(`{
		"model":"gpt-5.4",
		"stream":true,
		"input":[
			{"role":"user","content":[
				{"type":"input_text","text":"Describe this"REDACTED,
				{"type":"input_image","image_url":"data:image/png;base64,"REDACTED
			]REDACTED
		]
REDACTED`))
REDACTED
	require.True(t, changed)
	require.JSONEq(t, `{
		"model":"gpt-5.4",
		"stream":true,
		"input":[
			{"role":"user","content":[
				{"type":"input_text","text":"Describe this"REDACTED
			]REDACTED
		]
REDACTED`, string(body))
REDACTED
