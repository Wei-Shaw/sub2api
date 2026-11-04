package service

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestNativePartialArbitraryInputIsZero(t *testing.T) {
	for _, value := range []string{"-1", "0", "2", "4", "9999", "1.5", "1e999", `"hello"`, `""`, "null", "true", "{}", "[]"} {
		t.Run("json/"+value, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"gpt-image-2.5-sunburst","prompt":"make blue","stream":true,"partial_images":%s,"images":[{"image_url":"data:image/png;base64,dGVzdA=="}]}`, value))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/images/edits", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			parsed, err := (&OpenAIGatewayService{}).ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			require.NotNil(t, parsed.PartialImages)
			require.Zero(t, *parsed.PartialImages)
			require.True(t, supportsOpenAINativeImageOptions(parsed))
		})
	}
	for _, value := range []string{"", "-1", "0", "2", "9999", "1.5", "hello", "null", "{}"} {
		t.Run("multipart/"+value, func(t *testing.T) {
			var body bytes.Buffer
			w := multipart.NewWriter(&body)
			for k, v := range map[string]string{"model": "gpt-image-2.5-sunburst", "prompt": "make blue", "stream": "true", "partial_images": value} {
				require.NoError(t, w.WriteField(k, v))
			}
			part, err := w.CreateFormFile("image[]", "input.png")
			require.NoError(t, err)
			_, err = part.Write([]byte("fixture"))
			require.NoError(t, err)
			require.NoError(t, w.Close())
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/images/edits", bytes.NewReader(body.Bytes()))
			c.Request.Header.Set("Content-Type", w.FormDataContentType())
			parsed, err := (&OpenAIGatewayService{}).ParseOpenAIImagesRequest(c, body.Bytes())
			require.NoError(t, err)
			require.NotNil(t, parsed.PartialImages)
			require.Zero(t, *parsed.PartialImages)
			require.True(t, supportsOpenAINativeImageOptions(parsed))
		})
	}
}
