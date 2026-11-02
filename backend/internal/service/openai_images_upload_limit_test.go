package service

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func buildImagesEditMultipartBody(t *testing.T, imageSize int) ([]byte, string, []byte) {
	t.Helper()
	image := bytes.Repeat([]byte{0xAB}, imageSize)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("prompt", "replace background"))
	part, err := writer.CreateFormFile("image", "source.png")
	require.NoError(t, err)
	_, err = part.Write(image)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return body.Bytes(), writer.FormDataContentType(), image
}

func parseImagesEditMultipart(t *testing.T, body []byte, contentType string) (*OpenAIImagesRequest, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	svc := &OpenAIGatewayService{}
	return svc.ParseOpenAIImagesRequest(c, body)
}

func TestParseOpenAIImagesRequest_MultipartKeepsUploadAbove20MB(t *testing.T) {
	body, contentType, image := buildImagesEditMultipartBody(t, 21<<20)

	parsed, err := parseImagesEditMultipart(t, body, contentType)
	require.NoError(t, err)
	require.Len(t, parsed.Uploads, 1)
	require.Equal(t, len(image), len(parsed.Uploads[0].Data))
	require.True(t, bytes.Equal(image, parsed.Uploads[0].Data))
}

func TestParseOpenAIImagesRequest_MultipartRejectsUploadAbovePartLimit(t *testing.T) {
	body, contentType, _ := buildImagesEditMultipartBody(t, openAIImageMaxUploadPartSize+1)

	parsed, err := parseImagesEditMultipart(t, body, contentType)
	require.Nil(t, parsed)
	require.ErrorContains(t, err, `multipart field "image" exceeds the 50 MB per-part limit`)
}

func TestParseGrokMediaRequest_MultipartKeepsUploadAbove20MB(t *testing.T) {
	body, contentType, image := buildImagesEditMultipartBody(t, 21<<20)

	info := ParseGrokMediaRequest(contentType, body)
	require.Len(t, info.Uploads, 1)
	require.Equal(t, len(image), len(info.Uploads[0].Data))
}
