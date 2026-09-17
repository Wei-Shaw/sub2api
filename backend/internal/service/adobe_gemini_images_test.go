package service

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const adobeGeminiTinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func TestParseAdobeGeminiImageRequestTextAndImageConfig(t *testing.T) {
	body := []byte(`{
		"systemInstruction": {"parts": [{"text": "stay wholesome"}]},
		"contents": [{"role": "user", "parts": [{"text": "a red panda"}]}],
		"generationConfig": {
			"candidateCount": 2,
			"responseMimeType": "image/png",
			"imageConfig": {"aspectRatio": "16:9", "imageSize": "2K"}
		}
	}`)
	req, call, err := ParseAdobeGeminiImageRequest("models/nano-banana-pro", body)
	require.NoError(t, err)
	require.Equal(t, "nano-banana-pro", req.Model)
	require.Equal(t, "stay wholesome\na red panda", req.Prompt)
	require.Equal(t, 2, req.N)
	require.Equal(t, "png", req.OutputFormat)
	require.Equal(t, "2048x1152", req.Size)
	require.Equal(t, "2K", req.SizeTier)
	require.Equal(t, openAIImagesGenerationsEndpoint, req.Endpoint)
	require.Equal(t, "16:9", call.AspectRatio)
	require.Equal(t, "2K", call.ImageSize)

	relayBody, err := MarshalAdobeGeminiRelayImagesBody(req)
	require.NoError(t, err)
	require.Equal(t, "2048x1152", gjson.GetBytes(relayBody, "size").String())
}

func TestParseAdobeGeminiImageRequestSizeFromImageConfig(t *testing.T) {
	tests := []struct {
		name        string
		imageConfig string
		wantSize    string
		wantTier    string
	}{
		{name: "ratio only defaults to 1K", imageConfig: `{"aspectRatio":"9:16"}`, wantSize: "576x1024", wantTier: "1K"},
		{name: "size only stays square", imageConfig: `{"imageSize":"4K"}`, wantSize: "4096x4096", wantTier: "4K"},
		{name: "neither keeps auto", imageConfig: `{}`, wantSize: "", wantTier: normalizeOpenAIImageSizeTier("")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"contents":[{"parts":[{"text":"x"}]}],"generationConfig":{"imageConfig":` + tt.imageConfig + `}}`)
			req, _, err := ParseAdobeGeminiImageRequest("gpt-image-2", body)
			require.NoError(t, err)
			require.Equal(t, tt.wantSize, req.Size)
			require.Equal(t, tt.wantSize != "", req.ExplicitSize)
			require.Equal(t, tt.wantTier, req.SizeTier)
		})
	}

	_, _, err := ParseAdobeGeminiImageRequest("gpt-image-2", []byte(`{
		"contents": [{"parts": [{"text": "x"}]}],
		"generationConfig": {"imageConfig": {"aspectRatio": "wide"}}
	}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "aspectRatio")
}

func TestValidateAdobeGeminiModel(t *testing.T) {
	for _, model := range []string{"nano-banana-pro", "models/nano-banana-pro", "gpt-4o-image", "gpt-image-2", "firefly-nano-banana-pro", "my-banana"} {
		require.NoError(t, ValidateAdobeGeminiModel(model), model)
	}
	for _, model := range []string{"gemini-2.5-flash", "claude-sonnet-4-6", "dall-e-3", ""} {
		require.Error(t, ValidateAdobeGeminiModel(model), model)
	}
	require.Contains(t, ValidateAdobeGeminiModel("claude-sonnet-4-6").Error(), "image model")
}

func TestParseAdobeGeminiImageRequestInlineDataAndHTTPS(t *testing.T) {
	body := []byte(`{
		"contents": [{"role": "user", "parts": [
			{"text": "make it night"},
			{"inlineData": {"mimeType": "image/png", "data": "` + adobeGeminiTinyPNG + `"}},
			{"fileData": {"fileUri": "https://cdn.example.com/ref.png"}}
		]}]
	}`)
	req, _, err := ParseAdobeGeminiImageRequest("nano-banana-pro", body)
	require.NoError(t, err)
	require.Equal(t, openAIImagesEditsEndpoint, req.Endpoint)
	require.Len(t, req.Uploads, 1)
	decoded, err := base64.StdEncoding.DecodeString(adobeGeminiTinyPNG)
	require.NoError(t, err)
	require.Equal(t, decoded, req.Uploads[0].Data)
	require.Equal(t, []string{"https://cdn.example.com/ref.png"}, req.InputImageURLs)
}

func TestParseAdobeGeminiImageRequestRejectsFilesURIAndTools(t *testing.T) {
	_, _, err := ParseAdobeGeminiImageRequest("nano-banana-pro", []byte(`{
		"contents": [{"parts": [{"fileData": {"fileUri": "files/abc"}}]}]
	}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "https")

	_, _, err = ParseAdobeGeminiImageRequest("nano-banana-pro", []byte(`{
		"contents": [{"parts": [{"text": "hi"}]}],
		"tools": [{"googleSearch": {}}]
	}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "tools")

	_, _, err = ParseAdobeGeminiImageRequest("nano-banana-pro", []byte(`{
		"contents": [{"parts": [{"text": "hi"}]}],
		"generationConfig": {"responseModalities": ["TEXT"]}
	}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "IMAGE")
}

func TestBuildGeminiGenerateContentResponseAndSSE(t *testing.T) {
	decoded, err := base64.StdEncoding.DecodeString(adobeGeminiTinyPNG)
	require.NoError(t, err)

	body, err := BuildGeminiGenerateContentResponse("nano-banana-pro", AdobeGeminiImagesFromBytes([][]byte{decoded, decoded}))
	require.NoError(t, err)
	require.Equal(t, "nano-banana-pro", gjson.GetBytes(body, "modelVersion").String())
	// candidateCount 语义：每张图一个 candidate，各带 index。
	candidates := gjson.GetBytes(body, "candidates").Array()
	require.Len(t, candidates, 2)
	for i, candidate := range candidates {
		require.True(t, candidate.Get("index").Exists())
		require.Equal(t, int64(i), candidate.Get("index").Int())
		require.Equal(t, "model", candidate.Get("content.role").String())
		require.Equal(t, "STOP", candidate.Get("finishReason").String())
		require.Len(t, candidate.Get("content.parts").Array(), 1)
		require.Equal(t, "image/png", candidate.Get("content.parts.0.inlineData.mimeType").String())
		require.Equal(t, adobeGeminiTinyPNG, candidate.Get("content.parts.0.inlineData.data").String())
	}

	sse := EncodeGeminiGenerateContentSSE(body)
	require.True(t, strings.HasPrefix(string(sse), "data: "))
	require.True(t, strings.HasSuffix(string(sse), "\n\n"))
	require.NotContains(t, string(sse), "[DONE]")
	frame := strings.TrimPrefix(strings.TrimSpace(string(sse)), "data: ")
	require.JSONEq(t, string(body), frame)
}

func TestBuildGeminiGenerateContentResponseFileDataAndEmpty(t *testing.T) {
	body, err := BuildGeminiGenerateContentResponse("gpt-image-2", []AdobeGeminiImage{
		{FileURI: "https://cdn.example.com/a.png", MimeType: "image/png"},
		{FileURI: "https://cdn.example.com/b.jpg"},
	})
	require.NoError(t, err)
	require.Equal(t, "https://cdn.example.com/a.png", gjson.GetBytes(body, "candidates.0.content.parts.0.fileData.fileUri").String())
	require.Equal(t, "image/png", gjson.GetBytes(body, "candidates.0.content.parts.0.fileData.mimeType").String())
	require.False(t, gjson.GetBytes(body, "candidates.0.content.parts.0.inlineData").Exists())
	require.False(t, gjson.GetBytes(body, "candidates.1.content.parts.0.fileData.mimeType").Exists())
	require.Equal(t, int64(1), gjson.GetBytes(body, "candidates.1.index").Int())

	_, err = BuildGeminiGenerateContentResponse("gpt-image-2", nil)
	require.Error(t, err)
	_, err = BuildGeminiGenerateContentResponse("gpt-image-2", []AdobeGeminiImage{{}})
	require.Error(t, err)
}

func TestOpenAIImagesJSONToGeminiImagesFromB64(t *testing.T) {
	decoded, err := base64.StdEncoding.DecodeString(adobeGeminiTinyPNG)
	require.NoError(t, err)
	payload, err := json.Marshal(map[string]any{
		"data": []map[string]string{{"b64_json": adobeGeminiTinyPNG}},
	})
	require.NoError(t, err)
	images, err := OpenAIImagesJSONToGeminiImages(payload, "")
	require.NoError(t, err)
	require.Equal(t, []AdobeGeminiImage{{Data: decoded}}, images)
}

// 回填失败只剩 url 的项降级为 fileData，不二次下载；两者都没有的项被丢弃。
func TestOpenAIImagesJSONToGeminiImagesDegradesAndDrops(t *testing.T) {
	decoded, err := base64.StdEncoding.DecodeString(adobeGeminiTinyPNG)
	require.NoError(t, err)
	payload := []byte(`{"data":[
		{"b64_json":"` + adobeGeminiTinyPNG + `"},
		{"url":"http://cdn.example.com/out.png"},
		{"b64_json":"!!!"},
		{"url":"ftp://cdn.example.com/out.png"},
		{"url":"data:image/png;base64,` + adobeGeminiTinyPNG + `"},
		{"b64_json":"!!!","url":"https://cdn.example.com/fallback.jpg"}
	]}`)
	images, err := OpenAIImagesJSONToGeminiImages(payload, "jpeg")
	require.NoError(t, err)
	require.Equal(t, []AdobeGeminiImage{
		{Data: decoded},
		{FileURI: "http://cdn.example.com/out.png", MimeType: "image/jpeg"},
		{Data: decoded},
		{FileURI: "https://cdn.example.com/fallback.jpg", MimeType: "image/jpeg"},
	}, images)
}

func TestOpenAIImagesJSONToGeminiImagesRejectsUnusable(t *testing.T) {
	for _, body := range []string{
		``,
		`not json`,
		`{"data":[]}`,
		`{"data":[{"b64_json":"!!!"},{}]}`,
		`{"data":[{"url":"file:///etc/passwd"}]}`,
	} {
		_, err := OpenAIImagesJSONToGeminiImages([]byte(body), "png")
		require.Error(t, err, body)
	}
}

func TestAdobeGeminiModelsListShape(t *testing.T) {
	models := AdobeGeminiModels()
	require.NotEmpty(t, models)
	require.Equal(t, adobe.ImageModelIDs()[0], strings.TrimPrefix(models[0].Name, "models/"))
	require.Contains(t, models[0].SupportedGenerationMethods, "generateContent")
	require.Contains(t, models[0].SupportedGenerationMethods, "streamGenerateContent")

	model, ok := LookupAdobeGeminiModel("models/nano-banana-pro")
	require.True(t, ok)
	require.Equal(t, "models/nano-banana-pro", model.Name)

	_, ok = LookupAdobeGeminiModel("gemini-2.5-flash")
	require.False(t, ok)
}

func TestHandleOpenAIImagesNonStreamingResponseBuffersClientWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	ctx, sink := WithOpenAIImagesBufferedResponse(c.Request.Context())
	c.Request = c.Request.WithContext(ctx)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"created":1,"data":[{"b64_json":"aW1hZ2U="}]}`)),
	}
	svc := &OpenAIGatewayService{}
	_, count, _, err := svc.handleOpenAIImagesNonStreamingResponse(ctx, resp, c, nil, &OpenAIImagesRequest{})
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Empty(t, rec.Body.String())
	require.JSONEq(t, `{"created":1,"data":[{"b64_json":"aW1hZ2U="}]}`, string(sink.Body))
}
