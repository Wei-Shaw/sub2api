package service

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayServiceParseOpenAIImagesRequest_JSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat","size":"1024x1024","quality":"high","stream":trueREDACTED`)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{REDACTED
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
REDACTED
	require.NotNil(t, parsed)
	require.Equal(t, "/v1/images/generations", parsed.Endpoint)
	require.Equal(t, "gpt-image-2", parsed.Model)
	require.Equal(t, "draw a cat", parsed.Prompt)
	require.True(t, parsed.Stream)
	require.Equal(t, "1024x1024", parsed.Size)
	require.Equal(t, "1K", parsed.SizeTier)
	require.Equal(t, OpenAIImagesCapabilityNative, parsed.RequiredCapability)
	require.False(t, parsed.Multipart)
REDACTED

func TestOpenAIGatewayServiceParseOpenAIImagesRequest_MultipartEdit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("prompt", "replace background"))
	require.NoError(t, writer.WriteField("size", "1536x1024"))
	part, err := writer.CreateFormFile("image", "source.png")
REDACTED
	_, err = part.Write([]byte("fake-image-bytes"))
REDACTED
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{REDACTED
	parsed, err := svc.ParseOpenAIImagesRequest(c, body.Bytes())
REDACTED
	require.NotNil(t, parsed)
	require.Equal(t, "/v1/images/edits", parsed.Endpoint)
	require.True(t, parsed.Multipart)
	require.Equal(t, "gpt-image-2", parsed.Model)
	require.Equal(t, "replace background", parsed.Prompt)
	require.Equal(t, "1536x1024", parsed.Size)
	require.Equal(t, "2K", parsed.SizeTier)
	require.Len(t, parsed.Uploads, 1)
	require.Equal(t, OpenAIImagesCapabilityNative, parsed.RequiredCapability)
REDACTED

func TestOpenAIGatewayServiceParseOpenAIImagesRequest_MultipartEditWithMaskAndNativeOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("prompt", "replace foreground"))
	require.NoError(t, writer.WriteField("output_format", "png"))
	require.NoError(t, writer.WriteField("input_fidelity", "high"))
	require.NoError(t, writer.WriteField("output_compression", "80"))
	require.NoError(t, writer.WriteField("partial_images", "2"))

	imageHeader := make(textproto.MIMEHeader)
	imageHeader.Set("Content-Disposition", `form-data; name="image"; filename="source.png"`)
	imageHeader.Set("Content-Type", "image/png")
	imagePart, err := writer.CreatePart(imageHeader)
REDACTED
	_, err = imagePart.Write([]byte("source-image-bytes"))
REDACTED

	maskHeader := make(textproto.MIMEHeader)
	maskHeader.Set("Content-Disposition", `form-data; name="mask"; filename="mask.png"`)
	maskHeader.Set("Content-Type", "image/png")
	maskPart, err := writer.CreatePart(maskHeader)
REDACTED
	_, err = maskPart.Write([]byte("mask-image-bytes"))
REDACTED

	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{REDACTED
	parsed, err := svc.ParseOpenAIImagesRequest(c, body.Bytes())
REDACTED
	require.NotNil(t, parsed)
	require.Len(t, parsed.Uploads, 1)
	require.NotNil(t, parsed.MaskUpload)
	require.True(t, parsed.HasMask)
	require.Equal(t, "png", parsed.OutputFormat)
	require.Equal(t, "high", parsed.InputFidelity)
	require.NotNil(t, parsed.OutputCompression)
	require.Equal(t, 80, *parsed.OutputCompression)
	require.NotNil(t, parsed.PartialImages)
	require.Equal(t, 2, *parsed.PartialImages)
	require.Equal(t, OpenAIImagesCapabilityNative, parsed.RequiredCapability)
REDACTED

func TestOpenAIGatewayServiceParseOpenAIImagesRequest_PromptOnlyDefaultsRemainBasic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"prompt":"draw a cat"REDACTED`)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{REDACTED
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
REDACTED
	require.NotNil(t, parsed)
	require.Equal(t, "gpt-image-2", parsed.Model)
	require.Equal(t, OpenAIImagesCapabilityBasic, parsed.RequiredCapability)
REDACTED

func TestOpenAIGatewayServiceParseOpenAIImagesRequest_ExplicitSizeRequiresNativeCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"prompt":"draw a cat","size":"1024x1024"REDACTED`)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{REDACTED
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
REDACTED
	require.NotNil(t, parsed)
	require.Equal(t, OpenAIImagesCapabilityNative, parsed.RequiredCapability)
REDACTED

func TestOpenAIGatewayServiceParseOpenAIImagesRequest_RejectsNonImageModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","prompt":"draw a cat"REDACTED`)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{REDACTED
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.Nil(t, parsed)
	require.ErrorContains(t, err, `images endpoint requires an image model, got "gpt-5.4"`)
REDACTED

func TestOpenAIGatewayServiceParseOpenAIImagesRequest_JSONEditURLs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"gpt-image-2",
		"prompt":"replace the background",
		"images":[{"image_url":"https://example.com/source.png"REDACTED],
		"mask":{"image_url":"https://example.com/mask.png"REDACTED,
		"input_fidelity":"high",
		"output_compression":90,
		"partial_images":2,
		"response_format":"url"
REDACTED`)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{REDACTED
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
REDACTED
	require.NotNil(t, parsed)
	require.Equal(t, []string{"https://example.com/source.png"REDACTED, parsed.InputImageURLs)
	require.Equal(t, "https://example.com/mask.png", parsed.MaskImageURL)
	require.Equal(t, "high", parsed.InputFidelity)
	require.NotNil(t, parsed.OutputCompression)
	require.Equal(t, 90, *parsed.OutputCompression)
	require.NotNil(t, parsed.PartialImages)
	require.Equal(t, 2, *parsed.PartialImages)
	require.True(t, parsed.HasMask)
	require.Equal(t, OpenAIImagesCapabilityNative, parsed.RequiredCapability)
REDACTED

func TestCollectOpenAIImagePointers_RecognizesDirectAssets(t *testing.T) {
	items := collectOpenAIImagePointers([]byte(`{
		"revised_prompt": "cat astronaut",
		"parts": [
			{"b64_json":"QUJD"REDACTED,
			{"download_url":"https://files.example.com/image.png?sig=1"REDACTED,
			{"asset_pointer":"file-service://file_123"REDACTED
		]
REDACTED`))

	require.Len(t, items, 3)
	var sawBase64, sawURL, sawPointer bool
	for _, item := range items {
		if item.B64JSON == "QUJD" {
			sawBase64 = true
			require.Equal(t, "cat astronaut", item.Prompt)
	REDACTED
		if item.DownloadURL == "https://files.example.com/image.png?sig=1" {
			sawURL = true
	REDACTED
		if item.Pointer == "file-service://file_123" {
			sawPointer = true
	REDACTED
REDACTED
	require.True(t, sawBase64)
	require.True(t, sawURL)
	require.True(t, sawPointer)
REDACTED

func TestResolveOpenAIImageBytes_PrefersInlineBase64(t *testing.T) {
	data, err := resolveOpenAIImageBytes(context.Background(), nil, nil, "", openAIImagePointerInfo{
		B64JSON: "data:image/png;base64,QUJD",
REDACTED)
REDACTED
	require.Equal(t, []byte("ABC"), data)
REDACTED

func TestAccountSupportsOpenAIImageCapability_OAuthSupportsNative(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED

	require.True(t, account.SupportsOpenAIImageCapability(OpenAIImagesCapabilityBasic))
	require.True(t, account.SupportsOpenAIImageCapability(OpenAIImagesCapabilityNative))
REDACTED

func TestOpenAIGatewayServiceForwardImages_OAuthUsesResponsesAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat","size":"1024x1024","quality":"high"REDACTED`)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set("api_key", &APIKey{ID: 42REDACTED)

	svc := &OpenAIGatewayService{REDACTED
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
REDACTED

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"REDACTED,
				"X-Request-Id": []string{"req_img_123"REDACTED,
		REDACTED,
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.completed\",\"response\":{\"created_at\":1710000000,\"usage\":{\"input_tokens\":11,\"output_tokens\":22,\"input_tokens_details\":{\"cached_tokens\":3REDACTED,\"output_tokens_details\":{\"image_tokens\":7REDACTEDREDACTED,\"tool_usage\":{\"image_gen\":{\"images\":1REDACTEDREDACTED,\"output\":[{\"type\":\"image_generation_call\",\"result\":\"aGVsbG8=\",\"revised_prompt\":\"draw a cat\",\"output_format\":\"png\",\"quality\":\"high\",\"size\":\"1024x1024\"REDACTED]REDACTEDREDACTED\n\n" +
					"data: [DONE]\n\n",
			)),
	REDACTED,
REDACTED
	svc.httpUpstream = upstream

	account := &Account{
		ID:       1,
		Name:     "openai-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token":       "token-123",
			"chatgpt_account_id": "acct-123",
	REDACTED,
REDACTED

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "gpt-image-2", result.Model)
	require.Equal(t, "gpt-image-2", result.UpstreamModel)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 22, result.Usage.OutputTokens)
	require.Equal(t, 7, result.Usage.ImageOutputTokens)

	require.NotNil(t, upstream.lastReq)
	require.Equal(t, chatgptCodexURL, upstream.lastReq.URL.String())
	require.Equal(t, "chatgpt.com", upstream.lastReq.Host)
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Content-Type"))
	require.Equal(t, "text/event-stream", upstream.lastReq.Header.Get("Accept"))
	require.Equal(t, "acct-123", upstream.lastReq.Header.Get("chatgpt-account-id"))
	require.Equal(t, "responses=experimental", upstream.lastReq.Header.Get("OpenAI-Beta"))

	require.Equal(t, openAIImagesResponsesMainModel, gjson.GetBytes(upstream.lastBody, "model").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.Equal(t, "image_generation", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
	require.Equal(t, "generate", gjson.GetBytes(upstream.lastBody, "tools.0.action").String())
	require.Equal(t, "gpt-image-2", gjson.GetBytes(upstream.lastBody, "tools.0.model").String())
	require.Equal(t, "1024x1024", gjson.GetBytes(upstream.lastBody, "tools.0.size").String())
	require.Equal(t, "high", gjson.GetBytes(upstream.lastBody, "tools.0.quality").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tools.0.n").Exists())
	require.Equal(t, "draw a cat", gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String())

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "aGVsbG8=", gjson.Get(rec.Body.String(), "data.0.b64_json").String())
	require.Equal(t, "draw a cat", gjson.Get(rec.Body.String(), "data.0.revised_prompt").String())
REDACTED

func TestOpenAIGatewayServiceForwardImages_OAuthStreamingTransformsEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat","stream":true,"response_format":"url"REDACTED`)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{REDACTED
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
REDACTED

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"REDACTED,
				"X-Request-Id": []string{"req_img_stream"REDACTED,
		REDACTED,
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.image_generation_call.partial_image\",\"partial_image_b64\":\"cGFydGlhbA==\",\"partial_image_index\":0,\"output_format\":\"png\"REDACTED\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"created_at\":1710000001,\"usage\":{\"input_tokens\":5,\"output_tokens\":9,\"output_tokens_details\":{\"image_tokens\":4REDACTEDREDACTED,\"tool_usage\":{\"image_gen\":{\"images\":1REDACTEDREDACTED,\"output\":[{\"type\":\"image_generation_call\",\"result\":\"ZmluYWw=\",\"output_format\":\"png\"REDACTED]REDACTEDREDACTED\n\n" +
					"data: [DONE]\n\n",
			)),
	REDACTED,
REDACTED
	svc.httpUpstream = upstream

	account := &Account{
		ID:       2,
		Name:     "openai-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "token-123",
	REDACTED,
REDACTED

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")
REDACTED
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, 1, result.ImageCount)
	require.Contains(t, rec.Body.String(), "event: image_generation.partial_image")
	require.Contains(t, rec.Body.String(), "event: image_generation.completed")
	require.Contains(t, rec.Body.String(), "\"type\":\"image_generation.partial_image\"")
	require.Contains(t, rec.Body.String(), "\"type\":\"image_generation.completed\"")
	require.Contains(t, rec.Body.String(), "\"url\":\"data:image/png;base64,cGFydGlhbA==\"")
	require.Contains(t, rec.Body.String(), "\"url\":\"data:image/png;base64,ZmluYWw=\"")
REDACTED

func TestOpenAIGatewayServiceForwardImages_OAuthEditsMultipartUsesResponsesAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("prompt", "replace background with aurora"))
	require.NoError(t, writer.WriteField("input_fidelity", "high"))
	require.NoError(t, writer.WriteField("output_format", "webp"))
	require.NoError(t, writer.WriteField("quality", "high"))

	imageHeader := make(textproto.MIMEHeader)
	imageHeader.Set("Content-Disposition", `form-data; name="image"; filename="source.png"`)
	imageHeader.Set("Content-Type", "image/png")
	imagePart, err := writer.CreatePart(imageHeader)
REDACTED
	_, err = imagePart.Write([]byte("png-image-content"))
REDACTED

	maskHeader := make(textproto.MIMEHeader)
	maskHeader.Set("Content-Disposition", `form-data; name="mask"; filename="mask.png"`)
	maskHeader.Set("Content-Type", "image/png")
	maskPart, err := writer.CreatePart(maskHeader)
REDACTED
	_, err = maskPart.Write([]byte("png-mask-content"))
REDACTED

	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set("api_key", &APIKey{ID: 100REDACTED)

	svc := &OpenAIGatewayService{REDACTED
	parsed, err := svc.ParseOpenAIImagesRequest(c, body.Bytes())
REDACTED

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"REDACTED,
				"X-Request-Id": []string{"req_img_edit_123"REDACTED,
		REDACTED,
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.completed\",\"response\":{\"created_at\":1710000002,\"usage\":{\"input_tokens\":13,\"output_tokens\":21,\"output_tokens_details\":{\"image_tokens\":8REDACTEDREDACTED,\"tool_usage\":{\"image_gen\":{\"images\":1REDACTEDREDACTED,\"output\":[{\"type\":\"image_generation_call\",\"result\":\"ZWRpdGVk\",\"revised_prompt\":\"replace background with aurora\",\"output_format\":\"webp\",\"quality\":\"high\"REDACTED]REDACTEDREDACTED\n\n" +
					"data: [DONE]\n\n",
			)),
	REDACTED,
REDACTED
	svc.httpUpstream = upstream

	account := &Account{
		ID:       3,
		Name:     "openai-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "token-123",
	REDACTED,
REDACTED

	result, err := svc.ForwardImages(context.Background(), c, account, body.Bytes(), parsed, "")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, "gpt-image-2", gjson.GetBytes(upstream.lastBody, "tools.0.model").String())
	require.Equal(t, "edit", gjson.GetBytes(upstream.lastBody, "tools.0.action").String())
	require.Equal(t, "high", gjson.GetBytes(upstream.lastBody, "tools.0.input_fidelity").String())
	require.Equal(t, "webp", gjson.GetBytes(upstream.lastBody, "tools.0.output_format").String())
	require.True(t, strings.HasPrefix(gjson.GetBytes(upstream.lastBody, "input.0.content.1.image_url").String(), "data:image/png;base64,"))
	require.True(t, strings.HasPrefix(gjson.GetBytes(upstream.lastBody, "tools.0.input_image_mask.image_url").String(), "data:image/png;base64,"))
	require.Equal(t, "replace background with aurora", gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String())
	require.Equal(t, "ZWRpdGVk", gjson.Get(rec.Body.String(), "data.0.b64_json").String())
	require.Equal(t, "replace background with aurora", gjson.Get(rec.Body.String(), "data.0.revised_prompt").String())
REDACTED

func TestOpenAIGatewayServiceForwardImages_OAuthEditsStreamingTransformsEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"gpt-image-2",
		"prompt":"replace background with aurora",
		"images":[{"image_url":"https://example.com/source.png"REDACTED],
		"mask":{"image_url":"https://example.com/mask.png"REDACTED,
		"stream":true,
		"response_format":"url"
REDACTED`)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{REDACTED
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
REDACTED

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"REDACTED,
		REDACTED,
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.image_generation_call.partial_image\",\"partial_image_b64\":\"cGFydGlhbA==\",\"partial_image_index\":0,\"output_format\":\"webp\"REDACTED\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"created_at\":1710000003,\"usage\":{\"input_tokens\":7,\"output_tokens\":10,\"output_tokens_details\":{\"image_tokens\":5REDACTEDREDACTED,\"tool_usage\":{\"image_gen\":{\"images\":1REDACTEDREDACTED,\"output\":[{\"type\":\"image_generation_call\",\"result\":\"ZWRpdGVk\",\"revised_prompt\":\"replace background with aurora\",\"output_format\":\"webp\"REDACTED]REDACTEDREDACTED\n\n" +
					"data: [DONE]\n\n",
			)),
	REDACTED,
REDACTED
	svc.httpUpstream = upstream

	account := &Account{
		ID:       4,
		Name:     "openai-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "token-123",
	REDACTED,
REDACTED

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, "edit", gjson.GetBytes(upstream.lastBody, "tools.0.action").String())
	require.Equal(t, "https://example.com/source.png", gjson.GetBytes(upstream.lastBody, "input.0.content.1.image_url").String())
	require.Equal(t, "https://example.com/mask.png", gjson.GetBytes(upstream.lastBody, "tools.0.input_image_mask.image_url").String())
	require.Contains(t, rec.Body.String(), "event: image_edit.partial_image")
	require.Contains(t, rec.Body.String(), "event: image_edit.completed")
	require.Contains(t, rec.Body.String(), "\"type\":\"image_edit.partial_image\"")
	require.Contains(t, rec.Body.String(), "\"type\":\"image_edit.completed\"")
	require.Contains(t, rec.Body.String(), "\"url\":\"data:image/webp;base64,cGFydGlhbA==\"")
	require.Contains(t, rec.Body.String(), "\"url\":\"data:image/webp;base64,ZWRpdGVk\"")
REDACTED

func TestBuildOpenAIImagesResponsesRequest_RejectsMultipleImages(t *testing.T) {
	parsed := &OpenAIImagesRequest{
		Endpoint: openAIImagesGenerationsEndpoint,
		Model:    "gpt-image-2",
		Prompt:   "draw a cat",
		N:        2,
REDACTED

	body, err := buildOpenAIImagesResponsesRequest(parsed, "gpt-image-2")
REDACTED
	require.Nil(t, body)
	require.Contains(t, err.Error(), "only n=1")
REDACTED

func TestCollectOpenAIImagesFromResponsesBody_FallsBackToOutputItemDone(t *testing.T) {
	body := []byte(
		"data: {\"type\":\"response.created\",\"response\":{\"created_at\":1710000004REDACTEDREDACTED\n\n" +
			"data: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"ig_123\",\"type\":\"image_generation_call\",\"result\":\"aGVsbG8=\",\"revised_prompt\":\"draw a cat\",\"output_format\":\"png\",\"quality\":\"high\"REDACTEDREDACTED\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"created_at\":1710000004,\"tool_usage\":{\"image_gen\":{\"images\":1REDACTEDREDACTED,\"output\":[]REDACTEDREDACTED\n\n" +
			"data: [DONE]\n\n",
	)

	results, createdAt, usageRaw, firstMeta, foundFinal, err := collectOpenAIImagesFromResponsesBody(body)
REDACTED
	require.True(t, foundFinal)
	require.Equal(t, int64(1710000004), createdAt)
	require.Len(t, results, 1)
	require.Equal(t, "aGVsbG8=", results[0].Result)
	require.Equal(t, "draw a cat", results[0].RevisedPrompt)
	require.Equal(t, "png", firstMeta.OutputFormat)
	require.JSONEq(t, `{"images":1REDACTED`, string(usageRaw))
REDACTED

func TestOpenAIGatewayServiceForwardImages_OAuthStreamingHandlesOutputItemDoneFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat","stream":true,"response_format":"url"REDACTED`)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{REDACTED
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
REDACTED

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"REDACTED,
				"X-Request-Id": []string{"req_img_stream_output_item_done"REDACTED,
		REDACTED,
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"ig_123\",\"type\":\"image_generation_call\",\"result\":\"ZmluYWw=\",\"revised_prompt\":\"draw a cat\",\"output_format\":\"png\"REDACTEDREDACTED\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"created_at\":1710000005,\"usage\":{\"input_tokens\":5,\"output_tokens\":9,\"output_tokens_details\":{\"image_tokens\":4REDACTEDREDACTED,\"tool_usage\":{\"image_gen\":{\"images\":1REDACTEDREDACTED,\"output\":[]REDACTEDREDACTED\n\n" +
					"data: [DONE]\n\n",
			)),
	REDACTED,
REDACTED
	svc.httpUpstream = upstream

	account := &Account{
		ID:       5,
		Name:     "openai-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "token-123",
	REDACTED,
REDACTED

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")
REDACTED
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, 1, result.ImageCount)
	require.Contains(t, rec.Body.String(), "event: image_generation.completed")
	require.Contains(t, rec.Body.String(), "\"type\":\"image_generation.completed\"")
	require.Contains(t, rec.Body.String(), "\"url\":\"data:image/png;base64,ZmluYWw=\"")
	require.NotContains(t, rec.Body.String(), "event: error")
REDACTED
