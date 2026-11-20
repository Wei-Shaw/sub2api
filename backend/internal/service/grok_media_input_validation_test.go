//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPrepareGrokImageEditSourceLimit(t *testing.T) {
	for _, format := range []string{"json", "multipart URLs", "multipart uploads", "multipart mixed"} {
		for _, count := range []int{3, 4, 5, 6} {
			t.Run(fmt.Sprintf("%s/%d", format, count), func(t *testing.T) {
				var body []byte
				contentType := "application/json"
				if format == "json" {
					images := make([]string, count-1)
					for index := range images {
						images[index] = fmt.Sprintf("https://example.test/%d.png", index+1)
					}
					var err error
					body, err = json.Marshal(map[string]any{
						"image": "https://example.test/0.png", "images": images,
						"mask": "https://example.test/mask.png",
					})
					require.NoError(t, err)
				} else {
					var buffer bytes.Buffer
					writer := multipart.NewWriter(&buffer)
					for index := 0; index < count; index++ {
						if format == "multipart URLs" || (format == "multipart mixed" && index%2 == 0) {
							require.NoError(t, writer.WriteField("image", fmt.Sprintf("https://example.test/%d.png", index)))
						} else {
							part, err := writer.CreateFormFile("image[]", fmt.Sprintf("%d.png", index))
							require.NoError(t, err)
							_, err = part.Write([]byte("\x89PNG\r\n\x1a\nsynthetic"))
							require.NoError(t, err)
						}
					}
					if format == "multipart URLs" {
						require.NoError(t, writer.WriteField("mask", "https://example.test/mask.png"))
					} else {
						part, err := writer.CreateFormFile("mask", "mask.png")
						require.NoError(t, err)
						_, err = part.Write([]byte("\x89PNG\r\n\x1a\nmask"))
						require.NoError(t, err)
					}
					require.NoError(t, writer.Close())
					body, contentType = buffer.Bytes(), writer.FormDataContentType()
				}
				out, outputType, err := prepareGrokMediaForwardBody(GrokMediaEndpointImagesEdits, body, contentType)
				if count > 5 {
					var validationErr *GrokMediaInputValidationError
					require.ErrorAs(t, err, &validationErr)
					require.Equal(t, "a maximum of 5 source images is supported for image edits", validationErr.Error())
					require.Nil(t, out)
					return
				}
				require.NoError(t, err)
				require.Equal(t, "application/json", outputType)
				if format == "json" {
					require.Len(t, gjson.GetBytes(out, "images").Array(), count-1)
				} else {
					require.Len(t, gjson.GetBytes(out, "images").Array(), count)
				}
				require.NotEmpty(t, gjson.GetBytes(out, "image.url").String())
				require.NotEmpty(t, gjson.GetBytes(out, "mask.url").String())
			})
		}
	}
}

func TestGrokMediaInternalErrorsAreNotInputValidation(t *testing.T) {
	for _, test := range []struct {
		name    string
		account *Account
	}{
		{"missing account", nil},
		{"wrong platform", &Account{Platform: PlatformOpenAI}},
		{"missing credential", &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey}},
		{"invalid base URL", &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test", "base_url": "://invalid"}}},
		{"missing OAuth provider", &Account{Platform: PlatformGrok, Type: AccountTypeOAuth}},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := &OpenAIGatewayService{}
			_, err := svc.ForwardGrokMedia(context.Background(), nil, test.account, GrokMediaEndpointImagesEdits, "", []byte(`{"images":[]}`), "application/json")
			require.Error(t, err)
			var validationErr *GrokMediaInputValidationError
			require.NotErrorAs(t, err, &validationErr)
		})
	}
}
