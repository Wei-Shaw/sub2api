//go:build unit

package service

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newImagesExtraPrefixesService(prefixes ...string) *OpenAIGatewayService {
	return &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		OpenAIImagesExtraModelPrefixes: prefixes,
	}}}
}

func parseImagesRequest(t *testing.T, svc *OpenAIGatewayService, model string) (*OpenAIImagesRequest, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	body := []byte(fmt.Sprintf(`{"model":%q,"prompt":"draw","size":"1024x1024"}`, model))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, openAIImagesGenerationsEndpoint, bytes.NewReader(body))
	return svc.ParseOpenAIImagesRequest(c, body)
}

func TestCompatibleImagesExtraModelPrefixes(t *testing.T) {
	svc := newImagesExtraPrefixesService("gemini-nano-banana-")

	parsed, err := parseImagesRequest(t, svc, "gemini-nano-banana-2.1")
	require.NoError(t, err)
	// 额外前缀与内置 gemini-*-image* 同等对待：只允许 api_key 账号承接。
	require.Equal(t, OpenAIImagesCapabilityAPIKey, parsed.RequiredCapability)
	require.Equal(t, OpenAIImagesCapabilityAPIKey, parsed.RequiredCapabilityForModel("gemini-nano-banana-2.1"))
	require.True(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}).
		SupportsOpenAIImageCapability(parsed.RequiredCapability))
	for _, typ := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		require.False(t, (&Account{Platform: PlatformOpenAI, Type: typ}).
			SupportsOpenAIImageCapability(parsed.RequiredCapability))
	}
	// 不能进入原生 Responses 图像归一化。
	require.False(t, isOpenAIImageGenerationModel("gemini-nano-banana-2.1"))

	// 前缀按不区分大小写匹配。
	parsed, err = parseImagesRequest(t, svc, "Gemini-Nano-Banana-2.5")
	require.NoError(t, err)
	require.Equal(t, OpenAIImagesCapabilityAPIKey, parsed.RequiredCapability)

	// 未配置的前缀仍然被拒绝。
	for _, model := range []string{"gemini-nano-x2", "gemini-2.5-pro", "gpt-5.5", "nano-banana-2.1"} {
		_, err := parseImagesRequest(t, svc, model)
		require.ErrorContains(t, err, "images endpoint requires an image model")
	}
}

func TestCompatibleImagesExtraModelExactName(t *testing.T) {
	svc := newImagesExtraPrefixesService("image2-pro")
	parsed, err := parseImagesRequest(t, svc, "image2-pro-async")
	require.NoError(t, err)
	require.Equal(t, OpenAIImagesCapabilityAPIKey, parsed.RequiredCapability)
}

func TestCompatibleImagesNoExtraPrefixesKeepsBuiltinGate(t *testing.T) {
	for _, svc := range []*OpenAIGatewayService{
		nil,
		{},
		{cfg: &config.Config{}},
		newImagesExtraPrefixesService("", "  "),
	} {
		_, err := parseImagesRequest(t, svc, "gemini-nano-banana-2.1")
		require.ErrorContains(t, err, "images endpoint requires an image model")
		// 内置三族不受影响。
		_, err = parseImagesRequest(t, svc, "gemini-2.5-flash-image")
		require.NoError(t, err)
		_, err = parseImagesRequest(t, svc, "gpt-image-2")
		require.NoError(t, err)
	}
}

func TestImagesModelMatchesExtraPrefix(t *testing.T) {
	require.True(t, imagesModelMatchesExtraPrefix("gemini-nano-banana-2.1", []string{"gemini-nano-banana-"}))
	require.True(t, imagesModelMatchesExtraPrefix("  Nano-Banana-X  ", []string{"nano-banana"}))
	require.False(t, imagesModelMatchesExtraPrefix("", []string{"nano"}))
	require.False(t, imagesModelMatchesExtraPrefix("gpt-image-2", nil))
	require.False(t, imagesModelMatchesExtraPrefix("banana", []string{"nano"}))
}
